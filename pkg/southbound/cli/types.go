package cli

import "time"

// CLIConfig holds SSH connection configuration.
type CLIConfig struct {
	Host           string        `json:"host"`
	Port           int           `json:"port"`
	Username       string        `json:"username"`
	Password       string        `json:"password"`
	PrivateKeyPath string        `json:"private_key_path,omitempty"`
	Timeout        time.Duration `json:"timeout"`
	Vendor         string        `json:"vendor"`
}

// ONUProvisionRequest contains parameters for adding an ONU.
type ONUProvisionRequest struct {
	PonPort        string            `json:"pon_port"`       // e.g., "0/1" or "gpon-olt_0/1/1"
	OnuID          int               `json:"onu_id"`         // ONU/ONT ID on the PON port
	SerialNumber   string            `json:"serial_number"`  // ONU serial number
	Type           string            `json:"type,omitempty"` // ONU type/model
	Description    string            `json:"description,omitempty"`
	ServicePorts   []ServicePortSpec `json:"service_ports,omitempty"`
	NativeVLAN     int               `json:"native_vlan,omitempty"`
	AllowedVLANs   []int             `json:"allowed_vlans,omitempty"`
	LineProfile    string            `json:"line_profile,omitempty"`    // Huawei-specific
	ServiceProfile string            `json:"service_profile,omitempty"` // Huawei-specific
}

// ServicePortSpec defines a service port configuration.
type ServicePortSpec struct {
	Index    int    `json:"index"`
	VLAN     int    `json:"vlan"`
	GemPort  int    `json:"gem_port,omitempty"`
	UserVLAN int    `json:"user_vlan,omitempty"`
	Mode     string `json:"mode,omitempty"` // tag, translate, transparent
}

// ONUCLIInfo contains detailed ONU information from CLI.
type ONUCLIInfo struct {
	PonPort        string    `json:"pon_port"`
	OnuID          int       `json:"onu_id"`
	SerialNumber   string    `json:"serial_number"`
	MAC            string    `json:"mac,omitempty"`
	Status         string    `json:"status"`
	Type           string    `json:"type,omitempty"`
	Description    string    `json:"description,omitempty"`
	LineProfile    string    `json:"line_profile,omitempty"`
	ServiceProfile string    `json:"service_profile,omitempty"`
	Distance       int       `json:"distance,omitempty"`
	RxPower        float64   `json:"rx_power,omitempty"`
	LastOnline     time.Time `json:"last_online,omitempty"`
	LastOffline    time.Time `json:"last_offline,omitempty"`
	OfflineReason  string    `json:"offline_reason,omitempty"`
	// Optical diagnostics (populated when available)
	TxPower     float64 `json:"tx_power,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	Voltage     float64 `json:"voltage,omitempty"`
	BiasCurrent float64 `json:"bias_current,omitempty"`
	// Traffic counters (populated when available)
	RxBytes uint64 `json:"rx_bytes,omitempty"`
	TxBytes uint64 `json:"tx_bytes,omitempty"`
}

// CommandResult represents the result of a CLI command execution.
type CommandResult struct {
	Command  string        `json:"command"`
	Output   string        `json:"output"`
	Duration time.Duration `json:"duration"`
	Error    error         `json:"error,omitempty"`
}

// =============================================================================
// VLAN Management Types
// =============================================================================

// VLANConfig represents VLAN configuration for an ONU.
type VLANConfig struct {
	OnuID        int               `json:"onu_id"`
	PonPort      string            `json:"pon_port"`
	NativeVLAN   int               `json:"native_vlan,omitempty"`
	TaggedVLANs  []int             `json:"tagged_vlans,omitempty"`
	ServiceVLAN  int               `json:"service_vlan,omitempty"`
	Translations []VLANTranslation `json:"translations,omitempty"`
}

// VLANTranslation represents a VLAN translation rule.
type VLANTranslation struct {
	CustomerVLAN int    `json:"customer_vlan"` // C-VLAN (user side)
	ServiceVLAN  int    `json:"service_vlan"`  // S-VLAN (network side)
	Mode         string `json:"mode"`          // translate, transparent, tag
}

// VLANInfo represents VLAN information from the device.
type VLANInfo struct {
	ID          int      `json:"id"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Tagged      []string `json:"tagged_ports,omitempty"`
	Untagged    []string `json:"untagged_ports,omitempty"`
}

// =============================================================================
// Profile Management Types
// =============================================================================

// LineProfile represents a line profile configuration (Huawei).
type LineProfile struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`                   // gpon, epon
	MappingMode string          `json:"mapping_mode,omitempty"` // vlan, port
	GemPorts    []GemPortConfig `json:"gem_ports,omitempty"`
}

// GemPortConfig represents a GEM port configuration.
type GemPortConfig struct {
	ID        int    `json:"id"`
	Name      string `json:"name,omitempty"`
	TCont     int    `json:"tcont,omitempty"`
	Direction string `json:"direction,omitempty"` // upstream, downstream, bidirectional
}

// ServiceProfile represents a service profile configuration (Huawei).
type ServiceProfile struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ONUType   string `json:"onu_type,omitempty"`
	PortCount int    `json:"port_count,omitempty"`
	ETHPorts  int    `json:"eth_ports,omitempty"`
	POTSPorts int    `json:"pots_ports,omitempty"`
}

// TrafficProfile represents a traffic/bandwidth profile.
type TrafficProfile struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`          // cir, pir, assured, max
	CIR              int    `json:"cir,omitempty"` // Committed Information Rate (kbps)
	PIR              int    `json:"pir,omitempty"` // Peak Information Rate (kbps)
	CBS              int    `json:"cbs,omitempty"` // Committed Burst Size (bytes)
	PBS              int    `json:"pbs,omitempty"` // Peak Burst Size (bytes)
	FixedBandwidth   int    `json:"fixed_bandwidth,omitempty"`
	AssuredBandwidth int    `json:"assured_bandwidth,omitempty"`
	MaxBandwidth     int    `json:"max_bandwidth,omitempty"`
}

// DBAProfile represents a Dynamic Bandwidth Allocation profile.
type DBAProfile struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"` // type1, type2, type3, type4, type5
	FixedBandwidth   int    `json:"fixed_bandwidth,omitempty"`
	AssuredBandwidth int    `json:"assured_bandwidth,omitempty"`
	MaxBandwidth     int    `json:"max_bandwidth,omitempty"`
}

// =============================================================================
// Port Control Types
// =============================================================================

// PONPortInfo represents PON port information.
type PONPortInfo struct {
	Slot        int     `json:"slot"`
	Port        int     `json:"port"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`       // up, down, admin-down
	AdminStatus string  `json:"admin_status"` // enable, disable
	Type        string  `json:"type"`         // gpon, epon, xgpon
	ONUCount    int     `json:"onu_count"`
	MaxONUs     int     `json:"max_onus,omitempty"`
	TxPower     float64 `json:"tx_power,omitempty"`
	RxPower     float64 `json:"rx_power,omitempty"`
	Description string  `json:"description,omitempty"`
	Uptime      string  `json:"uptime,omitempty"`
}

// PortConfig represents port configuration parameters.
type PortConfig struct {
	Slot        int    `json:"slot"`
	Port        int    `json:"port"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
	AutoNeg     bool   `json:"auto_negotiation,omitempty"`
	Speed       string `json:"speed,omitempty"`  // auto, 100, 1000, 10000
	Duplex      string `json:"duplex,omitempty"` // auto, full, half
}

// =============================================================================
// Batch Operations Types
// =============================================================================

// BatchProvisionRequest represents a batch ONU provisioning request.
type BatchProvisionRequest struct {
	ONUs []ONUProvisionRequest `json:"onus"`
	// Common settings applied to all ONUs if not specified per-ONU
	DefaultLineProfile    string `json:"default_line_profile,omitempty"`
	DefaultServiceProfile string `json:"default_service_profile,omitempty"`
	DefaultVLAN           int    `json:"default_vlan,omitempty"`
	StopOnError           bool   `json:"stop_on_error,omitempty"`
}

// BatchResult represents the result of a batch operation.
type BatchResult struct {
	TotalCount   int               `json:"total_count"`
	SuccessCount int               `json:"success_count"`
	FailedCount  int               `json:"failed_count"`
	Results      []BatchItemResult `json:"results"`
	Duration     time.Duration     `json:"duration"`
}

// BatchItemResult represents the result of a single item in a batch.
type BatchItemResult struct {
	Index      int    `json:"index"`
	Identifier string `json:"identifier"` // Serial number or ID
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	Output     string `json:"output,omitempty"`
}

// BatchVLANRequest represents a batch VLAN assignment request.
type BatchVLANRequest struct {
	Assignments []VLANConfig `json:"assignments"`
	StopOnError bool         `json:"stop_on_error,omitempty"`
}

// ConfigExport represents an exported configuration.
type ConfigExport struct {
	Timestamp  time.Time             `json:"timestamp"`
	DeviceHost string                `json:"device_host"`
	DeviceType string                `json:"device_type"`
	ONUs       []ONUProvisionRequest `json:"onus,omitempty"`
	VLANs      []VLANInfo            `json:"vlans,omitempty"`
	Profiles   ConfigProfiles        `json:"profiles,omitempty"`
}

// ConfigProfiles holds all profile configurations.
type ConfigProfiles struct {
	LineProfiles    []LineProfile    `json:"line_profiles,omitempty"`
	ServiceProfiles []ServiceProfile `json:"service_profiles,omitempty"`
	TrafficProfiles []TrafficProfile `json:"traffic_profiles,omitempty"`
	DBAProfiles     []DBAProfile     `json:"dba_profiles,omitempty"`
}

// =============================================================================
// Diagnostics Types
// =============================================================================

// ONUDiagnostics represents comprehensive ONU diagnostic information.
type ONUDiagnostics struct {
	PonPort      string `json:"pon_port"`
	OnuID        int    `json:"onu_id"`
	SerialNumber string `json:"serial_number"`
	Status       string `json:"status"`

	// Optical diagnostics
	Optical OpticalDiagnostics `json:"optical"`

	// Performance counters
	Counters PerformanceCounters `json:"counters"`

	// Device health
	Health DeviceHealth `json:"health"`

	// Connectivity
	Connectivity ConnectivityInfo `json:"connectivity"`

	// Timestamps
	LastUpdated time.Time `json:"last_updated"`
}

// OpticalDiagnostics represents optical power measurements.
type OpticalDiagnostics struct {
	RxPower       float64 `json:"rx_power_dbm"`
	TxPower       float64 `json:"tx_power_dbm"`
	OltRxPower    float64 `json:"olt_rx_power_dbm"`
	Temperature   float64 `json:"temperature_c"`
	Voltage       float64 `json:"voltage_v"`
	BiasCurrent   float64 `json:"bias_current_ma"`
	RxPowerStatus string  `json:"rx_power_status"` // normal, warning, critical
	TxPowerStatus string  `json:"tx_power_status"`
}

// PerformanceCounters represents traffic and error counters.
type PerformanceCounters struct {
	// Traffic counters
	RxBytes   uint64 `json:"rx_bytes"`
	TxBytes   uint64 `json:"tx_bytes"`
	RxPackets uint64 `json:"rx_packets"`
	TxPackets uint64 `json:"tx_packets"`

	// Error counters
	RxErrors  uint64 `json:"rx_errors"`
	TxErrors  uint64 `json:"tx_errors"`
	RxDropped uint64 `json:"rx_dropped"`
	TxDropped uint64 `json:"tx_dropped"`
	CRCErrors uint64 `json:"crc_errors"`
	FCSErrors uint64 `json:"fcs_errors"`

	// PON-specific
	BIPErrors uint64 `json:"bip_errors,omitempty"`
	REIErrors uint64 `json:"rei_errors,omitempty"`
}

// DeviceHealth represents ONU device health information.
type DeviceHealth struct {
	CPUUsage      float64 `json:"cpu_usage_percent,omitempty"`
	MemoryUsage   float64 `json:"memory_usage_percent,omitempty"`
	Temperature   float64 `json:"temperature_c,omitempty"`
	Uptime        string  `json:"uptime"`
	UptimeSeconds int64   `json:"uptime_seconds"`
	LastReboot    string  `json:"last_reboot_reason,omitempty"`
	FirmwareVer   string  `json:"firmware_version,omitempty"`
	HardwareVer   string  `json:"hardware_version,omitempty"`
}

// ConnectivityInfo represents ONU connectivity information.
type ConnectivityInfo struct {
	Distance         int       `json:"distance_m"`
	RTT              float64   `json:"rtt_ms,omitempty"`
	RegistrationTime time.Time `json:"registration_time,omitempty"`
	LastOnline       time.Time `json:"last_online,omitempty"`
	LastOffline      time.Time `json:"last_offline,omitempty"`
	OfflineReason    string    `json:"offline_reason,omitempty"`
	OfflineCount     int       `json:"offline_count_24h,omitempty"`
}

// SignalQuality represents signal quality metrics.
type SignalQuality struct {
	SNR     float64 `json:"snr_db,omitempty"`   // Signal-to-Noise Ratio
	BER     float64 `json:"ber,omitempty"`      // Bit Error Rate
	FER     float64 `json:"fer,omitempty"`      // Frame Error Rate
	RSSI    float64 `json:"rssi_dbm,omitempty"` // Received Signal Strength
	Quality string  `json:"quality"`            // excellent, good, fair, poor
}
