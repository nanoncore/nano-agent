package huawei

import (
	"testing"

	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
)

func TestNewHuaweiCLIDriver(t *testing.T) {
	config := cli.CLIConfig{
		Host:     "192.168.1.1",
		Username: "admin",
		Password: "admin",
	}

	driver := NewHuaweiCLIDriver(config)
	if driver == nil {
		t.Fatal("NewHuaweiCLIDriver returned nil")
	}

	if driver.Vendor() != "huawei" {
		t.Errorf("Vendor() = %s, want huawei", driver.Vendor())
	}
}

func TestParseHuaweiONTInfo(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   *cli.ONUCLIInfo
	}{
		{
			name: "standard output",
			output: `
ONT Information:
  SN                 : HWTC12345678
  Description        : Customer-A
  Run state          : online
  Distance(m)        : 1500
  ONU RX optical power(dBm) : -22.5
  Line profile id    : 10
  Service profile id : 20
`,
			want: &cli.ONUCLIInfo{
				SerialNumber:   "HWTC12345678",
				Description:    "Customer-A",
				Status:         "online",
				Distance:       1500,
				RxPower:        -22.5,
				LineProfile:    "10",
				ServiceProfile: "20",
			},
		},
		{
			name: "offline ONU with down cause",
			output: `
ONT Information:
  SN                 : HWTC87654321
  Run state          : offline
  Distance           : 0
  Last down cause    : LOS
`,
			want: &cli.ONUCLIInfo{
				SerialNumber:  "HWTC87654321",
				Status:        "offline",
				Distance:      0,
				OfflineReason: "LOS",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHuaweiONTInfo(tt.output, "0/1/1", 1)
			if err != nil {
				t.Fatalf("parseHuaweiONTInfo() error = %v", err)
			}

			if got.SerialNumber != tt.want.SerialNumber {
				t.Errorf("SerialNumber = %s, want %s", got.SerialNumber, tt.want.SerialNumber)
			}
			if got.Status != tt.want.Status {
				t.Errorf("Status = %s, want %s", got.Status, tt.want.Status)
			}
			if got.Distance != tt.want.Distance {
				t.Errorf("Distance = %d, want %d", got.Distance, tt.want.Distance)
			}
			if got.LineProfile != tt.want.LineProfile {
				t.Errorf("LineProfile = %s, want %s", got.LineProfile, tt.want.LineProfile)
			}
			if got.ServiceProfile != tt.want.ServiceProfile {
				t.Errorf("ServiceProfile = %s, want %s", got.ServiceProfile, tt.want.ServiceProfile)
			}
		})
	}
}

func TestEvaluateOpticalPowerStatus(t *testing.T) {
	// Based on the actual implementation thresholds:
	// < -30.0 → critical
	// < -27.0 → warning
	// > -5.0 → critical
	// > -8.0 → warning
	// otherwise → normal
	tests := []struct {
		name       string
		power      float64
		wantStatus string
	}{
		{
			name:       "normal power in range",
			power:      -20.0,
			wantStatus: "normal",
		},
		{
			name:       "warning low power",
			power:      -28.0,
			wantStatus: "warning",
		},
		{
			name:       "critical low power",
			power:      -31.0,
			wantStatus: "critical",
		},
		{
			name:       "warning high power",
			power:      -7.0,
			wantStatus: "warning",
		},
		{
			name:       "critical high power",
			power:      -4.0,
			wantStatus: "critical",
		},
		{
			name:       "boundary normal",
			power:      -27.0,
			wantStatus: "normal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateOpticalPowerStatus(tt.power)
			if got != tt.wantStatus {
				t.Errorf("evaluateOpticalPowerStatus(%f) = %s, want %s", tt.power, got, tt.wantStatus)
			}
		})
	}
}

func TestCLIConfig(t *testing.T) {
	config := cli.CLIConfig{
		Host:     "192.168.1.1",
		Port:     22,
		Username: "admin",
		Password: "admin",
		Vendor:   "huawei",
	}

	driver := NewHuaweiCLIDriver(config)

	// Test that we can access the config
	cfg := driver.Config()
	if cfg.Host != "192.168.1.1" {
		t.Errorf("Config().Host = %s, want 192.168.1.1", cfg.Host)
	}
	if cfg.Vendor != "huawei" {
		t.Errorf("Config().Vendor = %s, want huawei", cfg.Vendor)
	}
}

func TestDriverNotConnected(t *testing.T) {
	config := cli.CLIConfig{
		Host:     "192.168.1.1",
		Username: "admin",
		Password: "admin",
	}

	driver := NewHuaweiCLIDriver(config)

	// Should not be connected initially
	if driver.IsConnected() {
		t.Error("IsConnected() = true, want false for new driver")
	}
}
