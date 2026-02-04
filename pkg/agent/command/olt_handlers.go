package command

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nanoncore/nano-agent/pkg/agent"
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
	"github.com/nanoncore/nano-southbound/types"
)

// handleOLTStatusV2 retrieves OLT status using the efficient DriverV2 SNMP interface.
func (e *Executor) handleOLTStatusV2(ctx context.Context, driver types.DriverV2, cmd agent.PendingCommand) (map[string]interface{}, error) {
	status, err := driver.GetOLTStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get OLT status: %w", err)
	}

	// Build status response
	result := map[string]interface{}{
		"vendor":        status.Vendor,
		"model":         status.Model,
		"firmware":      status.Firmware,
		"isReachable":   status.IsReachable,
		"isHealthy":     status.IsHealthy,
		"uptimeSeconds": status.UptimeSeconds,
		"cpuPercent":    status.CPUPercent,
		"memoryPercent": status.MemoryPercent,
		"temperature":   status.Temperature,
		"activeONUs":    status.ActiveONUs,
		"totalONUs":     status.TotalONUs,
	}

	// Add uptime in human-readable format
	if status.UptimeSeconds > 0 {
		days := status.UptimeSeconds / 86400
		hours := (status.UptimeSeconds % 86400) / 3600
		minutes := (status.UptimeSeconds % 3600) / 60
		result["uptime"] = fmt.Sprintf("%d days, %d hours, %d minutes", days, hours, minutes)
	}

	// Add PON port count if available
	if len(status.PONPorts) > 0 {
		result["ponPorts"] = len(status.PONPorts)
		onlinePorts := 0
		for _, port := range status.PONPorts {
			if strings.EqualFold(port.OperState, "up") || strings.EqualFold(port.OperState, "online") {
				onlinePorts++
			}
		}
		result["onlinePorts"] = onlinePorts
	}

	// Add any additional metadata
	if status.Metadata != nil {
		for k, v := range status.Metadata {
			result[k] = v
		}
	}

	return map[string]interface{}{
		"status": result,
	}, nil
}

// handleOLTStatus retrieves the overall status of the OLT including system metrics.
func (e *Executor) handleOLTStatus(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	vendor := driver.Vendor()

	// Collect system information from multiple commands
	var versionOutput, cpuOutput, memoryOutput, tempOutput string

	// Get version/system info (contains uptime on some devices)
	switch vendor {
	case "huawei":
		versionOutput, _ = driver.Execute(ctx, "display version")
		cpuOutput, _ = driver.Execute(ctx, "display cpu")
		memoryOutput, _ = driver.Execute(ctx, "display memory")
		tempOutput, _ = driver.Execute(ctx, "display temperature all")
	case "vsol":
		versionOutput, _ = driver.Execute(ctx, "show system")
		cpuOutput, _ = driver.Execute(ctx, "show cpu")
		memoryOutput, _ = driver.Execute(ctx, "show memory")
		// V-Sol temperature is typically in "show system" output
	default:
		versionOutput, _ = driver.Execute(ctx, "display version")
	}

	// Parse metrics from CLI output
	metrics := parseOLTStatus(vendor, versionOutput, cpuOutput, memoryOutput, tempOutput)

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

	// Build status response with all available metrics
	status := map[string]interface{}{
		"vendor":      vendor,
		"ponPorts":    len(ports),
		"onlinePorts": onlinePorts,
		"totalONUs":   totalONUs,
	}

	// Add parsed metrics (nil values indicate unavailable data)
	if uptime, ok := metrics["uptime"]; ok {
		status["uptime"] = uptime
	}
	if cpuUsage, ok := metrics["cpuUsage"]; ok {
		status["cpuUsage"] = cpuUsage
	}
	if memoryUsage, ok := metrics["memoryUsage"]; ok {
		status["memoryUsage"] = memoryUsage
	}
	if temperature, ok := metrics["temperature"]; ok {
		status["temperature"] = temperature
	}
	if version, ok := metrics["version"]; ok {
		status["version"] = version
	}

	return map[string]interface{}{
		"status": status,
	}, nil
}

// handleOLTAlarms retrieves active alarms from the OLT with structured parsing.
func (e *Executor) handleOLTAlarms(ctx context.Context, driver cli.CLIDriver, cmd agent.PendingCommand) (map[string]interface{}, error) {
	vendor := driver.Vendor()

	var alarms []map[string]interface{}
	switch vendor {
	case "vsol":
		if _, err := driver.Execute(ctx, "configure terminal"); err != nil {
			return nil, fmt.Errorf("failed to enter config mode: %w", err)
		}
		defer driver.Execute(ctx, "end")

		_, _ = driver.Execute(ctx, "show alarm summary")
		oamlogOutput, err := driver.Execute(ctx, "show alarm oamlog")
		if err != nil {
			return nil, fmt.Errorf("failed to get OLT alarm log: %w", err)
		}

		alarms = parseVSolOamlog(oamlogOutput)
	case "huawei":
		output, err := driver.Execute(ctx, "display alarm active")
		if err != nil {
			return nil, fmt.Errorf("failed to get OLT alarms: %w", err)
		}
		alarms = parseOLTAlarms(vendor, output)
	default:
		output, err := driver.Execute(ctx, "display alarm active")
		if err != nil {
			return nil, fmt.Errorf("failed to get OLT alarms: %w", err)
		}
		alarms = parseOLTAlarms(vendor, output)
	}

	return map[string]interface{}{
		"alarms": alarms,
		"count":  len(alarms),
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

// =============================================================================
// Parsing Helper Functions
// =============================================================================

// parseOLTStatus extracts system metrics from CLI output based on vendor.
// Returns a map with available metrics; missing metrics are not included.
func parseOLTStatus(vendor, versionOutput, cpuOutput, memoryOutput, tempOutput string) map[string]interface{} {
	metrics := make(map[string]interface{})

	switch vendor {
	case "huawei":
		parseHuaweiStatus(metrics, versionOutput, cpuOutput, memoryOutput, tempOutput)
	case "vsol":
		parseVSolStatus(metrics, versionOutput, cpuOutput, memoryOutput)
	default:
		// For unknown vendors, try to extract any recognizable patterns
		parseGenericStatus(metrics, versionOutput, cpuOutput, memoryOutput)
	}

	return metrics
}

// parseHuaweiStatus extracts metrics from Huawei CLI output.
func parseHuaweiStatus(metrics map[string]interface{}, versionOutput, cpuOutput, memoryOutput, tempOutput string) {
	// Parse uptime from version output
	// Huawei format: "Uptime is 15 days, 23 hours, 45 minutes" or "Uptime is 5 days"
	uptimeRe := regexp.MustCompile(`(?i)uptime\s+(?:is\s+)?(.+?)(?:\n|$)`)
	if match := uptimeRe.FindStringSubmatch(versionOutput); match != nil {
		uptime := strings.TrimSpace(match[1])
		if uptime != "" {
			metrics["uptime"] = uptime
		}
	}

	// Parse version
	// Huawei format: "VERSION : MA5683T V800R021C00"
	versionRe := regexp.MustCompile(`(?i)VERSION\s*:\s*(.+)`)
	if match := versionRe.FindStringSubmatch(versionOutput); match != nil {
		metrics["version"] = strings.TrimSpace(match[1])
	}

	// Parse CPU usage from "display cpu" output
	// Huawei format: "CPU Usage : 15%"
	cpuRe := regexp.MustCompile(`(?i)CPU\s+(?:Usage|Utilization)\s*[:\s]+(\d+(?:\.\d+)?)\s*%`)
	if match := cpuRe.FindStringSubmatch(cpuOutput); match != nil {
		if cpu, err := strconv.ParseFloat(match[1], 64); err == nil {
			metrics["cpuUsage"] = cpu
		}
	}

	// Parse memory usage from "display memory" output
	// Huawei format: "Memory Usage : 45%" or "Used: 45%"
	memRe := regexp.MustCompile(`(?i)(?:Memory\s+)?(?:Usage|Used)\s*[:\s]+(\d+(?:\.\d+)?)\s*%`)
	if match := memRe.FindStringSubmatch(memoryOutput); match != nil {
		if mem, err := strconv.ParseFloat(match[1], 64); err == nil {
			metrics["memoryUsage"] = mem
		}
	}

	// Parse temperature from "display temperature all" output
	// Huawei format: "Temperature : 45 C" or "Current Temperature: 45"
	tempRe := regexp.MustCompile(`(?i)(?:Current\s+)?Temperature\s*[:\s]+(\d+(?:\.\d+)?)\s*(?:C|Celsius)?`)
	if match := tempRe.FindStringSubmatch(tempOutput); match != nil {
		if temp, err := strconv.ParseFloat(match[1], 64); err == nil {
			metrics["temperature"] = temp
		}
	}
}

// parseVSolStatus extracts metrics from V-Sol CLI output.
func parseVSolStatus(metrics map[string]interface{}, systemOutput, cpuOutput, memoryOutput string) {
	// V-Sol "show system" often contains uptime, version, and sometimes CPU/memory
	// Format: "System uptime: 15 days, 23:45:12" or "Uptime: 15d 23h 45m"
	uptimeRe := regexp.MustCompile(`(?i)(?:system\s+)?uptime\s*[:\s]+(.+?)(?:\n|$)`)
	if match := uptimeRe.FindStringSubmatch(systemOutput); match != nil {
		uptime := strings.TrimSpace(match[1])
		if uptime != "" {
			metrics["uptime"] = uptime
		}
	}

	// Parse version from system output
	// V-Sol format: "Software Version: V2.0.1" or "Firmware: 2.0.1"
	versionRe := regexp.MustCompile(`(?i)(?:Software\s+)?(?:Version|Firmware)\s*[:\s]+([^\s]+)`)
	if match := versionRe.FindStringSubmatch(systemOutput); match != nil {
		metrics["version"] = strings.TrimSpace(match[1])
	}

	// Parse CPU from "show cpu" or embedded in system output
	cpuRe := regexp.MustCompile(`(?i)CPU\s*[:\s]+(\d+(?:\.\d+)?)\s*%`)
	if match := cpuRe.FindStringSubmatch(cpuOutput); match != nil {
		if cpu, err := strconv.ParseFloat(match[1], 64); err == nil {
			metrics["cpuUsage"] = cpu
		}
	} else if match := cpuRe.FindStringSubmatch(systemOutput); match != nil {
		if cpu, err := strconv.ParseFloat(match[1], 64); err == nil {
			metrics["cpuUsage"] = cpu
		}
	}

	// Parse memory from "show memory" or embedded in system output
	memRe := regexp.MustCompile(`(?i)(?:Memory|Mem)\s*[:\s]+(\d+(?:\.\d+)?)\s*%`)
	if match := memRe.FindStringSubmatch(memoryOutput); match != nil {
		if mem, err := strconv.ParseFloat(match[1], 64); err == nil {
			metrics["memoryUsage"] = mem
		}
	} else if match := memRe.FindStringSubmatch(systemOutput); match != nil {
		if mem, err := strconv.ParseFloat(match[1], 64); err == nil {
			metrics["memoryUsage"] = mem
		}
	}

	// Parse temperature from system output if available
	tempRe := regexp.MustCompile(`(?i)(?:Temperature|Temp)\s*[:\s]+(\d+(?:\.\d+)?)\s*(?:C|°)?`)
	if match := tempRe.FindStringSubmatch(systemOutput); match != nil {
		if temp, err := strconv.ParseFloat(match[1], 64); err == nil {
			metrics["temperature"] = temp
		}
	}
}

// parseGenericStatus attempts to extract metrics using common patterns.
func parseGenericStatus(metrics map[string]interface{}, versionOutput, cpuOutput, memoryOutput string) {
	// Generic uptime pattern
	uptimeRe := regexp.MustCompile(`(?i)uptime\s*[:\s]+(.+?)(?:\n|$)`)
	if match := uptimeRe.FindStringSubmatch(versionOutput); match != nil {
		metrics["uptime"] = strings.TrimSpace(match[1])
	}

	// Generic version pattern
	versionRe := regexp.MustCompile(`(?i)(?:version|firmware)\s*[:\s]+([^\s\n]+)`)
	if match := versionRe.FindStringSubmatch(versionOutput); match != nil {
		metrics["version"] = strings.TrimSpace(match[1])
	}

	// Generic CPU pattern
	cpuRe := regexp.MustCompile(`(?i)cpu\s*[:\s]+(\d+(?:\.\d+)?)\s*%`)
	if match := cpuRe.FindStringSubmatch(cpuOutput); match != nil {
		if cpu, err := strconv.ParseFloat(match[1], 64); err == nil {
			metrics["cpuUsage"] = cpu
		}
	}

	// Generic memory pattern
	memRe := regexp.MustCompile(`(?i)(?:memory|mem)\s*[:\s]+(\d+(?:\.\d+)?)\s*%`)
	if match := memRe.FindStringSubmatch(memoryOutput); match != nil {
		if mem, err := strconv.ParseFloat(match[1], 64); err == nil {
			metrics["memoryUsage"] = mem
		}
	}
}

// parseOLTAlarms extracts structured alarm data from CLI output.
func parseOLTAlarms(vendor, output string) []map[string]interface{} {
	// Check for "no alarms" indicators
	noAlarmPatterns := []string{
		"no active alarm",
		"no alarm",
		"alarm table is empty",
		"0 alarm",
		"total: 0",
	}
	lowerOutput := strings.ToLower(output)
	for _, pattern := range noAlarmPatterns {
		if strings.Contains(lowerOutput, pattern) {
			return []map[string]interface{}{}
		}
	}

	switch vendor {
	case "huawei":
		return parseHuaweiAlarms(output)
	case "vsol":
		return parseVSolAlarms(output)
	default:
		return parseGenericAlarms(output)
	}
}

// parseVSolOamlog parses V-SOL alarm/event log output into alarm records.
func parseVSolOamlog(output string) []map[string]interface{} {
	alarms := []map[string]interface{}{}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Example:
		// 2026/02/04 10:46:15   ONU Online               PON 0/2 ONU 1 sn GPON00929978
		// 2026/02/04 10:34:24 major       User Login               User admin logged in from 10.0.0.254 on vty
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		dateStr := fields[0]
		timeStr := fields[1]
		timestamp, err := time.Parse("2006/01/02 15:04:05", fmt.Sprintf("%s %s", dateStr, timeStr))
		if err != nil {
			continue
		}

		rest := strings.TrimSpace(line[len(fields[0])+len(fields[1])+2:])
		columns := regexp.MustCompile(`\s{2,}`).Split(rest, -1)

		severity := "unknown"
		eventName := ""
		message := ""

		if len(columns) == 1 {
			eventName = columns[0]
		} else if len(columns) == 2 {
			eventName = columns[0]
			message = columns[1]
		} else if len(columns) >= 3 {
			severity = strings.ToLower(columns[0])
			eventName = columns[1]
			message = strings.Join(columns[2:], " ")
		}

		if eventName == "" {
			continue
		}

		alarm := map[string]interface{}{
			"id":        fmt.Sprintf("%d", len(alarms)+1),
			"severity":  strings.ToLower(severity),
			"type":      strings.ToLower(strings.ReplaceAll(eventName, " ", "_")),
			"source":    "system",
			"message":   strings.TrimSpace(message),
			"timestamp": timestamp.Format(time.RFC3339),
		}

		if message == "" {
			alarm["message"] = strings.TrimSpace(eventName)
		}

		alarms = append(alarms, alarm)
	}

	return alarms
}

// parseHuaweiAlarms parses Huawei alarm table format.
// Example format:
// ID  Level  Type         Source      Time
// 1   Major  LOS          0/0/1       2026-01-25 10:30:00
// 2   Minor  Temperature  Board 0     2026-01-25 09:15:00
func parseHuaweiAlarms(output string) []map[string]interface{} {
	alarms := []map[string]interface{}{}

	lines := strings.Split(output, "\n")
	// Look for table-formatted alarms
	// Pattern: ID followed by severity level, type, source (may contain spaces), and timestamp
	// Use a lookahead-like approach: capture everything before the timestamp
	alarmLineRe := regexp.MustCompile(`^\s*(\d+)\s+(Critical|Major|Minor|Warning)\s+(\S+)\s+(.+?)\s+(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`)

	for _, line := range lines {
		if match := alarmLineRe.FindStringSubmatch(line); match != nil {
			id, _ := strconv.Atoi(match[1])
			timestamp := parseAlarmTimestamp(match[5])

			alarm := map[string]interface{}{
				"id":        id,
				"severity":  normalizeSeverity(match[2]),
				"type":      match[3],
				"source":    strings.TrimSpace(match[4]),
				"timestamp": timestamp,
			}
			alarms = append(alarms, alarm)
		}
	}

	// If table parsing didn't work, try alternative Huawei format
	if len(alarms) == 0 {
		alarms = parseHuaweiAlarmsAlt(output)
	}

	return alarms
}

// parseHuaweiAlarmsAlt handles alternative Huawei alarm formats.
func parseHuaweiAlarmsAlt(output string) []map[string]interface{} {
	alarms := []map[string]interface{}{}

	// Alternative format with labeled fields:
	// Alarm ID: 1
	// Alarm Level: Major
	// Alarm Type: LOS
	// ...
	idRe := regexp.MustCompile(`(?i)Alarm\s+ID\s*:\s*(\d+)`)
	levelRe := regexp.MustCompile(`(?i)(?:Alarm\s+)?Level\s*:\s*(\w+)`)
	typeRe := regexp.MustCompile(`(?i)(?:Alarm\s+)?Type\s*:\s*(\S+)`)
	sourceRe := regexp.MustCompile(`(?i)(?:Source|Location)\s*:\s*(.+)`)
	timeRe := regexp.MustCompile(`(?i)(?:Time|Timestamp)\s*:\s*(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`)
	msgRe := regexp.MustCompile(`(?i)(?:Message|Description)\s*:\s*(.+)`)

	// Re-split including the delimiter
	fullMatches := idRe.FindAllStringSubmatchIndex(output, -1)
	for i, matchIdx := range fullMatches {
		endIdx := len(output)
		if i+1 < len(fullMatches) {
			endIdx = fullMatches[i+1][0]
		}
		block := output[matchIdx[0]:endIdx]

		var id int
		var severity, alarmType, source, timestamp, message string

		if m := idRe.FindStringSubmatch(block); m != nil {
			id, _ = strconv.Atoi(m[1])
		}
		if m := levelRe.FindStringSubmatch(block); m != nil {
			severity = normalizeSeverity(m[1])
		}
		if m := typeRe.FindStringSubmatch(block); m != nil {
			alarmType = strings.TrimSpace(m[1])
		}
		if m := sourceRe.FindStringSubmatch(block); m != nil {
			source = strings.TrimSpace(m[1])
		}
		if m := timeRe.FindStringSubmatch(block); m != nil {
			timestamp = parseAlarmTimestamp(m[1])
		}
		if m := msgRe.FindStringSubmatch(block); m != nil {
			message = strings.TrimSpace(m[1])
		}

		if id > 0 || alarmType != "" {
			alarm := map[string]interface{}{
				"id":        id,
				"severity":  severity,
				"type":      alarmType,
				"source":    source,
				"timestamp": timestamp,
			}
			if message != "" {
				alarm["message"] = message
			}
			alarms = append(alarms, alarm)
		}
	}

	return alarms
}

// parseVSolAlarms parses V-Sol alarm format.
// Example format:
// Alarm ID: 1
// Severity: Critical
// Type: LOS
// Source: PON 0/1 ONU 5
// Time: 2026-01-25 10:30:00
func parseVSolAlarms(output string) []map[string]interface{} {
	alarms := []map[string]interface{}{}

	// V-Sol typically uses block format with labeled fields
	idRe := regexp.MustCompile(`(?i)Alarm\s+ID\s*:\s*(\d+)`)
	severityRe := regexp.MustCompile(`(?i)Severity\s*:\s*(\w+)`)
	typeRe := regexp.MustCompile(`(?i)Type\s*:\s*(\S+)`)
	sourceRe := regexp.MustCompile(`(?i)Source\s*:\s*(.+)`)
	timeRe := regexp.MustCompile(`(?i)Time\s*:\s*(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`)
	msgRe := regexp.MustCompile(`(?i)(?:Message|Description)\s*:\s*(.+)`)

	// Split by alarm ID to get individual alarm blocks
	fullMatches := idRe.FindAllStringSubmatchIndex(output, -1)
	for i, matchIdx := range fullMatches {
		endIdx := len(output)
		if i+1 < len(fullMatches) {
			endIdx = fullMatches[i+1][0]
		}
		block := output[matchIdx[0]:endIdx]

		var id int
		var severity, alarmType, source, timestamp, message string

		if m := idRe.FindStringSubmatch(block); m != nil {
			id, _ = strconv.Atoi(m[1])
		}
		if m := severityRe.FindStringSubmatch(block); m != nil {
			severity = normalizeSeverity(m[1])
		}
		if m := typeRe.FindStringSubmatch(block); m != nil {
			alarmType = strings.TrimSpace(m[1])
		}
		if m := sourceRe.FindStringSubmatch(block); m != nil {
			source = strings.TrimSpace(m[1])
		}
		if m := timeRe.FindStringSubmatch(block); m != nil {
			timestamp = parseAlarmTimestamp(m[1])
		}
		if m := msgRe.FindStringSubmatch(block); m != nil {
			message = strings.TrimSpace(m[1])
		}

		if id > 0 || alarmType != "" {
			alarm := map[string]interface{}{
				"id":        id,
				"severity":  severity,
				"type":      alarmType,
				"source":    source,
				"timestamp": timestamp,
			}
			if message != "" {
				alarm["message"] = message
			}
			alarms = append(alarms, alarm)
		}
	}

	// If block format didn't work, try table format
	if len(alarms) == 0 {
		alarms = parseVSolAlarmsTable(output)
	}

	return alarms
}

// parseVSolAlarmsTable parses V-Sol table-formatted alarms.
func parseVSolAlarmsTable(output string) []map[string]interface{} {
	alarms := []map[string]interface{}{}

	lines := strings.Split(output, "\n")
	// Table format: ID | Severity | Type | Source | Time
	tableLineRe := regexp.MustCompile(`^\s*(\d+)\s*\|\s*(\w+)\s*\|\s*(\S+)\s*\|\s*(.+?)\s*\|\s*(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`)

	for _, line := range lines {
		if match := tableLineRe.FindStringSubmatch(line); match != nil {
			id, _ := strconv.Atoi(match[1])
			timestamp := parseAlarmTimestamp(match[5])

			alarm := map[string]interface{}{
				"id":        id,
				"severity":  normalizeSeverity(match[2]),
				"type":      strings.TrimSpace(match[3]),
				"source":    strings.TrimSpace(match[4]),
				"timestamp": timestamp,
			}
			alarms = append(alarms, alarm)
		}
	}

	return alarms
}

// parseGenericAlarms attempts to parse alarms using common patterns.
func parseGenericAlarms(output string) []map[string]interface{} {
	alarms := []map[string]interface{}{}

	// Try to identify any alarm-like entries
	// Look for lines with severity indicators
	severityRe := regexp.MustCompile(`(?i)(critical|major|minor|warning|alarm)`)
	lines := strings.Split(output, "\n")

	for i, line := range lines {
		if severityRe.MatchString(line) && len(strings.TrimSpace(line)) > 10 {
			// Extract what we can
			alarm := map[string]interface{}{
				"id":       i + 1,
				"severity": extractSeverity(line),
				"message":  strings.TrimSpace(line),
			}

			// Try to extract timestamp
			timeRe := regexp.MustCompile(`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`)
			if m := timeRe.FindStringSubmatch(line); m != nil {
				alarm["timestamp"] = parseAlarmTimestamp(m[1])
			}

			alarms = append(alarms, alarm)
		}
	}

	return alarms
}

// normalizeSeverity converts various severity representations to standard format.
func normalizeSeverity(severity string) string {
	switch strings.ToLower(severity) {
	case "critical", "crit":
		return "critical"
	case "major", "maj":
		return "major"
	case "minor", "min":
		return "minor"
	case "warning", "warn":
		return "warning"
	case "info", "information":
		return "info"
	default:
		return strings.ToLower(severity)
	}
}

// extractSeverity extracts severity from a line containing severity keywords.
func extractSeverity(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "critical") {
		return "critical"
	}
	if strings.Contains(lower, "major") {
		return "major"
	}
	if strings.Contains(lower, "minor") {
		return "minor"
	}
	if strings.Contains(lower, "warning") {
		return "warning"
	}
	return "unknown"
}

// parseAlarmTimestamp converts a timestamp string to ISO 8601 format.
func parseAlarmTimestamp(ts string) string {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return ""
	}

	// Try parsing common formats
	formats := []string{
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"02-01-2006 15:04:05",
		"01/02/2006 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, ts); err == nil {
			return t.Format(time.RFC3339)
		}
	}

	// Return original if parsing fails
	return ts
}
