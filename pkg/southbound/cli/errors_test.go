package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestUnsupportedOperationError(t *testing.T) {
	tests := []struct {
		name     string
		err      *UnsupportedOperationError
		contains []string
	}{
		{
			name: "full error with reason",
			err: &UnsupportedOperationError{
				Vendor:    "vsol",
				Model:     "V1600D",
				Operation: "CreateDBAProfile",
				Reason:    "VSOL devices require manual configuration via web UI",
			},
			contains: []string{"vsol", "V1600D", "CreateDBAProfile", "web UI"},
		},
		{
			name: "error without model",
			err: &UnsupportedOperationError{
				Vendor:    "cdata",
				Operation: "BatchProvision",
			},
			contains: []string{"cdata", "BatchProvision"},
		},
		{
			name: "error with model but no reason",
			err: &UnsupportedOperationError{
				Vendor:    "huawei",
				Model:     "MA5600T",
				Operation: "GNMISubscribe",
			},
			contains: []string{"huawei", "MA5600T", "GNMISubscribe"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			for _, substr := range tt.contains {
				if !strings.Contains(errMsg, substr) {
					t.Errorf("expected error message to contain %q, got %q", substr, errMsg)
				}
			}
		})
	}
}

func TestIsUnsupportedOperation(t *testing.T) {
	unsupported := NewUnsupportedOperationError("vsol", "V1600D", "Reboot", "Not supported")
	generic := errors.New("generic error")

	if !IsUnsupportedOperation(unsupported) {
		t.Error("expected IsUnsupportedOperation to return true for UnsupportedOperationError")
	}
	if IsUnsupportedOperation(generic) {
		t.Error("expected IsUnsupportedOperation to return false for generic error")
	}
}

func TestConnectionError(t *testing.T) {
	err := NewConnectionError("192.168.1.1", 22, "connection refused", nil)

	if !strings.Contains(err.Error(), "192.168.1.1") {
		t.Error("expected error to contain host")
	}
	if !strings.Contains(err.Error(), "22") {
		t.Error("expected error to contain port")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Error("expected error to contain message")
	}

	// Test with wrapped error
	cause := errors.New("network unreachable")
	errWithCause := NewConnectionError("192.168.1.1", 22, "failed", cause)
	if !errors.Is(errWithCause, cause) {
		t.Error("expected Unwrap to return the cause")
	}
}

func TestIsConnectionError(t *testing.T) {
	connErr := NewConnectionError("192.168.1.1", 22, "failed", nil)
	generic := errors.New("generic error")

	if !IsConnectionError(connErr) {
		t.Error("expected IsConnectionError to return true for ConnectionError")
	}
	if IsConnectionError(generic) {
		t.Error("expected IsConnectionError to return false for generic error")
	}
}

func TestAuthenticationError(t *testing.T) {
	err := NewAuthenticationError("192.168.1.1", "admin", "invalid password", nil)

	if !strings.Contains(err.Error(), "192.168.1.1") {
		t.Error("expected error to contain host")
	}
	if !strings.Contains(err.Error(), "admin") {
		t.Error("expected error to contain username")
	}
	if !strings.Contains(err.Error(), "invalid password") {
		t.Error("expected error to contain message")
	}
}

func TestCommandError(t *testing.T) {
	err := NewCommandError("display ont info", "command not found", "Error: command not recognized", nil)

	if !strings.Contains(err.Error(), "display ont info") {
		t.Error("expected error to contain command")
	}
	if !strings.Contains(err.Error(), "command not found") {
		t.Error("expected error to contain message")
	}
	if !strings.Contains(err.Error(), "command not recognized") {
		t.Error("expected error to contain output")
	}
}

func TestValidationError(t *testing.T) {
	err := NewValidationError("serial_number", "ABC123", "must be 16 characters")

	if !strings.Contains(err.Error(), "serial_number") {
		t.Error("expected error to contain field name")
	}
	if !strings.Contains(err.Error(), "ABC123") {
		t.Error("expected error to contain value")
	}
	if !strings.Contains(err.Error(), "must be 16 characters") {
		t.Error("expected error to contain message")
	}
}

func TestResourceNotFoundError(t *testing.T) {
	err := NewResourceNotFoundError("ONU", "0/0/1/101", "not provisioned on this OLT")

	if !strings.Contains(err.Error(), "ONU") {
		t.Error("expected error to contain resource type")
	}
	if !strings.Contains(err.Error(), "0/0/1/101") {
		t.Error("expected error to contain identifier")
	}
	if !strings.Contains(err.Error(), "not provisioned") {
		t.Error("expected error to contain message")
	}
}

func TestTimeoutError(t *testing.T) {
	err := NewTimeoutError("GetONUList", "30s", "device not responding")

	if !strings.Contains(err.Error(), "GetONUList") {
		t.Error("expected error to contain operation")
	}
	if !strings.Contains(err.Error(), "30s") {
		t.Error("expected error to contain timeout")
	}
	if !strings.Contains(err.Error(), "device not responding") {
		t.Error("expected error to contain message")
	}
}
