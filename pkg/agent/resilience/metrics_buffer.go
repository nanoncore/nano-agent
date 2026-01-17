package resilience

import (
	"sync"
	"time"
)

// BufferedBatch represents a metrics batch waiting to be retried.
type BufferedBatch struct {
	Data      interface{}
	Timestamp time.Time
	Attempts  int
}

// MetricsBufferConfig configures the metrics buffer.
type MetricsBufferConfig struct {
	// MaxSize is the maximum number of batches to buffer.
	MaxSize int
	// MaxAge is the maximum age of a batch before it's dropped.
	MaxAge time.Duration
}

// DefaultMetricsBufferConfig returns the default buffer configuration.
func DefaultMetricsBufferConfig() MetricsBufferConfig {
	return MetricsBufferConfig{
		MaxSize: 1000,
		MaxAge:  15 * time.Minute,
	}
}

// MetricsBuffer is a thread-safe buffer for failed metric batches.
type MetricsBuffer struct {
	mu sync.Mutex

	config  MetricsBufferConfig
	batches []*BufferedBatch
}

// NewMetricsBuffer creates a new metrics buffer with the given configuration.
func NewMetricsBuffer(config MetricsBufferConfig) *MetricsBuffer {
	return &MetricsBuffer{
		config:  config,
		batches: make([]*BufferedBatch, 0, config.MaxSize),
	}
}

// Add adds a batch to the buffer.
// If the buffer is full, the oldest batch is dropped.
// Returns true if the batch was added, false if it was rejected (e.g., too old).
func (b *MetricsBuffer) Add(data interface{}) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	batch := &BufferedBatch{
		Data:      data,
		Timestamp: time.Now(),
		Attempts:  1,
	}

	// Drop oldest if at capacity
	if len(b.batches) >= b.config.MaxSize {
		b.batches = b.batches[1:]
	}

	b.batches = append(b.batches, batch)
	return true
}

// DrainAll returns all buffered batches and clears the buffer.
// Stale batches (older than MaxAge) are automatically dropped.
func (b *MetricsBuffer) DrainAll() []*BufferedBatch {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Filter out stale batches
	now := time.Now()
	result := make([]*BufferedBatch, 0, len(b.batches))
	for _, batch := range b.batches {
		if now.Sub(batch.Timestamp) < b.config.MaxAge {
			batch.Attempts++
			result = append(result, batch)
		}
	}

	// Clear the buffer
	b.batches = make([]*BufferedBatch, 0, b.config.MaxSize)

	return result
}

// DrainN returns up to n buffered batches and removes them from the buffer.
// Stale batches are automatically dropped.
func (b *MetricsBuffer) DrainN(n int) []*BufferedBatch {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n <= 0 || len(b.batches) == 0 {
		return nil
	}

	// Filter out stale batches first
	now := time.Now()
	valid := make([]*BufferedBatch, 0, len(b.batches))
	for _, batch := range b.batches {
		if now.Sub(batch.Timestamp) < b.config.MaxAge {
			valid = append(valid, batch)
		}
	}
	b.batches = valid

	// Take up to n batches
	count := n
	if count > len(b.batches) {
		count = len(b.batches)
	}

	result := make([]*BufferedBatch, count)
	for i := 0; i < count; i++ {
		b.batches[i].Attempts++
		result[i] = b.batches[i]
	}

	// Remove drained batches from buffer
	b.batches = b.batches[count:]

	return result
}

// Requeue adds batches back to the front of the buffer for retry.
// This is used when a retry batch fails and needs to be retried again.
func (b *MetricsBuffer) Requeue(batches []*BufferedBatch) {
	if len(batches) == 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Filter stale and prepend valid batches
	now := time.Now()
	valid := make([]*BufferedBatch, 0, len(batches))
	for _, batch := range batches {
		if now.Sub(batch.Timestamp) < b.config.MaxAge {
			valid = append(valid, batch)
		}
	}

	if len(valid) == 0 {
		return
	}

	// Drop oldest if over capacity
	total := len(valid) + len(b.batches)
	if total > b.config.MaxSize {
		// Keep newest batches up to MaxSize
		excess := total - b.config.MaxSize
		if excess >= len(b.batches) {
			b.batches = valid
		} else {
			b.batches = append(valid, b.batches[:len(b.batches)-excess]...)
		}
	} else {
		b.batches = append(valid, b.batches...)
	}
}

// Size returns the current number of buffered batches.
func (b *MetricsBuffer) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.batches)
}

// Clear removes all batches from the buffer.
func (b *MetricsBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.batches = make([]*BufferedBatch, 0, b.config.MaxSize)
}

// CleanupStale removes batches older than MaxAge.
// This should be called periodically to prevent memory from growing.
func (b *MetricsBuffer) CleanupStale() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	valid := make([]*BufferedBatch, 0, len(b.batches))
	dropped := 0

	for _, batch := range b.batches {
		if now.Sub(batch.Timestamp) < b.config.MaxAge {
			valid = append(valid, batch)
		} else {
			dropped++
		}
	}

	b.batches = valid
	return dropped
}

// Stats returns current statistics for the buffer.
func (b *MetricsBuffer) Stats() MetricsBufferStats {
	b.mu.Lock()
	defer b.mu.Unlock()

	var oldestAge time.Duration
	var totalAttempts int

	if len(b.batches) > 0 {
		oldestAge = time.Since(b.batches[0].Timestamp)
		for _, batch := range b.batches {
			totalAttempts += batch.Attempts
		}
	}

	return MetricsBufferStats{
		Size:          len(b.batches),
		MaxSize:       b.config.MaxSize,
		OldestAge:     oldestAge,
		TotalAttempts: totalAttempts,
	}
}

// MetricsBufferStats contains statistics about the metrics buffer.
type MetricsBufferStats struct {
	Size          int
	MaxSize       int
	OldestAge     time.Duration
	TotalAttempts int
}
