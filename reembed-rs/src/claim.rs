use anyhow::{Context, Result};
use pgvector::Vector;
use reqwest::Client as HttpClient;
use serde::Deserialize;
use serde_json::json;
use sqlx::{FromRow, PgPool};
use std::time::Duration;
use tokio::time;
use tracing::warn;

use crate::Config;

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
) -> Result<(usize, usize)> {
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
        return Ok((0, 0));
    }

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
        embed_batch(&http, &cfg.litellm_url, &cfg.litellm_api_key, &cfg.embed_model, cfg.embed_dims, &texts),
    )
    .await
    {
        Err(_) => {
            warn!(chunk_count = rows.len(), "embed sub-slice timed out");
            return Ok((rows.len(), rows.len()));
        }
        Ok(Err(e)) => {
            warn!(chunk_count = rows.len(), err = %e, "embed sub-slice failed");
            return Ok((rows.len(), rows.len()));
        }
        Ok(Ok(v)) => v,
    };

    let mut failed = 0usize;

    let mut i = 0;
    while i < rows.len() {
        let row = &rows[i];
        let vec = match embeddings.get(i) {
            Some(v) => v,
            None => {
                warn!(chunk_id = %row.id, "embedding payload shorter than claimed chunk set");
                failed += 1;
                i += 1;
                continue;
            }
        };

        if let Err(e) = sqlx::query("UPDATE chunks SET embedding = $1::vector WHERE id = $2")
            .bind(Vector::from(vec.clone()))
            .bind(&row.id)
            .execute(pool)
            .await
        {
            warn!(chunk_id = %row.id, err = %e, "failed to persist embedding");
            failed += 1;
        }
        i += 1;
    }

    if embeddings.len() > rows.len() {
        warn!(
            claimed = rows.len(),
            embeddings = embeddings.len(),
            "received more embeddings than claimed rows"
        );
    }

    Ok((rows.len(), failed))
}
