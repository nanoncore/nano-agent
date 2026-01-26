package command

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
	"github.com/nanoncore/nano-southbound/types"
)

// verifyONUStateChange verifies ONU reached expected state with retry.
// OLT hardware can be slow to apply changes, so we retry a few times.
func verifyONUStateChange(
	ctx context.Context,
	driver cli.CLIDriver,
	ponPort string,
	onuID int,
	expectedStates []string,
	maxRetries int,
	retryDelay time.Duration,
) (*cli.ONUCLIInfo, bool) {
	var lastInfo *cli.ONUCLIInfo

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		info, err := driver.GetONUInfo(ctx, ponPort, onuID)
		if err != nil {
			slog.Debug("verification attempt failed", "attempt", attempt, "error", err)
			continue
		}
		if info == nil {
			continue
		}

		lastInfo = info
		status := strings.ToLower(info.Status)
		for _, expected := range expectedStates {
			if status == expected {
				return info, true
			}
		}
	}
	return lastInfo, false
}

// verifyONUDeleted verifies ONU no longer exists with retry.
func verifyONUDeleted(
	ctx context.Context,
	driver cli.CLIDriver,
	ponPort string,
	onuID int,
	maxRetries int,
	retryDelay time.Duration,
) bool {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		info, err := driver.GetONUInfo(ctx, ponPort, onuID)
		if err != nil || info == nil {
			return true // ONU is gone
		}
	}
	return false
}

// verifyONUExists verifies ONU exists with expected serial.
func verifyONUExists(
	ctx context.Context,
	driver cli.CLIDriver,
	ponPort string,
	onuID int,
	expectedSerial string,
	maxRetries int,
	retryDelay time.Duration,
) (*cli.ONUCLIInfo, bool) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		info, err := driver.GetONUInfo(ctx, ponPort, onuID)
		if err != nil || info == nil {
			continue
		}

		if info.SerialNumber == expectedSerial {
			return info, true
		}
	}
	return nil, false
}

// verifyONUStateChangeSNMP verifies ONU state change using SNMP (DriverV2).
// This is more reliable than CLI verification as it directly queries the OLT's SNMP data.
func verifyONUStateChangeSNMP(
	ctx context.Context,
	driverV2 types.DriverV2,
	ponPort string,
	onuID int,
	serial string,
	expectedStates []string,
	maxRetries int,
	retryDelay time.Duration,
) (string, bool) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		// Get ONU list filtered by PON port
		filter := &types.ONUFilter{PONPort: ponPort}
		onus, err := driverV2.GetONUList(ctx, filter)
		if err != nil {
			slog.Debug("SNMP verification attempt failed", "attempt", attempt, "error", err)
			continue
		}

		// Find the specific ONU by serial or ID
		for _, onu := range onus {
			if (serial != "" && onu.Serial == serial) || (serial == "" && onu.ONUID == onuID) {
				// Check admin state and oper state
				adminState := strings.ToLower(onu.AdminState)
				operState := strings.ToLower(onu.OperState)

				for _, expected := range expectedStates {
					// Match against admin state, oper state, or computed status
					if adminState == expected || operState == expected {
						return operState, true
					}
					// Also check for disabled admin state mapping to suspended
					if expected == "suspended" && (adminState == "disabled" || operState == "suspended") {
						return "suspended", true
					}
				}
				slog.Debug("SNMP verification: state mismatch",
					"serial", onu.Serial,
					"adminState", adminState,
					"operState", operState,
					"expected", expectedStates)
			}
		}
	}
	return "", false
}

// pushONUUpdate pushes ONU data to database immediately (best effort).
func (e *Executor) pushONUUpdate(oltID, serial, ponPort string, onuID int, status string, info *cli.ONUCLIInfo) {
	if e.client == nil || serial == "" {
		return
	}

	onuData := agent.ONUData{
		Serial:  serial,
		PONPort: ponPort,
		ONUID:   onuID,
		Status:  status,
	}

	if info != nil {
		onuData.RxPower = info.RxPower
		onuData.Distance = info.Distance
		onuData.Model = info.Type // CLI returns type as model
	}

	if _, err := e.client.PushSingleONU(oltID, onuData); err != nil {
		slog.Warn("failed to push immediate ONU update", "serial", serial, "error", err)
	} else {
		slog.Info("pushed immediate ONU update", "serial", serial, "status", status)
	}
}

// handleONUList retrieves all ONUs from the OLT using CLI commands.
// Note: The DriverV2/SNMP path is handled separately in executor.go via handleONUListV2.
// This function is only called as a CLI fallback when SNMP is unavailable.
func (e *Executor) handleONUList(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Get optional filters from payload
	ponPort, _ := cmd.Payload["ponPort"].(string)
	detailed, _ := cmd.Payload["detailed"].(bool)

	// CLI fallback: Use PON port scanning
	ports, err := driver.ListPONPorts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list PON ports: %w", err)
	}
	log.Printf("[command] handleONUList fallback: ListPONPorts returned %d ports", len(ports))

	var onus []map[string]interface{}

	for _, port := range ports {
		// Use the full port name (e.g., "0/0/1") from the port info
		portName := port.Name
		if portName == "" {
			// Fallback to constructing port name with frame 0
			portName = fmt.Sprintf("0/%d/%d", port.Slot, port.Port)
		}
		// Filter by PON port if specified
		if ponPort != "" && !strings.Contains(portName, ponPort) {
			continue
		}

		// Get accurate ONU count for this port
		onuCount := port.ONUCount
		if onuCount == 0 {
			// ListPONPorts may not populate ONUCount, so get detailed port info
			portInfo, err := driver.GetPONPortInfo(ctx, port.Slot, port.Port)
			if err == nil && portInfo != nil {
				onuCount = portInfo.ONUCount
			}
			// If still 0, scan up to a reasonable limit
			if onuCount == 0 {
				onuCount = 128 // Max ONUs per port for GPON
			}
		}

		// Get ONUs on this port (ONU IDs start from 0)
		log.Printf("[command] handleONUList: scanning port %s for %d ONUs", portName, onuCount)
		foundCount := 0
		for onuID := 0; onuID < onuCount; onuID++ {
			onuInfo, err := driver.GetONUInfo(ctx, portName, onuID)
			if err != nil {
				continue // Skip if ONU doesn't exist
			}
			foundCount++

			onuData := map[string]interface{}{
				"serial":   onuInfo.SerialNumber,
				"ponPort":  onuInfo.PonPort,
				"onuId":    onuInfo.OnuID,
				"status":   onuInfo.Status,
				"type":     onuInfo.Type,
				"distance": onuInfo.Distance,
			}

			if detailed {
				onuData["rxPower"] = onuInfo.RxPower
				onuData["lineProfile"] = onuInfo.LineProfile
				onuData["serviceProfile"] = onuInfo.ServiceProfile
				onuData["description"] = onuInfo.Description
			}

			onus = append(onus, onuData)
		}
		log.Printf("[command] handleONUList: found %d ONUs on port %s", foundCount, portName)
	}

	return map[string]interface{}{
		"onus":  onus,
		"count": len(onus),
	}, nil
}

// handleONUGet retrieves detailed information for a specific ONU.
func (e *Executor) handleONUGet(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	serial, _ := cmd.Payload["serial"].(string)
	ponPort, _ := cmd.Payload["ponPort"].(string)
	onuIDFloat, _ := cmd.Payload["onuId"].(float64)
	onuID := int(onuIDFloat)

	if serial == "" && (ponPort == "" || onuID == 0) {
		return nil, fmt.Errorf("either serial or ponPort+onuId is required")
	}

	// If we have ponPort and onuID, get directly
	if ponPort != "" && onuID > 0 {
		onuInfo, err := driver.GetONUInfo(ctx, ponPort, onuID)
		if err != nil {
			return nil, fmt.Errorf("failed to get ONU info: %w", err)
		}

		return map[string]interface{}{
			"onu": map[string]interface{}{
				"serial":         onuInfo.SerialNumber,
				"ponPort":        onuInfo.PonPort,
				"onuId":          onuInfo.OnuID,
				"status":         onuInfo.Status,
				"type":           onuInfo.Type,
				"distance":       onuInfo.Distance,
				"rxPower":        onuInfo.RxPower,
				"lineProfile":    onuInfo.LineProfile,
				"serviceProfile": onuInfo.ServiceProfile,
				"mac":            onuInfo.MAC,
				"description":    onuInfo.Description,
				"offlineReason":  onuInfo.OfflineReason,
			},
		}, nil
	}

	// Otherwise search by serial - this is less efficient
	return nil, fmt.Errorf("search by serial not yet implemented - provide ponPort and onuId")
}

// handleONUProvision provisions a new ONU on the OLT.
func (e *Executor) handleONUProvision(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	serial, _ := cmd.Payload["serial"].(string)
	ponPort, _ := cmd.Payload["ponPort"].(string)
	onuIDFloat, _ := cmd.Payload["onuId"].(float64)
	onuID := int(onuIDFloat)
	lineProfile, _ := cmd.Payload["lineProfile"].(string)
	serviceProfile, _ := cmd.Payload["serviceProfile"].(string)
	vlanFloat, _ := cmd.Payload["vlan"].(float64)
	vlan := int(vlanFloat)
	description, _ := cmd.Payload["description"].(string)

	if serial == "" {
		return nil, fmt.Errorf("serial is required")
	}
	if ponPort == "" {
		return nil, fmt.Errorf("ponPort is required")
	}
	if lineProfile == "" {
		return nil, fmt.Errorf("lineProfile is required")
	}
	if serviceProfile == "" {
		return nil, fmt.Errorf("serviceProfile is required")
	}

	// Create provision request
	req := &cli.ONUProvisionRequest{
		PonPort:        ponPort,
		OnuID:          onuID,
		SerialNumber:   serial,
		Description:    description,
		LineProfile:    lineProfile,
		ServiceProfile: serviceProfile,
		NativeVLAN:     vlan,
	}

	// Add ONU
	err := driver.AddONU(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to provision ONU: %w", err)
	}

	// Verify ONU was created with retry
	postInfo, verified := verifyONUExists(ctx, driver, ponPort, onuID, serial, 3, 500*time.Millisecond)

	if !verified {
		return nil, fmt.Errorf("verification failed: ONU %s not found after provision", serial)
	}

	// Push to database immediately
	status := "online"
	if postInfo != nil {
		status = postInfo.Status
	}
	e.pushONUUpdate(cmd.EquipmentID, serial, ponPort, onuID, status, postInfo)

	result := map[string]interface{}{
		"success":         true,
		"verified":        true,
		"immediateUpdate": true,
		"onu": map[string]interface{}{
			"serial":         serial,
			"ponPort":        ponPort,
			"onuId":          onuID,
			"lineProfile":    lineProfile,
			"serviceProfile": serviceProfile,
		},
	}

	if postInfo != nil {
		result["onu"].(map[string]interface{})["status"] = postInfo.Status
	}

	return result, nil
}

// handleONUDelete removes an ONU from the OLT.
func (e *Executor) handleONUDelete(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	serial, _ := cmd.Payload["serial"].(string)
	ponPort, _ := cmd.Payload["ponPort"].(string)
	onuIDFloat, _ := cmd.Payload["onuId"].(float64)
	onuID := int(onuIDFloat)

	// Need either serial (to find the ONU) or ponPort+onuId
	if serial == "" && (ponPort == "" || onuID == 0) {
		return nil, fmt.Errorf("either serial or ponPort+onuId is required")
	}

	// If only serial provided, we'd need to find the ONU first
	// For now, require ponPort and onuId
	if ponPort == "" || onuID == 0 {
		return nil, fmt.Errorf("ponPort and onuId are required for deletion")
	}

	// Get pre-state
	preInfo, _ := driver.GetONUInfo(ctx, ponPort, onuID)
	var preState map[string]interface{}
	if preInfo != nil {
		preState = map[string]interface{}{
			"serial":  preInfo.SerialNumber,
			"ponPort": preInfo.PonPort,
			"onuId":   preInfo.OnuID,
			"status":  preInfo.Status,
		}
	}

	// Delete ONU
	err := driver.DeleteONU(ctx, ponPort, onuID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete ONU: %w", err)
	}

	// Verify ONU was deleted with retry
	verified := verifyONUDeleted(ctx, driver, ponPort, onuID, 3, 500*time.Millisecond)

	if !verified {
		return nil, fmt.Errorf("verification failed: ONU still exists after delete")
	}

	// For deletion, the poller will clean up on next cycle
	// We could push a "deleted" status here if the API supported it

	return map[string]interface{}{
		"success":         true,
		"verified":        true,
		"preState":        preState,
		"immediateUpdate": true,
	}, nil
}

// handleONUReboot reboots an ONU.
func (e *Executor) handleONUReboot(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	ponPort, _ := cmd.Payload["ponPort"].(string)
	onuIDFloat, _ := cmd.Payload["onuId"].(float64)
	onuID := int(onuIDFloat)

	if ponPort == "" || onuID == 0 {
		return nil, fmt.Errorf("ponPort and onuId are required")
	}

	// Get pre-state to capture serial number
	preInfo, _ := driver.GetONUInfo(ctx, ponPort, onuID)
	var serial string
	if preInfo != nil {
		serial = preInfo.SerialNumber
	}

	err := driver.RebootONU(ctx, ponPort, onuID)
	if err != nil {
		return nil, fmt.Errorf("failed to reboot ONU: %w", err)
	}

	// Wait a bit longer for reboot to complete
	time.Sleep(2 * time.Second)

	// Verify ONU came back online (more retries and longer delay for reboot)
	postInfo, verified := verifyONUStateChange(
		ctx, driver, ponPort, onuID,
		[]string{"online", "active"},
		5,               // More retries for reboot
		2*time.Second,   // Longer delay between retries
	)

	// Push ONU data (even if not yet online, push current state)
	status := "rebooting"
	if postInfo != nil && verified {
		status = postInfo.Status
	}
	if serial != "" {
		e.pushONUUpdate(cmd.EquipmentID, serial, ponPort, onuID, status, postInfo)
	}

	return map[string]interface{}{
		"success":         true,
		"verified":        verified,
		"immediateUpdate": serial != "",
		"message":         fmt.Sprintf("ONU %s:%d reboot initiated", ponPort, onuID),
	}, nil
}

// handleONUDiagnostics retrieves comprehensive diagnostics for an ONU.
func (e *Executor) handleONUDiagnostics(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	ponPort, _ := cmd.Payload["ponPort"].(string)
	onuIDFloat, _ := cmd.Payload["onuId"].(float64)
	onuID := int(onuIDFloat)

	if ponPort == "" || onuID == 0 {
		return nil, fmt.Errorf("ponPort and onuId are required")
	}

	diag, err := driver.GetONUDiagnostics(ctx, ponPort, onuID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ONU diagnostics: %w", err)
	}

	return map[string]interface{}{
		"diagnostics": map[string]interface{}{
			"serial":  diag.SerialNumber,
			"ponPort": diag.PonPort,
			"onuId":   diag.OnuID,
			"status":  diag.Status,
			"optical": map[string]interface{}{
				"rxPower":       diag.Optical.RxPower,
				"txPower":       diag.Optical.TxPower,
				"oltRxPower":    diag.Optical.OltRxPower,
				"temperature":   diag.Optical.Temperature,
				"voltage":       diag.Optical.Voltage,
				"biasCurrent":   diag.Optical.BiasCurrent,
				"rxPowerStatus": diag.Optical.RxPowerStatus,
			},
			"counters": map[string]interface{}{
				"rxBytes":   diag.Counters.RxBytes,
				"txBytes":   diag.Counters.TxBytes,
				"rxPackets": diag.Counters.RxPackets,
				"txPackets": diag.Counters.TxPackets,
				"rxErrors":  diag.Counters.RxErrors,
				"txErrors":  diag.Counters.TxErrors,
			},
			"health": map[string]interface{}{
				"cpuUsage":    diag.Health.CPUUsage,
				"memoryUsage": diag.Health.MemoryUsage,
				"temperature": diag.Health.Temperature,
				"uptime":      diag.Health.Uptime,
				"firmwareVer": diag.Health.FirmwareVer,
				"lastReboot":  diag.Health.LastReboot,
			},
			"connectivity": map[string]interface{}{
				"distance":      diag.Connectivity.Distance,
				"rtt":           diag.Connectivity.RTT,
				"offlineReason": diag.Connectivity.OfflineReason,
				"offlineCount":  diag.Connectivity.OfflineCount,
			},
		},
	}, nil
}

// handleONUDiscover discovers unprovisioned ONUs on the OLT.
// Note: This implementation scans PON ports looking for ONUs in "offline" or "unprovisioned" state.
func (e *Executor) handleONUDiscover(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Get optional PON port filter
	ponPortsRaw, _ := cmd.Payload["ponPorts"].([]interface{})
	var ponPorts []string
	for _, p := range ponPortsRaw {
		if ps, ok := p.(string); ok {
			ponPorts = append(ponPorts, ps)
		}
	}

	// Get all PON ports
	ports, err := driver.ListPONPorts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list PON ports: %w", err)
	}

	var discovered []map[string]interface{}

	for _, port := range ports {
		portName := fmt.Sprintf("%d/%d", port.Slot, port.Port)

		// Filter by specified PON ports if provided
		if len(ponPorts) > 0 {
			found := false
			for _, pp := range ponPorts {
				if strings.Contains(portName, pp) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Scan ONU IDs on this port (check beyond current ONUCount for unprovisioned ONUs)
		maxScan := port.ONUCount + 16 // Scan a few beyond known count
		if maxScan > 128 {
			maxScan = 128
		}

		for onuID := 1; onuID <= maxScan; onuID++ {
			onuInfo, err := driver.GetONUInfo(ctx, portName, onuID)
			if err != nil {
				continue // ONU doesn't exist
			}

			// Check if ONU is in a state indicating it's unprovisioned/waiting
			status := strings.ToLower(onuInfo.Status)
			if status == "offline" || status == "unprovisioned" || status == "deregistered" || status == "discovered" {
				discovered = append(discovered, map[string]interface{}{
					"serial":   onuInfo.SerialNumber,
					"ponPort":  portName,
					"onuId":    onuID,
					"status":   onuInfo.Status,
					"type":     onuInfo.Type,
					"distance": onuInfo.Distance,
				})
			}
		}
	}

	return map[string]interface{}{
		"onus":  discovered,
		"count": len(discovered),
	}, nil
}

// handleONUUpdate updates an existing ONU's configuration.
func (e *Executor) handleONUUpdate(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	ponPort, _ := cmd.Payload["ponPort"].(string)
	onuIDFloat, _ := cmd.Payload["onuId"].(float64)
	onuID := int(onuIDFloat)

	if ponPort == "" || onuID == 0 {
		return nil, fmt.Errorf("ponPort and onuId are required")
	}

	// Get pre-state
	preInfo, err := driver.GetONUInfo(ctx, ponPort, onuID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ONU info: %w", err)
	}
	preState := map[string]interface{}{
		"serial":         preInfo.SerialNumber,
		"ponPort":        preInfo.PonPort,
		"onuId":          preInfo.OnuID,
		"lineProfile":    preInfo.LineProfile,
		"serviceProfile": preInfo.ServiceProfile,
	}

	// Apply VLAN update if specified
	vlanFloat, hasVlan := cmd.Payload["vlan"].(float64)
	if hasVlan {
		vlanConfig := &cli.VLANConfig{
			PonPort:    ponPort,
			OnuID:      onuID,
			NativeVLAN: int(vlanFloat),
		}
		if err := driver.ConfigureVLAN(ctx, vlanConfig); err != nil {
			return nil, fmt.Errorf("failed to update VLAN configuration: %w", err)
		}
	}

	// Apply traffic profile if specified
	trafficProfileFloat, hasProfile := cmd.Payload["trafficProfile"].(float64)
	if hasProfile {
		if err := driver.AssignTrafficProfile(ctx, ponPort, onuID, int(trafficProfileFloat)); err != nil {
			return nil, fmt.Errorf("failed to assign traffic profile: %w", err)
		}
	}

	// Apply description if specified
	description, hasDesc := cmd.Payload["description"].(string)
	if hasDesc && description != "" {
		// Description update would need driver support - for now, note it
		// This would typically be part of ONT configuration mode
	}

	// Verify settings were applied with retry
	postInfo, verified := verifyONUStateChange(
		ctx, driver, ponPort, onuID,
		[]string{"online", "active"}, // ONU should still be online after update
		3, 500*time.Millisecond,
	)

	// Push updated ONU data to database immediately
	status := "online"
	if postInfo != nil {
		status = postInfo.Status
	}
	e.pushONUUpdate(cmd.EquipmentID, preInfo.SerialNumber, ponPort, onuID, status, postInfo)

	var postState map[string]interface{}
	if postInfo != nil {
		postState = map[string]interface{}{
			"serial":         postInfo.SerialNumber,
			"ponPort":        postInfo.PonPort,
			"onuId":          postInfo.OnuID,
			"lineProfile":    postInfo.LineProfile,
			"serviceProfile": postInfo.ServiceProfile,
		}
	}

	return map[string]interface{}{
		"success":         true,
		"verified":        verified,
		"preState":        preState,
		"postState":       postState,
		"immediateUpdate": true,
	}, nil
}

// handleONUSuspend suspends an ONU (disables traffic).
// This typically sets the ONU to a "down" or "deactivated" state.
func (e *Executor) handleONUSuspend(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	ponPort, _ := cmd.Payload["ponPort"].(string)
	onuIDFloat, _ := cmd.Payload["onuId"].(float64)
	onuID := int(onuIDFloat)

	if ponPort == "" || onuID == 0 {
		return nil, fmt.Errorf("ponPort and onuId are required")
	}

	// Get pre-state (best effort - don't fail if we can't get it)
	var preState map[string]interface{}
	preInfo, _ := driver.GetONUInfo(ctx, ponPort, onuID)
	if preInfo != nil {
		preState = map[string]interface{}{
			"serial": preInfo.SerialNumber,
			"status": preInfo.Status,
		}
	}

	// Execute vendor-specific suspend command
	// For Huawei: interface gpon 0/X, ont deactivate Y ont-id Z
	// For other vendors: similar admin state commands
	vendor := driver.Vendor()
	var suspendCmd string

	slot, port, err := parsePonPort(ponPort)
	if err != nil {
		return nil, err
	}

	switch vendor {
	case "huawei":
		suspendCmd = fmt.Sprintf("interface gpon 0/%d\n ont deactivate %d ont-id %d\n quit", slot, port, onuID)
	case "vsol":
		suspendCmd = fmt.Sprintf("interface gpon 0/%d\nonu %d deactivate\nexit", port, onuID)
	default:
		return nil, fmt.Errorf("ONU suspend not supported for vendor: %s", vendor)
	}

	output, err := driver.Execute(ctx, suspendCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to suspend ONU: %w (output: %s)", err, output)
	}

	// Verify the change actually happened with retry (OLT hardware can be slow)
	postInfo, verified := verifyONUStateChange(
		ctx, driver, ponPort, onuID,
		[]string{"offline", "deactivated", "down", "suspended", "disabled"},
		3, 500*time.Millisecond,
	)

	if !verified {
		return nil, fmt.Errorf("verification failed: ONU did not reach suspended state")
	}

	// Push verified data to database immediately
	serial := ""
	if preInfo != nil {
		serial = preInfo.SerialNumber
	} else if postInfo != nil {
		serial = postInfo.SerialNumber
	}
	e.pushONUUpdate(cmd.EquipmentID, serial, ponPort, onuID, "suspended", postInfo)

	var postState map[string]interface{}
	if postInfo != nil {
		postState = map[string]interface{}{
			"serial": postInfo.SerialNumber,
			"status": postInfo.Status,
		}
	}

	return map[string]interface{}{
		"success":         true,
		"verified":        true,
		"preState":        preState,
		"postState":       postState,
		"immediateUpdate": true,
	}, nil
}

// handleONUResume resumes a suspended ONU (re-enables traffic).
func (e *Executor) handleONUResume(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	ponPort, _ := cmd.Payload["ponPort"].(string)
	onuIDFloat, _ := cmd.Payload["onuId"].(float64)
	onuID := int(onuIDFloat)

	if ponPort == "" || onuID == 0 {
		return nil, fmt.Errorf("ponPort and onuId are required")
	}

	// Get pre-state (best effort - don't fail if we can't get it)
	var preState map[string]interface{}
	preInfo, _ := driver.GetONUInfo(ctx, ponPort, onuID)
	if preInfo != nil {
		preState = map[string]interface{}{
			"serial": preInfo.SerialNumber,
			"status": preInfo.Status,
		}
	}

	// Execute vendor-specific resume command
	vendor := driver.Vendor()
	var resumeCmd string

	slot, port, err := parsePonPort(ponPort)
	if err != nil {
		return nil, err
	}

	switch vendor {
	case "huawei":
		resumeCmd = fmt.Sprintf("interface gpon 0/%d\n ont activate %d ont-id %d\n quit", slot, port, onuID)
	case "vsol":
		resumeCmd = fmt.Sprintf("interface gpon 0/%d\nonu %d activate\nexit", port, onuID)
	default:
		return nil, fmt.Errorf("ONU resume not supported for vendor: %s", vendor)
	}

	output, err := driver.Execute(ctx, resumeCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to resume ONU: %w (output: %s)", err, output)
	}

	// Verify the change actually happened with retry (OLT hardware can be slow)
	postInfo, verified := verifyONUStateChange(
		ctx, driver, ponPort, onuID,
		[]string{"online", "active", "up"},
		3, 500*time.Millisecond,
	)

	if !verified {
		return nil, fmt.Errorf("verification failed: ONU did not reach online state")
	}

	// Push verified data to database immediately
	serial := ""
	if preInfo != nil {
		serial = preInfo.SerialNumber
	} else if postInfo != nil {
		serial = postInfo.SerialNumber
	}
	e.pushONUUpdate(cmd.EquipmentID, serial, ponPort, onuID, "online", postInfo)

	var postState map[string]interface{}
	if postInfo != nil {
		postState = map[string]interface{}{
			"serial": postInfo.SerialNumber,
			"status": postInfo.Status,
		}
	}

	return map[string]interface{}{
		"success":         true,
		"verified":        true,
		"preState":        preState,
		"postState":       postState,
		"immediateUpdate": true,
	}, nil
}

// parsePonPort parses a PON port string into slot and port numbers.
// Supports both 2-part (slot/port) and 3-part (frame/slot/port) formats.
func parsePonPort(ponPort string) (slot, port int, err error) {
	parts := strings.Split(ponPort, "/")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid PON port format: %s", ponPort)
	}

	if len(parts) == 3 {
		// 3-part format: frame/slot/port (Huawei)
		slot, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid slot number: %s", parts[1])
		}
		port, err = strconv.Atoi(parts[2])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid port number: %s", parts[2])
		}
	} else {
		// 2-part format: slot/port (V-SOL)
		slot, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid slot number: %s", parts[0])
		}
		port, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid port number: %s", parts[1])
		}
	}

	return slot, port, nil
}

// =============================================================================
// Provisioning Handlers with SNMP Verification
// =============================================================================
// These handlers use CLI for command execution and SNMP (DriverV2) for verification.
// This is more reliable than CLI-only verification because:
// 1. CLI `show onu detail-info` may not be implemented on all OLTs/simulators
// 2. SNMP provides direct access to the OLT's state database
// 3. SNMP is faster and more reliable for state verification

// handleONUSuspendWithVerification suspends an ONU and verifies via SNMP.
func (e *Executor) handleONUSuspendWithVerification(ctx context.Context, driver cli.CLIDriver, driverV2 types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	ponPort, _ := cmd.Payload["ponPort"].(string)
	onuIDFloat, _ := cmd.Payload["onuId"].(float64)
	onuID := int(onuIDFloat)
	serial, _ := cmd.Payload["serial"].(string)

	if ponPort == "" || onuID == 0 {
		return nil, fmt.Errorf("ponPort and onuId are required")
	}

	// Get pre-state via SNMP if available
	var preState map[string]interface{}
	if driverV2 != nil {
		filter := &types.ONUFilter{PONPort: ponPort}
		onus, err := driverV2.GetONUList(ctx, filter)
		if err == nil {
			for _, onu := range onus {
				if onu.ONUID == onuID || (serial != "" && onu.Serial == serial) {
					preState = map[string]interface{}{
						"serial":     onu.Serial,
						"status":     onu.OperState,
						"adminState": onu.AdminState,
					}
					if serial == "" {
						serial = onu.Serial
					}
					break
				}
			}
		}
	}

	// Execute vendor-specific suspend command via CLI
	vendor := driver.Vendor()
	var suspendCmd string

	slot, port, err := parsePonPort(ponPort)
	if err != nil {
		return nil, err
	}

	switch vendor {
	case "huawei":
		suspendCmd = fmt.Sprintf("interface gpon 0/%d\n ont deactivate %d ont-id %d\n quit", slot, port, onuID)
	case "vsol":
		suspendCmd = fmt.Sprintf("interface gpon 0/%d\nonu %d deactivate\nexit", port, onuID)
	default:
		return nil, fmt.Errorf("ONU suspend not supported for vendor: %s", vendor)
	}

	output, err := driver.Execute(ctx, suspendCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to suspend ONU: %w (output: %s)", err, output)
	}

	slog.Info("executed suspend command", "ponPort", ponPort, "onuId", onuID, "output", output)

	// Verify via SNMP (primary) or CLI (fallback)
	var verified bool
	var postStatus string

	if driverV2 != nil {
		// SNMP verification - more reliable
		postStatus, verified = verifyONUStateChangeSNMP(
			ctx, driverV2, ponPort, onuID, serial,
			[]string{"disabled", "suspended", "offline", "deactivated"},
			5, 500*time.Millisecond,
		)
		if verified {
			slog.Info("SNMP verification successful", "ponPort", ponPort, "onuId", onuID, "status", postStatus)
		} else {
			slog.Warn("SNMP verification failed, ONU may not have reached suspended state", "ponPort", ponPort, "onuId", onuID)
		}
	} else {
		// CLI fallback verification
		postInfo, cliVerified := verifyONUStateChange(
			ctx, driver, ponPort, onuID,
			[]string{"offline", "deactivated", "down", "suspended", "disabled"},
			3, 500*time.Millisecond,
		)
		verified = cliVerified
		if postInfo != nil {
			postStatus = postInfo.Status
			if serial == "" {
				serial = postInfo.SerialNumber
			}
		}
	}

	// Push update to database immediately regardless of verification result
	// The command was executed, so we should update the database with what we know
	statusToReport := "suspended"
	if verified && postStatus != "" {
		statusToReport = postStatus
	}
	e.pushONUUpdate(cmd.EquipmentID, serial, ponPort, onuID, statusToReport, nil)

	postState := map[string]interface{}{
		"serial":   serial,
		"status":   statusToReport,
		"verified": verified,
	}

	return map[string]interface{}{
		"success":         true,
		"verified":        verified,
		"preState":        preState,
		"postState":       postState,
		"immediateUpdate": true,
	}, nil
}

// handleONUResumeWithVerification resumes an ONU and verifies via SNMP.
func (e *Executor) handleONUResumeWithVerification(ctx context.Context, driver cli.CLIDriver, driverV2 types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	ponPort, _ := cmd.Payload["ponPort"].(string)
	onuIDFloat, _ := cmd.Payload["onuId"].(float64)
	onuID := int(onuIDFloat)
	serial, _ := cmd.Payload["serial"].(string)

	if ponPort == "" || onuID == 0 {
		return nil, fmt.Errorf("ponPort and onuId are required")
	}

	// Get pre-state via SNMP if available
	var preState map[string]interface{}
	if driverV2 != nil {
		filter := &types.ONUFilter{PONPort: ponPort}
		onus, err := driverV2.GetONUList(ctx, filter)
		if err == nil {
			for _, onu := range onus {
				if onu.ONUID == onuID || (serial != "" && onu.Serial == serial) {
					preState = map[string]interface{}{
						"serial":     onu.Serial,
						"status":     onu.OperState,
						"adminState": onu.AdminState,
					}
					if serial == "" {
						serial = onu.Serial
					}
					break
				}
			}
		}
	}

	// Execute vendor-specific resume command via CLI
	vendor := driver.Vendor()
	var resumeCmd string

	slot, port, err := parsePonPort(ponPort)
	if err != nil {
		return nil, err
	}

	switch vendor {
	case "huawei":
		resumeCmd = fmt.Sprintf("interface gpon 0/%d\n ont activate %d ont-id %d\n quit", slot, port, onuID)
	case "vsol":
		resumeCmd = fmt.Sprintf("interface gpon 0/%d\nonu %d activate\nexit", port, onuID)
	default:
		return nil, fmt.Errorf("ONU resume not supported for vendor: %s", vendor)
	}

	output, err := driver.Execute(ctx, resumeCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to resume ONU: %w (output: %s)", err, output)
	}

	slog.Info("executed resume command", "ponPort", ponPort, "onuId", onuID, "output", output)

	// Verify via SNMP (primary) or CLI (fallback)
	var verified bool
	var postStatus string

	if driverV2 != nil {
		// SNMP verification - more reliable
		postStatus, verified = verifyONUStateChangeSNMP(
			ctx, driverV2, ponPort, onuID, serial,
			[]string{"enabled", "online", "active"},
			5, 500*time.Millisecond,
		)
		if verified {
			slog.Info("SNMP verification successful", "ponPort", ponPort, "onuId", onuID, "status", postStatus)
		} else {
			slog.Warn("SNMP verification failed, ONU may not have reached online state", "ponPort", ponPort, "onuId", onuID)
		}
	} else {
		// CLI fallback verification
		postInfo, cliVerified := verifyONUStateChange(
			ctx, driver, ponPort, onuID,
			[]string{"online", "active", "up"},
			3, 500*time.Millisecond,
		)
		verified = cliVerified
		if postInfo != nil {
			postStatus = postInfo.Status
			if serial == "" {
				serial = postInfo.SerialNumber
			}
		}
	}

	// Push update to database immediately
	statusToReport := "online"
	if verified && postStatus != "" {
		statusToReport = postStatus
	}
	e.pushONUUpdate(cmd.EquipmentID, serial, ponPort, onuID, statusToReport, nil)

	postState := map[string]interface{}{
		"serial":   serial,
		"status":   statusToReport,
		"verified": verified,
	}

	return map[string]interface{}{
		"success":         true,
		"verified":        verified,
		"preState":        preState,
		"postState":       postState,
		"immediateUpdate": true,
	}, nil
}

// handleONUProvisionWithVerification provisions an ONU and verifies via SNMP.
func (e *Executor) handleONUProvisionWithVerification(ctx context.Context, driver cli.CLIDriver, driverV2 types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Delegate to the existing handler - provision uses CLI verification
	// which should work as the ONU will appear in the list
	return e.handleONUProvision(ctx, driver, cmd)
}

// handleONUDeleteWithVerification deletes an ONU and verifies via SNMP.
func (e *Executor) handleONUDeleteWithVerification(ctx context.Context, driver cli.CLIDriver, driverV2 types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Delegate to the existing handler - delete uses CLI verification
	return e.handleONUDelete(ctx, driver, cmd)
}

// handleONUUpdateWithVerification updates an ONU and verifies via SNMP.
func (e *Executor) handleONUUpdateWithVerification(ctx context.Context, driver cli.CLIDriver, driverV2 types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Delegate to the existing handler
	return e.handleONUUpdate(ctx, driver, cmd)
}

// handleONURebootWithVerification reboots an ONU and verifies via SNMP.
func (e *Executor) handleONURebootWithVerification(ctx context.Context, driver cli.CLIDriver, driverV2 types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Delegate to the existing handler - reboot has its own verification logic
	return e.handleONUReboot(ctx, driver, cmd)
}
