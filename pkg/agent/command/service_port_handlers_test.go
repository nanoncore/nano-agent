package command

import (
	"context"
	"errors"
	"testing"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseServicePorts(t *testing.T) {
	tests := []struct {
		name           string
		vendor         string
		output         string
		expectedCount  int
		expectedFields map[string]interface{}
	}{
		{
			name:   "vsol format single port",
			vendor: "vsol",
			output: "1    100   -      0/1  1    1    eth",
			expectedCount: 1,
			expectedFields: map[string]interface{}{
				"id":      1,
				"vlanId":  100,
				"ponPort": "0/1",
				"onuId":   1,
				"gemPort": 1,
				"type":    "eth",
			},
		},
		{
			name:   "vsol format multiple ports",
			vendor: "vsol",
			output: `1    100   -      0/1  1    1    eth
2    200   500    0/1  2    1    eth
3    300   -      0/2  1    2    pppoe`,
			expectedCount: 3,
		},
		{
			name:   "vsol format with svlan",
			vendor: "vsol",
			output: "5    100   200    0/1  3    1    eth",
			expectedCount: 1,
			expectedFields: map[string]interface{}{
				"vlanId": 100,
				"svlan":  200,
			},
		},
		{
			name:   "huawei format",
			vendor: "huawei",
			output: "1      100   0/0/1            1      1",
			expectedCount: 1,
			expectedFields: map[string]interface{}{
				"index":   1,
				"vlanId":  100,
				"ponPort": "0/0/1",
				"onuId":   1,
				"gemPort": 1,
			},
		},
		{
			name:          "empty output",
			vendor:        "vsol",
			output:        "",
			expectedCount: 0,
		},
		{
			name:   "output with header lines",
			vendor: "vsol",
			output: `Index  VLAN  SVLAN  PON  ONU  GEM  Type
-----------------------------------------
1    100   -      0/1  1    1    eth`,
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseServicePorts(tt.vendor, tt.output)

			assert.Len(t, result, tt.expectedCount)

			if tt.expectedFields != nil && len(result) > 0 {
				for key, expectedValue := range tt.expectedFields {
					assert.Equal(t, expectedValue, result[0][key], "field %s mismatch", key)
				}
			}
		})
	}
}

func TestVerifyServicePortOperation(t *testing.T) {
	tests := []struct {
		name         string
		preState     map[string]interface{}
		postState    map[string]interface{}
		operation    string
		expectVerify bool
		expectChange string
	}{
		{
			name:         "successful add",
			preState:     map[string]interface{}{"count": 1},
			postState:    map[string]interface{}{"count": 2},
			operation:    "add",
			expectVerify: true,
			expectChange: "service_port_added",
		},
		{
			name:         "failed add - count unchanged",
			preState:     map[string]interface{}{"count": 1},
			postState:    map[string]interface{}{"count": 1},
			operation:    "add",
			expectVerify: false,
		},
		{
			name:         "successful delete",
			preState:     map[string]interface{}{"count": 2},
			postState:    map[string]interface{}{"count": 1},
			operation:    "delete",
			expectVerify: true,
			expectChange: "service_port_removed",
		},
		{
			name:         "failed delete - count unchanged",
			preState:     map[string]interface{}{"count": 2},
			postState:    map[string]interface{}{"count": 2},
			operation:    "delete",
			expectVerify: false,
		},
		{
			name:         "add from zero",
			preState:     map[string]interface{}{"count": 0},
			postState:    map[string]interface{}{"count": 1},
			operation:    "add",
			expectVerify: true,
			expectChange: "service_port_added",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := verifyServicePortOperation(tt.preState, tt.postState, tt.operation)

			assert.Equal(t, tt.expectVerify, result["verified"])

			if tt.expectChange != "" {
				changes := result["changes"].([]string)
				assert.Contains(t, changes, tt.expectChange)
			}
		})
	}
}

func TestHandleServicePortList(t *testing.T) {
	tests := []struct {
		name          string
		vendor        string
		ponPort       string
		onuID         float64
		mockOutput    string
		mockError     error
		expectedCount int
		expectedError bool
	}{
		{
			name:          "vsol list all",
			vendor:        "vsol",
			mockOutput:    "1    100   -      0/1  1    1    eth\n2    200   -      0/1  2    1    eth",
			expectedCount: 2,
		},
		{
			name:          "vsol list filtered by ONU",
			vendor:        "vsol",
			ponPort:       "0/1",
			onuID:         1,
			mockOutput:    "1    100   -      0/1  1    1    eth",
			expectedCount: 1,
		},
		{
			name:          "huawei list all",
			vendor:        "huawei",
			mockOutput:    "1      100   0/0/1            1      1",
			expectedCount: 1,
		},
		{
			name:          "unsupported vendor",
			vendor:        "unknown",
			expectedError: true,
		},
		{
			name:          "execute error",
			vendor:        "vsol",
			mockError:     errors.New("connection failed"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCLIDriver{
				vendor: tt.vendor,
				executeFunc: func(ctx context.Context, cmd string) (string, error) {
					if tt.mockError != nil {
						return "", tt.mockError
					}
					return tt.mockOutput, nil
				},
			}

			executor := newTestExecutor()
			payload := map[string]interface{}{}
			if tt.ponPort != "" {
				payload["ponPort"] = tt.ponPort
			}
			if tt.onuID > 0 {
				payload["onuId"] = tt.onuID
			}

			cmd := agent.PendingCommand{
				ID:      "test-sp-list",
				Type:    "service_port_list",
				Payload: payload,
			}

			result, err := executor.handleServicePortList(context.Background(), mock, cmd)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			servicePorts := result["servicePorts"].([]map[string]interface{})
			assert.Len(t, servicePorts, tt.expectedCount)
			assert.Equal(t, tt.expectedCount, result["total"])
		})
	}
}

func TestHandleServicePortAdd_WithVerification(t *testing.T) {
	tests := []struct {
		name           string
		vendor         string
		vlanId         float64
		ponPort        string
		onuId          float64
		gemPort        float64
		preOutput      string
		postOutput     string
		executeError   error
		expectedError  bool
		expectedVerify bool
	}{
		{
			name:           "successful add vsol",
			vendor:         "vsol",
			vlanId:         100,
			ponPort:        "0/1",
			onuId:          1,
			gemPort:        1,
			preOutput:      "", // No existing service ports
			postOutput:     "1    100   -      0/1  1    1    eth",
			expectedVerify: true,
		},
		{
			name:           "successful add huawei",
			vendor:         "huawei",
			vlanId:         100,
			ponPort:        "0/1",
			onuId:          1,
			gemPort:        1,
			preOutput:      "",
			postOutput:     "1      100   0/0/1            1      1",
			expectedVerify: true,
		},
		{
			name:          "missing vlanId",
			vendor:        "vsol",
			ponPort:       "0/1",
			onuId:         1,
			expectedError: true,
		},
		{
			name:          "missing ponPort",
			vendor:        "vsol",
			vlanId:        100,
			onuId:         1,
			expectedError: true,
		},
		{
			name:          "missing onuId",
			vendor:        "vsol",
			vlanId:        100,
			ponPort:       "0/1",
			expectedError: true,
		},
		{
			name:          "unsupported vendor",
			vendor:        "unknown",
			vlanId:        100,
			ponPort:       "0/1",
			onuId:         1,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &mockCLIDriver{
				vendor: tt.vendor,
				executeFunc: func(ctx context.Context, cmd string) (string, error) {
					if tt.executeError != nil {
						return "", tt.executeError
					}
					callCount++
					// First call is pre-state, second is add command, third is post-state
					if callCount == 1 {
						return tt.preOutput, nil
					} else if callCount == 3 {
						return tt.postOutput, nil
					}
					return "", nil
				},
			}

			executor := newTestExecutor()
			payload := map[string]interface{}{}
			if tt.vlanId > 0 {
				payload["vlanId"] = tt.vlanId
			}
			if tt.ponPort != "" {
				payload["ponPort"] = tt.ponPort
			}
			if tt.onuId > 0 {
				payload["onuId"] = tt.onuId
			}
			if tt.gemPort > 0 {
				payload["gemPort"] = tt.gemPort
			}

			cmd := agent.PendingCommand{
				ID:      "test-sp-add",
				Type:    "service_port_add",
				Payload: payload,
			}

			result, err := executor.handleServicePortAdd(context.Background(), mock, cmd)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, result["success"].(bool))
			assert.Contains(t, result, "preState")
			assert.Contains(t, result, "postState")
			assert.Contains(t, result, "verified")
			assert.Contains(t, result, "changes")
			assert.Equal(t, tt.expectedVerify, result["verified"])

			servicePort := result["servicePort"].(map[string]interface{})
			assert.Equal(t, int(tt.vlanId), servicePort["vlanId"])
			assert.Equal(t, tt.ponPort, servicePort["ponPort"])
			assert.Equal(t, int(tt.onuId), servicePort["onuId"])
		})
	}
}

func TestHandleServicePortDelete_WithVerification(t *testing.T) {
	tests := []struct {
		name           string
		vendor         string
		ponPort        string
		onuId          float64
		index          float64
		preOutput      string
		postOutput     string
		executeError   error
		expectedError  bool
		expectedVerify bool
	}{
		{
			name:           "successful delete vsol",
			vendor:         "vsol",
			ponPort:        "0/1",
			onuId:          1,
			preOutput:      "1    100   -      0/1  1    1    eth",
			postOutput:     "", // Removed
			expectedVerify: true,
		},
		{
			name:           "successful delete huawei by index",
			vendor:         "huawei",
			ponPort:        "0/1",
			onuId:          1,
			index:          1,
			preOutput:      "1      100   0/0/1            1      1",
			postOutput:     "",
			expectedVerify: true,
		},
		{
			name:          "missing ponPort",
			vendor:        "vsol",
			onuId:         1,
			expectedError: true,
		},
		{
			name:          "missing onuId",
			vendor:        "vsol",
			ponPort:       "0/1",
			expectedError: true,
		},
		{
			name:          "unsupported vendor",
			vendor:        "unknown",
			ponPort:       "0/1",
			onuId:         1,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &mockCLIDriver{
				vendor: tt.vendor,
				executeFunc: func(ctx context.Context, cmd string) (string, error) {
					if tt.executeError != nil {
						return "", tt.executeError
					}
					callCount++
					// First call is pre-state, second is delete command, third is post-state
					if callCount == 1 {
						return tt.preOutput, nil
					} else if callCount == 3 {
						return tt.postOutput, nil
					}
					return "", nil
				},
			}

			executor := newTestExecutor()
			payload := map[string]interface{}{}
			if tt.ponPort != "" {
				payload["ponPort"] = tt.ponPort
			}
			if tt.onuId > 0 {
				payload["onuId"] = tt.onuId
			}
			if tt.index > 0 {
				payload["index"] = tt.index
			}

			cmd := agent.PendingCommand{
				ID:      "test-sp-delete",
				Type:    "service_port_delete",
				Payload: payload,
			}

			result, err := executor.handleServicePortDelete(context.Background(), mock, cmd)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, result["success"].(bool))
			assert.Contains(t, result, "preState")
			assert.Contains(t, result, "postState")
			assert.Contains(t, result, "verified")
			assert.Contains(t, result, "changes")
			assert.Equal(t, tt.expectedVerify, result["verified"])

			deleted := result["deleted"].(map[string]interface{})
			assert.Equal(t, tt.ponPort, deleted["ponPort"])
			assert.Equal(t, int(tt.onuId), deleted["onuId"])
		})
	}
}

func TestServicePortHandlers_NoRawOutput(t *testing.T) {
	// Verify that service port list no longer returns rawOutput
	mock := &mockCLIDriver{
		vendor: "vsol",
		executeFunc: func(ctx context.Context, cmd string) (string, error) {
			return "1    100   -      0/1  1    1    eth", nil
		},
	}

	executor := newTestExecutor()
	cmd := agent.PendingCommand{
		ID:      "test-no-raw",
		Type:    "service_port_list",
		Payload: map[string]interface{}{},
	}

	result, err := executor.handleServicePortList(context.Background(), mock, cmd)

	require.NoError(t, err)
	assert.NotContains(t, result, "rawOutput", "service port list should not return rawOutput")
	assert.Contains(t, result, "servicePorts", "service port list should return parsed servicePorts")
	assert.Contains(t, result, "total", "service port list should return total count")
}
