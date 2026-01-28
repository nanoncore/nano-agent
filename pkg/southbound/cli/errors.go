package cli

import (
	"fmt"
)

// UnsupportedOperationError is returned when an operation is not supported
// by a specific vendor or model. This enables graceful degradation when
// certain features aren't available on particular OLT hardware.
type UnsupportedOperationError struct {
	Vendor    string
	Model     string
	Operation string
	Reason    string
}

// Error implements the error interface.
func (e *UnsupportedOperationError) Error() string {
	if e.Model != "" {
		if e.Reason != "" {
			return fmt.Sprintf("operation %q not supported by %s %s: %s",
				e.Operation, e.Vendor, e.Model, e.Reason)
		}
		return fmt.Sprintf("operation %q not supported by %s %s",
			e.Operation, e.Vendor, e.Model)
	}
	if e.Reason != "" {
		return fmt.Sprintf("operation %q not supported by %s: %s",
			e.Operation, e.Vendor, e.Reason)
	}
	return fmt.Sprintf("operation %q not supported by %s",
		e.Operation, e.Vendor)
}

// NewUnsupportedOperationError creates a new UnsupportedOperationError.
func NewUnsupportedOperationError(vendor, model, operation, reason string) *UnsupportedOperationError {
	return &UnsupportedOperationError{
		Vendor:    vendor,
		Model:     model,
		Operation: operation,
		Reason:    reason,
	}
}

// IsUnsupportedOperation checks if an error is an UnsupportedOperationError.
func IsUnsupportedOperation(err error) bool {
	_, ok := err.(*UnsupportedOperationError)
	return ok
}

// ConnectionError represents errors related to OLT connection failures.
type ConnectionError struct {
	Host    string
	Port    int
	Cause   error
	Message string
}

// Error implements the error interface.
func (e *ConnectionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("connection to %s:%d failed: %s (%v)",
			e.Host, e.Port, e.Message, e.Cause)
	}
	return fmt.Sprintf("connection to %s:%d failed: %s",
		e.Host, e.Port, e.Message)
}

// Unwrap returns the underlying error.
func (e *ConnectionError) Unwrap() error {
	return e.Cause
}

// NewConnectionError creates a new ConnectionError.
func NewConnectionError(host string, port int, message string, cause error) *ConnectionError {
	return &ConnectionError{
		Host:    host,
		Port:    port,
		Message: message,
		Cause:   cause,
	}
}

// IsConnectionError checks if an error is a ConnectionError.
func IsConnectionError(err error) bool {
	_, ok := err.(*ConnectionError)
	return ok
}

// AuthenticationError represents authentication failures with the OLT.
type AuthenticationError struct {
	Host     string
	Username string
	Cause    error
	Message  string
}

// Error implements the error interface.
func (e *AuthenticationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("authentication failed for %s@%s: %s (%v)",
			e.Username, e.Host, e.Message, e.Cause)
	}
	return fmt.Sprintf("authentication failed for %s@%s: %s",
		e.Username, e.Host, e.Message)
}

// Unwrap returns the underlying error.
func (e *AuthenticationError) Unwrap() error {
	return e.Cause
}

// NewAuthenticationError creates a new AuthenticationError.
func NewAuthenticationError(host, username, message string, cause error) *AuthenticationError {
	return &AuthenticationError{
		Host:     host,
		Username: username,
		Message:  message,
		Cause:    cause,
	}
}

// IsAuthenticationError checks if an error is an AuthenticationError.
func IsAuthenticationError(err error) bool {
	_, ok := err.(*AuthenticationError)
	return ok
}

// CommandError represents errors from executing CLI commands on the OLT.
type CommandError struct {
	Command string
	Output  string
	Cause   error
	Message string
}

// Error implements the error interface.
func (e *CommandError) Error() string {
	if e.Output != "" {
		return fmt.Sprintf("command %q failed: %s (output: %s)",
			e.Command, e.Message, e.Output)
	}
	if e.Cause != nil {
		return fmt.Sprintf("command %q failed: %s (%v)",
			e.Command, e.Message, e.Cause)
	}
	return fmt.Sprintf("command %q failed: %s",
		e.Command, e.Message)
}

// Unwrap returns the underlying error.
func (e *CommandError) Unwrap() error {
	return e.Cause
}

// NewCommandError creates a new CommandError.
func NewCommandError(command, message, output string, cause error) *CommandError {
	return &CommandError{
		Command: command,
		Message: message,
		Output:  output,
		Cause:   cause,
	}
}

// IsCommandError checks if an error is a CommandError.
func IsCommandError(err error) bool {
	_, ok := err.(*CommandError)
	return ok
}

// ValidationError represents errors from input validation.
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Value != nil {
		return fmt.Sprintf("validation failed for %s=%v: %s",
			e.Field, e.Value, e.Message)
	}
	return fmt.Sprintf("validation failed for %s: %s",
		e.Field, e.Message)
}

// NewValidationError creates a new ValidationError.
func NewValidationError(field string, value interface{}, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}

// IsValidationError checks if an error is a ValidationError.
func IsValidationError(err error) bool {
	_, ok := err.(*ValidationError)
	return ok
}

// ResourceNotFoundError represents errors when an ONU, port, or profile is not found.
type ResourceNotFoundError struct {
	ResourceType string
	Identifier   string
	Message      string
}

// Error implements the error interface.
func (e *ResourceNotFoundError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s %q not found: %s",
			e.ResourceType, e.Identifier, e.Message)
	}
	return fmt.Sprintf("%s %q not found",
		e.ResourceType, e.Identifier)
}

// NewResourceNotFoundError creates a new ResourceNotFoundError.
func NewResourceNotFoundError(resourceType, identifier, message string) *ResourceNotFoundError {
	return &ResourceNotFoundError{
		ResourceType: resourceType,
		Identifier:   identifier,
		Message:      message,
	}
}

// IsResourceNotFoundError checks if an error is a ResourceNotFoundError.
func IsResourceNotFoundError(err error) bool {
	_, ok := err.(*ResourceNotFoundError)
	return ok
}

// TimeoutError represents timeout errors when communicating with the OLT.
type TimeoutError struct {
	Operation string
	Timeout   string
	Message   string
}

// Error implements the error interface.
func (e *TimeoutError) Error() string {
	return fmt.Sprintf("operation %q timed out after %s: %s",
		e.Operation, e.Timeout, e.Message)
}

// NewTimeoutError creates a new TimeoutError.
func NewTimeoutError(operation, timeout, message string) *TimeoutError {
	return &TimeoutError{
		Operation: operation,
		Timeout:   timeout,
		Message:   message,
	}
}

// IsTimeoutError checks if an error is a TimeoutError.
func IsTimeoutError(err error) bool {
	_, ok := err.(*TimeoutError)
	return ok
}
