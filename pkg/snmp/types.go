// Package snmp provides SNMP-based telemetry collection for OLT/ONU devices.
// Supports V-SOL, Huawei, ZTE, and FiberHome equipment.
package snmp

import (
	"context"
	"net"
	"time"
)

// Vendor represents supported OLT vendors.
type Vendor string

const (
	VendorVSOL      Vendor = "vsol"
	VendorHuawei    Vendor = "huawei"
	VendorZTE       Vendor = "zte"
	VendorFiberHome Vendor = "fiberhome"
)

// SNMPVersion represents SNMP protocol version.
type SNMPVersion string

const (
	SNMPv2c SNMPVersion = "2c"
	SNMPv3  SNMPVersion = "v3"
)

// DeviceConfig holds SNMP device connection configuration.
type DeviceConfig struct {
	Host      string            `json:"host"`
	Port      uint16            `json:"port"`
	Vendor    Vendor            `json:"vendor"`
	Community string            `json:"community,omitempty"`
	Version   SNMPVersion       `json:"version"`
	Timeout   time.Duration     `json:"timeout"`
	Retries   int               `json:"retries"`
	Labels    map[string]string `json:"labels,omitempty"`

	// SNMPv3 fields
	Username     string `json:"username,omitempty"`
	AuthProtocol string `json:"auth_protocol,omitempty"` // MD5, SHA
	AuthPassword string `json:"auth_password,omitempty"`
	PrivProtocol string `json:"priv_protocol,omitempty"` // DES, AES
	PrivPassword string `json:"priv_password,omitempty"`

	// Custom offline reason code overrides (merges with vendor defaults)
	OfflineReasons map[int]string `json:"offline_reasons,omitempty"`
}

// Collector defines the interface for vendor-specific SNMP collectors.
type Collector interface {
	// Vendor returns the vendor type for this collector.
	Vendor() Vendor

	// Connect establishes the SNMP connection.
	Connect() error

	// Close terminates the SNMP connection.
	Close() error

	// CollectOLTInfo gathers OLT system information.
	CollectOLTInfo(ctx context.Context) (*OLTInfo, error)

	// CollectPONPorts gathers PON port information including optical power.
	CollectPONPorts(ctx context.Context) ([]PONPort, error)

	// CollectONUs gathers authorized ONU information.
	CollectONUs(ctx context.Context) ([]ONUInfo, error)

	// CollectUnauthONUs gathers unauthorized/discovered ONUs.
	CollectUnauthONUs(ctx context.Context) ([]UnauthONU, error)

	// CollectONUOptical gathers ONU optical power readings.
	CollectONUOptical(ctx context.Context) ([]ONUOptical, error)

	// CollectAll gathers complete telemetry in a single call.
	CollectAll(ctx context.Context) (*OLTTelemetry, error)
}

// OLTInfo represents OLT system-level information.
type OLTInfo struct {
	Host            string    `json:"host"`
	Vendor          Vendor    `json:"vendor"`
	IP              net.IP    `json:"ip,omitempty"`
	MAC             string    `json:"mac,omitempty"`
	SoftwareVersion string    `json:"software_version,omitempty"`
	HardwareVersion string    `json:"hardware_version,omitempty"`
	Model           string    `json:"model,omitempty"`
	Uptime          uint64    `json:"uptime,omitempty"`
	CPUPercent      float64   `json:"cpu_percent"`
	MemoryPercent   float64   `json:"memory_percent"`
	Temperature     float64   `json:"temperature"`
	CollectedAt     time.Time `json:"collected_at"`
}

// PONPort represents a PON port on the OLT.
type PONPort struct {
	Index       int     `json:"index"`
	SlotID      int     `json:"slot_id"`
	PortID      int     `json:"port_id"`
	Name        string  `json:"name,omitempty"`
	Enabled     bool    `json:"enabled"`
	Status      string  `json:"status"`             // up, down, testing
	PONType     string  `json:"pon_type,omitempty"` // GPON, EPON, XG-PON
	ONUCount    int     `json:"onu_count"`
	TxPowerDBm  float64 `json:"tx_power_dbm"`
	RxPowerDBm  float64 `json:"rx_power_dbm"`
	Description string  `json:"description,omitempty"`
}

// ONUInfo represents an authorized ONU.
type ONUInfo struct {
	PonIndex        int       `json:"pon_index"`
	OnuIndex        int       `json:"onu_index"`
	OnuID           string    `json:"onu_id"` // Formatted: slot/port/onu
	SerialNumber    string    `json:"serial_number"`
	MAC             string    `json:"mac,omitempty"`
	Type            string    `json:"type,omitempty"`
	Model           string    `json:"model,omitempty"`
	Description     string    `json:"description,omitempty"`
	Status          string    `json:"status"`               // online, offline
	OperState       string    `json:"oper_state,omitempty"` // operational state (los, dying_gasp, etc.)
	Distance        int       `json:"distance"`             // meters
	RxPower         float64   `json:"rx_power,omitempty"`   // dBm
	TxPower         float64   `json:"tx_power,omitempty"`   // dBm
	SoftwareVersion string    `json:"software_version,omitempty"`
	HardwareVersion string    `json:"hardware_version,omitempty"`
	AuthMode        string    `json:"auth_mode,omitempty"` // sn, mac, loid, hybrid
	LastOnline      time.Time `json:"last_online,omitempty"`
	LastOffline     time.Time `json:"last_offline,omitempty"`
	OfflineReason   string    `json:"offline_reason,omitempty"`
}

// UnauthONU represents an unauthorized/discovered ONU.
type UnauthONU struct {
	PonIndex     int       `json:"pon_index"`
	OnuIndex     int       `json:"onu_index"`
	SerialNumber string    `json:"serial_number"`
	MAC          string    `json:"mac,omitempty"`
	Type         string    `json:"type,omitempty"`
	FirstSeen    time.Time `json:"first_seen,omitempty"`
}

// ONUOptical represents ONU optical power readings.
type ONUOptical struct {
	PonIndex    int     `json:"pon_index"`
	OnuIndex    int     `json:"onu_index"`
	OnuID       string  `json:"onu_id"`
	RxPowerDBm  float64 `json:"rx_power_dbm"`
	TxPowerDBm  float64 `json:"tx_power_dbm"`
	OltRxDBm    float64 `json:"olt_rx_dbm"` // OLT's received power from this ONU
	Temperature float64 `json:"temperature,omitempty"`
	Voltage     float64 `json:"voltage,omitempty"`
	BiasCurrent float64 `json:"bias_current,omitempty"`
	Status      string  `json:"status"` // normal, low, high, critical
}

// ONUTraffic represents ONU traffic statistics.
type ONUTraffic struct {
	PonIndex  int    `json:"pon_index"`
	OnuIndex  int    `json:"onu_index"`
	OnuID     string `json:"onu_id"`
	RxBytes   uint64 `json:"rx_bytes"`
	TxBytes   uint64 `json:"tx_bytes"`
	RxPackets uint64 `json:"rx_packets"`
	TxPackets uint64 `json:"tx_packets"`
	RxDropped uint64 `json:"rx_dropped,omitempty"`
	TxDropped uint64 `json:"tx_dropped,omitempty"`
	RxErrors  uint64 `json:"rx_errors,omitempty"`
	TxErrors  uint64 `json:"tx_errors,omitempty"`
}

// CardInfo represents a line card in the OLT.
type CardInfo struct {
	SlotIndex       int     `json:"slot_index"`
	CardType        string  `json:"card_type"`
	Status          string  `json:"status"` // normal, fault, offline
	SoftwareVersion string  `json:"software_version,omitempty"`
	HardwareVersion string  `json:"hardware_version,omitempty"`
	SerialNumber    string  `json:"serial_number,omitempty"`
	CPUPercent      float64 `json:"cpu_percent,omitempty"`
	MemoryPercent   float64 `json:"memory_percent,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

// Alarm represents an SNMP trap/alarm.
type Alarm struct {
	Type        string    `json:"type"`
	Severity    string    `json:"severity"` // critical, major, minor, warning, info
	Source      string    `json:"source"`   // Device/port/onu identifier
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Cleared     bool      `json:"cleared"`
}

// OLTTelemetry aggregates all telemetry from an OLT.
type OLTTelemetry struct {
	OLTInfo     OLTInfo       `json:"olt_info"`
	Cards       []CardInfo    `json:"cards,omitempty"`
	PONPorts    []PONPort     `json:"pon_ports"`
	ONUs        []ONUInfo     `json:"onus"`
	UnauthONUs  []UnauthONU   `json:"unauth_onus,omitempty"`
	ONUOptical  []ONUOptical  `json:"onu_optical"`
	ONUTraffic  []ONUTraffic  `json:"onu_traffic,omitempty"`
	Alarms      []Alarm       `json:"alarms,omitempty"`
	CollectedAt time.Time     `json:"collected_at"`
	Duration    time.Duration `json:"duration"`
	Errors      []string      `json:"errors,omitempty"`
}

// CollectionStats tracks collection statistics.
type CollectionStats struct {
	LastCollection time.Time     `json:"last_collection"`
	LastDuration   time.Duration `json:"last_duration"`
	TotalCollects  uint64        `json:"total_collects"`
	TotalErrors    uint64        `json:"total_errors"`
	ONUCount       int           `json:"onu_count"`
	OnlineONUs     int           `json:"online_onus"`
	OfflineONUs    int           `json:"offline_onus"`
}

// OpticalThresholds defines optical power thresholds for alerting.
type OpticalThresholds struct {
	RxLowWarning   float64 `json:"rx_low_warning"`   // dBm
	RxLowCritical  float64 `json:"rx_low_critical"`  // dBm
	RxHighWarning  float64 `json:"rx_high_warning"`  // dBm
	RxHighCritical float64 `json:"rx_high_critical"` // dBm
}

// DefaultOpticalThresholds returns standard optical power thresholds.
func DefaultOpticalThresholds() OpticalThresholds {
	return OpticalThresholds{
		RxLowWarning:   -27.0,
		RxLowCritical:  -30.0,
		RxHighWarning:  -8.0,
		RxHighCritical: -5.0,
	}
}

// EvaluateOpticalStatus determines optical power status based on thresholds.
func EvaluateOpticalStatus(powerDBm float64, thresholds OpticalThresholds) string {
	switch {
	case powerDBm <= thresholds.RxLowCritical:
		return "critical_low"
	case powerDBm <= thresholds.RxLowWarning:
		return "low"
	case powerDBm >= thresholds.RxHighCritical:
		return "critical_high"
	case powerDBm >= thresholds.RxHighWarning:
		return "high"
	default:
		return "normal"
	}
}
