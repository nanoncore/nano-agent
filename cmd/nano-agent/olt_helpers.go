package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nanoncore/nano-southbound/model"
	"github.com/nanoncore/nano-southbound/types"
)

// =============================================================================
// Connection Helpers
// =============================================================================

// oltConnection holds a connected driver and its V2 interface
type oltConnection struct {
	driver   types.Driver
	driverV2 types.DriverV2
	ctx      context.Context
	cancel   context.CancelFunc
}

// connectToOLT establishes a connection to the OLT and returns the driver
func connectToOLT(timeoutSecs int) (*oltConnection, error) {
	driver, err := createOLTDriver()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)

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
		Timeout:       time.Duration(timeoutSecs) * time.Second,
		Metadata:      make(map[string]string),
	}
	// Add SNMP metadata for drivers that read it from there
	if oltProtocol == "snmp" {
		config.Metadata["snmp_community"] = oltCommunity
		config.Metadata["snmp_version"] = oltSNMPVersion
	}
	if strings.ToLower(oltProtocol) == "cli" {
		config.Metadata["prefer_cli"] = "true"
		if oltAddress == "127.0.0.1" || strings.EqualFold(oltAddress, "localhost") {
			config.Metadata["disable_pager"] = "false"
		}
	}

	if !outputJSON {
		fmt.Printf("Connecting to OLT... ")
	}
	if err := driver.Connect(ctx, config); err != nil {
		cancel()
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n")
	}

	return &oltConnection{
		driver: driver,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// getDriverV2 returns the DriverV2 interface or an error if not supported
func (c *oltConnection) getDriverV2() (types.DriverV2, error) {
	if c.driverV2 != nil {
		return c.driverV2, nil
	}
	driverV2, ok := c.driver.(types.DriverV2)
	if !ok {
		return nil, fmt.Errorf("driver for vendor %s does not support this operation", oltVendor)
	}
	c.driverV2 = driverV2
	return driverV2, nil
}

// close disconnects from the OLT
func (c *oltConnection) close() {
	if c.driver != nil {
		c.driver.Disconnect(c.ctx)
	}
	if c.cancel != nil {
		c.cancel()
	}
}

// =============================================================================
// ONU Lookup Helpers
// =============================================================================

// lookupONUBySerial finds an ONU by serial number
func lookupONUBySerial(ctx context.Context, driverV2 types.DriverV2, serial string) (*types.ONUInfo, error) {
	if !outputJSON {
		fmt.Printf("Looking up ONU by serial... ")
	}
	onu, err := driverV2.GetONUBySerial(ctx, serial)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return nil, fmt.Errorf("failed to find ONU: %w", err)
	}
	if onu == nil {
		if !outputJSON {
			fmt.Printf("NOT FOUND\n")
		}
		return nil, fmt.Errorf("ONU with serial %s not found", serial)
	}
	if !outputJSON {
		fmt.Printf("OK (port %s, id %d)\n", onu.PONPort, onu.ONUID)
	}

	// Enrich with detailed info including VLAN (NAN-242)
	if detailProvider, ok := driverV2.(interface {
		GetONUDetails(ctx context.Context, ponPort string, onuID int) (*types.ONUInfo, error)
	}); ok {
		detailedONU, err := detailProvider.GetONUDetails(ctx, onu.PONPort, onu.ONUID)
		if err == nil && detailedONU != nil {
			// Merge detailed info into basic ONU info
			onu.RxPowerDBm = detailedONU.RxPowerDBm
			onu.TxPowerDBm = detailedONU.TxPowerDBm
			onu.Temperature = detailedONU.Temperature
			onu.Voltage = detailedONU.Voltage
			onu.BiasCurrent = detailedONU.BiasCurrent
			onu.BytesUp = detailedONU.BytesUp
			onu.BytesDown = detailedONU.BytesDown
			onu.PacketsUp = detailedONU.PacketsUp
			onu.PacketsDown = detailedONU.PacketsDown
			onu.InputRateBps = detailedONU.InputRateBps
			onu.OutputRateBps = detailedONU.OutputRateBps
			onu.VLAN = detailedONU.VLAN
		}
	}

	return onu, nil
}

// lookupONUByPortID finds an ONU by PON port and ONU ID
func lookupONUByPortID(ctx context.Context, driverV2 types.DriverV2, ponPort string, onuID int) (*types.ONUInfo, error) {
	if !outputJSON {
		fmt.Printf("Looking up ONU by port/id... ")
	}
	filter := &types.ONUFilter{PONPort: ponPort}
	onus, err := driverV2.GetONUList(ctx, filter)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return nil, fmt.Errorf("failed to get ONU list: %w", err)
	}

	for i := range onus {
		if onus[i].ONUID == onuID && onus[i].PONPort == ponPort {
			if !outputJSON {
				fmt.Printf("OK\n")
			}

			// Enrich with detailed info including VLAN (NAN-242)
			if detailProvider, ok := driverV2.(interface {
				GetONUDetails(ctx context.Context, ponPort string, onuID int) (*types.ONUInfo, error)
			}); ok {
				detailedONU, err := detailProvider.GetONUDetails(ctx, ponPort, onuID)
				if err == nil && detailedONU != nil {
					// Merge detailed info into basic ONU info
					onus[i].RxPowerDBm = detailedONU.RxPowerDBm
					onus[i].TxPowerDBm = detailedONU.TxPowerDBm
					onus[i].Temperature = detailedONU.Temperature
					onus[i].Voltage = detailedONU.Voltage
					onus[i].BiasCurrent = detailedONU.BiasCurrent
					onus[i].BytesUp = detailedONU.BytesUp
					onus[i].BytesDown = detailedONU.BytesDown
					onus[i].PacketsUp = detailedONU.PacketsUp
					onus[i].PacketsDown = detailedONU.PacketsDown
					onus[i].InputRateBps = detailedONU.InputRateBps
					onus[i].OutputRateBps = detailedONU.OutputRateBps
					onus[i].VLAN = detailedONU.VLAN
				}
			}

			return &onus[i], nil
		}
	}

	if !outputJSON {
		fmt.Printf("NOT FOUND\n")
	}
	return nil, fmt.Errorf("ONU %d not found on port %s", onuID, ponPort)
}

// resolveONU looks up an ONU by serial or port/id and returns port and id
func resolveONU(ctx context.Context, driverV2 types.DriverV2, serial, ponPort string, onuID int) (string, int, error) {
	if serial != "" && (ponPort == "" || onuID == 0) {
		onu, err := lookupONUBySerial(ctx, driverV2, serial)
		if err != nil {
			return "", 0, err
		}
		return onu.PONPort, onu.ONUID, nil
	}
	return ponPort, onuID, nil
}

// =============================================================================
// Validation Helpers
// =============================================================================

// serialNumberRegex matches ONU serial number format: 4 letters + 8 hex digits
var serialNumberRegex = regexp.MustCompile(`^[A-Za-z]{4}[0-9A-Fa-f]{8}$`)

// validateSerialNumber validates the ONU serial number format
func validateSerialNumber(serial string) error {
	if serial == "" {
		return fmt.Errorf("serial number is required")
	}
	if !serialNumberRegex.MatchString(serial) {
		return fmt.Errorf("invalid serial number format: must be 4 letters followed by 8 hex digits (e.g., HWTC12345678)")
	}
	return nil
}

// =============================================================================
// Output Helpers
// =============================================================================

// printONURegistration prints ONU registration details
func printONURegistration(onu *types.ONUInfo) {
	fmt.Printf("Registration\n")
	fmt.Printf("------------\n")
	fmt.Printf("  Serial:          %s\n", onu.Serial)
	fmt.Printf("  PON Port:        %s\n", onu.PONPort)
	fmt.Printf("  ONU ID:          %d\n", onu.ONUID)
	if onu.MAC != "" {
		fmt.Printf("  MAC Address:     %s\n", onu.MAC)
	}
	if onu.Model != "" {
		fmt.Printf("  Model:           %s\n", onu.Model)
	}
	fmt.Println()
}

// printONUStatus prints ONU status details
func printONUStatus(onu *types.ONUInfo) {
	fmt.Printf("Status\n")
	fmt.Printf("------\n")
	fmt.Printf("  Admin State:     %s\n", onu.AdminState)
	fmt.Printf("  Oper State:      %s\n", onu.OperState)
	status := "offline"
	if onu.IsOnline {
		status = "online"
	}
	fmt.Printf("  Connection:      %s\n", status)
	if onu.UptimeSeconds > 0 {
		uptime := time.Duration(onu.UptimeSeconds) * time.Second
		fmt.Printf("  Uptime:          %s\n", uptime)
	}
	if !onu.LastOnline.IsZero() {
		fmt.Printf("  Last Online:     %s\n", onu.LastOnline.Format("2006-01-02 15:04:05"))
	}
	fmt.Println()
}

// printOpticalPower prints optical power information
func printOpticalPower(onu *types.ONUInfo, power *types.ONUPowerReading) {
	fmt.Printf("Optical Power\n")
	fmt.Printf("-------------\n")
	if power != nil {
		printPowerReading("ONU Tx", power.TxPowerDBm, types.GPONTxLowThreshold, types.GPONTxHighThreshold)
		printPowerReading("ONU Rx", power.RxPowerDBm, types.GPONRxLowThreshold, types.GPONRxHighThreshold)
		fmt.Printf("  OLT Rx:          %.1f dBm\n", power.OLTRxDBm)
		if power.DistanceM > 0 {
			fmt.Printf("  Distance:        %d m\n", power.DistanceM)
		}
		if power.IsWithinSpec {
			fmt.Printf("  Status:          Within spec\n")
		} else {
			fmt.Printf("  Status:          OUT OF SPEC - check fiber\n")
		}
	} else {
		printBasicPower(onu)
	}
	fmt.Println()
}

// printPowerReading prints a single power reading with spec check
func printPowerReading(label string, value, lowThreshold, highThreshold float64) {
	fmt.Printf("  %-14s %.1f dBm", label+":", value)
	if value < lowThreshold || value > highThreshold {
		fmt.Printf(" [OUT OF SPEC]")
	}
	fmt.Println()
}

// printBasicPower prints basic power info from ONU when detailed reading not available
func printBasicPower(onu *types.ONUInfo) {
	if onu.TxPowerDBm != 0 {
		fmt.Printf("  ONU Tx:          %.1f dBm\n", onu.TxPowerDBm)
	}
	if onu.RxPowerDBm != 0 {
		fmt.Printf("  ONU Rx:          %.1f dBm\n", onu.RxPowerDBm)
	}
	if onu.DistanceM > 0 {
		fmt.Printf("  Distance:        %d m\n", onu.DistanceM)
	}
	if onu.TxPowerDBm == 0 && onu.RxPowerDBm == 0 {
		fmt.Printf("  (detailed power readings not available)\n")
	}
}

// printServiceConfig prints ONU service configuration
func printServiceConfig(onu *types.ONUInfo) {
	fmt.Printf("Service Configuration\n")
	fmt.Printf("---------------------\n")
	printConfigField("ONU Profile", onu.ONUProfile)
	printConfigField("Line Profile", onu.LineProfile)
	printConfigField("Service Profile", onu.ServiceProfile)
	if onu.VLAN > 0 {
		fmt.Printf("  VLAN:            %d\n", onu.VLAN)
	} else {
		fmt.Printf("  VLAN:            -\n")
	}
	if onu.BandwidthDown > 0 || onu.BandwidthUp > 0 {
		fmt.Printf("  Bandwidth:       %d/%d Mbps (down/up)\n", onu.BandwidthDown, onu.BandwidthUp)
	}
	fmt.Println()
}

// printConfigField prints a config field with dash for empty values
func printConfigField(label, value string) {
	if value != "" {
		fmt.Printf("  %-14s %s\n", label+":", value)
	} else {
		fmt.Printf("  %-14s -\n", label+":")
	}
}

// printONUSummary prints a brief ONU summary (for delete/reboot confirmation)
func printONUSummary(onu *types.ONUInfo) {
	fmt.Printf("ONU Details\n")
	fmt.Printf("-----------\n")
	fmt.Printf("  Serial:          %s\n", onu.Serial)
	fmt.Printf("  PON Port:        %s\n", onu.PONPort)
	fmt.Printf("  ONU ID:          %d\n", onu.ONUID)
	status := "offline"
	if onu.IsOnline {
		status = "online"
	}
	fmt.Printf("  Status:          %s\n", status)
	if onu.Model != "" {
		fmt.Printf("  Model:           %s\n", onu.Model)
	}
	fmt.Println()
}

// =============================================================================
// Provisioning Helpers
// =============================================================================

// printProvisionHeader prints the provision command header
func printProvisionHeader(dryRun bool, serial string, vlan, bwDown, bwUp int, ponPort string, onuID int, lineProfile, srvProfile, onuProfile string) {
	if outputJSON {
		return
	}
	if dryRun {
		fmt.Printf("ONU Provisioning (DRY RUN)\n")
		fmt.Printf("==========================\n\n")
	} else {
		fmt.Printf("ONU Provisioning\n")
		fmt.Printf("================\n\n")
	}
	fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
	fmt.Printf("ONU Serial: %s\n", serial)
	fmt.Printf("VLAN: %d\n", vlan)
	fmt.Printf("Bandwidth: %d/%d Mbps (down/up)\n", bwDown, bwUp)
	if ponPort != "" {
		fmt.Printf("Target PON Port: %s\n", ponPort)
	}
	if onuID != 0 {
		fmt.Printf("Target ONU ID: %d\n", onuID)
	}
	if lineProfile != "" {
		fmt.Printf("Line Profile: %s\n", lineProfile)
	}
	if srvProfile != "" {
		fmt.Printf("Service Profile: %s\n", srvProfile)
	}
	if onuProfile != "" {
		fmt.Printf("ONU Profile: %s\n", onuProfile)
	}
	fmt.Println()
}

// printDryRunOutput prints dry run output
func printDryRunOutput(serial string, vlan, bwDown, bwUp int, ponPort string, onuID int) {
	if outputJSON {
		return
	}
	fmt.Printf("DRY RUN - No changes made\n\n")
	fmt.Printf("Would provision ONU with the following configuration:\n")
	fmt.Printf("  Serial:          %s\n", serial)
	fmt.Printf("  VLAN:            %d\n", vlan)
	fmt.Printf("  Bandwidth Down:  %d Mbps\n", bwDown)
	fmt.Printf("  Bandwidth Up:    %d Mbps\n", bwUp)
	if ponPort != "" {
		fmt.Printf("  PON Port:        %s\n", ponPort)
	} else {
		fmt.Printf("  PON Port:        (auto-detect)\n")
	}
	if onuID != 0 {
		fmt.Printf("  ONU ID:          %d\n", onuID)
	} else {
		fmt.Printf("  ONU ID:          (auto-assign)\n")
	}
}

// printProvisionSuccess prints provisioning success message
func printProvisionSuccess(subscriberID, sessionID, assignedIP string) {
	if outputJSON {
		return
	}
	fmt.Printf("Provisioning Complete\n")
	fmt.Printf("---------------------\n")
	if subscriberID != "" {
		fmt.Printf("  Subscriber ID:   %s\n", subscriberID)
	}
	if sessionID != "" {
		fmt.Printf("  Session ID:      %s\n", sessionID)
	}
	if assignedIP != "" {
		fmt.Printf("  Assigned IP:     %s\n", assignedIP)
	}
	fmt.Printf("  Status:          Success\n")
}

// =============================================================================
// Delete/Reboot Helpers
// =============================================================================

// printDeleteHeader prints the delete command header
func printDeleteHeader(serial, ponPort string, onuID int) {
	if outputJSON {
		return
	}
	fmt.Printf("ONU Deletion\n")
	fmt.Printf("============\n\n")
	fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
	if serial != "" {
		fmt.Printf("ONU: %s\n\n", serial)
	} else {
		fmt.Printf("ONU: %s ONU %d\n\n", ponPort, onuID)
	}
}

// printRebootHeader prints the reboot command header
func printRebootHeader(serial, ponPort string, onuID int) {
	if outputJSON {
		return
	}
	fmt.Printf("ONU Reboot\n")
	fmt.Printf("==========\n\n")
	fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
	if serial != "" {
		fmt.Printf("ONU: %s\n\n", serial)
	} else {
		fmt.Printf("ONU: %s ONU %d\n\n", ponPort, onuID)
	}
}

func printSuspendHeader(serial, ponPort string, onuID int) {
	if outputJSON {
		return
	}
	fmt.Printf("ONU Suspend\n")
	fmt.Printf("===========\n\n")
	fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
	if serial != "" {
		fmt.Printf("ONU: %s\n\n", serial)
	} else {
		fmt.Printf("ONU: %s ONU %d\n\n", ponPort, onuID)
	}
}

func printResumeHeader(serial, ponPort string, onuID int) {
	if outputJSON {
		return
	}
	fmt.Printf("ONU Resume\n")
	fmt.Printf("==========\n\n")
	fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
	if serial != "" {
		fmt.Printf("ONU: %s\n\n", serial)
	} else {
		fmt.Printf("ONU: %s ONU %d\n\n", ponPort, onuID)
	}
}

// printDeleteSuccess prints deletion success message
func printDeleteSuccess(ponPort string, onuID int) {
	if outputJSON {
		return
	}
	fmt.Printf("ONU deleted successfully\n")
	fmt.Printf("  PON Port: %s\n", ponPort)
	fmt.Printf("  ONU ID:   %d\n", onuID)
}

// printSuspendSuccess prints suspend success message
func printSuspendSuccess(ponPort string, onuID int) {
	if outputJSON {
		return
	}
	fmt.Printf("ONU suspended successfully\n")
	fmt.Printf("  PON Port: %s\n", ponPort)
	fmt.Printf("  ONU ID:   %d\n", onuID)
}

func printResumeSuccess(ponPort string, onuID int) {
	if outputJSON {
		return
	}
	fmt.Printf("ONU resumed successfully\n")
	fmt.Printf("  PON Port: %s\n", ponPort)
	fmt.Printf("  ONU ID:   %d\n", onuID)
}

// printRebootSuccess prints reboot success message
func printRebootSuccess(ponPort string, onuID int) {
	if outputJSON {
		return
	}
	fmt.Printf("ONU reboot initiated\n")
	fmt.Printf("  PON Port: %s\n", ponPort)
	fmt.Printf("  ONU ID:   %d\n", onuID)
	fmt.Printf("\nNote: ONU will be temporarily offline during reboot.\n")
}

// executeDelete performs the ONU deletion
func executeDelete(ctx context.Context, driver types.Driver, serial, ponPort string, onuID int) error {
	if !outputJSON {
		fmt.Printf("Deleting ONU... ")
	}
	// Always use ont-frame/slot/port-onuID format as that's what DeleteSubscriber expects
	// ponPort is in format "frame/slot/port", e.g., "0/0/1"
	subscriberID := fmt.Sprintf("ont-%s-%d", ponPort, onuID)
	if err := driver.DeleteSubscriber(ctx, subscriberID); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("deletion failed: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}
	return nil
}

// executeSuspend performs the ONU suspend action
func executeSuspend(ctx context.Context, driver types.Driver, ponPort string, onuID int) error {
	if !outputJSON {
		fmt.Printf("Suspending ONU... ")
	}
	subscriberID := fmt.Sprintf("ont-%s-%d", ponPort, onuID)
	if err := driver.SuspendSubscriber(ctx, subscriberID); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("suspend failed: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}
	return nil
}

// executeResume performs the ONU resume action
func executeResume(ctx context.Context, driver types.Driver, ponPort string, onuID int) error {
	if !outputJSON {
		fmt.Printf("Resuming ONU... ")
	}
	subscriberID := fmt.Sprintf("ont-%s-%d", ponPort, onuID)
	if err := driver.ResumeSubscriber(ctx, subscriberID); err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("resume failed: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}
	return nil
}

// executeReboot performs the ONU reboot
func executeReboot(ctx context.Context, driverV2 types.DriverV2, ponPort string, onuID int) error {
	if !outputJSON {
		fmt.Printf("Rebooting ONU... ")
	}
	result, err := driverV2.RestartONU(ctx, ponPort, onuID)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("reboot failed: %w", err)
	}
	if !outputJSON {
		if result != nil && result.Success {
			fmt.Printf("OK\n")
			fmt.Printf("  Deactivate: %s (verified: %v)\n",
				boolToStatus(result.DeactivateSuccess), result.DeactivateVerified)
			fmt.Printf("  Activate: %s (verified: %v)\n",
				boolToStatus(result.ActivateSuccess), result.ActivateVerified)
			if result.RetryCount > 0 {
				fmt.Printf("  Retries: %d\n", result.RetryCount)
			}
			fmt.Printf("  %s\n\n", result.Message)
		} else {
			fmt.Printf("PARTIAL\n")
			if result != nil {
				fmt.Printf("  %s\n\n", result.Message)
			}
		}
	}
	return nil
}

// boolToStatus converts a boolean to a status string
func boolToStatus(b bool) string {
	if b {
		return "OK"
	}
	return "FAILED"
}

// =============================================================================
// Update Helpers
// =============================================================================

func printUpdateHeader(ponPort string, onuID, vlan, trafficProfile int, description, lineProfile, serviceProfile string) {
	if outputJSON {
		return
	}
	fmt.Printf("ONU Configuration Update\n")
	fmt.Printf("========================\n\n")
	fmt.Printf("OLT: %s (%s)\n", oltAddress, oltVendor)
	fmt.Printf("ONU: %s ONU %d\n\n", ponPort, onuID)
	fmt.Printf("Updates to Apply:\n")
	if vlan > 0 {
		fmt.Printf("  VLAN:            %d\n", vlan)
	}
	if trafficProfile > 0 {
		fmt.Printf("  Traffic Profile: %d\n", trafficProfile)
	}
	if description != "" {
		fmt.Printf("  Description:     %s\n", description)
	}
	if lineProfile != "" {
		fmt.Printf("  Line Profile:    %s\n", lineProfile)
	}
	if serviceProfile != "" {
		fmt.Printf("  Service Profile: %s\n", serviceProfile)
	}
	fmt.Println()
}

func printCurrentConfig(onu *types.ONUInfo) {
	fmt.Printf("Current Configuration\n")
	fmt.Printf("---------------------\n")
	fmt.Printf("  Serial:          %s\n", onu.Serial)
	fmt.Printf("  Status:          %s\n", onu.OperState)
	if onu.VLAN > 0 {
		fmt.Printf("  VLAN:            %d\n", onu.VLAN)
	}
	if onu.ONUProfile != "" {
		fmt.Printf("  ONU Profile:     %s\n", onu.ONUProfile)
	}
	if onu.LineProfile != "" {
		fmt.Printf("  Line Profile:    %s\n", onu.LineProfile)
	}
	if onu.ServiceProfile != "" {
		fmt.Printf("  Service Profile: %s\n", onu.ServiceProfile)
	}
	fmt.Println()
}

// validateProfileVLANConsistency checks if line profile and VLAN are consistent
// Returns decision ("profile" | "direct-vlan") and error if validation fails
func validateProfileVLANConsistency(lineProfile string, vlan int, force bool) (string, error) {
	// No validation needed if either parameter is empty/zero
	if lineProfile == "" || vlan == 0 {
		return "", nil
	}

	// Extract VLAN from profile name convention (line_vlan_100 → 100)
	re := regexp.MustCompile(`(?:line[_-])?vlan[_-](\d+)`)
	match := re.FindStringSubmatch(lineProfile)

	if len(match) == 2 {
		extractedVLAN, err := strconv.Atoi(match[1])
		if err == nil {
			if extractedVLAN == vlan {
				// Match: Apply profile binding
				return "profile", nil
			}

			// Mismatch detected
			if force {
				// User confirmed: unbind and apply direct VLAN
				return "direct-vlan", nil
			}

			// No --force: Fail validation
			return "", fmt.Errorf(
				"profile/VLAN mismatch: profile '%s' implies VLAN %d, but --vlan %d provided\n"+
					"  Use profile-only:  --line-profile %s (omit --vlan)\n"+
					"  Use VLAN-only:     --vlan %d (omit --line-profile)\n"+
					"  Use --force flag:  --vlan %d --force (unbind profile, apply direct config)",
				lineProfile, extractedVLAN, vlan, lineProfile, vlan, vlan,
			)
		}
	}

	// Convention not followed: trust user, apply profile
	return "profile", nil
}

// buildUpdateModels creates subscriber and tier models for update
func buildUpdateModels(preONU *types.ONUInfo, ponPort string, onuID, vlan, trafficProfile int, description, lineProfile, serviceProfile string) (*model.Subscriber, *model.ServiceTier) {
	subscriber := &model.Subscriber{
		Name: preONU.Serial,
		Annotations: map[string]string{
			"nano.io/pon-port": ponPort,
			"nano.io/onu-id":   fmt.Sprintf("%d", onuID),
		},
		Spec: model.SubscriberSpec{
			ONUSerial: preONU.Serial,
			VLAN:      preONU.VLAN, // Keep existing VLAN by default
			Tier:      "cli-update",
		},
	}

	// Three-way logic for profile/VLAN updates (NAN-251)
	if lineProfile != "" || serviceProfile != "" {
		// Scenario 1: Profile update
		if lineProfile != "" {
			subscriber.Annotations["nano.io/line-profile"] = lineProfile
		}
		if serviceProfile != "" {
			subscriber.Annotations["nano.io/service-profile"] = serviceProfile
		}
	} else if vlan > 0 {
		// Scenario 2/3: VLAN-only update
		subscriber.Spec.VLAN = vlan
		if preONU.LineProfile != "" {
			// Mark for profile unbinding (ONU currently has profile)
			subscriber.Annotations["nano.io/unbind-profile"] = "true"
		}
	} else {
		// No VLAN or profile update, preserve existing profiles
		if preONU.LineProfile != "" {
			subscriber.Annotations["nano.io/line-profile"] = preONU.LineProfile
		}
		if preONU.ServiceProfile != "" {
			subscriber.Annotations["nano.io/service-profile"] = preONU.ServiceProfile
		}
	}

	// Apply VLAN update for profile scenario (match case)
	if vlan > 0 && (lineProfile != "" || serviceProfile != "") {
		subscriber.Spec.VLAN = vlan
	}

	// Apply description update if specified
	if description != "" {
		subscriber.Spec.Description = description
	}

	tier := &model.ServiceTier{
		Name: "cli-update",
		Spec: model.ServiceTierSpec{
			BandwidthDown: preONU.BandwidthDown,
			BandwidthUp:   preONU.BandwidthUp,
			QoSClass:      "standard",
		},
	}

	// Apply traffic profile (bandwidth) update if specified
	// Note: trafficProfile parameter would need to be mapped to actual bandwidth values
	// For now, we preserve existing bandwidth unless explicitly changed via tier
	if trafficProfile > 0 {
		// Store traffic profile ID in annotations for driver to handle
		subscriber.Annotations["nano.io/traffic-profile"] = fmt.Sprintf("%d", trafficProfile)
	}

	return subscriber, tier
}

// buildProvisionModelsFromUpdate converts update parameters into provision models
// Used for delete+re-provision flow when profile changes (NAN-259)
func buildProvisionModelsFromUpdate(
	preONU *types.ONUInfo,
	serial string,
	ponPort string,
	onuID int,
	lineProfile string,
	serviceProfile string,
	vlan int,
	trafficProfile int,
	description string,
) (*model.Subscriber, *model.ServiceTier) {
	subscriber := &model.Subscriber{
		Name: serial,
		Annotations: map[string]string{
			"nano.io/pon-port": ponPort,
			"nano.io/onu-id":   strconv.Itoa(onuID),
		},
		Spec: model.SubscriberSpec{
			ONUSerial: serial,
			VLAN:      vlan,
		},
	}

	// Add profile annotations if provided
	if lineProfile != "" {
		subscriber.Annotations["nano.io/line-profile"] = lineProfile
	}
	if serviceProfile != "" {
		subscriber.Annotations["nano.io/service-profile"] = serviceProfile
	}

	// Add description if provided
	if description != "" {
		subscriber.Annotations["nano.io/description"] = description
	}

	// Preserve bandwidth or use provided traffic profile
	bandwidthDown := preONU.BandwidthDown
	bandwidthUp := preONU.BandwidthUp

	if trafficProfile > 0 {
		// If traffic profile provided, use it (actual values depend on profile mapping)
		bandwidthDown = trafficProfile // Simplified - actual implementation needs profile lookup
		bandwidthUp = trafficProfile / 2
	} else {
		// Preserve existing bandwidth, or use defaults if not set
		if bandwidthDown == 0 {
			bandwidthDown = 100 // Default 100 Mbps
		}
		if bandwidthUp == 0 {
			bandwidthUp = 50 // Default 50 Mbps
		}
	}

	tier := &model.ServiceTier{
		Spec: model.ServiceTierSpec{
			BandwidthDown: bandwidthDown,
			BandwidthUp:   bandwidthUp,
		},
	}

	return subscriber, tier
}

// executeUpdate performs the ONU configuration update
func executeUpdate(ctx context.Context, driver types.Driver, subscriber *model.Subscriber, tier *model.ServiceTier) error {
	if !outputJSON {
		fmt.Printf("Updating ONU configuration... ")
	}
	err := driver.UpdateSubscriber(ctx, subscriber, tier)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return fmt.Errorf("update failed: %w", err)
	}
	if !outputJSON {
		fmt.Printf("OK\n\n")
	}
	return nil
}

func outputUpdateResult(preONU, postONU *types.ONUInfo, vlan, trafficProfile int) error {
	if outputJSON {
		return outputUpdateResultJSON(preONU, postONU, vlan, trafficProfile)
	}

	fmt.Printf("\nUpdate Complete\n")
	fmt.Printf("---------------\n")

	if vlan > 0 {
		if postONU.VLAN == vlan {
			fmt.Printf("  VLAN:            %d → %d ✓\n", preONU.VLAN, postONU.VLAN)
		} else {
			fmt.Printf("  VLAN:            %d → %d (verification pending)\n", preONU.VLAN, vlan)
		}
	}

	if trafficProfile > 0 {
		fmt.Printf("  Traffic Profile: Applied (ID: %d)\n", trafficProfile)
	}

	fmt.Printf("  Status:          %s\n", postONU.OperState)

	if postONU.IsOnline {
		fmt.Printf("\nONU is online and operational.\n")
	}

	return nil
}

func outputUpdateResultJSON(preONU, postONU *types.ONUInfo, vlan, trafficProfile int) error {
	output := struct {
		Success   bool                   `json:"success"`
		PreState  *types.ONUInfo         `json:"pre_state"`
		PostState *types.ONUInfo         `json:"post_state"`
		Updates   map[string]interface{} `json:"updates"`
	}{
		Success:   true,
		PreState:  preONU,
		PostState: postONU,
		Updates:   make(map[string]interface{}),
	}

	if vlan > 0 {
		output.Updates["vlan"] = map[string]interface{}{
			"requested": vlan,
			"previous":  preONU.VLAN,
			"current":   postONU.VLAN,
			"verified":  postONU.VLAN == vlan,
		}
	}

	if trafficProfile > 0 {
		output.Updates["traffic_profile"] = map[string]interface{}{
			"requested": trafficProfile,
			"applied":   true,
		}
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(data))
	return nil
}

// extractVLANFromProfileName extracts VLAN ID from a profile name following naming convention
// Supports formats: line_vlan_100, line-vlan-100, vlan_100, vlan-100
// Returns VLAN ID and nil error if found, otherwise returns 0 and error (NAN-258)
func extractVLANFromProfileName(profileName string) (int, error) {
	// Match patterns like: line_vlan_100, line-vlan-100, vlan_100, vlan-100
	re := regexp.MustCompile(`(?:line[_-])?vlan[_-](\d+)`)
	if match := re.FindStringSubmatch(profileName); len(match) == 2 {
		vlan, err := strconv.Atoi(match[1])
		if err != nil {
			return 0, fmt.Errorf("invalid VLAN number in profile name: %w", err)
		}
		return vlan, nil
	}
	return 0, fmt.Errorf("VLAN not found in profile name %q (expected format: line_vlan_XXX or vlan_XXX)", profileName)
}

// =============================================================================
// Verification & Retry Logic (NAN-257)
// =============================================================================

// verifyONUChange performs a generic retry verification of an OLT operation
// It retries up to maxRetries times with retryDelay between attempts
func verifyONUChange(ctx context.Context, verifyFunc func() (bool, error), maxRetries int, retryDelay time.Duration) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Wait before checking (give OLT time to process)
		time.Sleep(retryDelay)

		success, err := verifyFunc()
		if err != nil {
			return fmt.Errorf("verification error: %w", err)
		}

		if success {
			if attempt > 1 && !outputJSON {
				fmt.Printf("  Verified on attempt %d/%d\n", attempt, maxRetries)
			}
			return nil
		}

		if attempt < maxRetries && !outputJSON {
			fmt.Printf("  Change not reflected yet (attempt %d/%d), retrying...\n", attempt, maxRetries)
		}
	}

	return fmt.Errorf("change not reflected on OLT after %d attempts", maxRetries)
}

// verifyONUProvision verifies that an ONU was successfully provisioned
func verifyONUProvision(ctx context.Context, driverV2 types.DriverV2, ponPort string, onuID int) error {
	if !outputJSON {
		fmt.Printf("Verifying ONU provisioning... ")
	}

	verifyFunc := func() (bool, error) {
		onu, err := lookupONUByPortID(ctx, driverV2, ponPort, onuID)
		if err != nil {
			return false, nil // ONU not found yet, but not an error - keep retrying
		}
		// ONU should exist and be online
		return onu != nil && onu.IsOnline, nil
	}

	err := verifyONUChange(ctx, verifyFunc, 6, 2*time.Second)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return err
	}

	if !outputJSON {
		fmt.Printf("OK\n")
	}
	return nil
}

// verifyVLANUpdate verifies that a VLAN update was successfully applied
func verifyVLANUpdate(ctx context.Context, driverV2 types.DriverV2, ponPort string, onuID, expectedVLAN int) error {
	if !outputJSON {
		fmt.Printf("Verifying VLAN update... ")
	}

	// Check if driver has SNMP VLAN verification capability
	type vlanVerifier interface {
		GetONUVLANViaSNMP(ctx context.Context, ponPort string, onuID int) (int, error)
	}

	verifier, hasSNMP := driverV2.(vlanVerifier)

	verifyFunc := func() (bool, error) {
		if hasSNMP {
			// Prefer SNMP verification for accuracy
			snmpVLAN, err := verifier.GetONUVLANViaSNMP(ctx, ponPort, onuID)
			if err != nil {
				return false, nil // SNMP failed, retry
			}
			return snmpVLAN == expectedVLAN, nil
		}

		// Fallback: Check via GetONUDetails
		if detailProvider, ok := driverV2.(interface {
			GetONUDetails(ctx context.Context, ponPort string, onuID int) (*types.ONUInfo, error)
		}); ok {
			onu, err := detailProvider.GetONUDetails(ctx, ponPort, onuID)
			if err != nil {
				return false, nil // Details fetch failed, retry
			}
			return onu.VLAN == expectedVLAN, nil
		}

		// No verification method available
		return false, fmt.Errorf("driver does not support VLAN verification")
	}

	err := verifyONUChange(ctx, verifyFunc, 3, 2*time.Second)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return err
	}

	if !outputJSON {
		fmt.Printf("OK\n")
	}
	return nil
}

// verifyLineProfileAssociation verifies that a line profile was applied to an ONU
func verifyLineProfileAssociation(ctx context.Context, driverV2 types.DriverV2, ponPort string, onuID int, lineProfile string) error {
	if !outputJSON {
		fmt.Printf("Verifying line profile association... ")
	}

	// Check if driver has running config capability
	type configProvider interface {
		GetONURunningConfig(ctx context.Context, ponPort string, onuID int) (string, error)
	}

	provider, hasConfig := driverV2.(configProvider)
	if !hasConfig {
		// No verification method available, skip
		if !outputJSON {
			fmt.Printf("SKIPPED (not supported)\n")
		}
		return nil
	}

	verifyFunc := func() (bool, error) {
		config, err := provider.GetONURunningConfig(ctx, ponPort, onuID)
		if err != nil {
			return false, nil // Config fetch failed, retry
		}
		// Check if line profile is bound to the expected ONU ID
		if strings.Contains(config, fmt.Sprintf("onu %d profile line name %s", onuID, lineProfile)) {
			return true, nil
		}
		// Fallback for partial configs
		if strings.Contains(config, fmt.Sprintf("profile line name %s", lineProfile)) {
			return true, nil
		}
		pattern := fmt.Sprintf(`profile line id \d+ name %s`, regexp.QuoteMeta(lineProfile))
		return regexp.MustCompile(pattern).MatchString(config), nil
	}

	err := verifyONUChange(ctx, verifyFunc, 3, 2*time.Second)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return err
	}

	if !outputJSON {
		fmt.Printf("OK\n")
	}
	return nil
}

func parseMetadataInt(metadata map[string]any, key string) (int, bool) {
	if metadata == nil {
		return 0, false
	}
	value, ok := metadata[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// verifyONUDeletion verifies that an ONU was successfully deleted
func verifyONUDeletion(ctx context.Context, driverV2 types.DriverV2, ponPort string, onuID int) error {
	if !outputJSON {
		fmt.Printf("Verifying ONU deletion... ")
	}

	verifyFunc := func() (bool, error) {
		onu, err := lookupONUByPortID(ctx, driverV2, ponPort, onuID)
		if err != nil {
			// ONU not found - deletion successful
			if strings.Contains(err.Error(), "not found") {
				return true, nil
			}
			return false, nil // Other error, retry
		}
		// ONU still exists
		return onu == nil, nil
	}

	err := verifyONUChange(ctx, verifyFunc, 3, 1*time.Second)
	if err != nil {
		if !outputJSON {
			fmt.Printf("FAILED\n")
		}
		return err
	}

	if !outputJSON {
		fmt.Printf("OK\n")
	}
	return nil
}
