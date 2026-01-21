package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
)

// handleOLTStatus retrieves the overall status of the OLT.
func (e *Executor) handleOLTStatus(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	vendor := driver.Vendor()

	var statusCmd string
	switch vendor {
	case "huawei":
		statusCmd = "display version"
	case "vsol":
		statusCmd = "show system"
	default:
		statusCmd = "display version"
	}

	output, err := driver.Execute(ctx, statusCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get OLT status: %w", err)
	}

	// Get board/slot information
	var boardCmd string
	switch vendor {
	case "huawei":
		boardCmd = "display board 0"
	case "vsol":
		boardCmd = "show board"
	default:
		boardCmd = "display board 0"
	}

	boardOutput, boardErr := driver.Execute(ctx, boardCmd)

	// Get PON port summary
	ports, portsErr := driver.ListPONPorts(ctx)
	var totalONUs int
	var onlinePorts int
	if portsErr == nil {
		for _, port := range ports {
			totalONUs += port.ONUCount
			if strings.ToLower(port.Status) == "up" || strings.ToLower(port.Status) == "online" {
				onlinePorts++
			}
		}
	}

	status := map[string]interface{}{
		"vendor":      vendor,
		"version":     output,
		"ponPorts":    len(ports),
		"onlinePorts": onlinePorts,
		"totalONUs":   totalONUs,
	}

	if boardErr == nil {
		status["board"] = boardOutput
	}

	return map[string]interface{}{
		"status": status,
	}, nil
}

// handleOLTAlarms retrieves active alarms from the OLT.
func (e *Executor) handleOLTAlarms(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	vendor := driver.Vendor()

	var alarmCmd string
	switch vendor {
	case "huawei":
		alarmCmd = "display alarm active"
	case "vsol":
		alarmCmd = "show alarm active"
	default:
		alarmCmd = "display alarm active"
	}

	output, err := driver.Execute(ctx, alarmCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get OLT alarms: %w", err)
	}

	// Parse alarm output - this is vendor-specific
	// For now, return raw output
	return map[string]interface{}{
		"alarms":    []map[string]interface{}{},
		"rawOutput": output,
		"message":   "Alarm parsing is vendor-specific. Raw output provided.",
	}, nil
}

// handleOLTHealthCheck performs a basic health check on the OLT.
func (e *Executor) handleOLTHealthCheck(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	// Check if we can communicate with the OLT
	vendor := driver.Vendor()

	var pingCmd string
	switch vendor {
	case "huawei":
		pingCmd = "display version"
	case "vsol":
		pingCmd = "show version"
	default:
		pingCmd = "display version"
	}

	_, err := driver.Execute(ctx, pingCmd)
	if err != nil {
		return map[string]interface{}{
			"healthy": false,
			"message": fmt.Sprintf("OLT communication failed: %v", err),
		}, nil
	}

	// Check PON ports
	ports, portsErr := driver.ListPONPorts(ctx)
	var healthIssues []string

	if portsErr != nil {
		healthIssues = append(healthIssues, fmt.Sprintf("Could not retrieve PON port status: %v", portsErr))
	} else {
		var downPorts int
		for _, port := range ports {
			status := strings.ToLower(port.Status)
			adminStatus := strings.ToLower(port.AdminStatus)
			// Port is unhealthy if admin is enabled but status is down
			if adminStatus == "enable" && (status == "down" || status == "offline") {
				downPorts++
			}
		}
		if downPorts > 0 {
			healthIssues = append(healthIssues, fmt.Sprintf("%d PON ports are down despite being enabled", downPorts))
		}
	}

	healthy := len(healthIssues) == 0
	message := "OLT is healthy"
	if !healthy {
		message = strings.Join(healthIssues, "; ")
	}

	return map[string]interface{}{
		"healthy":   healthy,
		"message":   message,
		"issues":    healthIssues,
		"portCount": len(ports),
	}, nil
}
