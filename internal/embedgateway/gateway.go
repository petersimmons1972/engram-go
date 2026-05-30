package embedgateway

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petersimmons1972/engram/internal/embedmodel"
	"github.com/petersimmons1972/engram/internal/metrics"
)

const (
	DefaultBatchSize      = 100
	DefaultMaxConcurrency = 8
	DefaultMinIdle        = 5 * time.Second
	DefaultMaxIdle        = 300 * time.Second

	consecutiveValidationThreshold = 5
	defaultHoldDuration            = 2 * time.Minute
)

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedWithModel(ctx context.Context, text string) ([]float32, string, error)
	Name() string
	Dimensions() int
}

type drainFunc func(context.Context) (int, error)

type EmbedGateway struct {
	pool     *pgxpool.Pool
	userPool *pgxpool.Pool
	embedder Embedder

	batchSize int
	minIdle   time.Duration
	maxIdle   time.Duration
	holdFor   time.Duration
	throttle  *AdaptiveThrottle

	stateMu                  sync.Mutex
	holdUntil                time.Time
	consecutiveValidationErr atomic.Int64

	notify    chan struct{}
	done      chan struct{}
	stop      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once

	testDrain drainFunc
}

func New(pool, userPool *pgxpool.Pool, embedder Embedder, batchSize int) *EmbedGateway {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	return &EmbedGateway{
		pool:      pool,
		userPool:  userPool,
		embedder:  embedder,
		batchSize: batchSize,
		minIdle:   DefaultMinIdle,
		maxIdle:   DefaultMaxIdle,
		holdFor:   defaultHoldDuration,
		throttle:  NewAdaptiveThrottle(DefaultMaxConcurrency),
		notify:    make(chan struct{}, 1),
		done:      make(chan struct{}),
		stop:      make(chan struct{}),
	}
}

func NewForTest(embedder Embedder, drain drainFunc) *EmbedGateway {
	g := New(nil, nil, embedder, 1)
	g.minIdle = time.Hour
	g.maxIdle = time.Hour
	g.testDrain = drain
	return g
}

func (g *EmbedGateway) Enqueue(chunkIDs []string) {
	if len(chunkIDs) == 0 {
		return
	}
	select {
	case g.notify <- struct{}{}:
	default:
	}
}

func (g *EmbedGateway) Start(ctx context.Context) {
	g.startOnce.Do(func() {
		go g.run(ctx)
	})
}

func (g *EmbedGateway) Stop() {
	g.stopOnce.Do(func() { close(g.stop) })
	<-g.done
}

func (g *EmbedGateway) WaitDrained(ctx context.Context) error {
	if g.pool == nil {
		return nil
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var pending int
		if err := g.pool.QueryRow(ctx, "SELECT COUNT(*) FROM chunks WHERE embedding IS NULL").Scan(&pending); err != nil {
			return fmt.Errorf("count pending embeddings: %w", err)
		}
		if pending == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (g *EmbedGateway) run(ctx context.Context) {
	defer close(g.done)
	slog.Info("embed gateway started", "batch_size", g.batchSize)

	backoff := g.minIdle
	for {
		if g.userPool != nil {
			stat := g.userPool.Stat()
			g.throttle.Update(ThrottleStats{AcquiredConns: stat.AcquiredConns(), MaxConns: stat.MaxConns()})
			metrics.EmbedGatewayConcurrency.Set(float64(g.throttle.Concurrency()))
		}
		g.updatePendingGauge(ctx)

		iterCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		n, err := g.drainBatch(iterCtx)
		cancel()
		if err != nil {
			slog.Warn("embed gateway drain error", "err", err)
			metrics.EmbedGatewayBatches.WithLabelValues("error").Inc()
		} else if n > 0 {
			metrics.EmbedGatewayBatches.WithLabelValues("work").Inc()
		} else {
			metrics.EmbedGatewayBatches.WithLabelValues("empty").Inc()
		}
		select {
		case <-g.stop:
			return
		default:
		}
		if n > 0 {
			backoff = g.minIdle
			if n == g.batchSize {
				continue
			}
		} else if backoff < g.maxIdle {
			backoff = min(backoff*2, g.maxIdle)
		}

		if g.pool != nil {
			woke, ok := g.waitForWake(ctx, backoff)
			if !ok {
				return
			}
			if woke {
				backoff = g.minIdle
			}
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-g.stop:
			return
		case <-g.notify:
			backoff = g.minIdle
		case <-time.After(backoff):
		}
	}
}

func (g *EmbedGateway) inDegradedHold(now time.Time) (time.Duration, bool) {
	g.stateMu.Lock()
	defer g.stateMu.Unlock()
	if g.holdUntil.IsZero() || !now.Before(g.holdUntil) {
		return 0, false
	}
	return time.Until(g.holdUntil), true
}

func (g *EmbedGateway) updatePendingGauge(ctx context.Context) {
	if g.pool == nil {
		return
	}
	countCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var pending int
	if err := g.pool.QueryRow(countCtx, "SELECT COUNT(*) FROM chunks WHERE embedding IS NULL").Scan(&pending); err == nil {
		metrics.ChunksPendingReembed.Set(float64(pending))
	}
}

func (g *EmbedGateway) noteValidationRejected(modelID string, dims int) {
	consecutive := g.consecutiveValidationErr.Add(1)
	if consecutive < consecutiveValidationThreshold {
		return
	}
	g.stateMu.Lock()
	g.holdUntil = time.Now().Add(g.holdFor)
	g.stateMu.Unlock()

	metrics.EmbedGatewayDegraded.Inc()
	slog.Error("embed gateway degraded hold after repeated validation rejections",
		"consecutive_rejections", consecutive,
		"hold_for", g.holdFor,
		"model_id", modelID,
		"dims", dims)
}

func (g *EmbedGateway) noteEmbedAccepted() {
	g.consecutiveValidationErr.Store(0)
}

func (g *EmbedGateway) waitForWake(ctx context.Context, backoff time.Duration) (woke bool, ok bool) {
	acquireCtx, acquireCancel := context.WithTimeout(ctx, backoff)
	conn, err := g.pool.Acquire(acquireCtx)
	acquireCancel()
	if err != nil {
		select {
		case <-ctx.Done():
			return false, false
		case <-g.stop:
			return false, false
		case <-g.notify:
			return true, true
		case <-time.After(backoff):
			return false, true
		}
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+embedmodel.PGNotifyChannel); err != nil {
		slog.Warn("embed gateway LISTEN failed", "err", err)
		return false, true
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, backoff)
	defer waitCancel()

	_, err = conn.Conn().WaitForNotification(waitCtx)
	if err == nil {
		return true, true
	}

	select {
	case <-ctx.Done():
		return false, false
	case <-g.stop:
		return false, false
	case <-g.notify:
		return true, true
	default:
		return false, true
	}
}
