#[path = "../src/main.rs"]
mod app;

use app::{claim::process_slice, run_with_shutdown, Config};
use reqwest::Client as HttpClient;
use serde_json::json;
use sqlx::postgres::PgPoolOptions;
use sqlx::PgPool;
use std::collections::HashMap;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use tokio::task::JoinSet;
use tokio_util::sync::CancellationToken;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

#[derive(Clone)]
struct SchemaCtx {
    pool: PgPool,
    schema: String,
}

fn now_nanos() -> u128 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or_default()
}

fn schema_name() -> String {
    format!("it_{}", now_nanos())
}

fn quoted(name: &str) -> String {
    format!("\"{name}\"")
}

fn embed_response_body() -> serde_json::Value {
    let mut data = Vec::with_capacity(200);
    for i in 0..200 {
        data.push(json!({
            "index": i,
            "embedding": [i as f32, 0.5, 0.25],
        }));
    }
    json!({"data": data})
}

fn response_template(delay: Duration) -> ResponseTemplate {
    ResponseTemplate::new(200)
        .set_body_json(embed_response_body())
        .set_delay(delay)
}

fn scoped_database_url(base: &str, schema: &str) -> String {
    let sep = if base.contains('?') { '&' } else { '?' };
    format!("{base}{sep}options=-csearch_path={schema}")
}

async fn setup_schema(database_url: &str) -> anyhow::Result<SchemaCtx> {
    let schema = schema_name();
    let scoped = scoped_database_url(database_url, &schema);
    let pool = PgPoolOptions::new()
        .max_connections(16)
        .connect(&scoped)
        .await
        .map_err(anyhow::Error::from)?;

    let schema_q = quoted(&schema);
    sqlx::query(&format!("CREATE SCHEMA IF NOT EXISTS {schema_q}"))
        .execute(&pool)
        .await?;
    sqlx::query(&format!("DROP TABLE IF EXISTS {schema_q}.chunks CASCADE"))
        .execute(&pool)
        .await?;
    sqlx::query("CREATE EXTENSION IF NOT EXISTS vector")
        .execute(&pool)
        .await?;
    sqlx::query(&format!(
        "CREATE TABLE {schema_q}.chunks (\n            id text PRIMARY KEY,\n            chunk_text text NOT NULL,\n            embedding vector(3),\n            updated_count int NOT NULL DEFAULT 0\n        )"
    ))
    .execute(&pool)
    .await?;

    let func = format!("{schema}_reembed_update_count_fn");
    let trg = format!("{schema}_reembed_update_count_tg");
    let func_q = quoted(&func);
    let trg_q = quoted(&trg);
    sqlx::query(&format!(
        "CREATE OR REPLACE FUNCTION {func_q}() RETURNS TRIGGER AS $$\nBEGIN\n    NEW.updated_count := COALESCE(OLD.updated_count, 0) + 1;\n    RETURN NEW;\nEND;\n$$ LANGUAGE plpgsql"
    ))
    .execute(&pool)
    .await?;
    sqlx::query(&format!(
        "CREATE TRIGGER {trg_q} BEFORE UPDATE OF embedding ON {schema_q}.chunks FOR EACH ROW EXECUTE FUNCTION {func_q}()"
    ))
    .execute(&pool)
    .await?;

    Ok(SchemaCtx { pool, schema })
}

async fn insert_rows(ctx: &SchemaCtx, total: usize) -> anyhow::Result<()> {
    let table = format!("{}.chunks", quoted(&ctx.schema));
    for i in 0..total {
        sqlx::query(&format!("INSERT INTO {table} (id, chunk_text) VALUES ($1, $2)"))
            .bind(format!("row-{i}"))
            .bind(format!("chunk text {i}"))
            .execute(&ctx.pool)
            .await?;
    }
    Ok(())
}

async fn count_embedded(ctx: &SchemaCtx) -> anyhow::Result<i64> {
    let table = format!("{}.chunks", quoted(&ctx.schema));
    let count = sqlx::query_scalar::<_, i64>(&format!(
        "SELECT count(*) FROM {table} WHERE embedding IS NOT NULL"
    ))
    .fetch_one(&ctx.pool)
    .await?;
    Ok(count)
}

async fn count_dupe_updates(ctx: &SchemaCtx) -> anyhow::Result<i64> {
    let table = format!("{}.chunks", quoted(&ctx.schema));
    let count = sqlx::query_scalar::<_, i64>(&format!(
        "SELECT count(*) FROM {table} WHERE updated_count > 1"
    ))
    .fetch_one(&ctx.pool)
    .await?;
    Ok(count)
}

fn test_config(database_url: &str, litellm_url: &str, workers: usize, batch: usize) -> Config {
    Config {
        database_url: database_url.to_string(),
        litellm_url: litellm_url.to_string(),
        litellm_api_key: String::new(),
        embed_model: "mock-embedding-model".to_string(),
        embed_dims: Some(3),
        max_chunk_chars: 2048,
        batch_size: batch,
        embed_sub_batch: batch,
        interval: Duration::from_millis(10),
        embed_timeout: Duration::from_millis(60),
        startup_probe_max_attempts: 2,
        startup_probe_initial_backoff: Duration::from_millis(1),
        startup_probe_max_backoff: Duration::from_millis(10),
        concurrency_min: 1,
        concurrency_max: workers,
        latency_high_ms: 2000,
        latency_low_ms: 400,
        ramp_after: 3,
        failure_rate_backpressure: 0.10,
    }
}

async fn wait_processed(ctx: &SchemaCtx, expected: i64, timeout: Duration) -> anyhow::Result<()> {
    let deadline = std::time::Instant::now() + timeout;
    loop {
        if count_embedded(ctx).await? >= expected {
            return Ok(());
        }
        if std::time::Instant::now() > deadline {
            anyhow::bail!("timed out waiting for {expected} completed rows");
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }
}

async fn run_custom_workers(
    ctx: SchemaCtx,
    cfg: Config,
    endpoints: Vec<String>,
    shutdown: CancellationToken,
    duration: Duration,
) -> anyhow::Result<HashMap<String, usize>> {
    let mut set = JoinSet::new();
    for endpoint in endpoints {
        let mut local_cfg = cfg.clone();
        local_cfg.litellm_url = endpoint.clone();
        let pool = ctx.pool.clone();
        let token = shutdown.clone();
        let http = HttpClient::new();
        let endpoint_id = endpoint.clone();
        set.spawn(async move {
            let mut claimed_total = 0usize;
            loop {
                if token.is_cancelled() {
                    break;
                }
                let (claimed, _) = process_slice(&pool, &http, &local_cfg)
                    .await
                    .unwrap_or((0, 0));
                claimed_total += claimed;
                if claimed == 0 {
                    tokio::time::sleep(Duration::from_millis(5)).await;
                }
            }
            (endpoint_id, claimed_total)
        });
    }

    tokio::time::sleep(duration).await;
    shutdown.cancel();

    let mut totals = HashMap::new();
    while let Some(joined) = set.join_next().await {
        if let Ok((endpoint, n)) = joined {
            totals.entry(endpoint).and_modify(|v| *v += n).or_insert(n);
        }
    }

    Ok(totals)
}

#[tokio::test]
async fn skip_locked_no_double_processing() -> anyhow::Result<()> {
    let base_url = match std::env::var("TEST_DATABASE_URL") {
        Ok(v) => v,
        Err(_) => return Ok(()),
    };

    let ctx = setup_schema(&base_url).await?;
    insert_rows(&ctx, 20).await?;

    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/embeddings"))
        .respond_with(response_template(Duration::from_millis(2)))
        .mount(&server)
        .await;

    let cfg = test_config(&scoped_database_url(&base_url, &ctx.schema), &server.uri(), 1, 4);
    let shutdown = CancellationToken::new();
    let task = tokio::spawn(run_with_shutdown(cfg, shutdown.clone()));

    wait_processed(&ctx, 20, Duration::from_secs(6)).await?;
    shutdown.cancel();
    task.await??;

    assert_eq!(count_embedded(&ctx).await?, 20);
    assert_eq!(count_dupe_updates(&ctx).await?, 0);

    Ok(())
}

#[tokio::test]
async fn slow_backend_does_not_gate_fast() -> anyhow::Result<()> {
    let base_url = match std::env::var("TEST_DATABASE_URL") {
        Ok(v) => v,
        Err(_) => return Ok(()),
    };

    // Baseline: one fast worker.
    let fast_ctx = setup_schema(&base_url).await?;
    insert_rows(&fast_ctx, 80).await?;

    let fast_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/embeddings"))
        .respond_with(response_template(Duration::from_millis(2)))
        .mount(&fast_server)
        .await;

    let fast_cfg = test_config(
        &scoped_database_url(&base_url, &fast_ctx.schema),
        &fast_server.uri(),
        1,
        4,
    );
    let shutdown = CancellationToken::new();
    let baseline = run_custom_workers(
        fast_ctx.clone(),
        fast_cfg,
        vec![fast_server.uri()],
        shutdown,
        Duration::from_millis(900),
    )
    .await?;
    let fast_baseline = *baseline.get(&fast_server.uri()).unwrap_or(&0);

    // Parallel fast+slow workers on a new schema.
    let mixed_ctx = setup_schema(&base_url).await?;
    insert_rows(&mixed_ctx, 80).await?;

    let slow_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/embeddings"))
        .respond_with(response_template(Duration::from_millis(90)))
        .mount(&slow_server)
        .await;

    let mixed_cfg = test_config(
        &scoped_database_url(&base_url, &mixed_ctx.schema),
        &fast_server.uri(),
        1,
        4,
    );
    let shutdown = CancellationToken::new();
    let mixed = run_custom_workers(
        mixed_ctx,
        mixed_cfg,
        vec![fast_server.uri(), slow_server.uri()],
        shutdown,
        Duration::from_millis(900),
    )
    .await?;

    let fast_mixed = *mixed.get(&fast_server.uri()).unwrap_or(&0);
    let slow_mixed = *mixed.get(&slow_server.uri()).unwrap_or(&0);

    assert!(fast_baseline > 0);
    assert!(fast_mixed + slow_mixed >= fast_baseline);

    Ok(())
}

#[tokio::test]
async fn process_slice_happy_and_failure_paths() -> anyhow::Result<()> {
    let base_url = match std::env::var("TEST_DATABASE_URL") {
        Ok(v) => v,
        Err(_) => return Ok(()),
    };

    let success_ctx = setup_schema(&base_url).await?;
    insert_rows(&success_ctx, 4).await?;

    let success_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/embeddings"))
        .respond_with(response_template(Duration::from_millis(2)))
        .mount(&success_server)
        .await;

    let cfg = test_config(
        &scoped_database_url(&base_url, &success_ctx.schema),
        &success_server.uri(),
        1,
        4,
    );
    let http = HttpClient::new();
    let (claimed, failed) = process_slice(&success_ctx.pool, &http, &cfg).await?;
    assert_eq!(claimed, 4);
    assert_eq!(failed, 0);

    let fail_ctx = setup_schema(&base_url).await?;
    insert_rows(&fail_ctx, 3).await?;

    let timeout_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/embeddings"))
        .respond_with(response_template(Duration::from_millis(120)))
        .mount(&timeout_server)
        .await;

    let mut slow_cfg = test_config(
        &scoped_database_url(&base_url, &fail_ctx.schema),
        &timeout_server.uri(),
        1,
        3,
    );
    slow_cfg.embed_timeout = Duration::from_millis(20);
    let slow_http = HttpClient::new();
    let (failed_claimed, failed_count) = process_slice(&fail_ctx.pool, &slow_http, &slow_cfg).await?;
    assert_eq!(failed_claimed, 3);
    assert_eq!(failed_count, 3);

    Ok(())
}

#[tokio::test]
async fn graceful_shutdown_drains_inflight() -> anyhow::Result<()> {
    let base_url = match std::env::var("TEST_DATABASE_URL") {
        Ok(v) => v,
        Err(_) => return Ok(()),
    };

    let ctx = setup_schema(&base_url).await?;
    insert_rows(&ctx, 30).await?;

    let slow_server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/embeddings"))
        .respond_with(response_template(Duration::from_millis(200)))
        .mount(&slow_server)
        .await;

    let mut cfg = test_config(&scoped_database_url(&base_url, &ctx.schema), &slow_server.uri(), 1, 4);
    cfg.embed_timeout = Duration::from_millis(500);

    let shutdown = CancellationToken::new();
    let runner = tokio::spawn(run_with_shutdown(cfg, shutdown.clone()));

    tokio::time::sleep(Duration::from_millis(120)).await;
    shutdown.cancel();
    runner.await??;

    let done = count_embedded(&ctx).await?;
    assert!(done > 0);

    Ok(())
}

#[tokio::test]
async fn startup_probe_gates_workers() -> anyhow::Result<()> {
    let base_url = match std::env::var("TEST_DATABASE_URL") {
        Ok(v) => v,
        Err(_) => return Ok(()),
    };

    let ctx = setup_schema(&base_url).await?;
    insert_rows(&ctx, 8).await?;

    let mut cfg = test_config(&scoped_database_url(&base_url, &ctx.schema), "http://127.0.0.1:9", 2, 4);
    cfg.startup_probe_max_attempts = 1;
    cfg.startup_probe_initial_backoff = Duration::from_millis(1);

    let result = run_with_shutdown(cfg, CancellationToken::new()).await;
    assert!(result.is_err());
    assert_eq!(count_embedded(&ctx).await?, 0);

    Ok(())
}
