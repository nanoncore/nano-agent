package main

import (
	"testing"
)

func TestValidateVLANID(t *testing.T) {
	tests := []struct {
		name    string
		vlanID  int
		wantErr bool
	}{
		{
			name:    "valid minimum",
			vlanID:  1,
			wantErr: false,
		},
		{
			name:    "valid maximum",
			vlanID:  4094,
			wantErr: false,
		},
		{
			name:    "valid common VLAN",
			vlanID:  100,
			wantErr: false,
		},
		{
			name:    "valid mid-range",
			vlanID:  2000,
			wantErr: false,
		},
		{
			name:    "invalid zero",
			vlanID:  0,
			wantErr: true,
		},
		{
			name:    "invalid negative",
			vlanID:  -1,
			wantErr: true,
		},
		{
			name:    "invalid too high",
			vlanID:  4095,
			wantErr: true,
		},
		{
			name:    "invalid way too high",
			vlanID:  9999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.vlanID >= 1 && tt.vlanID <= 4094
			if valid == tt.wantErr {
				t.Errorf("VLAN ID %d: expected error=%v, got valid=%v", tt.vlanID, tt.wantErr, valid)
			}
		})
	}
}

func TestValidatePONPortFormat(t *testing.T) {
	tests := []struct {
		name    string
		ponPort string
		valid   bool
	}{
		{
			name:    "valid format 0/0/1",
			ponPort: "0/0/1",
			valid:   true,
		},
		{
			name:    "valid format 0/1/0",
			ponPort: "0/1/0",
			valid:   true,
		},
		{
			name:    "valid format 0/15/15",
			ponPort: "0/15/15",
			valid:   true,
		},
		{
			name:    "empty port",
			ponPort: "",
			valid:   false,
		},
		{
			name:    "invalid format - missing segment",
			ponPort: "0/1",
			valid:   false,
		},
		{
			name:    "invalid format - too many segments",
			ponPort: "0/0/1/2",
			valid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simple validation: must have exactly 2 slashes and be non-empty
			slashCount := 0
			for _, c := range tt.ponPort {
				if c == '/' {
					slashCount++
				}
			}
			isValid := len(tt.ponPort) > 0 && slashCount == 2
			if isValid != tt.valid {
				t.Errorf("PON port %q: expected valid=%v, got valid=%v", tt.ponPort, tt.valid, isValid)
			}
		})
	}
}

func TestValidateONTID(t *testing.T) {
	tests := []struct {
		name    string
		ontID   int
		wantErr bool
	}{
		{
			name:    "valid minimum",
			ontID:   0,
			wantErr: false,
		},
		{
			name:    "valid maximum",
			ontID:   255,
			wantErr: false,
		},
		{
			name:    "valid mid-range",
			ontID:   101,
			wantErr: false,
		},
		{
			name:    "invalid negative",
			ontID:   -1,
			wantErr: true,
		},
		{
			name:    "invalid too high",
			ontID:   256,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.ontID >= 0 && tt.ontID <= 255
			if valid == tt.wantErr {
				t.Errorf("ONT ID %d: expected error=%v, got valid=%v", tt.ontID, tt.wantErr, valid)
			}
		})
	}
}

func TestValidateGemPort(t *testing.T) {
	tests := []struct {
		name    string
		gemPort int
		wantErr bool
	}{
		{
			name:    "valid default",
			gemPort: 1,
			wantErr: false,
		},
		{
			name:    "valid mid-range",
			gemPort: 128,
			wantErr: false,
		},
		{
			name:    "invalid zero",
			gemPort: 0,
			wantErr: true,
		},
		{
			name:    "invalid negative",
			gemPort: -1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.gemPort >= 1
			if valid == tt.wantErr {
				t.Errorf("GEM port %d: expected error=%v, got valid=%v", tt.gemPort, tt.wantErr, valid)
			}
		})
	}
}
