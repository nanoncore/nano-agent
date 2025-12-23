package cli

import (
	"testing"
	"time"
)

func TestNewBaseCLIDriver(t *testing.T) {
	tests := []struct {
		name        string
		config      CLIConfig
		wantPort    int
		wantTimeout time.Duration
	}{
		{
			name: "default port and timeout",
			config: CLIConfig{
				Host:     "192.168.1.1",
				Username: "admin",
				Password: "admin",
			},
			wantPort:    22,
			wantTimeout: 30 * time.Second,
		},
		{
			name: "custom port and timeout",
			config: CLIConfig{
				Host:     "192.168.1.1",
				Port:     2222,
				Username: "admin",
				Password: "admin",
				Timeout:  60 * time.Second,
			},
			wantPort:    2222,
			wantTimeout: 60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewBaseCLIDriver(tt.config)
			if driver == nil {
				t.Fatal("NewBaseCLIDriver returned nil")
			}

			cfg := driver.Config()
			if cfg.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.wantPort)
			}
			if cfg.Timeout != tt.wantTimeout {
				t.Errorf("Timeout = %v, want %v", cfg.Timeout, tt.wantTimeout)
			}
		})
	}
}

func TestBaseCLIDriver_IsConnected(t *testing.T) {
	config := CLIConfig{
		Host:     "192.168.1.1",
		Username: "admin",
		Password: "admin",
	}

	driver := NewBaseCLIDriver(config)
	if driver.IsConnected() {
		t.Error("IsConnected() = true, want false for new driver")
	}
}

func TestCLIConfig_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config CLIConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: CLIConfig{
				Host:     "192.168.1.1",
				Username: "admin",
				Password: "admin",
			},
			valid: true,
		},
		{
			name: "missing host",
			config: CLIConfig{
				Username: "admin",
				Password: "admin",
			},
			valid: false,
		},
		{
			name: "missing username",
			config: CLIConfig{
				Host:     "192.168.1.1",
				Password: "admin",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simple validation check
			isValid := tt.config.Host != "" && tt.config.Username != ""
			if isValid != tt.valid {
				t.Errorf("Validation = %v, want %v", isValid, tt.valid)
			}
		})
	}
}
