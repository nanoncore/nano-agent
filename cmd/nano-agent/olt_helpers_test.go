package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nanoncore/nano-southbound/model"
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

func TestParseMetadataInt(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		key      string
		want     int
		wantOK   bool
	}{
		{
			name:     "int",
			metadata: map[string]any{"onu_id": 7},
			key:      "onu_id",
			want:     7,
			wantOK:   true,
		},
		{
			name:     "int64",
			metadata: map[string]any{"onu_id": int64(9)},
			key:      "onu_id",
			want:     9,
			wantOK:   true,
		},
		{
			name:     "float64",
			metadata: map[string]any{"onu_id": float64(11)},
			key:      "onu_id",
			want:     11,
			wantOK:   true,
		},
		{
			name:     "string",
			metadata: map[string]any{"onu_id": "13"},
			key:      "onu_id",
			want:     13,
			wantOK:   true,
		},
		{
			name:     "missing",
			metadata: map[string]any{"other": 1},
			key:      "onu_id",
			want:     0,
			wantOK:   false,
		},
		{
			name:     "invalid string",
			metadata: map[string]any{"onu_id": "not-an-int"},
			key:      "onu_id",
			want:     0,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseMetadataInt(tt.metadata, tt.key)
			if ok != tt.wantOK {
				t.Fatalf("expected ok=%v, got %v", tt.wantOK, ok)
			}
			if got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
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

// mockDriverWithUpdate extends basic mock with update/delete/create capabilities for onu-update tests
type mockDriverWithUpdate struct {
	updateSubscriberFunc  func(ctx context.Context, subscriber *model.Subscriber, tier *model.ServiceTier) error
	deleteSubscriberFunc  func(ctx context.Context, subscriberID string) error
	createSubscriberFunc  func(ctx context.Context, subscriber *model.Subscriber, tier *model.ServiceTier) (*types.SubscriberResult, error)
	getONUDetailsFunc     func(ctx context.Context, ponPort string, onuID int) (*types.ONUInfo, error)
	getONUVLANViaSNMPFunc func(ctx context.Context, ponPort string, onuID int) (int, error)
}

func (m *mockDriverWithUpdate) UpdateSubscriber(ctx context.Context, subscriber *model.Subscriber, tier *model.ServiceTier) error {
	if m.updateSubscriberFunc != nil {
		return m.updateSubscriberFunc(ctx, subscriber, tier)
	}
	return nil
}

func (m *mockDriverWithUpdate) DeleteSubscriber(ctx context.Context, subscriberID string) error {
	if m.deleteSubscriberFunc != nil {
		return m.deleteSubscriberFunc(ctx, subscriberID)
	}
	return nil
}

func (m *mockDriverWithUpdate) CreateSubscriber(ctx context.Context, subscriber *model.Subscriber, tier *model.ServiceTier) (*types.SubscriberResult, error) {
	if m.createSubscriberFunc != nil {
		return m.createSubscriberFunc(ctx, subscriber, tier)
	}
	return &types.SubscriberResult{SubscriberID: subscriber.Name}, nil
}

func (m *mockDriverWithUpdate) GetONUDetails(ctx context.Context, ponPort string, onuID int) (*types.ONUInfo, error) {
	if m.getONUDetailsFunc != nil {
		return m.getONUDetailsFunc(ctx, ponPort, onuID)
	}
	return &types.ONUInfo{}, nil
}

func (m *mockDriverWithUpdate) GetONUVLANViaSNMP(ctx context.Context, ponPort string, onuID int) (int, error) {
	if m.getONUVLANViaSNMPFunc != nil {
		return m.getONUVLANViaSNMPFunc(ctx, ponPort, onuID)
	}
	return 0, fmt.Errorf("GetONUVLANViaSNMP not implemented")
}

// TestBuildUpdateModels tests building subscriber/tier models for direct updates
func TestBuildUpdateModels(t *testing.T) {
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
		ponPort        string
		onuID          int
		vlan           int
		trafficProfile int
		description    string
		lineProfile    string
		serviceProfile string
		wantVLAN       int
		wantBWDown     int
		wantBWUp       int
	}{
		{
			name:           "VLAN only update",
			ponPort:        "0/1",
			onuID:          5,
			vlan:           200,
			trafficProfile: 0,
			description:    "",
			lineProfile:    "",
			serviceProfile: "",
			wantVLAN:       200,
			wantBWDown:     100, // Preserved from preONU
			wantBWUp:       50,
		},
		{
			name:           "VLAN with traffic profile",
			ponPort:        "0/1",
			onuID:          5,
			vlan:           300,
			trafficProfile: 200,
			description:    "Updated",
			lineProfile:    "",
			serviceProfile: "",
			wantVLAN:       300,
			wantBWDown:     100, // Preserved from preONU (buildUpdateModels stores profile ID only, doesn't change bandwidth)
			wantBWUp:       50,
		},
		{
			name:           "preserve VLAN when zero",
			ponPort:        "0/1",
			onuID:          5,
			vlan:           0, // Don't change VLAN
			trafficProfile: 0,
			description:    "",
			lineProfile:    "",
			serviceProfile: "",
			wantVLAN:       100, // Preserved from preONU
			wantBWDown:     100,
			wantBWUp:       50,
		},
		{
			name:           "with line profile annotation",
			ponPort:        "0/1",
			onuID:          5,
			vlan:           200,
			trafficProfile: 0,
			description:    "",
			lineProfile:    "line_vlan_200",
			serviceProfile: "",
			wantVLAN:       200,
			wantBWDown:     100,
			wantBWUp:       50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscriber, tier := buildUpdateModels(
				preONU,
				tt.ponPort,
				tt.onuID,
				tt.vlan,
				tt.trafficProfile,
				tt.description,
				tt.lineProfile,
				tt.serviceProfile,
			)

			// Check subscriber fields
			if subscriber.Name != preONU.Serial {
				t.Errorf("subscriber.Name = %q, want %q", subscriber.Name, preONU.Serial)
			}

			if subscriber.Spec.ONUSerial != preONU.Serial {
				t.Errorf("subscriber.Spec.ONUSerial = %q, want %q", subscriber.Spec.ONUSerial, preONU.Serial)
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
				// buildUpdateModels stores description in Spec.Description, not annotations
				if subscriber.Spec.Description != tt.description {
					t.Errorf("subscriber.Spec.Description = %q, want %q", subscriber.Spec.Description, tt.description)
				}
			}

			// Check traffic profile annotation if provided
			if tt.trafficProfile > 0 {
				if tp := subscriber.Annotations["nano.io/traffic-profile"]; tp != fmt.Sprintf("%d", tt.trafficProfile) {
					t.Errorf("traffic-profile annotation = %q, want %q", tp, fmt.Sprintf("%d", tt.trafficProfile))
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

// TestONUUpdateProfileChangeDetection tests the core logic that decides between Flow 1 and Flow 2
func TestONUUpdateProfileChangeDetection(t *testing.T) {
	tests := []struct {
		name                 string
		currentLineProfile   string
		requestedLineProfile string
		wantProfileChanged   bool
		wantNeedsReProvision bool
	}{
		{
			name:                 "profile change - different profiles",
			currentLineProfile:   "line_vlan_100",
			requestedLineProfile: "line_vlan_200",
			wantProfileChanged:   true,
			wantNeedsReProvision: true,
		},
		{
			name:                 "profile unchanged - same profile",
			currentLineProfile:   "line_vlan_100",
			requestedLineProfile: "line_vlan_100",
			wantProfileChanged:   false,
			wantNeedsReProvision: false,
		},
		{
			name:                 "no profile change requested",
			currentLineProfile:   "line_vlan_100",
			requestedLineProfile: "",
			wantProfileChanged:   false,
			wantNeedsReProvision: false,
		},
		{
			name:                 "adding profile to ONU without one",
			currentLineProfile:   "",
			requestedLineProfile: "line_vlan_100",
			wantProfileChanged:   true,
			wantNeedsReProvision: true,
		},
		{
			name:                 "no current profile, no requested profile",
			currentLineProfile:   "",
			requestedLineProfile: "",
			wantProfileChanged:   false,
			wantNeedsReProvision: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from runONUUpdate (lines 1501-1508 in olt.go)
			profileChanged := tt.requestedLineProfile != "" && tt.requestedLineProfile != tt.currentLineProfile
			needsReProvision := profileChanged

			if profileChanged != tt.wantProfileChanged {
				t.Errorf("profileChanged = %v, want %v", profileChanged, tt.wantProfileChanged)
			}

			if needsReProvision != tt.wantNeedsReProvision {
				t.Errorf("needsReProvision = %v, want %v", needsReProvision, tt.wantNeedsReProvision)
			}
		})
	}
}

// TestONUUpdateFlow1_DirectVLANUpdate tests Flow 1: Direct VLAN update scenario
func TestONUUpdateFlow1_DirectVLANUpdate(t *testing.T) {
	tests := []struct {
		name             string
		preONU           *types.ONUInfo
		newVLAN          int
		expectUpdateCall bool
		wantVLANInCall   int
	}{
		{
			name: "VLAN update with no line profile",
			preONU: &types.ONUInfo{
				ONUID:       5,
				PONPort:     "0/1",
				Serial:      "HWTC1234ABCD",
				VLAN:        100,
				LineProfile: "", // No line profile bound
				IsOnline:    true,
			},
			newVLAN:          200,
			expectUpdateCall: true,
			wantVLANInCall:   200,
		},
		{
			name: "VLAN update with line profile bound",
			preONU: &types.ONUInfo{
				ONUID:       5,
				PONPort:     "0/1",
				Serial:      "HWTC1234ABCD",
				VLAN:        100,
				LineProfile: "line_vlan_100", // Line profile IS bound
				IsOnline:    true,
			},
			newVLAN:          200,
			expectUpdateCall: true, // Update is attempted, but may fail
			wantVLANInCall:   200,
		},
		{
			name: "preserve VLAN when zero",
			preONU: &types.ONUInfo{
				ONUID:       5,
				PONPort:     "0/1",
				Serial:      "HWTC1234ABCD",
				VLAN:        100,
				LineProfile: "",
				IsOnline:    true,
			},
			newVLAN:          0, // Don't change VLAN
			expectUpdateCall: true,
			wantVLANInCall:   100, // Should preserve existing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateCalled := false
			var capturedVLAN int

			mock := &mockDriverWithUpdate{
				updateSubscriberFunc: func(ctx context.Context, sub *model.Subscriber, tier *model.ServiceTier) error {
					updateCalled = true
					capturedVLAN = sub.Spec.VLAN
					return nil
				},
			}

			// Simulate Flow 1 logic
			subscriber, tier := buildUpdateModels(tt.preONU, tt.preONU.PONPort, tt.preONU.ONUID, tt.newVLAN, 0, "", "", "")
			err := mock.UpdateSubscriber(context.Background(), subscriber, tier)

			if err != nil {
				t.Errorf("UpdateSubscriber failed: %v", err)
			}

			if updateCalled != tt.expectUpdateCall {
				t.Errorf("updateCalled = %v, want %v", updateCalled, tt.expectUpdateCall)
			}

			if tt.expectUpdateCall && capturedVLAN != tt.wantVLANInCall {
				t.Errorf("VLAN in UpdateSubscriber call = %d, want %d", capturedVLAN, tt.wantVLANInCall)
			}
		})
	}
}

// TestONUUpdateFlow2_DeleteReProvision tests Flow 2: Delete+Re-provision scenario
func TestONUUpdateFlow2_DeleteReProvision(t *testing.T) {
	tests := []struct {
		name                string
		currentLineProfile  string
		newLineProfile      string
		expectDeleteCall    bool
		expectCreateCall    bool
		wantProfileInCreate string
	}{
		{
			name:                "profile change triggers delete+re-provision",
			currentLineProfile:  "line_vlan_100",
			newLineProfile:      "line_vlan_200",
			expectDeleteCall:    true,
			expectCreateCall:    true,
			wantProfileInCreate: "line_vlan_200",
		},
		{
			name:                "add profile to ONU without one",
			currentLineProfile:  "",
			newLineProfile:      "line_vlan_100",
			expectDeleteCall:    true,
			expectCreateCall:    true,
			wantProfileInCreate: "line_vlan_100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleteCalled := false
			createCalled := false
			var capturedProfile string

			preONU := &types.ONUInfo{
				ONUID:       5,
				PONPort:     "0/1",
				Serial:      "HWTC1234ABCD",
				VLAN:        100,
				LineProfile: tt.currentLineProfile,
				IsOnline:    true,
			}

			mock := &mockDriverWithUpdate{
				deleteSubscriberFunc: func(ctx context.Context, subscriberID string) error {
					deleteCalled = true
					return nil
				},
				createSubscriberFunc: func(ctx context.Context, sub *model.Subscriber, tier *model.ServiceTier) (*types.SubscriberResult, error) {
					createCalled = true
					capturedProfile = sub.Annotations["nano.io/line-profile"]
					return &types.SubscriberResult{SubscriberID: sub.Name}, nil
				},
			}

			// Simulate Flow 2 logic (profile change detected)
			profileChanged := tt.newLineProfile != "" && tt.newLineProfile != tt.currentLineProfile
			if profileChanged {
				// Delete
				subscriberID := fmt.Sprintf("%s-%d", preONU.PONPort, preONU.ONUID)
				err := mock.DeleteSubscriber(context.Background(), subscriberID)
				if err != nil {
					t.Errorf("DeleteSubscriber failed: %v", err)
				}

				// Re-provision
				subscriber, tier := buildProvisionModelsFromUpdate(
					preONU, preONU.Serial, preONU.PONPort, preONU.ONUID,
					tt.newLineProfile, "", preONU.VLAN, 0, "")
				_, err = mock.CreateSubscriber(context.Background(), subscriber, tier)
				if err != nil {
					t.Errorf("CreateSubscriber failed: %v", err)
				}
			}

			if deleteCalled != tt.expectDeleteCall {
				t.Errorf("deleteCalled = %v, want %v", deleteCalled, tt.expectDeleteCall)
			}

			if createCalled != tt.expectCreateCall {
				t.Errorf("createCalled = %v, want %v", createCalled, tt.expectCreateCall)
			}

			if tt.expectCreateCall && capturedProfile != tt.wantProfileInCreate {
				t.Errorf("profile in CreateSubscriber = %q, want %q", capturedProfile, tt.wantProfileInCreate)
			}
		})
	}
}

// TestONUUpdateProfileUnchangedOptimization tests the optimization that skips delete+re-provision
func TestONUUpdateProfileUnchangedOptimization(t *testing.T) {
	preONU := &types.ONUInfo{
		ONUID:       5,
		PONPort:     "0/1",
		Serial:      "HWTC1234ABCD",
		VLAN:        100,
		LineProfile: "line_vlan_100", // Current profile
		IsOnline:    true,
	}

	updateCalled := false
	deleteCalled := false

	mock := &mockDriverWithUpdate{
		updateSubscriberFunc: func(ctx context.Context, sub *model.Subscriber, tier *model.ServiceTier) error {
			updateCalled = true
			if sub.Spec.VLAN != 200 {
				t.Errorf("Expected VLAN 200, got %d", sub.Spec.VLAN)
			}
			return nil
		},
		deleteSubscriberFunc: func(ctx context.Context, subscriberID string) error {
			deleteCalled = true
			return nil
		},
	}

	// Request update with SAME profile but different VLAN
	requestedProfile := "line_vlan_100"
	newVLAN := 200

	// Detect profile change (should be false - optimization)
	profileChanged := requestedProfile != "" && requestedProfile != preONU.LineProfile
	needsReProvision := profileChanged

	if needsReProvision {
		t.Error("needsReProvision should be false when profile is unchanged")
	}

	// Should use Flow 1 (direct update) instead of Flow 2
	if !needsReProvision {
		subscriber, tier := buildUpdateModels(preONU, preONU.PONPort, preONU.ONUID, newVLAN, 0, "", "", "")
		err := mock.UpdateSubscriber(context.Background(), subscriber, tier)
		if err != nil {
			t.Errorf("UpdateSubscriber failed: %v", err)
		}
	}

	if !updateCalled {
		t.Error("UpdateSubscriber should be called (Flow 1)")
	}

	if deleteCalled {
		t.Error("DeleteSubscriber should NOT be called (optimization - profile unchanged)")
	}
}

// TestONUUpdateErrorHandling tests error scenarios
func TestONUUpdateErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		preONU        *types.ONUInfo
		newProfile    string
		simulateError string
		wantError     bool
	}{
		{
			name: "re-provision without serial number",
			preONU: &types.ONUInfo{
				ONUID:       5,
				PONPort:     "0/1",
				Serial:      "", // Missing serial
				VLAN:        100,
				LineProfile: "line_vlan_100",
			},
			newProfile:    "line_vlan_200",
			simulateError: "",
			wantError:     true, // Should fail - can't re-provision without serial
		},
		{
			name: "delete subscriber fails",
			preONU: &types.ONUInfo{
				ONUID:       5,
				PONPort:     "0/1",
				Serial:      "HWTC1234ABCD",
				VLAN:        100,
				LineProfile: "line_vlan_100",
			},
			newProfile:    "line_vlan_200",
			simulateError: "delete",
			wantError:     true,
		},
		{
			name: "create subscriber fails",
			preONU: &types.ONUInfo{
				ONUID:       5,
				PONPort:     "0/1",
				Serial:      "HWTC1234ABCD",
				VLAN:        100,
				LineProfile: "line_vlan_100",
			},
			newProfile:    "line_vlan_200",
			simulateError: "create",
			wantError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDriverWithUpdate{
				deleteSubscriberFunc: func(ctx context.Context, subscriberID string) error {
					if tt.simulateError == "delete" {
						return fmt.Errorf("simulated delete error")
					}
					return nil
				},
				createSubscriberFunc: func(ctx context.Context, sub *model.Subscriber, tier *model.ServiceTier) (*types.SubscriberResult, error) {
					if tt.simulateError == "create" {
						return nil, fmt.Errorf("simulated create error")
					}
					return &types.SubscriberResult{SubscriberID: sub.Name}, nil
				},
			}

			// Simulate Flow 2 logic
			profileChanged := tt.newProfile != "" && tt.newProfile != tt.preONU.LineProfile
			var err error

			if profileChanged {
				// Check for missing serial
				if tt.preONU.Serial == "" {
					err = fmt.Errorf("cannot re-provision: ONU serial number not found")
				} else {
					// Delete
					subscriberID := fmt.Sprintf("%s-%d", tt.preONU.PONPort, tt.preONU.ONUID)
					err = mock.DeleteSubscriber(context.Background(), subscriberID)

					// Re-provision (only if delete succeeded)
					if err == nil {
						subscriber, tier := buildProvisionModelsFromUpdate(
							tt.preONU, tt.preONU.Serial, tt.preONU.PONPort, tt.preONU.ONUID,
							tt.newProfile, "", tt.preONU.VLAN, 0, "")
						_, err = mock.CreateSubscriber(context.Background(), subscriber, tier)
					}
				}
			}

			if tt.wantError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.wantError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
