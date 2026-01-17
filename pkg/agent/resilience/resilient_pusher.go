package resilience

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/nanoncore/nano-agent/pkg/agent/poller"
)

// ResilientPusherConfig configures the resilient metrics pusher.
type ResilientPusherConfig struct {
	// InitialBackoff is the initial retry delay.
	InitialBackoff time.Duration
	// MaxBackoff is the maximum retry delay.
	MaxBackoff time.Duration
	// BackoffMultiplier is the multiplier for exponential backoff.
	BackoffMultiplier float64
	// RetryInterval is how often to retry buffered metrics.
	RetryInterval time.Duration
	// MaxRetries is the maximum number of retries per batch (0 = unlimited).
	MaxRetries int
	// LogPrefix is prepended to log messages.
	LogPrefix string
}

// DefaultResilientPusherConfig returns the default configuration.
func DefaultResilientPusherConfig() ResilientPusherConfig {
	return ResilientPusherConfig{
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		RetryInterval:     30 * time.Second,
		MaxRetries:        5,
		LogPrefix:         "[resilient-pusher]",
	}
}

// ResilientMetricsPusher wraps a MetricsPusher with circuit breaker and retry logic.
type ResilientMetricsPusher struct {
	mu sync.RWMutex

	inner          poller.MetricsPusher
	config         ResilientPusherConfig
	circuitBreaker *CircuitBreaker
	buffer         *MetricsBuffer

	// Background retry state
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Stats
	totalPushed   int64
	totalFailed   int64
	totalBuffered int64
	totalRetried  int64
}

// NewResilientMetricsPusher creates a new resilient metrics pusher.
func NewResilientMetricsPusher(
	inner poller.MetricsPusher,
	config ResilientPusherConfig,
	cbConfig CircuitBreakerConfig,
	bufferConfig MetricsBufferConfig,
) *ResilientMetricsPusher {
	ctx, cancel := context.WithCancel(context.Background())

	rp := &ResilientMetricsPusher{
		inner:          inner,
		config:         config,
		circuitBreaker: NewCircuitBreaker(cbConfig),
		buffer:         NewMetricsBuffer(bufferConfig),
		ctx:            ctx,
		cancel:         cancel,
	}

	// Start background retry goroutine
	rp.wg.Add(1)
	go rp.retryLoop()

	return rp
}

// PushMetrics pushes metrics with circuit breaker and buffering.
func (rp *ResilientMetricsPusher) PushMetrics(batch *poller.MetricsBatch) (*poller.PushMetricsResponse, error) {
	if batch == nil || len(batch.Metrics) == 0 {
		return &poller.PushMetricsResponse{Success: true, Count: 0}, nil
	}

	// Check circuit breaker
	if !rp.circuitBreaker.Allow() {
		rp.log("Circuit open, buffering %d metrics", len(batch.Metrics))
		rp.buffer.Add(batch)
		rp.mu.Lock()
		rp.totalBuffered++
		rp.mu.Unlock()
		return &poller.PushMetricsResponse{
			Success: false,
			Message: "circuit breaker open, metrics buffered",
		}, nil
	}

	// Try to push with retry
	resp, err := rp.pushWithRetry(batch)
	if err != nil {
		rp.circuitBreaker.RecordFailure()
		rp.buffer.Add(batch)
		rp.mu.Lock()
		rp.totalFailed++
		rp.totalBuffered++
		rp.mu.Unlock()
		rp.log("Push failed, buffering %d metrics: %v", len(batch.Metrics), err)
		return nil, err
	}

	rp.circuitBreaker.RecordSuccess()
	rp.mu.Lock()
	rp.totalPushed++
	rp.mu.Unlock()

	return resp, nil
}

// pushWithRetry attempts to push with exponential backoff.
func (rp *ResilientMetricsPusher) pushWithRetry(batch *poller.MetricsBatch) (*poller.PushMetricsResponse, error) {
	backoff := rp.config.InitialBackoff
	var lastErr error

	for attempt := 0; rp.config.MaxRetries == 0 || attempt < rp.config.MaxRetries; attempt++ {
		// Check context
		select {
		case <-rp.ctx.Done():
			return nil, rp.ctx.Err()
		default:
		}

		resp, err := rp.inner.PushMetrics(batch)
		if err == nil && resp.Success {
			return resp, nil
		}

		if err != nil {
			lastErr = err
		}

		// Don't sleep on the last attempt
		if rp.config.MaxRetries > 0 && attempt >= rp.config.MaxRetries-1 {
			break
		}

		// Exponential backoff
		select {
		case <-rp.ctx.Done():
			return nil, rp.ctx.Err()
		case <-time.After(backoff):
		}

		backoff = time.Duration(float64(backoff) * rp.config.BackoffMultiplier)
		if backoff > rp.config.MaxBackoff {
			backoff = rp.config.MaxBackoff
		}
	}

	return nil, lastErr
}

// retryLoop periodically retries buffered metrics.
func (rp *ResilientMetricsPusher) retryLoop() {
	defer rp.wg.Done()

	ticker := time.NewTicker(rp.config.RetryInterval)
	defer ticker.Stop()

	// Also run cleanup periodically
	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-rp.ctx.Done():
			return

		case <-ticker.C:
			rp.retryBufferedMetrics()

		case <-cleanupTicker.C:
			dropped := rp.buffer.CleanupStale()
			if dropped > 0 {
				rp.log("Dropped %d stale batches", dropped)
			}
		}
	}
}

// retryBufferedMetrics attempts to push buffered metrics.
func (rp *ResilientMetricsPusher) retryBufferedMetrics() {
	// Only retry if circuit breaker allows
	if !rp.circuitBreaker.Allow() {
		return
	}

	batches := rp.buffer.DrainN(10) // Process in small batches
	if len(batches) == 0 {
		return
	}

	rp.log("Retrying %d buffered batches", len(batches))

	var failed []*BufferedBatch
	for _, buffered := range batches {
		batch, ok := buffered.Data.(*poller.MetricsBatch)
		if !ok {
			continue
		}

		resp, err := rp.inner.PushMetrics(batch)
		if err != nil || !resp.Success {
			// Check if we should keep retrying
			if rp.config.MaxRetries > 0 && buffered.Attempts >= rp.config.MaxRetries {
				rp.log("Dropping batch after %d attempts", buffered.Attempts)
				continue
			}
			failed = append(failed, buffered)
			rp.circuitBreaker.RecordFailure()
		} else {
			rp.circuitBreaker.RecordSuccess()
			rp.mu.Lock()
			rp.totalRetried++
			rp.mu.Unlock()
		}
	}

	// Requeue failed batches
	if len(failed) > 0 {
		rp.buffer.Requeue(failed)
		rp.log("Requeued %d failed batches", len(failed))
	}
}

// Stop stops the background retry goroutine.
func (rp *ResilientMetricsPusher) Stop() {
	rp.cancel()
	rp.wg.Wait()
}

// Stats returns current statistics.
func (rp *ResilientMetricsPusher) Stats() ResilientPusherStats {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	return ResilientPusherStats{
		TotalPushed:         rp.totalPushed,
		TotalFailed:         rp.totalFailed,
		TotalBuffered:       rp.totalBuffered,
		TotalRetried:        rp.totalRetried,
		BufferSize:          rp.buffer.Size(),
		CircuitBreakerState: rp.circuitBreaker.State().String(),
	}
}

// ResilientPusherStats contains statistics about the resilient pusher.
type ResilientPusherStats struct {
	TotalPushed         int64
	TotalFailed         int64
	TotalBuffered       int64
	TotalRetried        int64
	BufferSize          int
	CircuitBreakerState string
}

// log outputs a log message with the pusher prefix.
func (rp *ResilientMetricsPusher) log(format string, args ...interface{}) {
	log.Printf("%s %s "+format, append([]interface{}{time.Now().Format("15:04:05"), rp.config.LogPrefix}, args...)...)
}
