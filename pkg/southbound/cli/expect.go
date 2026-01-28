package cli

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	expect "github.com/google/goexpect"
	"golang.org/x/crypto/ssh"
)

// DefaultPromptPattern matches common CLI prompts like "hostname#" or "hostname>"
// This pattern matches:
// - Word characters and hyphens followed by # or >
// - Handles prompts with mode indicators like "[config]#"
var DefaultPromptPattern = regexp.MustCompile(`(?m)[\w\-\[\]()]+[#>]\s*$`)

// VendorPrompts contains vendor-specific prompt patterns
// V-SOL uses Cisco-style prompts with optional mode indicators: hostname>, hostname#, hostname(config)#
var VendorPrompts = map[string]*regexp.Regexp{
	"huawei": regexp.MustCompile(`(?m)(<[\w\-]+>|\[[\w\-~/]+\])\s*$`),
	"vsol":   regexp.MustCompile(`(?m)[\w\-]+(\([\w\-/]+\))?[#>]\s*$`),
	"cdata":  regexp.MustCompile(`(?m)[\w\-]+(\([\w\-/]+\))?[#>]\s*$`),
	"zte":    regexp.MustCompile(`(?m)(<[\w\-]+>|\[[\w\-~]+\])\s*$`),
	"cisco":  regexp.MustCompile(`(?m)[\w\-]+(\([\w\-/]+\))?[#>]\s*$`),
}

// PagerDisableCommands contains commands to disable paging per vendor
var PagerDisableCommands = map[string]string{
	"huawei": "screen-length 0 temporary",
	"vsol":   "terminal length 0",
	"cdata":  "terminal length 0",
	"zte":    "screen-length 0 temporary",
	"cisco":  "terminal length 0",
}

// CLIErrorPatterns contains common CLI error patterns that indicate command failure
var CLIErrorPatterns = []string{
	"command not found",
	"% unknown command",
	"% invalid",
	"% incomplete command",
	"syntax error",
	"unrecognized command",
	"bad command",
}

// ExpectSession wraps google/goexpect for network equipment CLI interaction
type ExpectSession struct {
	expecter    *expect.GExpect
	sshClient   *ssh.Client
	promptRE    *regexp.Regexp
	timeout     time.Duration
	vendor      string
	initialized bool
}

// ExpectSessionConfig holds configuration for creating an expect session
type ExpectSessionConfig struct {
	SSHClient    *ssh.Client
	Vendor       string
	Timeout      time.Duration
	CustomPrompt *regexp.Regexp
	DisablePager bool
	// Credentials for CLI-level authentication (double-login scenarios like V-Sol)
	Username string
	Password string
}

// NewExpectSession creates a new interactive CLI session using expect
func NewExpectSession(cfg ExpectSessionConfig) (*ExpectSession, error) {
	if cfg.SSHClient == nil {
		return nil, fmt.Errorf("SSH client is required")
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	// Determine prompt pattern
	promptRE := cfg.CustomPrompt
	if promptRE == nil {
		if vendorPrompt, ok := VendorPrompts[strings.ToLower(cfg.Vendor)]; ok {
			promptRE = vendorPrompt
		} else {
			promptRE = DefaultPromptPattern
		}
	}

	// Spawn expect session over SSH
	exp, _, err := expect.SpawnSSH(cfg.SSHClient, cfg.Timeout,
		expect.Verbose(false),
		expect.CheckDuration(500*time.Millisecond),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn SSH expect session: %w", err)
	}

	session := &ExpectSession{
		expecter:  exp,
		sshClient: cfg.SSHClient,
		promptRE:  promptRE,
		timeout:   cfg.Timeout,
		vendor:    cfg.Vendor,
	}

	// Handle double-login scenarios (e.g., V-Sol OLTs that require CLI-level auth after SSH)
	// Try to detect either: CLI prompt, "Login:", or "Username:"
	loginRE := regexp.MustCompile(`(?i)(Login|Username)\s*:\s*$`)
	passwordRE := regexp.MustCompile(`(?i)Password\s*:\s*$`)
	combinedRE := regexp.MustCompile(`(?m)(` + promptRE.String() + `|(?i)(Login|Username)\s*:\s*$)`)

	output, _, err := exp.Expect(combinedRE, cfg.Timeout)
	if err != nil {
		exp.Close()
		return nil, fmt.Errorf("failed to detect initial prompt or login: %w", err)
	}

	// Check if we got a login prompt instead of CLI prompt
	if loginRE.MatchString(output) {
		// Send username
		if cfg.Username == "" {
			exp.Close()
			return nil, fmt.Errorf("CLI login required but no username provided")
		}
		if err := exp.Send(cfg.Username + "\n"); err != nil {
			exp.Close()
			return nil, fmt.Errorf("failed to send username: %w", err)
		}

		// Wait for password prompt
		if _, _, err := exp.Expect(passwordRE, cfg.Timeout); err != nil {
			exp.Close()
			return nil, fmt.Errorf("failed to detect password prompt: %w", err)
		}

		// Send password
		if err := exp.Send(cfg.Password + "\n"); err != nil {
			exp.Close()
			return nil, fmt.Errorf("failed to send password: %w", err)
		}

		// Wait for CLI prompt after authentication
		if _, _, err := exp.Expect(promptRE, cfg.Timeout); err != nil {
			exp.Close()
			return nil, fmt.Errorf("failed to detect CLI prompt after login: %w", err)
		}
	}

	// Disable pager if requested (non-fatal if it fails)
	// Skip for vendors that require privileged mode first (e.g., V-SOL)
	// Those vendors should call DisablePager() manually after mode escalation
	if cfg.DisablePager && !session.requiresPrivilegedPager() {
		_ = session.disablePager()
	}

	session.initialized = true
	return session, nil
}

// requiresPrivilegedPager returns true for vendors that need privileged mode before pager disable
func (s *ExpectSession) requiresPrivilegedPager() bool {
	switch strings.ToLower(s.vendor) {
	case "vsol":
		return true // V-SOL requires privileged mode for terminal length 0
	default:
		return false
	}
}

// disablePager sends the appropriate command to disable pagination
func (s *ExpectSession) disablePager() error {
	cmd := PagerDisableCommands[strings.ToLower(s.vendor)]
	if cmd == "" {
		cmd = "terminal length 0" // Generic fallback
	}

	_, err := s.Execute(cmd)
	return err
}

// DisablePager is the public method to disable pager (for drivers that need to call it after mode escalation)
func (s *ExpectSession) DisablePager() error {
	return s.disablePager()
}

// Execute sends a command and waits for the prompt, returning the output
func (s *ExpectSession) Execute(command string) (string, error) {
	if s.expecter == nil {
		return "", fmt.Errorf("expect session not initialized")
	}

	// Send command
	if err := s.expecter.Send(command + "\n"); err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	// Wait for prompt and capture output
	output, _, err := s.expecter.Expect(s.promptRE, s.timeout)
	if err != nil {
		return output, fmt.Errorf("timeout waiting for prompt after command %q: %w", command, err)
	}

	// Clean up output: remove the command echo and trailing prompt
	output = s.cleanOutput(output, command)

	// Check for CLI error patterns in output
	if err := s.checkCLIErrors(output); err != nil {
		return output, err
	}

	return output, nil
}

// checkCLIErrors checks the output for common CLI error patterns
func (s *ExpectSession) checkCLIErrors(output string) error {
	outputLower := strings.ToLower(output)
	for _, pattern := range CLIErrorPatterns {
		if strings.Contains(outputLower, pattern) {
			return fmt.Errorf("CLI error detected: %s", strings.TrimSpace(output))
		}
	}
	return nil
}

// ExecuteBatch executes multiple commands in sequence
func (s *ExpectSession) ExecuteBatch(commands []string) ([]string, error) {
	results := make([]string, 0, len(commands))

	for _, cmd := range commands {
		output, err := s.Execute(cmd)
		if err != nil {
			return results, fmt.Errorf("command %q failed: %w", cmd, err)
		}
		results = append(results, output)
	}

	return results, nil
}

// cleanOutput removes command echo and prompt from output
func (s *ExpectSession) cleanOutput(output, command string) string {
	lines := strings.Split(output, "\n")
	var cleaned []string

	for i, line := range lines {
		// Skip the first line if it's the command echo
		if i == 0 && strings.Contains(line, command) {
			continue
		}
		// Skip lines that match the prompt pattern
		if s.promptRE.MatchString(strings.TrimSpace(line)) {
			continue
		}
		cleaned = append(cleaned, line)
	}

	result := strings.Join(cleaned, "\n")
	return strings.TrimSpace(result)
}

// IsAlive checks if the session is still responsive
func (s *ExpectSession) IsAlive() bool {
	if s.expecter == nil {
		return false
	}

	// Send empty line and expect prompt
	if err := s.expecter.Send("\n"); err != nil {
		return false
	}

	_, _, err := s.expecter.Expect(s.promptRE, 5*time.Second)
	return err == nil
}

// Close closes the expect session
func (s *ExpectSession) Close() error {
	if s.expecter != nil {
		return s.expecter.Close()
	}
	return nil
}

// SetTimeout updates the command timeout
func (s *ExpectSession) SetTimeout(timeout time.Duration) {
	s.timeout = timeout
}

// GetPromptPattern returns the current prompt pattern
func (s *ExpectSession) GetPromptPattern() *regexp.Regexp {
	return s.promptRE
}

// ExecuteEnableWithPassword handles the enable command with password prompt.
// V-SOL and some other vendors require a password after enable command.
func (s *ExpectSession) ExecuteEnableWithPassword(password string) (string, error) {
	if s.expecter == nil {
		return "", fmt.Errorf("expect session not initialized")
	}

	// Send enable command
	if err := s.expecter.Send("enable\n"); err != nil {
		return "", fmt.Errorf("failed to send enable command: %w", err)
	}

	// Wait for either password prompt or privileged prompt
	passwordRE := regexp.MustCompile(`(?i)Password\s*:\s*$`)
	combinedRE := regexp.MustCompile(`(?m)(` + s.promptRE.String() + `|(?i)Password\s*:\s*$)`)

	output, _, err := s.expecter.Expect(combinedRE, s.timeout)
	if err != nil {
		return output, fmt.Errorf("timeout waiting for enable response: %w", err)
	}

	// Check if we got a password prompt
	if passwordRE.MatchString(output) {
		// Send password
		if err := s.expecter.Send(password + "\n"); err != nil {
			return output, fmt.Errorf("failed to send enable password: %w", err)
		}

		// Wait for privileged prompt
		output2, _, err := s.expecter.Expect(s.promptRE, s.timeout)
		if err != nil {
			return output + output2, fmt.Errorf("failed to enter privileged mode after password: %w", err)
		}
		output = output + output2
	}

	return s.cleanOutput(output, "enable"), nil
}

// SetPromptPattern updates the prompt pattern
func (s *ExpectSession) SetPromptPattern(pattern *regexp.Regexp) {
	s.promptRE = pattern
}
