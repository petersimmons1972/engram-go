// Package metrics defines and registers Prometheus metrics for engram.
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// ToolRequests counts MCP tool calls by tool name and result status.
	// status is "ok" or "error".
	ToolRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_tool_requests_total",
		Help: "Total MCP tool requests",
	}, []string{"tool", "status"})

	// ToolDuration records the latency of each MCP tool call in seconds.
	ToolDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "engram_tool_duration_seconds",
		Help:    "MCP tool request duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"tool"})

	// WorkerTicks counts background worker tick iterations by worker name.
	WorkerTicks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_worker_ticks_total",
		Help: "Background worker tick count",
	}, []string{"worker"})

	// WorkerErrors counts errors encountered by background workers by worker name.
	WorkerErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_worker_errors_total",
		Help: "Background worker error count",
	}, []string{"worker"})

	// ChunksPendingReembed is the current number of chunks with NULL embedding_vec.
	ChunksPendingReembed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "engram_chunks_pending_reembed",
		Help: "Chunks with NULL embedding_vec awaiting reembedding",
	})

	// IngestQueueDepth is the current number of async ingestion jobs queued but not yet started.
	IngestQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "engram_ingest_queue_depth",
		Help: "Async ingestion jobs queued but not yet started",
	})

	// EpisodesStartedTotal counts auto-episodes started on SSE session connect.
	EpisodesStartedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "engram_episodes_started_total",
		Help: "Total auto-episodes started on SSE session connect",
	})

	// EpisodesEndedCleanTotal counts episodes closed cleanly on session disconnect.
	EpisodesEndedCleanTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "engram_episodes_ended_clean_total",
		Help: "Total auto-episodes closed cleanly on session disconnect",
	})

	// EpisodesEndedByReaperTotal counts episodes closed by the TTL reaper.
	// A ratio of reaper >> clean indicates a disconnect-handler bug or crash loop.
	EpisodesEndedByReaperTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "engram_episodes_ended_by_reaper_total",
		Help: "Total auto-episodes closed by the TTL reaper (high reaper:clean ratio indicates disconnect-handler bugs)",
	})

	// EmbedRetries counts embedding requests that were retried due to transient errors
	// (502, 503, 504, connection refused, EOF, context timeout).
	EmbedRetries = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "engram_embed_retries_total",
		Help: "Total embedding retries due to transient errors",
	})

	// EmbedFailures counts final embedding failures by reason.
	// reason is "exhausted" (retries exceeded) or "non_retryable" (4xx except 408/429, context canceled, etc.).
	EmbedFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_embed_failures_total",
		Help: "Total final embedding failures by reason",
	}, []string{"reason"})

	EmbedValidationRejections = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_embed_validation_rejections_total",
		Help: "Embedding responses rejected before storage by rejection class",
	}, []string{"class"})

	EmbedGatewayDegraded = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "engram_embed_gateway_degraded_total",
		Help: "Times the embedding gateway entered degraded hold after repeated validation rejections",
	})

	EmbedGatewayBatches = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_embed_gateway_batches_total",
		Help: "Embedding gateway drain batches by result",
	}, []string{"result"})

	EmbedGatewayConcurrency = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "engram_embed_gateway_concurrency",
		Help: "Current embedding gateway worker concurrency limit",
	})

	// WorkerPanics counts panics caught and recovered by background workers.
	// Incremented by the deferred recover() in each worker's main loop.
	WorkerPanics = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_worker_panics_total",
		Help: "Background worker panics caught and recovered",
	}, []string{"worker"})

	// ExtractionDropped counts entity-extraction jobs dropped when the semaphore is full.
	// Labels: reason="semaphore_full" or "queue_error".
	ExtractionDropped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_extraction_dropped_total",
		Help: "Entity extraction jobs dropped (semaphore_full or queue_error)",
	}, []string{"reason"})

	// EmbedCircuitState records the current state of the embed circuit breaker.
	// Labels: state=open|closed|half_open.
	EmbedCircuitState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "engram_embed_circuit_state",
		Help: "Current state of the embed circuit breaker (1=open, 2=closed, 3=half_open)",
	}, []string{"state"})

	// EmbedCircuitTransitions counts state transitions in the embed circuit breaker.
	// Labels: from=X, to=Y where X,Y are open|closed|half_open.
	EmbedCircuitTransitions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_embed_circuit_transitions_total",
		Help: "Circuit breaker state transitions",
	}, []string{"from", "to"})

	// StoreEmbedAsyncTotal counts memory_store calls that returned before embedding
	// completed (i.e., embed was deferred to the reembed worker). This is the normal
	// path when ENGRAM_STORE_EMBED_MODE=async (default). Monotonically increasing;
	// a flat counter while stores are occurring indicates sync mode is enabled.
	StoreEmbedAsyncTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "engram_store_embed_async_total",
		Help: "memory_store calls that deferred embedding to the reembed worker (async mode)",
	})

	// RecallEmbedTimeoutTotal counts memory_recall calls where the embed query
	// exceeded ENGRAM_EMBED_RECALL_TIMEOUT_MS and fell back to BM25+recency.
	// A sustained high rate indicates embed pool saturation; consider increasing
	// ENGRAM_EMBED_RECALL_TIMEOUT_MS or scaling the embed backend.
	RecallEmbedTimeoutTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "engram_recall_embed_timeout_total",
		Help: "memory_recall calls that exceeded embed timeout and fell back to BM25+recency",
	})

	// RecallDegradedTotal counts memory_recall calls that returned degraded
	// results due to embed backend unavailability/fallback.
	RecallDegradedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_recall_degraded_total",
		Help: "memory_recall degraded responses by reason",
	}, []string{"reason"})

	// #673: pgxpool connection pool gauges. Operators need visibility into
	// pool saturation — until now /health Ping succeeded even when the pool
	// was fully acquired with callers blocking on Acquire.
	DBPoolAcquiredConns = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "engram_db_pool_acquired_conns",
		Help: "Currently-acquired connections from the shared pgxpool",
	})
	DBPoolIdleConns = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "engram_db_pool_idle_conns",
		Help: "Idle connections in the shared pgxpool",
	})
	DBPoolTotalConns = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "engram_db_pool_total_conns",
		Help: "Total (acquired + idle) connections in the shared pgxpool",
	})
	DBPoolMaxConns = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "engram_db_pool_max_conns",
		Help: "Configured maximum connections for the shared pgxpool",
	})
	// AcquireCount and AcquireDuration are exposed as gauges (not counters)
	// because we re-publish the cumulative value at each sample rather than
	// tracking deltas — Prometheus rate() over the gauge gives the same
	// per-second view that a counter would, without delta-tracking bugs.
	DBPoolAcquireCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "engram_db_pool_acquire_count_total",
		Help: "Cumulative number of successful pool acquires (republished each sample)",
	})
	DBPoolAcquireDurationSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "engram_db_pool_acquire_duration_seconds_total",
		Help: "Cumulative wall seconds spent blocked acquiring (republished each sample)",
	})

	// #695: retrieval-drift alert counter. Increments every time an audit
	// snapshot's RBO-vs-previous drops below the alert threshold for a
	// canonical query. Watch this in Prometheus; pair with a runbook entry.
	AuditDriftAlerts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_audit_drift_alerts_total",
		Help: "Retrieval drift alerts (RBO-vs-previous below alert_threshold) per project",
	}, []string{"project"})

	// ConsolidateBatchErrors counts batch errors during ConsolidateWithClaude.
	// Incremented when reviewer.ReviewMergeCandidates or backend.MergeMemoriesAtomic fails.
	ConsolidateBatchErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "engram_consolidate_batch_errors_total",
		Help: "Number of batch errors during ConsolidateWithClaude.",
	})
)

func init() {
	prometheus.MustRegister(
		ToolRequests,
		ToolDuration,
		WorkerTicks,
		WorkerErrors,
		ChunksPendingReembed,
		IngestQueueDepth,
		EpisodesStartedTotal,
		EpisodesEndedCleanTotal,
		EpisodesEndedByReaperTotal,
		EmbedRetries,
		EmbedFailures,
		EmbedValidationRejections,
		EmbedGatewayDegraded,
		EmbedGatewayBatches,
		EmbedGatewayConcurrency,
		WorkerPanics,
		ExtractionDropped,
		EmbedCircuitState,
		EmbedCircuitTransitions,
		StoreEmbedAsyncTotal,
		RecallEmbedTimeoutTotal,
		RecallDegradedTotal,
		DBPoolAcquiredConns,
		DBPoolIdleConns,
		DBPoolTotalConns,
		DBPoolMaxConns,
		DBPoolAcquireCount,
		DBPoolAcquireDurationSeconds,
		AuditDriftAlerts,
		ConsolidateBatchErrors,
	)
}
