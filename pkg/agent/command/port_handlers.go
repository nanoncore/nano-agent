package command

import (
	"context"
	"fmt"
	"log"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
	"github.com/nanoncore/nano-southbound/types"
)

// handlePortList retrieves all PON ports from the OLT.
func (e *Executor) handlePortList(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	ports, err := driver.ListPONPorts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list PON ports: %w", err)
	}

	portList := make([]map[string]interface{}, 0, len(ports))
	for _, port := range ports {
		portList = append(portList, map[string]interface{}{
			"slot":        port.Slot,
			"port":        port.Port,
			"name":        port.Name,
			"status":      port.Status,
			"adminStatus": port.AdminStatus,
			"type":        port.Type,
			"onuCount":    port.ONUCount,
			"maxOnus":     port.MaxONUs,
			"txPower":     port.TxPower,
			"rxPower":     port.RxPower,
			"description": port.Description,
		})
	}

	return map[string]interface{}{
		"ports": portList,
		"count": len(portList),
	}, nil
}

// handlePortListV2 retrieves all PON ports using the efficient DriverV2 interface.
func (e *Executor) handlePortListV2(ctx context.Context, driver types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	ports, err := driver.ListPorts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list ports: %w", err)
	}

	portList := make([]map[string]interface{}, 0, len(ports))
	for _, port := range ports {
		portData := map[string]interface{}{
			"name":        port.Port,
			"status":      port.OperState,
			"adminStatus": port.AdminState,
			"type":        "gpon",
			"onuCount":    port.ONUCount,
			"maxOnus":     port.MaxONUs,
			"description": port.Description,
		}

		if port.TxPowerDBm != 0 {
			portData["txPower"] = port.TxPowerDBm
		}
		if port.RxPowerDBm != 0 {
			portData["rxPower"] = port.RxPowerDBm
		}

		portList = append(portList, portData)
	}

	return map[string]interface{}{
		"ports": portList,
		"count": len(portList),
	}, nil
}

// handlePortEnable enables a PON port.
func (e *Executor) handlePortEnable(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	port, _ := cmd.Payload["port"].(string)
	if port == "" {
		return nil, fmt.Errorf("port is required")
	}

	slot, portNum, err := parsePonPort(port)
	if err != nil {
		return nil, err
	}

	// Get pre-state
	preInfo, err := driver.GetPONPortInfo(ctx, slot, portNum)
	if err != nil {
		log.Printf("[command] warning: failed to capture pre-state for port enable verification: %v", err)
	}
	var preState map[string]interface{}
	if preInfo != nil {
		preState = map[string]interface{}{
			"status":      preInfo.Status,
			"adminStatus": preInfo.AdminStatus,
		}
	}

	// Enable the port
	err = driver.EnablePONPort(ctx, slot, portNum)
	if err != nil {
		return nil, fmt.Errorf("failed to enable port: %w", err)
	}

	// Verify
	postInfo, err := driver.GetPONPortInfo(ctx, slot, portNum)
	if err != nil {
		log.Printf("[command] warning: failed to capture post-state for port enable verification: %v", err)
	}
	var postState map[string]interface{}
	verified := false
	if postInfo != nil {
		postState = map[string]interface{}{
			"status":      postInfo.Status,
			"adminStatus": postInfo.AdminStatus,
		}
		verified = postInfo.AdminStatus == "enable"
	}

	return map[string]interface{}{
		"success":   true,
		"verified":  verified,
		"port":      port,
		"preState":  preState,
		"postState": postState,
	}, nil
}

// handlePortDisable disables a PON port.
func (e *Executor) handlePortDisable(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	port, _ := cmd.Payload["port"].(string)
	if port == "" {
		return nil, fmt.Errorf("port is required")
	}

	slot, portNum, err := parsePonPort(port)
	if err != nil {
		return nil, err
	}

	// Get pre-state
	preInfo, err := driver.GetPONPortInfo(ctx, slot, portNum)
	if err != nil {
		log.Printf("[command] warning: failed to capture pre-state for port disable verification: %v", err)
	}
	var preState map[string]interface{}
	if preInfo != nil {
		preState = map[string]interface{}{
			"status":      preInfo.Status,
			"adminStatus": preInfo.AdminStatus,
			"onuCount":    preInfo.ONUCount,
		}
	}

	// Disable the port
	err = driver.DisablePONPort(ctx, slot, portNum)
	if err != nil {
		return nil, fmt.Errorf("failed to disable port: %w", err)
	}

	// Verify
	postInfo, err := driver.GetPONPortInfo(ctx, slot, portNum)
	if err != nil {
		log.Printf("[command] warning: failed to capture post-state for port disable verification: %v", err)
	}
	var postState map[string]interface{}
	verified := false
	if postInfo != nil {
		postState = map[string]interface{}{
			"status":      postInfo.Status,
			"adminStatus": postInfo.AdminStatus,
		}
		verified = postInfo.AdminStatus == "disable"
	}

	return map[string]interface{}{
		"success":   true,
		"verified":  verified,
		"port":      port,
		"preState":  preState,
		"postState": postState,
	}, nil
}

// handlePortPower retrieves the optical power readings for a PON port.
func (e *Executor) handlePortPower(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	port, _ := cmd.Payload["port"].(string)
	if port == "" {
		return nil, fmt.Errorf("port is required")
	}

	slot, portNum, err := parsePonPort(port)
	if err != nil {
		return nil, err
	}

	// Get port info which should include power readings
	portInfo, err := driver.GetPONPortInfo(ctx, slot, portNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get port info: %w", err)
	}

	return map[string]interface{}{
		"port": port,
		"power": map[string]interface{}{
			"txPower": portInfo.TxPower,
			"rxPower": portInfo.RxPower,
		},
		"status":      portInfo.Status,
		"adminStatus": portInfo.AdminStatus,
		"onuCount":    portInfo.ONUCount,
	}, nil
}
