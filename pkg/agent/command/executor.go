// Package command provides command execution capabilities for the nano-agent.
// It processes commands from the control plane command queue and executes them
// via the southbound CLI drivers against OLT devices.
package command

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
	southbound "github.com/nanoncore/nano-southbound"
	"github.com/nanoncore/nano-southbound/types"
)

// Executor processes commands from the control plane and executes them on OLT devices.
type Executor struct {
	client        *agent.Client
	driverFactory func(config cli.CLIConfig) (cli.CLIDriver, error)
	oltConfigs    map[string]agent.OLTConfig // equipmentID -> OLTConfig
}

// NewExecutor creates a new command executor.
func NewExecutor(client *agent.Client, driverFactory func(config cli.CLIConfig) (cli.CLIDriver, error)) *Executor {
	return &Executor{
		client:        client,
		driverFactory: driverFactory,
		oltConfigs:    make(map[string]agent.OLTConfig),
	}
}

// UpdateOLTConfigs updates the cached OLT configurations.
func (e *Executor) UpdateOLTConfigs(olts []agent.OLTConfig) {
	e.oltConfigs = make(map[string]agent.OLTConfig)
	for _, olt := range olts {
		e.oltConfigs[olt.ID] = olt
	}
}

// ProcessCommands executes all pending commands sequentially.
// Each command is acknowledged before execution and results are pushed after completion.
func (e *Executor) ProcessCommands(ctx context.Context, commands []agent.PendingCommand) error {
	for _, cmd := range commands {
		if err := e.executeCommand(ctx, cmd); err != nil {
			log.Printf("[command] Error executing command %s: %v", cmd.ID, err)
			// Continue with other commands even if one fails
		}
	}
	return nil
}

// executeCommand processes a single command through the full lifecycle:
// 1. Acknowledge the command (marks as in_progress)
// 2. Execute the command via the appropriate driver
// 3. Push the result back to the control plane
func (e *Executor) executeCommand(ctx context.Context, cmd agent.PendingCommand) error {
	startTime := time.Now()

	// 1. Acknowledge the command
	_, err := e.client.AckCommand(cmd.ID)
	if err != nil {
		return fmt.Errorf("failed to acknowledge command: %w", err)
	}
	log.Printf("[command] Acknowledged command %s (type: %s)", cmd.ID, cmd.Type)

	// 2. Get OLT configuration
	oltConfig, ok := e.oltConfigs[cmd.EquipmentID]
	if !ok {
		return e.pushError(cmd.ID, startTime, fmt.Errorf("OLT configuration not found for equipment %s", cmd.EquipmentID))
	}

	// 3. For read operations (onu_list, port_list, olt_status), use southbound driver (DriverV2) for efficient SNMP-based operations
	var result map[string]interface{}
	if cmd.Type == "onu_list" || cmd.Type == "port_list" || cmd.Type == "olt_status" {
		sbDriver, driverV2, err := e.createSouthboundDriver(ctx, oltConfig)
		if err != nil {
			// Fall back to CLI driver
			log.Printf("[command] Southbound driver unavailable, using CLI fallback: %v", err)
		} else {
			switch cmd.Type {
			case "onu_list":
				result, err = e.handleONUListV2(ctx, driverV2, cmd)
			case "port_list":
				result, err = e.handlePortListV2(ctx, driverV2, cmd)
			case "olt_status":
				result, err = e.handleOLTStatusV2(ctx, driverV2, cmd)
			}
			_ = sbDriver.Disconnect(ctx)
			if err != nil {
				// DriverV2 operation failed, fall back to CLI
				log.Printf("[command] DriverV2 operation failed, using CLI fallback: %v", err)
			} else {
				goto pushResult
			}
		}
	}

	// 4. For provisioning commands, use CLI for execution + SNMP for verification
	if isProvisioningCommand(cmd.Type) {
		// Create CLI driver for command execution
		driver, err := e.createDriver(oltConfig)
		if err != nil {
			return e.pushError(cmd.ID, startTime, fmt.Errorf("failed to create driver: %w", err))
		}

		if err := driver.Connect(ctx); err != nil {
			return e.pushError(cmd.ID, startTime, fmt.Errorf("failed to connect to OLT: %w", err))
		}
		defer driver.Close()

		// Create DriverV2 for SNMP-based verification (best effort)
		var driverV2 types.DriverV2
		sbDriver, dv2, err := e.createSouthboundDriver(ctx, oltConfig)
		if err != nil {
			log.Printf("[command] SNMP driver unavailable for verification, will use CLI only: %v", err)
		} else {
			driverV2 = dv2
			defer sbDriver.Disconnect(ctx)
		}

		// Execute provisioning command with SNMP verification capability
		result, err = e.dispatchProvisioning(ctx, driver, driverV2, cmd)
		if err != nil {
			// For bulk operations, we may have partial results even on error
			// Push the result with the error so the UI can show details
			if result != nil {
				return e.pushErrorWithResult(cmd.ID, startTime, err, result)
			}
			return e.pushError(cmd.ID, startTime, err)
		}
		goto pushResult
	}

	// Create CLI driver and connect for other commands (or onu_list fallback)
	{
		driver, err := e.createDriver(oltConfig)
		if err != nil {
			return e.pushError(cmd.ID, startTime, fmt.Errorf("failed to create driver: %w", err))
		}

		if err := driver.Connect(ctx); err != nil {
			return e.pushError(cmd.ID, startTime, fmt.Errorf("failed to connect to OLT: %w", err))
		}
		defer driver.Close()

		// 5. Execute the command based on type
		result, err = e.dispatch(ctx, driver, cmd)
		if err != nil {
			return e.pushError(cmd.ID, startTime, err)
		}
	}

pushResult:
	duration := time.Since(startTime)

	// 5. Push result
	resultReq := &agent.CommandResultRequest{
		Success:    err == nil,
		Result:     result,
		DurationMs: duration.Milliseconds(),
	}
	if err != nil {
		resultReq.Error = err.Error()
	}

	_, pushErr := e.client.PushCommandResult(cmd.ID, resultReq)
	if pushErr != nil {
		log.Printf("[command] Failed to push result for command %s: %v", cmd.ID, pushErr)
		return pushErr
	}

	if err != nil {
		log.Printf("[command] Command %s failed: %v (duration: %v)", cmd.ID, err, duration)
	} else {
		log.Printf("[command] Command %s completed successfully (duration: %v)", cmd.ID, duration)
	}

	return err
}

// pushError is a helper to push an error result for a command.
func (e *Executor) pushError(commandID string, startTime time.Time, err error) error {
	duration := time.Since(startTime)
	resultReq := &agent.CommandResultRequest{
		Success:    false,
		Error:      err.Error(),
		DurationMs: duration.Milliseconds(),
	}
	_, pushErr := e.client.PushCommandResult(commandID, resultReq)
	if pushErr != nil {
		log.Printf("[command] Failed to push error result for command %s: %v", commandID, pushErr)
	}
	return err
}

// pushErrorWithResult is a helper to push an error result that includes partial results.
// This is used for bulk operations where some items may have succeeded before a failure.
func (e *Executor) pushErrorWithResult(commandID string, startTime time.Time, err error, result map[string]interface{}) error {
	duration := time.Since(startTime)
	resultReq := &agent.CommandResultRequest{
		Success:    false,
		Error:      err.Error(),
		Result:     result,
		DurationMs: duration.Milliseconds(),
	}
	_, pushErr := e.client.PushCommandResult(commandID, resultReq)
	if pushErr != nil {
		log.Printf("[command] Failed to push error result with data for command %s: %v", commandID, pushErr)
	}
	log.Printf("[command] Command %s failed with partial results: %v (duration: %v)", commandID, err, duration)
	return err
}

// createDriver creates a CLI driver for the given OLT configuration.
func (e *Executor) createDriver(oltConfig agent.OLTConfig) (cli.CLIDriver, error) {
	cliConfig := cli.CLIConfig{
		Host:     oltConfig.Address,
		Port:     oltConfig.Protocols.SSH.Port,
		Username: oltConfig.Protocols.SSH.Username,
		Password: oltConfig.Protocols.SSH.Password,
		Vendor:   oltConfig.Vendor,
		Timeout:  30 * time.Second,
	}

	return e.driverFactory(cliConfig)
}

// createSouthboundDriver creates a southbound driver for read operations.
// This driver supports DriverV2 interface with efficient SNMP-based operations.
func (e *Executor) createSouthboundDriver(ctx context.Context, oltConfig agent.OLTConfig) (southbound.Driver, types.DriverV2, error) {
	vendor := southbound.Vendor(strings.ToLower(oltConfig.Vendor))

	// Determine protocol - prefer SNMP for read operations if enabled
	protocol := southbound.ProtocolCLI
	if oltConfig.Protocols.SNMP.Enabled {
		protocol = southbound.ProtocolSNMP
	}

	config := &southbound.EquipmentConfig{
		Address:  oltConfig.Address,
		Vendor:   vendor,
		Protocol: protocol,
	}

	if protocol == southbound.ProtocolSNMP {
		config.Port = oltConfig.Protocols.SNMP.Port
		config.SNMPCommunity = oltConfig.Protocols.SNMP.Community
		config.SNMPVersion = oltConfig.Protocols.SNMP.Version
	} else {
		config.Port = oltConfig.Protocols.SSH.Port
		config.Username = oltConfig.Protocols.SSH.Username
		config.Password = oltConfig.Protocols.SSH.Password
	}

	driver, err := southbound.NewDriver(vendor, protocol, config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create southbound driver: %w", err)
	}

	// Connect - pass the types.EquipmentConfig
	// Note: SNMP driver reads community/version from Metadata, not direct fields
	typesConfig := &types.EquipmentConfig{
		Name:          oltConfig.Name,
		Type:          types.EquipmentTypeOLT,
		Vendor:        types.Vendor(vendor),
		Address:       oltConfig.Address,
		Port:          config.Port,
		Protocol:      types.Protocol(protocol),
		Username:      config.Username,
		Password:      config.Password,
		SNMPCommunity: config.SNMPCommunity,
		SNMPVersion:   config.SNMPVersion,
		Metadata:      make(map[string]string),
		Timeout:       30 * time.Second,
	}
	if protocol == southbound.ProtocolSNMP {
		typesConfig.Metadata["snmp_community"] = oltConfig.Protocols.SNMP.Community
		typesConfig.Metadata["snmp_version"] = oltConfig.Protocols.SNMP.Version
	}
	if err := driver.Connect(ctx, typesConfig); err != nil {
		return nil, nil, fmt.Errorf("failed to connect southbound driver: %w", err)
	}

	// Check if driver supports DriverV2
	driverV2, ok := driver.(types.DriverV2)
	if !ok {
		_ = driver.Disconnect(ctx)
		return nil, nil, fmt.Errorf("southbound driver does not support DriverV2 interface")
	}

	return driver, driverV2, nil
}

// handleONUListV2 retrieves all ONUs using the efficient DriverV2 interface.
func (e *Executor) handleONUListV2(ctx context.Context, driver types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Get optional filters from payload
	ponPort, _ := cmd.Payload["ponPort"].(string)
	detailed, _ := cmd.Payload["detailed"].(bool)

	// Use the efficient GetONUList method
	onuList, err := driver.GetONUList(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get ONU list: %w", err)
	}

	// If detailed poll is needed, try to get extended info
	if detailed {
		if detailProvider, ok := driver.(interface {
			GetAllONUDetails(ctx context.Context, onus []types.ONUInfo) ([]types.ONUInfo, error)
		}); ok {
			detailedONUs, err := detailProvider.GetAllONUDetails(ctx, onuList)
			if err == nil {
				onuList = detailedONUs
			}
		}
	}

	var onus []map[string]interface{}
	for _, onu := range onuList {
		// Filter by PON port if specified
		if ponPort != "" && !strings.Contains(onu.PONPort, ponPort) {
			continue
		}

		// Determine status from IsOnline, AdminState, and OperState
		// Bug fix: Check for suspended status when AdminState is disabled or OperState is suspended
		status := "offline"
		if onu.IsOnline {
			status = "online"
		} else if onu.AdminState == "disabled" || onu.OperState == "suspended" {
			status = "suspended"
		} else if onu.OperState == "los" {
			status = "los"
		} else if onu.OperState == "discovered" {
			status = "discovered"
		}

		onuData := map[string]interface{}{
			"serial":   onu.Serial,
			"ponPort":  onu.PONPort,
			"onuId":    onu.ONUID,
			"status":   status,
			"type":     onu.Model,
			"distance": onu.DistanceM,
		}

		if detailed {
			onuData["rxPower"] = onu.RxPowerDBm
			onuData["txPower"] = onu.TxPowerDBm
			onuData["temperature"] = onu.Temperature
			onuData["voltage"] = onu.Voltage
			onuData["model"] = onu.Model
			onuData["vendor"] = onu.Vendor
		}

		onus = append(onus, onuData)
	}

	return map[string]interface{}{
		"onus":  onus,
		"count": len(onus),
	}, nil
}

// dispatch routes the command to the appropriate handler based on type.
func (e *Executor) dispatch(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	switch cmd.Type {
	// VLAN commands
	case "vlan_list":
		return e.handleVLANList(ctx, driver, cmd)
	case "vlan_get":
		return e.handleVLANGet(ctx, driver, cmd)
	case "vlan_create":
		return e.handleVLANCreate(ctx, driver, cmd)
	case "vlan_delete":
		return e.handleVLANDelete(ctx, driver, cmd)

	// ONU commands
	case "onu_list":
		return e.handleONUList(ctx, driver, cmd)
	case "onu_get":
		return e.handleONUGet(ctx, driver, cmd)
	case "onu_discover":
		return e.handleONUDiscover(ctx, driver, cmd)
	case "onu_provision":
		return e.handleONUProvision(ctx, driver, cmd)
	case "onu_update":
		return e.handleONUUpdate(ctx, driver, cmd)
	case "onu_delete":
		return e.handleONUDelete(ctx, driver, cmd)
	case "onu_suspend":
		return e.handleONUSuspend(ctx, driver, cmd)
	case "onu_resume":
		return e.handleONUResume(ctx, driver, cmd)
	case "onu_reboot":
		return e.handleONUReboot(ctx, driver, cmd)
	case "onu_diagnostics":
		return e.handleONUDiagnostics(ctx, driver, cmd)

	// Port commands
	case "port_list":
		return e.handlePortList(ctx, driver, cmd)
	case "port_enable":
		return e.handlePortEnable(ctx, driver, cmd)
	case "port_disable":
		return e.handlePortDisable(ctx, driver, cmd)
	case "port_power":
		return e.handlePortPower(ctx, driver, cmd)

	// Service port commands
	case "service_port_list":
		return e.handleServicePortList(ctx, driver, cmd)
	case "service_port_add":
		return e.handleServicePortAdd(ctx, driver, cmd)
	case "service_port_delete":
		return e.handleServicePortDelete(ctx, driver, cmd)

	// OLT status commands
	case "olt_status":
		return e.handleOLTStatus(ctx, driver, cmd)
	case "olt_alarms":
		return e.handleOLTAlarms(ctx, driver, cmd)
	case "olt_health_check":
		return e.handleOLTHealthCheck(ctx, driver, cmd)

	default:
		return nil, fmt.Errorf("unsupported command type: %s", cmd.Type)
	}
}

// isProvisioningCommand returns true if the command type is a provisioning operation
// that benefits from SNMP-based verification after CLI execution.
func isProvisioningCommand(cmdType string) bool {
	switch cmdType {
	case "onu_suspend", "onu_resume", "onu_provision", "onu_delete", "onu_update", "onu_reboot", "onu_bulk_provision":
		return true
	default:
		return false
	}
}

// dispatchProvisioning routes provisioning commands to handlers that support SNMP verification.
// These handlers use CLI for command execution and SNMP (via driverV2) for verification.
func (e *Executor) dispatchProvisioning(ctx context.Context, driver cli.CLIDriver, driverV2 types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	switch cmd.Type {
	case "onu_suspend":
		return e.handleONUSuspendWithVerification(ctx, driver, driverV2, cmd)
	case "onu_resume":
		return e.handleONUResumeWithVerification(ctx, driver, driverV2, cmd)
	case "onu_provision":
		return e.handleONUProvisionWithVerification(ctx, driver, driverV2, cmd)
	case "onu_delete":
		return e.handleONUDeleteWithVerification(ctx, driver, driverV2, cmd)
	case "onu_update":
		return e.handleONUUpdateWithVerification(ctx, driver, driverV2, cmd)
	case "onu_reboot":
		return e.handleONURebootWithVerification(ctx, driver, driverV2, cmd)
	case "onu_bulk_provision":
		return e.handleONUBulkProvisionWithVerification(ctx, driver, driverV2, cmd)
	default:
		// Fallback to regular dispatch without SNMP verification
		return e.dispatch(ctx, driver, cmd)
	}
}
