package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// CLIDriver defines the interface for vendor CLI operations.
type CLIDriver interface {
	// ==========================================================================
	// Connection Management
	// ==========================================================================

	// Connect establishes SSH connection to the device.
	Connect(ctx context.Context) error

	// Close terminates the SSH connection.
	Close() error

	// Execute runs a CLI command and returns output.
	Execute(ctx context.Context, cmd string) (string, error)

	// Vendor returns the vendor type for this driver.
	Vendor() string

	// ==========================================================================
	// ONU Provisioning
	// ==========================================================================

	// AddONU provisions a new ONU on the OLT.
	AddONU(ctx context.Context, req *ONUProvisionRequest) error

	// DeleteONU removes an ONU from the OLT.
	DeleteONU(ctx context.Context, ponPort string, onuID int) error

	// GetONUInfo retrieves detailed ONU information via CLI.
	GetONUInfo(ctx context.Context, ponPort string, onuID int) (*ONUCLIInfo, error)

	// RebootONU reboots a specific ONU.
	RebootONU(ctx context.Context, ponPort string, onuID int) error

	// ==========================================================================
	// VLAN Management
	// ==========================================================================

	// ConfigureVLAN configures VLAN settings for an ONU.
	ConfigureVLAN(ctx context.Context, config *VLANConfig) error

	// GetVLANConfig retrieves VLAN configuration for an ONU.
	GetVLANConfig(ctx context.Context, ponPort string, onuID int) (*VLANConfig, error)

	// AddVLANTranslation adds a VLAN translation rule.
	AddVLANTranslation(ctx context.Context, ponPort string, onuID int, translation VLANTranslation) error

	// RemoveVLANTranslation removes a VLAN translation rule.
	RemoveVLANTranslation(ctx context.Context, ponPort string, onuID int, customerVLAN int) error

	// ListVLANs lists all VLANs on the device.
	ListVLANs(ctx context.Context) ([]VLANInfo, error)

	// ==========================================================================
	// Profile Management
	// ==========================================================================

	// ListLineProfiles lists all line profiles.
	ListLineProfiles(ctx context.Context) ([]LineProfile, error)

	// GetLineProfile retrieves a specific line profile.
	GetLineProfile(ctx context.Context, profileID int) (*LineProfile, error)

	// ListServiceProfiles lists all service profiles.
	ListServiceProfiles(ctx context.Context) ([]ServiceProfile, error)

	// GetServiceProfile retrieves a specific service profile.
	GetServiceProfile(ctx context.Context, profileID int) (*ServiceProfile, error)

	// ListTrafficProfiles lists all traffic/bandwidth profiles.
	ListTrafficProfiles(ctx context.Context) ([]TrafficProfile, error)

	// AssignTrafficProfile assigns a traffic profile to an ONU.
	AssignTrafficProfile(ctx context.Context, ponPort string, onuID int, profileID int) error

	// ==========================================================================
	// Port Control
	// ==========================================================================

	// ListPONPorts lists all PON ports on the device.
	ListPONPorts(ctx context.Context) ([]PONPortInfo, error)

	// GetPONPortInfo retrieves information about a specific PON port.
	GetPONPortInfo(ctx context.Context, slot, port int) (*PONPortInfo, error)

	// EnablePONPort enables a PON port.
	EnablePONPort(ctx context.Context, slot, port int) error

	// DisablePONPort disables a PON port.
	DisablePONPort(ctx context.Context, slot, port int) error

	// SetPortDescription sets the description for a port.
	SetPortDescription(ctx context.Context, slot, port int, description string) error

	// ==========================================================================
	// Batch Operations
	// ==========================================================================

	// BatchProvision provisions multiple ONUs in one operation.
	BatchProvision(ctx context.Context, req *BatchProvisionRequest) (*BatchResult, error)

	// BatchConfigureVLAN configures VLANs for multiple ONUs.
	BatchConfigureVLAN(ctx context.Context, req *BatchVLANRequest) (*BatchResult, error)

	// ExportConfig exports the current device configuration.
	ExportConfig(ctx context.Context) (*ConfigExport, error)

	// ==========================================================================
	// Diagnostics
	// ==========================================================================

	// GetONUDiagnostics retrieves comprehensive diagnostics for an ONU.
	GetONUDiagnostics(ctx context.Context, ponPort string, onuID int) (*ONUDiagnostics, error)

	// GetONUCounters retrieves performance counters for an ONU.
	GetONUCounters(ctx context.Context, ponPort string, onuID int) (*PerformanceCounters, error)

	// ClearONUCounters clears/resets performance counters for an ONU.
	ClearONUCounters(ctx context.Context, ponPort string, onuID int) error

	// GetOpticalDiagnostics retrieves optical power readings for an ONU.
	GetOpticalDiagnostics(ctx context.Context, ponPort string, onuID int) (*OpticalDiagnostics, error)

	// ==========================================================================
	// Configuration Management
	// ==========================================================================

	// SaveConfig saves the running configuration to persistent storage.
	SaveConfig(ctx context.Context) error
}

// BaseCLIDriver provides common SSH functionality for vendor drivers.
type BaseCLIDriver struct {
	config  CLIConfig
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	mu      sync.Mutex
}

// NewBaseCLIDriver creates a new base CLI driver.
func NewBaseCLIDriver(config CLIConfig) *BaseCLIDriver {
	if config.Port == 0 {
		config.Port = 22
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	return &BaseCLIDriver{
		config: config,
	}
}

// Connect establishes SSH connection to the device.
func (d *BaseCLIDriver) Connect(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.client != nil {
		return nil // Already connected
	}

	sshConfig := &ssh.ClientConfig{
		User: d.config.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(d.config.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Implement proper host key verification
		Timeout:         d.config.Timeout,
	}

	addr := fmt.Sprintf("%s:%d", d.config.Host, d.config.Port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Request pseudo-terminal for interactive CLI
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // Disable echo
		ssh.TTY_OP_ISPEED: 14400, // Input speed
		ssh.TTY_OP_OSPEED: 14400, // Output speed
	}
	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("failed to request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		return fmt.Errorf("failed to start shell: %w", err)
	}

	d.client = client
	d.session = session
	d.stdin = stdin
	d.stdout = bufio.NewReader(stdout)

	// Wait for initial prompt
	if _, err := d.readUntilPrompt(ctx); err != nil {
		d.Close()
		return fmt.Errorf("failed to get initial prompt: %w", err)
	}

	return nil
}

// Close terminates the SSH connection.
func (d *BaseCLIDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var errs []error
	if d.session != nil {
		if err := d.session.Close(); err != nil {
			errs = append(errs, err)
		}
		d.session = nil
	}
	if d.client != nil {
		if err := d.client.Close(); err != nil {
			errs = append(errs, err)
		}
		d.client = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// Execute runs a CLI command and returns the output.
func (d *BaseCLIDriver) Execute(ctx context.Context, cmd string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.client == nil {
		return "", fmt.Errorf("not connected")
	}

	// Write command
	if _, err := d.stdin.Write([]byte(cmd + "\n")); err != nil {
		return "", fmt.Errorf("failed to write command: %w", err)
	}

	// Read response until prompt
	output, err := d.readUntilPrompt(ctx)
	if err != nil {
		return output, err
	}

	// Remove the command echo from output
	lines := strings.Split(output, "\n")
	if len(lines) > 1 {
		output = strings.Join(lines[1:], "\n")
	}

	return strings.TrimSpace(output), nil
}

// readUntilPrompt reads output until a command prompt is detected.
func (d *BaseCLIDriver) readUntilPrompt(ctx context.Context) (string, error) {
	var output strings.Builder
	readDone := make(chan error, 1)

	go func() {
		for {
			line, err := d.stdout.ReadString('\n')
			if err != nil && err != io.EOF {
				readDone <- err
				return
			}
			output.WriteString(line)

			// Check for common prompts (vendor-specific drivers should override)
			trimmed := strings.TrimSpace(line)
			if strings.HasSuffix(trimmed, "#") ||
				strings.HasSuffix(trimmed, ">") ||
				strings.HasSuffix(trimmed, "]") {
				readDone <- nil
				return
			}

			if err == io.EOF {
				readDone <- nil
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		return output.String(), ctx.Err()
	case err := <-readDone:
		return output.String(), err
	}
}

// Config returns the CLI configuration.
func (d *BaseCLIDriver) Config() CLIConfig {
	return d.config
}

// IsConnected returns true if the driver is connected.
func (d *BaseCLIDriver) IsConnected() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.client != nil
}
