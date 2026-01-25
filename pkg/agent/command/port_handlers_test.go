package command

import (
	"context"
	"errors"
	"testing"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCLIDriver is a configurable mock driver for testing
type mockCLIDriver struct {
	vendor               string
	executeFunc          func(ctx context.Context, cmd string) (string, error)
	listPONPortsFunc     func(ctx context.Context) ([]cli.PONPortInfo, error)
	getPONPortInfoFunc   func(ctx context.Context, slot, port int) (*cli.PONPortInfo, error)
	enablePONPortFunc    func(ctx context.Context, slot, port int) error
	disablePONPortFunc   func(ctx context.Context, slot, port int) error
	capabilities         *cli.VendorCapabilities
}

func (m *mockCLIDriver) Connect(ctx context.Context) error              { return nil }
func (m *mockCLIDriver) Close() error                                   { return nil }
func (m *mockCLIDriver) Vendor() string                                 { return m.vendor }
func (m *mockCLIDriver) GetCapabilities() *cli.VendorCapabilities       { return m.capabilities }
func (m *mockCLIDriver) SaveConfig(ctx context.Context) error           { return nil }

func (m *mockCLIDriver) Execute(ctx context.Context, cmd string) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, cmd)
	}
	return "", nil
}

func (m *mockCLIDriver) ListPONPorts(ctx context.Context) ([]cli.PONPortInfo, error) {
	if m.listPONPortsFunc != nil {
		return m.listPONPortsFunc(ctx)
	}
	return nil, nil
}

func (m *mockCLIDriver) GetPONPortInfo(ctx context.Context, slot, port int) (*cli.PONPortInfo, error) {
	if m.getPONPortInfoFunc != nil {
		return m.getPONPortInfoFunc(ctx, slot, port)
	}
	return nil, nil
}

func (m *mockCLIDriver) EnablePONPort(ctx context.Context, slot, port int) error {
	if m.enablePONPortFunc != nil {
		return m.enablePONPortFunc(ctx, slot, port)
	}
	return nil
}

func (m *mockCLIDriver) DisablePONPort(ctx context.Context, slot, port int) error {
	if m.disablePONPortFunc != nil {
		return m.disablePONPortFunc(ctx, slot, port)
	}
	return nil
}

// Stub implementations for other CLIDriver methods
func (m *mockCLIDriver) AddONU(ctx context.Context, req *cli.ONUProvisionRequest) error     { return nil }
func (m *mockCLIDriver) DeleteONU(ctx context.Context, ponPort string, onuID int) error     { return nil }
func (m *mockCLIDriver) GetONUInfo(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
	return nil, nil
}
func (m *mockCLIDriver) RebootONU(ctx context.Context, ponPort string, onuID int) error { return nil }
func (m *mockCLIDriver) ConfigureVLAN(ctx context.Context, config *cli.VLANConfig) error { return nil }
func (m *mockCLIDriver) GetVLANConfig(ctx context.Context, ponPort string, onuID int) (*cli.VLANConfig, error) {
	return nil, nil
}
func (m *mockCLIDriver) AddVLANTranslation(ctx context.Context, ponPort string, onuID int, translation cli.VLANTranslation) error {
	return nil
}
func (m *mockCLIDriver) RemoveVLANTranslation(ctx context.Context, ponPort string, onuID int, customerVLAN int) error {
	return nil
}
func (m *mockCLIDriver) ListVLANs(ctx context.Context) ([]cli.VLANInfo, error)           { return nil, nil }
func (m *mockCLIDriver) ListLineProfiles(ctx context.Context) ([]cli.LineProfile, error) { return nil, nil }
func (m *mockCLIDriver) GetLineProfile(ctx context.Context, profileID int) (*cli.LineProfile, error) {
	return nil, nil
}
func (m *mockCLIDriver) ListServiceProfiles(ctx context.Context) ([]cli.ServiceProfile, error) {
	return nil, nil
}
func (m *mockCLIDriver) GetServiceProfile(ctx context.Context, profileID int) (*cli.ServiceProfile, error) {
	return nil, nil
}
func (m *mockCLIDriver) ListTrafficProfiles(ctx context.Context) ([]cli.TrafficProfile, error) {
	return nil, nil
}
func (m *mockCLIDriver) AssignTrafficProfile(ctx context.Context, ponPort string, onuID int, profileID int) error {
	return nil
}
func (m *mockCLIDriver) SetPortDescription(ctx context.Context, slot, port int, description string) error {
	return nil
}
func (m *mockCLIDriver) BatchProvision(ctx context.Context, req *cli.BatchProvisionRequest) (*cli.BatchResult, error) {
	return nil, nil
}
func (m *mockCLIDriver) BatchConfigureVLAN(ctx context.Context, req *cli.BatchVLANRequest) (*cli.BatchResult, error) {
	return nil, nil
}
func (m *mockCLIDriver) ExportConfig(ctx context.Context) (*cli.ConfigExport, error) { return nil, nil }
func (m *mockCLIDriver) GetONUDiagnostics(ctx context.Context, ponPort string, onuID int) (*cli.ONUDiagnostics, error) {
	return nil, nil
}
func (m *mockCLIDriver) GetONUCounters(ctx context.Context, ponPort string, onuID int) (*cli.PerformanceCounters, error) {
	return nil, nil
}
func (m *mockCLIDriver) ClearONUCounters(ctx context.Context, ponPort string, onuID int) error {
	return nil
}
func (m *mockCLIDriver) GetOpticalDiagnostics(ctx context.Context, ponPort string, onuID int) (*cli.OpticalDiagnostics, error) {
	return nil, nil
}

// newTestExecutor creates an executor for testing
func newTestExecutor() *Executor {
	return &Executor{
		client:     nil,
		oltConfigs: make(map[string]agent.OLTConfig),
	}
}

func TestHandlePortList(t *testing.T) {
	tests := []struct {
		name           string
		mockPorts      []cli.PONPortInfo
		mockError      error
		expectedPorts  int
		expectedError  bool
	}{
		{
			name: "success with multiple ports",
			mockPorts: []cli.PONPortInfo{
				{Slot: 0, Port: 1, Name: "0/0/1", Status: "up", AdminStatus: "enable", Type: "gpon", ONUCount: 32, MaxONUs: 128, TxPower: -2.5, RxPower: -18.3},
				{Slot: 0, Port: 2, Name: "0/0/2", Status: "up", AdminStatus: "enable", Type: "gpon", ONUCount: 16, MaxONUs: 128, TxPower: -2.6, RxPower: -19.1},
			},
			expectedPorts: 2,
		},
		{
			name:          "empty port list",
			mockPorts:     []cli.PONPortInfo{},
			expectedPorts: 0,
		},
		{
			name:          "driver error",
			mockError:     errors.New("connection failed"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCLIDriver{
				vendor: "vsol",
				listPONPortsFunc: func(ctx context.Context) ([]cli.PONPortInfo, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockPorts, nil
				},
			}

			executor := newTestExecutor()
			cmd := agent.PendingCommand{
				ID:      "test-cmd-1",
				Type:    "port_list",
				Payload: map[string]interface{}{},
			}

			result, err := executor.handlePortList(context.Background(), mock, cmd)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			ports, ok := result["ports"].([]map[string]interface{})
			require.True(t, ok)
			assert.Len(t, ports, tt.expectedPorts)

			// Verify first port has expected fields including RxPower
			if len(ports) > 0 {
				assert.Contains(t, ports[0], "txPower")
				assert.Contains(t, ports[0], "rxPower")
				assert.Equal(t, -2.5, ports[0]["txPower"])
				assert.Equal(t, -18.3, ports[0]["rxPower"])
			}
		})
	}
}

func TestHandlePortPower(t *testing.T) {
	tests := []struct {
		name          string
		port          string
		mockPortInfo  *cli.PONPortInfo
		mockError     error
		expectedError bool
		expectTxPower float64
		expectRxPower float64
	}{
		{
			name: "success with power readings",
			port: "0/1",
			mockPortInfo: &cli.PONPortInfo{
				Slot:        0,
				Port:        1,
				Status:      "up",
				AdminStatus: "enable",
				TxPower:     -2.5,
				RxPower:     -18.3,
				ONUCount:    32,
			},
			expectTxPower: -2.5,
			expectRxPower: -18.3,
		},
		{
			name:          "missing port parameter",
			port:          "",
			expectedError: true,
		},
		{
			name:          "driver error",
			port:          "0/1",
			mockError:     errors.New("port not found"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCLIDriver{
				vendor: "vsol",
				getPONPortInfoFunc: func(ctx context.Context, slot, port int) (*cli.PONPortInfo, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockPortInfo, nil
				},
			}

			executor := newTestExecutor()
			cmd := agent.PendingCommand{
				ID:      "test-cmd-2",
				Type:    "port_power",
				Payload: map[string]interface{}{"port": tt.port},
			}

			result, err := executor.handlePortPower(context.Background(), mock, cmd)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.port, result["port"])

			power, ok := result["power"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, tt.expectTxPower, power["txPower"])
			assert.Equal(t, tt.expectRxPower, power["rxPower"])
		})
	}
}

func TestHandlePortEnable(t *testing.T) {
	tests := []struct {
		name           string
		port           string
		preState       *cli.PONPortInfo
		postState      *cli.PONPortInfo
		enableError    error
		expectedError  bool
		expectedVerify bool
	}{
		{
			name: "successful enable",
			port: "0/1",
			preState: &cli.PONPortInfo{
				Status:      "down",
				AdminStatus: "disable",
			},
			postState: &cli.PONPortInfo{
				Status:      "up",
				AdminStatus: "enable",
			},
			expectedVerify: true,
		},
		{
			name: "enable fails verification",
			port: "0/1",
			preState: &cli.PONPortInfo{
				Status:      "down",
				AdminStatus: "disable",
			},
			postState: &cli.PONPortInfo{
				Status:      "down",
				AdminStatus: "disable", // Still disabled after attempt
			},
			expectedVerify: false,
		},
		{
			name:          "missing port parameter",
			port:          "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &mockCLIDriver{
				vendor: "vsol",
				getPONPortInfoFunc: func(ctx context.Context, slot, port int) (*cli.PONPortInfo, error) {
					callCount++
					if callCount == 1 {
						return tt.preState, nil
					}
					return tt.postState, nil
				},
				enablePONPortFunc: func(ctx context.Context, slot, port int) error {
					return tt.enableError
				},
			}

			executor := newTestExecutor()
			cmd := agent.PendingCommand{
				ID:      "test-cmd-3",
				Type:    "port_enable",
				Payload: map[string]interface{}{"port": tt.port},
			}

			result, err := executor.handlePortEnable(context.Background(), mock, cmd)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, result["success"].(bool))
			assert.Equal(t, tt.expectedVerify, result["verified"])
			assert.Contains(t, result, "preState")
			assert.Contains(t, result, "postState")
		})
	}
}

func TestHandlePortDisable(t *testing.T) {
	tests := []struct {
		name           string
		port           string
		preState       *cli.PONPortInfo
		postState      *cli.PONPortInfo
		disableError   error
		expectedError  bool
		expectedVerify bool
	}{
		{
			name: "successful disable",
			port: "0/1",
			preState: &cli.PONPortInfo{
				Status:      "up",
				AdminStatus: "enable",
				ONUCount:    32,
			},
			postState: &cli.PONPortInfo{
				Status:      "down",
				AdminStatus: "disable",
			},
			expectedVerify: true,
		},
		{
			name:          "invalid port format",
			port:          "invalid",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &mockCLIDriver{
				vendor: "vsol",
				getPONPortInfoFunc: func(ctx context.Context, slot, port int) (*cli.PONPortInfo, error) {
					callCount++
					if callCount == 1 {
						return tt.preState, nil
					}
					return tt.postState, nil
				},
				disablePONPortFunc: func(ctx context.Context, slot, port int) error {
					return tt.disableError
				},
			}

			executor := newTestExecutor()
			cmd := agent.PendingCommand{
				ID:      "test-cmd-4",
				Type:    "port_disable",
				Payload: map[string]interface{}{"port": tt.port},
			}

			result, err := executor.handlePortDisable(context.Background(), mock, cmd)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, result["success"].(bool))
			assert.Equal(t, tt.expectedVerify, result["verified"])
		})
	}
}

func TestParsePonPort(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectSlot  int
		expectPort  int
		expectError bool
	}{
		{
			name:       "valid port 0/1",
			input:      "0/1",
			expectSlot: 0,
			expectPort: 1,
		},
		{
			name:       "valid port 1/8",
			input:      "1/8",
			expectSlot: 1,
			expectPort: 8,
		},
		{
			name:        "invalid single number",
			input:       "1",
			expectError: true,
		},
		{
			name:        "invalid non-numeric slot",
			input:       "a/1",
			expectError: true,
		},
		{
			name:        "invalid non-numeric port",
			input:       "0/b",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slot, port, err := parsePonPort(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectSlot, slot)
			assert.Equal(t, tt.expectPort, port)
		})
	}
}
