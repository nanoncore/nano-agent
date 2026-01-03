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
// Supports both legacy format (snmp/ssh fields) and new multi-protocol format.
type OLTProtocols struct {
	// New multi-protocol format
	Primary string `json:"primary,omitempty"` // Primary protocol: cli, snmp, netconf, gnmi, rest

	// Protocol-specific configurations (new format)
	CLI     *CLIConfig     `json:"cli,omitempty"`
	NETCONF *NETCONFConfig `json:"netconf,omitempty"`
	GNMI    *GNMIConfig    `json:"gnmi,omitempty"`
	REST    *RESTConfig    `json:"rest,omitempty"`

	// Legacy format (still supported)
	SNMP SNMPConfig `json:"snmp"`
	SSH  SSHConfig  `json:"ssh"`
}

// CLIConfig contains CLI/SSH configuration for OLT access.
type CLIConfig struct {
	Enabled           bool   `json:"enabled"`
	Port              int    `json:"port"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
}

// NETCONFConfig contains NETCONF configuration.
type NETCONFConfig struct {
	Enabled           bool   `json:"enabled"`
	Port              int    `json:"port"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
}

// GNMIConfig contains gNMI configuration.
type GNMIConfig struct {
	Enabled           bool   `json:"enabled"`
	Port              int    `json:"port"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
	TLSEnabled        bool   `json:"tlsEnabled,omitempty"`
}

// RESTConfig contains REST API configuration.
type RESTConfig struct {
	Enabled           bool   `json:"enabled"`
	Port              int    `json:"port"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
	TLSEnabled        bool   `json:"tlsEnabled,omitempty"`
	BasePath          string `json:"basePath,omitempty"`
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

// GetPrimaryProtocol returns the primary protocol for this OLT.
func (p *OLTProtocols) GetPrimaryProtocol() string {
	if p.Primary != "" {
		return p.Primary
	}
	// Check new format CLI first
	if p.CLI != nil && p.CLI.Enabled {
		return "cli"
	}
	// Legacy: check SSH
	if p.SSH.Enabled {
		return "cli"
	}
	// Check SNMP
	if p.SNMP.Enabled {
		return "snmp"
	}
	// Check other protocols
	if p.NETCONF != nil && p.NETCONF.Enabled {
		return "netconf"
	}
	if p.GNMI != nil && p.GNMI.Enabled {
		return "gnmi"
	}
	if p.REST != nil && p.REST.Enabled {
		return "rest"
	}
	return "cli"
}

// HasProtocol checks if a specific protocol is enabled.
func (p *OLTProtocols) HasProtocol(protocol string) bool {
	switch protocol {
	case "cli", "ssh":
		if p.CLI != nil && p.CLI.Enabled {
			return true
		}
		return p.SSH.Enabled
	case "snmp":
		return p.SNMP.Enabled
	case "netconf":
		return p.NETCONF != nil && p.NETCONF.Enabled
	case "gnmi":
		return p.GNMI != nil && p.GNMI.Enabled
	case "rest":
		return p.REST != nil && p.REST.Enabled
	default:
		return false
	}
}

// GetEnabledProtocols returns a list of all enabled protocol names.
func (p *OLTProtocols) GetEnabledProtocols() []string {
	var enabled []string
	if p.CLI != nil && p.CLI.Enabled {
		enabled = append(enabled, "cli")
	} else if p.SSH.Enabled {
		enabled = append(enabled, "cli")
	}
	if p.SNMP.Enabled {
		enabled = append(enabled, "snmp")
	}
	if p.NETCONF != nil && p.NETCONF.Enabled {
		enabled = append(enabled, "netconf")
	}
	if p.GNMI != nil && p.GNMI.Enabled {
		enabled = append(enabled, "gnmi")
	}
	if p.REST != nil && p.REST.Enabled {
		enabled = append(enabled, "rest")
	}
	return enabled
}

// ONUData represents ONU data to be pushed to the control plane.
type ONUData struct {
	Serial          string  `json:"serialNumber"`
	PONPort         string  `json:"ponPort"`
	ONUID           int     `json:"onuId,omitempty"`
	Status          string  `json:"status"`
	OperState       string  `json:"operState,omitempty"`
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
