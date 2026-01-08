package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/nanoncore/nano-agent/pkg/agent/poller"
	"github.com/spf13/cobra"
)

var (
	version   = "0.1.0-dev"
	commit    = "unknown"
	buildDate = "unknown"
)

var (
	configDir string
)

var rootCmd = &cobra.Command{
	Use:   "nano-agent",
	Short: "Nanoncore edge agent for BNG data plane nodes",
	Long: `nano-agent is the Nanoncore edge agent that runs on bare-metal BNG nodes.

It handles enrollment with the control plane, configuration synchronization,
telemetry reporting, and integration with the VPP data plane.

Quick start:
  sudo nano-agent enroll --api https://api.nanoncore.com --token YOUR_TOKEN --node-id pop-paris-01
  sudo nano-agent status`,
	Version: version,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("nano-agent version %s (commit: %s, built: %s)\n", version, commit, buildDate)
	},
}

var enrollCmd = &cobra.Command{
	Use:   "enroll",
	Short: "Enroll this node with the Nanoncore control plane",
	Long: `Enroll registers this edge node with the Nanoncore control plane.

Interactive mode (after 'nano-agent login'):
  - Automatically uses your saved API key
  - Lists available networks for selection
  - Prompts for node ID if not provided

Token mode (legacy):
  - Requires --api, --token, and --node-id flags

The enrollment process:
1. Validates credentials with the API
2. Downloads mTLS certificates for secure communication
3. Saves configuration to /etc/nano-agent/config.json
4. Marks the node as enrolled

Examples:
  # Interactive mode (recommended)
  nano-agent login
  sudo nano-agent enroll --node-id pop-paris-01

  # Token mode
  sudo nano-agent enroll \
    --api https://api.nanoncore.com \
    --token YOUR_ENROLLMENT_TOKEN \
    --node-id pop-paris-01 \
    --labels "pop=paris,role=bng,tier=edge"`,
	RunE: runEnroll,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current agent and node status",
	Long: `Display the current status of the nano-agent and the node.

Shows enrollment status, VPP data plane status, and connection to control plane.`,
	RunE: runStatus,
}

var unenrollCmd = &cobra.Command{
	Use:   "unenroll",
	Short: "Remove this node from the control plane",
	Long:  `Unenroll removes this node's registration and clears local configuration.`,
	RunE:  runUnenroll,
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the agent in daemon mode",
	Long: `Run the nano-agent in daemon mode.

The agent will:
1. Send periodic heartbeats to the control plane
2. Report VPP data plane status
3. Sync configuration changes
4. Report telemetry data

This command runs in the foreground and is designed to be managed by systemd.

Example:
  sudo nano-agent run
  sudo nano-agent run --heartbeat-interval 30s`,
	RunE: runDaemon,
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Nanoncore using an API key",
	Long: `Login authenticates you with the Nanoncore control plane using an API key.

After logging in, you can use 'nano-agent enroll' interactively to select
your organization and network.

Get your API key from the Nanoncore dashboard at Settings → API Keys.

Example:
  nano-agent login --api https://api.nanoncore.com
  nano-agent login --api-key YOUR_API_KEY`,
	RunE: runLogin,
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored credentials",
	Long:  `Logout removes your stored API key and credentials from this machine.`,
	RunE:  runLogout,
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current logged-in user",
	Long:  `Display information about the currently logged-in user.`,
	RunE:  runWhoami,
}

// Enroll flags
var (
	enrollAPIURL string
	enrollToken  string
	enrollNodeID string
	enrollLabels string
)

// Run flags
var (
	heartbeatInterval  time.Duration
	configSyncInterval time.Duration
	enableOLTPolling   bool
	pollerWorkers      int
)

// Login flags
var (
	loginAPIURL string
	loginAPIKey string
)

// Default API URL
const defaultAPIURL = "https://api.nanoncore.com"

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", agent.DefaultConfigDir,
		"Configuration directory")

	// Enroll flags
	enrollCmd.Flags().StringVar(&enrollAPIURL, "api", "", "Nanoncore API URL (uses saved credentials if not set)")
	enrollCmd.Flags().StringVar(&enrollToken, "token", "", "Enrollment token (uses API key auth if logged in)")
	enrollCmd.Flags().StringVar(&enrollNodeID, "node-id", "", "Unique node identifier (prompted if not set)")
	enrollCmd.Flags().StringVar(&enrollLabels, "labels", "", "Node labels (key=value,key2=value2)")

	// Run flags
	runCmd.Flags().DurationVar(&heartbeatInterval, "heartbeat-interval", 30*time.Second,
		"Interval between heartbeats to control plane")
	runCmd.Flags().DurationVar(&configSyncInterval, "config-sync-interval", 5*time.Minute,
		"Interval between configuration syncs")
	runCmd.Flags().BoolVar(&enableOLTPolling, "enable-olt-polling", true,
		"Enable OLT polling for ONU discovery")
	runCmd.Flags().IntVar(&pollerWorkers, "poller-workers", 5,
		"Number of concurrent OLT polling workers")

	// Login flags
	loginCmd.Flags().StringVar(&loginAPIURL, "api", defaultAPIURL, "Nanoncore API URL")
	loginCmd.Flags().StringVar(&loginAPIKey, "api-key", "", "API key (will prompt if not provided)")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(whoamiCmd)
	rootCmd.AddCommand(enrollCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(unenrollCmd)
	rootCmd.AddCommand(runCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runEnroll(cmd *cobra.Command, args []string) error {
	fmt.Printf("Nanoncore Edge Agent Enrollment\n")
	fmt.Printf("================================\n\n")

	// Check if already enrolled
	state, _ := agent.LoadState(configDir)
	if state != nil && state.Enrolled {
		return fmt.Errorf("node already enrolled. Use 'nano-agent unenroll' first to re-enroll")
	}

	// Check if we have saved credentials for interactive mode
	creds, _ := agent.LoadCredentials(configDir)
	useInteractiveMode := creds != nil && creds.APIKey != "" && enrollToken == ""

	// Parse labels
	labels := make(map[string]string)
	if enrollLabels != "" {
		for _, pair := range strings.Split(enrollLabels, ",") {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				labels[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}

	var (
		apiURL         string
		client         *agent.Client
		selectedOrg    *agent.Organization
		selectedNet    *agent.Network
		nodeID         string
	)

	if useInteractiveMode {
		// Interactive mode using saved API key
		fmt.Printf("Using saved credentials for %s\n\n", creds.UserEmail)

		apiURL = creds.DefaultAPIURL
		if enrollAPIURL != "" {
			apiURL = enrollAPIURL
		}

		client = agent.NewClientWithAPIKey(apiURL, creds.APIKey)

		// Check API connectivity
		fmt.Printf("Checking API connectivity... ")
		if err := client.CheckAPIHealth(); err != nil {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("cannot reach API: %w", err)
		}
		fmt.Printf("OK\n")

		// Fetch organizations
		fmt.Printf("Fetching organizations... ")
		orgs, err := client.ListOrganizations()
		if err != nil {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("failed to fetch organizations: %w", err)
		}
		if len(orgs) == 0 {
			fmt.Printf("NONE\n")
			return fmt.Errorf("no organizations found. Create one at %s", apiURL)
		}
		fmt.Printf("OK (%d found)\n", len(orgs))

		// Select organization (auto-select if only one)
		if len(orgs) == 1 {
			selectedOrg = &orgs[0]
			fmt.Printf("Organization: %s\n", selectedOrg.Name)
		} else {
			orgOptions := make([]string, len(orgs))
			for i, org := range orgs {
				orgOptions[i] = fmt.Sprintf("%s (%s)", org.Name, org.Slug)
			}
			choice, err := promptSelection("Select organization:", orgOptions)
			if err != nil {
				return fmt.Errorf("organization selection failed: %w", err)
			}
			selectedOrg = &orgs[choice]
		}

		// Fetch networks
		fmt.Printf("Fetching networks... ")
		networks, err := client.ListNetworks(selectedOrg.ID)
		if err != nil {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("failed to fetch networks: %w", err)
		}
		if len(networks) == 0 {
			fmt.Printf("NONE\n")
			return fmt.Errorf("no networks found in %s. Create one in the dashboard", selectedOrg.Name)
		}
		fmt.Printf("OK (%d found)\n", len(networks))

		// Select network (auto-select if only one)
		if len(networks) == 1 {
			selectedNet = &networks[0]
			fmt.Printf("Network: %s", selectedNet.Name)
			if selectedNet.City != "" {
				fmt.Printf(" (%s)", selectedNet.City)
			}
			fmt.Printf("\n")
		} else {
			netOptions := make([]string, len(networks))
			for i, net := range networks {
				opt := net.Name
				if net.City != "" {
					opt += fmt.Sprintf(" (%s)", net.City)
				}
				if net.IsDefault {
					opt += " [default]"
				}
				netOptions[i] = opt
			}
			choice, err := promptSelection("Select network:", netOptions)
			if err != nil {
				return fmt.Errorf("network selection failed: %w", err)
			}
			selectedNet = &networks[choice]
		}

		// Get node ID
		nodeID = enrollNodeID
		if nodeID == "" {
			hostname, _ := os.Hostname()
			var err error
			nodeID, err = promptString("Node ID", hostname)
			if err != nil {
				return fmt.Errorf("failed to get node ID: %w", err)
			}
		}

		fmt.Printf("\nEnrollment Summary:\n")
		fmt.Printf("  Organization: %s\n", selectedOrg.Name)
		fmt.Printf("  Network:      %s (namespace: %s)\n", selectedNet.Name, selectedNet.Slug)
		fmt.Printf("  Node ID:      %s\n", nodeID)
		if len(labels) > 0 {
			fmt.Printf("  Labels:       %v\n", labels)
		}
		fmt.Println()

		// Send enrollment request (V2 with org/network)
		fmt.Printf("Enrolling node... ")
		enrollReq := &agent.EnrollRequestV2{
			NodeID:         nodeID,
			Labels:         labels,
			OrganizationID: selectedOrg.ID,
			NetworkID:      selectedNet.ID,
			NetworkSlug:    selectedNet.Slug,
		}

		resp, err := client.EnrollV2(enrollReq)
		if err != nil {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("enrollment failed: %w", err)
		}

		if !resp.Success {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("enrollment rejected: %s", resp.Message)
		}
		fmt.Printf("OK\n")

		// Save certificates if provided
		if err := saveCertificates(resp); err != nil {
			return err
		}

		// Save configuration with org/network info and agent API key
		fmt.Printf("Saving configuration... ")
		cfg := &agent.Config{
			APIURL:           apiURL,
			NodeID:           nodeID,
			Labels:           labels,
			CertFile:         configDir + "/client.crt",
			KeyFile:          configDir + "/client.key",
			CAFile:           configDir + "/ca.crt",
			OrganizationID:   selectedOrg.ID,
			OrganizationName: selectedOrg.Name,
			NetworkID:        selectedNet.ID,
			NetworkName:      selectedNet.Name,
			NetworkSlug:      selectedNet.Slug,
		}

		// Store agent API key if provided (for per-agent rate limiting)
		if resp.AgentAPIKey != "" {
			cfg.AgentID = resp.AgentID
			cfg.AgentAPIKey = resp.AgentAPIKey
			cfg.AgentAPIKeyPrefix = resp.AgentAPIKeyPrefix
		}

		if err := agent.SaveConfig(configDir, cfg); err != nil {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("OK\n")

		// Log agent key info
		if resp.AgentAPIKey != "" {
			fmt.Printf("Agent API key:  %s (per-agent rate limiting enabled)\n", resp.AgentAPIKeyPrefix)
		}

	} else {
		// Legacy token mode
		if enrollAPIURL == "" || enrollToken == "" || enrollNodeID == "" {
			return fmt.Errorf("token mode requires --api, --token, and --node-id flags.\nAlternatively, run 'nano-agent login' first for interactive mode")
		}

		apiURL = enrollAPIURL
		nodeID = enrollNodeID

		fmt.Printf("API URL:  %s\n", apiURL)
		fmt.Printf("Node ID:  %s\n", nodeID)
		if len(labels) > 0 {
			fmt.Printf("Labels:   %v\n", labels)
		}
		fmt.Println()

		// Create client and check API health
		fmt.Printf("Checking API connectivity... ")
		client = agent.NewClient(apiURL, enrollToken)
		if err := client.CheckAPIHealth(); err != nil {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("cannot reach API: %w", err)
		}
		fmt.Printf("OK\n")

		// Send enrollment request
		fmt.Printf("Enrolling node... ")
		enrollReq := &agent.EnrollRequest{
			NodeID: nodeID,
			Token:  enrollToken,
			Labels: labels,
		}

		resp, err := client.Enroll(enrollReq)
		if err != nil {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("enrollment failed: %w", err)
		}

		if !resp.Success {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("enrollment rejected: %s", resp.Message)
		}
		fmt.Printf("OK\n")

		// Save certificates if provided
		if err := saveCertificates(resp); err != nil {
			return err
		}

		// Save configuration
		fmt.Printf("Saving configuration... ")
		cfg := &agent.Config{
			APIURL:   apiURL,
			NodeID:   nodeID,
			Labels:   labels,
			CertFile: configDir + "/client.crt",
			KeyFile:  configDir + "/client.key",
			CAFile:   configDir + "/ca.crt",
		}

		// Store agent API key if provided (for per-agent rate limiting)
		if resp.AgentAPIKey != "" {
			cfg.AgentID = resp.AgentID
			cfg.AgentAPIKey = resp.AgentAPIKey
			cfg.AgentAPIKeyPrefix = resp.AgentAPIKeyPrefix
		}

		// Store org/network info if returned from enrollment
		if resp.OrganizationID != "" {
			cfg.OrganizationID = resp.OrganizationID
			cfg.OrganizationName = resp.OrganizationName
		}
		if resp.NetworkID != "" {
			cfg.NetworkID = resp.NetworkID
			cfg.NetworkName = resp.NetworkName
			cfg.NetworkSlug = resp.NetworkSlug
		}

		if err := agent.SaveConfig(configDir, cfg); err != nil {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("OK\n")

		// Log agent key info
		if resp.AgentAPIKey != "" {
			fmt.Printf("Agent API key:  %s (per-agent rate limiting enabled)\n", resp.AgentAPIKeyPrefix)
		}
	}

	// Save state
	fmt.Printf("Updating state... ")
	newState := &agent.State{
		Enrolled:     true,
		EnrolledAt:   time.Now().UTC().Format(time.RFC3339),
		AgentVersion: version,
	}

	if err := agent.SaveState(configDir, newState); err != nil {
		fmt.Printf("FAILED\n")
		return fmt.Errorf("failed to save state: %w", err)
	}
	fmt.Printf("OK\n")

	fmt.Printf("\n✓ Node '%s' successfully enrolled!\n", nodeID)
	if selectedNet != nil {
		fmt.Printf("  Network: %s (namespace: %s)\n", selectedNet.Name, selectedNet.Slug)
	}
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  - Start daemon: sudo nano-agent run\n")
	fmt.Printf("  - Check status: sudo nano-agent status\n")

	return nil
}

// saveCertificates saves the enrollment certificates to disk
func saveCertificates(resp *agent.EnrollResponse) error {
	if resp.Certificate == "" || resp.PrivateKey == "" {
		return nil
	}

	fmt.Printf("Saving certificates... ")
	certFile := configDir + "/client.crt"
	keyFile := configDir + "/client.key"
	caFile := configDir + "/ca.crt"

	if err := os.MkdirAll(configDir, 0750); err != nil {
		fmt.Printf("FAILED\n")
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(certFile, []byte(resp.Certificate), 0600); err != nil {
		fmt.Printf("FAILED\n")
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	if err := os.WriteFile(keyFile, []byte(resp.PrivateKey), 0600); err != nil {
		fmt.Printf("FAILED\n")
		return fmt.Errorf("failed to write private key: %w", err)
	}

	if resp.CACert != "" {
		if err := os.WriteFile(caFile, []byte(resp.CACert), 0600); err != nil {
			fmt.Printf("FAILED\n")
			return fmt.Errorf("failed to write CA certificate: %w", err)
		}
	}
	fmt.Printf("OK\n")
	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	fmt.Printf("Nanoncore Edge Agent Status\n")
	fmt.Printf("===========================\n\n")

	// Load state
	state, err := agent.LoadState(configDir)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Load config if enrolled
	var cfg *agent.Config
	if state.Enrolled {
		cfg, err = agent.LoadConfig(configDir)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Agent info
	fmt.Printf("Agent Version:  %s\n", version)
	fmt.Printf("Config Dir:     %s\n", configDir)
	fmt.Println()

	// Enrollment status
	fmt.Printf("Enrollment Status\n")
	fmt.Printf("-----------------\n")
	if state.Enrolled {
		fmt.Printf("  Enrolled:     Yes\n")
		fmt.Printf("  Enrolled At:  %s\n", state.EnrolledAt)
		fmt.Printf("  Node ID:      %s\n", cfg.NodeID)
		fmt.Printf("  API URL:      %s\n", cfg.APIURL)
		if cfg.OrganizationName != "" {
			fmt.Printf("  Organization: %s\n", cfg.OrganizationName)
		}
		if cfg.NetworkName != "" {
			fmt.Printf("  Network:      %s (namespace: %s)\n", cfg.NetworkName, cfg.NetworkSlug)
		}
		if len(cfg.Labels) > 0 {
			fmt.Printf("  Labels:       %v\n", cfg.Labels)
		}
		if cfg.AgentAPIKey != "" {
			fmt.Printf("  Agent Key:    %s (per-agent rate limiting)\n", cfg.AgentAPIKeyPrefix)
		}
		if state.LastSync != "" {
			fmt.Printf("  Last Sync:    %s\n", state.LastSync)
		}
		if state.LastError != "" {
			fmt.Printf("  Last Error:   %s\n", state.LastError)
		}
	} else {
		fmt.Printf("  Enrolled:     No\n")
		fmt.Printf("  Run 'nano-agent login' then 'sudo nano-agent enroll' to register this node\n")
	}
	fmt.Println()

	// VPP status
	fmt.Printf("VPP Data Plane Status\n")
	fmt.Printf("---------------------\n")
	vppStatus := checkVPPStatus()
	if vppStatus.Running {
		fmt.Printf("  Status:       Running\n")
		if vppStatus.Version != "" {
			fmt.Printf("  Version:      %s\n", vppStatus.Version)
		}
		fmt.Printf("  Interfaces:   %d\n", vppStatus.Interfaces)
		if len(vppStatus.InterfaceList) > 0 {
			for _, iface := range vppStatus.InterfaceList {
				fmt.Printf("    - %s\n", iface)
			}
		}
	} else {
		fmt.Printf("  Status:       Not running\n")
		fmt.Printf("  (VPP/vppctl not found or not accessible)\n")
	}
	fmt.Println()

	// Control plane connectivity
	if state.Enrolled && cfg != nil {
		fmt.Printf("Control Plane Connectivity\n")
		fmt.Printf("--------------------------\n")

		// Use mTLS if certificates are available
		var client *agent.Client
		if cfg.CertFile != "" && cfg.KeyFile != "" && cfg.CAFile != "" {
			if _, statErr := os.Stat(cfg.CertFile); statErr == nil {
				client, _ = agent.NewClientWithMTLS(cfg.APIURL, cfg.CertFile, cfg.KeyFile, cfg.CAFile)
			}
		}
		if client == nil {
			client = agent.NewClient(cfg.APIURL, "")
		}

		if err := client.CheckAPIHealth(); err != nil {
			fmt.Printf("  API Status:   Unreachable (%v)\n", err)
		} else {
			fmt.Printf("  API Status:   Connected\n")
		}
	}

	return nil
}

func runUnenroll(cmd *cobra.Command, args []string) error {
	state, _ := agent.LoadState(configDir)
	if state == nil || !state.Enrolled {
		return fmt.Errorf("node is not enrolled")
	}

	fmt.Printf("Removing node enrollment...\n")

	// Clear state
	newState := &agent.State{
		Enrolled: false,
	}

	if err := agent.SaveState(configDir, newState); err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	// Remove config file
	configFile := configDir + "/" + agent.DefaultConfigFile
	if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: failed to remove config file: %v\n", err)
	}

	// Remove certificates
	for _, file := range []string{"client.crt", "client.key", "ca.crt"} {
		path := configDir + "/" + file
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: failed to remove %s: %v\n", file, err)
		}
	}

	fmt.Printf("✓ Node unenrolled successfully\n")
	return nil
}

func checkVPPStatus() *agent.VPPStatus {
	status := &agent.VPPStatus{
		Running: false,
	}

	// Check if vppctl is available
	vppctlPath, err := exec.LookPath("vppctl")
	if err != nil {
		return status
	}

	// Check if VPP is running
	out, err := exec.Command(vppctlPath, "show", "version").Output()
	if err != nil {
		return status
	}

	status.Running = true
	status.Version = strings.TrimSpace(string(out))

	// Get interface list
	out, err = exec.Command(vppctlPath, "show", "interface").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "Name") && !strings.HasPrefix(line, "local0") {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					status.InterfaceList = append(status.InterfaceList, fields[0])
					status.Interfaces++
				}
			}
		}
	}

	return status
}

// runDaemon runs the agent in daemon mode
func runDaemon(cmd *cobra.Command, args []string) error {
	fmt.Printf("Nanoncore Edge Agent v%s\n", version)
	fmt.Printf("========================\n\n")

	// Load state
	state, err := agent.LoadState(configDir)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if !state.Enrolled {
		return fmt.Errorf("agent not enrolled. Run 'nano-agent enroll' first")
	}

	// Load config
	cfg, err := agent.LoadConfig(configDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Node ID:            %s\n", cfg.NodeID)
	fmt.Printf("API URL:            %s\n", cfg.APIURL)
	fmt.Printf("Heartbeat Interval: %s\n", heartbeatInterval)
	fmt.Printf("Config Sync:        %s\n", configSyncInterval)
	fmt.Printf("OLT Polling:        %v (workers: %d)\n", enableOLTPolling, pollerWorkers)
	fmt.Println()

	// Create API client - prefer agent API key, then mTLS, then user API key
	var client *agent.Client

	// Priority 1: Agent API key (na_ prefix) for per-agent rate limiting
	if cfg.AgentAPIKey != "" {
		fmt.Printf("Using agent API key authentication (%s)\n", cfg.AgentAPIKeyPrefix)
		client = agent.NewClientWithAgentKey(cfg.APIURL, cfg.AgentAPIKey, cfg.AgentID)
	}

	// Priority 2: mTLS certificates
	if client == nil && cfg.CertFile != "" && cfg.KeyFile != "" && cfg.CAFile != "" {
		// Check if certificate files exist
		if _, err := os.Stat(cfg.CertFile); err == nil {
			fmt.Printf("Using mTLS authentication\n")
			client, err = agent.NewClientWithMTLS(cfg.APIURL, cfg.CertFile, cfg.KeyFile, cfg.CAFile)
			if err != nil {
				return fmt.Errorf("failed to create mTLS client: %w", err)
			}
		} else {
			fmt.Printf("Warning: Certificate file not found\n")
		}
	}

	// Priority 3: User API key (nk_ prefix) - legacy mode, will signal rotation
	if client == nil {
		creds, _ := agent.LoadCredentials(configDir)
		if creds != nil && creds.APIKey != "" {
			fmt.Printf("Using user API key authentication (legacy - will upgrade to agent key)\n")
			client = agent.NewClientWithAPIKey(cfg.APIURL, creds.APIKey)
		} else {
			fmt.Printf("Warning: No authentication configured, using unauthenticated client\n")
			client = agent.NewClient(cfg.APIURL, "")
		}
	}

	// Check initial connectivity
	fmt.Printf("Checking control plane connectivity... ")
	if err := client.CheckAPIHealth(); err != nil {
		fmt.Printf("FAILED\n")
		fmt.Printf("Warning: Cannot reach control plane: %v\n", err)
		fmt.Printf("Will retry in background...\n")
	} else {
		fmt.Printf("OK\n")
	}

	fmt.Printf("\nStarting daemon...\n")
	fmt.Printf("Press Ctrl+C to stop\n\n")

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start heartbeat ticker
	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	// Start config sync ticker
	configSyncTicker := time.NewTicker(configSyncInterval)
	defer configSyncTicker.Stop()

	// Create OLT poller if enabled
	var oltPoller *poller.Poller
	if enableOLTPolling {
		pollerCfg := &poller.Config{
			WorkerCount:    pollerWorkers,
			CheckInterval:  10 * time.Second,
			MaxBackoff:     5 * time.Minute,
			ConnectTimeout: 30 * time.Second,
			LogPrefix:      "[olt-poller]",
		}
		adapter := poller.NewClientAdapter(client)
		oltPoller = poller.New(adapter, adapter, adapter, pollerCfg)
		oltPoller.Start(ctx)
		fmt.Printf("[%s] OLT poller started with %d workers\n", time.Now().Format("15:04:05"), pollerWorkers)
	}

	// Send initial heartbeat
	sendHeartbeat(client, cfg.NodeID, state, cfg)

	// Perform initial config sync (this also populates the poller with OLTs)
	syncConfigWithPoller(client, cfg.NodeID, state, cfg, oltPoller)

	// Main loop
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("\nContext cancelled, shutting down...\n")
			if oltPoller != nil {
				oltPoller.Stop()
			}
			return nil

		case sig := <-sigChan:
			fmt.Printf("\nReceived signal %v, shutting down gracefully...\n", sig)

			// Stop OLT poller
			if oltPoller != nil {
				oltPoller.Stop()
			}

			// Send final status update
			finalState := state
			finalState.LastSync = time.Now().UTC().Format(time.RFC3339)
			_ = agent.SaveState(configDir, finalState)

			fmt.Printf("Agent stopped\n")
			return nil

		case <-heartbeatTicker.C:
			sendHeartbeat(client, cfg.NodeID, state, cfg)

		case <-configSyncTicker.C:
			syncConfigWithPoller(client, cfg.NodeID, state, cfg, oltPoller)
		}
	}
}

// sendHeartbeat sends a heartbeat to the control plane
func sendHeartbeat(client *agent.Client, nodeID string, state *agent.State, cfg *agent.Config) {
	vppStatus := checkVPPStatus()

	req := &agent.HeartbeatRequest{
		NodeID:    nodeID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		VPPStatus: vppStatus,
	}

	resp, err := client.Heartbeat(req)
	if err != nil {
		fmt.Printf("[%s] Heartbeat failed: %v\n", time.Now().Format("15:04:05"), err)
		state.LastError = err.Error()
		return
	}

	if !resp.Acknowledged {
		fmt.Printf("[%s] Heartbeat not acknowledged: %s\n", time.Now().Format("15:04:05"), resp.Message)
		return
	}

	// Update state
	state.LastSync = time.Now().UTC().Format(time.RFC3339)
	state.LastError = ""
	_ = agent.SaveState(configDir, state)

	// Check if config update needed
	if resp.ConfigUpdate {
		fmt.Printf("[%s] Configuration update available\n", time.Now().Format("15:04:05"))
	}

	// Check if key rotation is needed (server signaled via header)
	if client.NeedsKeyRotation() {
		handleKeyRotation(client, cfg)
	}

	// Log successful heartbeat (only on debug or first success)
	fmt.Printf("[%s] Heartbeat OK (VPP: %v, interfaces: %d)\n",
		time.Now().Format("15:04:05"), vppStatus.Running, vppStatus.Interfaces)
}

// syncConfigWithPoller retrieves configuration from the control plane and updates the OLT poller
func syncConfigWithPoller(client *agent.Client, nodeID string, state *agent.State, cfg *agent.Config, oltPoller *poller.Poller) {
	// Get typed OLT config
	oltConfig, err := client.GetOLTConfig(nodeID)
	if err != nil {
		fmt.Printf("[%s] Config sync failed: %v\n", time.Now().Format("15:04:05"), err)
		return
	}

	// Log config sync
	fmt.Printf("[%s] Config synced (version: %d, OLTs: %d)\n",
		time.Now().Format("15:04:05"), oltConfig.Version, len(oltConfig.OLTs))

	// Update OLT poller with new config
	if oltPoller != nil && len(oltConfig.OLTs) > 0 {
		pollerConfigs := poller.ConvertOLTConfigs(oltConfig.OLTs)
		oltPoller.UpdateOLTs(pollerConfigs)
		fmt.Printf("[%s] Updated poller with %d OLTs\n", time.Now().Format("15:04:05"), len(pollerConfigs))
	}

	// Check if key rotation is needed (server signaled via header)
	if client.NeedsKeyRotation() {
		handleKeyRotation(client, cfg)
	}
}

// handleKeyRotation handles the key rotation process when server signals it's required
func handleKeyRotation(client *agent.Client, cfg *agent.Config) {
	fmt.Printf("[%s] Server requested API key rotation...\n", time.Now().Format("15:04:05"))

	// Request new key from server
	rotateResp, err := client.RotateAgentKey()
	if err != nil {
		fmt.Printf("[%s] Key rotation failed: %v\n", time.Now().Format("15:04:05"), err)
		return
	}

	if !rotateResp.Success {
		fmt.Printf("[%s] Key rotation rejected: %s\n", time.Now().Format("15:04:05"), rotateResp.Message)
		return
	}

	// Update local config with new key
	cfg.AgentID = rotateResp.AgentID
	cfg.AgentAPIKey = rotateResp.AgentAPIKey
	cfg.AgentAPIKeyPrefix = rotateResp.AgentAPIKeyPrefix

	// Save updated config
	if err := agent.SaveConfig(configDir, cfg); err != nil {
		fmt.Printf("[%s] Failed to save rotated key: %v\n", time.Now().Format("15:04:05"), err)
		return
	}

	// Update client to use new key
	client.UpdateToken(rotateResp.AgentAPIKey)
	client.SetAgentID(rotateResp.AgentID)

	fmt.Printf("[%s] API key rotated successfully (new: %s, old valid until: %s)\n",
		time.Now().Format("15:04:05"),
		rotateResp.AgentAPIKeyPrefix,
		rotateResp.OldKeyValidUntil)
}

// runLogin handles the login command
func runLogin(cmd *cobra.Command, args []string) error {
	fmt.Printf("Nanoncore Login\n")
	fmt.Printf("===============\n\n")

	// Check if already logged in
	existingCreds, _ := agent.LoadCredentials(configDir)
	if existingCreds != nil && existingCreds.APIKey != "" {
		fmt.Printf("You are already logged in as %s (%s)\n", existingCreds.UserEmail, existingCreds.UserID)
		fmt.Printf("Use 'nano-agent logout' first to login as a different user.\n")
		return nil
	}

	apiKey := loginAPIKey
	if apiKey == "" {
		// Prompt for API key
		fmt.Printf("Get your API key from the Nanoncore dashboard:\n")
		fmt.Printf("  %s/settings/api-keys\n\n", strings.TrimSuffix(loginAPIURL, "/api"))
		fmt.Printf("Enter your API key: ")

		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		apiKey = strings.TrimSpace(input)
	}

	if apiKey == "" {
		return fmt.Errorf("API key is required")
	}

	// Validate the API key
	fmt.Printf("\nValidating API key... ")
	client := agent.NewClientWithAPIKey(loginAPIURL, apiKey)

	validateResp, err := client.ValidateAPIKey()
	if err != nil {
		fmt.Printf("FAILED\n")
		return fmt.Errorf("failed to validate API key: %w", err)
	}

	if !validateResp.Valid {
		fmt.Printf("INVALID\n")
		return fmt.Errorf("invalid API key: %s", validateResp.Message)
	}
	fmt.Printf("OK\n")

	// Save credentials
	fmt.Printf("Saving credentials... ")
	creds := &agent.Credentials{
		APIKey:        apiKey,
		UserID:        validateResp.UserID,
		UserEmail:     validateResp.UserEmail,
		LoggedInAt:    time.Now().UTC().Format(time.RFC3339),
		DefaultAPIURL: loginAPIURL,
	}

	if err := agent.SaveCredentials(configDir, creds); err != nil {
		fmt.Printf("FAILED\n")
		return fmt.Errorf("failed to save credentials: %w", err)
	}
	fmt.Printf("OK\n")

	fmt.Printf("\n✓ Logged in as %s\n", validateResp.UserEmail)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  - Enroll this node: sudo nano-agent enroll --node-id YOUR_NODE_ID\n")
	fmt.Printf("  - Check status:     nano-agent whoami\n")

	return nil
}

// runLogout handles the logout command
func runLogout(cmd *cobra.Command, args []string) error {
	creds, _ := agent.LoadCredentials(configDir)
	if creds == nil || creds.APIKey == "" {
		fmt.Printf("Not logged in.\n")
		return nil
	}

	if err := agent.DeleteCredentials(configDir); err != nil {
		return fmt.Errorf("failed to remove credentials: %w", err)
	}

	fmt.Printf("✓ Logged out successfully\n")
	return nil
}

// runWhoami handles the whoami command
func runWhoami(cmd *cobra.Command, args []string) error {
	creds, err := agent.LoadCredentials(configDir)
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	if creds == nil || creds.APIKey == "" {
		fmt.Printf("Not logged in.\n")
		fmt.Printf("Run 'nano-agent login' to authenticate.\n")
		return nil
	}

	fmt.Printf("Logged in as:\n")
	fmt.Printf("  Email:     %s\n", creds.UserEmail)
	fmt.Printf("  User ID:   %s\n", creds.UserID)
	fmt.Printf("  API URL:   %s\n", creds.DefaultAPIURL)
	fmt.Printf("  Since:     %s\n", creds.LoggedInAt)

	// Validate the API key is still valid
	client := agent.NewClientWithAPIKey(creds.DefaultAPIURL, creds.APIKey)
	validateResp, err := client.ValidateAPIKey()
	if err != nil {
		fmt.Printf("  Status:    Unable to verify (%v)\n", err)
	} else if !validateResp.Valid {
		fmt.Printf("  Status:    API key expired or revoked\n")
	} else {
		fmt.Printf("  Status:    Active\n")
	}

	return nil
}

// promptSelection displays a list of options and returns the user's choice
func promptSelection(prompt string, options []string) (int, error) {
	fmt.Printf("\n%s\n", prompt)
	for i, opt := range options {
		fmt.Printf("  [%d] %s\n", i+1, opt)
	}
	fmt.Printf("\nEnter number (1-%d): ", len(options))

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return -1, fmt.Errorf("failed to read input: %w", err)
	}

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(options) {
		return -1, fmt.Errorf("invalid selection")
	}

	return choice - 1, nil
}

// promptString prompts for a string input with a default value
func promptString(prompt string, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}
