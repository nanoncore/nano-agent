package main

import (
	"context"
	"fmt"
	"testing"
	"time"
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
