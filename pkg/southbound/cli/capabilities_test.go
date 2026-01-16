package cli

import (
	"testing"
)

func TestFullCapabilities(t *testing.T) {
	caps := FullCapabilities("huawei", "MA5800")

	if caps.Vendor != "huawei" {
		t.Errorf("expected vendor 'huawei', got %q", caps.Vendor)
	}
	if caps.Model != "MA5800" {
		t.Errorf("expected model 'MA5800', got %q", caps.Model)
	}
	if !caps.SupportsProvision {
		t.Error("expected SupportsProvision to be true")
	}
	if !caps.SupportsDelete {
		t.Error("expected SupportsDelete to be true")
	}
	if !caps.SupportsReboot {
		t.Error("expected SupportsReboot to be true")
	}
	if !caps.SupportsVLAN {
		t.Error("expected SupportsVLAN to be true")
	}
	if !caps.SupportsBatchProvision {
		t.Error("expected SupportsBatchProvision to be true")
	}
	if !caps.HasSNMP {
		t.Error("expected HasSNMP to be true")
	}
	if !caps.HasCLI {
		t.Error("expected HasCLI to be true")
	}
}

func TestReadOnlyCapabilities(t *testing.T) {
	caps := ReadOnlyCapabilities("zte", "C300")

	if caps.SupportsProvision {
		t.Error("expected SupportsProvision to be false")
	}
	if caps.SupportsDelete {
		t.Error("expected SupportsDelete to be false")
	}
	if caps.SupportsReboot {
		t.Error("expected SupportsReboot to be false")
	}
	if !caps.SupportsONUInfo {
		t.Error("expected SupportsONUInfo to be true")
	}
	if !caps.SupportsPortList {
		t.Error("expected SupportsPortList to be true")
	}
	if caps.SupportsPortControl {
		t.Error("expected SupportsPortControl to be false")
	}
}

func TestMinimalCapabilities(t *testing.T) {
	caps := MinimalCapabilities("cdata", "FD1104")

	if !caps.SupportsProvision {
		t.Error("expected SupportsProvision to be true")
	}
	if caps.SupportsReboot {
		t.Error("expected SupportsReboot to be false")
	}
	if caps.SupportsLineProfiles {
		t.Error("expected SupportsLineProfiles to be false")
	}
	if caps.SupportsBatchProvision {
		t.Error("expected SupportsBatchProvision to be false")
	}
	if caps.MaxONUsPerPort != 32 {
		t.Errorf("expected MaxONUsPerPort 32, got %d", caps.MaxONUsPerPort)
	}
}

func TestHuaweiMA5800Capabilities(t *testing.T) {
	caps := HuaweiMA5800Capabilities()

	if caps.Vendor != "huawei" {
		t.Errorf("expected vendor 'huawei', got %q", caps.Vendor)
	}
	if caps.Model != "MA5800" {
		t.Errorf("expected model 'MA5800', got %q", caps.Model)
	}
	if !caps.HasNETCONF {
		t.Error("expected HasNETCONF to be true")
	}
	if caps.PortFormat != "frame/slot/port" {
		t.Errorf("expected port format 'frame/slot/port', got %q", caps.PortFormat)
	}
	if caps.MaxONUsPerPort != 128 {
		t.Errorf("expected MaxONUsPerPort 128, got %d", caps.MaxONUsPerPort)
	}
	if caps.MaxPONPorts != 256 {
		t.Errorf("expected MaxPONPorts 256, got %d", caps.MaxPONPorts)
	}
}

func TestHuaweiMA5600TCapabilities(t *testing.T) {
	caps := HuaweiMA5600TCapabilities()

	if caps.HasNETCONF {
		t.Error("expected HasNETCONF to be false for older model")
	}
	if caps.MaxONUsPerPort != 64 {
		t.Errorf("expected MaxONUsPerPort 64, got %d", caps.MaxONUsPerPort)
	}
}

func TestVSOLCapabilities(t *testing.T) {
	caps := VSOLCapabilities("V1600D")

	if caps.Vendor != "vsol" {
		t.Errorf("expected vendor 'vsol', got %q", caps.Vendor)
	}
	if !caps.SupportsProvision {
		t.Error("expected SupportsProvision to be true")
	}
	if caps.SupportsLineProfiles {
		t.Error("expected SupportsLineProfiles to be false")
	}
	if caps.PortFormat != "slot/port" {
		t.Errorf("expected port format 'slot/port', got %q", caps.PortFormat)
	}
}

func TestZTECapabilities(t *testing.T) {
	c300 := ZTEC300Capabilities()
	c600 := ZTEC600Capabilities()

	if c300.HasNETCONF {
		t.Error("expected C300 HasNETCONF to be false")
	}
	if !c600.HasNETCONF {
		t.Error("expected C600 HasNETCONF to be true")
	}
	if c300.PortFormat != "shelf/slot/port" {
		t.Errorf("expected ZTE port format 'shelf/slot/port', got %q", c300.PortFormat)
	}
}

func TestNokiaCapabilities(t *testing.T) {
	caps := NokiaISAMCapabilities()

	if caps.Vendor != "nokia" {
		t.Errorf("expected vendor 'nokia', got %q", caps.Vendor)
	}
	if !caps.HasNETCONF {
		t.Error("expected HasNETCONF to be true")
	}
	if caps.PortFormat != "rack/shelf/slot/port" {
		t.Errorf("expected port format 'rack/shelf/slot/port', got %q", caps.PortFormat)
	}
}

// =============================================================================
// Helper Method Tests
// =============================================================================

func TestCanProvisionONU(t *testing.T) {
	full := FullCapabilities("huawei", "MA5800")
	if !full.CanProvisionONU() {
		t.Error("expected CanProvisionONU to be true for full capabilities")
	}

	readOnly := ReadOnlyCapabilities("zte", "C300")
	if readOnly.CanProvisionONU() {
		t.Error("expected CanProvisionONU to be false for read-only capabilities")
	}
}

func TestCanManageONU(t *testing.T) {
	full := FullCapabilities("huawei", "MA5800")
	if !full.CanManageONU() {
		t.Error("expected CanManageONU to be true for full capabilities")
	}

	minimal := MinimalCapabilities("cdata", "FD1104")
	if minimal.CanManageONU() {
		t.Error("expected CanManageONU to be false for minimal capabilities (no reboot)")
	}
}

func TestCanManagePorts(t *testing.T) {
	full := FullCapabilities("huawei", "MA5800")
	if !full.CanManagePorts() {
		t.Error("expected CanManagePorts to be true for full capabilities")
	}

	readOnly := ReadOnlyCapabilities("zte", "C300")
	if readOnly.CanManagePorts() {
		t.Error("expected CanManagePorts to be false for read-only capabilities")
	}
}

func TestCanManageVLAN(t *testing.T) {
	full := FullCapabilities("huawei", "MA5800")
	if !full.CanManageVLAN() {
		t.Error("expected CanManageVLAN to be true for full capabilities")
	}
}

func TestCanManageProfiles(t *testing.T) {
	full := FullCapabilities("huawei", "MA5800")
	if !full.CanManageProfiles() {
		t.Error("expected CanManageProfiles to be true for full capabilities")
	}

	minimal := MinimalCapabilities("cdata", "FD1104")
	if minimal.CanManageProfiles() {
		t.Error("expected CanManageProfiles to be false for minimal capabilities")
	}
}

func TestCanBatchProvision(t *testing.T) {
	full := FullCapabilities("huawei", "MA5800")
	if !full.CanBatchProvision() {
		t.Error("expected CanBatchProvision to be true for full capabilities")
	}

	minimal := MinimalCapabilities("cdata", "FD1104")
	if minimal.CanBatchProvision() {
		t.Error("expected CanBatchProvision to be false for minimal capabilities")
	}
}

func TestCanRunDiagnostics(t *testing.T) {
	full := FullCapabilities("huawei", "MA5800")
	if !full.CanRunDiagnostics() {
		t.Error("expected CanRunDiagnostics to be true for full capabilities")
	}
}

func TestHasProtocol(t *testing.T) {
	full := FullCapabilities("huawei", "MA5800")

	if !full.HasProtocol("snmp") {
		t.Error("expected HasProtocol('snmp') to be true")
	}
	if !full.HasProtocol("cli") {
		t.Error("expected HasProtocol('cli') to be true")
	}
	if !full.HasProtocol("ssh") {
		t.Error("expected HasProtocol('ssh') to be true (alias for cli)")
	}
	if full.HasProtocol("netconf") {
		t.Error("expected HasProtocol('netconf') to be false for FullCapabilities")
	}
	if full.HasProtocol("unknown") {
		t.Error("expected HasProtocol('unknown') to be false")
	}

	nokia := NokiaISAMCapabilities()
	if !nokia.HasProtocol("netconf") {
		t.Error("expected HasProtocol('netconf') to be true for Nokia")
	}
}

func TestIsReadOnly(t *testing.T) {
	readOnly := ReadOnlyCapabilities("zte", "C300")
	if !readOnly.IsReadOnly() {
		t.Error("expected IsReadOnly to be true for read-only capabilities")
	}

	full := FullCapabilities("huawei", "MA5800")
	if full.IsReadOnly() {
		t.Error("expected IsReadOnly to be false for full capabilities")
	}
}

func TestCapabilitiesString(t *testing.T) {
	caps := FullCapabilities("huawei", "MA5800")
	str := caps.String()

	if str == "" {
		t.Error("expected non-empty string representation")
	}
	if !contains(str, "huawei") {
		t.Errorf("expected string to contain 'huawei', got %q", str)
	}
	if !contains(str, "MA5800") {
		t.Errorf("expected string to contain 'MA5800', got %q", str)
	}
	if !contains(str, "SNMP") {
		t.Errorf("expected string to contain 'SNMP', got %q", str)
	}
	if !contains(str, "CLI") {
		t.Errorf("expected string to contain 'CLI', got %q", str)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
