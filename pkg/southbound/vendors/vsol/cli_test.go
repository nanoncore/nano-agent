package vsol

import (
	"testing"

	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
)

func TestNewVSOLCLIDriver(t *testing.T) {
	config := cli.CLIConfig{
		Host:     "192.168.1.1",
		Username: "admin",
		Password: "admin",
	}

	driver := NewVSOLCLIDriver(config)
	if driver == nil {
		t.Fatal("NewVSOLCLIDriver returned nil")
	}

	if driver.Vendor() != "vsol" {
		t.Errorf("Vendor() = %s, want vsol", driver.Vendor())
	}
}

func TestParseVSOLONUInfo(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   *cli.ONUCLIInfo
	}{
		{
			name: "standard output",
			output: `
ONU Information:
  Serial Number : VSOL12345678
  MAC Address   : aa:bb:cc:dd:ee:ff
  Status        : online
  Type          : V2801
  Distance      : 2000
  RX Power      : -23.5
  Description   : Home-User-1
`,
			want: &cli.ONUCLIInfo{
				SerialNumber: "VSOL12345678",
				MAC:          "aa:bb:cc:dd:ee:ff",
				Status:       "online",
				Type:         "V2801",
				Distance:     2000,
				RxPower:      -23.5,
				Description:  "Home-User-1",
			},
		},
		{
			name: "offline ONU",
			output: `
ONU Information:
  Serial Number : VSOL87654321
  Status        : offline
  Type          : V2802
`,
			want: &cli.ONUCLIInfo{
				SerialNumber: "VSOL87654321",
				Status:       "offline",
				Type:         "V2802",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVSOLONUInfo(tt.output, "0/1", 1)
			if err != nil {
				t.Fatalf("parseVSOLONUInfo() error = %v", err)
			}

			if got.SerialNumber != tt.want.SerialNumber {
				t.Errorf("SerialNumber = %s, want %s", got.SerialNumber, tt.want.SerialNumber)
			}
			if got.Status != tt.want.Status {
				t.Errorf("Status = %s, want %s", got.Status, tt.want.Status)
			}
			if got.Type != tt.want.Type {
				t.Errorf("Type = %s, want %s", got.Type, tt.want.Type)
			}
		})
	}
}

func TestParseVSOLVLANConfig(t *testing.T) {
	output := `
VLAN Configuration:
  PVID         : 100
  Tagged VLANs : 200, 300, 400
  Service VLAN : 1000
  Translations:
    10 -> 100 (translate)
    20 -> 200 (transparent)
`

	config, err := parseVSOLVLANConfig(output, "0/1", 1)
	if err != nil {
		t.Fatalf("parseVSOLVLANConfig() error = %v", err)
	}

	if config.NativeVLAN != 100 {
		t.Errorf("NativeVLAN = %d, want 100", config.NativeVLAN)
	}
	if config.ServiceVLAN != 1000 {
		t.Errorf("ServiceVLAN = %d, want 1000", config.ServiceVLAN)
	}
	if len(config.Translations) != 2 {
		t.Errorf("len(Translations) = %d, want 2", len(config.Translations))
	}
	if config.Translations[0].CustomerVLAN != 10 || config.Translations[0].ServiceVLAN != 100 {
		t.Errorf("Translations[0] = {%d -> %d}, want {10 -> 100}",
			config.Translations[0].CustomerVLAN, config.Translations[0].ServiceVLAN)
	}
}

func TestParseVLANList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []int
	}{
		{
			name:  "comma separated",
			input: "100, 200, 300",
			want:  []int{100, 200, 300},
		},
		{
			name:  "with range",
			input: "100, 200-203, 300",
			want:  []int{100, 200, 201, 202, 203, 300},
		},
		{
			name:  "single value",
			input: "100",
			want:  []int{100},
		},
		{
			name:  "empty",
			input: "",
			want:  []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVLANList(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("len(result) = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("result[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseVSOLVLANList(t *testing.T) {
	output := `
VLAN Configuration:
  VLAN ID    Name                 Description
  -------    ----                 -----------
  1          default              Default VLAN
  100        DATA                 Data VLAN
  200        VOICE                Voice VLAN
`

	vlans, err := parseVSOLVLANList(output)
	if err != nil {
		t.Fatalf("parseVSOLVLANList() error = %v", err)
	}

	if len(vlans) != 3 {
		t.Fatalf("len(vlans) = %d, want 3", len(vlans))
	}

	if vlans[0].ID != 1 || vlans[0].Name != "default" {
		t.Errorf("vlans[0] = {ID: %d, Name: %s}, want {ID: 1, Name: default}", vlans[0].ID, vlans[0].Name)
	}
}

func TestParseVSOLLineProfiles(t *testing.T) {
	output := `
Profile List:
  1    default    gpon
  2    residential    epon
  3    business    gpon
`

	profiles, err := parseVSOLLineProfiles(output)
	if err != nil {
		t.Fatalf("parseVSOLLineProfiles() error = %v", err)
	}

	if len(profiles) != 3 {
		t.Fatalf("len(profiles) = %d, want 3", len(profiles))
	}

	if profiles[0].ID != 1 || profiles[0].Name != "default" || profiles[0].Type != "gpon" {
		t.Errorf("profiles[0] = {ID: %d, Name: %s, Type: %s}, want {ID: 1, Name: default, Type: gpon}",
			profiles[0].ID, profiles[0].Name, profiles[0].Type)
	}
}

func TestParseVSOLServiceProfiles(t *testing.T) {
	output := `
Service Profile List:
  1    default-svc    V2801    4
  2    business-svc   V2802    1
`

	profiles, err := parseVSOLServiceProfiles(output)
	if err != nil {
		t.Fatalf("parseVSOLServiceProfiles() error = %v", err)
	}

	if len(profiles) != 2 {
		t.Fatalf("len(profiles) = %d, want 2", len(profiles))
	}

	if profiles[0].ONUType != "V2801" {
		t.Errorf("profiles[0].ONUType = %s, want V2801", profiles[0].ONUType)
	}
}

func TestParseVSOLTrafficProfiles(t *testing.T) {
	output := `
Bandwidth Profile List:
  1    100Mbps    100000    100000
  2    1Gbps      1000000   1000000
`

	profiles, err := parseVSOLTrafficProfiles(output)
	if err != nil {
		t.Fatalf("parseVSOLTrafficProfiles() error = %v", err)
	}

	if len(profiles) != 2 {
		t.Fatalf("len(profiles) = %d, want 2", len(profiles))
	}

	if profiles[0].CIR != 100000 || profiles[0].PIR != 100000 {
		t.Errorf("profiles[0] = {CIR: %d, PIR: %d}, want {CIR: 100000, PIR: 100000}",
			profiles[0].CIR, profiles[0].PIR)
	}
}

func TestParseVSOLPONPorts(t *testing.T) {
	output := `
Interface Status:
  epon 0/1     enable   up      32      PON-Port-1
  epon 0/2     enable   up      64      PON-Port-2
  epon 0/3     disable  down    0       Unused
`

	ports, err := parseVSOLPONPorts(output)
	if err != nil {
		t.Fatalf("parseVSOLPONPorts() error = %v", err)
	}

	if len(ports) != 3 {
		t.Fatalf("len(ports) = %d, want 3", len(ports))
	}

	if ports[0].Slot != 0 || ports[0].Port != 1 {
		t.Errorf("ports[0] slot/port = %d/%d, want 0/1", ports[0].Slot, ports[0].Port)
	}
	if ports[0].ONUCount != 32 {
		t.Errorf("ports[0].ONUCount = %d, want 32", ports[0].ONUCount)
	}
	if ports[0].Type != "epon" {
		t.Errorf("ports[0].Type = %s, want epon", ports[0].Type)
	}
}

func TestParseVSOLPONPortInfo(t *testing.T) {
	output := `
Interface epon 0/1:
  Admin Status  : enable
  Link Status   : up
  ONU Count     : 32
  Max ONUs      : 64
  TX Power      : 3.5
  Description   : Main-PON-Port
`

	info, err := parseVSOLPONPortInfo(output, 0, 1)
	if err != nil {
		t.Fatalf("parseVSOLPONPortInfo() error = %v", err)
	}

	if info.AdminStatus != "enable" {
		t.Errorf("AdminStatus = %s, want enable", info.AdminStatus)
	}
	if info.Status != "up" {
		t.Errorf("Status = %s, want up", info.Status)
	}
	if info.ONUCount != 32 {
		t.Errorf("ONUCount = %d, want 32", info.ONUCount)
	}
	if info.MaxONUs != 64 {
		t.Errorf("MaxONUs = %d, want 64", info.MaxONUs)
	}
}

func TestParseVSOLOpticalDiag(t *testing.T) {
	output := `
Optical Diagnostics:
  RX Power      : -22.5
  TX Power      : 2.1
  OLT RX Power  : -23.0
  Temperature   : 42.5
  Voltage       : 3.3
  Bias Current  : 22.0
`

	diag, err := parseVSOLOpticalDiag(output)
	if err != nil {
		t.Fatalf("parseVSOLOpticalDiag() error = %v", err)
	}

	if diag.RxPower != -22.5 {
		t.Errorf("RxPower = %f, want -22.5", diag.RxPower)
	}
	if diag.TxPower != 2.1 {
		t.Errorf("TxPower = %f, want 2.1", diag.TxPower)
	}
	if diag.Temperature != 42.5 {
		t.Errorf("Temperature = %f, want 42.5", diag.Temperature)
	}
}

func TestParseVSOLCounters(t *testing.T) {
	output := `
Performance Counters:
  RX Bytes    : 5000000000
  TX Bytes    : 2500000000
  RX Packets  : 4000000
  TX Packets  : 2000000
  RX Errors   : 5
  TX Errors   : 2
  RX Dropped  : 10
  TX Dropped  : 5
  CRC Errors  : 1
  FCS Errors  : 0
`

	counters, err := parseVSOLCounters(output)
	if err != nil {
		t.Fatalf("parseVSOLCounters() error = %v", err)
	}

	if counters.RxBytes != 5000000000 {
		t.Errorf("RxBytes = %d, want 5000000000", counters.RxBytes)
	}
	if counters.TxBytes != 2500000000 {
		t.Errorf("TxBytes = %d, want 2500000000", counters.TxBytes)
	}
	if counters.RxErrors != 5 {
		t.Errorf("RxErrors = %d, want 5", counters.RxErrors)
	}
	if counters.CRCErrors != 1 {
		t.Errorf("CRCErrors = %d, want 1", counters.CRCErrors)
	}
}

func TestDetermineOpticalStatus(t *testing.T) {
	tests := []struct {
		name       string
		power      float64
		critical   float64
		warning    float64
		wantStatus string
	}{
		{
			name:       "normal power",
			power:      -20.0,
			critical:   -28.0,
			warning:    -25.0,
			wantStatus: "normal",
		},
		{
			name:       "warning power",
			power:      -26.0,
			critical:   -28.0,
			warning:    -25.0,
			wantStatus: "warning",
		},
		{
			name:       "critical power",
			power:      -29.0,
			critical:   -28.0,
			warning:    -25.0,
			wantStatus: "critical",
		},
		{
			name:       "zero power",
			power:      0,
			critical:   -28.0,
			warning:    -25.0,
			wantStatus: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineOpticalStatus(tt.power, tt.critical, tt.warning)
			if got != tt.wantStatus {
				t.Errorf("determineOpticalStatus(%f) = %s, want %s", tt.power, got, tt.wantStatus)
			}
		})
	}
}
