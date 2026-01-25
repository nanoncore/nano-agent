package command

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
)

const (
	// vsolMatchCount is the expected number of regex groups for V-SOL format.
	// Groups: full match, ID, VLAN, SVLAN, PON, ONU, GEM, Type
	vsolMatchCount = 8

	// huaweiMatchCount is the expected number of regex groups for Huawei format.
	// Groups: full match, Index, VLAN, Frame/Slot/Port, ONTID, GEM-PORT
	huaweiMatchCount = 6
)

// unsupportedVendorError creates a consistent error message for unsupported vendor operations.
func unsupportedVendorError(operation, vendor string) error {
	return fmt.Errorf("%s not supported for vendor: %s", operation, vendor)
}

// parseServicePorts extracts service port info from CLI output.
// Parses vendor-specific formats into structured data.
func parseServicePorts(vendor, output string) []map[string]interface{} {
	var servicePorts []map[string]interface{}

	lines := strings.Split(output, "\n")

	switch vendor {
	case "vsol":
		// V-SOL format: "ID  VLAN  SVLAN  PON  ONU  GEM  Type"
		// Example: "1    100   -      0/1  1    1    eth"
		re := regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s+(\S+)\s+(\d+/\d+)\s+(\d+)\s+(\d+)\s+(\w+)`)
		for _, line := range lines {
			matches := re.FindStringSubmatch(strings.TrimSpace(line))
			if len(matches) >= vsolMatchCount {
				id, _ := strconv.Atoi(matches[1])
				vlan, _ := strconv.Atoi(matches[2])
				onuID, _ := strconv.Atoi(matches[5])
				gemPort, _ := strconv.Atoi(matches[6])

				svlan := 0
				if matches[3] != "-" {
					svlan, _ = strconv.Atoi(matches[3])
				}

				servicePorts = append(servicePorts, map[string]interface{}{
					"id":      id,
					"vlanId":  vlan,
					"svlan":   svlan,
					"ponPort": matches[4],
					"onuId":   onuID,
					"gemPort": gemPort,
					"type":    matches[7],
				})
			}
		}
	case "huawei":
		// Huawei format: "Index  VLAN  Frame/Slot/Port  ONTID  GEM-PORT  ..."
		// Example: "1      100   0/0/1            1      1        ..."
		re := regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s+(\d+/\d+/\d+)\s+(\d+)\s+(\d+)`)
		for _, line := range lines {
			matches := re.FindStringSubmatch(strings.TrimSpace(line))
			if len(matches) >= huaweiMatchCount {
				index, _ := strconv.Atoi(matches[1])
				vlan, _ := strconv.Atoi(matches[2])
				ontID, _ := strconv.Atoi(matches[4])
				gemPort, _ := strconv.Atoi(matches[5])

				servicePorts = append(servicePorts, map[string]interface{}{
					"index":   index,
					"vlanId":  vlan,
					"ponPort": matches[3],
					"onuId":   ontID,
					"gemPort": gemPort,
				})
			}
		}
	default:
		// Generic parsing - try to extract any table-like data
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "=") {
				continue
			}
			// Skip header lines
			if strings.Contains(strings.ToLower(line), "index") || strings.Contains(strings.ToLower(line), "vlan") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				servicePorts = append(servicePorts, map[string]interface{}{
					"raw": fields,
				})
			}
		}
	}

	return servicePorts
}

// getServicePortState captures the current service port state for verification.
func (e *Executor) getServicePortState(ctx context.Context, driver cli.CLIDriver, ponPort string, onuID int) (map[string]interface{}, error) {
	vendor := driver.Vendor()

	var listCmd string
	switch vendor {
	case "huawei":
		listCmd = fmt.Sprintf("display service-port port 0/%s ont %d", ponPort, onuID)
	case "vsol":
		listCmd = fmt.Sprintf("show service-port interface gpon 0/%s onu %d", strings.Split(ponPort, "/")[1], onuID)
	default:
		listCmd = "show service-port"
	}

	output, err := driver.Execute(ctx, listCmd)
	if err != nil {
		return nil, err
	}

	servicePorts := parseServicePorts(vendor, output)

	return map[string]interface{}{
		"servicePorts": servicePorts,
		"count":        len(servicePorts),
		"ponPort":      ponPort,
		"onuId":        onuID,
	}, nil
}

// verifyServicePortOperation verifies whether a service port operation succeeded.
func verifyServicePortOperation(preState, postState map[string]interface{}, operation string) map[string]interface{} {
	preCount := 0
	postCount := 0

	if pre, ok := preState["count"].(int); ok {
		preCount = pre
	}
	if post, ok := postState["count"].(int); ok {
		postCount = post
	}

	verified := false
	changes := []string{}

	switch operation {
	case "add":
		// Verify new service port exists in postState
		verified = postCount > preCount
		if verified {
			changes = append(changes, "service_port_added")
		}
	case "delete":
		// Verify service port removed from postState
		verified = postCount < preCount
		if verified {
			changes = append(changes, "service_port_removed")
		}
	}

	return map[string]interface{}{
		"verified": verified,
		"changes":  changes,
	}
}

// handleServicePortList retrieves all service ports from the OLT.
// Note: Service ports are the mapping between VLANs and ONUs.
func (e *Executor) handleServicePortList(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Get optional filters from payload
	ponPort, _ := cmd.Payload["ponPort"].(string)
	onuIDFloat, _ := cmd.Payload["onuId"].(float64)
	onuID := int(onuIDFloat)

	vendor := driver.Vendor()

	var listCmd string
	switch vendor {
	case "huawei":
		if ponPort != "" && onuID > 0 {
			listCmd = fmt.Sprintf("display service-port port 0/%s ont %d", ponPort, onuID)
		} else {
			listCmd = "display service-port all"
		}
	case "vsol":
		if ponPort != "" && onuID > 0 {
			port := strings.Split(ponPort, "/")
			portNum := ponPort
			if len(port) > 1 {
				portNum = port[1]
			}
			listCmd = fmt.Sprintf("show service-port interface gpon 0/%s onu %d", portNum, onuID)
		} else {
			listCmd = "show service-port"
		}
	default:
		return nil, unsupportedVendorError("service port listing", vendor)
	}

	output, err := driver.Execute(ctx, listCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list service ports: %w", err)
	}

	// Parse the output into structured data
	servicePorts := parseServicePorts(vendor, output)

	return map[string]interface{}{
		"servicePorts": servicePorts,
		"total":        len(servicePorts),
		"ponPort":      ponPort,
		"onuId":        onuID,
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

	// Capture pre-state
	preState, err := e.getServicePortState(ctx, driver, ponPort, onuID)
	if err != nil {
		log.Printf("[command] warning: failed to capture pre-state for service port add verification: %v", err)
	}

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
		return nil, unsupportedVendorError("service port add", vendor)
	}

	output, err := driver.Execute(ctx, addCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to add service port: %w (output: %s)", err, output)
	}

	// Capture post-state
	postState, err := e.getServicePortState(ctx, driver, ponPort, onuID)
	if err != nil {
		log.Printf("[command] warning: failed to capture post-state for service port add verification: %v", err)
	}

	// Verify operation
	verification := verifyServicePortOperation(preState, postState, "add")

	return map[string]interface{}{
		"success": true,
		"servicePort": map[string]interface{}{
			"vlanId":   vlanID,
			"ponPort":  ponPort,
			"onuId":    onuID,
			"gemPort":  gemPort,
			"userVlan": userVlan,
		},
		"preState":  preState,
		"postState": postState,
		"verified":  verification["verified"],
		"changes":   verification["changes"],
		"message":   "Service port added successfully",
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

	// Optional: specific service port index to delete
	indexFloat, hasIndex := cmd.Payload["index"].(float64)
	index := int(indexFloat)

	// Parse PON port
	slot, port, err := parsePonPort(ponPort)
	if err != nil {
		return nil, err
	}

	vendor := driver.Vendor()

	// Capture pre-state
	preState, err := e.getServicePortState(ctx, driver, ponPort, onuID)
	if err != nil {
		log.Printf("[command] warning: failed to capture pre-state for service port delete verification: %v", err)
	}

	var deleteCmd string
	switch vendor {
	case "huawei":
		if hasIndex && index > 0 {
			// Delete specific service port by index
			deleteCmd = fmt.Sprintf("undo service-port %d", index)
		} else {
			// Delete all service ports for this ONU
			deleteCmd = fmt.Sprintf("undo service-port port 0/%d/%d ont %d", slot, port, onuID)
		}
	case "vsol":
		if hasIndex && index > 0 {
			deleteCmd = fmt.Sprintf("no service-port %d", index)
		} else {
			deleteCmd = fmt.Sprintf("no service-port interface gpon 0/%d onu %d", port, onuID)
		}
	default:
		return nil, unsupportedVendorError("service port delete", vendor)
	}

	output, err := driver.Execute(ctx, deleteCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to delete service port: %w (output: %s)", err, output)
	}

	// Capture post-state
	postState, err := e.getServicePortState(ctx, driver, ponPort, onuID)
	if err != nil {
		log.Printf("[command] warning: failed to capture post-state for service port delete verification: %v", err)
	}

	// Verify operation
	verification := verifyServicePortOperation(preState, postState, "delete")

	return map[string]interface{}{
		"success": true,
		"deleted": map[string]interface{}{
			"ponPort": ponPort,
			"onuId":   onuID,
			"index":   index,
		},
		"preState":  preState,
		"postState": postState,
		"verified":  verification["verified"],
		"changes":   verification["changes"],
		"message":   "Service port deleted successfully",
	}, nil
}
