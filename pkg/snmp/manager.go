package snmp

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Manager handles concurrent SNMP polling of multiple OLT devices.
type Manager struct {
	collectors map[string]Collector
	stats      map[string]*CollectionStats
	mu         sync.RWMutex
	interval   time.Duration
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	// Callbacks
	OnTelemetry func(host string, telemetry *OLTTelemetry)
	OnError     func(host string, err error)
}

// ManagerConfig holds manager configuration.
type ManagerConfig struct {
	PollInterval time.Duration   `json:"poll_interval"`
	Devices      []DeviceConfig  `json:"devices"`
}

// NewManager creates a new SNMP manager.
func NewManager(interval time.Duration) *Manager {
	if interval == 0 {
		interval = 60 * time.Second
	}
	return &Manager{
		collectors: make(map[string]Collector),
		stats:      make(map[string]*CollectionStats),
		interval:   interval,
	}
}

// AddDevice adds a device to be polled.
func (m *Manager) AddDevice(config DeviceConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.collectors[config.Host]; exists {
		return fmt.Errorf("device %s already exists", config.Host)
	}

	collector, err := NewCollector(config)
	if err != nil {
		return fmt.Errorf("failed to create collector for %s: %w", config.Host, err)
	}

	if err := collector.Connect(); err != nil {
		return fmt.Errorf("failed to connect to %s: %w", config.Host, err)
	}

	m.collectors[config.Host] = collector
	m.stats[config.Host] = &CollectionStats{}

	return nil
}

// RemoveDevice removes a device from polling.
func (m *Manager) RemoveDevice(host string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	collector, exists := m.collectors[host]
	if !exists {
		return fmt.Errorf("device %s not found", host)
	}

	if err := collector.Close(); err != nil {
		return fmt.Errorf("failed to close connection to %s: %w", host, err)
	}

	delete(m.collectors, host)
	delete(m.stats, host)

	return nil
}

// Start begins the polling loop.
func (m *Manager) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.pollLoop(ctx)
	}()
}

// Stop terminates the polling loop.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()

	// Close all collectors
	m.mu.Lock()
	defer m.mu.Unlock()

	for host, collector := range m.collectors {
		if err := collector.Close(); err != nil {
			if m.OnError != nil {
				m.OnError(host, err)
			}
		}
	}
}

// pollLoop runs the periodic polling.
func (m *Manager) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	// Initial poll
	m.pollAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollAll(ctx)
		}
	}
}

// pollAll polls all devices concurrently.
func (m *Manager) pollAll(ctx context.Context) {
	m.mu.RLock()
	hosts := make([]string, 0, len(m.collectors))
	for host := range m.collectors {
		hosts = append(hosts, host)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			m.pollDevice(ctx, h)
		}(host)
	}
	wg.Wait()
}

// pollDevice polls a single device.
func (m *Manager) pollDevice(ctx context.Context, host string) {
	m.mu.RLock()
	collector, exists := m.collectors[host]
	stats := m.stats[host]
	m.mu.RUnlock()

	if !exists {
		return
	}

	telemetry, err := collector.CollectAll(ctx)
	if err != nil {
		m.mu.Lock()
		stats.TotalErrors++
		m.mu.Unlock()

		if m.OnError != nil {
			m.OnError(host, err)
		}
		return
	}

	// Update stats
	m.mu.Lock()
	stats.LastCollection = telemetry.CollectedAt
	stats.LastDuration = telemetry.Duration
	stats.TotalCollects++
	stats.ONUCount = len(telemetry.ONUs)
	stats.OnlineONUs = 0
	stats.OfflineONUs = 0
	for _, onu := range telemetry.ONUs {
		if onu.Status == "online" {
			stats.OnlineONUs++
		} else {
			stats.OfflineONUs++
		}
	}
	m.mu.Unlock()

	if m.OnTelemetry != nil {
		m.OnTelemetry(host, telemetry)
	}
}

// PollNow triggers an immediate poll of all devices.
func (m *Manager) PollNow(ctx context.Context) {
	m.pollAll(ctx)
}

// PollDevice triggers an immediate poll of a specific device.
func (m *Manager) PollDevice(ctx context.Context, host string) (*OLTTelemetry, error) {
	m.mu.RLock()
	collector, exists := m.collectors[host]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("device %s not found", host)
	}

	return collector.CollectAll(ctx)
}

// GetStats returns collection statistics for a device.
func (m *Manager) GetStats(host string) (*CollectionStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats, exists := m.stats[host]
	if !exists {
		return nil, fmt.Errorf("device %s not found", host)
	}

	// Return a copy
	statsCopy := *stats
	return &statsCopy, nil
}

// GetAllStats returns collection statistics for all devices.
func (m *Manager) GetAllStats() map[string]CollectionStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]CollectionStats, len(m.stats))
	for host, stats := range m.stats {
		result[host] = *stats
	}
	return result
}

// ListDevices returns all configured device hosts.
func (m *Manager) ListDevices() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hosts := make([]string, 0, len(m.collectors))
	for host := range m.collectors {
		hosts = append(hosts, host)
	}
	return hosts
}

// DeviceCount returns the number of configured devices.
func (m *Manager) DeviceCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.collectors)
}

// Summary provides a quick overview of all devices.
type Summary struct {
	TotalDevices  int `json:"total_devices"`
	TotalONUs     int `json:"total_onus"`
	OnlineONUs    int `json:"online_onus"`
	OfflineONUs   int `json:"offline_onus"`
	TotalErrors   int `json:"total_errors"`
}

// GetSummary returns a summary of all devices.
func (m *Manager) GetSummary() Summary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summary := Summary{
		TotalDevices: len(m.collectors),
	}

	for _, stats := range m.stats {
		summary.TotalONUs += stats.ONUCount
		summary.OnlineONUs += stats.OnlineONUs
		summary.OfflineONUs += stats.OfflineONUs
		summary.TotalErrors += int(stats.TotalErrors)
	}

	return summary
}
