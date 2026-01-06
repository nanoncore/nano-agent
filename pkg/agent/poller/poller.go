package poller

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	southbound "github.com/nanoncore/nano-southbound"
	"github.com/nanoncore/nano-southbound/types"
)

// ONUPusher is the interface for pushing ONUs to the control plane.
type ONUPusher interface {
	PushONUs(oltID string, onus []ONUData) (*PushONUsResponse, error)
}

// Poller manages OLT polling with a worker pool.
type Poller struct {
	mu sync.RWMutex

	// Configuration
	workerCount    int
	checkInterval  time.Duration
	maxBackoff     time.Duration
	connectTimeout time.Duration

	// State
	oltStates map[string]*OLTState
	running   bool

	// Dependencies
	pusher ONUPusher

	// Channels
	jobChan    chan *OLTState
	resultChan chan *PollResult
	stopChan   chan struct{}
	doneChan   chan struct{}

	// Logging
	logPrefix string
}

// Config contains configuration for the poller.
type Config struct {
	// WorkerCount is the number of concurrent polling workers (default: 5)
	WorkerCount int

	// CheckInterval is how often to check for OLTs needing polling (default: 10s)
	CheckInterval time.Duration

	// MaxBackoff is the maximum backoff time after errors (default: 5m)
	MaxBackoff time.Duration

	// ConnectTimeout is the timeout for connecting to OLTs (default: 30s)
	ConnectTimeout time.Duration

	// LogPrefix is prepended to log messages (default: "[poller]")
	LogPrefix string
}

// DefaultConfig returns the default poller configuration.
func DefaultConfig() *Config {
	return &Config{
		WorkerCount:    5,
		CheckInterval:  10 * time.Second,
		MaxBackoff:     5 * time.Minute,
		ConnectTimeout: 30 * time.Second,
		LogPrefix:      "[poller]",
	}
}

// New creates a new OLT poller.
func New(pusher ONUPusher, cfg *Config) *Poller {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 5
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 10 * time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 5 * time.Minute
	}
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 30 * time.Second
	}
	if cfg.LogPrefix == "" {
		cfg.LogPrefix = "[poller]"
	}

	return &Poller{
		workerCount:    cfg.WorkerCount,
		checkInterval:  cfg.CheckInterval,
		maxBackoff:     cfg.MaxBackoff,
		connectTimeout: cfg.ConnectTimeout,
		oltStates:      make(map[string]*OLTState),
		pusher:         pusher,
		logPrefix:      cfg.LogPrefix,
	}
}

// UpdateOLTs updates the list of OLTs to poll.
func (p *Poller) UpdateOLTs(olts []OLTConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Track which OLTs we've seen
	seen := make(map[string]bool)

	for _, olt := range olts {
		seen[olt.ID] = true

		// Skip OLTs with polling disabled
		if !olt.Polling.Enabled {
			delete(p.oltStates, olt.ID)
			continue
		}

		// Update or create state
		if state, exists := p.oltStates[olt.ID]; exists {
			// Update config but preserve state
			state.Config = olt
		} else {
			// New OLT - add to state
			p.oltStates[olt.ID] = &OLTState{
				Config: olt,
			}
		}
	}

	// Remove OLTs that are no longer in the config
	for id := range p.oltStates {
		if !seen[id] {
			delete(p.oltStates, id)
		}
	}

	p.log("Updated OLT list: %d OLTs configured for polling", len(p.oltStates))
}

// Start begins the polling loop.
func (p *Poller) Start(ctx context.Context) {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true

	// Initialize channels
	p.jobChan = make(chan *OLTState, p.workerCount*2)
	p.resultChan = make(chan *PollResult, p.workerCount*2)
	p.stopChan = make(chan struct{})
	p.doneChan = make(chan struct{})
	p.mu.Unlock()

	p.log("Starting with %d workers, check interval %s", p.workerCount, p.checkInterval)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < p.workerCount; i++ {
		wg.Add(1)
		go p.worker(ctx, i, &wg)
	}

	// Start result processor
	go p.processResults(ctx)

	// Start scheduler
	go p.scheduler(ctx)

	// Wait for stop signal
	go func() {
		<-p.stopChan
		close(p.jobChan)
		wg.Wait()
		close(p.resultChan)
		close(p.doneChan)
	}()
}

// Stop halts the polling loop.
func (p *Poller) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	p.mu.Unlock()

	p.log("Stopping...")
	close(p.stopChan)
	<-p.doneChan
	p.log("Stopped")
}

// scheduler periodically checks which OLTs need polling and queues them.
func (p *Poller) scheduler(ctx context.Context) {
	ticker := time.NewTicker(p.checkInterval)
	defer ticker.Stop()

	// Initial poll - stagger start times
	p.scheduleInitialPolls()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.schedulePolls()
		}
	}
}

// scheduleInitialPolls staggers the initial polls to avoid thundering herd.
func (p *Poller) scheduleInitialPolls() {
	p.mu.RLock()
	olts := make([]*OLTState, 0, len(p.oltStates))
	for _, state := range p.oltStates {
		olts = append(olts, state)
	}
	p.mu.RUnlock()

	if len(olts) == 0 {
		return
	}

	// Calculate stagger interval
	// For 30 OLTs with 5min polling, stagger = 5min / 30 = 10s between each
	minInterval := 300 * time.Second // Default 5 min polling
	for _, state := range olts {
		if state.Config.Polling.Interval > 0 {
			interval := time.Duration(state.Config.Polling.Interval) * time.Second
			if interval < minInterval {
				minInterval = interval
			}
		}
	}
	stagger := minInterval / time.Duration(len(olts))
	if stagger < time.Second {
		stagger = time.Second
	}
	if stagger > 30*time.Second {
		stagger = 30 * time.Second
	}

	p.log("Scheduling initial polls with %s stagger for %d OLTs", stagger, len(olts))

	// Schedule with stagger
	go func() {
		for i, state := range olts {
			select {
			case <-p.stopChan:
				return
			case p.jobChan <- state:
				p.log("Queued initial poll for %s (%d/%d)", state.Config.Name, i+1, len(olts))
			default:
				p.log("Job queue full, skipping initial poll for %s", state.Config.Name)
			}
			time.Sleep(stagger)
		}
	}()
}

// schedulePolls checks which OLTs need polling and queues them.
func (p *Poller) schedulePolls() {
	p.mu.RLock()
	now := time.Now()
	var toSchedule []*OLTState

	for _, state := range p.oltStates {
		// Skip if in backoff
		if now.Before(state.BackoffUntil) {
			continue
		}

		// Check if poll interval has elapsed
		interval := time.Duration(state.Config.Polling.Interval) * time.Second
		if interval <= 0 {
			interval = 5 * time.Minute // Default
		}

		if now.Sub(state.LastPoll) >= interval {
			toSchedule = append(toSchedule, state)
		}
	}
	p.mu.RUnlock()

	// Queue jobs
	for _, state := range toSchedule {
		select {
		case p.jobChan <- state:
			// Queued successfully
		default:
			// Queue full, skip this cycle
			p.log("Job queue full, skipping poll for %s", state.Config.Name)
		}
	}
}

// worker processes polling jobs from the job channel.
func (p *Poller) worker(ctx context.Context, id int, wg *sync.WaitGroup) {
	defer wg.Done()

	for state := range p.jobChan {
		select {
		case <-ctx.Done():
			return
		default:
			result := p.pollOLT(ctx, state)
			select {
			case p.resultChan <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}

// pollOLT polls a single OLT and returns the result.
func (p *Poller) pollOLT(ctx context.Context, state *OLTState) *PollResult {
	start := time.Now()
	result := &PollResult{
		OLTID:     state.Config.ID,
		Timestamp: start,
	}

	// Update last poll time
	p.mu.Lock()
	state.LastPoll = start
	p.mu.Unlock()

	// Create driver
	vendor := types.Vendor(strings.ToLower(state.Config.Vendor))
	protocol := p.determineProtocol(state.Config)

	// Get protocol-specific configuration
	port, username, password := p.getProtocolCredentials(state.Config, protocol)

	config := &types.EquipmentConfig{
		Name:          state.Config.ID,
		Vendor:        vendor,
		Address:       state.Config.Address,
		Port:          port,
		Protocol:      protocol,
		Username:      username,
		Password:      password,
		TLSEnabled:    false,
		TLSSkipVerify: true,
		Timeout:       p.connectTimeout,
		Metadata:      make(map[string]string),
	}

	// Add SNMP-specific config to metadata
	if protocol == types.ProtocolSNMP {
		config.Metadata["snmp_community"] = state.Config.Protocols.SNMP.Community
		config.Metadata["snmp_version"] = state.Config.Protocols.SNMP.Version
	}

	driver, err := southbound.NewDriver(vendor, protocol, config)
	if err != nil {
		result.Error = fmt.Errorf("failed to create driver: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Connect with timeout
	connectCtx, cancel := context.WithTimeout(ctx, p.connectTimeout)
	defer cancel()

	if err := driver.Connect(connectCtx, config); err != nil {
		result.Error = fmt.Errorf("failed to connect: %w", err)
		result.Duration = time.Since(start)
		return result
	}
	defer driver.Disconnect(ctx)

	// Check if driver supports DriverV2
	driverV2, ok := driver.(types.DriverV2)
	if !ok {
		result.Error = fmt.Errorf("driver for vendor %s does not support ONU listing", state.Config.Vendor)
		result.Duration = time.Since(start)
		return result
	}

	// Get provisioned ONU list
	onus, err := driverV2.GetONUList(ctx, nil)
	if err != nil {
		result.Error = fmt.Errorf("failed to get ONU list: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Convert provisioned ONUs to ONUData
	result.ONUs = make([]ONUData, 0, len(onus))
	seenSerials := make(map[string]bool)

	for _, onu := range onus {
		status := "offline"
		offlineReason := ""
		if onu.IsOnline {
			status = "online"
		} else {
			// Determine offline reason from OperState
			switch onu.OperState {
			case "los", "LOS":
				status = "los"
				offlineReason = "los" // Loss of Signal
			case "dying_gasp", "DyingGasp":
				status = "offline"
				offlineReason = "dying_gasp"
			case "low_power", "LowPower":
				status = "offline"
				offlineReason = "low_power"
			case "disabled":
				status = "offline"
				offlineReason = "admin_disabled"
			default:
				if onu.OperState != "" {
					offlineReason = onu.OperState
				}
			}
		}

		seenSerials[onu.Serial] = true
		result.ONUs = append(result.ONUs, ONUData{
			Serial:        onu.Serial,
			PONPort:       onu.PONPort,
			ONUID:         onu.ONUID,
			Status:        status,
			OperState:     onu.OperState,
			OfflineReason: offlineReason,
			Distance:      onu.DistanceM,
			RxPower:       onu.RxPowerDBm,
			TxPower:       onu.TxPowerDBm,
			Model:         onu.Model,
		})
	}

	// Also get discovered (unprovisioned) ONUs
	discoveries, err := driverV2.DiscoverONUs(ctx, nil)
	if err != nil {
		// Log but don't fail - discovery is optional
		fmt.Printf("[olt-poller] Warning: failed to discover ONUs for %s: %v\n", state.Config.Name, err)
	} else {
		for _, disc := range discoveries {
			// Skip if we already have this ONU in the provisioned list
			if seenSerials[disc.Serial] {
				continue
			}
			result.ONUs = append(result.ONUs, ONUData{
				Serial:        disc.Serial,
				PONPort:       disc.PONPort,
				Status:        "discovered",
				OperState:     "discovered",
				OfflineReason: "unauthorized", // Not yet provisioned
				Distance:      disc.DistanceM,
				RxPower:       disc.RxPowerDBm,
				Model:         disc.Model,
			})
		}
	}

	result.Duration = time.Since(start)
	return result
}

// determineProtocol determines the best protocol to use for an OLT.
func (p *Poller) determineProtocol(cfg OLTConfig) types.Protocol {
	// Use Primary field if set
	if cfg.Protocols.Primary != "" {
		switch cfg.Protocols.Primary {
		case "cli", "ssh":
			return types.ProtocolCLI
		case "snmp":
			return types.ProtocolSNMP
		case "netconf":
			return types.ProtocolNETCONF
		case "gnmi":
			return types.ProtocolGNMI
		case "rest":
			return types.ProtocolREST
		}
	}

	// Check new multi-protocol format
	if cfg.Protocols.CLI != nil && cfg.Protocols.CLI.Enabled {
		return types.ProtocolCLI
	}
	if cfg.Protocols.SNMP.Enabled {
		return types.ProtocolSNMP
	}

	// Legacy: check SSH
	if cfg.Protocols.SSH.Enabled {
		return types.ProtocolCLI
	}

	// Default to CLI
	return types.ProtocolCLI
}

// getProtocolCredentials returns the port, username, and password for the given protocol.
func (p *Poller) getProtocolCredentials(cfg OLTConfig, protocol types.Protocol) (port int, username, password string) {
	switch protocol {
	case types.ProtocolSNMP:
		port = cfg.Protocols.SNMP.Port
		if port == 0 {
			port = 161
		}
		// SNMP uses community string, not username/password
		return port, "", ""

	case types.ProtocolCLI:
		// Check new CLI config first
		if cfg.Protocols.CLI != nil && cfg.Protocols.CLI.Enabled {
			port = cfg.Protocols.CLI.Port
			username = cfg.Protocols.CLI.Username
			password = cfg.Protocols.CLI.Password
		} else {
			// Fallback to legacy SSH config
			port = cfg.Protocols.SSH.Port
			username = cfg.Protocols.SSH.Username
			password = cfg.Protocols.SSH.Password
		}
		if port == 0 {
			port = 22
		}
		return port, username, password

	case types.ProtocolNETCONF:
		if cfg.Protocols.NETCONF != nil {
			port = cfg.Protocols.NETCONF.Port
			username = cfg.Protocols.NETCONF.Username
			password = cfg.Protocols.NETCONF.Password
		}
		if port == 0 {
			port = 830
		}
		return port, username, password

	case types.ProtocolGNMI:
		if cfg.Protocols.GNMI != nil {
			port = cfg.Protocols.GNMI.Port
			username = cfg.Protocols.GNMI.Username
			password = cfg.Protocols.GNMI.Password
		}
		if port == 0 {
			port = 6030
		}
		return port, username, password

	case types.ProtocolREST:
		if cfg.Protocols.REST != nil {
			port = cfg.Protocols.REST.Port
			username = cfg.Protocols.REST.Username
			password = cfg.Protocols.REST.Password
		}
		if port == 0 {
			port = 443
		}
		return port, username, password

	default:
		// Default to SSH/CLI
		port = cfg.Protocols.SSH.Port
		if port == 0 {
			port = 22
		}
		return port, cfg.Protocols.SSH.Username, cfg.Protocols.SSH.Password
	}
}

// processResults handles poll results from workers.
func (p *Poller) processResults(ctx context.Context) {
	for result := range p.resultChan {
		p.handleResult(result)
	}
}

// handleResult processes a single poll result.
func (p *Poller) handleResult(result *PollResult) {
	p.mu.Lock()
	state, exists := p.oltStates[result.OLTID]
	if !exists {
		p.mu.Unlock()
		return
	}

	if result.Error != nil {
		// Handle error with exponential backoff
		state.LastError = result.Error
		state.ErrorCount++

		// Calculate backoff: min(2^errorCount * 10s, maxBackoff)
		backoff := time.Duration(1<<uint(state.ErrorCount)) * 10 * time.Second
		if backoff > p.maxBackoff {
			backoff = p.maxBackoff
		}
		state.BackoffUntil = time.Now().Add(backoff)

		p.mu.Unlock()
		p.log("Poll failed for %s (attempt %d, backoff %s): %v",
			result.OLTID, state.ErrorCount, backoff, result.Error)
		return
	}

	// Success - reset error state
	state.LastSuccess = result.Timestamp
	state.LastError = nil
	state.ErrorCount = 0
	state.BackoffUntil = time.Time{}
	oltName := state.Config.Name
	p.mu.Unlock()

	p.log("Poll succeeded for %s: %d ONUs in %s", oltName, len(result.ONUs), result.Duration)

	// Push ONUs to control plane
	if p.pusher != nil && len(result.ONUs) > 0 {
		resp, err := p.pusher.PushONUs(result.OLTID, result.ONUs)
		if err != nil {
			p.log("Failed to push ONUs for %s: %v", oltName, err)
		} else if resp != nil {
			p.log("Pushed %d ONUs for %s (created: %d, updated: %d, unchanged: %d)",
				len(result.ONUs), oltName, resp.Created, resp.Updated, resp.Unchanged)
		}
	}
}

// GetStats returns current polling statistics.
func (p *Poller) GetStats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := map[string]interface{}{
		"running":      p.running,
		"worker_count": p.workerCount,
		"olt_count":    len(p.oltStates),
	}

	oltStats := make([]map[string]interface{}, 0, len(p.oltStates))
	for id, state := range p.oltStates {
		oltStat := map[string]interface{}{
			"id":           id,
			"name":         state.Config.Name,
			"last_poll":    state.LastPoll,
			"last_success": state.LastSuccess,
			"error_count":  state.ErrorCount,
		}
		if state.LastError != nil {
			oltStat["last_error"] = state.LastError.Error()
		}
		oltStats = append(oltStats, oltStat)
	}
	stats["olts"] = oltStats

	return stats
}

// log outputs a log message with the poller prefix.
func (p *Poller) log(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("%s %s %s", time.Now().Format("15:04:05"), p.logPrefix, msg)
}
