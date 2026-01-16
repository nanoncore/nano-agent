package cli

// VendorCapabilities defines the feature support matrix for a vendor/model combination.
// This enables runtime capability detection for graceful degradation when
// features aren't supported by a specific OLT vendor or model.
type VendorCapabilities struct {
	// Vendor identity
	Vendor   string `json:"vendor"`
	Model    string `json:"model,omitempty"`
	Firmware string `json:"firmware,omitempty"`

	// ONU Management capabilities
	SupportsProvision bool `json:"supports_provision"`
	SupportsDelete    bool `json:"supports_delete"`
	SupportsReboot    bool `json:"supports_reboot"`
	SupportsONUInfo   bool `json:"supports_onu_info"`

	// Port Management capabilities
	SupportsPortList    bool `json:"supports_port_list"`
	SupportsPortControl bool `json:"supports_port_control"`

	// VLAN Management capabilities
	SupportsVLAN            bool `json:"supports_vlan"`
	SupportsVLANTranslation bool `json:"supports_vlan_translation"`
	SupportsServicePorts    bool `json:"supports_service_ports"`

	// Profile Management capabilities
	SupportsLineProfiles    bool `json:"supports_line_profiles"`
	SupportsServiceProfiles bool `json:"supports_service_profiles"`
	SupportsTrafficProfiles bool `json:"supports_traffic_profiles"`
	SupportsDBAProfiles     bool `json:"supports_dba_profiles"`

	// Batch Operations capabilities
	SupportsBatchProvision bool `json:"supports_batch_provision"`
	SupportsBatchVLAN      bool `json:"supports_batch_vlan"`
	SupportsConfigExport   bool `json:"supports_config_export"`

	// Diagnostics capabilities
	SupportsDiagnostics       bool `json:"supports_diagnostics"`
	SupportsOpticalDiag       bool `json:"supports_optical_diag"`
	SupportsPerformanceCounters bool `json:"supports_performance_counters"`

	// Protocol support
	HasSNMP    bool `json:"has_snmp"`
	HasCLI     bool `json:"has_cli"`
	HasNETCONF bool `json:"has_netconf"`
	HasGNMI    bool `json:"has_gnmi"`
	HasREST    bool `json:"has_rest"`

	// Addressing scheme
	PortFormat     string `json:"port_format"`      // "frame/slot/port", "shelf/slot/port", "slot/port"
	MaxONUsPerPort int    `json:"max_onus_per_port"`
	MaxPONPorts    int    `json:"max_pon_ports,omitempty"`
}

// =============================================================================
// Helper Methods for Capability Checking
// =============================================================================

// CanProvisionONU returns true if the driver supports ONU provisioning operations.
func (c *VendorCapabilities) CanProvisionONU() bool {
	return c.SupportsProvision && c.SupportsDelete
}

// CanManageONU returns true if the driver supports full ONU management
// (provision, delete, reboot, and info retrieval).
func (c *VendorCapabilities) CanManageONU() bool {
	return c.SupportsProvision && c.SupportsDelete && c.SupportsReboot && c.SupportsONUInfo
}

// CanManagePorts returns true if the driver supports port management operations.
func (c *VendorCapabilities) CanManagePorts() bool {
	return c.SupportsPortList && c.SupportsPortControl
}

// CanManageVLAN returns true if the driver supports VLAN configuration.
func (c *VendorCapabilities) CanManageVLAN() bool {
	return c.SupportsVLAN
}

// CanManageProfiles returns true if the driver supports profile management.
func (c *VendorCapabilities) CanManageProfiles() bool {
	return c.SupportsLineProfiles || c.SupportsServiceProfiles || c.SupportsTrafficProfiles
}

// CanBatchProvision returns true if the driver supports batch ONU provisioning.
func (c *VendorCapabilities) CanBatchProvision() bool {
	return c.SupportsBatchProvision && c.SupportsProvision
}

// CanRunDiagnostics returns true if the driver supports diagnostic operations.
func (c *VendorCapabilities) CanRunDiagnostics() bool {
	return c.SupportsDiagnostics || c.SupportsOpticalDiag
}

// HasProtocol returns true if the driver supports the specified protocol.
func (c *VendorCapabilities) HasProtocol(protocol string) bool {
	switch protocol {
	case "snmp":
		return c.HasSNMP
	case "cli", "ssh":
		return c.HasCLI
	case "netconf":
		return c.HasNETCONF
	case "gnmi":
		return c.HasGNMI
	case "rest", "http":
		return c.HasREST
	default:
		return false
	}
}

// IsReadOnly returns true if the driver only supports read operations.
func (c *VendorCapabilities) IsReadOnly() bool {
	return !c.SupportsProvision && !c.SupportsDelete && !c.SupportsReboot && !c.SupportsPortControl
}

// String returns a human-readable summary of the capabilities.
func (c *VendorCapabilities) String() string {
	protocols := []string{}
	if c.HasSNMP {
		protocols = append(protocols, "SNMP")
	}
	if c.HasCLI {
		protocols = append(protocols, "CLI")
	}
	if c.HasNETCONF {
		protocols = append(protocols, "NETCONF")
	}
	if c.HasGNMI {
		protocols = append(protocols, "gNMI")
	}
	if c.HasREST {
		protocols = append(protocols, "REST")
	}

	model := c.Model
	if model == "" {
		model = "generic"
	}

	return c.Vendor + "/" + model + " [" + c.PortFormat + "] protocols=" + joinStrings(protocols, ",")
}

// joinStrings joins strings with a separator (helper to avoid importing strings package).
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// =============================================================================
// Factory Functions for Creating Capabilities
// =============================================================================

// FullCapabilities returns a VendorCapabilities with all features enabled.
// Use this for vendors with complete feature support.
func FullCapabilities(vendor, model string) *VendorCapabilities {
	return &VendorCapabilities{
		Vendor:   vendor,
		Model:    model,

		// ONU Management
		SupportsProvision: true,
		SupportsDelete:    true,
		SupportsReboot:    true,
		SupportsONUInfo:   true,

		// Port Management
		SupportsPortList:    true,
		SupportsPortControl: true,

		// VLAN Management
		SupportsVLAN:            true,
		SupportsVLANTranslation: true,
		SupportsServicePorts:    true,

		// Profile Management
		SupportsLineProfiles:    true,
		SupportsServiceProfiles: true,
		SupportsTrafficProfiles: true,
		SupportsDBAProfiles:     true,

		// Batch Operations
		SupportsBatchProvision: true,
		SupportsBatchVLAN:      true,
		SupportsConfigExport:   true,

		// Diagnostics
		SupportsDiagnostics:       true,
		SupportsOpticalDiag:       true,
		SupportsPerformanceCounters: true,

		// Protocol support
		HasSNMP:    true,
		HasCLI:     true,
		HasNETCONF: false,
		HasGNMI:    false,
		HasREST:    false,

		// Defaults
		PortFormat:     "frame/slot/port",
		MaxONUsPerPort: 128,
	}
}

// ReadOnlyCapabilities returns a VendorCapabilities with only read operations enabled.
// Use this for vendors with limited CLI write support.
func ReadOnlyCapabilities(vendor, model string) *VendorCapabilities {
	return &VendorCapabilities{
		Vendor:   vendor,
		Model:    model,

		// ONU Management - read only
		SupportsProvision: false,
		SupportsDelete:    false,
		SupportsReboot:    false,
		SupportsONUInfo:   true,

		// Port Management - read only
		SupportsPortList:    true,
		SupportsPortControl: false,

		// VLAN Management - read only
		SupportsVLAN:            true,
		SupportsVLANTranslation: false,
		SupportsServicePorts:    false,

		// Profile Management - read only
		SupportsLineProfiles:    true,
		SupportsServiceProfiles: true,
		SupportsTrafficProfiles: true,
		SupportsDBAProfiles:     false,

		// Batch Operations - disabled
		SupportsBatchProvision: false,
		SupportsBatchVLAN:      false,
		SupportsConfigExport:   true,

		// Diagnostics - full support
		SupportsDiagnostics:       true,
		SupportsOpticalDiag:       true,
		SupportsPerformanceCounters: true,

		// Protocol support
		HasSNMP:    true,
		HasCLI:     true,
		HasNETCONF: false,
		HasGNMI:    false,
		HasREST:    false,

		// Defaults
		PortFormat:     "slot/port",
		MaxONUsPerPort: 64,
	}
}

// MinimalCapabilities returns a VendorCapabilities with minimal features.
// Use this for budget OLTs with very limited capabilities.
func MinimalCapabilities(vendor, model string) *VendorCapabilities {
	return &VendorCapabilities{
		Vendor:   vendor,
		Model:    model,

		// ONU Management - basic only
		SupportsProvision: true,
		SupportsDelete:    true,
		SupportsReboot:    false,
		SupportsONUInfo:   true,

		// Port Management - disabled
		SupportsPortList:    true,
		SupportsPortControl: false,

		// VLAN Management - basic only
		SupportsVLAN:            true,
		SupportsVLANTranslation: false,
		SupportsServicePorts:    false,

		// Profile Management - disabled
		SupportsLineProfiles:    false,
		SupportsServiceProfiles: false,
		SupportsTrafficProfiles: false,
		SupportsDBAProfiles:     false,

		// Batch Operations - disabled
		SupportsBatchProvision: false,
		SupportsBatchVLAN:      false,
		SupportsConfigExport:   false,

		// Diagnostics - basic
		SupportsDiagnostics:       true,
		SupportsOpticalDiag:       true,
		SupportsPerformanceCounters: false,

		// Protocol support
		HasSNMP:    true,
		HasCLI:     true,
		HasNETCONF: false,
		HasGNMI:    false,
		HasREST:    false,

		// Defaults
		PortFormat:     "slot/port",
		MaxONUsPerPort: 32,
	}
}

// HuaweiMA5800Capabilities returns capabilities for Huawei MA5800 series.
func HuaweiMA5800Capabilities() *VendorCapabilities {
	caps := FullCapabilities("huawei", "MA5800")
	caps.HasNETCONF = true
	caps.PortFormat = "frame/slot/port"
	caps.MaxONUsPerPort = 128
	caps.MaxPONPorts = 256
	return caps
}

// HuaweiMA5600TCapabilities returns capabilities for Huawei MA5600T.
func HuaweiMA5600TCapabilities() *VendorCapabilities {
	caps := FullCapabilities("huawei", "MA5600T")
	caps.HasNETCONF = false // Older model
	caps.PortFormat = "frame/slot/port"
	caps.MaxONUsPerPort = 64
	caps.MaxPONPorts = 128
	return caps
}

// ZTEC300Capabilities returns capabilities for ZTE C300 series.
func ZTEC300Capabilities() *VendorCapabilities {
	caps := FullCapabilities("zte", "C300")
	caps.PortFormat = "shelf/slot/port"
	caps.MaxONUsPerPort = 128
	return caps
}

// ZTEC600Capabilities returns capabilities for ZTE C600 series.
func ZTEC600Capabilities() *VendorCapabilities {
	caps := FullCapabilities("zte", "C600")
	caps.HasNETCONF = true
	caps.PortFormat = "shelf/slot/port"
	caps.MaxONUsPerPort = 128
	caps.MaxPONPorts = 512
	return caps
}

// NokiaISAMCapabilities returns capabilities for Nokia ISAM FX.
func NokiaISAMCapabilities() *VendorCapabilities {
	caps := FullCapabilities("nokia", "ISAM")
	caps.HasNETCONF = true
	caps.PortFormat = "rack/shelf/slot/port"
	caps.MaxONUsPerPort = 128
	return caps
}

// VSOLCapabilities returns capabilities for V-SOL OLTs.
func VSOLCapabilities(model string) *VendorCapabilities {
	caps := MinimalCapabilities("vsol", model)
	caps.SupportsProvision = true
	caps.SupportsDelete = true
	caps.SupportsVLAN = true
	caps.PortFormat = "slot/port"
	caps.MaxONUsPerPort = 64
	return caps
}

// CDataCapabilities returns capabilities for CData OLTs.
func CDataCapabilities(model string) *VendorCapabilities {
	caps := MinimalCapabilities("cdata", model)
	caps.PortFormat = "slot/port"
	caps.MaxONUsPerPort = 32
	return caps
}

// FiberHomeCapabilities returns capabilities for FiberHome OLTs.
func FiberHomeCapabilities(model string) *VendorCapabilities {
	caps := FullCapabilities("fiberhome", model)
	caps.HasNETCONF = true
	caps.PortFormat = "frame/slot/port"
	caps.MaxONUsPerPort = 128
	return caps
}
