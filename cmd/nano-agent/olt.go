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
	"github.com/nanoncore/nano-southbound/model"
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

// ONU info flags
var (
	onuInfoSerial  string
	onuInfoPONPort string
	onuInfoONUID   int
)

// ONU provision flags
var (
	onuProvSerial      string
	onuProvPONPort     string
	onuProvONUID       int
	onuProvVLAN        int
	onuProvBandwidthUp int
	onuProvBandwidthDn int
	onuProvLineProfile string
	onuProvSrvProfile  string
	onuProvDescription string
	onuProvDryRun      bool
)

// ONU delete flags
var (
	onuDelSerial  string
	onuDelPONPort string
	onuDelONUID   int
	onuDelForce   bool
)

// ONU reboot flags
var (
	onuRebootSerial  string
	onuRebootPONPort string
	onuRebootONUID   int
)

// Port management flags
var (
	portPONPort string
	portForce   bool
)

var onuInfoCmd = &cobra.Command{
	Use:   "onu-info",
	Short: "Get detailed information about a specific ONU",
	Long: `Retrieve detailed information about a specific ONU including:
- Registration info (serial, model, MAC address)
- Optical power readings (Tx/Rx power, OLT Rx power)
- Connection status (admin state, oper state, uptime)
- Service configuration (profiles, VLAN, bandwidth)

The ONU can be identified by serial number OR by PON port and ONU ID.

Examples:
  # Get ONU info by serial number
  nano-agent onu-info --serial HWTC12345678 --vendor huawei --address 192.168.1.1

  # Get ONU info by PON port and ONU ID
  nano-agent onu-info --pon-port 0/0/1 --onu-id 101 --vendor huawei --address 192.168.1.1

  # Output as JSON for scripting
  nano-agent onu-info --serial HWTC12345678 --vendor huawei --address 192.168.1.1 --json`,
	RunE: runONUInfo,
}

var onuProvisionCmd = &cobra.Command{
	Use:   "onu-provision",
	Short: "Provision a new ONU on the OLT",
	Long: `Register and provision a new ONU on the OLT.

This command adds a new ONU to the OLT with the specified configuration including
VLAN assignment and bandwidth profile.

IMPORTANT: Use --dry-run to preview the operation before making changes.

Examples:
  # Provision with auto-assigned ONU ID
  nano-agent onu-provision --serial HWTC12345678 --vlan 100 \
    --bandwidth-down 100 --bandwidth-up 50 \
    --vendor huawei --address 192.168.1.1

  # Provision with specific PON port and ONU ID
  nano-agent onu-provision --serial HWTC12345678 --pon-port 0/0/1 --onu-id 101 \
    --vlan 100 --vendor huawei --address 192.168.1.1

  # Dry run to preview the provisioning
  nano-agent onu-provision --dry-run --serial HWTC12345678 --vlan 100 \
    --vendor huawei --address 192.168.1.1

  # Provision with line profile
  nano-agent onu-provision --serial HWTC12345678 --vlan 100 \
    --line-profile HSI_1G --vendor huawei --address 192.168.1.1`,
	RunE: runONUProvision,
}

var onuDeleteCmd = &cobra.Command{
	Use:   "onu-delete",
	Short: "Delete an ONU from the OLT",
	Long: `Remove an ONU from the OLT configuration.

This command deprovisions and removes an ONU from the OLT. The ONU can be
identified by serial number OR by PON port and ONU ID.

WARNING: This is a destructive operation. Use --force to confirm.

Examples:
  # Delete by serial number
  nano-agent onu-delete --serial HWTC12345678 --force \
    --vendor huawei --address 192.168.1.1

  # Delete by PON port and ONU ID
  nano-agent onu-delete --pon-port 0/0/1 --onu-id 101 --force \
    --vendor huawei --address 192.168.1.1`,
	RunE: runONUDelete,
}

var onuRebootCmd = &cobra.Command{
	Use:   "onu-reboot",
	Short: "Reboot a specific ONU",
	Long: `Remotely reboot an ONU.

This command triggers a remote reboot of the specified ONU. Useful for
troubleshooting connectivity issues or clearing stuck states.

The ONU can be identified by serial number OR by PON port and ONU ID.

Examples:
  # Reboot by serial number
  nano-agent onu-reboot --serial HWTC12345678 \
    --vendor huawei --address 192.168.1.1

  # Reboot by PON port and ONU ID
  nano-agent onu-reboot --pon-port 0/0/1 --onu-id 101 \
    --vendor huawei --address 192.168.1.1`,
	RunE: runONUReboot,
}

var portListCmd = &cobra.Command{
	Use:   "port-list",
	Short: "List all PON ports on an OLT",
	Long: `List all PON ports on an OLT with their status and ONU counts.

Displays admin state, operational state, ONU count, and optical power levels
for each PON port.

Examples:
  # List all PON ports
  nano-agent port-list --vendor huawei --address 192.168.1.1 \
    --port 161 --protocol snmp --username admin --password admin

  # Output as JSON
  nano-agent port-list --vendor huawei --address 192.168.1.1 \
    --port 161 --protocol snmp --username admin --password admin --json`,
	RunE: runPortList,
}

var portEnableCmd = &cobra.Command{
	Use:   "port-enable",
	Short: "Enable a PON port",
	Long: `Administratively enable a PON port on the OLT.

This command enables a previously disabled PON port, allowing it to
accept ONU connections and pass traffic.

Examples:
  # Enable a PON port
  nano-agent port-enable --pon-port 0/0/1 \
    --vendor huawei --address 192.168.1.1 --username admin --password admin`,
	RunE: runPortEnable,
}

var portDisableCmd = &cobra.Command{
	Use:   "port-disable",
	Short: "Disable a PON port",
	Long: `Administratively disable a PON port on the OLT.

WARNING: This will disconnect all ONUs on the port. Use --force to confirm.

This command disables a PON port, preventing ONU connections and traffic.

Examples:
  # Disable a PON port (requires --force)
  nano-agent port-disable --pon-port 0/0/1 --force \
    --vendor huawei --address 192.168.1.1 --username admin --password admin`,
	RunE: runPortDisable,
}

func init() {
	// Common OLT connection flags for all OLT commands
	oltCommands := []*cobra.Command{
		discoverCmd, diagnoseCmd, oltStatusCmd, onuListCmd,
		onuInfoCmd, onuProvisionCmd, onuDeleteCmd, onuRebootCmd,
		portListCmd, portEnableCmd, portDisableCmd,
	}
	for _, cmd := range oltCommands {
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

	// ONU info flags
	onuInfoCmd.Flags().StringVar(&onuInfoSerial, "serial", "", "ONU serial number")
	onuInfoCmd.Flags().StringVar(&onuInfoPONPort, "pon-port", "", "PON port (required if not using serial)")
	onuInfoCmd.Flags().IntVar(&onuInfoONUID, "onu-id", 0, "ONU ID (required if not using serial)")

	// ONU provision flags
	onuProvisionCmd.Flags().StringVar(&onuProvSerial, "serial", "", "ONU serial number [required]")
	onuProvisionCmd.Flags().StringVar(&onuProvPONPort, "pon-port", "", "Target PON port (auto-detect if not specified)")
	onuProvisionCmd.Flags().IntVar(&onuProvONUID, "onu-id", 0, "Target ONU ID (auto-assign if not specified)")
	onuProvisionCmd.Flags().IntVar(&onuProvVLAN, "vlan", 0, "VLAN ID [required]")
	onuProvisionCmd.Flags().IntVar(&onuProvBandwidthDn, "bandwidth-down", 100, "Download bandwidth in Mbps")
	onuProvisionCmd.Flags().IntVar(&onuProvBandwidthUp, "bandwidth-up", 50, "Upload bandwidth in Mbps")
	onuProvisionCmd.Flags().StringVar(&onuProvLineProfile, "line-profile", "", "Line profile name")
	onuProvisionCmd.Flags().StringVar(&onuProvSrvProfile, "service-profile", "", "Service profile name")
	onuProvisionCmd.Flags().StringVar(&onuProvDescription, "description", "", "Subscriber description")
	onuProvisionCmd.Flags().BoolVar(&onuProvDryRun, "dry-run", false, "Preview the operation without making changes")
	onuProvisionCmd.MarkFlagRequired("serial")
	onuProvisionCmd.MarkFlagRequired("vlan")

	// ONU delete flags
	onuDeleteCmd.Flags().StringVar(&onuDelSerial, "serial", "", "ONU serial number")
	onuDeleteCmd.Flags().StringVar(&onuDelPONPort, "pon-port", "", "PON port (required if not using serial)")
	onuDeleteCmd.Flags().IntVar(&onuDelONUID, "onu-id", 0, "ONU ID (required if not using serial)")
	onuDeleteCmd.Flags().BoolVar(&onuDelForce, "force", false, "Confirm the deletion")

	// ONU reboot flags
	onuRebootCmd.Flags().StringVar(&onuRebootSerial, "serial", "", "ONU serial number")
	onuRebootCmd.Flags().StringVar(&onuRebootPONPort, "pon-port", "", "PON port (required if not using serial)")
	onuRebootCmd.Flags().IntVar(&onuRebootONUID, "onu-id", 0, "ONU ID (required if not using serial)")

	// Port enable flags
	portEnableCmd.Flags().StringVar(&portPONPort, "pon-port", "", "PON port (e.g., 0/0/1) [required]")
	portEnableCmd.MarkFlagRequired("pon-port")

	// Port disable flags
	portDisableCmd.Flags().StringVar(&portPONPort, "pon-port", "", "PON port (e.g., 0/0/1) [required]")
	portDisableCmd.Flags().BoolVar(&portForce, "force", false, "Confirm the disable operation")
	portDisableCmd.MarkFlagRequired("pon-port")

	// Add commands to root
	rootCmd.AddCommand(discoverCmd)
	rootCmd.AddCommand(diagnoseCmd)
	rootCmd.AddCommand(oltStatusCmd)
	rootCmd.AddCommand(onuListCmd)
	rootCmd.AddCommand(onuInfoCmd)
	rootCmd.AddCommand(onuProvisionCmd)
	rootCmd.AddCommand(onuDeleteCmd)
	rootCmd.AddCommand(onuRebootCmd)
	rootCmd.AddCommand(portListCmd)
	rootCmd.AddCommand(portEnableCmd)
	rootCmd.AddCommand(portDisableCmd)
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

func runONUInfo(cmd *cobra.Command, args []string) error {
	if err := validateONUIdentifier(onuInfoSerial, onuInfoPONPort, onuInfoONUID); err != nil {
		return err
	}

	printONUInfoHeader()

	conn, err := connectToOLT(60)
	if err != nil {
		return err
	}
	defer conn.close()

	driverV2, err := conn.getDriverV2()
	if err != nil {
		return err
	}

	onu, err := findONU(conn.ctx, driverV2, onuInfoSerial, onuInfoPONPort, onuInfoONUID)
	if err != nil {
		return err
	}

	powerReading := getOptionalPowerReading(conn.ctx, driverV2, onu)

	return outputONUInfo(onu, powerReading)
}

// validateONUIdentifier validates that either serial or port+id is provided
func validateONUIdentifier(serial, ponPort string, onuID int) error {
	if serial == "" && (ponPort == "" || onuID == 0) {
		return fmt.Errorf("provide either --serial OR both --pon-port and --onu-id")
	}
	return nil
}

// printONUInfoHeader prints the command header
func printONUInfoHeader() {
	if !outputJSON {
		fmt.Printf("ONU Information\n")
		fmt.Printf("===============\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		if onuInfoSerial != "" {
			fmt.Printf("ONU: %s\n\n", onuInfoSerial)
		} else {
			fmt.Printf("ONU: %s ONU %d\n\n", onuInfoPONPort, onuInfoONUID)
		}
	}
}

// findONU looks up an ONU by serial or port/id
func findONU(ctx context.Context, driverV2 types.DriverV2, serial, ponPort string, onuID int) (*types.ONUInfo, error) {
	if serial != "" {
		return lookupONUBySerial(ctx, driverV2, serial)
	}
	return lookupONUByPortID(ctx, driverV2, ponPort, onuID)
}

// getOptionalPowerReading attempts to get power reading, returns nil on failure
func getOptionalPowerReading(ctx context.Context, driverV2 types.DriverV2, onu *types.ONUInfo) *types.ONUPowerReading {
	if !outputJSON {
		fmt.Printf("Getting optical power... ")
	}
	power, err := driverV2.GetONUPower(ctx, onu.PONPort, onu.ONUID)
	if err != nil {
		if !outputJSON {
			fmt.Printf("SKIPPED (not available)\n")
		}
		return nil
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}
	return power
}

// outputONUInfo outputs ONU info in JSON or human-readable format
func outputONUInfo(onu *types.ONUInfo, power *types.ONUPowerReading) error {
	if outputJSON {
		return outputONUInfoJSON(onu, power)
	}
	fmt.Println()
	printONURegistration(onu)
	printONUStatus(onu)
	printOpticalPower(onu, power)
	printServiceConfig(onu)
	if !onu.ProvisionedAt.IsZero() {
		fmt.Printf("Provisioned at: %s\n", onu.ProvisionedAt.Format("2006-01-02 15:04:05"))
	}
	return nil
}

// outputONUInfoJSON outputs ONU info as JSON
func outputONUInfoJSON(onu *types.ONUInfo, power *types.ONUPowerReading) error {
	output := struct {
		ONU   *types.ONUInfo         `json:"onu"`
		Power *types.ONUPowerReading `json:"power,omitempty"`
	}{ONU: onu, Power: power}
	data, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(data))
	return nil
}

func runONUProvision(cmd *cobra.Command, args []string) error {
	// Validate serial number format (NAN-158)
	if err := validateSerialNumber(onuProvSerial); err != nil {
		return err
	}

	printProvisionHeader(onuProvDryRun, onuProvSerial, onuProvVLAN, onuProvBandwidthDn, onuProvBandwidthUp,
		onuProvPONPort, onuProvONUID, onuProvLineProfile, onuProvSrvProfile)

	subscriber, tier := buildProvisionModels()

	if onuProvDryRun {
		return outputProvisionDryRun(subscriber, tier)
	}

	conn, err := connectToOLT(120)
	if err != nil {
		return err
	}
	defer conn.close()

	result, err := executeProvision(conn.ctx, conn.driver, subscriber, tier)
	if err != nil {
		return err
	}

	return outputProvisionResult(result)
}

// buildProvisionModels creates subscriber and tier models for provisioning
func buildProvisionModels() (*model.Subscriber, *model.ServiceTier) {
	subscriber := &model.Subscriber{
		Name: onuProvSerial,
		Annotations: map[string]string{
			"nano.io/pon-port": onuProvPONPort,
		},
		Spec: model.SubscriberSpec{
			ONUSerial:   onuProvSerial,
			VLAN:        onuProvVLAN,
			Tier:        "cli-provision",
			Description: onuProvDescription,
		},
	}
	if onuProvONUID != 0 {
		subscriber.Annotations["nano.io/onu-id"] = fmt.Sprintf("%d", onuProvONUID)
	}
	if onuProvLineProfile != "" {
		subscriber.Annotations["nano.io/line-profile"] = onuProvLineProfile
	}
	if onuProvSrvProfile != "" {
		subscriber.Annotations["nano.io/service-profile"] = onuProvSrvProfile
	}

	tier := &model.ServiceTier{
		Name: "cli-provision",
		Spec: model.ServiceTierSpec{
			BandwidthDown: onuProvBandwidthDn,
			BandwidthUp:   onuProvBandwidthUp,
			QoSClass:      "standard",
		},
	}
	return subscriber, tier
}

// outputProvisionDryRun outputs dry run results
func outputProvisionDryRun(subscriber *model.Subscriber, tier *model.ServiceTier) error {
	if outputJSON {
		output := struct {
			Action     string             `json:"action"`
			Subscriber *model.Subscriber  `json:"subscriber"`
			Tier       *model.ServiceTier `json:"tier"`
		}{Action: "provision", Subscriber: subscriber, Tier: tier}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	printDryRunOutput(onuProvSerial, onuProvVLAN, onuProvBandwidthDn, onuProvBandwidthUp, onuProvPONPort, onuProvONUID)
	return nil
}

// executeProvision performs the ONU provisioning
func executeProvision(ctx context.Context, driver types.Driver, subscriber *model.Subscriber, tier *model.ServiceTier) (*types.SubscriberResult, error) {
	if !outputJSON {
		fmt.Printf("Provisioning ONU... ")
	}
	result, err := driver.CreateSubscriber(ctx, subscriber, tier)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return nil, fmt.Errorf("provisioning failed: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}
	return result, nil
}

// outputProvisionResult outputs provisioning results
func outputProvisionResult(result *types.SubscriberResult) error {
	if outputJSON {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	printProvisionSuccess(result.SubscriberID, result.SessionID, result.AssignedIP)
	return nil
}

func runONUDelete(cmd *cobra.Command, args []string) error {
	if err := validateONUIdentifier(onuDelSerial, onuDelPONPort, onuDelONUID); err != nil {
		return err
	}
	if !onuDelForce {
		return fmt.Errorf("this is a destructive operation; use --force to confirm")
	}

	printDeleteHeader(onuDelSerial, onuDelPONPort, onuDelONUID)

	conn, err := connectToOLT(120)
	if err != nil {
		return err
	}
	defer conn.close()

	driverV2, err := conn.getDriverV2()
	if err != nil {
		return err
	}

	// Resolve ONU and show details before delete (NAN-159)
	ponPort, onuID, err := resolveONU(conn.ctx, driverV2, onuDelSerial, onuDelPONPort, onuDelONUID)
	if err != nil {
		return err
	}

	// Show ONU details before deletion
	onu, err := lookupONUByPortID(conn.ctx, driverV2, ponPort, onuID)
	if err != nil {
		return err
	}
	if !outputJSON {
		printONUSummary(onu)
	}

	if err := executeDelete(conn.ctx, conn.driver, onuDelSerial, ponPort, onuID); err != nil {
		return err
	}

	return outputDeleteResult(onuDelSerial, ponPort, onuID)
}

// outputDeleteResult outputs deletion results
func outputDeleteResult(serial, ponPort string, onuID int) error {
	if outputJSON {
		output := struct {
			Status  string `json:"status"`
			Serial  string `json:"serial,omitempty"`
			PONPort string `json:"pon_port"`
			ONUID   int    `json:"onu_id"`
		}{Status: "deleted", Serial: serial, PONPort: ponPort, ONUID: onuID}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	printDeleteSuccess(ponPort, onuID)
	return nil
}

func runONUReboot(cmd *cobra.Command, args []string) error {
	if err := validateONUIdentifier(onuRebootSerial, onuRebootPONPort, onuRebootONUID); err != nil {
		return err
	}

	printRebootHeader(onuRebootSerial, onuRebootPONPort, onuRebootONUID)

	conn, err := connectToOLT(60)
	if err != nil {
		return err
	}
	defer conn.close()

	driverV2, err := conn.getDriverV2()
	if err != nil {
		return err
	}

	ponPort, onuID, err := resolveONU(conn.ctx, driverV2, onuRebootSerial, onuRebootPONPort, onuRebootONUID)
	if err != nil {
		return err
	}

	if err := executeReboot(conn.ctx, driverV2, ponPort, onuID); err != nil {
		return err
	}

	return outputRebootResult(onuRebootSerial, ponPort, onuID)
}

// outputRebootResult outputs reboot results
func outputRebootResult(serial, ponPort string, onuID int) error {
	if outputJSON {
		output := struct {
			Status  string `json:"status"`
			Serial  string `json:"serial,omitempty"`
			PONPort string `json:"pon_port"`
			ONUID   int    `json:"onu_id"`
		}{Status: "reboot_initiated", Serial: serial, PONPort: ponPort, ONUID: onuID}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	printRebootSuccess(ponPort, onuID)
	return nil
}

func runPortList(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("Port List\n")
		fmt.Printf("=========\n\n")
		fmt.Printf("OLT: %s (%s)\n\n", oltAddress, oltVendor)
	}

	conn, err := connectToOLT(60)
	if err != nil {
		return err
	}
	defer conn.close()

	driverV2, err := conn.getDriverV2()
	if err != nil {
		return err
	}

	if !outputJSON {
		fmt.Printf("Getting port list... ")
	}
	ports, err := driverV2.ListPorts(conn.ctx)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to get port list: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK (%d found)\n\n", len(ports))
	}

	if outputJSON {
		data, _ := json.MarshalIndent(ports, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(ports) == 0 {
		fmt.Println("No PON ports found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Port\tAdmin\tOper\tONUs\tTx Power\tDescription")
	fmt.Fprintln(w, "----\t-----\t----\t----\t--------\t-----------")
	for _, port := range ports {
		tx := "-"
		if port.TxPowerDBm != 0 {
			tx = fmt.Sprintf("%.1f dBm", port.TxPowerDBm)
		}
		desc := port.Description
		if desc == "" {
			desc = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d/%d\t%s\t%s\n",
			port.Port, port.AdminState, port.OperState,
			port.ONUCount, port.MaxONUs, tx, desc)
	}
	w.Flush()

	return nil
}

func runPortEnable(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("Port Enable\n")
		fmt.Printf("===========\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		fmt.Printf("Port: %s\n\n", portPONPort)
	}

	conn, err := connectToOLT(60)
	if err != nil {
		return err
	}
	defer conn.close()

	driverV2, err := conn.getDriverV2()
	if err != nil {
		return err
	}

	if !outputJSON {
		fmt.Printf("Enabling port %s... ", portPONPort)
	}
	if err := driverV2.SetPortState(conn.ctx, portPONPort, true); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to enable port: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
		fmt.Printf("Port %s enabled successfully\n", portPONPort)
	}

	if outputJSON {
		output := struct {
			Status string `json:"status"`
			Port   string `json:"port"`
		}{Status: "enabled", Port: portPONPort}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
	}

	return nil
}

func runPortDisable(cmd *cobra.Command, args []string) error {
	if !portForce {
		return fmt.Errorf("this will disconnect all ONUs on port %s; use --force to confirm", portPONPort)
	}

	if !outputJSON {
		fmt.Printf("Port Disable\n")
		fmt.Printf("============\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		fmt.Printf("Port: %s\n\n", portPONPort)
	}

	conn, err := connectToOLT(60)
	if err != nil {
		return err
	}
	defer conn.close()

	driverV2, err := conn.getDriverV2()
	if err != nil {
		return err
	}

	// Check for connected ONUs before disabling
	ports, err := driverV2.ListPorts(conn.ctx)
	if err == nil {
		for _, p := range ports {
			if p.Port == portPONPort && p.ONUCount > 0 {
				if !outputJSON {
					fmt.Printf("WARNING: Port %s has %d connected ONU(s)\n", portPONPort, p.ONUCount)
				}
				break
			}
		}
	}

	if !outputJSON {
		fmt.Printf("Disabling port %s... ", portPONPort)
	}
	if err := driverV2.SetPortState(conn.ctx, portPONPort, false); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to disable port: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
		fmt.Printf("Port %s disabled successfully\n", portPONPort)
	}

	if outputJSON {
		output := struct {
			Status string `json:"status"`
			Port   string `json:"port"`
		}{Status: "disabled", Port: portPONPort}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
	}

	return nil
}
