package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
	"github.com/nanoncore/nano-southbound/model"
	"github.com/nanoncore/nano-southbound/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// parseOLTStatus Tests
// =============================================================================

func TestParseOLTStatus_Huawei(t *testing.T) {
	tests := []struct {
		name           string
		versionOutput  string
		cpuOutput      string
		memoryOutput   string
		tempOutput     string
		expectedUptime string
		expectedCPU    *float64
		expectedMem    *float64
		expectedTemp   *float64
		expectedVer    string
	}{
		{
			name: "full metrics available",
			versionOutput: `
HUAWEI MA5683T
VERSION : MA5683T V800R021C00
Uptime is 15 days, 23 hours, 45 minutes
`,
			cpuOutput: `
CPU Usage : 25%
`,
			memoryOutput: `
Memory Usage : 45%
`,
			tempOutput: `
Board: 0
Temperature : 42 C
`,
			expectedUptime: "15 days, 23 hours, 45 minutes",
			expectedCPU:    floatPtr(25),
			expectedMem:    floatPtr(45),
			expectedTemp:   floatPtr(42),
			expectedVer:    "MA5683T V800R021C00",
		},
		{
			name: "partial metrics - no temperature",
			versionOutput: `
VERSION : MA5800-X2
Uptime is 5 days
`,
			cpuOutput:      "CPU Utilization: 15.5%",
			memoryOutput:   "Used: 60%",
			tempOutput:     "",
			expectedUptime: "5 days",
			expectedCPU:    floatPtr(15.5),
			expectedMem:    floatPtr(60),
			expectedTemp:   nil,
			expectedVer:    "MA5800-X2",
		},
		{
			name:           "empty outputs",
			versionOutput:  "",
			cpuOutput:      "",
			memoryOutput:   "",
			tempOutput:     "",
			expectedUptime: "",
			expectedCPU:    nil,
			expectedMem:    nil,
			expectedTemp:   nil,
			expectedVer:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := parseOLTStatus("huawei", tt.versionOutput, tt.cpuOutput, tt.memoryOutput, tt.tempOutput)

			if tt.expectedUptime != "" {
				assert.Equal(t, tt.expectedUptime, metrics["uptime"])
			} else {
				_, exists := metrics["uptime"]
				assert.False(t, exists, "uptime should not be set")
			}

			if tt.expectedCPU != nil {
				assert.Equal(t, *tt.expectedCPU, metrics["cpuUsage"])
			} else {
				_, exists := metrics["cpuUsage"]
				assert.False(t, exists, "cpuUsage should not be set")
			}

			if tt.expectedMem != nil {
				assert.Equal(t, *tt.expectedMem, metrics["memoryUsage"])
			} else {
				_, exists := metrics["memoryUsage"]
				assert.False(t, exists, "memoryUsage should not be set")
			}

			if tt.expectedTemp != nil {
				assert.Equal(t, *tt.expectedTemp, metrics["temperature"])
			} else {
				_, exists := metrics["temperature"]
				assert.False(t, exists, "temperature should not be set")
			}

			if tt.expectedVer != "" {
				assert.Equal(t, tt.expectedVer, metrics["version"])
			}
		})
	}
}

func TestParseOLTStatus_VSol(t *testing.T) {
	tests := []struct {
		name           string
		systemOutput   string
		cpuOutput      string
		memoryOutput   string
		expectedUptime string
		expectedCPU    *float64
		expectedMem    *float64
		expectedTemp   *float64
		expectedVer    string
	}{
		{
			name: "full system info",
			systemOutput: `
System Information
------------------
Software Version: V2.0.1
System uptime: 10 days, 5:30:00
CPU: 30%
Memory: 55%
Temperature: 38 C
`,
			cpuOutput:      "",
			memoryOutput:   "",
			expectedUptime: "10 days, 5:30:00",
			expectedCPU:    floatPtr(30),
			expectedMem:    floatPtr(55),
			expectedTemp:   floatPtr(38),
			expectedVer:    "V2.0.1",
		},
		{
			name: "separate cpu/memory commands",
			systemOutput: `
Firmware: 3.1.0
Uptime: 2d 4h 30m
`,
			cpuOutput:      "CPU: 20%",
			memoryOutput:   "Mem: 40%",
			expectedUptime: "2d 4h 30m",
			expectedCPU:    floatPtr(20),
			expectedMem:    floatPtr(40),
			expectedTemp:   nil,
			expectedVer:    "3.1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := parseOLTStatus("vsol", tt.systemOutput, tt.cpuOutput, tt.memoryOutput, "")

			if tt.expectedUptime != "" {
				assert.Equal(t, tt.expectedUptime, metrics["uptime"])
			}

			if tt.expectedCPU != nil {
				assert.Equal(t, *tt.expectedCPU, metrics["cpuUsage"])
			}

			if tt.expectedMem != nil {
				assert.Equal(t, *tt.expectedMem, metrics["memoryUsage"])
			}

			if tt.expectedTemp != nil {
				assert.Equal(t, *tt.expectedTemp, metrics["temperature"])
			}

			if tt.expectedVer != "" {
				assert.Equal(t, tt.expectedVer, metrics["version"])
			}
		})
	}
}

// =============================================================================
// parseOLTAlarms Tests
// =============================================================================

func TestParseOLTAlarms_NoAlarms(t *testing.T) {
	tests := []struct {
		name   string
		vendor string
		output string
	}{
		{
			name:   "huawei no active alarms",
			vendor: "huawei",
			output: "No active alarm.",
		},
		{
			name:   "vsol empty alarm table",
			vendor: "vsol",
			output: "Alarm table is empty",
		},
		{
			name:   "total zero",
			vendor: "huawei",
			output: "Active Alarms\nTotal: 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alarms := parseOLTAlarms(tt.vendor, tt.output)
			assert.Empty(t, alarms, "expected no alarms")
		})
	}
}

func TestParseOLTAlarms_Huawei_TableFormat(t *testing.T) {
	output := `
Active Alarms
ID  Level     Type         Source      Time
1   Critical  LOS          0/0/1       2026-01-25 10:30:00
2   Major     Temperature  Board 0     2026-01-25 09:15:00
3   Minor     FAN_FAIL     Slot 1      2026-01-25 08:00:00
`

	alarms := parseOLTAlarms("huawei", output)

	require.Len(t, alarms, 3)

	assert.Equal(t, 1, alarms[0]["id"])
	assert.Equal(t, "critical", alarms[0]["severity"])
	assert.Equal(t, "LOS", alarms[0]["type"])
	assert.Equal(t, "0/0/1", alarms[0]["source"])
	assert.Equal(t, "2026-01-25T10:30:00Z", alarms[0]["timestamp"])

	assert.Equal(t, 2, alarms[1]["id"])
	assert.Equal(t, "major", alarms[1]["severity"])

	assert.Equal(t, 3, alarms[2]["id"])
	assert.Equal(t, "minor", alarms[2]["severity"])
}

func TestParseOLTAlarms_Huawei_BlockFormat(t *testing.T) {
	output := `
Alarm ID: 1
Alarm Level: Major
Alarm Type: LOS
Source: 0/0/1
Time: 2026-01-25 10:30:00
Message: Loss of signal detected

Alarm ID: 2
Alarm Level: Warning
Alarm Type: HIGH_TEMP
Location: Board 0
Time: 2026-01-25 09:00:00
Description: Temperature above threshold
`

	alarms := parseOLTAlarms("huawei", output)

	require.Len(t, alarms, 2)

	assert.Equal(t, 1, alarms[0]["id"])
	assert.Equal(t, "major", alarms[0]["severity"])
	assert.Equal(t, "LOS", alarms[0]["type"])
	assert.Equal(t, "0/0/1", alarms[0]["source"])
	assert.Equal(t, "Loss of signal detected", alarms[0]["message"])

	assert.Equal(t, 2, alarms[1]["id"])
	assert.Equal(t, "warning", alarms[1]["severity"])
}

func TestParseOLTAlarms_VSol_BlockFormat(t *testing.T) {
	output := `
Active Alarms:

Alarm ID: 1
Severity: Critical
Type: LOS
Source: PON 0/1 ONU 5
Time: 2026-01-25 10:30:00
Message: ONU lost signal

Alarm ID: 2
Severity: Minor
Type: LOW_POWER
Source: PON 0/2 ONU 3
Time: 2026-01-25 11:00:00
`

	alarms := parseOLTAlarms("vsol", output)

	require.Len(t, alarms, 2)

	assert.Equal(t, 1, alarms[0]["id"])
	assert.Equal(t, "critical", alarms[0]["severity"])
	assert.Equal(t, "LOS", alarms[0]["type"])
	assert.Equal(t, "PON 0/1 ONU 5", alarms[0]["source"])
	assert.Equal(t, "2026-01-25T10:30:00Z", alarms[0]["timestamp"])
	assert.Equal(t, "ONU lost signal", alarms[0]["message"])

	assert.Equal(t, 2, alarms[1]["id"])
	assert.Equal(t, "minor", alarms[1]["severity"])
}

func TestParseOLTAlarms_VSol_TableFormat(t *testing.T) {
	output := `
ID | Severity | Type | Source | Time
1 | Critical | LOS | PON 0/1 ONU 5 | 2026-01-25 10:30:00
2 | Major | DYING_GASP | PON 0/2 ONU 1 | 2026-01-25 09:45:00
`

	alarms := parseOLTAlarms("vsol", output)

	require.Len(t, alarms, 2)

	assert.Equal(t, 1, alarms[0]["id"])
	assert.Equal(t, "critical", alarms[0]["severity"])
	assert.Equal(t, "LOS", alarms[0]["type"])

	assert.Equal(t, 2, alarms[1]["id"])
	assert.Equal(t, "major", alarms[1]["severity"])
}

// =============================================================================
// normalizeSeverity Tests
// =============================================================================

func TestNormalizeSeverity(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Critical", "critical"},
		{"CRITICAL", "critical"},
		{"crit", "critical"},
		{"Major", "major"},
		{"MAJ", "major"},
		{"Minor", "minor"},
		{"Warning", "warning"},
		{"WARN", "warning"},
		{"Info", "info"},
		{"information", "info"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeSeverity(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// parseAlarmTimestamp Tests
// =============================================================================

func TestParseAlarmTimestamp(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2026-01-25 10:30:00", "2026-01-25T10:30:00Z"},
		{"2026/01/25 10:30:00", "2026-01-25T10:30:00Z"},
		{"", ""},
		{"invalid", "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseAlarmTimestamp(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// handleOLTStatus Tests
// =============================================================================

func TestHandleOLTStatus(t *testing.T) {
	tests := []struct {
		name          string
		vendor        string
		executeFunc   func(ctx context.Context, cmd string) (string, error)
		mockPorts     []cli.PONPortInfo
		expectedError bool
		checkStatus   func(t *testing.T, status map[string]interface{})
	}{
		{
			name:   "huawei with all metrics",
			vendor: "huawei",
			executeFunc: func(ctx context.Context, cmd string) (string, error) {
				switch {
				case cmd == "display version":
					return "VERSION : MA5683T V800R021\nUptime is 5 days, 10 hours", nil
				case cmd == "display cpu":
					return "CPU Usage : 20%", nil
				case cmd == "display memory":
					return "Memory Usage : 50%", nil
				case cmd == "display temperature all":
					return "Temperature : 40 C", nil
				default:
					return "", nil
				}
			},
			mockPorts: []cli.PONPortInfo{
				{Status: "up", ONUCount: 10},
				{Status: "up", ONUCount: 5},
				{Status: "down", ONUCount: 0},
			},
			checkStatus: func(t *testing.T, status map[string]interface{}) {
				assert.Equal(t, "huawei", status["vendor"])
				assert.Equal(t, 3, status["ponPorts"])
				assert.Equal(t, 2, status["onlinePorts"])
				assert.Equal(t, 15, status["totalONUs"])
				assert.Equal(t, "5 days, 10 hours", status["uptime"])
				assert.Equal(t, float64(20), status["cpuUsage"])
				assert.Equal(t, float64(50), status["memoryUsage"])
				assert.Equal(t, float64(40), status["temperature"])
			},
		},
		{
			name:   "vsol with partial metrics",
			vendor: "vsol",
			executeFunc: func(ctx context.Context, cmd string) (string, error) {
				if cmd == "show system" {
					return "Version: V2.0\nUptime: 3 days", nil
				}
				return "", nil
			},
			mockPorts: []cli.PONPortInfo{
				{Status: "online", ONUCount: 20},
			},
			checkStatus: func(t *testing.T, status map[string]interface{}) {
				assert.Equal(t, "vsol", status["vendor"])
				assert.Equal(t, 1, status["ponPorts"])
				assert.Equal(t, 1, status["onlinePorts"])
				assert.Equal(t, 20, status["totalONUs"])
				assert.Equal(t, "3 days", status["uptime"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCLIDriver{
				vendor:      tt.vendor,
				executeFunc: tt.executeFunc,
				listPONPortsFunc: func(ctx context.Context) ([]cli.PONPortInfo, error) {
					return tt.mockPorts, nil
				},
			}

			executor := newTestExecutor()
			cmd := agent.PendingCommand{
				ID:      "test-olt-status",
				Type:    "olt_status",
				Payload: map[string]interface{}{},
			}

			result, err := executor.handleOLTStatus(context.Background(), mock, cmd)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			status, ok := result["status"].(map[string]interface{})
			require.True(t, ok)

			if tt.checkStatus != nil {
				tt.checkStatus(t, status)
			}
		})
	}
}

// =============================================================================
// handleOLTAlarms Tests
// =============================================================================

func TestHandleOLTAlarms(t *testing.T) {
	tests := []struct {
		name          string
		vendor        string
		alarmOutput   string
		executeError  error
		expectedCount int
		expectedError bool
	}{
		{
			name:   "with active alarms",
			vendor: "huawei",
			alarmOutput: `
ID  Level     Type    Source   Time
1   Critical  LOS     0/0/1    2026-01-25 10:30:00
2   Major     POWER   Board 0  2026-01-25 09:00:00
`,
			expectedCount: 2,
		},
		{
			name:          "no active alarms",
			vendor:        "vsol",
			alarmOutput:   "No active alarm",
			expectedCount: 0,
		},
		{
			name:          "execute error",
			vendor:        "huawei",
			executeError:  errors.New("connection timeout"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCLIDriver{
				vendor: tt.vendor,
				executeFunc: func(ctx context.Context, cmd string) (string, error) {
					if tt.executeError != nil {
						return "", tt.executeError
					}
					return tt.alarmOutput, nil
				},
			}

			executor := newTestExecutor()
			cmd := agent.PendingCommand{
				ID:      "test-olt-alarms",
				Type:    "olt_alarms",
				Payload: map[string]interface{}{},
			}

			result, err := executor.handleOLTAlarms(context.Background(), mock, cmd)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			alarms, ok := result["alarms"].([]map[string]interface{})
			require.True(t, ok)
			assert.Len(t, alarms, tt.expectedCount)
			assert.Equal(t, tt.expectedCount, result["count"])
		})
	}
}

// =============================================================================
// handleOLTHealthCheck Tests
// =============================================================================

func TestHandleOLTHealthCheck(t *testing.T) {
	tests := []struct {
		name            string
		vendor          string
		executeError    error
		mockPorts       []cli.PONPortInfo
		portsError      error
		expectedHealthy bool
		expectedIssues  int
	}{
		{
			name:   "healthy OLT",
			vendor: "huawei",
			mockPorts: []cli.PONPortInfo{
				{Status: "up", AdminStatus: "enable"},
				{Status: "up", AdminStatus: "enable"},
			},
			expectedHealthy: true,
			expectedIssues:  0,
		},
		{
			name:   "unhealthy - ports down",
			vendor: "vsol",
			mockPorts: []cli.PONPortInfo{
				{Status: "up", AdminStatus: "enable"},
				{Status: "down", AdminStatus: "enable"}, // enabled but down
				{Status: "down", AdminStatus: "disable"}, // disabled - not an issue
			},
			expectedHealthy: false,
			expectedIssues:  1,
		},
		{
			name:            "unhealthy - communication failed",
			vendor:          "huawei",
			executeError:    errors.New("connection refused"),
			expectedHealthy: false,
			expectedIssues:  0, // handled differently
		},
		{
			name:            "healthy - can't get port status",
			vendor:          "vsol",
			portsError:      errors.New("timeout"),
			expectedHealthy: false,
			expectedIssues:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCLIDriver{
				vendor: tt.vendor,
				executeFunc: func(ctx context.Context, cmd string) (string, error) {
					if tt.executeError != nil {
						return "", tt.executeError
					}
					return "OK", nil
				},
				listPONPortsFunc: func(ctx context.Context) ([]cli.PONPortInfo, error) {
					if tt.portsError != nil {
						return nil, tt.portsError
					}
					return tt.mockPorts, nil
				},
			}

			executor := newTestExecutor()
			cmd := agent.PendingCommand{
				ID:      "test-health-check",
				Type:    "olt_health_check",
				Payload: map[string]interface{}{},
			}

			result, err := executor.handleOLTHealthCheck(context.Background(), mock, cmd)
			require.NoError(t, err) // Health check doesn't return errors, just unhealthy status

			healthy, ok := result["healthy"].(bool)
			require.True(t, ok)
			assert.Equal(t, tt.expectedHealthy, healthy)

			if !tt.expectedHealthy && tt.executeError == nil {
				issues, ok := result["issues"].([]string)
				require.True(t, ok)
				assert.Len(t, issues, tt.expectedIssues)
			}
		})
	}
}

// =============================================================================
// handleOLTStatusV2 Tests (SNMP-based DriverV2)
// =============================================================================

// mockDriverV2 implements types.DriverV2 for testing
type mockDriverV2 struct {
	getOLTStatusFunc func(ctx context.Context) (*types.OLTStatus, error)
}

func (m *mockDriverV2) Connect(ctx context.Context, config *types.EquipmentConfig) error { return nil }
func (m *mockDriverV2) Disconnect(ctx context.Context) error                             { return nil }
func (m *mockDriverV2) IsConnected() bool                                                { return true }
func (m *mockDriverV2) CreateSubscriber(ctx context.Context, subscriber *model.Subscriber, tier *model.ServiceTier) (*types.SubscriberResult, error) {
	return nil, nil
}
func (m *mockDriverV2) UpdateSubscriber(ctx context.Context, subscriber *model.Subscriber, tier *model.ServiceTier) error {
	return nil
}
func (m *mockDriverV2) DeleteSubscriber(ctx context.Context, subscriberID string) error { return nil }
func (m *mockDriverV2) SuspendSubscriber(ctx context.Context, subscriberID string) error {
	return nil
}
func (m *mockDriverV2) ResumeSubscriber(ctx context.Context, subscriberID string) error { return nil }
func (m *mockDriverV2) GetSubscriberStatus(ctx context.Context, subscriberID string) (*types.SubscriberStatus, error) {
	return nil, nil
}
func (m *mockDriverV2) GetSubscriberStats(ctx context.Context, subscriberID string) (*types.SubscriberStats, error) {
	return nil, nil
}
func (m *mockDriverV2) HealthCheck(ctx context.Context) error { return nil }

// DriverV2 methods
func (m *mockDriverV2) DiscoverONUs(ctx context.Context, ponPorts []string) ([]types.ONUDiscovery, error) {
	return nil, nil
}
func (m *mockDriverV2) GetONUList(ctx context.Context, filter *types.ONUFilter) ([]types.ONUInfo, error) {
	return nil, nil
}
func (m *mockDriverV2) GetONUBySerial(ctx context.Context, serial string) (*types.ONUInfo, error) {
	return nil, nil
}
func (m *mockDriverV2) GetPONPower(ctx context.Context, ponPort string) (*types.PONPowerReading, error) {
	return nil, nil
}
func (m *mockDriverV2) GetONUPower(ctx context.Context, ponPort string, onuID int) (*types.ONUPowerReading, error) {
	return nil, nil
}
func (m *mockDriverV2) GetONUDistance(ctx context.Context, ponPort string, onuID int) (int, error) {
	return 0, nil
}
func (m *mockDriverV2) RestartONU(ctx context.Context, ponPort string, onuID int) error { return nil }
func (m *mockDriverV2) ApplyProfile(ctx context.Context, ponPort string, onuID int, profile *types.ONUProfile) error {
	return nil
}
func (m *mockDriverV2) BulkProvision(ctx context.Context, operations []types.BulkProvisionOp) (*types.BulkResult, error) {
	return nil, nil
}
func (m *mockDriverV2) RunDiagnostics(ctx context.Context, ponPort string, onuID int) (*types.ONUDiagnostics, error) {
	return nil, nil
}
func (m *mockDriverV2) GetAlarms(ctx context.Context) ([]types.OLTAlarm, error) { return nil, nil }
func (m *mockDriverV2) GetOLTStatus(ctx context.Context) (*types.OLTStatus, error) {
	if m.getOLTStatusFunc != nil {
		return m.getOLTStatusFunc(ctx)
	}
	return nil, nil
}
func (m *mockDriverV2) ListPorts(ctx context.Context) ([]*types.PONPortStatus, error) { return nil, nil }
func (m *mockDriverV2) SetPortState(ctx context.Context, port string, enabled bool) error {
	return nil
}
func (m *mockDriverV2) ListVLANs(ctx context.Context) ([]types.VLANInfo, error)      { return nil, nil }
func (m *mockDriverV2) GetVLAN(ctx context.Context, vlanID int) (*types.VLANInfo, error) {
	return nil, nil
}
func (m *mockDriverV2) CreateVLAN(ctx context.Context, req *types.CreateVLANRequest) error { return nil }
func (m *mockDriverV2) DeleteVLAN(ctx context.Context, vlanID int, force bool) error       { return nil }
func (m *mockDriverV2) ListServicePorts(ctx context.Context) ([]types.ServicePort, error) {
	return nil, nil
}
func (m *mockDriverV2) AddServicePort(ctx context.Context, req *types.AddServicePortRequest) error {
	return nil
}
func (m *mockDriverV2) DeleteServicePort(ctx context.Context, ponPort string, ontID int) error {
	return nil
}

func TestHandleOLTStatusV2(t *testing.T) {
	tests := []struct {
		name          string
		statusFunc    func(ctx context.Context) (*types.OLTStatus, error)
		expectedError bool
		checkResult   func(t *testing.T, result map[string]interface{})
	}{
		{
			name: "full status with all metrics",
			statusFunc: func(ctx context.Context) (*types.OLTStatus, error) {
				return &types.OLTStatus{
					Vendor:        "huawei",
					Model:         "ma5800",
					Firmware:      "V800R021C00",
					IsReachable:   true,
					IsHealthy:     true,
					UptimeSeconds: 1296000, // 15 days
					CPUPercent:    18,
					MemoryPercent: 32,
					Temperature:   45,
					ActiveONUs:    74,
					TotalONUs:     83,
					PONPorts: []types.PONPortStatus{
						{Port: "0/0/1", OperState: "up", ONUCount: 40},
						{Port: "0/0/2", OperState: "up", ONUCount: 34},
						{Port: "0/0/3", OperState: "down", ONUCount: 0},
					},
					LastPoll: time.Now(),
					Metadata: map[string]interface{}{
						"sys_descr": "Huawei SmartAX MA5800",
					},
				}, nil
			},
			checkResult: func(t *testing.T, result map[string]interface{}) {
				status, ok := result["status"].(map[string]interface{})
				require.True(t, ok, "result should contain status map")

				assert.Equal(t, "huawei", status["vendor"])
				assert.Equal(t, "ma5800", status["model"])
				assert.Equal(t, "V800R021C00", status["firmware"])
				assert.Equal(t, true, status["isReachable"])
				assert.Equal(t, true, status["isHealthy"])
				assert.Equal(t, int64(1296000), status["uptimeSeconds"])
				assert.Equal(t, float64(18), status["cpuPercent"])
				assert.Equal(t, float64(32), status["memoryPercent"])
				assert.Equal(t, float64(45), status["temperature"])
				assert.Equal(t, 74, status["activeONUs"])
				assert.Equal(t, 83, status["totalONUs"])
				assert.Equal(t, 3, status["ponPorts"])
				assert.Equal(t, 2, status["onlinePorts"])
				assert.Equal(t, "15 days, 0 hours, 0 minutes", status["uptime"])
				assert.Equal(t, "Huawei SmartAX MA5800", status["sys_descr"])
			},
		},
		{
			name: "partial status - no PON ports",
			statusFunc: func(ctx context.Context) (*types.OLTStatus, error) {
				return &types.OLTStatus{
					Vendor:        "vsol",
					Model:         "v1600g",
					IsReachable:   true,
					IsHealthy:     true,
					CPUPercent:    25,
					MemoryPercent: 50,
					ActiveONUs:    0,
					TotalONUs:     0,
					LastPoll:      time.Now(),
				}, nil
			},
			checkResult: func(t *testing.T, result map[string]interface{}) {
				status, ok := result["status"].(map[string]interface{})
				require.True(t, ok, "result should contain status map")

				assert.Equal(t, "vsol", status["vendor"])
				assert.Equal(t, "v1600g", status["model"])
				assert.Equal(t, float64(25), status["cpuPercent"])
				assert.Equal(t, float64(50), status["memoryPercent"])

				// No PON ports, so these shouldn't be set
				_, hasPonPorts := status["ponPorts"]
				assert.False(t, hasPonPorts, "ponPorts should not be set when empty")

				// No uptime, so uptime string shouldn't be set
				_, hasUptime := status["uptime"]
				assert.False(t, hasUptime, "uptime string should not be set when uptimeSeconds is 0")
			},
		},
		{
			name: "status with uptime calculation",
			statusFunc: func(ctx context.Context) (*types.OLTStatus, error) {
				return &types.OLTStatus{
					Vendor:        "huawei",
					Model:         "ma5683t",
					IsReachable:   true,
					IsHealthy:     true,
					UptimeSeconds: 90061, // 1 day, 1 hour, 1 minute, 1 second
					LastPoll:      time.Now(),
				}, nil
			},
			checkResult: func(t *testing.T, result map[string]interface{}) {
				status, ok := result["status"].(map[string]interface{})
				require.True(t, ok)

				assert.Equal(t, "1 days, 1 hours, 1 minutes", status["uptime"])
			},
		},
		{
			name: "driver returns error",
			statusFunc: func(ctx context.Context) (*types.OLTStatus, error) {
				return nil, errors.New("SNMP timeout: no response from OLT")
			},
			expectedError: true,
		},
		{
			name: "online ports counting with different states",
			statusFunc: func(ctx context.Context) (*types.OLTStatus, error) {
				return &types.OLTStatus{
					Vendor:      "huawei",
					Model:       "ma5800",
					IsReachable: true,
					IsHealthy:   true,
					PONPorts: []types.PONPortStatus{
						{Port: "0/0/1", OperState: "up", ONUCount: 10},
						{Port: "0/0/2", OperState: "UP", ONUCount: 20},    // uppercase
						{Port: "0/0/3", OperState: "online", ONUCount: 5}, // "online" state
						{Port: "0/0/4", OperState: "ONLINE", ONUCount: 3}, // uppercase online
						{Port: "0/0/5", OperState: "down", ONUCount: 0},
						{Port: "0/0/6", OperState: "offline", ONUCount: 0},
					},
					LastPoll: time.Now(),
				}, nil
			},
			checkResult: func(t *testing.T, result map[string]interface{}) {
				status, ok := result["status"].(map[string]interface{})
				require.True(t, ok)

				assert.Equal(t, 6, status["ponPorts"])
				assert.Equal(t, 4, status["onlinePorts"]) // up, UP, online, ONLINE
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDriverV2{
				getOLTStatusFunc: tt.statusFunc,
			}

			executor := newTestExecutor()
			cmd := agent.PendingCommand{
				ID:      "test-olt-status-v2",
				Type:    "olt_status",
				Payload: map[string]interface{}{},
			}

			result, err := executor.handleOLTStatusV2(context.Background(), mock, cmd)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to get OLT status")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func floatPtr(f float64) *float64 {
	return &f
}
