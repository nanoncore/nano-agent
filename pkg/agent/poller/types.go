// Package poller provides production-grade OLT polling with worker pool architecture.
package poller

import (
	"time"
)

// OLTConfig represents an OLT configuration from the control plane.
type OLTConfig struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Vendor    string             `json:"vendor"`
	Model     string             `json:"model"`
	Address   string             `json:"address"`
	Protocols OLTProtocols       `json:"protocols"`
	Polling   OLTPollingConfig   `json:"polling"`
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
	Error     error
	Duration  time.Duration
	Timestamp time.Time
}
