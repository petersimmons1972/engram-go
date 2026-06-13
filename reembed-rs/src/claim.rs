use anyhow::{Context, Result};
use pgvector::Vector;
use reqwest::Client as HttpClient;
use serde::Deserialize;
use serde_json::json;
use sqlx::{FromRow, PgPool};
use tokio::time;
use tracing::warn;

use crate::Config;

/// Validate that the embedding response is coherent with the claimed row set before any DB writes.
///
/// Contract (all-or-nothing):
/// - (a) `embeddings.len() == rows` — cardinality: the response must contain exactly one
///   embedding per claimed row. A shorter payload is truncation; a longer payload is a protocol
///   deviation. Both are rejected.
/// - (b) If `expected_dims` is `Some(d)`, every vector must have length `d`. Wrong dimensions
///   would cause the `::vector` UPDATE to fail at the DB layer; we surface it here instead so
///   we can reject the whole slice without partial writes.
///
/// Returns `Ok(())` if all conditions are met; `Err` with a descriptive message otherwise.
pub fn validate_embeddings(
    rows: usize,
    embeddings: &[Vec<f32>],
    expected_dims: Option<u32>,
) -> Result<()> {
    if embeddings.len() != rows {
        anyhow::bail!(
            "embedding cardinality mismatch: expected {} embeddings for {} claimed rows, got {}",
            rows,
            rows,
            embeddings.len()
        );
    }
    if let Some(dims) = expected_dims {
        let dims = dims as usize;
        for (i, vec) in embeddings.iter().enumerate() {
            if vec.len() != dims {
                anyhow::bail!(
                    "embedding dimension mismatch at index {}: expected {} dims, got {}",
                    i,
                    dims,
                    vec.len()
                );
            }
        }
    }
    Ok(())
}

#[derive(Deserialize)]
struct EmbedResponse {
    data: Vec<EmbedData>,
}

#[derive(Deserialize)]
struct EmbedData {
    #[serde(default)]
    index: usize,
    embedding: Vec<f32>,
}

#[derive(FromRow)]
struct PendingChunk {
    id: String,
    chunk_text: String,
}

#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub struct ProcessSliceResult {
    pub attempted: usize,
    pub written: usize,
    pub failed: usize,
}

pub async fn embed_batch(
    http: &HttpClient,
    litellm_url: &str,
    api_key: &str,
    model: &str,
    dims: Option<u32>,
    texts: &[String],
) -> Result<Vec<Vec<f32>>> {
    let mut body = json!({
        "model": model,
        "input": texts,
    });
    if let Some(d) = dims {
        body["dimensions"] = serde_json::json!(d);
    }

    let url = format!("{}/v1/embeddings", litellm_url.trim_end_matches('/'));
    let mut req = http.post(&url).json(&body);
    if !api_key.is_empty() {
        req = req.bearer_auth(api_key);
    }

    let resp = req.send().await.context("litellm request")?;
    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        anyhow::bail!("litellm embed HTTP {}: {}", status, body.trim());
    }

    let mut parsed: EmbedResponse = resp.json().await.context("litellm decode")?;
    parsed.data.sort_by_key(|d| d.index);
    Ok(parsed.data.into_iter().map(|d| d.embedding).collect())
}

pub async fn process_slice(
    pool: &PgPool,
    http: &HttpClient,
    cfg: &Config,
) -> Result<ProcessSliceResult> {
    let mut tx = pool.begin().await.context("begin claim tx")?;

    let rows: Vec<PendingChunk> = sqlx::query_as(
        r#"
        SELECT id, chunk_text
        FROM chunks
        WHERE embedding IS NULL
        ORDER BY id DESC
        LIMIT $1
        FOR UPDATE SKIP LOCKED
        "#,
    )
    .bind(cfg.embed_sub_batch as i64)
    .fetch_all(&mut *tx)
    .await
    .context("query pending chunks")?;

    tx.commit().await.context("commit claim tx")?;

    if rows.is_empty() {
        return Ok(ProcessSliceResult::default());
    }

    // The claim transaction commits before embedding so workers do not hold DB
    // locks across slow HTTP calls. Another worker can re-claim the same
    // embedding-null row in that window, so the UPDATE below is conditional and
    // row-counted: duplicate work is wasted but cannot double-write a vector.

    let texts: Vec<String> = rows
        .iter()
        .map(|chunk| {
            if chunk.chunk_text.len() > cfg.max_chunk_chars {
                let max_chars = cfg.max_chunk_chars;
                let end = (0..=max_chars)
                    .rev()
                    .find(|&i| chunk.chunk_text.is_char_boundary(i))
                    .unwrap_or(0);
                chunk.chunk_text[..end].to_string()
            } else {
                chunk.chunk_text.clone()
            }
        })
        .collect();

    let embeddings = match time::timeout(
        cfg.embed_timeout,
        embed_batch(
            &http,
            &cfg.litellm_url,
            &cfg.litellm_api_key,
            &cfg.embed_model,
            cfg.embed_dims,
            &texts,
        ),
    )
    .await
    {
        Err(_) => {
            warn!(chunk_count = rows.len(), "embed sub-slice timed out");
            return Ok(ProcessSliceResult {
                attempted: rows.len(),
                written: 0,
                failed: rows.len(),
            });
        }
        Ok(Err(e)) => {
            warn!(chunk_count = rows.len(), err = %e, "embed sub-slice failed");
            return Ok(ProcessSliceResult {
                attempted: rows.len(),
                written: 0,
                failed: rows.len(),
            });
        }
        Ok(Ok(v)) => v,
    };

    // Atomic pre-persist validation: reject the entire slice if cardinality or dimensions
    // are wrong. This ensures write semantics are all-or-nothing — partial progress
    // (where write count depends on response-truncation shape) is never observable.
    if let Err(e) = validate_embeddings(rows.len(), &embeddings, cfg.embed_dims) {
        warn!(
            chunk_count = rows.len(),
            err = %e,
            "embedding response failed validation; rejecting entire slice (zero writes)"
        );
        return Ok(ProcessSliceResult {
            attempted: rows.len(),
            written: 0,
            failed: rows.len(),
        });
    }

    let mut written = 0usize;
    let mut failed = 0usize;

    // At this point embeddings.len() == rows.len() is guaranteed by validate_embeddings,
    // so indexing by position is safe without bounds checks.
    for (row, vec) in rows.iter().zip(embeddings.iter()) {
        match sqlx::query(
            "UPDATE chunks SET embedding = $1::vector WHERE id = $2 AND embedding IS NULL",
        )
        .bind(Vector::from(vec.clone()))
        .bind(&row.id)
        .execute(pool)
        .await
        {
            Ok(result) => {
                if result.rows_affected() > 0 {
                    written += 1;
                }
            }
            Err(e) => {
                warn!(chunk_id = %row.id, err = %e, "failed to persist embedding");
                failed += 1;
            }
        }
    }

    Ok(ProcessSliceResult {
        attempted: rows.len(),
        written,
        failed,
    })
}

// ── validate_embeddings unit tests ────────────────────────────────────────────

#[cfg(test)]
mod validate_embeddings_tests {
    use super::validate_embeddings;

    fn vecs(count: usize, dims: usize) -> Vec<Vec<f32>> {
        vec![vec![1.0f32; dims]; count]
    }

    // ── cardinality ──────────────────────────────────────────────────────────

    #[test]
    fn valid_payload_no_dims_check_returns_ok() {
        // 3 rows, 3 embeddings, no dimension constraint.
        let embs = vecs(3, 5);
        assert!(validate_embeddings(3, &embs, None).is_ok());
    }

    #[test]
    fn valid_payload_with_dims_check_returns_ok() {
        let embs = vecs(4, 8);
        assert!(validate_embeddings(4, &embs, Some(8)).is_ok());
    }

    #[test]
    fn short_payload_is_err() {
        // 5 rows but only 3 embeddings — truncation case.
        let embs = vecs(3, 4);
        let result = validate_embeddings(5, &embs, None);
        assert!(result.is_err());
        let msg = result.unwrap_err().to_string();
        assert!(
            msg.contains("cardinality mismatch"),
            "expected 'cardinality mismatch' in error; got: {msg}"
        );
    }

    #[test]
    fn long_payload_is_err() {
        // 3 rows but 5 embeddings — protocol deviation.
        let embs = vecs(5, 4);
        let result = validate_embeddings(3, &embs, None);
        assert!(result.is_err());
        let msg = result.unwrap_err().to_string();
        assert!(
            msg.contains("cardinality mismatch"),
            "expected 'cardinality mismatch' in error; got: {msg}"
        );
    }

    #[test]
    fn zero_rows_zero_embeddings_is_ok() {
        // Edge case: empty slice — nothing to validate.
        assert!(validate_embeddings(0, &[], None).is_ok());
    }

    #[test]
    fn zero_rows_non_empty_embeddings_is_err() {
        let embs = vecs(1, 4);
        let result = validate_embeddings(0, &embs, None);
        assert!(result.is_err());
    }

    // ── dimension ────────────────────────────────────────────────────────────

    #[test]
    fn wrong_dimension_first_vec_is_err() {
        // All embeddings have 4 dims but we expected 8.
        let embs = vecs(3, 4);
        let result = validate_embeddings(3, &embs, Some(8));
        assert!(result.is_err());
        let msg = result.unwrap_err().to_string();
        assert!(
            msg.contains("dimension mismatch"),
            "expected 'dimension mismatch' in error; got: {msg}"
        );
    }

    #[test]
    fn wrong_dimension_only_last_vec_is_err() {
        // First two correct, last one wrong — partial dimension corruption.
        let mut embs = vecs(2, 8);
        embs.push(vec![1.0f32; 4]); // wrong dims on the third
        let result = validate_embeddings(3, &embs, Some(8));
        assert!(result.is_err());
        let msg = result.unwrap_err().to_string();
        assert!(msg.contains("index 2"), "expected index 2 in error; got: {msg}");
    }

    #[test]
    fn correct_dims_all_vecs_ok() {
        let embs = vecs(10, 1536);
        assert!(validate_embeddings(10, &embs, Some(1536)).is_ok());
    }

    // ── none dims skips dimension check ──────────────────────────────────────

    #[test]
    fn none_dims_skips_dimension_check_for_mixed_sizes() {
        // Without expected_dims, varying vector lengths are accepted (cardinality only).
        let embs = vec![vec![1.0f32; 4], vec![1.0f32; 8], vec![1.0f32; 2]];
        assert!(validate_embeddings(3, &embs, None).is_ok());
    }
}
