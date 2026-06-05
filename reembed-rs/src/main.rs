use std::sync::Arc;
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};
use std::time::Duration;

use anyhow::{bail, Context};
use reqwest::Client as HttpClient;
use sqlx::postgres::PgPoolOptions;
use sqlx::PgPool;
use tokio::signal;
use tokio::task::JoinSet;
use tokio::time::{sleep, Instant};
use tokio_util::sync::CancellationToken;
use tracing::{error, info, warn};

pub mod claim;

// ── Config ────────────────────────────────────────────────────────────────────

#[derive(Clone)]
pub struct Config {
    pub database_url: String,
    pub litellm_url: String,
    pub litellm_api_key: String,
    pub embed_model: String,
    pub embed_dims: Option<u32>,
    /// Maximum characters to send per chunk. Prevents context-window overflow.
    /// Default 2048 chars (~512 tokens) — conservative for all supported models.
    pub max_chunk_chars: usize,
    /// Legacy alias retained: REEMBED_BATCH_SIZE now maps directly to the per-claim
    /// slice size and therefore replaces the old batched fan-out chunk count.
    pub batch_size: usize,
    /// Per-claim slice size. In the worker-pool model this is the same as
    /// batch_size and maps to the max number of rows claimed per DB transaction.
    pub embed_sub_batch: usize,
    pub interval: Duration,
    pub embed_timeout: Duration,
    pub startup_probe_max_attempts: usize,
    pub startup_probe_initial_backoff: Duration,
    pub startup_probe_max_backoff: Duration,
    // Fixed worker-count model keeps concurrency in this range.
    pub concurrency_min: usize,
    pub concurrency_max: usize,
    pub latency_high_ms: u64,
    pub latency_low_ms: u64,
    pub ramp_after: usize,
    /// Failure-rate threshold above which backpressure fires (halve + reset streak).
    /// Below this rate the controller holds position without ramp credit.
    /// Configurable via ENGRAM_REEMBED_FAILURE_RATE_BACKPRESSURE (default 0.10).
    pub failure_rate_backpressure: f64,
}

impl Config {
    fn from_env() -> Self {
        let legacy_concurrency = env_usize("ENGRAM_REEMBED_CONCURRENCY", 16);
        let embed_sub_batch = env_usize("ENGRAM_EMBED_SUB_BATCH", 8);
        let batch_size = env_usize("ENGRAM_REEMBED_BATCH_SIZE", embed_sub_batch);

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
            batch_size,
            embed_sub_batch: batch_size,
            interval: env_duration("ENGRAM_REEMBED_INTERVAL", Duration::from_secs(10)),
            embed_timeout: Duration::from_secs(120),
            startup_probe_max_attempts: env_usize("ENGRAM_REEMBED_STARTUP_PROBE_MAX_ATTEMPTS", 5),
            startup_probe_initial_backoff: env_duration(
                "ENGRAM_REEMBED_STARTUP_PROBE_INITIAL_BACKOFF",
                Duration::from_secs(2),
            ),
            startup_probe_max_backoff: env_duration(
                "ENGRAM_REEMBED_STARTUP_PROBE_MAX_BACKOFF",
                Duration::from_secs(60),
            ),
            concurrency_min: env_usize("ENGRAM_REEMBED_CONCURRENCY_MIN", 1),
            concurrency_max: env_usize("ENGRAM_REEMBED_CONCURRENCY_MAX", legacy_concurrency),
            latency_high_ms: env_u64("ENGRAM_REEMBED_LATENCY_HIGH_MS", 2000),
            latency_low_ms: env_u64("ENGRAM_REEMBED_LATENCY_LOW_MS", 400),
            ramp_after: env_usize("ENGRAM_REEMBED_RAMP_AFTER", 3),
            failure_rate_backpressure: env_f64("ENGRAM_REEMBED_FAILURE_RATE_BACKPRESSURE", 0.10),
        }
    }
}

async fn startup_probe(http: &HttpClient, cfg: &Config) -> anyhow::Result<()> {
    let attempts = cfg.startup_probe_max_attempts.max(1);
    let mut backoff = cfg.startup_probe_initial_backoff;
    let mut last_err: Option<anyhow::Error> = None;

    for attempt in 1..=attempts {
        match claim::embed_batch(
            http,
            &cfg.litellm_url,
            &cfg.litellm_api_key,
            &cfg.embed_model,
            cfg.embed_dims,
            &[String::from("probe")],
        )
        .await
        {
            Ok(v) => {
                info!(
                    dims = v.first().map(|e| e.len()).unwrap_or(0),
                    model = %cfg.embed_model,
                    attempt,
                    "litellm probe ok"
                );
                return Ok(());
            }
            Err(e) => {
                last_err = Some(e);
                if attempt == attempts {
                    break;
                }
                warn!(
                    attempt,
                    max_attempts = attempts,
                    sleep_ms = backoff.as_millis(),
                    url = %cfg.litellm_url,
                    "startup embed probe failed; retrying with backoff"
                );
                sleep(backoff).await;
                backoff = (backoff * 2).min(cfg.startup_probe_max_backoff);
            }
        }
    }

    if let Some(e) = last_err {
        bail!("embed endpoint unreachable after startup probe retries: {}", e);
    }
    bail!("embed endpoint unreachable after startup probe retries")
}

async fn warmup_embeddings(http: &HttpClient, cfg: &Config) -> usize {
    use std::sync::atomic::{AtomicUsize, Ordering};
    let successes = Arc::new(AtomicUsize::new(0));
    let mut handles = Vec::with_capacity(cfg.concurrency_max);

    for _ in 0..cfg.concurrency_max {
        let h = http.clone();
        let url = cfg.litellm_url.clone();
        let key = cfg.litellm_api_key.clone();
        let model = cfg.embed_model.clone();
        let dims = cfg.embed_dims;
        let successes = Arc::clone(&successes);

        handles.push(tokio::spawn(async move {
            if claim::embed_batch(&h, &url, &key, &model, dims, &[String::from("warmup")])
                .await
                .is_ok()
            {
                successes.fetch_add(1, Ordering::Relaxed);
            }
        }));
    }

    for handle in handles {
        let _ = handle.await;
    }

    successes.load(Ordering::Relaxed).max(1)
}

fn millis_as_u64(duration: Duration) -> u64 {
    duration
        .as_millis()
        .try_into()
        .unwrap_or(u64::MAX)
}

fn increase_backoff(current_ms: u64) -> u64 {
    (current_ms * 2).min(millis_as_u64(Duration::from_secs(300)))
}

async fn run_worker(
    worker_id: usize,
    pool: PgPool,
    http: HttpClient,
    cfg: Config,
    shutdown: CancellationToken,
    in_flight: Arc<AtomicUsize>,
    completed: Arc<AtomicUsize>,
    failed: Arc<AtomicUsize>,
    backoff_ms: Arc<AtomicU64>,
) {
    let max_backoff_ms = millis_as_u64(Duration::from_secs(300));
    let mut worker_completed = 0usize;
    let mut worker_failed = 0usize;

    loop {
        if shutdown.is_cancelled() {
            break;
        }

        in_flight.fetch_add(1, Ordering::AcqRel);
        let started = Instant::now();
        let result = claim::process_slice(&pool, &http, &cfg).await;
        in_flight.fetch_sub(1, Ordering::AcqRel);

        match result {
            Ok((claimed, failed_count)) => {
                let elapsed = started.elapsed();
                if claimed == 0 {
                    let wait_ms = backoff_ms.load(Ordering::Acquire);
                    let next_ms = increase_backoff(wait_ms);
                    let _ = backoff_ms.compare_exchange(
                        wait_ms,
                        next_ms.min(max_backoff_ms),
                        Ordering::AcqRel,
                        Ordering::Acquire,
                    );

                    warn!(
                        worker_id,
                        wait_ms,
                        "no claim-eligible chunks; backing off"
                    );
                    tokio::select! {
                        _ = tokio::time::sleep(Duration::from_millis(wait_ms)) => {}
                        _ = shutdown.cancelled() => {}
                    }
                    if shutdown.is_cancelled() {
                        break;
                    }
                    continue;
                }

                backoff_ms.store(millis_as_u64(cfg.interval), Ordering::Release);
                worker_completed += claimed;
                worker_failed += failed_count;
                completed.fetch_add(claimed, Ordering::AcqRel);
                failed.fetch_add(failed_count, Ordering::AcqRel);

                let secs = elapsed.as_secs_f64().max(0.001);
                let throughput = claimed as f64 / secs;
                info!(
                    worker_id,
                    claimed,
                    failed_slices = failed_count,
                    slice_ms = elapsed.as_millis(),
                    throughput_rows_per_sec = throughput,
                    worker_total_claimed = worker_completed,
                    worker_total_failed = worker_failed,
                    "worker throughput"
                );

                continue;
            }
            Err(err) => {
                warn!(worker_id, err = %err, "claim slice failed");
                let wait_ms = backoff_ms.load(Ordering::Acquire).max(millis_as_u64(cfg.interval));
                let next_ms = increase_backoff(wait_ms).max(wait_ms);
                let _ = backoff_ms.compare_exchange(
                    wait_ms,
                    next_ms,
                    Ordering::AcqRel,
                    Ordering::Acquire,
                );
                tokio::select! {
                    _ = tokio::time::sleep(Duration::from_millis(wait_ms.min(max_backoff_ms))) => {}
                    _ = shutdown.cancelled() => {}
                }
                if shutdown.is_cancelled() {
                    break;
                }
            }
        }
    }

    info!(
        worker_id,
        claimed = worker_completed,
        failed = worker_failed,
        "worker exiting"
    );
}

pub async fn run_with_shutdown(cfg: Config, shutdown: CancellationToken) -> anyhow::Result<()> {
    let requested_workers = cfg.concurrency_max.max(1);

    let pool = PgPoolOptions::new()
        .max_connections(requested_workers as u32 + 4)
        .connect(&cfg.database_url)
        .await
        .context("connect to postgres")?;

    let pool_for_embed = HttpClient::builder()
        .timeout(cfg.embed_timeout + Duration::from_secs(5))
        // Keep enough idle connections for all concurrent workers so warmup and
        // normal slices can reuse sockets without churn.
        .pool_max_idle_per_host(requested_workers)
        .build()
        .context("build http client")?;

    startup_probe(&pool_for_embed, &cfg).await?;

    let effective_workers = {
        let n = warmup_embeddings(&pool_for_embed, &cfg).await;
        let n = n.max(1);
        if n < requested_workers {
            warn!(
                requested = requested_workers,
                effective = n,
                "embed endpoint handled fewer concurrent requests than configured — capping worker count"
            );
        }
        info!(connections = n, "connection pool warmed");
        n.min(requested_workers)
    };

    info!(
        worker_count = effective_workers,
        interval_ms = cfg.interval.as_millis(),
        concurrency_min = cfg.concurrency_min,
        concurrency_max = cfg.concurrency_max,
        startup_probe_max_attempts = cfg.startup_probe_max_attempts,
        "engram-reembed started"
    );

    let in_flight = Arc::new(AtomicUsize::new(0));
    let completed = Arc::new(AtomicUsize::new(0));
    let failed = Arc::new(AtomicUsize::new(0));
    let backoff_ms = Arc::new(AtomicU64::new(millis_as_u64(cfg.interval)));

    let gauge = {
        let in_flight = in_flight.clone();
        let completed = completed.clone();
        let failed = failed.clone();
        let backoff_ms = backoff_ms.clone();
        let shutdown = shutdown.clone();
        tokio::spawn(async move {
            let mut tick = tokio::time::interval(Duration::from_secs(5));
            loop {
                tokio::select! {
                    _ = tick.tick() => {
                        info!(
                            in_flight = in_flight.load(Ordering::Acquire),
                            completed_total = completed.load(Ordering::Acquire),
                            failed_total = failed.load(Ordering::Acquire),
                            backoff_ms = backoff_ms.load(Ordering::Acquire),
                            "reembed progress gauge"
                        );
                    }
                    _ = shutdown.cancelled() => break,
                }
            }
        })
    };

    let mut workers = JoinSet::new();
    for worker_id in 0..effective_workers {
        workers.spawn(run_worker(
            worker_id,
            pool.clone(),
            pool_for_embed.clone(),
            cfg.clone(),
            shutdown.clone(),
            in_flight.clone(),
            completed.clone(),
            failed.clone(),
            backoff_ms.clone(),
        ));
    }

    while let Some(result) = workers.join_next().await {
        if let Err(err) = result {
            warn!(worker_error = %err, "worker task ended with panic");
        }
    }

    gauge.abort();
    let _ = gauge.await;

    pool.close().await;
    Ok(())
}

async fn run(cfg: Config) -> anyhow::Result<()> {
    let shutdown = CancellationToken::new();
    let runner = tokio::spawn(run_with_shutdown(cfg, shutdown.clone()));

    tokio::select! {
        _ = signal::ctrl_c() => {
            info!("shutdown signal received");
            shutdown.cancel();
            runner
                .await
                .context("reembed worker runner join failed")?
        }
        result = runner => {
            result.context("reembed worker runner join failed")?
        }
    }
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

fn env_f64(key: &str, default: f64) -> f64 {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
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
    failure_rate_backpressure: f64,
}

impl AdaptiveConcurrency {
    pub fn new(
        min: usize,
        max: usize,
        latency_high_ms: u64,
        latency_low_ms: u64,
        ramp_after: usize,
        failure_rate_backpressure: f64,
    ) -> Self {
        Self {
            current: min, // slow-start: earn concurrency, don't assume it
            min,
            max,
            latency_high_ms,
            latency_low_ms,
            ramp_after,
            clean_count: 0,
            failure_rate_backpressure,
        }
    }

    /// Update the concurrency level based on observed batch results.
    ///
    /// Three branches based on `failed_sub_batches / total_sub_batches`:
    ///
    /// - ≥ `failure_rate_backpressure` (default 10%): halve + reset streak.
    /// - > 0% but < threshold: hold — no halve, no ramp credit, streak preserved.
    /// - 0%: latency-based ramp logic (unchanged).
    pub fn update(
        &mut self,
        chunks: usize,
        failed_sub_batches: usize,
        total_sub_batches: usize,
        elapsed: Duration,
    ) {
        if chunks == 0 {
            return;
        }

        let failure_rate = if total_sub_batches > 0 {
            failed_sub_batches as f64 / total_sub_batches as f64
        } else {
            0.0
        };

        if failure_rate >= self.failure_rate_backpressure {
            // Significant failure rate → backpressure: halve immediately, reset streak.
            self.current = (self.current / 2).max(self.min);
            self.clean_count = 0;
            return;
        }

        if failure_rate > 0.0 {
            // Low-rate failure → hold position: no halve, no ramp credit, streak preserved.
            return;
        }

        // Clean batch (0% failure) → latency-based ramp/backoff.
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

    fn reset_env() {
        for key in [
            "ENGRAM_REEMBED_CONCURRENCY_MIN",
            "ENGRAM_REEMBED_CONCURRENCY_MAX",
            "ENGRAM_REEMBED_LATENCY_HIGH_MS",
            "ENGRAM_REEMBED_LATENCY_LOW_MS",
            "ENGRAM_REEMBED_RAMP_AFTER",
            "ENGRAM_REEMBED_CONCURRENCY",
            "ENGRAM_REEMBED_FAILURE_RATE_BACKPRESSURE",
            "ENGRAM_REEMBED_STARTUP_PROBE_MAX_ATTEMPTS",
            "ENGRAM_REEMBED_STARTUP_PROBE_INITIAL_BACKOFF",
            "ENGRAM_REEMBED_STARTUP_PROBE_MAX_BACKOFF",
            "ENGRAM_EMBED_SUB_BATCH",
            "ENGRAM_REEMBED_BATCH_SIZE",
        ] {
            std::env::remove_var(key);
        }
    }

    #[test]
    fn config_adaptive_defaults() {
        reset_env();
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert_eq!(cfg.concurrency_min, 1);
        assert_eq!(cfg.concurrency_max, 16);
        assert_eq!(cfg.latency_high_ms, 2000);
        assert_eq!(cfg.latency_low_ms, 400);
        assert_eq!(cfg.ramp_after, 3);
        assert_eq!(cfg.embed_sub_batch, 8);
    }

    #[test]
    fn config_concurrency_backward_compat() {
        // Legacy ENGRAM_REEMBED_CONCURRENCY sets concurrency_max when MAX absent.
        reset_env();
        std::env::set_var("ENGRAM_REEMBED_CONCURRENCY", "4");
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert_eq!(cfg.concurrency_max, 4);
    }

    #[test]
    fn config_explicit_max_overrides_legacy() {
        reset_env();
        std::env::set_var("ENGRAM_REEMBED_CONCURRENCY", "4");
        std::env::set_var("ENGRAM_REEMBED_CONCURRENCY_MAX", "12");
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert_eq!(cfg.concurrency_max, 12);
    }

    #[test]
    fn config_failure_rate_backpressure_default() {
        reset_env();
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert!((cfg.failure_rate_backpressure - 0.10).abs() < f64::EPSILON);
    }

    #[test]
    fn config_failure_rate_backpressure_override() {
        reset_env();
        std::env::set_var("ENGRAM_REEMBED_FAILURE_RATE_BACKPRESSURE", "0.05");
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert!((cfg.failure_rate_backpressure - 0.05).abs() < f64::EPSILON);
    }

    #[test]
    fn config_embed_sub_batch_alias_is_reem_batch_size() {
        reset_env();
        std::env::set_var("ENGRAM_REEMBED_BATCH_SIZE", "23");
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert_eq!(cfg.batch_size, 23);
        assert_eq!(cfg.embed_sub_batch, 23);
    }
}

#[cfg(test)]
mod adaptive_concurrency_tests {
    use super::*;

    fn controller(min: usize, max: usize) -> AdaptiveConcurrency {
        AdaptiveConcurrency::new(min, max, 2000, 400, 3, 0.10)
    }

    // ── Slow-start: begins at min, not max ────────────────────────────────────

    #[test]
    fn initial_current_equals_min() {
        let c = controller(1, 8);
        assert_eq!(c.current, 1);
    }

    // ── 429 / failure backpressure ────────────────────────────────────────────

    #[test]
    fn any_failed_sub_batches_halves_concurrency() {
        let mut c = controller(1, 8);
        // Ramp to 4 via successive clean batches, then hit a 429.
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean — ramp to 2
        c.update(10, 0, 1, Duration::from_millis(1_000));
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 3 → 2
        c.update(10, 0, 1, Duration::from_millis(1_000));
        c.update(10, 0, 1, Duration::from_millis(1_000));
        c.update(10, 0, 1, Duration::from_millis(1_000)); // → 3
        c.update(10, 0, 1, Duration::from_millis(1_000));
        c.update(10, 0, 1, Duration::from_millis(1_000));
        c.update(10, 0, 1, Duration::from_millis(1_000)); // → 4
        assert_eq!(c.current, 4);
        // Single failed sub-batch — halve immediately regardless of latency.
        c.update(10, 1, 1, Duration::from_millis(30)); // 30ms looks fast but has failures
        assert_eq!(c.current, 2);
    }

    #[test]
    fn failures_reset_clean_count() {
        let mut c = controller(1, 8);
        // Two clean batches (not enough to ramp), then a failure.
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 1
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 2
        c.update(10, 1, 1, Duration::from_millis(30)); // failure — resets streak
                                                       // Two more clean batches — should NOT ramp (streak restarted).
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 1 (new streak)
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 2
        assert_eq!(c.current, 1); // still at min, streak not complete
    }

    #[test]
    fn failures_override_low_latency_signal() {
        // Failures win even when per-chunk latency looks below low threshold.
        // This is the core 429 bug: 429s return in ~10ms (below latency_low_ms=400)
        // and used to be misread as fast success, ramping concurrency up.
        let mut c = controller(1, 8);
        c.update(10, 0, 1, Duration::from_millis(1_000));
        c.update(10, 0, 1, Duration::from_millis(1_000));
        c.update(10, 0, 1, Duration::from_millis(1_000)); // ramp: 1 → 2
        c.update(2000, 20, 20, Duration::from_millis(50)); // all sub-batches 429 @ ~25µs each
        assert_eq!(c.current, 1); // halved to min, NOT ramped
    }

    #[test]
    fn failures_floor_at_min() {
        let mut c = controller(2, 4);
        c.update(10, 1, 1, Duration::from_millis(30)); // halve 2 → 1, floor at 2
        assert_eq!(c.current, 2);
    }

    #[test]
    fn recovery_after_failures_is_gradual() {
        let mut c = controller(1, 8);
        // Ramp to 4 then hit failures to drop to 2.
        for _ in 0..9 {
            c.update(10, 0, 1, Duration::from_millis(1_000));
        }
        assert_eq!(c.current, 4);
        c.update(10, 1, 1, Duration::from_millis(30)); // 4 → 2
                                                       // Recovery: needs ramp_after=3 clean batches per step.
        c.update(10, 0, 1, Duration::from_millis(1_000));
        c.update(10, 0, 1, Duration::from_millis(1_000));
        assert_eq!(c.current, 2); // not yet
        c.update(10, 0, 1, Duration::from_millis(1_000)); // ramp: 2 → 3
        assert_eq!(c.current, 3);
    }

    // ── Existing latency-based behaviour (unchanged) ──────────────────────────

    #[test]
    fn high_latency_halves_concurrency() {
        let mut c = controller(1, 8);
        // Ramp first so we have room to observe halving.
        for _ in 0..9 {
            c.update(10, 0, 1, Duration::from_millis(1_000));
        }
        assert_eq!(c.current, 4);
        c.update(10, 0, 1, Duration::from_millis(30_000)); // 3000ms/chunk > 2000 threshold
        assert_eq!(c.current, 2);
    }

    #[test]
    fn high_latency_floors_at_min() {
        let mut c = controller(3, 4);
        for _ in 0..3 {
            c.update(10, 0, 1, Duration::from_millis(1_000));
        } // ramp: 3 → 4
        c.update(10, 0, 1, Duration::from_millis(30_000)); // halve 4 → 2, floor at 3
        assert_eq!(c.current, 3);
    }

    #[test]
    fn floor_is_respected_on_md() {
        let mut c = controller(2, 2);
        c.update(10, 0, 1, Duration::from_millis(30_000)); // halve 2 → 1, floor at 2
        assert_eq!(c.current, 2);
    }

    #[test]
    fn low_latency_does_not_ramp_before_threshold() {
        let mut c = controller(1, 8);
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 1
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 2 (ramp_after=3, not yet)
        assert_eq!(c.current, 1);
    }

    #[test]
    fn low_latency_ramps_after_threshold_and_resets_counter() {
        let mut c = controller(1, 8);
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 1
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 2
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 3 → ramp: 1 → 2
        assert_eq!(c.current, 2);
        // counter reset: two more cleans should NOT ramp again
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 1 (reset)
        c.update(10, 0, 1, Duration::from_millis(1_000)); // clean 2
        assert_eq!(c.current, 2);
    }

    #[test]
    fn dead_band_latency_leaves_state_unchanged() {
        let mut c = controller(1, 8);
        for _ in 0..9 {
            c.update(10, 0, 1, Duration::from_millis(1_000));
        } // → 4
        c.update(10, 0, 1, Duration::from_millis(8_000)); // 800ms/chunk — dead band [400, 2000)
        assert_eq!(c.current, 4);
    }

    #[test]
    fn empty_batch_skips_update() {
        let mut c = controller(1, 8);
        for _ in 0..9 {
            c.update(10, 0, 1, Duration::from_millis(1_000));
        } // → 4
        c.update(0, 0, 1, Duration::from_millis(1_000)); // empty — no change
        assert_eq!(c.current, 4);
    }

    #[test]
    fn ceiling_is_respected_on_ramp() {
        let mut c = controller(1, 4);
        for _ in 0..9 {
            c.update(10, 0, 1, Duration::from_millis(1_000));
        }
        assert_eq!(c.current, 4); // at ceiling
        for _ in 0..3 {
            c.update(10, 0, 1, Duration::from_millis(1_000));
        }
        assert_eq!(c.current, 4); // clamped
    }

    // ── Failure-rate threshold (issue #651) ───────────────────────────────────

    #[test]
    fn low_failure_rate_holds_concurrency_no_halve() {
        // 1/20 sub-batches failed (5%) — below 10% threshold.
        // Concurrency must hold; no halve, no ramp credit.
        let mut c = controller(1, 8);
        for _ in 0..9 {
            c.update(100, 0, 1, Duration::from_millis(1_000));
        }
        assert_eq!(c.current, 4); // ramped to 4 by 9 clean batches
        c.update(2000, 1, 20, Duration::from_millis(80_000)); // 5% failure rate
        assert_eq!(c.current, 4); // must not halve
    }

    #[test]
    fn low_failure_rate_does_not_earn_ramp_credit() {
        // A hold does not reset clean_count — prior streak is preserved.
        // 2 clean batches + 1 hold (5%) + 1 clean batch = streak of 3 = ramp.
        let mut c = controller(1, 8);
        c.update(100, 0, 1, Duration::from_millis(1_000)); // streak 1
        c.update(100, 0, 1, Duration::from_millis(1_000)); // streak 2
        c.update(2000, 1, 20, Duration::from_millis(80_000)); // 5% hold — streak unchanged at 2
        c.update(100, 0, 1, Duration::from_millis(1_000)); // streak 3 → ramp
        assert_eq!(c.current, 2);
    }

    #[test]
    fn high_failure_rate_halves_concurrency() {
        // 3/20 (15%) ≥ 10% threshold → halve + reset streak.
        let mut c = controller(1, 8);
        for _ in 0..9 {
            c.update(100, 0, 1, Duration::from_millis(1_000));
        }
        assert_eq!(c.current, 4);
        c.update(2000, 3, 20, Duration::from_millis(80_000)); // 15%
        assert_eq!(c.current, 2);
    }

    #[test]
    fn threshold_boundary_exactly_ten_percent_triggers_backoff() {
        // 10/100 = exactly 10.0% — must trigger halve.
        let mut c = controller(1, 8);
        for _ in 0..9 {
            c.update(100, 0, 1, Duration::from_millis(1_000));
        }
        assert_eq!(c.current, 4);
        c.update(1000, 10, 100, Duration::from_millis(80_000));
        assert_eq!(c.current, 2);
    }

    #[test]
    fn threshold_boundary_nine_percent_holds() {
        // 9/100 = 9% < 10% — must hold, not halve.
        let mut c = controller(1, 8);
        for _ in 0..9 {
            c.update(100, 0, 1, Duration::from_millis(1_000));
        }
        assert_eq!(c.current, 4);
        c.update(1000, 9, 100, Duration::from_millis(80_000));
        assert_eq!(c.current, 4);
    }

    #[test]
    fn total_sub_batches_zero_does_not_panic() {
        // total_sub_batches=0 must not panic (treated as 0% failure rate).
        let mut c = controller(1, 8);
        c.update(10, 0, 0, Duration::from_millis(1_000)); // no panic = pass
        assert_eq!(c.current, 1); // ramp credit from low latency
    }
}

// ── Ordering regression tests ─────────────────────────────────────────────────

#[cfg(test)]
mod ordering_tests {
    /// Regression guard: the chunk-selection query must use ORDER BY id DESC
    /// so that newest (highest-id) chunks are embedded first (#914).
    #[test]
    fn chunk_query_uses_desc_ordering() {
        let query = r#"
        SELECT id, chunk_text
        FROM chunks
        WHERE embedding IS NULL
        ORDER BY id DESC
        LIMIT $1
        FOR UPDATE SKIP LOCKED
        "#;
        assert!(
            query.contains("ORDER BY id DESC"),
            "chunk-selection query must use ORDER BY id DESC (newest-first) — found ASC or missing (#914)"
        );
        assert!(
            !query.contains("ORDER BY id\n"),
            "chunk-selection query must not use bare ORDER BY id (ASC) — must be DESC (#914)"
        );
    }
}
