package snmp

import (
	"strings"
	"testing"
)

func TestParseHuaweiDownCause(t *testing.T) {
	tests := []struct {
		name     string
		cause    int
		expected string
	}{
		{"unknown", 1, "unknown"},
		{"los", 2, "los"},
		{"lof", 3, "lof"},
		{"lopc_miss", 4, "lopc_miss"},
		{"dying_gasp", 5, "dying_gasp"},
		{"ont_deregister", 6, "ont_deregister"},
		{"ont_reboot", 7, "ont_reboot"},
		{"losi", 8, "losi"},
		{"lofi", 9, "lofi"},
		{"loami", 10, "loami"},
		{"mem_failure", 11, "mem_failure"},
		{"sw_failure", 12, "sw_failure"},
		{"unknown cause", 99, "unknown(99)"},
		{"zero cause", 0, "unknown(0)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseHuaweiDownCause(tt.cause)
			if result != tt.expected {
				t.Errorf("parseHuaweiDownCause(%d) = %s, want %s", tt.cause, result, tt.expected)
			}
		})
	}
}

func TestDecodeHuaweiIfIndex(t *testing.T) {
	tests := []struct {
		name     string
		ifIndex  int
		wantSlot int
		wantPort int
	}{
		{"basic decode", 4352, 1, 0},
		{"slot 0 port 0", 0, 0, 0},
		{"slot 1 port 1", 4353, 1, 1},
		{"slot 2 port 3", 4867, 3, 3},
		{"large index", 8704, 2, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slot, port := decodeHuaweiIfIndex(tt.ifIndex)
			if slot != tt.wantSlot || port != tt.wantPort {
				t.Errorf("decodeHuaweiIfIndex(%d) = (%d, %d), want (%d, %d)",
					tt.ifIndex, slot, port, tt.wantSlot, tt.wantPort)
			}
		})
	}
}

func TestHuaweiCollector_Vendor(t *testing.T) {
	config := DeviceConfig{
		Host:   "test-host",
		Vendor: VendorHuawei,
	}

	collector := NewHuaweiCollector(config)
	if collector.Vendor() != VendorHuawei {
		t.Errorf("Vendor() = %v, want %v", collector.Vendor(), VendorHuawei)
	}
}

func TestHuaweiCollector_CommunityPadding(t *testing.T) {
	tests := []struct {
		name          string
		community     string
		wantMinLength int
	}{
		{"short community", "pub", 8},
		{"exact 8 chars", "password", 8},
		{"longer community", "longpassword", 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DeviceConfig{
				Host:      "test-host",
				Community: tt.community,
				Vendor:    VendorHuawei,
			}

			// NewHuaweiCollector should pad short communities
			collector := NewHuaweiCollector(config)
			if len(collector.config.Community) < tt.wantMinLength {
				t.Errorf("Community length = %d, want >= %d",
					len(collector.config.Community), tt.wantMinLength)
			}
		})
	}
}

func TestHuaweiOIDConstants(t *testing.T) {
	// Verify OID structure is correct
	if !strings.HasPrefix(huaweiOntOIDs.InfoTable, HuaweiXPON) {
		t.Error("huaweiOntOIDs.InfoTable should have HuaweiXPON prefix")
	}
	if !strings.HasPrefix(huaweiOpticalOIDs.DdmTable, HuaweiXPON) {
		t.Error("huaweiOpticalOIDs.DdmTable should have HuaweiXPON prefix")
	}
	if !strings.HasPrefix(huaweiAutoFindOIDs.AutoFindTable, HuaweiXPON) {
		t.Error("huaweiAutoFindOIDs.AutoFindTable should have HuaweiXPON prefix")
	}
}

func TestHuaweiOntStatus(t *testing.T) {
	if HuaweiOntStatusOnline != 1 {
		t.Errorf("HuaweiOntStatusOnline = %d, want 1", HuaweiOntStatusOnline)
	}
	if HuaweiOntStatusOffline != 2 {
		t.Errorf("HuaweiOntStatusOffline = %d, want 2", HuaweiOntStatusOffline)
	}
}
