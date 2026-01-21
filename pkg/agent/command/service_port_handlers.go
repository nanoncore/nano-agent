package command

import (
	"context"
	"fmt"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
)

// handleServicePortList retrieves all service ports from the OLT.
// Note: Service ports are the mapping between VLANs and ONUs.
func (e *Executor) handleServicePortList(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Get service ports via vendor-specific commands
	vendor := driver.Vendor()

	var listCmd string
	switch vendor {
	case "huawei":
		listCmd = "display service-port all"
	case "vsol":
		listCmd = "show service-port"
	default:
		return nil, fmt.Errorf("service port listing not supported for vendor: %s", vendor)
	}

	output, err := driver.Execute(ctx, listCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list service ports: %w", err)
	}

	// Parse the output - this is vendor-specific
	// For now, return raw output and count
	return map[string]interface{}{
		"servicePorts": []map[string]interface{}{},
		"rawOutput":    output,
		"message":      "Service port parsing is vendor-specific. Raw output provided.",
	}, nil
}

// handleServicePortAdd adds a new service port to map a VLAN to an ONU.
func (e *Executor) handleServicePortAdd(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	vlanIDFloat, ok := cmd.Payload["vlanId"].(float64)
	if !ok {
		return nil, fmt.Errorf("vlanId is required")
	}
	vlanID := int(vlanIDFloat)

	ponPort, _ := cmd.Payload["ponPort"].(string)
	if ponPort == "" {
		return nil, fmt.Errorf("ponPort is required")
	}

	onuIDFloat, ok := cmd.Payload["onuId"].(float64)
	if !ok {
		return nil, fmt.Errorf("onuId is required")
	}
	onuID := int(onuIDFloat)

	gemPortFloat, _ := cmd.Payload["gemPort"].(float64)
	gemPort := int(gemPortFloat)
	if gemPort == 0 {
		gemPort = 1 // Default GEM port
	}

	userVlanFloat, _ := cmd.Payload["userVlan"].(float64)
	userVlan := int(userVlanFloat)

	// Parse PON port
	slot, port, err := parsePonPort(ponPort)
	if err != nil {
		return nil, err
	}

	vendor := driver.Vendor()

	var addCmd string
	switch vendor {
	case "huawei":
		// Huawei service-port command
		if userVlan > 0 {
			addCmd = fmt.Sprintf("service-port vlan %d gpon 0/%d/%d ont %d gemport %d multi-service user-vlan %d tag-transform translate",
				vlanID, slot, port, onuID, gemPort, userVlan)
		} else {
			addCmd = fmt.Sprintf("service-port vlan %d gpon 0/%d/%d ont %d gemport %d multi-service user-vlan untagged tag-transform translate",
				vlanID, slot, port, onuID, gemPort)
		}
	case "vsol":
		addCmd = fmt.Sprintf("service-port vlan %d interface gpon 0/%d onu %d gemport %d", vlanID, port, onuID, gemPort)
	default:
		return nil, fmt.Errorf("service port add not supported for vendor: %s", vendor)
	}

	output, err := driver.Execute(ctx, addCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to add service port: %w (output: %s)", err, output)
	}

	return map[string]interface{}{
		"success": true,
		"servicePort": map[string]interface{}{
			"vlanId":   vlanID,
			"ponPort":  ponPort,
			"onuId":    onuID,
			"gemPort":  gemPort,
			"userVlan": userVlan,
		},
		"message": "Service port added successfully",
	}, nil
}

// handleServicePortDelete removes a service port.
func (e *Executor) handleServicePortDelete(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	ponPort, _ := cmd.Payload["ponPort"].(string)
	if ponPort == "" {
		return nil, fmt.Errorf("ponPort is required")
	}

	onuIDFloat, ok := cmd.Payload["onuId"].(float64)
	if !ok {
		return nil, fmt.Errorf("onuId is required")
	}
	onuID := int(onuIDFloat)

	// Parse PON port
	slot, port, err := parsePonPort(ponPort)
	if err != nil {
		return nil, err
	}

	vendor := driver.Vendor()

	// First, we need to find the service port index
	// Then delete it
	var deleteCmd string
	switch vendor {
	case "huawei":
		// Huawei requires the service-port index, which we'd need to look up
		// For now, we use a pattern-based approach
		deleteCmd = fmt.Sprintf("undo service-port port 0/%d/%d ont %d", slot, port, onuID)
	case "vsol":
		deleteCmd = fmt.Sprintf("no service-port interface gpon 0/%d onu %d", port, onuID)
	default:
		return nil, fmt.Errorf("service port delete not supported for vendor: %s", vendor)
	}

	output, err := driver.Execute(ctx, deleteCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to delete service port: %w (output: %s)", err, output)
	}

	return map[string]interface{}{
		"success": true,
		"deleted": map[string]interface{}{
			"ponPort": ponPort,
			"onuId":   onuID,
		},
		"message": "Service port deleted successfully",
	}, nil
}
