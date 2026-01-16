package main

import (
	"testing"
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
		"HWT12345678",      // only 3 letters
		"HWTC1234567",      // only 7 hex digits
		"HWTC123456789",    // 9 hex digits
		"1234ABCDEFGH",     // starts with numbers
		"HWTC1234567G",     // invalid hex digit 'G'
		"HWTC 2345678",     // space
		"HWTC-12345678",    // hyphen
	}

	for _, serial := range invalidSerials {
		if serialNumberRegex.MatchString(serial) {
			t.Errorf("serialNumberRegex should not match invalid serial %q", serial)
		}
	}
}
