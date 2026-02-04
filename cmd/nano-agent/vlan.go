package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nanoncore/nano-southbound/types"
	"github.com/spf13/cobra"
)

// VLAN command flags
var (
	vlanID          int
	vlanName        string
	vlanDescription string
	vlanForce       bool
)

// Service port flags
var (
	spPONPort      string
	spONTID        int
	spVLAN         int
	spGemPort      int
	spUserVLAN     int
	spTagTransform string
	spETHPort      int
)

var vlanListCmd = &cobra.Command{
	Use:   "vlan-list",
	Short: "List all VLANs on the OLT",
	Long: `List all configured VLANs on the OLT.

Displays VLAN ID, name, type, service port count, and description.

Examples:
  # List all VLANs
  nano-agent vlan-list --vendor huawei --address 192.168.1.1 \
    --username admin --password admin

  # Output as JSON
  nano-agent vlan-list --vendor huawei --address 192.168.1.1 \
    --username admin --password admin --json`,
	RunE: runVLANList,
}

var vlanCreateCmd = &cobra.Command{
	Use:   "vlan-create",
	Short: "Create a new VLAN on the OLT",
	Long: `Create a new VLAN on the OLT with the specified ID and name.

Examples:
  # Create a VLAN
  nano-agent vlan-create --vlan-id 100 --name "Customer_VLAN" \
    --vendor huawei --address 192.168.1.1 --username admin --password admin

  # Create with description
  nano-agent vlan-create --vlan-id 100 --name "Customer_VLAN" \
    --description "Customer traffic VLAN" \
    --vendor huawei --address 192.168.1.1 --username admin --password admin`,
	RunE: runVLANCreate,
}

var vlanGetCmd = &cobra.Command{
	Use:   "vlan-get",
	Short: "Get VLAN details by ID",
	Long: `Get VLAN details for the specified VLAN ID.

Examples:
  # Get VLAN details
  nano-agent vlan-get --vlan-id 702 \
    --vendor vsol --address 10.0.0.254 --username admin --password admin

  # Output as JSON
  nano-agent vlan-get --vlan-id 702 \
    --vendor vsol --address 10.0.0.254 --username admin --password admin --json`,
	RunE: runVLANGet,
}

var vlanDeleteCmd = &cobra.Command{
	Use:   "vlan-delete",
	Short: "Delete a VLAN from the OLT",
	Long: `Delete a VLAN from the OLT.

WARNING: This will fail if the VLAN has service ports configured.
Use --force to override this check.

Examples:
  # Delete a VLAN
  nano-agent vlan-delete --vlan-id 100 --force \
    --vendor huawei --address 192.168.1.1 --username admin --password admin`,
	RunE: runVLANDelete,
}

var servicePortAddCmd = &cobra.Command{
	Use:   "service-port-add",
	Short: "Add a service port mapping",
	Long: `Create a service port mapping between a VLAN and an ONU.

Examples:
  # Add service port
  nano-agent service-port-add --pon-port 0/0/1 --ont-id 101 --vlan-id 100 \
    --vendor huawei --address 192.168.1.1 --username admin --password admin

  # With custom gemport and user-vlan
  nano-agent service-port-add --pon-port 0/0/1 --ont-id 101 --vlan-id 100 \
    --gemport 2 --user-vlan 200 \
    --vendor huawei --address 192.168.1.1 --username admin --password admin`,
	RunE: runServicePortAdd,
}

var servicePortDeleteCmd = &cobra.Command{
	Use:   "service-port-delete",
	Short: "Delete a service port mapping",
	Long: `Delete a service port mapping for an ONU.

Examples:
  # Delete service port mapping for an ONU
  nano-agent service-port-delete --pon-port 0/0/1 --ont-id 101 \
    --vendor huawei --address 192.168.1.1 --username admin --password admin`,
	RunE: runServicePortDelete,
}

func init() {
	// Add VLAN commands with common OLT connection flags
	vlanCommands := []*cobra.Command{
		vlanListCmd, vlanGetCmd, vlanCreateCmd, vlanDeleteCmd, servicePortAddCmd, servicePortDeleteCmd,
	}
	for _, cmd := range vlanCommands {
		cmd.Flags().StringVar(&oltVendor, "vendor", "", "OLT vendor [required]")
		cmd.Flags().StringVar(&oltAddress, "address", "", "OLT IP address [required]")
		cmd.Flags().IntVar(&oltPort, "port", 0, "OLT management port")
		cmd.Flags().StringVar(&oltProtocol, "protocol", "", "Management protocol")
		cmd.Flags().StringVar(&oltUsername, "username", "", "OLT username [required]")
		cmd.Flags().StringVar(&oltPassword, "password", "", "OLT password [required]")
		cmd.Flags().BoolVar(&oltTLS, "tls", false, "Enable TLS")
		cmd.Flags().BoolVar(&oltTLSSkipVe, "tls-skip-verify", false, "Skip TLS verification")
		cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")

		cmd.MarkFlagRequired("vendor")
		cmd.MarkFlagRequired("address")
		cmd.MarkFlagRequired("username")
		cmd.MarkFlagRequired("password")
	}

	// vlan-create flags
	vlanCreateCmd.Flags().IntVar(&vlanID, "vlan-id", 0, "VLAN ID (1-4094) [required]")
	vlanCreateCmd.Flags().StringVar(&vlanName, "name", "", "VLAN name")
	vlanCreateCmd.Flags().StringVar(&vlanDescription, "description", "", "VLAN description")
	vlanCreateCmd.MarkFlagRequired("vlan-id")

	// vlan-get flags
	vlanGetCmd.Flags().IntVar(&vlanID, "vlan-id", 0, "VLAN ID (1-4094) [required]")
	vlanGetCmd.MarkFlagRequired("vlan-id")

	// vlan-delete flags
	vlanDeleteCmd.Flags().IntVar(&vlanID, "vlan-id", 0, "VLAN ID [required]")
	vlanDeleteCmd.Flags().BoolVar(&vlanForce, "force", false, "Force deletion even with service ports")
	vlanDeleteCmd.MarkFlagRequired("vlan-id")

	// service-port-add flags
	servicePortAddCmd.Flags().StringVar(&spPONPort, "pon-port", "", "PON port (e.g., 0/0/1) [required]")
	servicePortAddCmd.Flags().IntVar(&spONTID, "ont-id", 0, "ONT ID [required]")
	servicePortAddCmd.Flags().IntVar(&spVLAN, "vlan-id", 0, "VLAN ID [required]")
	servicePortAddCmd.Flags().IntVar(&spGemPort, "gemport", 1, "GEM port (default: 1)")
	servicePortAddCmd.Flags().IntVar(&spUserVLAN, "user-vlan", 0, "User VLAN (default: same as vlan-id)")
	servicePortAddCmd.Flags().StringVar(&spTagTransform, "tag-transform", "translate", "Tag transform mode")
	servicePortAddCmd.Flags().IntVar(&spETHPort, "eth-port", 1, "Ethernet port (default: 1)")
	servicePortAddCmd.MarkFlagRequired("pon-port")
	servicePortAddCmd.MarkFlagRequired("ont-id")
	servicePortAddCmd.MarkFlagRequired("vlan-id")

	// service-port-delete flags
	servicePortDeleteCmd.Flags().StringVar(&spPONPort, "pon-port", "", "PON port (e.g., 0/0/1) [required]")
	servicePortDeleteCmd.Flags().IntVar(&spONTID, "ont-id", 0, "ONT ID [required]")
	servicePortDeleteCmd.MarkFlagRequired("pon-port")
	servicePortDeleteCmd.MarkFlagRequired("ont-id")

	// Add to root command
	rootCmd.AddCommand(vlanListCmd)
	rootCmd.AddCommand(vlanGetCmd)
	rootCmd.AddCommand(vlanCreateCmd)
	rootCmd.AddCommand(vlanDeleteCmd)
	rootCmd.AddCommand(servicePortAddCmd)
	rootCmd.AddCommand(servicePortDeleteCmd)
}

func runVLANList(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("VLAN List\n")
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
		fmt.Printf("Getting VLAN list... ")
	}
	vlans, err := driverV2.ListVLANs(conn.ctx)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to list VLANs: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK (%d found)\n\n", len(vlans))
	}

	if outputJSON {
		data, _ := json.MarshalIndent(vlans, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(vlans) == 0 {
		fmt.Println("No VLANs configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VLAN ID\tName\tType\tService Ports\tDescription")
	fmt.Fprintln(w, "-------\t----\t----\t-------------\t-----------")
	for _, vlan := range vlans {
		desc := vlan.Description
		if desc == "" {
			desc = "-"
		}
		name := vlan.Name
		if name == "" {
			name = "-"
		}
		vlanType := vlan.Type
		if vlanType == "" {
			vlanType = "smart"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%s\n",
			vlan.ID, name, vlanType, vlan.ServicePortCount, desc)
	}
	w.Flush()

	return nil
}

func runVLANGet(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("VLAN Details\n")
		fmt.Printf("===========\n\n")
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
		fmt.Printf("Getting VLAN... ")
	}
	vlan, err := driverV2.GetVLAN(conn.ctx, vlanID)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to get VLAN %d: %w", vlanID, err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}

	if vlan == nil {
		if outputJSON {
			fmt.Println("null")
		} else {
			fmt.Printf("VLAN %d not found.\n", vlanID)
		}
		return nil
	}

	if outputJSON {
		data, _ := json.MarshalIndent(vlan, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Field\tValue")
	fmt.Fprintln(w, "-----\t-----")
	fmt.Fprintf(w, "VLAN ID\t%d\n", vlan.ID)
	fmt.Fprintf(w, "Name\t%s\n", fallbackString(vlan.Name, "-"))
	fmt.Fprintf(w, "Type\t%s\n", fallbackString(vlan.Type, "static"))
	fmt.Fprintf(w, "Service Ports\t%d\n", vlan.ServicePortCount)
	fmt.Fprintf(w, "Description\t%s\n", fallbackString(vlan.Description, "-"))
	w.Flush()

	return nil
}

func fallbackString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func runVLANCreate(cmd *cobra.Command, args []string) error {
	// Validate VLAN ID
	if vlanID < 1 || vlanID > 4094 {
		return fmt.Errorf("VLAN ID must be between 1 and 4094")
	}

	if !outputJSON {
		fmt.Printf("VLAN Create\n")
		fmt.Printf("===========\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		fmt.Printf("VLAN ID: %d\n", vlanID)
		if vlanName != "" {
			fmt.Printf("Name: %s\n", vlanName)
		}
		fmt.Println()
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

	req := &types.CreateVLANRequest{
		ID:          vlanID,
		Name:        vlanName,
		Description: vlanDescription,
	}

	if !outputJSON {
		fmt.Printf("Creating VLAN %d... ", vlanID)
	}
	if err := driverV2.CreateVLAN(conn.ctx, req); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to create VLAN: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
		fmt.Printf("VLAN %d created successfully\n", vlanID)
	}

	if outputJSON {
		output := struct {
			Status string `json:"status"`
			VLANID int    `json:"vlan_id"`
			Name   string `json:"name,omitempty"`
		}{Status: "created", VLANID: vlanID, Name: vlanName}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
	}

	return nil
}

func runVLANDelete(cmd *cobra.Command, args []string) error {
	if !vlanForce {
		return fmt.Errorf("this is a destructive operation; use --force to confirm")
	}

	if !outputJSON {
		fmt.Printf("VLAN Delete\n")
		fmt.Printf("===========\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		fmt.Printf("VLAN ID: %d\n\n", vlanID)
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
		fmt.Printf("Deleting VLAN %d... ", vlanID)
	}
	if err := driverV2.DeleteVLAN(conn.ctx, vlanID, vlanForce); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to delete VLAN: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
		fmt.Printf("VLAN %d deleted successfully\n", vlanID)
	}

	if outputJSON {
		output := struct {
			Status string `json:"status"`
			VLANID int    `json:"vlan_id"`
		}{Status: "deleted", VLANID: vlanID}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
	}

	return nil
}

func runServicePortAdd(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("Service Port Add\n")
		fmt.Printf("================\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		fmt.Printf("PON Port: %s\n", spPONPort)
		fmt.Printf("ONT ID: %d\n", spONTID)
		fmt.Printf("VLAN: %d\n\n", spVLAN)
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

	req := &types.AddServicePortRequest{
		VLAN:         spVLAN,
		PONPort:      spPONPort,
		ONTID:        spONTID,
		GemPort:      spGemPort,
		UserVLAN:     spUserVLAN,
		TagTransform: spTagTransform,
		ETHPort:      spETHPort,
	}

	if !outputJSON {
		fmt.Printf("Adding service port... ")
	}
	if err := driverV2.AddServicePort(conn.ctx, req); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to add service port: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
		fmt.Printf("Service port added successfully\n")
	}

	if outputJSON {
		output := struct {
			Status  string `json:"status"`
			PONPort string `json:"pon_port"`
			ONTID   int    `json:"ont_id"`
			VLAN    int    `json:"vlan"`
		}{Status: "created", PONPort: spPONPort, ONTID: spONTID, VLAN: spVLAN}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
	}

	return nil
}

func runServicePortDelete(cmd *cobra.Command, args []string) error {
	if !outputJSON {
		fmt.Printf("Service Port Delete\n")
		fmt.Printf("===================\n\n")
		fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
		fmt.Printf("PON Port: %s\n", spPONPort)
		fmt.Printf("ONT ID: %d\n\n", spONTID)
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
		fmt.Printf("Deleting service port... ")
	}
	if err := driverV2.DeleteServicePort(conn.ctx, spPONPort, spONTID); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("failed to delete service port: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
		fmt.Printf("Service port deleted successfully\n")
	}

	if outputJSON {
		output := struct {
			Status  string `json:"status"`
			PONPort string `json:"pon_port"`
			ONTID   int    `json:"ont_id"`
		}{Status: "deleted", PONPort: spPONPort, ONTID: spONTID}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
	}

	return nil
}
