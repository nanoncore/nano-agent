package snmp

import (
	"testing"
)

func TestVSOLCollector_ParseDownCause(t *testing.T) {
	// Test with default config (no overrides)
	collector := NewVSOLCollector(DeviceConfig{Host: "test"})

	tests := []struct {
		name     string
		cause    int
		expected string
	}{
		{"unknown", 0, "unknown"},
		{"los", 1, "los"},
		{"lof", 2, "lof"},
		{"dying_gasp", 3, "dying_gasp"},
		{"power_off", 4, "power_off"},
		{"deregister", 5, "deregister"},
		{"onu_reboot", 6, "onu_reboot"},
		{"ranging_fail", 7, "ranging_fail"},
		{"lofi", 8, "lofi"},
		{"loami", 9, "loami"},
		{"sf_failure", 10, "sf_failure"},
		{"unknown cause", 99, "unknown(99)"},
		{"negative cause", -1, "unknown(-1)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.parseDownCause(tt.cause)
			if result != tt.expected {
				t.Errorf("parseDownCause(%d) = %s, want %s", tt.cause, result, tt.expected)
			}
		})
	}
}

func TestVSOLCollector_ParseDownCause_Overrides(t *testing.T) {
	// Test with config overrides
	collector := NewVSOLCollector(DeviceConfig{
		Host: "test",
		OfflineReasons: map[int]string{
			1:  "fiber_cut",     // Override default "los"
			99: "custom_reason", // Add new code
		},
	})

	tests := []struct {
		name     string
		cause    int
		expected string
	}{
		{"overridden los to fiber_cut", 1, "fiber_cut"},
		{"unchanged default lof", 2, "lof"},
		{"custom code 99", 99, "custom_reason"},
		{"still unknown for undefined", 200, "unknown(200)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.parseDownCause(tt.cause)
			if result != tt.expected {
				t.Errorf("parseDownCause(%d) = %s, want %s", tt.cause, result, tt.expected)
			}
		})
	}
}

func TestParseVSOLPortStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		expected string
	}{
		{"up", 1, "up"},
		{"down", 2, "down"},
		{"unknown zero", 0, "unknown"},
		{"unknown other", 99, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVSOLPortStatus(tt.status)
			if result != tt.expected {
				t.Errorf("parseVSOLPortStatus(%d) = %s, want %s", tt.status, result, tt.expected)
			}
		})
	}
}

func TestParseVSOLOpticalPower(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected float64
	}{
		{"integer positive", int64(2350), 23.50},
		{"integer negative", int64(-1250), -12.50},
		{"integer no signal low", int64(-32768), -40.0},
		{"integer no signal high", int64(0x7FFF), -40.0},
		{"string value", []byte("-15.5"), -15.5},
		{"nil value", nil, -40.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVSOLOpticalPower(tt.value)
			if result != tt.expected {
				t.Errorf("parseVSOLOpticalPower(%v) = %f, want %f", tt.value, result, tt.expected)
			}
		})
	}
}

func TestVSOLCollector_Vendor(t *testing.T) {
	config := DeviceConfig{
		Host:   "test-host",
		Vendor: VendorVSOL,
	}

	collector := NewVSOLCollector(config)
	if collector.Vendor() != VendorVSOL {
		t.Errorf("Vendor() = %v, want %v", collector.Vendor(), VendorVSOL)
	}
}

func TestVSOLOIDConstants(t *testing.T) {
	// Verify OID structure is correct
	if !hasPrefix(vsolOnuOIDs.InfoTable, VSOLOltBase) {
		t.Error("vsolOnuOIDs.InfoTable should have VSOLOltBase prefix")
	}
	if !hasPrefix(vsolOpticalOIDs.DiagTable, VSOLOltBase) {
		t.Error("vsolOpticalOIDs.DiagTable should have VSOLOltBase prefix")
	}
	if !hasPrefix(vsolTrafficOIDs.TrafficTable, VSOLOltBase) {
		t.Error("vsolTrafficOIDs.TrafficTable should have VSOLOltBase prefix")
	}
	if !hasPrefix(vsolUnauthOIDs.UnauthTable, VSOLOltBase) {
		t.Error("vsolUnauthOIDs.UnauthTable should have VSOLOltBase prefix")
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
