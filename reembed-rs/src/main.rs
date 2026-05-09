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
    embed_timeout: Duration,
    // Adaptive concurrency config
    concurrency_min: usize,
    concurrency_max: usize,
    latency_high_ms: u64,
    latency_low_ms: u64,
    ramp_after: usize,
}

impl Config {
    fn from_env() -> Self {
        // ENGRAM_REEMBED_CONCURRENCY is the legacy fixed-concurrency knob.
        // It now sets concurrency_max when ENGRAM_REEMBED_CONCURRENCY_MAX is absent.
        let legacy_concurrency = env_usize("ENGRAM_REEMBED_CONCURRENCY", 16);
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
            embed_timeout: Duration::from_secs(120),
            concurrency_min: env_usize("ENGRAM_REEMBED_CONCURRENCY_MIN", 1),
            concurrency_max: env_usize("ENGRAM_REEMBED_CONCURRENCY_MAX", legacy_concurrency),
            latency_high_ms: env_u64("ENGRAM_REEMBED_LATENCY_HIGH_MS", 2000),
            latency_low_ms: env_u64("ENGRAM_REEMBED_LATENCY_LOW_MS", 400),
            ramp_after: env_usize("ENGRAM_REEMBED_RAMP_AFTER", 3),
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

async fn run_batch(
    pool: &PgPool,
    http: &HttpClient,
    cfg: &Config,
    concurrency: usize,
) -> anyhow::Result<(usize, Duration)> {
    let start = std::time::Instant::now();

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
        return Ok((0, start.elapsed()));
    }

    let n = rows.len();
    let t_select = start.elapsed();
    tracing::debug!(count = n, concurrency, "processing batch");

    let sem = Arc::new(Semaphore::new(concurrency));
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
            let t0 = std::time::Instant::now();

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

            let t_embed = t0.elapsed();

            let vecs = match embed_result {
                Err(_) => {
                    warn!(count = chunks.len(), embed_ms = t_embed.as_millis(), "embed sub-batch timeout");
                    return;
                }
                Ok(Err(e)) => {
                    warn!(count = chunks.len(), embed_ms = t_embed.as_millis(), err = %e, "embed sub-batch failed");
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

            let t_write = t0.elapsed() - t_embed;
            tracing::debug!(
                count = chunks.len(),
                embed_ms = t_embed.as_millis(),
                write_ms = t_write.as_millis(),
                "sub-batch done"
            );
        }));
    }

    for h in handles {
        let _ = h.await;
    }

    let total = start.elapsed();
    tracing::info!(
        chunks = n,
        select_ms = t_select.as_millis(),
        total_ms = total.as_millis(),
        "batch complete"
    );

    Ok((n, total))
}

// ── Main loop ─────────────────────────────────────────────────────────────────

async fn run(cfg: Config) -> anyhow::Result<()> {
    let pool = PgPoolOptions::new()
        .max_connections(cfg.concurrency_max as u32 + 4)
        .connect(&cfg.database_url)
        .await
        .context("connect to postgres")?;

    let http = HttpClient::builder()
        .timeout(cfg.embed_timeout + Duration::from_secs(5))
        // Keep enough idle connections for all concurrent sub-batch tasks so
        // every batch cycle reuses warm connections. Without this, reqwest may
        // close excess connections between cycles, causing 19/20 tasks to pay
        // TCP setup cost (~1ms each) and stagger their arrival at vLLM —
        // preventing vLLM from batching all requests in a single forward pass.
        .pool_max_idle_per_host(cfg.concurrency_max)
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

    // Pre-warm connection pool — establish concurrency_max keep-alive connections
    // so all sub-batch tasks start with ready sockets (no TCP setup during the
    // batch). With TOKIO_WORKER_THREADS=concurrency_max, all async tasks also
    // get OS-thread-level parallelism, ensuring socket writes overlap in time.
    {
        let mut warmup = tokio::task::JoinSet::new();
        for _ in 0..cfg.concurrency_max {
            let h = http.clone();
            let url = cfg.litellm_url.clone();
            let key = cfg.litellm_api_key.clone();
            let model = cfg.embed_model.clone();
            let dims = cfg.embed_dims;
            warmup.spawn(async move {
                let _ = embed_batch(&h, &url, &key, &model, dims, &[String::from("warmup")]).await;
            });
        }
        while warmup.join_next().await.is_some() {}
        info!(connections = cfg.concurrency_max, "connection pool warmed");
    }

    info!(
        batch_size = cfg.batch_size,
        interval_ms = cfg.interval.as_millis(),
        concurrency_min = cfg.concurrency_min,
        concurrency_max = cfg.concurrency_max,
        "engram-reembed started"
    );

    let mut controller = AdaptiveConcurrency::new(
        cfg.concurrency_min,
        cfg.concurrency_max,
        cfg.latency_high_ms,
        cfg.latency_low_ms,
        cfg.ramp_after,
    );

    let mut backoff = cfg.interval;
    let max_backoff = Duration::from_secs(300);

    loop {
        let prev_concurrency = controller.current;
        match run_batch(&pool, &http, &cfg, controller.current).await {
            Err(e) => {
                error!(err = %e, "batch error");
                // Treat batch error as high-latency to back off the semaphore.
                controller.update(1, Duration::from_millis(cfg.latency_high_ms * 2));
                backoff = (backoff * 2).min(max_backoff);
            }
            Ok((0, _)) => {
                backoff = (backoff * 2).min(max_backoff);
            }
            Ok((n, elapsed)) => {
                controller.update(n, elapsed);
                if controller.current != prev_concurrency {
                    let per_chunk_ms = elapsed.as_millis() as u64 / n as u64;
                    let reason = if controller.current < prev_concurrency {
                        "latency_high"
                    } else {
                        "latency_low"
                    };
                    info!(
                        prev = prev_concurrency,
                        next = controller.current,
                        reason,
                        per_chunk_latency_ms = per_chunk_ms,
                        "reembed concurrency adjusted"
                    );
                }
                if n >= cfg.batch_size {
                    // Full batch — drain immediately without sleeping.
                    backoff = cfg.interval;
                    continue;
                }
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

fn env_u64(key: &str, default: u64) -> u64 {
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
            if let Some(ms) = v.strip_suffix("ms") {
                return ms.parse::<u64>().ok().map(Duration::from_millis);
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

// ── Adaptive concurrency ──────────────────────────────────────────────────────

pub struct AdaptiveConcurrency {
    pub current: usize,
    min: usize,
    max: usize,
    latency_high_ms: u64,
    latency_low_ms: u64,
    ramp_after: usize,
    clean_count: usize,
}

impl AdaptiveConcurrency {
    pub fn new(
        min: usize,
        max: usize,
        latency_high_ms: u64,
        latency_low_ms: u64,
        ramp_after: usize,
    ) -> Self {
        Self {
            current: max,
            min,
            max,
            latency_high_ms,
            latency_low_ms,
            ramp_after,
            clean_count: 0,
        }
    }

    pub fn update(&mut self, chunks: usize, elapsed: Duration) {
        if chunks == 0 {
            return;
        }
        let per_chunk_ms = (elapsed.as_millis() / chunks as u128) as u64;
        if per_chunk_ms >= self.latency_high_ms {
            self.current = (self.current / 2).max(self.min);
            self.clean_count = 0;
        } else if per_chunk_ms < self.latency_low_ms {
            self.clean_count += 1;
            if self.clean_count >= self.ramp_after {
                self.current = (self.current + 1).min(self.max);
                self.clean_count = 0;
            }
        }
    }
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod config_tests {
    use super::*;

    #[test]
    fn config_adaptive_defaults() {
        // Unset all adaptive vars so we see defaults.
        std::env::remove_var("ENGRAM_REEMBED_CONCURRENCY_MIN");
        std::env::remove_var("ENGRAM_REEMBED_CONCURRENCY_MAX");
        std::env::remove_var("ENGRAM_REEMBED_LATENCY_HIGH_MS");
        std::env::remove_var("ENGRAM_REEMBED_LATENCY_LOW_MS");
        std::env::remove_var("ENGRAM_REEMBED_RAMP_AFTER");
        std::env::remove_var("ENGRAM_REEMBED_CONCURRENCY");
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert_eq!(cfg.concurrency_min, 1);
        assert_eq!(cfg.concurrency_max, 16);
        assert_eq!(cfg.latency_high_ms, 2000);
        assert_eq!(cfg.latency_low_ms, 400);
        assert_eq!(cfg.ramp_after, 3);
    }

    #[test]
    fn config_concurrency_backward_compat() {
        // Legacy ENGRAM_REEMBED_CONCURRENCY sets concurrency_max when MAX absent.
        std::env::remove_var("ENGRAM_REEMBED_CONCURRENCY_MAX");
        std::env::set_var("ENGRAM_REEMBED_CONCURRENCY", "4");
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert_eq!(cfg.concurrency_max, 4);
        std::env::remove_var("ENGRAM_REEMBED_CONCURRENCY");
    }

    #[test]
    fn config_explicit_max_overrides_legacy() {
        std::env::set_var("ENGRAM_REEMBED_CONCURRENCY", "4");
        std::env::set_var("ENGRAM_REEMBED_CONCURRENCY_MAX", "12");
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert_eq!(cfg.concurrency_max, 12);
        std::env::remove_var("ENGRAM_REEMBED_CONCURRENCY");
        std::env::remove_var("ENGRAM_REEMBED_CONCURRENCY_MAX");
    }
}

#[cfg(test)]
mod adaptive_concurrency_tests {
    use super::*;

    fn controller(min: usize, max: usize) -> AdaptiveConcurrency {
        AdaptiveConcurrency::new(min, max, 2000, 400, 3)
    }

    #[test]
    fn initial_current_equals_max() {
        let c = controller(1, 8);
        assert_eq!(c.current, 8);
    }

    #[test]
    fn high_latency_halves_concurrency() {
        let mut c = controller(1, 8);
        c.update(10, Duration::from_millis(30_000)); // 3000ms/chunk > 2000 threshold
        assert_eq!(c.current, 4);
    }

    #[test]
    fn high_latency_floors_at_min() {
        let mut c = controller(3, 4);
        c.update(10, Duration::from_millis(30_000)); // halve 4 → 2, floor at 3
        assert_eq!(c.current, 3);
    }

    #[test]
    fn floor_is_respected_on_md() {
        let mut c = controller(2, 2);
        c.update(10, Duration::from_millis(30_000)); // halve 2 → 1, floor at 2
        assert_eq!(c.current, 2);
    }

    #[test]
    fn low_latency_increments_clean_counter() {
        let mut c = controller(1, 8);
        c.update(10, Duration::from_millis(1_000)); // 100ms/chunk < 400 threshold
        // not enough batches yet — current unchanged
        assert_eq!(c.current, 8);
    }

    #[test]
    fn low_latency_does_not_ramp_before_threshold() {
        let mut c = controller(1, 8);
        // trigger MD first so current < max and we can observe a ramp
        c.update(10, Duration::from_millis(30_000)); // → 4
        c.update(10, Duration::from_millis(1_000));  // clean 1
        c.update(10, Duration::from_millis(1_000));  // clean 2 (ramp_after=3, not yet)
        assert_eq!(c.current, 4);
    }

    #[test]
    fn low_latency_ramps_after_threshold_and_resets_counter() {
        let mut c = controller(1, 8);
        c.update(10, Duration::from_millis(30_000)); // MD: 8 → 4
        c.update(10, Duration::from_millis(1_000));  // clean 1
        c.update(10, Duration::from_millis(1_000));  // clean 2
        c.update(10, Duration::from_millis(1_000));  // clean 3 → ramp to 5
        assert_eq!(c.current, 5);
        // counter reset: one more clean should NOT ramp again
        c.update(10, Duration::from_millis(1_000));  // clean 1 (reset)
        assert_eq!(c.current, 5);
    }

    #[test]
    fn dead_band_latency_leaves_state_unchanged() {
        let mut c = controller(1, 8);
        c.update(10, Duration::from_millis(30_000)); // MD: 8 → 4
        c.update(10, Duration::from_millis(8_000));  // 800ms/chunk — dead band [400, 2000)
        assert_eq!(c.current, 4);
    }

    #[test]
    fn empty_batch_skips_update() {
        let mut c = controller(1, 8);
        c.update(10, Duration::from_millis(30_000)); // MD: 8 → 4
        c.update(0, Duration::from_millis(1_000));   // empty — no change
        assert_eq!(c.current, 4);
    }

    #[test]
    fn ceiling_is_respected_on_ramp() {
        let mut c = controller(1, 4);
        // starts at 4 (max); trigger MD then ramp back to ceiling
        c.update(10, Duration::from_millis(30_000)); // → 2
        c.update(10, Duration::from_millis(1_000));
        c.update(10, Duration::from_millis(1_000));
        c.update(10, Duration::from_millis(1_000));  // ramp: 2 → 3
        c.update(10, Duration::from_millis(1_000));
        c.update(10, Duration::from_millis(1_000));
        c.update(10, Duration::from_millis(1_000));  // ramp: 3 → 4
        c.update(10, Duration::from_millis(1_000));
        c.update(10, Duration::from_millis(1_000));
        c.update(10, Duration::from_millis(1_000));  // ramp: would be 5, clamped to 4
        assert_eq!(c.current, 4);
    }
}
