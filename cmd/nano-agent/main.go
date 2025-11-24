package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nanoncore/nano-agent/pkg/agent"
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

The enrollment process:
1. Validates the enrollment token with the API
2. Downloads mTLS certificates for secure communication
3. Saves configuration to /etc/nano-agent/config.json
4. Marks the node as enrolled

Example:
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

// Enroll flags
var (
	enrollAPIURL string
	enrollToken  string
	enrollNodeID string
	enrollLabels string
)

// Run flags
var (
	heartbeatInterval time.Duration
	configSyncInterval time.Duration
)

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", agent.DefaultConfigDir,
		"Configuration directory")

	// Enroll flags
	enrollCmd.Flags().StringVar(&enrollAPIURL, "api", "", "Nanoncore API URL (required)")
	enrollCmd.Flags().StringVar(&enrollToken, "token", "", "Enrollment token (required)")
	enrollCmd.Flags().StringVar(&enrollNodeID, "node-id", "", "Unique node identifier (required)")
	enrollCmd.Flags().StringVar(&enrollLabels, "labels", "", "Node labels (key=value,key2=value2)")
	_ = enrollCmd.MarkFlagRequired("api")
	_ = enrollCmd.MarkFlagRequired("token")
	_ = enrollCmd.MarkFlagRequired("node-id")

	// Run flags
	runCmd.Flags().DurationVar(&heartbeatInterval, "heartbeat-interval", 30*time.Second,
		"Interval between heartbeats to control plane")
	runCmd.Flags().DurationVar(&configSyncInterval, "config-sync-interval", 5*time.Minute,
		"Interval between configuration syncs")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
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

	fmt.Printf("API URL:  %s\n", enrollAPIURL)
	fmt.Printf("Node ID:  %s\n", enrollNodeID)
	if len(labels) > 0 {
		fmt.Printf("Labels:   %v\n", labels)
	}
	fmt.Println()

	// Create client and check API health
	fmt.Printf("Checking API connectivity... ")
	client := agent.NewClient(enrollAPIURL, enrollToken)
	if err := client.CheckAPIHealth(); err != nil {
		fmt.Printf("FAILED\n")
		return fmt.Errorf("cannot reach API: %w", err)
	}
	fmt.Printf("OK\n")

	// Send enrollment request
	fmt.Printf("Enrolling node... ")
	enrollReq := &agent.EnrollRequest{
		NodeID: enrollNodeID,
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
	if resp.Certificate != "" && resp.PrivateKey != "" {
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
	}

	// Save configuration
	fmt.Printf("Saving configuration... ")
	cfg := &agent.Config{
		APIURL:   enrollAPIURL,
		NodeID:   enrollNodeID,
		Labels:   labels,
		CertFile: configDir + "/client.crt",
		KeyFile:  configDir + "/client.key",
		CAFile:   configDir + "/ca.crt",
	}

	if err := agent.SaveConfig(configDir, cfg); err != nil {
		fmt.Printf("FAILED\n")
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("OK\n")

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

	fmt.Printf("\n✓ Node '%s' successfully enrolled!\n", enrollNodeID)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  - Check status: sudo nano-agent status\n")
	fmt.Printf("  - View logs: journalctl -u nano-agent\n")

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
		if len(cfg.Labels) > 0 {
			fmt.Printf("  Labels:       %v\n", cfg.Labels)
		}
		if state.LastSync != "" {
			fmt.Printf("  Last Sync:    %s\n", state.LastSync)
		}
		if state.LastError != "" {
			fmt.Printf("  Last Error:   %s\n", state.LastError)
		}
	} else {
		fmt.Printf("  Enrolled:     No\n")
		fmt.Printf("  Run 'sudo nano-agent enroll' to register this node\n")
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
	fmt.Println()

	// Create API client with mTLS if certificates are available
	var client *agent.Client
	if cfg.CertFile != "" && cfg.KeyFile != "" && cfg.CAFile != "" {
		// Check if certificate files exist
		if _, err := os.Stat(cfg.CertFile); err == nil {
			fmt.Printf("Using mTLS authentication\n")
			client, err = agent.NewClientWithMTLS(cfg.APIURL, cfg.CertFile, cfg.KeyFile, cfg.CAFile)
			if err != nil {
				return fmt.Errorf("failed to create mTLS client: %w", err)
			}
		} else {
			fmt.Printf("Warning: Certificate file not found, using unauthenticated client\n")
			client = agent.NewClient(cfg.APIURL, "")
		}
	} else {
		client = agent.NewClient(cfg.APIURL, "")
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

	// Send initial heartbeat
	sendHeartbeat(client, cfg.NodeID, state)

	// Main loop
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("\nContext cancelled, shutting down...\n")
			return nil

		case sig := <-sigChan:
			fmt.Printf("\nReceived signal %v, shutting down gracefully...\n", sig)

			// Send final status update
			finalState := state
			finalState.LastSync = time.Now().UTC().Format(time.RFC3339)
			_ = agent.SaveState(configDir, finalState)

			fmt.Printf("Agent stopped\n")
			return nil

		case <-heartbeatTicker.C:
			sendHeartbeat(client, cfg.NodeID, state)

		case <-configSyncTicker.C:
			syncConfig(client, cfg.NodeID, state)
		}
	}
}

// sendHeartbeat sends a heartbeat to the control plane
func sendHeartbeat(client *agent.Client, nodeID string, state *agent.State) {
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

	// Log successful heartbeat (only on debug or first success)
	fmt.Printf("[%s] Heartbeat OK (VPP: %v, interfaces: %d)\n",
		time.Now().Format("15:04:05"), vppStatus.Running, vppStatus.Interfaces)
}

// syncConfig retrieves and applies configuration from the control plane
func syncConfig(client *agent.Client, nodeID string, state *agent.State) {
	config, err := client.GetConfig(nodeID)
	if err != nil {
		fmt.Printf("[%s] Config sync failed: %v\n", time.Now().Format("15:04:05"), err)
		return
	}

	// Log config sync
	fmt.Printf("[%s] Config synced (%d keys)\n", time.Now().Format("15:04:05"), len(config))

	// TODO: Apply configuration changes
	// - VPP configuration
	// - Routing updates
	// - QoS policies
	// etc.
}
