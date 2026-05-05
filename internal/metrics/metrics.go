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
	)
}
