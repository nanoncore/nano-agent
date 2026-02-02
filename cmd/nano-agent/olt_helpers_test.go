package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nanoncore/nano-southbound/types"
)

func TestValidateSerialNumber(t *testing.T) {
	tests := []struct {
		name    string
		serial  string
		wantErr bool
	}{
		{
			name:    "valid serial - uppercase",
			serial:  "HWTC12345678",
			wantErr: false,
		},
		{
			name:    "valid serial - lowercase letters",
			serial:  "hwtc12345678",
			wantErr: false,
		},
		{
			name:    "valid serial - mixed case",
			serial:  "HwTc12345678",
			wantErr: false,
		},
		{
			name:    "valid serial - hex letters in digits",
			serial:  "VSOL0ABCDEF1",
			wantErr: false,
		},
		{
			name:    "empty serial",
			serial:  "",
			wantErr: true,
		},
		{
			name:    "too short",
			serial:  "HWTC1234567",
			wantErr: true,
		},
		{
			name:    "too long",
			serial:  "HWTC123456789",
			wantErr: true,
		},
		{
			name:    "only 3 letters",
			serial:  "HWT12345678",
			wantErr: true,
		},
		{
			name:    "5 letters prefix",
			serial:  "HWTCA234567",
			wantErr: true,
		},
		{
			name:    "invalid hex digit",
			serial:  "HWTC1234567G",
			wantErr: true,
		},
		{
			name:    "number in letter section",
			serial:  "HWT112345678",
			wantErr: true,
		},
		{
			name:    "spaces not allowed",
			serial:  "HWTC 2345678",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSerialNumber(tt.serial)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSerialNumber(%q) error = %v, wantErr %v", tt.serial, err, tt.wantErr)
			}
		})
	}
}

func TestValidateONUIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		serial  string
		ponPort string
		onuID   int
		wantErr bool
	}{
		{
			name:    "valid serial only",
			serial:  "HWTC12345678",
			ponPort: "",
			onuID:   0,
			wantErr: false,
		},
		{
			name:    "valid port and id",
			serial:  "",
			ponPort: "0/0/1",
			onuID:   101,
			wantErr: false,
		},
		{
			name:    "serial with port and id",
			serial:  "HWTC12345678",
			ponPort: "0/0/1",
			onuID:   101,
			wantErr: false,
		},
		{
			name:    "missing all identifiers",
			serial:  "",
			ponPort: "",
			onuID:   0,
			wantErr: true,
		},
		{
			name:    "port without id",
			serial:  "",
			ponPort: "0/0/1",
			onuID:   0,
			wantErr: true,
		},
		{
			name:    "id without port",
			serial:  "",
			ponPort: "",
			onuID:   101,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateONUIdentifier(tt.serial, tt.ponPort, tt.onuID)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateONUIdentifier(%q, %q, %d) error = %v, wantErr %v",
					tt.serial, tt.ponPort, tt.onuID, err, tt.wantErr)
			}
		})
	}
}

func TestSerialNumberRegex(t *testing.T) {
	validSerials := []string{
		"HWTC12345678",
		"VSOL0ABCDEF1",
		"ZTEG00000000",
		"ALCLffffffff",
		"abcd00000000",
		"XyZw12AB34CD",
	}

	for _, serial := range validSerials {
		if !serialNumberRegex.MatchString(serial) {
			t.Errorf("serialNumberRegex should match valid serial %q", serial)
		}
	}

	invalidSerials := []string{
		"",
		"HWT12345678",   // only 3 letters
		"HWTC1234567",   // only 7 hex digits
		"HWTC123456789", // 9 hex digits
		"1234ABCDEFGH",  // starts with numbers
		"HWTC1234567G",  // invalid hex digit 'G'
		"HWTC 2345678",  // space
		"HWTC-12345678", // hyphen
	}

	for _, serial := range invalidSerials {
		if serialNumberRegex.MatchString(serial) {
			t.Errorf("serialNumberRegex should not match invalid serial %q", serial)
		}
	}
}

func TestValidateProfileVLANConsistency(t *testing.T) {
	tests := []struct {
		name        string
		lineProfile string
		vlan        int
		force       bool
		wantDecision string
		wantErr     bool
	}{
		{
			name:        "no validation - empty profile",
			lineProfile: "",
			vlan:        100,
			force:       false,
			wantDecision: "",
			wantErr:     false,
		},
		{
			name:        "no validation - zero VLAN",
			lineProfile: "line_vlan_100",
			vlan:        0,
			force:       false,
			wantDecision: "",
			wantErr:     false,
		},
		{
			name:        "no validation - both empty",
			lineProfile: "",
			vlan:        0,
			force:       false,
			wantDecision: "",
			wantErr:     false,
		},
		{
			name:        "match - profile and VLAN match (underscore)",
			lineProfile: "line_vlan_100",
			vlan:        100,
			force:       false,
			wantDecision: "profile",
			wantErr:     false,
		},
		{
			name:        "match - profile and VLAN match (hyphen)",
			lineProfile: "line-vlan-200",
			vlan:        200,
			force:       false,
			wantDecision: "profile",
			wantErr:     false,
		},
		{
			name:        "match - without line prefix (underscore)",
			lineProfile: "vlan_300",
			vlan:        300,
			force:       false,
			wantDecision: "profile",
			wantErr:     false,
		},
		{
			name:        "match - without line prefix (hyphen)",
			lineProfile: "vlan-400",
			vlan:        400,
			force:       false,
			wantDecision: "profile",
			wantErr:     false,
		},
		{
			name:        "mismatch - no force flag",
			lineProfile: "line_vlan_100",
			vlan:        200,
			force:       false,
			wantDecision: "",
			wantErr:     true,
		},
		{
			name:        "mismatch - with force flag",
			lineProfile: "line_vlan_100",
			vlan:        200,
			force:       true,
			wantDecision: "direct-vlan",
			wantErr:     false,
		},
		{
			name:        "convention not followed - arbitrary name",
			lineProfile: "HSI_1G",
			vlan:        100,
			force:       false,
			wantDecision: "profile",
			wantErr:     false,
		},
		{
			name:        "convention not followed - no vlan keyword",
			lineProfile: "line_100",
			vlan:        100,
			force:       false,
			wantDecision: "profile",
			wantErr:     false,
		},
		{
			name:        "match - multi-digit VLAN",
			lineProfile: "line_vlan_4094",
			vlan:        4094,
			force:       false,
			wantDecision: "profile",
			wantErr:     false,
		},
		{
			name:        "mismatch - multi-digit VLAN with force",
			lineProfile: "line_vlan_4094",
			vlan:        100,
			force:       true,
			wantDecision: "direct-vlan",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := validateProfileVLANConsistency(tt.lineProfile, tt.vlan, tt.force)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProfileVLANConsistency(%q, %d, %v) error = %v, wantErr %v",
					tt.lineProfile, tt.vlan, tt.force, err, tt.wantErr)
				return
			}

			// Check decision matches
			if decision != tt.wantDecision {
				t.Errorf("validateProfileVLANConsistency(%q, %d, %v) decision = %q, want %q",
					tt.lineProfile, tt.vlan, tt.force, decision, tt.wantDecision)
			}

			// For mismatch errors, verify error message contains helpful guidance
			if tt.wantErr && err != nil {
				errMsg := err.Error()
				if tt.lineProfile != "" && tt.vlan > 0 {
					// Should contain the profile name and VLAN in error message
					// This is a basic check - could be more specific
					if len(errMsg) < 50 {
						t.Errorf("Error message too short, expected detailed guidance: %q", errMsg)
					}
				}
			}
		})
	}
}

// TestVerifyONUChange tests the generic retry verification logic (NAN-257)
func TestVerifyONUChange(t *testing.T) {
	tests := []struct {
		name         string
		verifyFunc   func() (bool, error)
		maxRetries   int
		retryDelay   time.Duration
		expectErr    bool
		expectAttempts int
	}{
		{
			name: "success on first attempt",
			verifyFunc: func() (bool, error) {
				return true, nil
			},
			maxRetries:     3,
			retryDelay:     1 * time.Millisecond,
			expectErr:      false,
			expectAttempts: 1,
		},
		{
			name: "success on second attempt",
			verifyFunc: (func() func() (bool, error) {
				attempt := 0
				return func() (bool, error) {
					attempt++
					if attempt == 2 {
						return true, nil
					}
					return false, nil
				}
			})(),
			maxRetries:     3,
			retryDelay:     1 * time.Millisecond,
			expectErr:      false,
			expectAttempts: 2,
		},
		{
			name: "fail all retries",
			verifyFunc: func() (bool, error) {
				return false, nil
			},
			maxRetries:     3,
			retryDelay:     1 * time.Millisecond,
			expectErr:      true,
			expectAttempts: 3,
		},
		{
			name: "verification error",
			verifyFunc: func() (bool, error) {
				return false, fmt.Errorf("verification error")
			},
			maxRetries:     3,
			retryDelay:     1 * time.Millisecond,
			expectErr:      true,
			expectAttempts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original outputJSON state and restore after test
			origOutputJSON := outputJSON
			outputJSON = true
			defer func() { outputJSON = origOutputJSON }()

			ctx := context.Background()
			err := verifyONUChange(ctx, tt.verifyFunc, tt.maxRetries, tt.retryDelay)

			if (err != nil) != tt.expectErr {
				t.Errorf("verifyONUChange() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestExtractVLANFromProfileName tests VLAN extraction from profile names (NAN-258)
func TestExtractVLANFromProfileName(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
		wantVLAN    int
		wantErr     bool
	}{
		{
			name:        "line profile with underscore",
			profileName: "line_vlan_100",
			wantVLAN:    100,
			wantErr:     false,
		},
		{
			name:        "line profile with hyphen",
			profileName: "line-vlan-200",
			wantVLAN:    200,
			wantErr:     false,
		},
		{
			name:        "without line prefix - underscore",
			profileName: "vlan_300",
			wantVLAN:    300,
			wantErr:     false,
		},
		{
			name:        "without line prefix - hyphen",
			profileName: "vlan-400",
			wantVLAN:    400,
			wantErr:     false,
		},
		{
			name:        "multi-digit VLAN",
			profileName: "line_vlan_4094",
			wantVLAN:    4094,
			wantErr:     false,
		},
		{
			name:        "no vlan keyword",
			profileName: "HSI_1G",
			wantVLAN:    0,
			wantErr:     true,
		},
		{
			name:        "empty profile name",
			profileName: "",
			wantVLAN:    0,
			wantErr:     true,
		},
		{
			name:        "profile with vlan but no number",
			profileName: "line_vlan_",
			wantVLAN:    0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vlan, err := extractVLANFromProfileName(tt.profileName)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("extractVLANFromProfileName(%q) error = %v, wantErr %v",
					tt.profileName, err, tt.wantErr)
				return
			}

			// Check VLAN matches
			if vlan != tt.wantVLAN {
				t.Errorf("extractVLANFromProfileName(%q) vlan = %d, want %d",
					tt.profileName, vlan, tt.wantVLAN)
			}
		})
	}
}

// TestBuildProvisionModelsFromUpdate tests conversion of update params to provision models (NAN-259)
func TestBuildProvisionModelsFromUpdate(t *testing.T) {
	preONU := &types.ONUInfo{
		ONUID:         5,
		PONPort:       "0/1",
		Serial:        "HWTC12345678",
		VLAN:          100,
		LineProfile:   "line_vlan_100",
		BandwidthDown: 100,
		BandwidthUp:   50,
	}

	tests := []struct {
		name           string
		serial         string
		ponPort        string
		onuID          int
		lineProfile    string
		serviceProfile string
		vlan           int
		trafficProfile int
		description    string
		wantVLAN       int
		wantBWDown     int
		wantBWUp       int
	}{
		{
			name:           "new line profile with VLAN",
			serial:         "HWTC12345678",
			ponPort:        "0/1",
			onuID:          5,
			lineProfile:    "line_vlan_200",
			serviceProfile: "",
			vlan:           200,
			trafficProfile: 0,
			description:    "Test ONU",
			wantVLAN:       200,
			wantBWDown:     100, // Preserved from preONU
			wantBWUp:       50,
		},
		{
			name:           "VLAN only, preserve bandwidth",
			serial:         "HWTC12345678",
			ponPort:        "0/1",
			onuID:          5,
			lineProfile:    "",
			serviceProfile: "",
			vlan:           300,
			trafficProfile: 0,
			description:    "",
			wantVLAN:       300,
			wantBWDown:     100, // Preserved from preONU
			wantBWUp:       50,
		},
		{
			name:           "with traffic profile",
			serial:         "HWTC12345678",
			ponPort:        "0/1",
			onuID:          5,
			lineProfile:    "",
			serviceProfile: "",
			vlan:           200,
			trafficProfile: 200,
			description:    "",
			wantVLAN:       200,
			wantBWDown:     200, // From traffic profile
			wantBWUp:       100, // traffic profile / 2
		},
		{
			name:           "default bandwidth when zero",
			serial:         "HWTC12345678",
			ponPort:        "0/1",
			onuID:          5,
			lineProfile:    "",
			serviceProfile: "",
			vlan:           200,
			trafficProfile: 0,
			description:    "",
			wantVLAN:       200,
			wantBWDown:     100, // Preserved from preONU (not zero)
			wantBWUp:       50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscriber, tier := buildProvisionModelsFromUpdate(
				preONU,
				tt.serial,
				tt.ponPort,
				tt.onuID,
				tt.lineProfile,
				tt.serviceProfile,
				tt.vlan,
				tt.trafficProfile,
				tt.description,
			)

			// Check subscriber fields
			if subscriber.Name != tt.serial {
				t.Errorf("subscriber.Name = %q, want %q", subscriber.Name, tt.serial)
			}

			if subscriber.Spec.ONUSerial != tt.serial {
				t.Errorf("subscriber.Spec.ONUSerial = %q, want %q", subscriber.Spec.ONUSerial, tt.serial)
			}

			if subscriber.Spec.VLAN != tt.wantVLAN {
				t.Errorf("subscriber.Spec.VLAN = %d, want %d", subscriber.Spec.VLAN, tt.wantVLAN)
			}

			// Check annotations
			if ponPort := subscriber.Annotations["nano.io/pon-port"]; ponPort != tt.ponPort {
				t.Errorf("pon-port annotation = %q, want %q", ponPort, tt.ponPort)
			}

			if tt.lineProfile != "" {
				if lp := subscriber.Annotations["nano.io/line-profile"]; lp != tt.lineProfile {
					t.Errorf("line-profile annotation = %q, want %q", lp, tt.lineProfile)
				}
			}

			if tt.serviceProfile != "" {
				if sp := subscriber.Annotations["nano.io/service-profile"]; sp != tt.serviceProfile {
					t.Errorf("service-profile annotation = %q, want %q", sp, tt.serviceProfile)
				}
			}

			if tt.description != "" {
				if desc := subscriber.Annotations["nano.io/description"]; desc != tt.description {
					t.Errorf("description annotation = %q, want %q", desc, tt.description)
				}
			}

			// Check tier fields
			if tier.Spec.BandwidthDown != tt.wantBWDown {
				t.Errorf("tier.Spec.BandwidthDown = %d, want %d", tier.Spec.BandwidthDown, tt.wantBWDown)
			}

			if tier.Spec.BandwidthUp != tt.wantBWUp {
				t.Errorf("tier.Spec.BandwidthUp = %d, want %d", tier.Spec.BandwidthUp, tt.wantBWUp)
			}
		})
	}
}
