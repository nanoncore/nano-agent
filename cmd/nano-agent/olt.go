package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	southbound "github.com/nanoncore/nano-southbound"
	"github.com/nanoncore/nano-southbound/types"
	"github.com/spf13/cobra"
)

// OLT connection flags
var (
	oltVendor    string
	oltAddress   string
	oltPort      int
	oltProtocol  string
	oltUsername  string
	oltPassword  string
	oltTLS       bool
	oltTLSSkipVe bool
	outputJSON   bool
)

// Discover flags
var (
	discoverPONPorts []string
)

// Diagnose flags
var (
	diagnosePONPort string
	diagnoseONUID   int
	diagnoseSerial  string
)

var discoverCmd = &cobra.Command{
	Use:   "discover [olt-id]",
	Short: "Discover unprovisioned ONUs on an OLT",
	Long: `Discover unprovisioned ONUs that have registered but are not yet provisioned.

This command connects to the OLT and retrieves a list of ONUs that have been
discovered (seen on the PON network) but not yet provisioned with a service.

Examples:
  # Discover all ONUs on all PON ports
  nano-agent discover --vendor vsol --address 192.168.1.1 --username admin --password admin123

  # Discover ONUs on specific PON ports
  nano-agent discover --vendor cdata --address 10.0.0.1 --pon-port 0/1 --pon-port 0/2

  # Output as JSON for scripting
  nano-agent discover --vendor vsol --address 192.168.1.1 --json`,
	RunE: runDiscover,
}

var diagnoseCmd = &cobra.Command{
	Use:   "diagnose <serial|pon-port:onu-id>",
	Short: "Run diagnostics on a specific ONU",
	Long: `Perform comprehensive diagnostics on a specific ONU.

Returns optical power readings, traffic statistics, configuration info,
and any active alarms for the ONU.

Examples:
  # Diagnose by serial number
  nano-agent diagnose VSOL12345678 --vendor vsol --address 192.168.1.1

  # Diagnose by PON port and ONU ID
  nano-agent diagnose --pon-port 0/1 --onu-id 5 --vendor cdata --address 10.0.0.1

  # Show only optical readings
  nano-agent diagnose VSOL12345678 --vendor vsol --address 192.168.1.1 --optical-only

  # Output as JSON
  nano-agent diagnose VSOL12345678 --vendor vsol --address 192.168.1.1 --json`,
	RunE: runDiagnose,
}

var oltStatusCmd = &cobra.Command{
	Use:   "olt-status",
	Short: "Show OLT status and health",
	Long: `Display comprehensive status information for an OLT.

Shows system information, PON port status, ONU counts, and resource utilization.

Examples:
  nano-agent olt-status --vendor vsol --address 192.168.1.1
  nano-agent olt-status --vendor cdata --address 10.0.0.1 --json`,
	RunE: runOLTStatus,
}

var onuListCmd = &cobra.Command{
	Use:   "onu-list",
	Short: "List all provisioned ONUs on an OLT",
	Long: `List all provisioned ONUs on an OLT with optional filtering.

Examples:
  # List all ONUs
  nano-agent onu-list --vendor vsol --address 192.168.1.1

  # List only online ONUs
  nano-agent onu-list --vendor vsol --address 192.168.1.1 --status online

  # List ONUs on a specific PON port
  nano-agent onu-list --vendor cdata --address 10.0.0.1 --pon-port 1/1/1

  # Output as JSON
  nano-agent onu-list --vendor vsol --address 192.168.1.1 --json`,
	RunE: runONUList,
}

var onuListStatus string
var onuListPONPort string

func init() {
	// Common OLT connection flags for all OLT commands
	for _, cmd := range []*cobra.Command{discoverCmd, diagnoseCmd, oltStatusCmd, onuListCmd} {
		cmd.Flags().StringVar(&oltVendor, "vendor", "", "OLT vendor (vsol, cdata, nokia, huawei, zte, etc.) [required]")
		cmd.Flags().StringVar(&oltAddress, "address", "", "OLT management IP address [required]")
		cmd.Flags().IntVar(&oltPort, "port", 0, "OLT management port (default based on protocol)")
		cmd.Flags().StringVar(&oltProtocol, "protocol", "", "Management protocol (cli, netconf, gnmi, snmp)")
		cmd.Flags().StringVar(&oltUsername, "username", "", "OLT username [required]")
		cmd.Flags().StringVar(&oltPassword, "password", "", "OLT password [required]")
		cmd.Flags().BoolVar(&oltTLS, "tls", false, "Enable TLS")
		cmd.Flags().BoolVar(&oltTLSSkipVe, "tls-skip-verify", false, "Skip TLS verification (insecure)")
		cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")

		cmd.MarkFlagRequired("vendor")
		cmd.MarkFlagRequired("address")
		cmd.MarkFlagRequired("username")
		cmd.MarkFlagRequired("password")
	}

	// Discover-specific flags
	discoverCmd.Flags().StringSliceVar(&discoverPONPorts, "pon-port", nil, "Filter by PON port (can be specified multiple times)")

	// Diagnose-specific flags
	diagnoseCmd.Flags().StringVar(&diagnosePONPort, "pon-port", "", "PON port (required if not using serial)")
	diagnoseCmd.Flags().IntVar(&diagnoseONUID, "onu-id", 0, "ONU ID (required if not using serial)")
	diagnoseCmd.Flags().StringVar(&diagnoseSerial, "serial", "", "ONU serial number")

	// ONU list flags
	onuListCmd.Flags().StringVar(&onuListStatus, "status", "", "Filter by status (online, offline, all)")
	onuListCmd.Flags().StringVar(&onuListPONPort, "pon-port", "", "Filter by PON port")

	// Add commands to root
	rootCmd.AddCommand(discoverCmd)
	rootCmd.AddCommand(diagnoseCmd)
	rootCmd.AddCommand(oltStatusCmd)
	rootCmd.AddCommand(onuListCmd)
}

// createOLTDriver creates a driver connection to the OLT
func createOLTDriver() (types.Driver, error) {
	vendor := types.Vendor(strings.ToLower(oltVendor))
	protocol := types.Protocol(strings.ToLower(oltProtocol))

	config := &types.EquipmentConfig{
		Name:          "cli-session",
		Vendor:        vendor,
		Address:       oltAddress,
		Port:          oltPort,
		Protocol:      protocol,
		Username:      oltUsername,
		Password:      oltPassword,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}

	driver, err := southbound.NewDriver(vendor, protocol, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create driver: %w", err)
	}

	return driver, nil
}

func runDiscover(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("ONU Discovery\n")
		fmt.Printf("=============\n\n")
		fmt.Printf("OLT: %s (%s)\n\n", oltAddress, oltVendor)
	}

	// Create driver
	driver, err := createOLTDriver()
	if err != nil {
		return err
	}

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := &types.EquipmentConfig{
		Name:          "cli-session",
		Vendor:        types.Vendor(strings.ToLower(oltVendor)),
		Address:       oltAddress,
		Port:          oltPort,
		Username:      oltUsername,
		Password:      oltPassword,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}

	if !outputJSON {
		fmt.Printf("Connecting to OLT... ")
	}
	if err := driver.Connect(ctx, config); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer driver.Disconnect(ctx)
	if !outputJSON {
		fmt.Printf("OK\n")
	}

	// Check if driver supports DriverV2
	driverV2, ok := driver.(types.DriverV2)
	if !ok {
		return fmt.Errorf("driver for vendor %s does not support ONU discovery", oltVendor)
	}

	// Discover ONUs
	if !outputJSON {
		fmt.Printf("Discovering ONUs... ")
	}
	discoveries, err := driverV2.DiscoverONUs(ctx, discoverPONPorts)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("discovery failed: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK (%d found)\n\n", len(discoveries))
	}

	// Output results
	if outputJSON {
		output, _ := json.MarshalIndent(discoveries, "", "  ")
		fmt.Println(string(output))
	} else {
		if len(discoveries) == 0 {
			fmt.Println("No unprovisioned ONUs found.")
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "PON Port\tSerial\tMAC\tModel\tDistance\tRx Power\tDiscovered")
			fmt.Fprintln(w, "--------\t------\t---\t-----\t--------\t--------\t----------")
			for _, d := range discoveries {
				dist := "-"
				if d.DistanceM > 0 {
					dist = fmt.Sprintf("%dm", d.DistanceM)
				}
				rx := "-"
				if d.RxPowerDBm != 0 {
					rx = fmt.Sprintf("%.1f dBm", d.RxPowerDBm)
				}
				mac := d.MAC
				if mac == "" {
					mac = "-"
				}
				model := d.Model
				if model == "" {
					model = "-"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					d.PONPort, d.Serial, mac, model, dist, rx, d.DiscoveredAt.Format("15:04:05"))
			}
			w.Flush()
		}
	}

	return nil
}

func runDiagnose(cmd *cobra.Command, args []string) error {
	// Determine the target ONU
	serial := diagnoseSerial
	if len(args) > 0 {
		serial = args[0]
	}

	// Need either serial or pon-port+onu-id
	if serial == "" && (diagnosePONPort == "" || diagnoseONUID == 0) {
		return fmt.Errorf("either provide a serial number as argument, or use --pon-port and --onu-id flags")
	}

	if !outputJSON {
		fmt.Printf("ONU Diagnostics\n")
		fmt.Printf("===============\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		if serial != "" {
			fmt.Printf("ONU: %s\n\n", serial)
		} else {
			fmt.Printf("ONU: %s ONU %d\n\n", diagnosePONPort, diagnoseONUID)
		}
	}

	// Create driver
	driver, err := createOLTDriver()
	if err != nil {
		return err
	}

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := &types.EquipmentConfig{
		Name:          "cli-session",
		Vendor:        types.Vendor(strings.ToLower(oltVendor)),
		Address:       oltAddress,
		Port:          oltPort,
		Username:      oltUsername,
		Password:      oltPassword,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}

	if !outputJSON {
		fmt.Printf("Connecting to OLT... ")
	}
	if err := driver.Connect(ctx, config); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer driver.Disconnect(ctx)
	if !outputJSON {
		fmt.Printf("OK\n")
	}

	// Check if driver supports DriverV2
	driverV2, ok := driver.(types.DriverV2)
	if !ok {
		return fmt.Errorf("driver for vendor %s does not support diagnostics", oltVendor)
	}

	// If we have a serial, find the ONU first to get PON port and ID
	ponPort := diagnosePONPort
	onuID := diagnoseONUID
	if serial != "" {
		if !outputJSON {
			fmt.Printf("Looking up ONU by serial... ")
		}
		onu, err := driverV2.GetONUBySerial(ctx, serial)
		if err != nil {
			if !outputJSON {
				fmt.Printf("FAILED\n")
			}
			return fmt.Errorf("failed to find ONU: %w", err)
		}
		if onu == nil {
			if !outputJSON {
				fmt.Printf("NOT FOUND\n")
			}
			return fmt.Errorf("ONU with serial %s not found", serial)
		}
		ponPort = onu.PONPort
		onuID = onu.ONUID
		if !outputJSON {
			fmt.Printf("OK (port %s, id %d)\n", ponPort, onuID)
		}
	}

	// Run diagnostics
	if !outputJSON {
		fmt.Printf("Running diagnostics... ")
	}
	diag, err := driverV2.RunDiagnostics(ctx, ponPort, onuID)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("diagnostics failed: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}

	// Output results
	if outputJSON {
		output, _ := json.MarshalIndent(diag, "", "  ")
		fmt.Println(string(output))
	} else {
		fmt.Printf("ONU Information\n")
		fmt.Printf("---------------\n")
		fmt.Printf("  Serial:          %s\n", diag.Serial)
		fmt.Printf("  PON Port:        %s\n", diag.PONPort)
		fmt.Printf("  ONU ID:          %d\n", diag.ONUID)
		fmt.Printf("  Admin State:     %s\n", diag.AdminState)
		fmt.Printf("  Oper State:      %s\n", diag.OperState)
		fmt.Println()

		if diag.Power != nil {
			fmt.Printf("Optical Power\n")
			fmt.Printf("-------------\n")
			fmt.Printf("  ONU Tx:          %.1f dBm", diag.Power.TxPowerDBm)
			if diag.Power.TxPowerDBm < types.GPONTxLowThreshold || diag.Power.TxPowerDBm > types.GPONTxHighThreshold {
				fmt.Printf(" [OUT OF SPEC]")
			}
			fmt.Println()
			fmt.Printf("  ONU Rx:          %.1f dBm", diag.Power.RxPowerDBm)
			if diag.Power.RxPowerDBm < types.GPONRxLowThreshold || diag.Power.RxPowerDBm > types.GPONRxHighThreshold {
				fmt.Printf(" [OUT OF SPEC]")
			}
			fmt.Println()
			fmt.Printf("  OLT Rx:          %.1f dBm\n", diag.Power.OLTRxDBm)
			if diag.Power.DistanceM > 0 {
				fmt.Printf("  Distance:        %d m\n", diag.Power.DistanceM)
			}
			if diag.Power.IsWithinSpec {
				fmt.Printf("  Status:          Within spec\n")
			} else {
				fmt.Printf("  Status:          OUT OF SPEC - check fiber\n")
			}
			fmt.Println()
		}

		fmt.Printf("Configuration\n")
		fmt.Printf("-------------\n")
		if diag.LineProfile != "" {
			fmt.Printf("  Line Profile:    %s\n", diag.LineProfile)
		}
		if diag.ServiceProfile != "" {
			fmt.Printf("  Service Profile: %s\n", diag.ServiceProfile)
		}
		if diag.VLAN > 0 {
			fmt.Printf("  VLAN:            %d\n", diag.VLAN)
		}
		if diag.BandwidthDown > 0 || diag.BandwidthUp > 0 {
			fmt.Printf("  Bandwidth:       %d/%d kbps (down/up)\n", diag.BandwidthDown, diag.BandwidthUp)
		}
		fmt.Println()

		fmt.Printf("Traffic Statistics\n")
		fmt.Printf("------------------\n")
		fmt.Printf("  Bytes Up:        %d\n", diag.BytesUp)
		fmt.Printf("  Bytes Down:      %d\n", diag.BytesDown)
		fmt.Printf("  Errors:          %d\n", diag.Errors)
		fmt.Printf("  Drops:           %d\n", diag.Drops)
		fmt.Println()

		if len(diag.Alarms) > 0 {
			fmt.Printf("Active Alarms\n")
			fmt.Printf("-------------\n")
			for _, alarm := range diag.Alarms {
				fmt.Printf("  - %s\n", alarm)
			}
			fmt.Println()
		}

		fmt.Printf("Collected at: %s\n", diag.Timestamp.Format("2006-01-02 15:04:05"))
	}

	return nil
}

func runOLTStatus(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("OLT Status\n")
		fmt.Printf("==========\n\n")
	}

	// Create driver
	driver, err := createOLTDriver()
	if err != nil {
		return err
	}

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := &types.EquipmentConfig{
		Name:          "cli-session",
		Vendor:        types.Vendor(strings.ToLower(oltVendor)),
		Address:       oltAddress,
		Port:          oltPort,
		Username:      oltUsername,
		Password:      oltPassword,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}

	if !outputJSON {
		fmt.Printf("Connecting to OLT... ")
	}
	if err := driver.Connect(ctx, config); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer driver.Disconnect(ctx)
	if !outputJSON {
		fmt.Printf("OK\n")
	}

	// Check if driver supports DriverV2
	driverV2, ok := driver.(types.DriverV2)
	if !ok {
		return fmt.Errorf("driver for vendor %s does not support OLT status", oltVendor)
	}

	// Get OLT status
	if !outputJSON {
		fmt.Printf("Getting OLT status... ")
	}
	status, err := driverV2.GetOLTStatus(ctx)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to get status: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}

	// Output results
	if outputJSON {
		output, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(output))
	} else {
		fmt.Printf("System Information\n")
		fmt.Printf("------------------\n")
		fmt.Printf("  Vendor:          %s\n", status.Vendor)
		fmt.Printf("  Model:           %s\n", status.Model)
		if status.Firmware != "" {
			fmt.Printf("  Firmware:        %s\n", status.Firmware)
		}
		if status.SerialNumber != "" {
			fmt.Printf("  Serial:          %s\n", status.SerialNumber)
		}
		fmt.Printf("  Reachable:       %v\n", status.IsReachable)
		fmt.Printf("  Healthy:         %v\n", status.IsHealthy)
		if status.UptimeSeconds > 0 {
			uptime := time.Duration(status.UptimeSeconds) * time.Second
			fmt.Printf("  Uptime:          %s\n", uptime)
		}
		fmt.Println()

		if status.CPUPercent > 0 || status.MemoryPercent > 0 {
			fmt.Printf("Resource Utilization\n")
			fmt.Printf("--------------------\n")
			if status.CPUPercent > 0 {
				fmt.Printf("  CPU:             %.1f%%\n", status.CPUPercent)
			}
			if status.MemoryPercent > 0 {
				fmt.Printf("  Memory:          %.1f%%\n", status.MemoryPercent)
			}
			if status.Temperature > 0 {
				fmt.Printf("  Temperature:     %.1f°C\n", status.Temperature)
			}
			fmt.Println()
		}

		fmt.Printf("ONU Summary\n")
		fmt.Printf("-----------\n")
		fmt.Printf("  Total ONUs:      %d\n", status.TotalONUs)
		fmt.Printf("  Active ONUs:     %d\n", status.ActiveONUs)
		fmt.Println()

		if len(status.PONPorts) > 0 {
			fmt.Printf("PON Ports\n")
			fmt.Printf("---------\n")
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "  Port\tAdmin\tOper\tONUs\tRx Power\tTx Power")
			fmt.Fprintln(w, "  ----\t-----\t----\t----\t--------\t--------")
			for _, port := range status.PONPorts {
				rx := "-"
				if port.RxPowerDBm != 0 {
					rx = fmt.Sprintf("%.1f dBm", port.RxPowerDBm)
				}
				tx := "-"
				if port.TxPowerDBm != 0 {
					tx = fmt.Sprintf("%.1f dBm", port.TxPowerDBm)
				}
				fmt.Fprintf(w, "  %s\t%s\t%s\t%d/%d\t%s\t%s\n",
					port.Port, port.AdminState, port.OperState,
					port.ONUCount, port.MaxONUs, rx, tx)
			}
			w.Flush()
			fmt.Println()
		}

		fmt.Printf("Last poll: %s\n", status.LastPoll.Format("2006-01-02 15:04:05"))
	}

	return nil
}

func runONUList(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("ONU List\n")
		fmt.Printf("========\n\n")
		fmt.Printf("OLT: %s (%s)\n\n", oltAddress, oltVendor)
	}

	// Create driver
	driver, err := createOLTDriver()
	if err != nil {
		return err
	}

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := &types.EquipmentConfig{
		Name:          "cli-session",
		Vendor:        types.Vendor(strings.ToLower(oltVendor)),
		Address:       oltAddress,
		Port:          oltPort,
		Username:      oltUsername,
		Password:      oltPassword,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}

	if !outputJSON {
		fmt.Printf("Connecting to OLT... ")
	}
	if err := driver.Connect(ctx, config); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer driver.Disconnect(ctx)
	if !outputJSON {
		fmt.Printf("OK\n")
	}

	// Check if driver supports DriverV2
	driverV2, ok := driver.(types.DriverV2)
	if !ok {
		return fmt.Errorf("driver for vendor %s does not support ONU listing", oltVendor)
	}

	// Build filter
	var filter *types.ONUFilter
	if onuListStatus != "" || onuListPONPort != "" {
		filter = &types.ONUFilter{
			Status:  onuListStatus,
			PONPort: onuListPONPort,
		}
	}

	// Get ONU list
	if !outputJSON {
		fmt.Printf("Getting ONU list... ")
	}
	onus, err := driverV2.GetONUList(ctx, filter)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to get ONU list: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK (%d found)\n\n", len(onus))
	}

	// Output results
	if outputJSON {
		output, _ := json.MarshalIndent(onus, "", "  ")
		fmt.Println(string(output))
	} else {
		if len(onus) == 0 {
			fmt.Println("No ONUs found.")
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "Port\tID\tSerial\tStatus\tRx Power\tProfile\tVLAN")
			fmt.Fprintln(w, "----\t--\t------\t------\t--------\t-------\t----")
			for _, onu := range onus {
				status := onu.OperState
				if onu.IsOnline {
					status = "online"
				}
				rx := "-"
				if onu.RxPowerDBm != 0 {
					rx = fmt.Sprintf("%.1f dBm", onu.RxPowerDBm)
				}
				profile := onu.LineProfile
				if profile == "" {
					profile = "-"
				}
				vlan := "-"
				if onu.VLAN > 0 {
					vlan = fmt.Sprintf("%d", onu.VLAN)
				}
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
					onu.PONPort, onu.ONUID, onu.Serial, status, rx, profile, vlan)
			}
			w.Flush()
		}
	}

	return nil
}
