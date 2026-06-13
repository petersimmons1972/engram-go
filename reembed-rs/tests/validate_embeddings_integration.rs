// Integration tests for Patch C: validate_embeddings gate in process_slice.
//
// These tests exercise the full claim → validate → persist path against a live
// Postgres instance.  They are skipped (return Ok(())) when TEST_DATABASE_URL is
// not set — the same convention used by worker_pool.rs.
//
// Each test:
//   - Spins up an isolated schema (nanosecond-stamped, dropped on reconnect)
//     so tests do not interfere with one another.
//   - Uses wiremock to control exactly what the embed endpoint returns.
//   - Calls process_slice(), which internally calls validate_embeddings() before
//     any UPDATE; the DB state after the call is the proof.

#[path = "../src/main.rs"]
mod app;

use app::{claim::process_slice, Config};
use reqwest::Client as HttpClient;
use serde_json::json;
use sqlx::postgres::PgPoolOptions;
use sqlx::PgPool;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

// ── Schema helpers (mirrors worker_pool.rs) ───────────────────────────────────

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
    format!("it_val_{}", now_nanos())
}

fn quoted(name: &str) -> String {
    format!("\"{name}\"")
}

fn scoped_database_url(base: &str, schema: &str) -> String {
    let sep = if base.contains('?') { '&' } else { '?' };
    format!("{base}{sep}options=-csearch_path={schema},public")
}

async fn setup_schema(database_url: &str) -> anyhow::Result<SchemaCtx> {
    let schema = schema_name();
    let scoped = scoped_database_url(database_url, &schema);
    let pool = PgPoolOptions::new()
        .max_connections(4)
        .connect(&scoped)
        .await
        .map_err(anyhow::Error::from)?;

    let schema_q = quoted(&schema);
    sqlx::query(&format!("CREATE SCHEMA IF NOT EXISTS {schema_q}"))
        .execute(&pool)
        .await?;
    sqlx::query(&format!(
        "DROP TABLE IF EXISTS {schema_q}.chunks CASCADE"
    ))
    .execute(&pool)
    .await?;
    sqlx::query("CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public")
        .execute(&pool)
        .await?;
    // dim=3 matches the embedding size the test mock server returns.
    sqlx::query(&format!(
        "CREATE TABLE {schema_q}.chunks (\
            id text PRIMARY KEY,\
            chunk_text text NOT NULL,\
            embedding vector(3)\
        )"
    ))
    .execute(&pool)
    .await?;

    Ok(SchemaCtx { pool, schema })
}

async fn insert_rows(ctx: &SchemaCtx, total: usize) -> anyhow::Result<()> {
    let table = format!("{}.chunks", quoted(&ctx.schema));
    for i in 0..total {
        sqlx::query(&format!(
            "INSERT INTO {table} (id, chunk_text) VALUES ($1, $2)"
        ))
        .bind(format!("row-{i}"))
        .bind(format!("text {i}"))
        .execute(&ctx.pool)
        .await?;
    }
    Ok(())
}

/// Returns the number of rows where embedding IS NOT NULL.
async fn count_embedded(ctx: &SchemaCtx) -> anyhow::Result<i64> {
    let table = format!("{}.chunks", quoted(&ctx.schema));
    let count = sqlx::query_scalar::<_, i64>(&format!(
        "SELECT count(*) FROM {table} WHERE embedding IS NOT NULL"
    ))
    .fetch_one(&ctx.pool)
    .await?;
    Ok(count)
}

// ── Config helper ─────────────────────────────────────────────────────────────

fn make_config(database_url: &str, litellm_url: &str, embed_dims: Option<u32>) -> Config {
    Config {
        database_url: database_url.to_string(),
        litellm_url: litellm_url.to_string(),
        litellm_api_key: String::new(),
        embed_model: "mock-model".to_string(),
        embed_dims,
        max_chunk_chars: 2048,
        batch_size: 10,
        embed_sub_batch: 10,
        interval: Duration::from_millis(10),
        embed_timeout: Duration::from_millis(500),
        startup_probe_max_attempts: 1,
        startup_probe_initial_backoff: Duration::from_millis(1),
        startup_probe_max_backoff: Duration::from_millis(10),
        concurrency_min: 1,
        concurrency_max: 1,
        latency_high_ms: 2000,
        latency_low_ms: 400,
        ramp_after: 3,
        failure_rate_backpressure: 0.10,
    }
}

// ── Embed response builders ───────────────────────────────────────────────────

/// Build a valid embed response body with `count` embeddings of `dims` dimensions each.
fn embed_body(count: usize, dims: usize) -> serde_json::Value {
    let data: Vec<_> = (0..count)
        .map(|i| {
            json!({
                "index": i,
                "embedding": vec![0.1f32; dims],
            })
        })
        .collect();
    json!({ "data": data })
}

// ── Tests ─────────────────────────────────────────────────────────────────────

/// When the embedder returns fewer embeddings than claimed rows (cardinality
/// mismatch), validate_embeddings must reject the whole slice atomically:
/// zero rows written, all rows remain embedding IS NULL.
#[tokio::test]
async fn wrong_cardinality_writes_nothing() -> anyhow::Result<()> {
    let base_url = match std::env::var("TEST_DATABASE_URL") {
        Ok(v) => v,
        Err(_) => return Ok(()),
    };

    // 4 rows claimed; embed response returns only 2 embeddings (truncated).
    let ctx = setup_schema(&base_url).await?;
    insert_rows(&ctx, 4).await?;

    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/embeddings"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(embed_body(2, 3)), // 2 < 4: cardinality mismatch
        )
        .mount(&server)
        .await;

    let cfg = make_config(
        &scoped_database_url(&base_url, &ctx.schema),
        &server.uri(),
        None, // no dimension constraint — only cardinality is wrong
    );
    let http = HttpClient::new();
    let outcome = process_slice(&ctx.pool, &http, &cfg).await?;

    // validate_embeddings fires before any UPDATE → zero writes, all failed.
    assert_eq!(outcome.attempted, 4, "attempted must equal row count");
    assert_eq!(outcome.written, 0, "zero rows must be written on cardinality mismatch");
    assert_eq!(outcome.failed, 4, "all rows must be counted as failed");

    // Confirm at the DB level: no embeddings were persisted.
    assert_eq!(
        count_embedded(&ctx).await?,
        0,
        "DB must have zero embedded rows after cardinality mismatch"
    );

    Ok(())
}

/// When embeddings have the wrong dimension vs cfg.embed_dims, validate_embeddings
/// must reject the whole slice: zero rows written, all rows remain NULL.
#[tokio::test]
async fn wrong_dims_writes_nothing() -> anyhow::Result<()> {
    let base_url = match std::env::var("TEST_DATABASE_URL") {
        Ok(v) => v,
        Err(_) => return Ok(()),
    };

    // 3 rows claimed; schema uses vector(3); cfg.embed_dims = Some(3).
    // Mock returns 3 embeddings but with dim=5 (wrong).
    let ctx = setup_schema(&base_url).await?;
    insert_rows(&ctx, 3).await?;

    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/embeddings"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(embed_body(3, 5)), // count ok, but dim 5 ≠ expected 3
        )
        .mount(&server)
        .await;

    let cfg = make_config(
        &scoped_database_url(&base_url, &ctx.schema),
        &server.uri(),
        Some(3), // expect dim=3; mock returns dim=5 → dimension mismatch
    );
    let http = HttpClient::new();
    let outcome = process_slice(&ctx.pool, &http, &cfg).await?;

    assert_eq!(outcome.attempted, 3);
    assert_eq!(outcome.written, 0, "zero rows must be written on dimension mismatch");
    assert_eq!(outcome.failed, 3, "all rows must be counted as failed");

    assert_eq!(
        count_embedded(&ctx).await?,
        0,
        "DB must have zero embedded rows after dimension mismatch"
    );

    Ok(())
}

/// A valid batch (correct cardinality + correct dims) must persist all rows
/// atomically: written == attempted, all rows non-NULL after the call.
#[tokio::test]
async fn valid_batch_persists_atomically() -> anyhow::Result<()> {
    let base_url = match std::env::var("TEST_DATABASE_URL") {
        Ok(v) => v,
        Err(_) => return Ok(()),
    };

    // 5 rows; embed response returns exactly 5 embeddings of dim=3.
    let ctx = setup_schema(&base_url).await?;
    insert_rows(&ctx, 5).await?;

    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/v1/embeddings"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_json(embed_body(5, 3)), // 5 embeddings, dim=3: all correct
        )
        .mount(&server)
        .await;

    let cfg = make_config(
        &scoped_database_url(&base_url, &ctx.schema),
        &server.uri(),
        Some(3), // matches mock dim
    );
    let http = HttpClient::new();
    let outcome = process_slice(&ctx.pool, &http, &cfg).await?;

    assert_eq!(outcome.attempted, 5);
    assert_eq!(outcome.written, 5, "all rows must be written on valid batch");
    assert_eq!(outcome.failed, 0, "zero failures on valid batch");

    assert_eq!(
        count_embedded(&ctx).await?,
        5,
        "DB must show all 5 rows embedded after valid batch"
    );

    Ok(())
}
