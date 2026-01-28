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

// handleONUDiscoverV2 discovers unprovisioned ONUs using DriverV2.
// Uses vendor-specific DiscoverONUs (e.g., "show onu auto-find" on V-SOL).
// Unprovisioned ONUs don't have an ONU ID and are only visible via auto-find.
func (e *Executor) handleONUDiscoverV2(ctx context.Context, driver types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	ponPorts := extractPonPortsFilter(cmd.Payload)

	discoveries, err := driver.DiscoverONUs(ctx, ponPorts)
	if err != nil {
		return nil, fmt.Errorf("failed to discover ONUs: %w", err)
	}

	onus := make([]map[string]interface{}, 0, len(discoveries))
	for _, d := range discoveries {
		onus = append(onus, discoveryToMap(d))
	}

	return map[string]interface{}{
		"onus":  onus,
		"count": len(onus),
	}, nil
}

// extractPonPortsFilter extracts PON port filter from command payload.
func extractPonPortsFilter(payload map[string]interface{}) []string {
	ponPortsRaw, _ := payload["ponPorts"].([]interface{})
	var ponPorts []string
	for _, p := range ponPortsRaw {
		if ps, ok := p.(string); ok {
			ponPorts = append(ponPorts, ps)
		}
	}
	return ponPorts
}

// discoveryToMap converts an ONUDiscovery to a map for JSON response.
func discoveryToMap(d types.ONUDiscovery) map[string]interface{} {
	m := map[string]interface{}{
		"serial":       d.Serial,
		"ponPort":      d.PONPort,
		"discoveredAt": d.DiscoveredAt,
	}
	if d.MAC != "" {
		m["mac"] = d.MAC
	}
	if d.Model != "" {
		m["model"] = d.Model
	}
	if d.State != "" {
		m["state"] = d.State
	}
	if d.DistanceM > 0 {
		m["distance"] = d.DistanceM
	}
	if d.RxPowerDBm != 0 {
		m["rxPower"] = d.RxPowerDBm
	}
	return m
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

// =============================================================================
// Bulk Provisioning Handler
// =============================================================================

// handleONUBulkProvision provisions multiple ONUs using the DriverV2 BulkProvision method.
// This is significantly more efficient than individual provisioning as it reuses a single
// SSH/CLI session for all operations.
func (e *Executor) handleONUBulkProvision(ctx context.Context, driverV2 types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Extract operations from payload
	operationsRaw, ok := cmd.Payload["operations"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid operations payload: expected array")
	}

	if len(operationsRaw) == 0 {
		return nil, fmt.Errorf("no operations provided")
	}

	slog.Info("starting bulk provision", "count", len(operationsRaw), "equipment", cmd.EquipmentID)

	// Convert payload to BulkProvisionOp slice
	operations := make([]types.BulkProvisionOp, 0, len(operationsRaw))
	for i, opRaw := range operationsRaw {
		opMap, ok := opRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid operation at index %d: expected object", i)
		}

		serial, _ := opMap["serial"].(string)
		if serial == "" {
			return nil, fmt.Errorf("operation at index %d missing serial", i)
		}

		ponPort, _ := opMap["pon_port"].(string)
		onuIDFloat, _ := opMap["onu_id"].(float64)
		onuID := int(onuIDFloat)

		// Build profile from payload fields
		lineProfile, _ := opMap["line_profile"].(string)
		serviceProfile, _ := opMap["service_profile"].(string)
		vlanFloat, _ := opMap["vlan"].(float64)
		vlan := int(vlanFloat)
		bandwidthUpFloat, _ := opMap["bandwidth_up_kbps"].(float64)
		bandwidthUp := int(bandwidthUpFloat)
		bandwidthDownFloat, _ := opMap["bandwidth_down_kbps"].(float64)
		bandwidthDown := int(bandwidthDownFloat)

		// Create the profile if any profile fields are set
		var profile *types.ONUProfile
		if lineProfile != "" || serviceProfile != "" || vlan > 0 || bandwidthUp > 0 || bandwidthDown > 0 {
			profile = &types.ONUProfile{
				LineProfile:    lineProfile,
				ServiceProfile: serviceProfile,
				VLAN:           vlan,
				BandwidthUp:    bandwidthUp,
				BandwidthDown:  bandwidthDown,
			}
		}

		operations = append(operations, types.BulkProvisionOp{
			Serial:  serial,
			PONPort: ponPort,
			ONUID:   onuID,
			Profile: profile,
		})
	}

	// Call driver BulkProvision
	result, err := driverV2.BulkProvision(ctx, operations)
	if err != nil {
		return nil, fmt.Errorf("bulk provision failed: %w", err)
	}

	slog.Info("bulk provision completed",
		"succeeded", result.Succeeded,
		"failed", result.Failed,
		"total", len(operations))

	// Convert results to response format
	results := make([]map[string]interface{}, 0, len(result.Results))
	for _, r := range result.Results {
		resultMap := map[string]interface{}{
			"serial":  r.Serial,
			"success": r.Success,
		}
		if r.Error != "" {
			resultMap["error"] = r.Error
		}
		if r.ErrorCode != "" {
			resultMap["error_code"] = r.ErrorCode
		}
		if r.ONUID > 0 {
			resultMap["onu_id"] = r.ONUID
		}
		if r.PONPort != "" {
			resultMap["pon_port"] = r.PONPort
		}
		results = append(results, resultMap)
	}

	// Push individual ONU updates for successful provisions
	for _, r := range result.Results {
		if r.Success {
			e.pushONUUpdate(cmd.EquipmentID, r.Serial, r.PONPort, r.ONUID, "online", nil)
		}
	}

	response := map[string]interface{}{
		"total":           len(operations),
		"succeeded":       result.Succeeded,
		"failed":          result.Failed,
		"results":         results,
		"immediateUpdate": true,
	}

	// Return error if all operations failed
	if result.Succeeded == 0 && result.Failed > 0 {
		return response, fmt.Errorf("all %d provisions failed", result.Failed)
	}

	return response, nil
}

// handleONUBulkProvisionWithVerification provisions multiple ONUs and verifies via SNMP.
// This handler uses the DriverV2 BulkProvision method which reuses a single CLI session.
// Note: BulkProvision requires a CLI-based southbound driver, not SNMP.
func (e *Executor) handleONUBulkProvisionWithVerification(ctx context.Context, driver cli.CLIDriver, driverV2 types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// For bulk provisioning, we need to use the southbound driver's BulkProvision method
	// which requires CLI protocol. The passed driverV2 might be SNMP-based (for reads),
	// so we need to check if it supports CLI operations.

	// First try to use the existing driverV2 for bulk provision
	result, err := e.handleONUBulkProvision(ctx, driverV2, cmd)
	if err != nil {
		// If bulk provision failed due to CLI executor, fall back to sequential provisioning
		// using the nano-agent's CLI driver
		if driverV2 == nil || strings.Contains(err.Error(), "CLI executor not available") {
			slog.Info("bulk provision via southbound driver failed, using sequential CLI provisioning", "error", err)
			return e.handleONUBulkProvisionSequential(ctx, driver, cmd)
		}
		return nil, err
	}

	// Verify provisioned ONUs via SNMP if available
	if driverV2 != nil {
		// Get the list of ONUs after provisioning
		onus, err := driverV2.GetONUList(ctx, nil)
		if err != nil {
			slog.Warn("failed to get ONU list for verification", "error", err)
			// Don't fail the operation - provisioning itself succeeded
			return result, nil
		}

		// Create a map for quick lookup
		onuBySerial := make(map[string]types.ONUInfo)
		for _, onu := range onus {
			onuBySerial[onu.Serial] = onu
		}

		// Update results with verification status
		if results, ok := result["results"].([]map[string]interface{}); ok {
			verifiedCount := 0
			for _, r := range results {
				if success, _ := r["success"].(bool); success {
					serial, _ := r["serial"].(string)
					if onu, found := onuBySerial[serial]; found {
						r["verified"] = true
						r["admin_state"] = onu.AdminState
						r["oper_state"] = onu.OperState
						verifiedCount++
					} else {
						r["verified"] = false
					}
				}
			}
			result["verified_count"] = verifiedCount
			slog.Info("bulk provision verification completed",
				"verified", verifiedCount,
				"succeeded", result["succeeded"])
		}
	}

	return result, nil
}

// ONUBasicInfo holds minimal ONU information for duplicate detection.
type ONUBasicInfo struct {
	ID     int
	Serial string
}

// handleONUBulkProvisionSequential provisions ONUs sequentially using the nano-agent's CLI driver.
// This is a fallback when the southbound driver's BulkProvision is not available.
// It's slower than true bulk provisioning but still reuses the same SSH session.
// It also detects duplicate serial numbers and skips already-provisioned ONUs.
func (e *Executor) handleONUBulkProvisionSequential(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Extract operations from payload
	operationsRaw, ok := cmd.Payload["operations"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid operations payload: expected array")
	}

	if len(operationsRaw) == 0 {
		return nil, fmt.Errorf("no operations provided")
	}

	slog.Info("starting sequential bulk provision", "count", len(operationsRaw), "equipment", cmd.EquipmentID)

	results := make([]map[string]interface{}, 0, len(operationsRaw))
	succeeded := 0
	failed := 0
	skipped := 0

	// Query existing ONUs to find used IDs and existing serials per port
	usedOnuIDs := make(map[string]map[int]bool)       // port -> set of used ONU IDs
	existingSerials := make(map[string]ONUBasicInfo)  // serial -> ONU info (for duplicate detection)

	// Collect unique ports from operations
	uniquePorts := make(map[string]bool)
	for _, opRaw := range operationsRaw {
		if opMap, ok := opRaw.(map[string]interface{}); ok {
			port, _ := opMap["pon_port"].(string)
			if port == "" {
				port = "0/1" // Default port
			}
			uniquePorts[port] = true
		}
	}

	// Try to list existing ONUs with serials for duplicate detection
	// First try ListONUsWithSerial which is more efficient (single command)
	type onuWithSerial interface {
		ListONUsWithSerial(ctx context.Context, ponPort string) ([]struct{ ID int; Serial string }, error)
	}

	for port := range uniquePorts {
		usedOnuIDs[port] = make(map[int]bool)

		// Try different methods to get ONU list with serials
		var ids []int
		var serials map[int]string = make(map[int]string)

		// Method 1: Try ListONUs first to get IDs
		if onuLister, ok := driver.(interface {
			ListONUs(ctx context.Context, ponPort string) ([]int, error)
		}); ok {
			foundIDs, err := onuLister.ListONUs(ctx, port)
			if err != nil {
				slog.Warn("failed to list ONUs on port", "port", port, "error", err)
				continue
			}
			ids = foundIDs
			for _, id := range ids {
				usedOnuIDs[port][id] = true
			}
		}

		// Method 2: Try to get serials via GetONUInfoAll (single command for all ONUs)
		if infoAllGetter, ok := driver.(interface {
			GetONUInfoAll(ctx context.Context, ponPort string) (map[int]string, error)
		}); ok {
			slog.Info("driver supports GetONUInfoAll, using efficient bulk serial fetch")
			serialMap, err := infoAllGetter.GetONUInfoAll(ctx, port)
			if err == nil {
				serials = serialMap
				// Also update IDs in case ListONUs missed some
				for id := range serialMap {
					usedOnuIDs[port][id] = true
				}
			} else {
				slog.Warn("GetONUInfoAll failed", "port", port, "error", err)
			}
		}

		// Add serials to existingSerials map
		for id, serial := range serials {
			if serial != "" {
				upperSerial := strings.ToUpper(serial)
				existingSerials[upperSerial] = ONUBasicInfo{
					ID:     id,
					Serial: serial,
				}
				slog.Debug("found existing ONU", "port", port, "id", id, "serial", upperSerial)
			}
		}

		// Log first few serials for debugging
		var sampleSerials []string
		for serial := range existingSerials {
			sampleSerials = append(sampleSerials, serial)
			if len(sampleSerials) >= 5 {
				break
			}
		}
		slog.Info("found existing ONUs", "port", port, "count", len(usedOnuIDs[port]), "serialsFound", len(existingSerials), "sampleSerials", sampleSerials)
	}

	if len(usedOnuIDs) == 0 {
		slog.Warn("driver does not support ListONUs, cannot auto-detect used ONU IDs or duplicates", "driverType", fmt.Sprintf("%T", driver))
	}

	// Track next available ONU ID per port for auto-assignment
	nextOnuID := make(map[string]int)

	// Helper to find next available ONU ID for a port
	findNextAvailableID := func(port string) int {
		if _, exists := nextOnuID[port]; !exists {
			nextOnuID[port] = 1
		}
		used := usedOnuIDs[port]
		for id := nextOnuID[port]; id <= 128; id++ {
			if used == nil || !used[id] {
				nextOnuID[port] = id + 1
				return id
			}
		}
		return 0 // No available ID
	}

	for i, opRaw := range operationsRaw {
		opMap, ok := opRaw.(map[string]interface{})
		if !ok {
			results = append(results, map[string]interface{}{
				"serial":  fmt.Sprintf("operation_%d", i),
				"success": false,
				"error":   "invalid operation format",
			})
			failed++
			continue
		}

		serial, _ := opMap["serial"].(string)
		if serial == "" {
			results = append(results, map[string]interface{}{
				"serial":  fmt.Sprintf("operation_%d", i),
				"success": false,
				"error":   "missing serial",
			})
			failed++
			continue
		}

		// Check if this serial already exists (duplicate detection)
		serialUpper := strings.ToUpper(serial)
		slog.Info("checking for duplicate", "serial", serialUpper, "existingSerialsCount", len(existingSerials))
		if existingONU, exists := existingSerials[serialUpper]; exists {
			results = append(results, map[string]interface{}{
				"serial":       serial,
				"success":      false,
				"skipped":      true,
				"error":        "ONU already exists",
				"error_code":   "ALREADY_EXISTS",
				"existing_id":  existingONU.ID,
			})
			// Count as failed - user wanted to provision but couldn't
			failed++
			slog.Info("failed duplicate ONU", "serial", serial, "existingId", existingONU.ID)
			continue
		} else {
			slog.Info("serial not found in existing", "serial", serialUpper)
		}

		ponPort, _ := opMap["pon_port"].(string)
		if ponPort == "" {
			ponPort = "0/1" // Default PON port
		}

		onuIDFloat, _ := opMap["onu_id"].(float64)
		onuID := int(onuIDFloat)

		// Auto-assign ONU ID if not provided or invalid (0)
		if onuID <= 0 {
			onuID = findNextAvailableID(ponPort)
			if onuID == 0 {
				results = append(results, map[string]interface{}{
					"serial":  serial,
					"success": false,
					"error":   "no available ONU ID on port " + ponPort,
				})
				failed++
				continue
			}
		}

		// Mark this ID as used for subsequent operations
		if usedOnuIDs[ponPort] == nil {
			usedOnuIDs[ponPort] = make(map[int]bool)
		}
		usedOnuIDs[ponPort][onuID] = true

		// Also mark the serial as existing to prevent duplicates within the same batch
		existingSerials[serialUpper] = ONUBasicInfo{ID: onuID, Serial: serial}

		lineProfile, _ := opMap["line_profile"].(string)
		serviceProfile, _ := opMap["service_profile"].(string)
		vlanFloat, _ := opMap["vlan"].(float64)
		vlan := int(vlanFloat)

		// Create provision request
		req := &cli.ONUProvisionRequest{
			PonPort:        ponPort,
			OnuID:          onuID,
			SerialNumber:   serial,
			LineProfile:    lineProfile,
			ServiceProfile: serviceProfile,
			NativeVLAN:     vlan,
		}

		// Provision the ONU
		err := driver.AddONU(ctx, req)
		resultMap := map[string]interface{}{
			"serial":   serial,
			"pon_port": ponPort,
			"onu_id":   onuID,
		}

		if err != nil {
			resultMap["success"] = false
			resultMap["error"] = err.Error()
			failed++
			slog.Warn("failed to provision ONU", "serial", serial, "error", err)
		} else {
			resultMap["success"] = true
			succeeded++
			// Push immediate update
			e.pushONUUpdate(cmd.EquipmentID, serial, ponPort, onuID, "online", nil)
			slog.Info("provisioned ONU", "serial", serial, "ponPort", ponPort, "onuId", onuID)
		}

		results = append(results, resultMap)
	}

	slog.Info("sequential bulk provision completed", "succeeded", succeeded, "failed", failed, "skipped", skipped)

	result := map[string]interface{}{
		"total":           len(operationsRaw),
		"succeeded":       succeeded,
		"failed":          failed,
		"skipped":         skipped, // Keep for informational purposes
		"results":         results,
		"immediateUpdate": true,
		"method":          "sequential", // Indicate fallback was used
	}

	// Return error if any operations failed (so history shows correct status)
	// The result data is still available for the UI to display details
	if failed > 0 {
		return result, fmt.Errorf("%d of %d provisions failed", failed, len(operationsRaw))
	}

	return result, nil
}
