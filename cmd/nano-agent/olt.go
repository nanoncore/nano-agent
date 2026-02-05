package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
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
	oltVendor      string
	oltAddress     string
	oltPort        int
	oltProtocol    string
	oltUsername    string
	oltPassword    string
	oltCommunity   string
	oltSNMPVersion string
	oltTLS         bool
	oltTLSSkipVe   bool
	outputJSON     bool
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

var oltAlarmsCmd = &cobra.Command{
	Use:   "olt-alarms",
	Short: "Show active OLT alarms",
	Long: `Display active OLT alarms.

Examples:
  nano-agent olt-alarms --vendor vsol --address 192.168.1.1 --protocol cli --username admin --password admin
  nano-agent olt-alarms --vendor vsol --address 192.168.1.1 --protocol cli --username admin --password admin --json`,
	RunE: runOLTAlarms,
}

var oltHealthCheckCmd = &cobra.Command{
	Use:   "olt-health-check",
	Short: "Perform basic OLT health check",
	Long: `Perform basic OLT health check.

Checks OLT connectivity and evaluates PON port status.

Examples:
  nano-agent olt-health-check --vendor vsol --address 192.168.1.1 --protocol snmp --community public
  nano-agent olt-health-check --vendor vsol --address 192.168.1.1 --protocol snmp --community public --json`,
	RunE: runOLTHealthCheck,
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
	onuProvForce       bool
)

// ONU delete flags
var (
	onuDelSerial  string
	onuDelPONPort string
	onuDelONUID   int
	onuDelForce   bool
)

// ONU suspend flags
var (
	onuSuspendSerial  string
	onuSuspendPONPort string
	onuSuspendONUID   int
)

// ONU resume flags
var (
	onuResumeSerial  string
	onuResumePONPort string
	onuResumeONUID   int
)

// ONU bulk provision flags
var (
	onuBulkCSV string
)

// ONU reboot flags
var (
	onuRebootSerial  string
	onuRebootPONPort string
	onuRebootONUID   int
)

// Profile ONU create flags
var (
	profileONUPortEth               int
	profileONUPortPots              int
	profileONUPortIPHost            int
	profileONUPortIPv6Host          int
	profileONUPortVeip              int
	profileONUTcontNum              int
	profileONUGemportNum            int
	profileONUSwitchNum             int
	profileONUServiceAbility        string
	profileONUOmciSendMode          string
	profileONUExOMCI                bool
	profileONUWifiMngViaNonOMCI     bool
	profileONUDefaultMulticastRange string
	profileONUDescription           string
)

// ONU update flags
var (
	onuUpdatePONPort        string
	onuUpdateONUID          int
	onuUpdateVLAN           int
	onuUpdateTrafficProfile int
	onuUpdateDescription    string
	onuUpdateLineProfile    string
	onuUpdateServiceProfile string
	onuUpdateForce          bool
)

// Port management flags
var (
	portPONPort string
	portForce   bool
)

// Service port list flags
var (
	servicePortListPONPort string
	servicePortListONUID   int
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

var onuSuspendCmd = &cobra.Command{
	Use:   "onu-suspend",
	Short: "Suspend an ONU (disable traffic)",
	Long: `Suspend a provisioned ONU while keeping its configuration.

This command disables traffic for the specified ONU without deleting it.
The ONU can be identified by serial number OR by PON port and ONU ID.

Examples:
  # Suspend by serial number
  nano-agent onu-suspend --serial HWTC12345678 \
    --vendor huawei --address 192.168.1.1

  # Suspend by PON port and ONU ID
  nano-agent onu-suspend --pon-port 0/0/1 --onu-id 101 \
    --vendor huawei --address 192.168.1.1`,
	RunE: runONUSuspend,
}

var onuResumeCmd = &cobra.Command{
	Use:   "onu-resume",
	Short: "Resume an ONU (enable traffic)",
	Long: `Resume a suspended ONU and restore traffic.

This command re-enables traffic for the specified ONU. The ONU can be
identified by serial number OR by PON port and ONU ID.

Examples:
  # Resume by serial number
  nano-agent onu-resume --serial HWTC12345678 \
    --vendor huawei --address 192.168.1.1

  # Resume by PON port and ONU ID
  nano-agent onu-resume --pon-port 0/0/1 --onu-id 101 \
    --vendor huawei --address 192.168.1.1`,
	RunE: runONUResume,
}

var onuBulkProvisionCmd = &cobra.Command{
	Use:   "onu-bulk-provision",
	Short: "Provision multiple ONUs from a CSV file",
	Long: `Provision multiple ONUs in a single operation using a CSV file.

CSV columns (header row required):
  serial,pon_port,onu_id,vlan,line_profile,service_profile,bandwidth_up,bandwidth_down,priority

Examples:
  nano-agent onu-bulk-provision --csv bulk.csv \
    --vendor vsol --address 10.0.0.254 --protocol cli --username admin --password admin`,
	RunE: runONUBulkProvision,
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

var onuUpdateCmd = &cobra.Command{
	Use:   "onu-update",
	Short: "Update configuration of an existing ONU",
	Long: `Update configuration settings for a provisioned ONU.

This command allows you to modify VLAN, traffic profile, or description
for an already provisioned ONU without reprovisioning.

Examples:
  # Update VLAN
  nano-agent onu-update --pon-port 0/1 --onu-id 1 --vlan 200 \
    --vendor vsol --address 10.0.0.254 --username admin --password admin

  # Update traffic profile
  nano-agent onu-update --pon-port 0/1 --onu-id 1 --traffic-profile 5 \
    --vendor vsol --address 10.0.0.254 --username admin --password admin

  # Update both VLAN and traffic profile
  nano-agent onu-update --pon-port 0/1 --onu-id 1 \
    --vlan 200 --traffic-profile 5 \
    --vendor vsol --address 10.0.0.254 --username admin --password admin --json`,
	RunE: runONUUpdate,
}

var profileONUCmd = &cobra.Command{
	Use:   "profile-onu",
	Short: "Manage ONU hardware profiles",
	Long: `Manage ONU hardware profiles on the OLT.

Supports listing, retrieving, creating, and deleting ONU hardware profiles.`,
}

var profileONUListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ONU hardware profiles",
	Long:  `List ONU hardware profiles available on the OLT.`,
	RunE:  runProfileONUList,
}

var profileONUGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get an ONU hardware profile by name",
	Long:  `Retrieve details for a specific ONU hardware profile by name.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileONUGet,
}

var profileONUCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create an ONU hardware profile",
	Long: `Create an ONU hardware profile with the provided fields.

Example:
  nano-agent profile-onu create AN5506-04-F1 \
    --port-eth 4 --port-veip 1 --tcont-num 8 --gemport-num 32 \
    --service-ability n:1 --description "AN5506-04-F 4-port GPON ONU"`,
	Args: cobra.ExactArgs(1),
	RunE: runProfileONUCreate,
}

var profileONUDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an ONU hardware profile",
	Long:  `Delete an ONU hardware profile by name.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileONUDelete,
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

var portPowerCmd = &cobra.Command{
	Use:   "port-power",
	Short: "Get optical power readings for a PON port",
	Long: `Retrieve optical power readings from a PON port's SFP/GBIC module.

Returns transmit power, receive power (if available), and module temperature.

Examples:
  # Get power readings for a PON port
  nano-agent port-power --pon-port 0/0/1 \
    --vendor huawei --address 192.168.1.1 --port 161 --protocol snmp --community public

  # Output as JSON
  nano-agent port-power --pon-port 0/0/1 \
    --vendor huawei --address 192.168.1.1 --port 161 --protocol snmp --community public --json`,
	RunE: runPortPower,
}

var servicePortListCmd = &cobra.Command{
	Use:   "service-port-list",
	Short: "List service ports (VLAN to ONU mappings)",
	Long: `List service ports configured on the OLT.

Service ports map VLANs to specific ONUs and GEM ports.

Examples:
  # List all service ports
  nano-agent service-port-list \
    --vendor vsol --address 192.168.1.1 --port 22 --protocol cli --username admin --password admin

  # Output as JSON
  nano-agent service-port-list \
    --vendor vsol --address 192.168.1.1 --port 22 --protocol cli --username admin --password admin --json`,
	RunE: runServicePortList,
}

func init() {
	// Common OLT connection flags for all OLT commands
	oltCommands := []*cobra.Command{
		discoverCmd, diagnoseCmd, oltStatusCmd, oltAlarmsCmd, oltHealthCheckCmd, onuListCmd,
		onuInfoCmd, onuProvisionCmd, onuDeleteCmd, onuSuspendCmd, onuResumeCmd, onuBulkProvisionCmd, onuRebootCmd, onuUpdateCmd,
		profileONUCmd, profileONUListCmd, profileONUGetCmd, profileONUCreateCmd, profileONUDeleteCmd,
		portListCmd, portEnableCmd, portDisableCmd, portPowerCmd, servicePortListCmd,
	}
	for _, cmd := range oltCommands {
		cmd.Flags().StringVar(&oltVendor, "vendor", "", "OLT vendor (vsol, cdata, nokia, huawei, zte, etc.) [required]")
		cmd.Flags().StringVar(&oltAddress, "address", "", "OLT management IP address [required]")
		cmd.Flags().IntVar(&oltPort, "port", 0, "OLT management port (default based on protocol)")
		cmd.Flags().StringVar(&oltProtocol, "protocol", "", "Management protocol (cli, netconf, gnmi, snmp)")
		cmd.Flags().StringVar(&oltUsername, "username", "", "OLT username (required for CLI/NETCONF)")
		cmd.Flags().StringVar(&oltPassword, "password", "", "OLT password (required for CLI/NETCONF)")
		cmd.Flags().StringVar(&oltCommunity, "community", "", "SNMP community string (required for SNMP)")
		cmd.Flags().StringVar(&oltSNMPVersion, "snmp-version", "2c", "SNMP version (1, 2c, 3)")
		cmd.Flags().BoolVar(&oltTLS, "tls", false, "Enable TLS")
		cmd.Flags().BoolVar(&oltTLSSkipVe, "tls-skip-verify", false, "Skip TLS verification (insecure)")
		cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")

		cmd.MarkFlagRequired("vendor")
		cmd.MarkFlagRequired("address")
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
	onuProvisionCmd.Flags().BoolVar(&onuProvForce, "force", false, "Force direct VLAN config (unbind profile if mismatch)")
	onuProvisionCmd.MarkFlagRequired("serial")
	onuProvisionCmd.MarkFlagRequired("vlan")

	// ONU delete flags
	onuDeleteCmd.Flags().StringVar(&onuDelSerial, "serial", "", "ONU serial number")
	onuDeleteCmd.Flags().StringVar(&onuDelPONPort, "pon-port", "", "PON port (required if not using serial)")
	onuDeleteCmd.Flags().IntVar(&onuDelONUID, "onu-id", 0, "ONU ID (required if not using serial)")
	onuDeleteCmd.Flags().BoolVar(&onuDelForce, "force", false, "Confirm the deletion")

	// ONU suspend flags
	onuSuspendCmd.Flags().StringVar(&onuSuspendSerial, "serial", "", "ONU serial number")
	onuSuspendCmd.Flags().StringVar(&onuSuspendPONPort, "pon-port", "", "PON port (required if not using serial)")
	onuSuspendCmd.Flags().IntVar(&onuSuspendONUID, "onu-id", 0, "ONU ID (required if not using serial)")

	// ONU resume flags
	onuResumeCmd.Flags().StringVar(&onuResumeSerial, "serial", "", "ONU serial number")
	onuResumeCmd.Flags().StringVar(&onuResumePONPort, "pon-port", "", "PON port (required if not using serial)")
	onuResumeCmd.Flags().IntVar(&onuResumeONUID, "onu-id", 0, "ONU ID (required if not using serial)")

	// ONU bulk provision flags
	onuBulkProvisionCmd.Flags().StringVar(&onuBulkCSV, "csv", "", "CSV file with bulk provision operations [required]")
	onuBulkProvisionCmd.MarkFlagRequired("csv")

	// ONU reboot flags
	onuRebootCmd.Flags().StringVar(&onuRebootSerial, "serial", "", "ONU serial number")
	onuRebootCmd.Flags().StringVar(&onuRebootPONPort, "pon-port", "", "PON port (required if not using serial)")
	onuRebootCmd.Flags().IntVar(&onuRebootONUID, "onu-id", 0, "ONU ID (required if not using serial)")

	// ONU update flags
	onuUpdateCmd.Flags().StringVar(&onuUpdatePONPort, "pon-port", "", "PON port [required]")
	onuUpdateCmd.Flags().IntVar(&onuUpdateONUID, "onu-id", 0, "ONU ID [required]")
	onuUpdateCmd.Flags().IntVar(&onuUpdateVLAN, "vlan", 0, "New VLAN ID (optional)")
	onuUpdateCmd.Flags().IntVar(&onuUpdateTrafficProfile, "traffic-profile", 0, "Traffic profile ID (optional)")
	onuUpdateCmd.Flags().StringVar(&onuUpdateDescription, "description", "", "Description (optional)")
	onuUpdateCmd.Flags().StringVar(&onuUpdateLineProfile, "line-profile", "", "Line profile name (optional)")
	onuUpdateCmd.Flags().StringVar(&onuUpdateServiceProfile, "service-profile", "", "Service profile name (optional)")
	onuUpdateCmd.Flags().BoolVar(&onuUpdateForce, "force", false, "Force unbind profile when switching to direct VLAN config")
	onuUpdateCmd.MarkFlagRequired("pon-port")
	onuUpdateCmd.MarkFlagRequired("onu-id")

	// Profile ONU create flags
	profileONUCreateCmd.Flags().IntVar(&profileONUPortEth, "port-eth", 0, "Number of Ethernet ports (1-255)")
	profileONUCreateCmd.Flags().IntVar(&profileONUPortPots, "port-pots", 0, "Number of POTS ports (1-255)")
	profileONUCreateCmd.Flags().IntVar(&profileONUPortIPHost, "port-iphost", 0, "Number of IP host interfaces (1-255)")
	profileONUCreateCmd.Flags().IntVar(&profileONUPortIPv6Host, "port-ipv6host", 0, "Number of IPv6 host interfaces (1-255)")
	profileONUCreateCmd.Flags().IntVar(&profileONUPortVeip, "port-veip", 0, "Number of VEIP ports (1-255)")
	profileONUCreateCmd.Flags().IntVar(&profileONUTcontNum, "tcont-num", 0, "Maximum TCONT count (1-255)")
	profileONUCreateCmd.Flags().IntVar(&profileONUGemportNum, "gemport-num", 0, "Maximum GEMPORT count (1-255)")
	profileONUCreateCmd.Flags().IntVar(&profileONUSwitchNum, "switch-num", 0, "ONU switch number")
	profileONUCreateCmd.Flags().StringVar(&profileONUServiceAbility, "service-ability", "", "Service ability (e.g., n:1)")
	profileONUCreateCmd.Flags().StringVar(&profileONUOmciSendMode, "omci-send-mode", "", "OMCI send mode (vendor-specific)")
	profileONUCreateCmd.Flags().BoolVar(&profileONUExOMCI, "ex-omci", false, "Enable extended OMCI")
	profileONUCreateCmd.Flags().BoolVar(&profileONUWifiMngViaNonOMCI, "wifi-mng-via-non-omci", false, "Enable WiFi management via non-OMCI")
	profileONUCreateCmd.Flags().StringVar(&profileONUDefaultMulticastRange, "default-multicast-range", "", "Default multicast range")
	profileONUCreateCmd.Flags().StringVar(&profileONUDescription, "description", "", "Profile description (max 64 chars)")

	// Port enable flags
	portEnableCmd.Flags().StringVar(&portPONPort, "pon-port", "", "PON port (e.g., 0/0/1) [required]")
	portEnableCmd.MarkFlagRequired("pon-port")

	// Port disable flags
	portDisableCmd.Flags().StringVar(&portPONPort, "pon-port", "", "PON port (e.g., 0/0/1) [required]")
	portDisableCmd.Flags().BoolVar(&portForce, "force", false, "Confirm the disable operation")
	portDisableCmd.MarkFlagRequired("pon-port")

	// Port power flags
	portPowerCmd.Flags().StringVar(&portPONPort, "pon-port", "", "PON port (e.g., 0/0/1) [required]")
	portPowerCmd.MarkFlagRequired("pon-port")

	// Service port list flags
	servicePortListCmd.Flags().StringVar(&servicePortListPONPort, "pon-port", "", "Filter by PON port (optional)")
	servicePortListCmd.Flags().IntVar(&servicePortListONUID, "onu-id", 0, "Filter by ONU ID (optional)")

	// Add commands to root
	rootCmd.AddCommand(discoverCmd)
	rootCmd.AddCommand(diagnoseCmd)
	rootCmd.AddCommand(oltStatusCmd)
	rootCmd.AddCommand(oltAlarmsCmd)
	rootCmd.AddCommand(oltHealthCheckCmd)
	rootCmd.AddCommand(onuListCmd)
	rootCmd.AddCommand(onuInfoCmd)
	rootCmd.AddCommand(onuProvisionCmd)
	rootCmd.AddCommand(onuDeleteCmd)
	rootCmd.AddCommand(onuSuspendCmd)
	rootCmd.AddCommand(onuResumeCmd)
	rootCmd.AddCommand(onuBulkProvisionCmd)
	rootCmd.AddCommand(onuRebootCmd)
	rootCmd.AddCommand(onuUpdateCmd)
	rootCmd.AddCommand(profileONUCmd)
	profileONUCmd.AddCommand(profileONUListCmd)
	profileONUCmd.AddCommand(profileONUGetCmd)
	profileONUCmd.AddCommand(profileONUCreateCmd)
	profileONUCmd.AddCommand(profileONUDeleteCmd)
	rootCmd.AddCommand(portListCmd)
	rootCmd.AddCommand(portEnableCmd)
	rootCmd.AddCommand(portDisableCmd)
	rootCmd.AddCommand(portPowerCmd)
	rootCmd.AddCommand(servicePortListCmd)
}

// createOLTDriver creates a driver connection to the OLT
func createOLTDriver() (types.Driver, error) {
	vendor := types.Vendor(strings.ToLower(oltVendor))
	protocol := types.Protocol(strings.ToLower(oltProtocol))

	// Validate credentials based on protocol
	if protocol == types.ProtocolSNMP {
		if oltCommunity == "" {
			return nil, fmt.Errorf("--community is required for SNMP protocol")
		}
	} else {
		// CLI, NETCONF, GNMI require username/password
		if oltUsername == "" || oltPassword == "" {
			return nil, fmt.Errorf("--username and --password are required for %s protocol", protocol)
		}
	}

	config := &types.EquipmentConfig{
		Name:          "cli-session",
		Vendor:        vendor,
		Address:       oltAddress,
		Port:          oltPort,
		Protocol:      protocol,
		Username:      oltUsername,
		Password:      oltPassword,
		SNMPCommunity: oltCommunity,
		SNMPVersion:   oltSNMPVersion,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}

	// Add SNMP community to metadata for drivers that read it from there
	if protocol == types.ProtocolSNMP {
		config.Metadata["snmp_community"] = oltCommunity
		config.Metadata["snmp_version"] = oltSNMPVersion
	}
	if protocol == types.ProtocolCLI {
		// Prefer CLI execution when explicitly using CLI protocol (avoid slow SNMP walks).
		config.Metadata["prefer_cli"] = "true"
		// Simulator CLI does not require pager disabling and can hang on terminal length.
		if oltAddress == "127.0.0.1" || strings.EqualFold(oltAddress, "localhost") {
			config.Metadata["disable_pager"] = "false"
		}
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
		Protocol:      types.Protocol(strings.ToLower(oltProtocol)),
		Username:      oltUsername,
		Password:      oltPassword,
		SNMPCommunity: oltCommunity,
		SNMPVersion:   oltSNMPVersion,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}
	// Add SNMP metadata for drivers that read it from there
	if oltProtocol == "snmp" {
		config.Metadata["snmp_community"] = oltCommunity
		config.Metadata["snmp_version"] = oltSNMPVersion
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
		Protocol:      types.Protocol(strings.ToLower(oltProtocol)),
		Username:      oltUsername,
		Password:      oltPassword,
		SNMPCommunity: oltCommunity,
		SNMPVersion:   oltSNMPVersion,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}
	// Add SNMP metadata for drivers that read it from there
	if oltProtocol == "snmp" {
		config.Metadata["snmp_community"] = oltCommunity
		config.Metadata["snmp_version"] = oltSNMPVersion
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
		Protocol:      types.Protocol(strings.ToLower(oltProtocol)),
		Username:      oltUsername,
		Password:      oltPassword,
		SNMPCommunity: oltCommunity,
		SNMPVersion:   oltSNMPVersion,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}
	// Add SNMP metadata for drivers that read it from there
	if oltProtocol == "snmp" {
		config.Metadata["snmp_community"] = oltCommunity
		config.Metadata["snmp_version"] = oltSNMPVersion
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

func runOLTAlarms(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("OLT Alarms\n")
		fmt.Printf("==========\n\n")
	}

	driver, err := createOLTDriver()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := &types.EquipmentConfig{
		Name:          "cli-session",
		Vendor:        types.Vendor(strings.ToLower(oltVendor)),
		Address:       oltAddress,
		Port:          oltPort,
		Protocol:      types.Protocol(strings.ToLower(oltProtocol)),
		Username:      oltUsername,
		Password:      oltPassword,
		SNMPCommunity: oltCommunity,
		SNMPVersion:   oltSNMPVersion,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}
	if oltProtocol == "snmp" {
		config.Metadata["snmp_community"] = oltCommunity
		config.Metadata["snmp_version"] = oltSNMPVersion
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

	driverV2, ok := driver.(types.DriverV2)
	if !ok {
		return fmt.Errorf("driver for vendor %s does not support OLT alarms", oltVendor)
	}

	if !outputJSON {
		fmt.Printf("Getting OLT alarms... ")
	}
	alarms, err := driverV2.GetAlarms(ctx)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to get alarms: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}

	if outputJSON {
		output, _ := json.MarshalIndent(alarms, "", "  ")
		fmt.Println(string(output))
		return nil
	}

	if len(alarms) == 0 {
		fmt.Printf("No active alarms.\n")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSeverity\tType\tSource\tSource ID\tMessage\tRaised At")
	fmt.Fprintln(w, "--\t--------\t----\t------\t---------\t-------\t---------")
	for _, alarm := range alarms {
		raisedAt := ""
		if !alarm.RaisedAt.IsZero() {
			raisedAt = alarm.RaisedAt.Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			alarm.ID,
			alarm.Severity,
			alarm.Type,
			alarm.Source,
			alarm.SourceID,
			alarm.Message,
			raisedAt,
		)
	}
	w.Flush()

	return nil
}

func runOLTHealthCheck(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("OLT Health Check\n")
		fmt.Printf("================\n\n")
	}

	driver, err := createOLTDriver()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := &types.EquipmentConfig{
		Name:          "cli-session",
		Vendor:        types.Vendor(strings.ToLower(oltVendor)),
		Address:       oltAddress,
		Port:          oltPort,
		Protocol:      types.Protocol(strings.ToLower(oltProtocol)),
		Username:      oltUsername,
		Password:      oltPassword,
		SNMPCommunity: oltCommunity,
		SNMPVersion:   oltSNMPVersion,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}
	if oltProtocol == "snmp" {
		config.Metadata["snmp_community"] = oltCommunity
		config.Metadata["snmp_version"] = oltSNMPVersion
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

	driverV2, ok := driver.(types.DriverV2)
	if !ok {
		return fmt.Errorf("driver for vendor %s does not support OLT health check", oltVendor)
	}

	// Thresholds (best-practice defaults)
	const (
		tempWarnC  = 70.0
		tempCritC  = 80.0
		cpuWarnPct = 75.0
		cpuCritPct = 90.0
		memWarnPct = 80.0
		memCritPct = 90.0
	)

	if !outputJSON {
		fmt.Printf("Checking PON ports... ")
	}
	ports, err := driverV2.ListPorts(ctx)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to list PON ports: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}

	var healthIssues []string
	var warnings []string

	for _, port := range ports {
		status := strings.ToLower(port.OperState)
		adminStatus := strings.ToLower(port.AdminState)
		if adminStatus == "enable" && (status == "down" || status == "offline") {
			healthIssues = append(healthIssues, fmt.Sprintf("%s is down while enabled", port.Port))
		}
	}

	// Optional metrics from OLT status (may be missing for some vendors)
	status, statusErr := driverV2.GetOLTStatus(ctx)
	if statusErr == nil && status != nil {
		if status.Temperature > 0 {
			switch {
			case status.Temperature >= tempCritC:
				healthIssues = append(healthIssues, fmt.Sprintf("temperature %.1fC exceeds critical threshold %.1fC", status.Temperature, tempCritC))
			case status.Temperature >= tempWarnC:
				warnings = append(warnings, fmt.Sprintf("temperature %.1fC exceeds warning threshold %.1fC", status.Temperature, tempWarnC))
			}
		}
		if status.CPUPercent > 0 {
			switch {
			case status.CPUPercent >= cpuCritPct:
				healthIssues = append(healthIssues, fmt.Sprintf("cpu %.1f%% exceeds critical threshold %.1f%%", status.CPUPercent, cpuCritPct))
			case status.CPUPercent >= cpuWarnPct:
				warnings = append(warnings, fmt.Sprintf("cpu %.1f%% exceeds warning threshold %.1f%%", status.CPUPercent, cpuWarnPct))
			}
		}
		if status.MemoryPercent > 0 {
			switch {
			case status.MemoryPercent >= memCritPct:
				healthIssues = append(healthIssues, fmt.Sprintf("memory %.1f%% exceeds critical threshold %.1f%%", status.MemoryPercent, memCritPct))
			case status.MemoryPercent >= memWarnPct:
				warnings = append(warnings, fmt.Sprintf("memory %.1f%% exceeds warning threshold %.1f%%", status.MemoryPercent, memWarnPct))
			}
		}
	}

	// Optional alarms (if vendor supports CLI alarm retrieval)
	if alarms, alarmsErr := driverV2.GetAlarms(ctx); alarmsErr == nil {
		for _, alarm := range alarms {
			sev := strings.ToLower(alarm.Severity)
			msg := alarm.Message
			if msg == "" {
				msg = alarm.Type
			}
			switch sev {
			case "critical", "major":
				healthIssues = append(healthIssues, fmt.Sprintf("alarm %s: %s", sev, msg))
			case "minor", "warning":
				warnings = append(warnings, fmt.Sprintf("alarm %s: %s", sev, msg))
			}
		}
	}

	healthy := len(healthIssues) == 0
	message := "OLT is healthy"
	if !healthy {
		message = strings.Join(healthIssues, "; ")
	}

	if outputJSON {
		output, _ := json.MarshalIndent(map[string]interface{}{
			"healthy":    healthy,
			"message":    message,
			"issues":     healthIssues,
			"warnings":   warnings,
			"port_count": len(ports),
			"thresholds": map[string]float64{
				"temp_warn_c":  tempWarnC,
				"temp_crit_c":  tempCritC,
				"cpu_warn_pct": cpuWarnPct,
				"cpu_crit_pct": cpuCritPct,
				"mem_warn_pct": memWarnPct,
				"mem_crit_pct": memCritPct,
			},
		}, "", "  ")
		fmt.Println(string(output))
		return nil
	}

	if healthy {
		fmt.Printf("Health: OK\n")
	} else {
		fmt.Printf("Health: Issues detected\n")
		for _, issue := range healthIssues {
			fmt.Printf("  - %s\n", issue)
		}
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings:\n")
		for _, warn := range warnings {
			fmt.Printf("  - %s\n", warn)
		}
	}
	fmt.Printf("PON ports checked: %d\n", len(ports))

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
		Protocol:      types.Protocol(strings.ToLower(oltProtocol)),
		Username:      oltUsername,
		Password:      oltPassword,
		SNMPCommunity: oltCommunity,
		SNMPVersion:   oltSNMPVersion,
		TLSEnabled:    oltTLS,
		TLSSkipVerify: oltTLSSkipVe,
		Timeout:       60 * time.Second,
		Metadata:      make(map[string]string),
	}
	// Add SNMP metadata for drivers that read it from there
	if oltProtocol == "snmp" {
		config.Metadata["snmp_community"] = oltCommunity
		config.Metadata["snmp_version"] = oltSNMPVersion
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

	// Validate profile/VLAN consistency (NAN-247)
	if onuProvVLAN > 0 && onuProvLineProfile != "" {
		decision, err := validateProfileVLANConsistency(onuProvLineProfile, onuProvVLAN, onuProvForce)
		if err != nil {
			return err
		}
		// If direct-vlan decision, clear profile for direct provisioning
		if decision == "direct-vlan" {
			if !outputJSON {
				fmt.Printf("Force unbinding profile, switching to direct VLAN configuration.\n\n")
			}
			onuProvLineProfile = ""
			onuProvSrvProfile = ""
		}
	}

	// Detect line profile usage for two-step provisioning (NAN-258)
	usesLineProfile := onuProvLineProfile != "" && strings.Contains(onuProvLineProfile, "line")

	if usesLineProfile && !outputJSON {
		fmt.Printf("Using line profile provisioning (two-step)\n\n")
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

	result, err := executeProvision(conn.ctx, conn.driver, subscriber, tier, usesLineProfile)
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

// executeProvision performs the ONU provisioning with verification (NAN-258)
func executeProvision(ctx context.Context, driver types.Driver, subscriber *model.Subscriber, tier *model.ServiceTier, usesLineProfile bool) (*types.SubscriberResult, error) {
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

	// Get DriverV2 interface for verification (NAN-258)
	driverV2, ok := driver.(types.DriverV2)
	if !ok {
		// Driver doesn't support verification, skip
		return result, nil
	}

	// Extract PON port and ONU ID from subscriber annotations
	ponPort := subscriber.Annotations["nano.io/pon-port"]
	onuIDStr := subscriber.Annotations["nano.io/onu-id"]
	if ponPort == "" || onuIDStr == "" {
		// Can't verify without location info, skip
		return result, nil
	}
	onuID, err := strconv.Atoi(onuIDStr)
	if err != nil {
		return result, nil // Skip verification if ONU ID invalid
	}

	// Verify ONU provisioning (NAN-258)
	if err := verifyONUProvision(ctx, driverV2, ponPort, onuID); err != nil {
		return nil, fmt.Errorf("provisioning verification failed: %w", err)
	}

	// If using line profile, verify line profile association (NAN-258)
	if usesLineProfile {
		lineProfile := subscriber.Annotations["nano.io/line-profile"]
		if lineProfile != "" {
			if err := verifyLineProfileAssociation(ctx, driverV2, ponPort, onuID, lineProfile); err != nil {
				return nil, fmt.Errorf("line profile verification failed: %w", err)
			}
		}
	}

	// Verify VLAN if specified (NAN-258)
	if subscriber.Spec.VLAN > 0 {
		if err := verifyVLANUpdate(ctx, driverV2, ponPort, onuID, subscriber.Spec.VLAN); err != nil {
			return nil, fmt.Errorf("VLAN verification failed: %w", err)
		}
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

func runONUSuspend(cmd *cobra.Command, args []string) error {
	if err := validateONUIdentifier(onuSuspendSerial, onuSuspendPONPort, onuSuspendONUID); err != nil {
		return err
	}

	printSuspendHeader(onuSuspendSerial, onuSuspendPONPort, onuSuspendONUID)

	conn, err := connectToOLT(60)
	if err != nil {
		return err
	}
	defer conn.close()

	driverV2, err := conn.getDriverV2()
	if err != nil {
		return err
	}

	ponPort, onuID, err := resolveONU(conn.ctx, driverV2, onuSuspendSerial, onuSuspendPONPort, onuSuspendONUID)
	if err != nil {
		return err
	}

	if err := executeSuspend(conn.ctx, conn.driver, ponPort, onuID); err != nil {
		return err
	}

	return outputSuspendResult(onuSuspendSerial, ponPort, onuID)
}

// outputSuspendResult outputs suspend results
func outputSuspendResult(serial, ponPort string, onuID int) error {
	if outputJSON {
		output := struct {
			Status  string `json:"status"`
			Serial  string `json:"serial,omitempty"`
			PONPort string `json:"pon_port"`
			ONUID   int    `json:"onu_id"`
		}{Status: "suspended", Serial: serial, PONPort: ponPort, ONUID: onuID}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	printSuspendSuccess(ponPort, onuID)
	return nil
}

func runONUResume(cmd *cobra.Command, args []string) error {
	if err := validateONUIdentifier(onuResumeSerial, onuResumePONPort, onuResumeONUID); err != nil {
		return err
	}

	printResumeHeader(onuResumeSerial, onuResumePONPort, onuResumeONUID)

	conn, err := connectToOLT(60)
	if err != nil {
		return err
	}
	defer conn.close()

	driverV2, err := conn.getDriverV2()
	if err != nil {
		return err
	}

	ponPort, onuID, err := resolveONU(conn.ctx, driverV2, onuResumeSerial, onuResumePONPort, onuResumeONUID)
	if err != nil {
		return err
	}

	if err := executeResume(conn.ctx, conn.driver, ponPort, onuID); err != nil {
		return err
	}

	return outputResumeResult(onuResumeSerial, ponPort, onuID)
}

// outputResumeResult outputs resume results
func outputResumeResult(serial, ponPort string, onuID int) error {
	if outputJSON {
		output := struct {
			Status  string `json:"status"`
			Serial  string `json:"serial,omitempty"`
			PONPort string `json:"pon_port"`
			ONUID   int    `json:"onu_id"`
		}{Status: "resumed", Serial: serial, PONPort: ponPort, ONUID: onuID}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	printResumeSuccess(ponPort, onuID)
	return nil
}

func runONUBulkProvision(cmd *cobra.Command, args []string) error {
	if onuBulkCSV == "" {
		return fmt.Errorf("--csv is required")
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

	ops, err := loadBulkProvisionCSV(onuBulkCSV)
	if err != nil {
		return err
	}

	if err := assignBulkONUIDs(conn.ctx, driverV2, ops); err != nil {
		return err
	}

	if !outputJSON {
		fmt.Printf("ONU Bulk Provision\n")
		fmt.Printf("==================\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		fmt.Printf("Operations: %d\n\n", len(ops))
		fmt.Printf("Provisioning... ")
	}

	result, err := driverV2.BulkProvision(conn.ctx, ops)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("bulk provision failed: %w", err)
	}

	if outputJSON {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("OK\n\n")
	fmt.Printf("Summary:\n")
	fmt.Printf("  Succeeded: %d\n", result.Succeeded)
	fmt.Printf("  Failed:    %d\n", result.Failed)
	fmt.Printf("\nResults:\n")
	for _, r := range result.Results {
		status := "OK"
		if !r.Success {
			status = "FAILED"
		}
		fmt.Printf("  %s - %s %s onu %d", status, r.Serial, r.PONPort, r.ONUID)
		if r.Error != "" {
			fmt.Printf(" (%s)", r.Error)
		}
		fmt.Println()
	}

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

func loadBulkProvisionCSV(path string) ([]types.BulkProvisionOp, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open csv: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read csv header: %w", err)
	}

	index := map[string]int{}
	for i, h := range headers {
		key := strings.ToLower(strings.TrimSpace(h))
		index[key] = i
	}

	get := func(row []string, key string) string {
		idx, ok := index[key]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	var ops []types.BulkProvisionOp
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read csv row: %w", err)
		}

		serial := get(row, "serial")
		ponPort := get(row, "pon_port")
		if serial == "" || ponPort == "" {
			return nil, fmt.Errorf("serial and pon_port are required")
		}

		onuID, _ := strconv.Atoi(get(row, "onu_id"))
		vlan, _ := strconv.Atoi(get(row, "vlan"))
		bwUp, _ := strconv.Atoi(get(row, "bandwidth_up"))
		bwDown, _ := strconv.Atoi(get(row, "bandwidth_down"))
		priority, _ := strconv.Atoi(get(row, "priority"))
		lineProfile := get(row, "line_profile")
		serviceProfile := get(row, "service_profile")

		op := types.BulkProvisionOp{
			Serial:  serial,
			PONPort: ponPort,
			ONUID:   onuID,
			Profile: &types.ONUProfile{
				LineProfile:    lineProfile,
				ServiceProfile: serviceProfile,
				BandwidthUp:    bwUp,
				BandwidthDown:  bwDown,
				VLAN:           vlan,
				Priority:       priority,
			},
		}

		ops = append(ops, op)
	}

	if len(ops) == 0 {
		return nil, fmt.Errorf("no operations found in csv")
	}

	return ops, nil
}

func assignBulkONUIDs(ctx context.Context, driver types.DriverV2, ops []types.BulkProvisionOp) error {
	usedByPort := map[string]map[int]struct{}{}

	for _, op := range ops {
		if _, ok := usedByPort[op.PONPort]; !ok {
			usedByPort[op.PONPort] = map[int]struct{}{}
		}
		if op.ONUID > 0 {
			usedByPort[op.PONPort][op.ONUID] = struct{}{}
		}
	}

	for port := range usedByPort {
		onus, err := driver.GetONUList(ctx, &types.ONUFilter{PONPort: port})
		if err != nil {
			return fmt.Errorf("failed to list ONUs on %s: %w", port, err)
		}
		for _, onu := range onus {
			usedByPort[port][onu.ONUID] = struct{}{}
		}
	}

	for i, op := range ops {
		if op.ONUID > 0 {
			continue
		}
		used := usedByPort[op.PONPort]
		assigned := 0
		for id := 1; id <= 128; id++ {
			if _, exists := used[id]; !exists {
				assigned = id
				used[id] = struct{}{}
				break
			}
		}
		if assigned == 0 {
			return fmt.Errorf("no available ONU IDs on port %s", op.PONPort)
		}
		ops[i].ONUID = assigned
	}

	return nil
}

func runONUUpdate(cmd *cobra.Command, args []string) error {
	// Validate at least one update parameter
	if onuUpdateVLAN == 0 && onuUpdateTrafficProfile == 0 && onuUpdateDescription == "" &&
		onuUpdateLineProfile == "" && onuUpdateServiceProfile == "" {
		return fmt.Errorf("at least one update parameter required: --vlan, --traffic-profile, --line-profile, --service-profile, or --description")
	}

	// Validate profile/VLAN consistency (NAN-250)
	if onuUpdateVLAN > 0 && onuUpdateLineProfile != "" {
		decision, err := validateProfileVLANConsistency(onuUpdateLineProfile, onuUpdateVLAN, onuUpdateForce)
		if err != nil {
			return err
		}
		// Store decision for buildUpdateModels
		if decision == "direct-vlan" {
			if !outputJSON {
				fmt.Printf("Force unbinding profile, switching to direct VLAN configuration.\n\n")
			}
			// User wants direct VLAN with --force, clear profile flags
			onuUpdateLineProfile = ""
			onuUpdateServiceProfile = ""
		}
	}

	printUpdateHeader(onuUpdatePONPort, onuUpdateONUID, onuUpdateVLAN, onuUpdateTrafficProfile, onuUpdateDescription, onuUpdateLineProfile, onuUpdateServiceProfile)

	conn, err := connectToOLT(120)
	if err != nil {
		return err
	}
	defer conn.close()

	// Get DriverV2 for ONU lookup
	driverV2, err := conn.getDriverV2()
	if err != nil {
		return err
	}

	// Verify ONU exists and get pre-state
	preONU, err := lookupONUByPortID(conn.ctx, driverV2, onuUpdatePONPort, onuUpdateONUID)
	if err != nil {
		return err
	}

	if !outputJSON {
		printCurrentConfig(preONU)
	}

	// Detect if profile change is required (NAN-259)
	// Optimization: Only re-provision if profile actually changes, not just because line profile is bound
	profileChanged := onuUpdateLineProfile != "" && onuUpdateLineProfile != preONU.LineProfile

	// Check if we should try direct VLAN update first (profile unchanged scenario)
	// CRITICAL: Line profiles MAY BLOCK direct VLAN changes (validated Test 7)
	// But if profile is unchanged, try direct update first before re-provisioning
	needsReProvision := profileChanged

	if needsReProvision {
		// Flow 2: Delete + Re-provision (profile change)
		if !outputJSON {
			fmt.Printf("\n⚠ Profile change requires ONU re-provisioning (brief service interruption)\n")
			fmt.Printf("Starting delete+re-provision flow...\n\n")
		}

		// CRITICAL: Store serial number before deletion
		serial := preONU.Serial
		if serial == "" {
			return fmt.Errorf("cannot re-provision: ONU serial number not found")
		}

		// Step 1: Delete ONU
		if !outputJSON {
			fmt.Printf("Deleting ONU %d... ", onuUpdateONUID)
		}

		// Build subscriber ID for deletion
		subscriberID := fmt.Sprintf("%s-%d", onuUpdatePONPort, onuUpdateONUID)
		err := conn.driver.DeleteSubscriber(conn.ctx, subscriberID)
		if err != nil {
			if !outputJSON {
				fmt.Printf("FAILED\n")
			}
			return fmt.Errorf("failed to delete ONU: %w", err)
		}

		if !outputJSON {
			fmt.Printf("OK\n")
		}

		// Verify deletion (retry up to 3 times)
		if !outputJSON {
			fmt.Printf("Verifying deletion... ")
		}
		err = verifyONUDeletion(conn.ctx, driverV2, onuUpdatePONPort, onuUpdateONUID)
		if err != nil {
			if !outputJSON {
				fmt.Printf("FAILED\n")
			}
			return fmt.Errorf("failed to verify ONU deletion: %w", err)
		}
		if !outputJSON {
			fmt.Printf("OK\n\n")
		}

		// Wait for OLT to process deletion
		time.Sleep(2 * time.Second)

		// Step 2: Build provision models with new configuration
		vlan := onuUpdateVLAN
		if vlan == 0 {
			vlan = preONU.VLAN // Preserve existing VLAN if not changing
		}

		subscriber, tier := buildProvisionModelsFromUpdate(
			preONU, serial, onuUpdatePONPort, onuUpdateONUID,
			onuUpdateLineProfile, onuUpdateServiceProfile,
			vlan, onuUpdateTrafficProfile, onuUpdateDescription)

		// Step 3: Re-provision
		if !outputJSON {
			if onuUpdateLineProfile != "" {
				fmt.Printf("Re-provisioning with line profile '%s'...\n", onuUpdateLineProfile)
			} else {
				fmt.Printf("Re-provisioning with VLAN %d...\n", vlan)
			}
		}

		// Detect if using line profile for two-step provisioning
		usesLineProfile := onuUpdateLineProfile != "" && strings.Contains(onuUpdateLineProfile, "line")

		_, err = executeProvision(conn.ctx, conn.driver, subscriber, tier, usesLineProfile)
		if err != nil {
			return fmt.Errorf("failed to re-provision ONU: %w", err)
		}

		// Get post-state for output
		postONU, err := lookupONUByPortID(conn.ctx, driverV2, onuUpdatePONPort, onuUpdateONUID)
		if err != nil {
			if !outputJSON {
				fmt.Printf("Warning: Could not verify final state: %v\n", err)
			}
			postONU = preONU
			postONU.VLAN = vlan
			if onuUpdateLineProfile != "" {
				postONU.LineProfile = onuUpdateLineProfile
			}
		}

		if !outputJSON {
			fmt.Printf("\n⚠ Service was interrupted during re-provisioning\n")
		}

		// Output re-provision result
		if !outputJSON {
			fmt.Printf("\nRe-Provision Complete\n")
			fmt.Printf("---------------------\n")
			fmt.Printf("  ONU ID:          %d\n", postONU.ONUID)
			fmt.Printf("  PON Port:        %s\n", postONU.PONPort)
			fmt.Printf("  Serial:          %s\n", postONU.Serial)
			if onuUpdateVLAN > 0 {
				fmt.Printf("  VLAN:            %d → %d\n", preONU.VLAN, postONU.VLAN)
			}
			if onuUpdateLineProfile != "" {
				fmt.Printf("  Line Profile:    %s → %s\n", preONU.LineProfile, postONU.LineProfile)
			}
			return nil
		}

		return outputUpdateResult(preONU, postONU, onuUpdateVLAN, onuUpdateTrafficProfile)
	}

	// Flow 1: Direct VLAN update (only works if NO line profile bound)
	if !outputJSON {
		fmt.Printf("\n")
	}

	// Build update models
	subscriber, tier := buildUpdateModels(preONU, onuUpdatePONPort, onuUpdateONUID, onuUpdateVLAN, onuUpdateTrafficProfile, onuUpdateDescription, onuUpdateLineProfile, onuUpdateServiceProfile)

	// Execute update
	if err := executeUpdate(conn.ctx, conn.driver, subscriber, tier); err != nil {
		// Check if ONU has line profile (may be blocking update)
		if preONU.LineProfile != "" {
			return fmt.Errorf(
				"VLAN update failed. ONU is managed by line profile '%s'. "+
					"Line profiles block direct VLAN changes. "+
					"Use --line-profile to change profile or provide --vlan to trigger delete+re-provision.",
				preONU.LineProfile)
		}
		return err
	}

	// Verify VLAN update with retries if VLAN changed
	if onuUpdateVLAN > 0 {
		if !outputJSON {
			fmt.Printf("Verifying VLAN update... ")
		}
		err = verifyVLANUpdate(conn.ctx, driverV2, onuUpdatePONPort, onuUpdateONUID, onuUpdateVLAN)
		if err != nil {
			if !outputJSON {
				fmt.Printf("FAILED\n")
			}
			// If ONU has line profile, VLAN update may have been blocked
			if preONU.LineProfile != "" {
				return fmt.Errorf(
					"VLAN update verification failed. ONU is managed by line profile '%s' which may be blocking direct VLAN changes. "+
						"Use --line-profile %s to trigger delete+re-provision with the same profile.",
					preONU.LineProfile, preONU.LineProfile)
			}
			return fmt.Errorf("VLAN update failed verification: %w", err)
		}
		if !outputJSON {
			fmt.Printf("OK\n\n")
		}
	}

	// Verify changes (wait for OLT to process)
	time.Sleep(1 * time.Second)
	postONU, err := lookupONUByPortID(conn.ctx, driverV2, onuUpdatePONPort, onuUpdateONUID)
	if err != nil {
		if !outputJSON {
			fmt.Printf("Warning: Could not verify changes: %v\n", err)
		}
		postONU = preONU
	}

	return outputUpdateResult(preONU, postONU, onuUpdateVLAN, onuUpdateTrafficProfile)
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

func runPortPower(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("Port Power\n")
		fmt.Printf("==========\n\n")
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
		fmt.Printf("Getting power readings... ")
	}
	reading, err := driverV2.GetPONPower(conn.ctx, portPONPort)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to get port power: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}

	if outputJSON {
		data, _ := json.MarshalIndent(reading, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Tx Power:     %.3f dBm\n", reading.TxPowerDBm)
	if reading.RxPowerDBm != 0 {
		fmt.Printf("Rx Power:     %.3f dBm\n", reading.RxPowerDBm)
	}
	if reading.Temperature != 0 {
		fmt.Printf("Temperature: %.2f °C\n", reading.Temperature)
	}
	fmt.Printf("Timestamp:   %s\n", reading.Timestamp.Format("2006-01-02 15:04:05"))

	return nil
}

func runServicePortList(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("Service Port List\n")
		fmt.Printf("=================\n\n")
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
		fmt.Printf("Getting service ports... ")
	}
	servicePorts, err := driverV2.ListServicePorts(conn.ctx)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to list service ports: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK (%d found)\n\n", len(servicePorts))
	}

	filtered := servicePorts
	if servicePortListPONPort != "" || servicePortListONUID > 0 {
		filtered = make([]types.ServicePort, 0, len(servicePorts))
		for _, sp := range servicePorts {
			if servicePortListPONPort != "" {
				if sp.Interface != servicePortListPONPort &&
					("0/"+sp.Interface) != servicePortListPONPort &&
					!strings.HasSuffix(servicePortListPONPort, sp.Interface) {
					continue
				}
			}
			if servicePortListONUID > 0 && sp.ONTID != servicePortListONUID {
				continue
			}
			filtered = append(filtered, sp)
		}
	}

	if outputJSON {
		data, _ := json.MarshalIndent(filtered, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(filtered) == 0 {
		fmt.Println("No service ports found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Index\tVLAN\tPON\tONU\tGEM\tUser VLAN\tTag")
	fmt.Fprintln(w, "-----\t----\t---\t---\t---\t---------\t---")
	for _, sp := range filtered {
		userVLAN := "-"
		if sp.UserVLAN != 0 {
			userVLAN = fmt.Sprintf("%d", sp.UserVLAN)
		}
		tag := sp.TagTransform
		if tag == "" {
			tag = "-"
		}
		gem := "-"
		if sp.GemPort != 0 {
			gem = fmt.Sprintf("%d", sp.GemPort)
		}
		fmt.Fprintf(w, "%d\t%d\t%s\t%d\t%s\t%s\t%s\n",
			sp.Index, sp.VLAN, sp.Interface, sp.ONTID, gem, userVLAN, tag)
	}
	w.Flush()

	return nil
}

func runProfileONUList(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("ONU Profile List\n")
		fmt.Printf("================\n\n")
		fmt.Printf("OLT: %s (%s)\n\n", oltAddress, oltVendor)
	}

	conn, err := connectToOLT(60)
	if err != nil {
		return err
	}
	defer conn.close()

	exec, ok := conn.driver.(types.CLIExecutor)
	if !ok {
		return fmt.Errorf("driver for vendor %s does not support CLI execution", oltVendor)
	}

	commands := []string{
		"configure terminal",
		"show profile onu",
		"exit",
	}
	outputs, err := exec.ExecCommands(conn.ctx, commands)
	if err != nil {
		return fmt.Errorf("failed to list ONU profiles: %w", err)
	}
	showOutput := ""
	if len(outputs) >= 2 {
		showOutput = outputs[1]
	}

	if outputJSON {
		payload := map[string]string{"output": showOutput}
		data, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Print(showOutput)
	return nil
}

func runProfileONUGet(cmd *cobra.Command, args []string) error {
	name := args[0]

	if !outputJSON {
		fmt.Printf("ONU Profile Get\n")
		fmt.Printf("===============\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		fmt.Printf("Profile: %s\n\n", name)
	}

	conn, err := connectToOLT(60)
	if err != nil {
		return err
	}
	defer conn.close()

	exec, ok := conn.driver.(types.CLIExecutor)
	if !ok {
		return fmt.Errorf("driver for vendor %s does not support CLI execution", oltVendor)
	}

	commands := []string{
		"configure terminal",
		fmt.Sprintf("show profile onu name %s", name),
		"exit",
	}
	outputs, err := exec.ExecCommands(conn.ctx, commands)
	if err != nil {
		return fmt.Errorf("failed to get ONU profile: %w", err)
	}
	showOutput := ""
	if len(outputs) >= 2 {
		showOutput = outputs[1]
	}

	if outputJSON {
		payload := map[string]string{"output": showOutput}
		data, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Print(showOutput)
	return nil
}

func runProfileONUCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	profile, err := buildProfileFromFlags(name)
	if err != nil {
		return err
	}

	if !outputJSON {
		fmt.Printf("ONU Profile Create\n")
		fmt.Printf("==================\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		fmt.Printf("Profile: %s\n\n", name)
	}

	conn, err := connectToOLT(60)
	if err != nil {
		return err
	}
	defer conn.close()

	exec, ok := conn.driver.(types.CLIExecutor)
	if !ok {
		return fmt.Errorf("driver for vendor %s does not support CLI execution", oltVendor)
	}

	commands := buildProfileCreateCommands(profile)
	outputs, err := exec.ExecCommands(conn.ctx, commands)
	if err != nil {
		return fmt.Errorf("failed to create ONU profile: %w", err)
	}

	if outputJSON {
		payload := map[string]interface{}{
			"commands": commands,
			"output":   outputs,
		}
		data, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Println("Profile created. (If you didn't set any fields, defaults were used.)")
	return nil
}

func runProfileONUDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	if !outputJSON {
		fmt.Printf("ONU Profile Delete\n")
		fmt.Printf("==================\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		fmt.Printf("Profile: %s\n\n", name)
	}

	conn, err := connectToOLT(60)
	if err != nil {
		return err
	}
	defer conn.close()

	exec, ok := conn.driver.(types.CLIExecutor)
	if !ok {
		return fmt.Errorf("driver for vendor %s does not support CLI execution", oltVendor)
	}

	commands := []string{
		"configure terminal",
		fmt.Sprintf("no profile onu name %s", name),
		"exit",
	}
	outputs, err := exec.ExecCommands(conn.ctx, commands)
	if err != nil {
		return fmt.Errorf("failed to delete ONU profile: %w", err)
	}

	if outputJSON {
		payload := map[string]interface{}{
			"commands": commands,
			"output":   outputs,
		}
		data, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Println("Profile delete requested.")
	return nil
}

func buildProfileFromFlags(name string) (*types.ONUHardwareProfile, error) {
	profile := &types.ONUHardwareProfile{
		Name: name,
	}

	if profileONUDescription != "" {
		desc := profileONUDescription
		profile.Description = &desc
	}

	if profileONUPortEth > 0 || profileONUPortPots > 0 || profileONUPortIPHost > 0 || profileONUPortIPv6Host > 0 || profileONUPortVeip > 0 {
		profile.Ports = &types.ONUProfilePorts{}
		if profileONUPortEth > 0 {
			val := profileONUPortEth
			profile.Ports.Eth = &val
		}
		if profileONUPortPots > 0 {
			val := profileONUPortPots
			profile.Ports.Pots = &val
		}
		if profileONUPortIPHost > 0 {
			val := profileONUPortIPHost
			profile.Ports.IPHost = &val
		}
		if profileONUPortIPv6Host > 0 {
			val := profileONUPortIPv6Host
			profile.Ports.IPv6Host = &val
		}
		if profileONUPortVeip > 0 {
			val := profileONUPortVeip
			profile.Ports.Veip = &val
		}
	}

	if profileONUTcontNum > 0 {
		val := profileONUTcontNum
		profile.TcontNum = &val
	}
	if profileONUGemportNum > 0 {
		val := profileONUGemportNum
		profile.GemportNum = &val
	}
	if profileONUSwitchNum > 0 {
		val := profileONUSwitchNum
		profile.SwitchNum = &val
	}
	if profileONUServiceAbility != "" {
		val := profileONUServiceAbility
		profile.ServiceAbility = &val
	}
	if profileONUOmciSendMode != "" {
		val := profileONUOmciSendMode
		profile.OmciSendMode = &val
	}
	if profileONUExOMCI {
		val := true
		profile.ExOMCI = &val
	}
	if profileONUWifiMngViaNonOMCI {
		val := true
		profile.WifiMngViaNonOMCI = &val
	}
	if profileONUDefaultMulticastRange != "" {
		val := profileONUDefaultMulticastRange
		profile.DefaultMulticastRange = &val
	}

	if err := profile.Validate(); err != nil {
		return nil, err
	}
	return profile, nil
}

func buildProfileCreateCommands(profile *types.ONUHardwareProfile) []string {
	commands := []string{
		"configure terminal",
		fmt.Sprintf("profile onu name %s", profile.Name),
	}

	if profile.Ports != nil {
		if profile.Ports.Eth != nil {
			commands = append(commands, fmt.Sprintf("port-num eth %d", *profile.Ports.Eth))
		}
		if profile.Ports.Pots != nil {
			commands = append(commands, fmt.Sprintf("port-num pots %d", *profile.Ports.Pots))
		}
		if profile.Ports.IPHost != nil {
			commands = append(commands, fmt.Sprintf("port-num iphost %d", *profile.Ports.IPHost))
		}
		if profile.Ports.IPv6Host != nil {
			commands = append(commands, fmt.Sprintf("port-num ipv6host %d", *profile.Ports.IPv6Host))
		}
		if profile.Ports.Veip != nil {
			commands = append(commands, fmt.Sprintf("port-num veip %d", *profile.Ports.Veip))
		}
	}

	if profile.TcontNum != nil && profile.GemportNum != nil {
		commands = append(commands, fmt.Sprintf("tcont-num %d gemport-num %d", *profile.TcontNum, *profile.GemportNum))
	}

	if profile.SwitchNum != nil {
		commands = append(commands, fmt.Sprintf("switch-num %d", *profile.SwitchNum))
	}
	if profile.ServiceAbility != nil {
		commands = append(commands, fmt.Sprintf("service-ability %s", *profile.ServiceAbility))
	}
	if profile.OmciSendMode != nil {
		commands = append(commands, fmt.Sprintf("omci-send-mode %s", *profile.OmciSendMode))
	}
	if profile.ExOMCI != nil && *profile.ExOMCI {
		commands = append(commands, "ex-omci")
	}
	if profile.WifiMngViaNonOMCI != nil && *profile.WifiMngViaNonOMCI {
		commands = append(commands, "wifi-mng-via-non-omci")
	}
	if profile.DefaultMulticastRange != nil {
		commands = append(commands, fmt.Sprintf("default-multicast-range %s", *profile.DefaultMulticastRange))
	}
	if profile.Description != nil {
		commands = append(commands, fmt.Sprintf("description %q", *profile.Description))
	}

	commands = append(commands, "commit", "exit", "exit")
	return commands
}
