package command

import (
	"context"
	"fmt"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
)

// handleVLANList retrieves all VLANs from the OLT.
func (e *Executor) handleVLANList(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	vlans, err := driver.ListVLANs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list VLANs: %w", err)
	}

	// Convert to result format
	vlanList := make([]map[string]interface{}, 0, len(vlans))
	for _, vlan := range vlans {
		vlanList = append(vlanList, map[string]interface{}{
			"id":          vlan.ID,
			"name":        vlan.Name,
			"description": vlan.Description,
			"tagged":      vlan.Tagged,
			"untagged":    vlan.Untagged,
		})
	}

	return map[string]interface{}{
		"vlans": vlanList,
		"count": len(vlanList),
	}, nil
}

// handleVLANGet retrieves a specific VLAN by ID.
func (e *Executor) handleVLANGet(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	vlanID, ok := cmd.Payload["vlanId"].(float64)
	if !ok {
		return nil, fmt.Errorf("vlanId is required")
	}

	vlans, err := driver.ListVLANs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list VLANs: %w", err)
	}

	for _, vlan := range vlans {
		if vlan.ID == int(vlanID) {
			return map[string]interface{}{
				"vlan": map[string]interface{}{
					"id":          vlan.ID,
					"name":        vlan.Name,
					"description": vlan.Description,
					"tagged":      vlan.Tagged,
					"untagged":    vlan.Untagged,
				},
			}, nil
		}
	}

	return nil, fmt.Errorf("VLAN %d not found", int(vlanID))
}

// handleVLANCreate creates a new VLAN on the OLT.
// Note: This requires extending the CLIDriver interface with CreateVLAN method.
// For now, we execute raw commands.
func (e *Executor) handleVLANCreate(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	vlanID, ok := cmd.Payload["vlanId"].(float64)
	if !ok {
		return nil, fmt.Errorf("vlanId is required")
	}

	name, _ := cmd.Payload["name"].(string)
	if name == "" {
		name = fmt.Sprintf("VLAN%d", int(vlanID))
	}

	// Execute vendor-specific VLAN creation commands
	// This is a basic implementation - vendor drivers should implement this properly
	vendor := driver.Vendor()

	var createCmd string
	switch vendor {
	case "huawei":
		createCmd = fmt.Sprintf("vlan %d smart\n vlan name %d %s\n quit", int(vlanID), int(vlanID), name)
	case "vsol":
		createCmd = fmt.Sprintf("vlan %d\n name %s\n exit", int(vlanID), name)
	default:
		return nil, fmt.Errorf("VLAN creation not supported for vendor: %s", vendor)
	}

	output, err := driver.Execute(ctx, createCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to create VLAN: %w (output: %s)", err, output)
	}

	// Verify creation by listing VLANs
	vlans, err := driver.ListVLANs(ctx)
	if err != nil {
		return map[string]interface{}{
			"success": true,
			"vlan": map[string]interface{}{
				"id":   int(vlanID),
				"name": name,
			},
			"verified": false,
			"message":  "VLAN created but verification failed",
		}, nil
	}

	for _, vlan := range vlans {
		if vlan.ID == int(vlanID) {
			return map[string]interface{}{
				"success": true,
				"vlan": map[string]interface{}{
					"id":          vlan.ID,
					"name":        vlan.Name,
					"description": vlan.Description,
				},
				"verified": true,
			}, nil
		}
	}

	return map[string]interface{}{
		"success":  true,
		"verified": false,
		"message":  "VLAN creation command executed but VLAN not found in list",
	}, nil
}

// handleVLANDelete removes a VLAN from the OLT.
func (e *Executor) handleVLANDelete(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	vlanID, ok := cmd.Payload["vlanId"].(float64)
	if !ok {
		return nil, fmt.Errorf("vlanId is required")
	}

	force, _ := cmd.Payload["force"].(bool)

	// Get pre-state for verification
	preVlans, _ := driver.ListVLANs(ctx)
	var preState []map[string]interface{}
	for _, v := range preVlans {
		preState = append(preState, map[string]interface{}{"id": v.ID, "name": v.Name})
	}

	// Execute vendor-specific VLAN deletion commands
	vendor := driver.Vendor()

	var deleteCmd string
	switch vendor {
	case "huawei":
		if force {
			deleteCmd = fmt.Sprintf("undo vlan %d force", int(vlanID))
		} else {
			deleteCmd = fmt.Sprintf("undo vlan %d", int(vlanID))
		}
	case "vsol":
		deleteCmd = fmt.Sprintf("no vlan %d", int(vlanID))
	default:
		return nil, fmt.Errorf("VLAN deletion not supported for vendor: %s", vendor)
	}

	output, err := driver.Execute(ctx, deleteCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to delete VLAN: %w (output: %s)", err, output)
	}

	// Verify deletion
	postVlans, _ := driver.ListVLANs(ctx)
	var postState []map[string]interface{}
	for _, v := range postVlans {
		postState = append(postState, map[string]interface{}{"id": v.ID, "name": v.Name})
	}

	// Check if VLAN was removed
	deleted := true
	for _, vlan := range postVlans {
		if vlan.ID == int(vlanID) {
			deleted = false
			break
		}
	}

	return map[string]interface{}{
		"success":   deleted,
		"verified":  deleted,
		"preState":  preState,
		"postState": postState,
	}, nil
}
