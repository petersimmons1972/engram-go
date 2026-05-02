use std::sync::Arc;
use std::time::Duration;

use anyhow::{bail, Context};
use pgvector::Vector;
use reqwest::Client as HttpClient;
use serde::Deserialize;
use sqlx::postgres::PgPoolOptions;
use sqlx::{FromRow, PgPool};
use tokio::signal;
use tokio::sync::Semaphore;
use tracing::{error, info, warn};

// ── Config ────────────────────────────────────────────────────────────────────

struct Config {
    database_url: String,
    litellm_url: String,
    litellm_api_key: String,
    embed_model: String,
    embed_dims: Option<u32>,
    /// Maximum characters to send per chunk. Prevents context-window overflow.
    /// Default 2048 chars (~512 tokens) — conservative for all supported models.
    max_chunk_chars: usize,
    batch_size: usize,
    embed_sub_batch: usize,
    interval: Duration,
    concurrency: usize,
    embed_timeout: Duration,
}

impl Config {
    fn from_env() -> Self {
        Self {
            database_url: env_require("DATABASE_URL"),
            litellm_url: env_or("LITELLM_URL", "http://litellm:4000"),
            litellm_api_key: env_or("LITELLM_API_KEY", ""),
            embed_model: env_or(
                "ENGRAM_EMBED_MODEL",
                &env_or("ENGRAM_OLLAMA_MODEL", "qwen3-embedding:8b"),
            ),
            embed_dims: env_opt_u32("ENGRAM_EMBED_DIMENSIONS"),
            max_chunk_chars: env_usize("ENGRAM_EMBED_MAX_CHARS", 2048),
            batch_size: env_usize("ENGRAM_REEMBED_BATCH_SIZE", 100),
            embed_sub_batch: env_usize("ENGRAM_EMBED_SUB_BATCH", 8),
            interval: env_duration("ENGRAM_REEMBED_INTERVAL", Duration::from_secs(10)),
            concurrency: env_usize("ENGRAM_REEMBED_CONCURRENCY", 8),
            embed_timeout: Duration::from_secs(120),
        }
    }
}

// ── LiteLLM embed client ──────────────────────────────────────────────────────

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

async fn embed_batch(
    http: &HttpClient,
    litellm_url: &str,
    api_key: &str,
    model: &str,
    dims: Option<u32>,
    texts: &[String],
) -> anyhow::Result<Vec<Vec<f32>>> {
    let mut body = serde_json::json!({
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
        bail!("litellm embed HTTP {}: {}", status, body.trim());
    }

    let mut parsed: EmbedResponse = resp.json().await.context("litellm decode")?;
    // Ollama returns results ordered by index; sort by index field if present.
    parsed.data.sort_by_key(|d| d.index);
    Ok(parsed.data.into_iter().map(|d| d.embedding).collect())
}

// ── Pending chunk ─────────────────────────────────────────────────────────────

#[derive(FromRow, Clone)]
struct PendingChunk {
    id: String,
    chunk_text: String,
}

// ── Batch processing ──────────────────────────────────────────────────────────

async fn run_batch(pool: &PgPool, http: &HttpClient, cfg: &Config) -> anyhow::Result<usize> {
    // Claim a batch inside a transaction and commit immediately so the
    // connection is returned to the pool before the (slow) embed calls begin.
    // FOR UPDATE SKIP LOCKED ensures concurrent replicas don't process the same chunks.
    let mut tx = pool.begin().await.context("begin tx")?;
    let rows: Vec<PendingChunk> = sqlx::query_as(
        r#"
        SELECT id, chunk_text
        FROM chunks
        WHERE embedding IS NULL
        ORDER BY id
        LIMIT $1
        FOR UPDATE SKIP LOCKED
        "#,
    )
    .bind(cfg.batch_size as i64)
    .fetch_all(&mut *tx)
    .await
    .context("query pending chunks")?;
    tx.commit().await.context("commit claim tx")?;

    if rows.is_empty() {
        return Ok(0);
    }

    let n = rows.len();
    tracing::debug!(count = n, "processing batch");

    let sem = Arc::new(Semaphore::new(cfg.concurrency));
    let mut handles = Vec::new();

    for sub_batch in rows.chunks(cfg.embed_sub_batch) {
        let permit = sem.clone().acquire_owned().await?;
        let pool = pool.clone();
        let http = http.clone();
        let litellm_url = cfg.litellm_url.clone();
        let api_key = cfg.litellm_api_key.clone();
        let model = cfg.embed_model.clone();
        let dims = cfg.embed_dims;
        let timeout = cfg.embed_timeout;
        let max_chars = cfg.max_chunk_chars;
        let chunks: Vec<PendingChunk> = sub_batch.to_vec();

        handles.push(tokio::spawn(async move {
            let _permit = permit;

            // Truncate each text to the model context window.
            let texts: Vec<String> = chunks.iter().map(|c| {
                if c.chunk_text.len() > max_chars {
                    let end = (0..=max_chars).rev()
                        .find(|&i| c.chunk_text.is_char_boundary(i))
                        .unwrap_or(0);
                    c.chunk_text[..end].to_string()
                } else {
                    c.chunk_text.clone()
                }
            }).collect();

            let embed_result = tokio::time::timeout(
                timeout,
                embed_batch(&http, &litellm_url, &api_key, &model, dims, &texts),
            )
            .await;

            let vecs = match embed_result {
                Err(_) => {
                    warn!(count = chunks.len(), "embed sub-batch timeout");
                    return;
                }
                Ok(Err(e)) => {
                    warn!(count = chunks.len(), err = %e, "embed sub-batch failed");
                    return;
                }
                Ok(Ok(v)) => v,
            };

            for (chunk, vec) in chunks.iter().zip(vecs.into_iter()) {
                if let Err(e) = sqlx::query(
                    "UPDATE chunks SET embedding = $1::vector WHERE id = $2",
                )
                .bind(Vector::from(vec))
                .bind(&chunk.id)
                .execute(&pool)
                .await
                {
                    warn!(chunk_id = %chunk.id, err = %e, "update failed");
                }
            }
        }));
    }

    for h in handles {
        let _ = h.await;
    }

    Ok(n)
}

// ── Main loop ─────────────────────────────────────────────────────────────────

async fn run(cfg: Config) -> anyhow::Result<()> {
    let pool = PgPoolOptions::new()
        .max_connections(cfg.concurrency as u32 + 4)
        .connect(&cfg.database_url)
        .await
        .context("connect to postgres")?;

    let http = HttpClient::builder()
        .timeout(cfg.embed_timeout + Duration::from_secs(5))
        .build()
        .context("build http client")?;

    // Startup probe — log dims so we know the model is reachable.
    match embed_batch(
        &http,
        &cfg.litellm_url,
        &cfg.litellm_api_key,
        &cfg.embed_model,
        cfg.embed_dims,
        &[String::from("probe")],
    )
    .await
    {
        Ok(v) => info!(dims = v.first().map(|e| e.len()).unwrap_or(0), model = %cfg.embed_model, "litellm probe ok"),
        Err(e) => warn!(err = %e, "litellm probe failed — will retry on each batch"),
    }

    info!(
        batch_size = cfg.batch_size,
        interval_ms = cfg.interval.as_millis(),
        concurrency = cfg.concurrency,
        "engram-reembed started"
    );

    let mut backoff = cfg.interval;
    let max_backoff = Duration::from_secs(300);

    loop {
        match run_batch(&pool, &http, &cfg).await {
            Err(e) => {
                error!(err = %e, "batch error");
                backoff = (backoff * 2).min(max_backoff);
            }
            Ok(0) => {
                backoff = (backoff * 2).min(max_backoff);
            }
            Ok(n) if n >= cfg.batch_size => {
                // Full batch — drain immediately without sleeping.
                backoff = cfg.interval;
                continue;
            }
            Ok(_) => {
                // Partial batch — queue exhausted, reset backoff.
                backoff = cfg.interval;
            }
        }

        tokio::select! {
            _ = tokio::time::sleep(backoff) => {}
            _ = signal::ctrl_c() => {
                info!("shutdown signal received");
                break;
            }
        }
    }

    pool.close().await;
    Ok(())
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .json()
        .init();

    let cfg = Config::from_env();

    if let Err(e) = run(cfg).await {
        error!(err = %e, "fatal error");
        std::process::exit(1);
    }
}

// ── Env helpers ───────────────────────────────────────────────────────────────

fn env_require(key: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| panic!("{key} environment variable is required"))
}

fn env_or(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_string())
}

fn env_opt_u32(key: &str) -> Option<u32> {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .filter(|&d: &u32| d > 0)
}

fn env_usize(key: &str, default: usize) -> usize {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

fn env_duration(key: &str, default: Duration) -> Duration {
    std::env::var(key)
        .ok()
        .and_then(|v| {
            if let Ok(secs) = v.parse::<u64>() {
                return Some(Duration::from_secs(secs));
            }
            if let Some(s) = v.strip_suffix('s') {
                return s.parse().ok().map(Duration::from_secs);
            }
            if let Some(m) = v.strip_suffix('m') {
                return m.parse::<u64>().ok().map(|n| Duration::from_secs(n * 60));
            }
            None
        })
        .unwrap_or(default)
}
