package huawei

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
)

// HuaweiCLIDriver implements CLI operations for Huawei OLTs.
type HuaweiCLIDriver struct {
	*cli.BaseCLIDriver
	model        string
	capabilities *cli.VendorCapabilities
}

// NewHuaweiCLIDriver creates a new Huawei CLI driver.
func NewHuaweiCLIDriver(config cli.CLIConfig) *HuaweiCLIDriver {
	return NewHuaweiCLIDriverWithModel(config, "")
}

// NewHuaweiCLIDriverWithModel creates a new Huawei CLI driver with model-specific capabilities.
func NewHuaweiCLIDriverWithModel(config cli.CLIConfig, model string) *HuaweiCLIDriver {
	return &HuaweiCLIDriver{
		BaseCLIDriver: cli.NewBaseCLIDriver(config),
		model:         model,
		capabilities:  cli.GetCapabilities("huawei", model),
	}
}

// Vendor returns the vendor type.
func (d *HuaweiCLIDriver) Vendor() string {
	return "huawei"
}

// GetCapabilities returns the feature support matrix for this driver.
func (d *HuaweiCLIDriver) GetCapabilities() *cli.VendorCapabilities {
	return d.capabilities
}

// Model returns the OLT model.
func (d *HuaweiCLIDriver) Model() string {
	return d.model
}

// Connect establishes connection and enters config mode.
func (d *HuaweiCLIDriver) Connect(ctx context.Context) error {
	if err := d.BaseCLIDriver.Connect(ctx); err != nil {
		return err
	}

	// Enter enable mode
	if _, err := d.Execute(ctx, "enable"); err != nil {
		return fmt.Errorf("failed to enter enable mode: %w", err)
	}

	// Enter config mode
	if _, err := d.Execute(ctx, "config"); err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}

	return nil
}

// =============================================================================
// ONU Provisioning
// =============================================================================

// AddONU provisions a new ONT on Huawei OLT.
func (d *HuaweiCLIDriver) AddONU(ctx context.Context, req *cli.ONUProvisionRequest) error {
	if req.SerialNumber == "" {
		return fmt.Errorf("serial number is required")
	}

	// Enter GPON interface context
	cmd := fmt.Sprintf("interface gpon %s", req.PonPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	// Build ONT add command
	cmdParts := []string{
		fmt.Sprintf("ont add %d sn-auth %s omci", req.OnuID, req.SerialNumber),
	}

	if req.LineProfile != "" {
		cmdParts = append(cmdParts, fmt.Sprintf("ont-lineprofile-id %s", req.LineProfile))
	}
	if req.ServiceProfile != "" {
		cmdParts = append(cmdParts, fmt.Sprintf("ont-srvprofile-id %s", req.ServiceProfile))
	}
	if req.Description != "" {
		cmdParts = append(cmdParts, fmt.Sprintf("desc \"%s\"", req.Description))
	}

	cmd = strings.Join(cmdParts, " ")
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to add ONT: %w", err)
	}

	if strings.Contains(strings.ToLower(output), "error") ||
		strings.Contains(strings.ToLower(output), "failure") {
		return fmt.Errorf("ONT add failed: %s", output)
	}

	// Configure service ports if specified
	for _, sp := range req.ServicePorts {
		cmd = fmt.Sprintf("ont port native-vlan %d eth %d vlan %d",
			req.OnuID, sp.Index, sp.VLAN)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to configure service port: %w", err)
		}
	}

	// Exit interface
	if _, err := d.Execute(ctx, "quit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// DeleteONU removes an ONT from Huawei OLT.
func (d *HuaweiCLIDriver) DeleteONU(ctx context.Context, ponPort string, onuID int) error {
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	cmd = fmt.Sprintf("ont delete %d", onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to delete ONT: %w", err)
	}

	if strings.Contains(strings.ToLower(output), "error") &&
		!strings.Contains(strings.ToLower(output), "not exist") {
		return fmt.Errorf("ONT delete failed: %s", output)
	}

	if _, err := d.Execute(ctx, "quit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// GetONUInfo retrieves ONT information via CLI.
func (d *HuaweiCLIDriver) GetONUInfo(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
	cmd := fmt.Sprintf("display ont info %s %d", ponPort, onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get ONT info: %w", err)
	}

	return parseHuaweiONTInfo(output, ponPort, onuID)
}

// RebootONU reboots a specific ONT.
func (d *HuaweiCLIDriver) RebootONU(ctx context.Context, ponPort string, onuID int) error {
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	cmd = fmt.Sprintf("ont reset %d", onuID)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to reboot ONT: %w", err)
	}

	if _, err := d.Execute(ctx, "quit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// =============================================================================
// VLAN Management
// =============================================================================

// ConfigureVLAN configures VLAN settings for an ONT.
func (d *HuaweiCLIDriver) ConfigureVLAN(ctx context.Context, config *cli.VLANConfig) error {
	cmd := fmt.Sprintf("interface gpon %s", config.PonPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	// Configure native VLAN
	if config.NativeVLAN > 0 {
		cmd = fmt.Sprintf("ont port native-vlan %d eth 1 vlan %d", config.OnuID, config.NativeVLAN)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to set native VLAN: %w", err)
		}
	}

	// Configure VLAN translations
	for _, trans := range config.Translations {
		cmd = fmt.Sprintf("ont port vlan %d eth 1 translation %d to %d",
			config.OnuID, trans.CustomerVLAN, trans.ServiceVLAN)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to add VLAN translation: %w", err)
		}
	}

	if _, err := d.Execute(ctx, "quit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// GetVLANConfig retrieves VLAN configuration for an ONT.
func (d *HuaweiCLIDriver) GetVLANConfig(ctx context.Context, ponPort string, onuID int) (*cli.VLANConfig, error) {
	cmd := fmt.Sprintf("display ont port vlan %s %d", ponPort, onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get VLAN config: %w", err)
	}

	config := &cli.VLANConfig{
		OnuID:   onuID,
		PonPort: ponPort,
	}

	// Parse native VLAN
	nativeRegex := regexp.MustCompile(`(?i)native[- ]vlan\s*:\s*(\d+)`)
	if matches := nativeRegex.FindStringSubmatch(output); len(matches) > 1 {
		if vlan, err := strconv.Atoi(matches[1]); err == nil {
			config.NativeVLAN = vlan
		}
	}

	// Parse VLAN translations
	transRegex := regexp.MustCompile(`(?i)(\d+)\s*->\s*(\d+)`)
	transMatches := transRegex.FindAllStringSubmatch(output, -1)
	for _, m := range transMatches {
		if len(m) >= 3 {
			cVlan, _ := strconv.Atoi(m[1])
			sVlan, _ := strconv.Atoi(m[2])
			config.Translations = append(config.Translations, cli.VLANTranslation{
				CustomerVLAN: cVlan,
				ServiceVLAN:  sVlan,
				Mode:         "translate",
			})
		}
	}

	return config, nil
}

// AddVLANTranslation adds a VLAN translation rule.
func (d *HuaweiCLIDriver) AddVLANTranslation(ctx context.Context, ponPort string, onuID int, translation cli.VLANTranslation) error {
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	cmd = fmt.Sprintf("ont port vlan %d eth 1 translation %d to %d",
		onuID, translation.CustomerVLAN, translation.ServiceVLAN)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to add VLAN translation: %w", err)
	}

	if _, err := d.Execute(ctx, "quit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// RemoveVLANTranslation removes a VLAN translation rule.
func (d *HuaweiCLIDriver) RemoveVLANTranslation(ctx context.Context, ponPort string, onuID int, customerVLAN int) error {
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	cmd = fmt.Sprintf("undo ont port vlan %d eth 1 translation %d", onuID, customerVLAN)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to remove VLAN translation: %w", err)
	}

	if _, err := d.Execute(ctx, "quit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// ListVLANs lists all VLANs on the device.
func (d *HuaweiCLIDriver) ListVLANs(ctx context.Context) ([]cli.VLANInfo, error) {
	output, err := d.Execute(ctx, "display vlan all")
	if err != nil {
		return nil, fmt.Errorf("failed to list VLANs: %w", err)
	}

	var vlans []cli.VLANInfo
	vlanRegex := regexp.MustCompile(`(?m)^\s*(\d+)\s+(\S+)?\s*`)
	matches := vlanRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		if len(m) >= 2 {
			vlanID, err := strconv.Atoi(m[1])
			if err != nil || vlanID == 0 {
				continue
			}
			vlan := cli.VLANInfo{ID: vlanID}
			if len(m) >= 3 {
				vlan.Name = m[2]
			}
			vlans = append(vlans, vlan)
		}
	}

	return vlans, nil
}

// =============================================================================
// Profile Management
// =============================================================================

// ListLineProfiles lists all line profiles.
func (d *HuaweiCLIDriver) ListLineProfiles(ctx context.Context) ([]cli.LineProfile, error) {
	output, err := d.Execute(ctx, "display ont-lineprofile gpon all")
	if err != nil {
		return nil, fmt.Errorf("failed to list line profiles: %w", err)
	}

	var profiles []cli.LineProfile
	profileRegex := regexp.MustCompile(`(?m)^\s*(\d+)\s+(\S+)\s+(\S+)`)
	matches := profileRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		if len(m) >= 4 {
			id, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			profiles = append(profiles, cli.LineProfile{
				ID:   id,
				Name: m[2],
				Type: "gpon",
			})
		}
	}

	return profiles, nil
}

// GetLineProfile retrieves a specific line profile.
func (d *HuaweiCLIDriver) GetLineProfile(ctx context.Context, profileID int) (*cli.LineProfile, error) {
	cmd := fmt.Sprintf("display ont-lineprofile gpon profile-id %d", profileID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get line profile: %w", err)
	}

	profile := &cli.LineProfile{
		ID:   profileID,
		Type: "gpon",
	}

	// Parse profile name
	nameRegex := regexp.MustCompile(`(?i)profile[- ]name\s*:\s*(\S+)`)
	if matches := nameRegex.FindStringSubmatch(output); len(matches) > 1 {
		profile.Name = matches[1]
	}

	// Parse mapping mode
	modeRegex := regexp.MustCompile(`(?i)mapping[- ]mode\s*:\s*(\S+)`)
	if matches := modeRegex.FindStringSubmatch(output); len(matches) > 1 {
		profile.MappingMode = matches[1]
	}

	return profile, nil
}

// ListServiceProfiles lists all service profiles.
func (d *HuaweiCLIDriver) ListServiceProfiles(ctx context.Context) ([]cli.ServiceProfile, error) {
	output, err := d.Execute(ctx, "display ont-srvprofile gpon all")
	if err != nil {
		return nil, fmt.Errorf("failed to list service profiles: %w", err)
	}

	var profiles []cli.ServiceProfile
	profileRegex := regexp.MustCompile(`(?m)^\s*(\d+)\s+(\S+)`)
	matches := profileRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		if len(m) >= 3 {
			id, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			profiles = append(profiles, cli.ServiceProfile{
				ID:   id,
				Name: m[2],
			})
		}
	}

	return profiles, nil
}

// GetServiceProfile retrieves a specific service profile.
func (d *HuaweiCLIDriver) GetServiceProfile(ctx context.Context, profileID int) (*cli.ServiceProfile, error) {
	cmd := fmt.Sprintf("display ont-srvprofile gpon profile-id %d", profileID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get service profile: %w", err)
	}

	profile := &cli.ServiceProfile{
		ID: profileID,
	}

	// Parse profile name
	nameRegex := regexp.MustCompile(`(?i)profile[- ]name\s*:\s*(\S+)`)
	if matches := nameRegex.FindStringSubmatch(output); len(matches) > 1 {
		profile.Name = matches[1]
	}

	// Parse ETH ports
	ethRegex := regexp.MustCompile(`(?i)eth[- ]port[- ]number\s*:\s*(\d+)`)
	if matches := ethRegex.FindStringSubmatch(output); len(matches) > 1 {
		if ports, err := strconv.Atoi(matches[1]); err == nil {
			profile.ETHPorts = ports
		}
	}

	return profile, nil
}

// ListTrafficProfiles lists all traffic/DBA profiles.
func (d *HuaweiCLIDriver) ListTrafficProfiles(ctx context.Context) ([]cli.TrafficProfile, error) {
	output, err := d.Execute(ctx, "display dba-profile all")
	if err != nil {
		return nil, fmt.Errorf("failed to list traffic profiles: %w", err)
	}

	var profiles []cli.TrafficProfile
	profileRegex := regexp.MustCompile(`(?m)^\s*(\d+)\s+(\S+)\s+(\S+)`)
	matches := profileRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		if len(m) >= 4 {
			id, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			profiles = append(profiles, cli.TrafficProfile{
				ID:   id,
				Name: m[2],
				Type: m[3],
			})
		}
	}

	return profiles, nil
}

// AssignTrafficProfile assigns a DBA profile to an ONT.
func (d *HuaweiCLIDriver) AssignTrafficProfile(ctx context.Context, ponPort string, onuID int, profileID int) error {
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	cmd = fmt.Sprintf("ont modify %d dba-profile-id %d", onuID, profileID)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to assign traffic profile: %w", err)
	}

	if _, err := d.Execute(ctx, "quit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// =============================================================================
// Port Control
// =============================================================================

// ListPONPorts lists all PON ports on the device.
func (d *HuaweiCLIDriver) ListPONPorts(ctx context.Context) ([]cli.PONPortInfo, error) {
	output, err := d.Execute(ctx, "display board 0")
	if err != nil {
		return nil, fmt.Errorf("failed to list boards: %w", err)
	}

	var ports []cli.PONPortInfo
	// Parse board info to find GPON ports
	portRegex := regexp.MustCompile(`(?i)(\d+)\s+GPON\s+(\S+)`)
	matches := portRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		if len(m) >= 3 {
			slot, _ := strconv.Atoi(m[1])
			for port := 0; port < 16; port++ {
				ports = append(ports, cli.PONPortInfo{
					Slot:   slot,
					Port:   port,
					Name:   fmt.Sprintf("0/%d/%d", slot, port),
					Type:   "gpon",
					Status: m[2],
				})
			}
		}
	}

	return ports, nil
}

// GetPONPortInfo retrieves information about a specific PON port.
func (d *HuaweiCLIDriver) GetPONPortInfo(ctx context.Context, slot, port int) (*cli.PONPortInfo, error) {
	cmd := fmt.Sprintf("display port state 0/%d/%d", slot, port)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get port info: %w", err)
	}

	info := &cli.PONPortInfo{
		Slot: slot,
		Port: port,
		Name: fmt.Sprintf("0/%d/%d", slot, port),
		Type: "gpon",
	}

	// Parse port status
	statusRegex := regexp.MustCompile(`(?i)port[- ]state\s*:\s*(\S+)`)
	if matches := statusRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.Status = strings.ToLower(matches[1])
	}

	// Parse ONU count
	onuRegex := regexp.MustCompile(`(?i)ont[- ]number\s*:\s*(\d+)`)
	if matches := onuRegex.FindStringSubmatch(output); len(matches) > 1 {
		if count, err := strconv.Atoi(matches[1]); err == nil {
			info.ONUCount = count
		}
	}

	return info, nil
}

// EnablePONPort enables a PON port.
func (d *HuaweiCLIDriver) EnablePONPort(ctx context.Context, slot, port int) error {
	cmd := fmt.Sprintf("interface gpon 0/%d", slot)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	cmd = fmt.Sprintf("port %d ont-auto-find enable", port)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enable port: %w", err)
	}

	if _, err := d.Execute(ctx, "quit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// DisablePONPort disables a PON port.
func (d *HuaweiCLIDriver) DisablePONPort(ctx context.Context, slot, port int) error {
	cmd := fmt.Sprintf("interface gpon 0/%d", slot)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	cmd = fmt.Sprintf("port %d ont-auto-find disable", port)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to disable port: %w", err)
	}

	if _, err := d.Execute(ctx, "quit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// SetPortDescription sets the description for a port.
func (d *HuaweiCLIDriver) SetPortDescription(ctx context.Context, slot, port int, description string) error {
	cmd := fmt.Sprintf("interface gpon 0/%d", slot)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	cmd = fmt.Sprintf("port %d description \"%s\"", port, description)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to set description: %w", err)
	}

	if _, err := d.Execute(ctx, "quit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// =============================================================================
// Batch Operations
// =============================================================================

// BatchProvision provisions multiple ONTs in one operation.
func (d *HuaweiCLIDriver) BatchProvision(ctx context.Context, req *cli.BatchProvisionRequest) (*cli.BatchResult, error) {
	start := time.Now()
	result := &cli.BatchResult{
		TotalCount: len(req.ONUs),
		Results:    make([]cli.BatchItemResult, 0, len(req.ONUs)),
	}

	for i, onu := range req.ONUs {
		// Apply defaults if not specified
		if onu.LineProfile == "" && req.DefaultLineProfile != "" {
			onu.LineProfile = req.DefaultLineProfile
		}
		if onu.ServiceProfile == "" && req.DefaultServiceProfile != "" {
			onu.ServiceProfile = req.DefaultServiceProfile
		}
		if onu.NativeVLAN == 0 && req.DefaultVLAN > 0 {
			onu.NativeVLAN = req.DefaultVLAN
		}

		itemResult := cli.BatchItemResult{
			Index:      i,
			Identifier: onu.SerialNumber,
		}

		err := d.AddONU(ctx, &onu)
		if err != nil {
			itemResult.Success = false
			itemResult.Error = err.Error()
			result.FailedCount++

			if req.StopOnError {
				result.Results = append(result.Results, itemResult)
				break
			}
		} else {
			itemResult.Success = true
			result.SuccessCount++
		}

		result.Results = append(result.Results, itemResult)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// BatchConfigureVLAN configures VLANs for multiple ONTs.
func (d *HuaweiCLIDriver) BatchConfigureVLAN(ctx context.Context, req *cli.BatchVLANRequest) (*cli.BatchResult, error) {
	start := time.Now()
	result := &cli.BatchResult{
		TotalCount: len(req.Assignments),
		Results:    make([]cli.BatchItemResult, 0, len(req.Assignments)),
	}

	for i, config := range req.Assignments {
		itemResult := cli.BatchItemResult{
			Index:      i,
			Identifier: fmt.Sprintf("%s/%d", config.PonPort, config.OnuID),
		}

		err := d.ConfigureVLAN(ctx, &config)
		if err != nil {
			itemResult.Success = false
			itemResult.Error = err.Error()
			result.FailedCount++

			if req.StopOnError {
				result.Results = append(result.Results, itemResult)
				break
			}
		} else {
			itemResult.Success = true
			result.SuccessCount++
		}

		result.Results = append(result.Results, itemResult)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// ExportConfig exports the current device configuration.
func (d *HuaweiCLIDriver) ExportConfig(ctx context.Context) (*cli.ConfigExport, error) {
	export := &cli.ConfigExport{
		Timestamp:  time.Now(),
		DeviceHost: d.Config().Host,
		DeviceType: "huawei",
	}

	// Export line profiles
	lineProfiles, err := d.ListLineProfiles(ctx)
	if err == nil {
		export.Profiles.LineProfiles = lineProfiles
	}

	// Export service profiles
	serviceProfiles, err := d.ListServiceProfiles(ctx)
	if err == nil {
		export.Profiles.ServiceProfiles = serviceProfiles
	}

	// Export VLANs
	vlans, err := d.ListVLANs(ctx)
	if err == nil {
		export.VLANs = vlans
	}

	return export, nil
}

// =============================================================================
// Diagnostics
// =============================================================================

// GetONUDiagnostics retrieves comprehensive diagnostics for an ONT.
func (d *HuaweiCLIDriver) GetONUDiagnostics(ctx context.Context, ponPort string, onuID int) (*cli.ONUDiagnostics, error) {
	diag := &cli.ONUDiagnostics{
		PonPort:     ponPort,
		OnuID:       onuID,
		LastUpdated: time.Now(),
	}

	// Get basic info
	info, err := d.GetONUInfo(ctx, ponPort, onuID)
	if err == nil && info != nil {
		diag.SerialNumber = info.SerialNumber
		diag.Status = info.Status
		diag.Connectivity.Distance = info.Distance
		diag.Connectivity.OfflineReason = info.OfflineReason
	}

	// Get optical diagnostics
	optical, err := d.GetOpticalDiagnostics(ctx, ponPort, onuID)
	if err == nil && optical != nil {
		diag.Optical = *optical
	}

	// Get performance counters
	counters, err := d.GetONUCounters(ctx, ponPort, onuID)
	if err == nil && counters != nil {
		diag.Counters = *counters
	}

	// Get device health
	cmd := fmt.Sprintf("display ont version 0/%s %d", ponPort, onuID)
	output, err := d.Execute(ctx, cmd)
	if err == nil {
		// Parse firmware version
		fwRegex := regexp.MustCompile(`(?i)software[- ]version\s*:\s*(\S+)`)
		if matches := fwRegex.FindStringSubmatch(output); len(matches) > 1 {
			diag.Health.FirmwareVer = matches[1]
		}
		// Parse hardware version
		hwRegex := regexp.MustCompile(`(?i)hardware[- ]version\s*:\s*(\S+)`)
		if matches := hwRegex.FindStringSubmatch(output); len(matches) > 1 {
			diag.Health.HardwareVer = matches[1]
		}
	}

	return diag, nil
}

// GetONUCounters retrieves performance counters for an ONT.
func (d *HuaweiCLIDriver) GetONUCounters(ctx context.Context, ponPort string, onuID int) (*cli.PerformanceCounters, error) {
	cmd := fmt.Sprintf("display ont traffic 0/%s %d", ponPort, onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get counters: %w", err)
	}

	counters := &cli.PerformanceCounters{}

	// Parse RX bytes
	rxBytesRegex := regexp.MustCompile(`(?i)rx[- ]bytes\s*:\s*(\d+)`)
	if matches := rxBytesRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.RxBytes, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse TX bytes
	txBytesRegex := regexp.MustCompile(`(?i)tx[- ]bytes\s*:\s*(\d+)`)
	if matches := txBytesRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.TxBytes, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse RX packets
	rxPktsRegex := regexp.MustCompile(`(?i)rx[- ]packets\s*:\s*(\d+)`)
	if matches := rxPktsRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.RxPackets, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse TX packets
	txPktsRegex := regexp.MustCompile(`(?i)tx[- ]packets\s*:\s*(\d+)`)
	if matches := txPktsRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.TxPackets, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse errors
	errRegex := regexp.MustCompile(`(?i)(?:crc|fcs)[- ]errors\s*:\s*(\d+)`)
	if matches := errRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.CRCErrors, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	return counters, nil
}

// ClearONUCounters clears/resets performance counters for an ONT.
func (d *HuaweiCLIDriver) ClearONUCounters(ctx context.Context, ponPort string, onuID int) error {
	cmd := fmt.Sprintf("clear ont statistics 0/%s %d", ponPort, onuID)
	_, err := d.Execute(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to clear counters: %w", err)
	}
	return nil
}

// GetOpticalDiagnostics retrieves optical power readings for an ONT.
func (d *HuaweiCLIDriver) GetOpticalDiagnostics(ctx context.Context, ponPort string, onuID int) (*cli.OpticalDiagnostics, error) {
	cmd := fmt.Sprintf("display ont optical-info 0/%s %d", ponPort, onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get optical info: %w", err)
	}

	optical := &cli.OpticalDiagnostics{}

	// Parse RX power
	rxRegex := regexp.MustCompile(`(?i)(?:rx|receive)[- ](?:optical[- ])?power\s*\(?dBm?\)?\s*:\s*([-\d.]+)`)
	if matches := rxRegex.FindStringSubmatch(output); len(matches) > 1 {
		optical.RxPower, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse TX power
	txRegex := regexp.MustCompile(`(?i)(?:tx|transmit)[- ](?:optical[- ])?power\s*\(?dBm?\)?\s*:\s*([-\d.]+)`)
	if matches := txRegex.FindStringSubmatch(output); len(matches) > 1 {
		optical.TxPower, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse OLT RX power
	oltRxRegex := regexp.MustCompile(`(?i)OLT[- ]rx[- ](?:optical[- ])?power\s*\(?dBm?\)?\s*:\s*([-\d.]+)`)
	if matches := oltRxRegex.FindStringSubmatch(output); len(matches) > 1 {
		optical.OltRxPower, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse temperature
	tempRegex := regexp.MustCompile(`(?i)temperature\s*\(?[°cC]?\)?\s*:\s*([-\d.]+)`)
	if matches := tempRegex.FindStringSubmatch(output); len(matches) > 1 {
		optical.Temperature, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse voltage
	voltRegex := regexp.MustCompile(`(?i)voltage\s*\(?V?\)?\s*:\s*([-\d.]+)`)
	if matches := voltRegex.FindStringSubmatch(output); len(matches) > 1 {
		optical.Voltage, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse bias current
	biasRegex := regexp.MustCompile(`(?i)bias[- ]current\s*\(?mA?\)?\s*:\s*([-\d.]+)`)
	if matches := biasRegex.FindStringSubmatch(output); len(matches) > 1 {
		optical.BiasCurrent, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Evaluate status
	optical.RxPowerStatus = evaluateOpticalPowerStatus(optical.RxPower)
	optical.TxPowerStatus = evaluateOpticalPowerStatus(optical.TxPower)

	return optical, nil
}

// SaveConfig saves the running configuration.
func (d *HuaweiCLIDriver) SaveConfig(ctx context.Context) error {
	output, err := d.Execute(ctx, "save")
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Confirm save if prompted
	if strings.Contains(output, "Are you sure") || strings.Contains(output, "Y/N") {
		if _, err := d.Execute(ctx, "y"); err != nil {
			return fmt.Errorf("failed to confirm save: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Helper Functions
// =============================================================================

func parseHuaweiONTInfo(output string, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
	info := &cli.ONUCLIInfo{
		PonPort: ponPort,
		OnuID:   onuID,
	}

	// Parse serial number
	snRegex := regexp.MustCompile(`(?i)SN\s*:\s*(\S+)`)
	if matches := snRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.SerialNumber = matches[1]
	}

	// Parse run state/status
	statusRegex := regexp.MustCompile(`(?i)Run\s+state\s*:\s*(\S+)`)
	if matches := statusRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.Status = strings.ToLower(matches[1])
	}

	// Parse description
	descRegex := regexp.MustCompile(`(?i)Description\s*:\s*(.+)`)
	if matches := descRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.Description = strings.TrimSpace(matches[1])
	}

	// Parse distance
	distRegex := regexp.MustCompile(`(?i)Distance\s*\(?m?\)?\s*:\s*(\d+)`)
	if matches := distRegex.FindStringSubmatch(output); len(matches) > 1 {
		if d, err := strconv.Atoi(matches[1]); err == nil {
			info.Distance = d
		}
	}

	// Parse line profile
	lpRegex := regexp.MustCompile(`(?i)Line\s+profile\s+id\s*:\s*(\d+)`)
	if matches := lpRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.LineProfile = matches[1]
	}

	// Parse service profile
	spRegex := regexp.MustCompile(`(?i)Service\s+profile\s+id\s*:\s*(\d+)`)
	if matches := spRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.ServiceProfile = matches[1]
	}

	// Parse RX power
	rxRegex := regexp.MustCompile(`(?i)ONU\s+RX\s+optical\s+power\s*\(?dBm?\)?\s*:\s*([-\d.]+)`)
	if matches := rxRegex.FindStringSubmatch(output); len(matches) > 1 {
		if rx, err := strconv.ParseFloat(matches[1], 64); err == nil {
			info.RxPower = rx
		}
	}

	// Parse offline reason
	offlineRegex := regexp.MustCompile(`(?i)Last\s+down\s+cause\s*:\s*(.+)`)
	if matches := offlineRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.OfflineReason = strings.TrimSpace(matches[1])
	}

	return info, nil
}

func evaluateOpticalPowerStatus(power float64) string {
	if power < -30.0 {
		return "critical"
	} else if power < -27.0 {
		return "warning"
	} else if power > -5.0 {
		return "critical"
	} else if power > -8.0 {
		return "warning"
	}
	return "normal"
}
