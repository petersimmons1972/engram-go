use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};
use std::sync::Arc;
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
        bail!(
            "embed endpoint unreachable after startup probe retries: {}",
            e
        );
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
    duration.as_millis().try_into().unwrap_or(u64::MAX)
}

fn increase_backoff(current_ms: u64) -> u64 {
    (current_ms * 2).min(millis_as_u64(Duration::from_secs(300)))
}

/// Determine the next backoff duration based on the outcome of a process_slice call.
///
/// Preconditions: `outcome.attempted > 0` (empty-slice is handled by the caller before
/// this function is invoked); `max_backoff_ms > 0`; `min_backoff_ms <= max_backoff_ms`.
///
/// Decision logic:
/// - Any HTTP failures (failed > 0): Grow — backend errors detected. This includes short-payload
///   responses where the embed API returns fewer vectors than chunks requested (claim.rs:160-164
///   increments failed++ per missing embedding); partial truncation is treated as a backend error.
/// - SKIP LOCKED contention loss (attempted>0, written=0, failed=0): Reset — backend was
///   fine; another worker won the race. Do not penalize a healthy backend.
/// - Partial or full progress (written > 0, failed == 0): Reset — backend healthy.
pub(crate) fn next_backoff_ms(
    outcome: &claim::ProcessSliceResult,
    current_backoff_ms: u64,
    min_backoff_ms: u64,
    max_backoff_ms: u64,
) -> u64 {
    debug_assert!(outcome.attempted > 0, "next_backoff_ms called with attempted == 0; empty-slice must be handled by caller");
    debug_assert!(max_backoff_ms > 0, "max_backoff_ms must be > 0");

    if outcome.failed > 0 {
        // Any HTTP failures → grow
        increase_backoff(current_backoff_ms).min(max_backoff_ms)
    } else {
        // attempted>0, failed==0 → either written>0 (progress) or written==0 (contention loss)
        // Both are "backend healthy" signals → reset to the configured interval floor
        min_backoff_ms
    }
}

async fn run_worker(
    worker_id: usize,
    pool: PgPool,
    http: HttpClient,
    cfg: Config,
    shutdown: CancellationToken,
    in_flight: Arc<AtomicUsize>,
    attempted: Arc<AtomicUsize>,
    completed: Arc<AtomicUsize>,
    failed: Arc<AtomicUsize>,
    backoff_ms: Arc<AtomicU64>,
) {
    let max_backoff_ms = millis_as_u64(Duration::from_secs(300));
    let mut worker_attempted = 0usize;
    let mut worker_written = 0usize;
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
            Ok(outcome) => {
                let elapsed = started.elapsed();
                if outcome.attempted == 0 {
                    let wait_ms = backoff_ms.load(Ordering::Acquire);
                    let next_ms = increase_backoff(wait_ms);
                    // compare_exchange here vs store in the failure path below: intentional
                    // asymmetry. In the idle path (no eligible chunks) multiple workers race to
                    // grow the shared backoff counter — compare_exchange lets exactly one winner
                    // commit the update while the rest silently retry their own load+grow cycle on
                    // the next iteration. The failure path (failed > 0) uses store instead because
                    // any worker observing embed errors should unconditionally impose its computed
                    // backoff; a later store always wins (last writer wins), which is acceptable
                    // since all workers converge on the same exponential curve regardless of
                    // ordering.
                    let _ = backoff_ms.compare_exchange(
                        wait_ms,
                        next_ms.min(max_backoff_ms),
                        Ordering::AcqRel,
                        Ordering::Acquire,
                    );

                    warn!(worker_id, wait_ms, "no claim-eligible chunks; backing off");
                    tokio::select! {
                        _ = tokio::time::sleep(Duration::from_millis(wait_ms)) => {}
                        _ = shutdown.cancelled() => {}
                    }
                    if shutdown.is_cancelled() {
                        break;
                    }
                    continue;
                }

                let interval_ms = millis_as_u64(cfg.interval);
                let next_ms = next_backoff_ms(
                    &outcome,
                    backoff_ms.load(Ordering::Acquire),
                    interval_ms,
                    max_backoff_ms,
                );

                worker_attempted += outcome.attempted;
                worker_written += outcome.written;
                worker_failed += outcome.failed;
                attempted.fetch_add(outcome.attempted, Ordering::AcqRel);
                completed.fetch_add(outcome.written, Ordering::AcqRel);
                failed.fetch_add(outcome.failed, Ordering::AcqRel);

                let secs = elapsed.as_secs_f64().max(0.001);
                let throughput = outcome.written as f64 / secs;
                info!(
                    worker_id,
                    attempted = outcome.attempted,
                    written = outcome.written,
                    failed_slices = outcome.failed,
                    slice_ms = elapsed.as_millis(),
                    throughput_rows_per_sec = throughput,
                    worker_total_attempted = worker_attempted,
                    worker_total_written = worker_written,
                    worker_total_failed = worker_failed,
                    "worker throughput"
                );

                if next_ms > interval_ms {
                    // All or some embed calls failed — backend likely down; back off.
                    backoff_ms.store(next_ms, Ordering::Release);
                    warn!(
                        worker_id,
                        attempted = outcome.attempted,
                        failed = outcome.failed,
                        next_backoff_ms = next_ms,
                        "embed failures detected — backend may be down; backing off"
                    );
                    tokio::select! {
                        _ = tokio::time::sleep(Duration::from_millis(next_ms)) => {}
                        _ = shutdown.cancelled() => {}
                    }
                    if shutdown.is_cancelled() {
                        break;
                    }
                } else {
                    // Backend healthy (progress or contention loss) — reset backoff.
                    backoff_ms.store(interval_ms, Ordering::Release);
                }

                continue;
            }
            Err(err) => {
                warn!(worker_id, err = %err, "claim slice failed");
                let wait_ms = backoff_ms
                    .load(Ordering::Acquire)
                    .max(millis_as_u64(cfg.interval));
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
        attempted = worker_attempted,
        written = worker_written,
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

    // FM-08: embed_timeout must exceed latency_high_ms. If the HTTP timeout fires before
    // the latency threshold, every sub-batch returns as failed, backoff grows indefinitely,
    // and all progress stalls. Warn loudly at startup so operators notice misconfiguration.
    if cfg.embed_timeout.as_millis() as u64 <= cfg.latency_high_ms {
        warn!(
            embed_timeout_ms = cfg.embed_timeout.as_millis(),
            latency_high_ms = cfg.latency_high_ms,
            "FM-08 INVARIANT VIOLATED: embed_timeout must exceed latency_high_ms; \
             embed timeouts will fire before backpressure triggers, causing indefinite backoff"
        );
    }

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
    let attempted = Arc::new(AtomicUsize::new(0));
    let completed = Arc::new(AtomicUsize::new(0));
    let failed = Arc::new(AtomicUsize::new(0));
    let backoff_ms = Arc::new(AtomicU64::new(millis_as_u64(cfg.interval)));

    let gauge = {
        let in_flight = in_flight.clone();
        let attempted = attempted.clone();
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
                            attempted_total = attempted.load(Ordering::Acquire),
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
            attempted.clone(),
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
    let mut runner = tokio::spawn(run_with_shutdown(cfg, shutdown.clone()));

    tokio::select! {
        _ = signal::ctrl_c() => {
            info!("shutdown signal received");
            shutdown.cancel();
            (&mut runner)
                .await
                .context("reembed worker runner join failed")?
        }
        result = &mut runner => {
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
    use std::sync::{Mutex, MutexGuard};

    static ENV_LOCK: Mutex<()> = Mutex::new(());

    fn env_guard() -> MutexGuard<'static, ()> {
        ENV_LOCK.lock().expect("config test env lock poisoned")
    }

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
        let _guard = env_guard();
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
        let _guard = env_guard();
        // Legacy ENGRAM_REEMBED_CONCURRENCY sets concurrency_max when MAX absent.
        reset_env();
        std::env::set_var("ENGRAM_REEMBED_CONCURRENCY", "4");
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert_eq!(cfg.concurrency_max, 4);
    }

    #[test]
    fn config_explicit_max_overrides_legacy() {
        let _guard = env_guard();
        reset_env();
        std::env::set_var("ENGRAM_REEMBED_CONCURRENCY", "4");
        std::env::set_var("ENGRAM_REEMBED_CONCURRENCY_MAX", "12");
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert_eq!(cfg.concurrency_max, 12);
    }

    #[test]
    fn config_failure_rate_backpressure_default() {
        let _guard = env_guard();
        reset_env();
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert!((cfg.failure_rate_backpressure - 0.10).abs() < f64::EPSILON);
    }

    #[test]
    fn config_failure_rate_backpressure_override() {
        let _guard = env_guard();
        reset_env();
        std::env::set_var("ENGRAM_REEMBED_FAILURE_RATE_BACKPRESSURE", "0.05");
        std::env::set_var("DATABASE_URL", "postgres://test");
        let cfg = Config::from_env();
        assert!((cfg.failure_rate_backpressure - 0.05).abs() < f64::EPSILON);
    }

    #[test]
    fn config_embed_sub_batch_alias_is_reem_batch_size() {
        let _guard = env_guard();
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

// ── Dead-backend backoff decision tests ──────────────────────────────────────

#[cfg(test)]
mod backoff_decision_tests {
    use super::*;
    use super::claim::ProcessSliceResult;

    // All HTTP embed failed → grow backoff
    #[test]
    fn all_failed_slice_grows_backoff() {
        let outcome = ProcessSliceResult { attempted: 32, written: 0, failed: 32 };
        // min=100ms, max=30_000ms, current=100ms → increase_backoff(100)=200
        let next = next_backoff_ms(&outcome, 100, 100, 30_000);
        assert!(next > 100, "expected backoff to grow when all failed");
        assert_eq!(next, 200, "increase_backoff doubles: 100 → 200");
    }

    // Partial progress → reset to min_backoff_ms (backend healthy)
    #[test]
    fn partial_progress_resets_backoff() {
        let outcome = ProcessSliceResult { attempted: 32, written: 16, failed: 0 };
        // Backoff was high (8_000ms); reset must return exactly min_backoff_ms (500ms)
        let next = next_backoff_ms(&outcome, 8_000, 500, 30_000);
        assert_eq!(next, 500, "expected backoff to reset to min_backoff_ms on partial progress");
    }

    // Empty-slice is handled by run_worker's early-continue BEFORE next_backoff_ms is called.
    // This test documents the precondition: next_backoff_ms must not be called with attempted=0.
    // In debug builds the debug_assert fires; in release we document the contract.
    #[test]
    #[cfg(debug_assertions)]
    #[should_panic(expected = "next_backoff_ms called with attempted == 0")]
    fn empty_slice_panics_in_debug() {
        // attempted=0 violates the precondition; run_worker never reaches this call.
        let outcome = ProcessSliceResult { attempted: 0, written: 0, failed: 0 };
        let _ = next_backoff_ms(&outcome, 100, 100, 30_000);
    }

    // SKIP LOCKED contention: all rows claimed, backend embedded OK,
    // but another worker already wrote the embedding → rows_affected=0 for all.
    // attempted>0, written=0, failed=0 → Reset (backend was fine)
    #[test]
    fn contention_loss_not_penalized() {
        let outcome = ProcessSliceResult { attempted: 32, written: 0, failed: 0 };
        // Backoff was high (8_000ms); contention loss means backend was healthy → reset
        let next = next_backoff_ms(&outcome, 8_000, 500, 30_000);
        assert_eq!(next, 500, "expected backoff to reset to min_backoff_ms on contention loss");
    }

    // Mixed failure: some embed OK, some HTTP fail → grow backoff
    #[test]
    fn mixed_failure_penalized() {
        let outcome = ProcessSliceResult { attempted: 32, written: 16, failed: 16 };
        let next = next_backoff_ms(&outcome, 100, 100, 30_000);
        assert!(next > 100, "expected backoff to grow on mixed failure");
        assert_eq!(next, 200, "increase_backoff doubles: 100 → 200");
    }

    // Backoff never exceeds max
    #[test]
    fn backoff_capped_at_max() {
        let outcome = ProcessSliceResult { attempted: 32, written: 0, failed: 32 };
        let next = next_backoff_ms(&outcome, 25_000, 100, 30_000);
        assert_eq!(next, 30_000, "backoff must be capped at max (25_000*2=50_000 → clamped to 30_000)");
    }

    // Reset returns exactly min_backoff_ms — no hardcoded sentinel, the caller's floor
    #[test]
    fn reset_returns_configured_min() {
        let outcome = ProcessSliceResult { attempted: 10, written: 10, failed: 0 };
        // Use a non-round min to confirm no constant is being returned
        let next = next_backoff_ms(&outcome, 5_000, 7_777, 30_000);
        assert_eq!(next, 7_777, "reset must return exactly min_backoff_ms, not a hardcoded sentinel");
    }

    // Short-payload: embed API returned fewer vectors than chunks requested.
    // claim.rs:160-164 increments failed++ per missing embedding; this is the
    // third failed-increment site. attempted=32, written=0, failed>0 → Grow.
    // This test verifies that a partially-truncated payload (backend returning a
    // short response) is treated as a backend error, not a contention loss.
    #[test]
    fn short_payload_grows_backoff() {
        // Simulate: 32 chunks attempted, API returned fewer vectors → some failed,
        // none written (short payload means no embeddings were usable)
        let outcome = ProcessSliceResult { attempted: 32, written: 0, failed: 16 };
        let next = next_backoff_ms(&outcome, 100, 100, 30_000);
        assert!(next > 100, "short payload (truncated embed response) must grow backoff");
        assert_eq!(next, 200, "increase_backoff doubles: 100 → 200");
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
