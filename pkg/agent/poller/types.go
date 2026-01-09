// Package poller provides OLT polling functionality for the nano-agent.
package poller

import (
	"time"
)

// OLTConfig represents an OLT configuration from the control plane.
type OLTConfig struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Vendor    string            `json:"vendor"`
	Model     string            `json:"model"`
	Address   string            `json:"address"`
	Protocols OLTProtocols      `json:"protocols"`
	Polling   OLTPollingConfig  `json:"polling"`
	Discovery OLTDiscoveryConfig `json:"discovery"`
}

// OLTProtocols contains protocol configurations for OLT access.
type OLTProtocols struct {
	SNMP SNMPConfig `json:"snmp"`
	SSH  SSHConfig  `json:"ssh"`
}

// SNMPConfig contains SNMP configuration.
type SNMPConfig struct {
	Enabled   bool   `json:"enabled"`
	Port      int    `json:"port"`
	Community string `json:"community"`
	Version   string `json:"version"`
}

// SSHConfig contains SSH configuration.
type SSHConfig struct {
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// OLTPollingConfig contains polling configuration.
type OLTPollingConfig struct {
	Enabled  bool     `json:"enabled"`
	Interval int      `json:"interval"` // seconds
	Metrics  []string `json:"metrics"`
}

// OLTDiscoveryConfig contains discovery configuration.
type OLTDiscoveryConfig struct {
	Enabled  bool     `json:"enabled"`
	Interval int      `json:"interval"` // seconds
	Protocol string   `json:"protocol"`
	PONPorts []string `json:"ponPorts"`
}

// AgentConfig represents the full configuration response from the control plane.
type AgentConfig struct {
	NodeID  string      `json:"nodeId"`
	Version int         `json:"version"`
	OLTs    []OLTConfig `json:"olts"`
}

// ONUData represents ONU data to be pushed to the control plane.
type ONUData struct {
	Serial          string  `json:"serialNumber"`
	PONPort         string  `json:"ponPort"`
	ONUID           int     `json:"onuId,omitempty"`
	Status          string  `json:"status"`
	Distance        int     `json:"distance,omitempty"`
	RxPower         float64 `json:"rxPower,omitempty"`
	TxPower         float64 `json:"txPower,omitempty"`
	Model           string  `json:"model,omitempty"`
	SoftwareVersion string  `json:"softwareVersion,omitempty"`

	// Thermal & Power (from detailed poll)
	Temperature float64 `json:"temperature,omitempty"` // °C
	Voltage     float64 `json:"voltage,omitempty"`     // V
	BiasCurrent float64 `json:"biasCurrent,omitempty"` // mA

	// Traffic Stats (from detailed poll)
	BytesUp       uint64 `json:"bytesUp,omitempty"`
	BytesDown     uint64 `json:"bytesDown,omitempty"`
	PacketsUp     uint64 `json:"packetsUp,omitempty"`
	PacketsDown   uint64 `json:"packetsDown,omitempty"`
	InputRateBps  uint64 `json:"inputRateBps,omitempty"`
	OutputRateBps uint64 `json:"outputRateBps,omitempty"`

	// Additional
	Vendor string `json:"vendor,omitempty"` // ONU vendor (detected from serial)
}

// PushONUsRequest is the request body for pushing ONUs to the control plane.
type PushONUsRequest struct {
	ONUs []ONUData `json:"onus"`
}

// PushONUsResponse is the response from the control plane.
type PushONUsResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message,omitempty"`
	Created     int    `json:"created"`
	Updated     int    `json:"updated"`
	Unchanged   int    `json:"unchanged"`
	OnlineCount int    `json:"onlineCount"`
}

// TelemetryData represents OLT telemetry to be pushed to the control plane.
type TelemetryData struct {
	CPUPercent    float64 `json:"cpuPercent,omitempty"`
	MemoryPercent float64 `json:"memoryPercent,omitempty"`
	Temperature   float64 `json:"temperature,omitempty"`
	Uptime        int64   `json:"uptime,omitempty"`
	IsReachable   bool    `json:"isReachable"`
	IsHealthy     bool    `json:"isHealthy"`
	Firmware      string  `json:"firmware,omitempty"`
	SerialNumber  string  `json:"serialNumber,omitempty"`
}

// PushTelemetryResponse is the response from pushing telemetry.
type PushTelemetryResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// MetricSample represents a single metric data point for time-series storage.
type MetricSample struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"` // Unix milliseconds
	Labels    map[string]string `json:"labels"`
}

// MetricsBatch is a batch of metrics to push.
type MetricsBatch struct {
	Metrics []MetricSample `json:"metrics"`
}

// PushMetricsResponse is the response from pushing metrics.
type PushMetricsResponse struct {
	Success bool   `json:"success"`
	Count   int    `json:"count"`
	Message string `json:"message,omitempty"`
}

// OLTState tracks the state of an OLT for polling purposes.
type OLTState struct {
	Config       OLTConfig
	LastPoll     time.Time
	LastSuccess  time.Time
	LastError    error
	ErrorCount   int
	BackoffUntil time.Time
}

// PollResult contains the result of polling an OLT.
type PollResult struct {
	OLTID     string
	ONUs      []ONUData
	Telemetry *TelemetryData
	Error     error
	Duration  time.Duration
	Timestamp time.Time
}
