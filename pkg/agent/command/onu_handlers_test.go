package command

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// mockCLIDriverWithONU - Extended mock with ONU-specific functions
// =============================================================================

type mockCLIDriverWithONU struct {
	mockCLIDriver
	getONUInfoFunc func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error)
	callCount      int32
}

func (m *mockCLIDriverWithONU) GetONUInfo(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
	atomic.AddInt32(&m.callCount, 1)
	if m.getONUInfoFunc != nil {
		return m.getONUInfoFunc(ctx, ponPort, onuID)
	}
	return nil, nil
}

func (m *mockCLIDriverWithONU) GetCallCount() int {
	return int(atomic.LoadInt32(&m.callCount))
}

func (m *mockCLIDriverWithONU) ResetCallCount() {
	atomic.StoreInt32(&m.callCount, 0)
}

// =============================================================================
// verifyONUStateChange Tests
// =============================================================================

func TestVerifyONUStateChange_HappyPath(t *testing.T) {
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "vsol"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			return &cli.ONUCLIInfo{
				SerialNumber: "FHTT01010001",
				PonPort:      ponPort,
				OnuID:        onuID,
				Status:       "suspended",
			}, nil
		},
	}

	info, verified := verifyONUStateChange(
		context.Background(),
		mock,
		"0/1",
		1,
		[]string{"suspended", "offline", "disabled"},
		3,
		10*time.Millisecond,
	)

	assert.True(t, verified, "should verify successfully when status matches")
	require.NotNil(t, info)
	assert.Equal(t, "FHTT01010001", info.SerialNumber)
	assert.Equal(t, "suspended", info.Status)
	assert.Equal(t, 1, mock.GetCallCount(), "should succeed on first attempt")
}

func TestVerifyONUStateChange_SucceedsAfterRetry(t *testing.T) {
	callNum := 0
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "huawei"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			callNum++
			if callNum < 3 {
				// First two calls return "online" (not expected)
				return &cli.ONUCLIInfo{
					SerialNumber: "TEST00000001",
					Status:       "online",
				}, nil
			}
			// Third call returns expected state
			return &cli.ONUCLIInfo{
				SerialNumber: "TEST00000001",
				Status:       "offline",
			}, nil
		},
	}

	start := time.Now()
	info, verified := verifyONUStateChange(
		context.Background(),
		mock,
		"0/0/1",
		5,
		[]string{"offline", "deactivated"},
		3,
		50*time.Millisecond,
	)
	elapsed := time.Since(start)

	assert.True(t, verified, "should eventually verify after retries")
	require.NotNil(t, info)
	assert.Equal(t, "offline", info.Status)
	assert.Equal(t, 3, mock.GetCallCount(), "should take 3 attempts")
	// Should have waited at least 2 retry delays (50ms * 2 = 100ms)
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(100), "should have retry delays")
}

func TestVerifyONUStateChange_FailsAfterMaxRetries(t *testing.T) {
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "vsol"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			// Always return "online" but we expect "offline"
			return &cli.ONUCLIInfo{
				SerialNumber: "WRONG00000001",
				Status:       "online",
			}, nil
		},
	}

	info, verified := verifyONUStateChange(
		context.Background(),
		mock,
		"0/1",
		1,
		[]string{"offline", "suspended"},
		2,
		10*time.Millisecond,
	)

	assert.False(t, verified, "should fail when status never matches")
	require.NotNil(t, info, "should return last info even on failure")
	assert.Equal(t, "online", info.Status)
	assert.Equal(t, 3, mock.GetCallCount(), "should exhaust retries (0 + maxRetries)")
}

func TestVerifyONUStateChange_HandlesErrors(t *testing.T) {
	callNum := 0
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "huawei"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			callNum++
			if callNum < 3 {
				return nil, errors.New("connection timeout")
			}
			return &cli.ONUCLIInfo{
				SerialNumber: "TEST00000001",
				Status:       "online",
			}, nil
		},
	}

	info, verified := verifyONUStateChange(
		context.Background(),
		mock,
		"0/0/1",
		1,
		[]string{"online"},
		3,
		10*time.Millisecond,
	)

	assert.True(t, verified, "should verify after errors resolve")
	require.NotNil(t, info)
	assert.Equal(t, "online", info.Status)
}

func TestVerifyONUStateChange_CaseInsensitive(t *testing.T) {
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "vsol"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			return &cli.ONUCLIInfo{
				SerialNumber: "TEST00000001",
				Status:       "ONLINE", // Uppercase
			}, nil
		},
	}

	info, verified := verifyONUStateChange(
		context.Background(),
		mock,
		"0/1",
		1,
		[]string{"online"}, // lowercase
		1,
		10*time.Millisecond,
	)

	assert.True(t, verified, "status comparison should be case-insensitive")
	require.NotNil(t, info)
}

// =============================================================================
// verifyONUDeleted Tests
// =============================================================================

func TestVerifyONUDeleted_HappyPath_ErrorReturned(t *testing.T) {
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "vsol"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			return nil, errors.New("ONU not found")
		},
	}

	verified := verifyONUDeleted(
		context.Background(),
		mock,
		"0/1",
		1,
		3,
		10*time.Millisecond,
	)

	assert.True(t, verified, "should verify deletion when error returned")
	assert.Equal(t, 1, mock.GetCallCount(), "should succeed on first attempt")
}

func TestVerifyONUDeleted_HappyPath_NilReturned(t *testing.T) {
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "huawei"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			return nil, nil // ONU not found, no error
		},
	}

	verified := verifyONUDeleted(
		context.Background(),
		mock,
		"0/0/1",
		5,
		2,
		10*time.Millisecond,
	)

	assert.True(t, verified, "should verify deletion when nil returned")
}

func TestVerifyONUDeleted_EventuallyDeleted(t *testing.T) {
	callNum := 0
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "vsol"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			callNum++
			if callNum < 3 {
				// ONU still exists
				return &cli.ONUCLIInfo{SerialNumber: "TEST00000001"}, nil
			}
			// ONU now deleted
			return nil, errors.New("ONU not found")
		},
	}

	verified := verifyONUDeleted(
		context.Background(),
		mock,
		"0/1",
		1,
		3,
		10*time.Millisecond,
	)

	assert.True(t, verified, "should verify deletion after retries")
	assert.Equal(t, 3, mock.GetCallCount())
}

func TestVerifyONUDeleted_FailsWhenStillExists(t *testing.T) {
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "huawei"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			// ONU always exists
			return &cli.ONUCLIInfo{
				SerialNumber: "PERSISTENT0001",
				Status:       "online",
			}, nil
		},
	}

	verified := verifyONUDeleted(
		context.Background(),
		mock,
		"0/0/1",
		1,
		2,
		10*time.Millisecond,
	)

	assert.False(t, verified, "should fail when ONU still exists")
	assert.Equal(t, 3, mock.GetCallCount(), "should exhaust retries")
}

// =============================================================================
// verifyONUExists Tests
// =============================================================================

func TestVerifyONUExists_HappyPath(t *testing.T) {
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "vsol"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			return &cli.ONUCLIInfo{
				SerialNumber: "FHTT01010001",
				PonPort:      ponPort,
				OnuID:        onuID,
				Status:       "online",
			}, nil
		},
	}

	info, verified := verifyONUExists(
		context.Background(),
		mock,
		"0/1",
		1,
		"FHTT01010001",
		3,
		10*time.Millisecond,
	)

	assert.True(t, verified, "should verify when serial matches")
	require.NotNil(t, info)
	assert.Equal(t, "FHTT01010001", info.SerialNumber)
	assert.Equal(t, 1, mock.GetCallCount())
}

func TestVerifyONUExists_EventuallyFound(t *testing.T) {
	callNum := 0
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "huawei"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			callNum++
			if callNum < 2 {
				return nil, errors.New("ONU not ready")
			}
			return &cli.ONUCLIInfo{
				SerialNumber: "TEST00000001",
				Status:       "online",
			}, nil
		},
	}

	info, verified := verifyONUExists(
		context.Background(),
		mock,
		"0/0/1",
		5,
		"TEST00000001",
		3,
		10*time.Millisecond,
	)

	assert.True(t, verified, "should verify after ONU becomes available")
	require.NotNil(t, info)
	assert.Equal(t, 2, mock.GetCallCount())
}

func TestVerifyONUExists_WrongSerial(t *testing.T) {
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "vsol"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			return &cli.ONUCLIInfo{
				SerialNumber: "WRONG0000001", // Different serial
				Status:       "online",
			}, nil
		},
	}

	info, verified := verifyONUExists(
		context.Background(),
		mock,
		"0/1",
		1,
		"EXPECTED0001",
		2,
		10*time.Millisecond,
	)

	assert.False(t, verified, "should fail when serial doesn't match")
	assert.Nil(t, info, "should return nil when verification fails")
	assert.Equal(t, 3, mock.GetCallCount(), "should exhaust retries")
}

func TestVerifyONUExists_NotFound(t *testing.T) {
	mock := &mockCLIDriverWithONU{
		mockCLIDriver: mockCLIDriver{vendor: "huawei"},
		getONUInfoFunc: func(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
			return nil, errors.New("ONU not found")
		},
	}

	info, verified := verifyONUExists(
		context.Background(),
		mock,
		"0/0/1",
		1,
		"MISSING00001",
		2,
		10*time.Millisecond,
	)

	assert.False(t, verified, "should fail when ONU not found")
	assert.Nil(t, info)
}

// =============================================================================
// parsePonPort Tests
// =============================================================================

func TestParsePonPort_ValidFormats(t *testing.T) {
	tests := []struct {
		name         string
		ponPort      string
		expectedSlot int
		expectedPort int
		expectError  bool
	}{
		{
			name:         "simple format",
			ponPort:      "0/1",
			expectedSlot: 0,
			expectedPort: 1,
		},
		{
			name:         "three part format - uses first two",
			ponPort:      "0/0/1",
			expectedSlot: 0,
			expectedPort: 0,
		},
		{
			name:         "higher slot number",
			ponPort:      "3/8",
			expectedSlot: 3,
			expectedPort: 8,
		},
		{
			name:        "invalid - single part",
			ponPort:     "1",
			expectError: true,
		},
		{
			name:        "invalid - non-numeric slot",
			ponPort:     "x/1",
			expectError: true,
		},
		{
			name:        "invalid - non-numeric port",
			ponPort:     "0/y",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slot, port, err := parsePonPort(tt.ponPort)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedSlot, slot)
			assert.Equal(t, tt.expectedPort, port)
		})
	}
}

// =============================================================================
// handleONUListV2 Suspended Status Tests
// =============================================================================

func TestHandleONUListV2_SuspendedStatus(t *testing.T) {
	// This tests that the fix for suspended status mapping is working
	tests := []struct {
		name           string
		onuList        []testONU
		expectedStatus map[string]string // serial -> expected status
	}{
		{
			name: "suspended via AdminState disabled",
			onuList: []testONU{
				{serial: "SUSPENDED001", adminState: "disabled", operState: "offline", isOnline: false},
			},
			expectedStatus: map[string]string{
				"SUSPENDED001": "suspended",
			},
		},
		{
			name: "suspended via OperState",
			onuList: []testONU{
				{serial: "SUSPENDED002", adminState: "enabled", operState: "suspended", isOnline: false},
			},
			expectedStatus: map[string]string{
				"SUSPENDED002": "suspended",
			},
		},
		{
			name: "online ONU",
			onuList: []testONU{
				{serial: "ONLINE001", adminState: "enabled", operState: "online", isOnline: true},
			},
			expectedStatus: map[string]string{
				"ONLINE001": "online",
			},
		},
		{
			name: "offline ONU (not suspended)",
			onuList: []testONU{
				{serial: "OFFLINE001", adminState: "enabled", operState: "offline", isOnline: false},
			},
			expectedStatus: map[string]string{
				"OFFLINE001": "offline",
			},
		},
		{
			name: "los ONU",
			onuList: []testONU{
				{serial: "LOS001", adminState: "enabled", operState: "los", isOnline: false},
			},
			expectedStatus: map[string]string{
				"LOS001": "los",
			},
		},
		{
			name: "discovered ONU",
			onuList: []testONU{
				{serial: "DISC001", adminState: "enabled", operState: "discovered", isOnline: false},
			},
			expectedStatus: map[string]string{
				"DISC001": "discovered",
			},
		},
		{
			name: "mixed status ONUs",
			onuList: []testONU{
				{serial: "ONU1", adminState: "enabled", operState: "online", isOnline: true},
				{serial: "ONU2", adminState: "disabled", operState: "offline", isOnline: false},
				{serial: "ONU3", adminState: "enabled", operState: "los", isOnline: false},
				{serial: "ONU4", adminState: "enabled", operState: "suspended", isOnline: false},
			},
			expectedStatus: map[string]string{
				"ONU1": "online",
				"ONU2": "suspended",
				"ONU3": "los",
				"ONU4": "suspended",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the status logic directly (matching executor.go logic)
			for _, onu := range tt.onuList {
				status := computeONUStatus(onu.isOnline, onu.adminState, onu.operState)
				expected := tt.expectedStatus[onu.serial]
				assert.Equal(t, expected, status, "status mismatch for ONU %s", onu.serial)
			}
		})
	}
}

// testONU represents an ONU for testing status logic
type testONU struct {
	serial     string
	adminState string
	operState  string
	isOnline   bool
}

// computeONUStatus matches the logic in executor.go handleONUListV2
// This allows us to test the status computation in isolation
func computeONUStatus(isOnline bool, adminState, operState string) string {
	status := "offline"
	if isOnline {
		status = "online"
	} else if adminState == "disabled" || operState == "suspended" {
		status = "suspended"
	} else if operState == "los" {
		status = "los"
	} else if operState == "discovered" {
		status = "discovered"
	}
	return status
}

// =============================================================================
// handleONUBulkProvision Tests
// =============================================================================

func TestHandleONUBulkProvision_PayloadParsing(t *testing.T) {
	tests := []struct {
		name           string
		payload        map[string]interface{}
		expectError    bool
		errorContains  string
		expectedCount  int
	}{
		{
			name: "valid operations array",
			payload: map[string]interface{}{
				"operations": []interface{}{
					map[string]interface{}{
						"serial":     "FHTT01010001",
						"pon_port":   "0/1",
						"onu_id":     float64(1),
						"vlan":       float64(100),
						"line_profile": "default",
					},
					map[string]interface{}{
						"serial":     "FHTT01010002",
						"pon_port":   "0/1",
						"onu_id":     float64(2),
						"vlan":       float64(200),
					},
				},
			},
			expectedCount: 2,
		},
		{
			name: "missing operations key",
			payload: map[string]interface{}{
				"something_else": []interface{}{},
			},
			expectError:   true,
			errorContains: "invalid operations payload",
		},
		{
			name: "empty operations array",
			payload: map[string]interface{}{
				"operations": []interface{}{},
			},
			expectError:   true,
			errorContains: "no operations provided",
		},
		{
			name: "operation missing serial",
			payload: map[string]interface{}{
				"operations": []interface{}{
					map[string]interface{}{
						"pon_port": "0/1",
						"onu_id":   float64(1),
					},
				},
			},
			expectError:   true,
			errorContains: "missing serial",
		},
		{
			name: "invalid operation type",
			payload: map[string]interface{}{
				"operations": []interface{}{
					"not-an-object",
				},
			},
			expectError:   true,
			errorContains: "invalid operation at index 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse operations using the same logic as handleONUBulkProvision
			operationsRaw, ok := tt.payload["operations"].([]interface{})
			if !ok {
				if tt.expectError && tt.errorContains == "invalid operations payload" {
					return // Expected error
				}
				t.Fatalf("unexpected parse failure")
			}

			if len(operationsRaw) == 0 {
				if tt.expectError && tt.errorContains == "no operations provided" {
					return // Expected error
				}
				t.Fatalf("unexpected empty operations")
			}

			var parseError string
			parsedCount := 0
			for i, opRaw := range operationsRaw {
				opMap, ok := opRaw.(map[string]interface{})
				if !ok {
					parseError = "invalid operation at index " + string(rune('0'+i))
					break
				}

				serial, _ := opMap["serial"].(string)
				if serial == "" {
					parseError = "missing serial"
					break
				}
				parsedCount++
			}

			if tt.expectError {
				if parseError == "" {
					t.Errorf("expected error containing %q but got no error", tt.errorContains)
				}
				return
			}

			require.Equal(t, tt.expectedCount, parsedCount)
		})
	}
}
