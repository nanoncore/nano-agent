package vsol

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
)

// VSOLCLIDriver implements CLI operations for V-Sol OLTs.
type VSOLCLIDriver struct {
	*cli.BaseCLIDriver
	model        string
	capabilities *cli.VendorCapabilities
}

// NewVSOLCLIDriver creates a new V-Sol CLI driver.
func NewVSOLCLIDriver(config cli.CLIConfig) *VSOLCLIDriver {
	return NewVSOLCLIDriverWithModel(config, "")
}

// NewVSOLCLIDriverWithModel creates a new V-Sol CLI driver with model-specific capabilities.
func NewVSOLCLIDriverWithModel(config cli.CLIConfig, model string) *VSOLCLIDriver {
	return &VSOLCLIDriver{
		BaseCLIDriver: cli.NewBaseCLIDriver(config),
		model:         model,
		capabilities:  cli.GetCapabilities("vsol", model),
	}
}

// Vendor returns the vendor type.
func (d *VSOLCLIDriver) Vendor() string {
	return "vsol"
}

// GetCapabilities returns the feature support matrix for this driver.
func (d *VSOLCLIDriver) GetCapabilities() *cli.VendorCapabilities {
	return d.capabilities
}

// Model returns the OLT model.
func (d *VSOLCLIDriver) Model() string {
	return d.model
}

// Connect establishes connection and enters config mode.
func (d *VSOLCLIDriver) Connect(ctx context.Context) error {
	if err := d.BaseCLIDriver.Connect(ctx); err != nil {
		return err
	}

	// V-Sol requires enable with password prompt
	// Use the special method that handles Password: prompt
	if _, err := d.ExecuteEnableWithPassword(ctx, d.Config().Password); err != nil {
		return fmt.Errorf("failed to enter enable mode: %w", err)
	}

	// Now that we're in privileged mode, disable pager
	// V-SOL requires privileged mode before terminal length 0 can be used
	if err := d.DisablePager(); err != nil {
		// Non-fatal, but log it
		_ = err // Ignore pager disable errors
	}

	// Enter global config mode
	if _, err := d.Execute(ctx, "configure terminal"); err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}

	return nil
}

// AddONU provisions a new ONU on V-Sol OLT.
// V-SOL CLI syntax: onu add <onu_id> profile <profile> sn <serial>
func (d *VSOLCLIDriver) AddONU(ctx context.Context, req *cli.ONUProvisionRequest) error {
	if req.SerialNumber == "" {
		return fmt.Errorf("serial number is required")
	}

	// Enter GPON interface
	// V-Sol GPON OLTs use "interface gpon X/Y" format
	cmd := fmt.Sprintf("interface gpon %s", req.PonPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	// Build ONU add command - V-SOL syntax: onu add <id> profile <profile> sn <serial>
	profile := req.LineProfile
	if profile == "" {
		profile = "AN5506-04-F1" // Use common V-SOL profile if none specified
	}

	cmd = fmt.Sprintf("onu add %d profile %s sn %s", req.OnuID, profile, req.SerialNumber)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to add ONU: %w", err)
	}

	// Check for error indicators - V-SOL uses various error messages
	outputLower := strings.ToLower(output)
	if strings.Contains(outputLower, "error") ||
		strings.Contains(outputLower, "fail") ||
		strings.Contains(outputLower, "isn't existed") ||
		strings.Contains(outputLower, "not exist") {
		return fmt.Errorf("ONU add failed: %s", output)
	}

	// Configure description if provided
	if req.Description != "" {
		cmd = fmt.Sprintf("onu %d description %s", req.OnuID, req.Description)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to set description: %w", err)
		}
	}

	// Configure native VLAN if specified - V-SOL requires service port configuration
	// This is handled separately with onu_service_port command if needed

	// Configure service ports
	for _, sp := range req.ServicePorts {
		cmd = fmt.Sprintf("onu %d service-port %d vlan %d", req.OnuID, sp.Index, sp.VLAN)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to configure service port: %w", err)
		}
	}

	// Exit interface
	if _, err := d.Execute(ctx, "exit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// DeleteONU removes an ONU from V-Sol OLT.
func (d *VSOLCLIDriver) DeleteONU(ctx context.Context, ponPort string, onuID int) error {
	// Enter GPON interface
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	// Delete ONU
	cmd = fmt.Sprintf("no onu %d", onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to delete ONU: %w", err)
	}

	// Ignore "not exist" errors
	if strings.Contains(strings.ToLower(output), "error") &&
		!strings.Contains(strings.ToLower(output), "not exist") &&
		!strings.Contains(strings.ToLower(output), "not found") {
		return fmt.Errorf("ONU delete failed: %s", output)
	}

	// Exit interface
	if _, err := d.Execute(ctx, "exit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// ONUBasicInfo holds minimal ONU information for duplicate detection.
type ONUBasicInfo struct {
	ID     int
	Serial string
}

// ListONUs returns a list of existing ONU IDs on a given PON port.
// This is used to find available ONU IDs for auto-assignment during provisioning.
func (d *VSOLCLIDriver) ListONUs(ctx context.Context, ponPort string) ([]int, error) {
	infos, err := d.ListONUsWithSerial(ctx, ponPort)
	if err != nil {
		return nil, err
	}
	ids := make([]int, len(infos))
	for i, info := range infos {
		ids[i] = info.ID
	}
	return ids, nil
}

// ListONUsWithSerial returns a list of existing ONUs with their IDs and serial numbers.
// This uses "show onu info all" which returns serial numbers in a single command.
func (d *VSOLCLIDriver) ListONUsWithSerial(ctx context.Context, ponPort string) ([]ONUBasicInfo, error) {
	serialMap, err := d.GetONUInfoAll(ctx, ponPort)
	if err != nil {
		return nil, err
	}

	result := make([]ONUBasicInfo, 0, len(serialMap))
	for id, serial := range serialMap {
		result = append(result, ONUBasicInfo{ID: id, Serial: serial})
	}
	return result, nil
}

// GetONUInfoAll returns a map of ONU ID to serial number for all ONUs on a port.
// This uses "show onu info all" which is more efficient than individual queries.
func (d *VSOLCLIDriver) GetONUInfoAll(ctx context.Context, ponPort string) (map[int]string, error) {
	// Enter GPON interface
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return nil, fmt.Errorf("failed to enter interface: %w", err)
	}

	// Get ONU info all - this includes serial numbers
	// Format: Onuindex   Model                Profile                Mode    AuthInfo
	//         GPON0/1:1  Generic-ONU          default                sn      VSOL00000001
	output, err := d.Execute(ctx, "show onu info all")
	if err != nil {
		d.Execute(ctx, "exit") // Try to exit interface
		return nil, fmt.Errorf("failed to get ONU info: %w", err)
	}

	// Exit interface
	d.Execute(ctx, "exit")

	// Parse ONU IDs and serials from output
	result := make(map[int]string)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Onuindex") || strings.HasPrefix(line, "-") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			// Parse Onuindex format: GPON0/X:Y where Y is the ONU ID
			onuIndex := fields[0]
			if colonIdx := strings.LastIndex(onuIndex, ":"); colonIdx != -1 {
				idStr := onuIndex[colonIdx+1:]
				if id, err := strconv.Atoi(idStr); err == nil && id > 0 && id <= 128 {
					// Serial is in the last column (AuthInfo)
					serial := ""
					if len(fields) >= 5 {
						serial = strings.ToUpper(fields[4])
					}
					result[id] = serial
				}
			}
		}
	}

	return result, nil
}

// GetONUInfo retrieves ONU information via CLI.
func (d *VSOLCLIDriver) GetONUInfo(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
	// Enter GPON interface first
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return nil, fmt.Errorf("failed to enter interface: %w", err)
	}
	defer d.Execute(ctx, "exit")

	// V-SOL uses "show onu <id> info" or "show onu info" in interface mode
	cmd = fmt.Sprintf("show onu %d detail-info", onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		// Try basic info
		cmd = fmt.Sprintf("show onu %d info", onuID)
		output, err = d.Execute(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("failed to get ONU info: %w", err)
		}
	}

	return parseVSOLONUInfo(output, ponPort, onuID)
}

// parseVSOLONUInfo parses V-Sol ONU info output.
// Handles output from "show onu <id> detail-info" which looks like:
//
//	---------onu 1 defail-info---------
//	Vendor ID:                              FHTT
//	SN:                                     FHTT5929e410
//	Operate status:                         enable
//	Equipment ID:                           HG6143D
//	Model:                                  HG6143D
func parseVSOLONUInfo(output string, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
	info := &cli.ONUCLIInfo{
		PonPort: ponPort,
		OnuID:   onuID,
	}

	// Parse serial number (SN field in V-SOL)
	snRegex := regexp.MustCompile(`(?i)(?:SN|Serial\s*[Nn]umber)\s*:[\s\x00-\x1F]*(\S+)`)
	if matches := snRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.SerialNumber = strings.ToUpper(matches[1])
	}

	// Parse MAC address
	macRegex := regexp.MustCompile(`(?i)MAC\s*[Aa]ddress\s*:[\s\x00-\x1F]*(\S+)`)
	if matches := macRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.MAC = matches[1]
	}

	// Parse status (Operate status in V-SOL)
	statusRegex := regexp.MustCompile(`(?i)(?:Operate\s*status|Status|Phase\s*State)\s*:[\s\x00-\x1F]*(\S+)`)
	if matches := statusRegex.FindStringSubmatch(output); len(matches) > 1 {
		status := strings.ToLower(matches[1])
		// Normalize status
		switch status {
		case "enable", "working":
			info.Status = "online"
		case "disable", "offline":
			info.Status = "offline"
		default:
			info.Status = status
		}
	}

	// Parse type/model (Equipment ID or Model in V-SOL)
	typeRegex := regexp.MustCompile(`(?i)(?:Equipment\s*ID|Model|Type)\s*:[\s\x00-\x1F]*(\S+)`)
	if matches := typeRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.Type = matches[1]
	}

	// Parse distance
	distRegex := regexp.MustCompile(`(?i)Distance\s*:[\s\x00-\x1F]*(\d+)`)
	if matches := distRegex.FindStringSubmatch(output); len(matches) > 1 {
		if d, err := strconv.Atoi(matches[1]); err == nil {
			info.Distance = d
		}
	}

	// Parse RX power (if present in output)
	rxRegex := regexp.MustCompile(`(?i)(?:RX|Receive)\s*[Pp]ower\s*:[\s\x00-\x1F]*([-\d.]+)`)
	if matches := rxRegex.FindStringSubmatch(output); len(matches) > 1 {
		if rx, err := strconv.ParseFloat(matches[1], 64); err == nil {
			info.RxPower = rx
		}
	}

	// Parse description
	descRegex := regexp.MustCompile(`(?i)Description\s*:[\s\x00-\x1F]*(.+)`)
	if matches := descRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.Description = strings.TrimSpace(matches[1])
	}

	return info, nil
}

// SaveConfig saves the running configuration.
func (d *VSOLCLIDriver) SaveConfig(ctx context.Context) error {
	output, err := d.Execute(ctx, "write memory")
	if err != nil {
		// Try alternate command
		output, err = d.Execute(ctx, "save")
		if err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	}

	if strings.Contains(strings.ToLower(output), "error") {
		return fmt.Errorf("save failed: %s", output)
	}

	return nil
}

// RebootONU reboots a specific ONU using deactivate/activate sequence.
// V-SOL GPON OLTs don't have a direct "onu reboot" command.
// Note: Connect() already enters config mode, so we go directly to interface.
func (d *VSOLCLIDriver) RebootONU(ctx context.Context, ponPort string, onuID int) error {
	// Enter GPON interface (already in config mode from Connect())
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	// Deactivate the ONU
	cmd = fmt.Sprintf("onu %d deactivate", onuID)
	if _, err := d.Execute(ctx, cmd); err != nil {
		// Try to exit gracefully (back to config mode)
		d.Execute(ctx, "exit")
		return fmt.Errorf("failed to deactivate ONU: %w", err)
	}

	// Wait for ONU to go offline
	time.Sleep(3 * time.Second)

	// Activate the ONU
	cmd = fmt.Sprintf("onu %d activate", onuID)
	if _, err := d.Execute(ctx, cmd); err != nil {
		// Try to exit gracefully (back to config mode)
		d.Execute(ctx, "exit")
		return fmt.Errorf("failed to activate ONU: %w", err)
	}

	// Exit interface mode (back to config mode)
	d.Execute(ctx, "exit")

	return nil
}

// =============================================================================
// VLAN Management
// =============================================================================

// ConfigureVLAN configures VLAN settings for an ONU.
func (d *VSOLCLIDriver) ConfigureVLAN(ctx context.Context, config *cli.VLANConfig) error {
	// Enter GPON interface
	cmd := fmt.Sprintf("interface gpon %s", config.PonPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	// Configure native VLAN (PVID)
	if config.NativeVLAN > 0 {
		cmd = fmt.Sprintf("onu %d vlan pvid %d", config.OnuID, config.NativeVLAN)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to configure native VLAN: %w", err)
		}
	}

	// Configure tagged VLANs
	for _, vlan := range config.TaggedVLANs {
		cmd = fmt.Sprintf("onu %d vlan tag %d", config.OnuID, vlan)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to add tagged VLAN %d: %w", vlan, err)
		}
	}

	// Configure service VLAN
	if config.ServiceVLAN > 0 {
		cmd = fmt.Sprintf("onu %d service-vlan %d", config.OnuID, config.ServiceVLAN)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to configure service VLAN: %w", err)
		}
	}

	// Configure VLAN translations
	for _, tr := range config.Translations {
		if err := d.addVLANTranslation(ctx, config.OnuID, tr); err != nil {
			return err
		}
	}

	if _, err := d.Execute(ctx, "exit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// addVLANTranslation adds a single VLAN translation rule (internal helper).
func (d *VSOLCLIDriver) addVLANTranslation(ctx context.Context, onuID int, tr cli.VLANTranslation) error {
	var cmd string
	switch tr.Mode {
	case "translate":
		cmd = fmt.Sprintf("onu %d vlan translate %d to %d", onuID, tr.CustomerVLAN, tr.ServiceVLAN)
	case "transparent":
		cmd = fmt.Sprintf("onu %d vlan transparent %d", onuID, tr.CustomerVLAN)
	case "tag":
		cmd = fmt.Sprintf("onu %d vlan tag %d service %d", onuID, tr.CustomerVLAN, tr.ServiceVLAN)
	default:
		cmd = fmt.Sprintf("onu %d vlan translate %d to %d", onuID, tr.CustomerVLAN, tr.ServiceVLAN)
	}

	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to add VLAN translation: %w", err)
	}
	return nil
}

// GetVLANConfig retrieves VLAN configuration for an ONU.
func (d *VSOLCLIDriver) GetVLANConfig(ctx context.Context, ponPort string, onuID int) (*cli.VLANConfig, error) {
	// Enter GPON interface first
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return nil, fmt.Errorf("failed to enter interface: %w", err)
	}
	defer d.Execute(ctx, "exit")

	cmd = fmt.Sprintf("show onu %d portvlan", onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get VLAN config: %w", err)
	}

	return parseVSOLVLANConfig(output, ponPort, onuID)
}

// parseVSOLVLANConfig parses V-Sol VLAN configuration output.
func parseVSOLVLANConfig(output string, ponPort string, onuID int) (*cli.VLANConfig, error) {
	config := &cli.VLANConfig{
		OnuID:   onuID,
		PonPort: ponPort,
	}

	// Parse PVID/Native VLAN
	pvidRegex := regexp.MustCompile(`(?i)PVID\s*:\s*(\d+)`)
	if matches := pvidRegex.FindStringSubmatch(output); len(matches) > 1 {
		if v, err := strconv.Atoi(matches[1]); err == nil {
			config.NativeVLAN = v
		}
	}

	// Parse tagged VLANs
	taggedRegex := regexp.MustCompile(`(?i)Tagged\s*VLAN[s]?\s*:\s*([\d,\s-]+)`)
	if matches := taggedRegex.FindStringSubmatch(output); len(matches) > 1 {
		config.TaggedVLANs = parseVLANList(matches[1])
	}

	// Parse service VLAN
	svcRegex := regexp.MustCompile(`(?i)Service\s*VLAN\s*:\s*(\d+)`)
	if matches := svcRegex.FindStringSubmatch(output); len(matches) > 1 {
		if v, err := strconv.Atoi(matches[1]); err == nil {
			config.ServiceVLAN = v
		}
	}

	// Parse VLAN translations
	transRegex := regexp.MustCompile(`(?i)(\d+)\s*->\s*(\d+)\s*\((\w+)\)`)
	transMatches := transRegex.FindAllStringSubmatch(output, -1)
	for _, m := range transMatches {
		if len(m) > 3 {
			cVlan, _ := strconv.Atoi(m[1])
			sVlan, _ := strconv.Atoi(m[2])
			config.Translations = append(config.Translations, cli.VLANTranslation{
				CustomerVLAN: cVlan,
				ServiceVLAN:  sVlan,
				Mode:         strings.ToLower(m[3]),
			})
		}
	}

	return config, nil
}

// parseVLANList parses a comma/dash separated VLAN list.
func parseVLANList(s string) []int {
	var vlans []int
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' '
	})

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			// Range: e.g., "100-105"
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) == 2 {
				start, _ := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
				end, _ := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
				for v := start; v <= end; v++ {
					vlans = append(vlans, v)
				}
			}
		} else {
			if v, err := strconv.Atoi(part); err == nil {
				vlans = append(vlans, v)
			}
		}
	}
	return vlans
}

// AddVLANTranslation adds a VLAN translation rule.
func (d *VSOLCLIDriver) AddVLANTranslation(ctx context.Context, ponPort string, onuID int, translation cli.VLANTranslation) error {
	// Enter GPON interface
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	if err := d.addVLANTranslation(ctx, onuID, translation); err != nil {
		return err
	}

	if _, err := d.Execute(ctx, "exit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// RemoveVLANTranslation removes a VLAN translation rule.
func (d *VSOLCLIDriver) RemoveVLANTranslation(ctx context.Context, ponPort string, onuID int, customerVLAN int) error {
	// Enter GPON interface
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	cmd = fmt.Sprintf("no onu %d vlan translate %d", onuID, customerVLAN)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to remove VLAN translation: %w", err)
	}

	if _, err := d.Execute(ctx, "exit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// ListVLANs lists all VLANs on the device.
func (d *VSOLCLIDriver) ListVLANs(ctx context.Context) ([]cli.VLANInfo, error) {
	output, err := d.Execute(ctx, "show vlan all")
	if err != nil {
		return nil, fmt.Errorf("failed to list VLANs: %w", err)
	}

	return parseVSOLVLANList(output)
}

// parseVSOLVLANList parses V-Sol VLAN list output.
// V-Sol format is a space-separated list of VLAN IDs:
//
//	Created VLANs:
//	            1   701   702
func parseVSOLVLANList(output string) ([]cli.VLANInfo, error) {
	var vlans []cli.VLANInfo

	// Extract all numbers from the output as VLAN IDs
	// The format is: "Created VLANs:\n            1   701   702"
	vlanIDRegex := regexp.MustCompile(`\b(\d+)\b`)
	matches := vlanIDRegex.FindAllStringSubmatch(output, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		id, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		// V-Sol auto-generates VLAN names as "vlan<id>"
		vlans = append(vlans, cli.VLANInfo{
			ID:   id,
			Name: fmt.Sprintf("vlan%d", id),
		})
	}

	return vlans, nil
}

// =============================================================================
// Profile Management
// =============================================================================

// ListLineProfiles lists all line profiles.
func (d *VSOLCLIDriver) ListLineProfiles(ctx context.Context) ([]cli.LineProfile, error) {
	output, err := d.Execute(ctx, "show pon-profile line all")
	if err != nil {
		return nil, fmt.Errorf("failed to list line profiles: %w", err)
	}

	return parseVSOLLineProfiles(output)
}

// parseVSOLLineProfiles parses V-Sol line profile list.
func parseVSOLLineProfiles(output string) ([]cli.LineProfile, error) {
	var profiles []cli.LineProfile

	// Profile ID    Profile Name           Type
	// ----------    ------------           ----
	// 1             default                gpon
	profileRegex := regexp.MustCompile(`^\s*(\d+)\s+(\S+)\s+(\S+)`)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if matches := profileRegex.FindStringSubmatch(line); len(matches) > 3 {
			id, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			profiles = append(profiles, cli.LineProfile{
				ID:   id,
				Name: matches[2],
				Type: matches[3],
			})
		}
	}

	return profiles, nil
}

// GetLineProfile retrieves a specific line profile.
func (d *VSOLCLIDriver) GetLineProfile(ctx context.Context, profileID int) (*cli.LineProfile, error) {
	cmd := fmt.Sprintf("show pon-profile line %d", profileID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get line profile: %w", err)
	}

	profile := &cli.LineProfile{ID: profileID}

	// Parse profile name
	nameRegex := regexp.MustCompile(`(?i)Name\s*:\s*(\S+)`)
	if matches := nameRegex.FindStringSubmatch(output); len(matches) > 1 {
		profile.Name = matches[1]
	}

	// Parse type
	typeRegex := regexp.MustCompile(`(?i)Type\s*:\s*(\S+)`)
	if matches := typeRegex.FindStringSubmatch(output); len(matches) > 1 {
		profile.Type = matches[1]
	}

	// Parse mapping mode
	mappingRegex := regexp.MustCompile(`(?i)Mapping\s*[Mm]ode\s*:\s*(\S+)`)
	if matches := mappingRegex.FindStringSubmatch(output); len(matches) > 1 {
		profile.MappingMode = matches[1]
	}

	return profile, nil
}

// ListServiceProfiles lists all service profiles.
func (d *VSOLCLIDriver) ListServiceProfiles(ctx context.Context) ([]cli.ServiceProfile, error) {
	output, err := d.Execute(ctx, "show pon-profile service all")
	if err != nil {
		return nil, fmt.Errorf("failed to list service profiles: %w", err)
	}

	return parseVSOLServiceProfiles(output)
}

// parseVSOLServiceProfiles parses V-Sol service profile list.
func parseVSOLServiceProfiles(output string) ([]cli.ServiceProfile, error) {
	var profiles []cli.ServiceProfile

	profileRegex := regexp.MustCompile(`^\s*(\d+)\s+(\S+)\s+(\S+)?\s*(\d+)?`)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if matches := profileRegex.FindStringSubmatch(line); len(matches) > 2 {
			id, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			profile := cli.ServiceProfile{
				ID:   id,
				Name: matches[2],
			}
			if len(matches) > 3 {
				profile.ONUType = matches[3]
			}
			if len(matches) > 4 {
				profile.PortCount, _ = strconv.Atoi(matches[4])
			}
			profiles = append(profiles, profile)
		}
	}

	return profiles, nil
}

// GetServiceProfile retrieves a specific service profile.
func (d *VSOLCLIDriver) GetServiceProfile(ctx context.Context, profileID int) (*cli.ServiceProfile, error) {
	cmd := fmt.Sprintf("show pon-profile service %d", profileID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get service profile: %w", err)
	}

	profile := &cli.ServiceProfile{ID: profileID}

	// Parse profile name
	nameRegex := regexp.MustCompile(`(?i)Name\s*:\s*(\S+)`)
	if matches := nameRegex.FindStringSubmatch(output); len(matches) > 1 {
		profile.Name = matches[1]
	}

	// Parse ONU type
	typeRegex := regexp.MustCompile(`(?i)ONU\s*[Tt]ype\s*:\s*(\S+)`)
	if matches := typeRegex.FindStringSubmatch(output); len(matches) > 1 {
		profile.ONUType = matches[1]
	}

	// Parse port counts
	ethRegex := regexp.MustCompile(`(?i)ETH\s*[Pp]orts?\s*:\s*(\d+)`)
	if matches := ethRegex.FindStringSubmatch(output); len(matches) > 1 {
		profile.ETHPorts, _ = strconv.Atoi(matches[1])
	}

	potsRegex := regexp.MustCompile(`(?i)POTS\s*[Pp]orts?\s*:\s*(\d+)`)
	if matches := potsRegex.FindStringSubmatch(output); len(matches) > 1 {
		profile.POTSPorts, _ = strconv.Atoi(matches[1])
	}

	return profile, nil
}

// ListTrafficProfiles lists all traffic/bandwidth profiles.
func (d *VSOLCLIDriver) ListTrafficProfiles(ctx context.Context) ([]cli.TrafficProfile, error) {
	output, err := d.Execute(ctx, "show bandwidth-profile all")
	if err != nil {
		return nil, fmt.Errorf("failed to list traffic profiles: %w", err)
	}

	return parseVSOLTrafficProfiles(output)
}

// parseVSOLTrafficProfiles parses V-Sol traffic profile list.
func parseVSOLTrafficProfiles(output string) ([]cli.TrafficProfile, error) {
	var profiles []cli.TrafficProfile

	// Profile ID    Name           CIR(kbps)    PIR(kbps)
	profileRegex := regexp.MustCompile(`^\s*(\d+)\s+(\S+)\s+(\d+)\s+(\d+)`)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if matches := profileRegex.FindStringSubmatch(line); len(matches) > 4 {
			id, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			cir, _ := strconv.Atoi(matches[3])
			pir, _ := strconv.Atoi(matches[4])
			profiles = append(profiles, cli.TrafficProfile{
				ID:   id,
				Name: matches[2],
				CIR:  cir,
				PIR:  pir,
			})
		}
	}

	return profiles, nil
}

// AssignTrafficProfile assigns a traffic profile to an ONU.
func (d *VSOLCLIDriver) AssignTrafficProfile(ctx context.Context, ponPort string, onuID int, profileID int) error {
	// Enter GPON interface
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	cmd = fmt.Sprintf("onu %d bandwidth-profile %d", onuID, profileID)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to assign traffic profile: %w", err)
	}

	if _, err := d.Execute(ctx, "exit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// =============================================================================
// Port Control
// =============================================================================

// ListPONPorts lists all PON ports on the device.
// V-SOL OLTs don't have a command to list PON ports directly.
// We enumerate ports 0/1 through 0/8 by trying to enter each interface.
func (d *VSOLCLIDriver) ListPONPorts(ctx context.Context) ([]cli.PONPortInfo, error) {
	var ports []cli.PONPortInfo

	// V-SOL OLTs typically have up to 8 PON ports (0/1 through 0/8)
	for portNum := 1; portNum <= 8; portNum++ {
		portName := fmt.Sprintf("0/%d", portNum)

		// Try to enter the interface to check if it exists
		cmd := fmt.Sprintf("interface gpon %s", portName)
		output, err := d.Execute(ctx, cmd)
		if err != nil {
			continue // Port doesn't exist or error entering
		}

		// Check if the command was rejected (interface doesn't exist)
		outputLower := strings.ToLower(output)
		if strings.Contains(outputLower, "error") ||
			strings.Contains(outputLower, "unknown") ||
			strings.Contains(outputLower, "invalid") ||
			strings.Contains(outputLower, "parameter error") {
			continue // Interface doesn't exist
		}

		// We're now in interface mode - get ONU count from show onu info all
		onuCount := 0
		onuOutput, err := d.Execute(ctx, "show onu info all")
		if err == nil {
			onuCount = countONUsFromOutput(onuOutput)
		}

		// Exit the interface mode
		d.Execute(ctx, "exit")

		ports = append(ports, cli.PONPortInfo{
			Slot:        0,
			Port:        portNum,
			Name:        portName,
			Type:        "gpon",
			Status:      "up",
			AdminStatus: "enable",
			ONUCount:    onuCount,
		})
	}

	if len(ports) == 0 {
		return nil, fmt.Errorf("no PON ports found")
	}
	return ports, nil
}

// countONUsFromOutput counts ONUs from "show onu info all" output.
// V-SOL format: GPON0/X:Y where Y is the ONU ID
func countONUsFromOutput(output string) int {
	count := 0
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip header and separator lines
		if line == "" || strings.HasPrefix(line, "Onuindex") || strings.HasPrefix(line, "-") {
			continue
		}
		// Check if line starts with GPON (indicates an ONU entry)
		if strings.HasPrefix(line, "GPON") {
			count++
		}
	}
	return count
}

// parseVSOLPONPorts parses V-Sol PON port list.
func parseVSOLPONPorts(output string) ([]cli.PONPortInfo, error) {
	var ports []cli.PONPortInfo

	// Interface         Admin    Oper    ONUs    Description
	// epon 0/1          enable   up      32      Uplink-1
	portRegex := regexp.MustCompile(`(?i)(?:epon|gpon)\s+(\d+)/(\d+)\s+(\S+)\s+(\S+)\s+(\d+)(?:\s+(.*))?`)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if matches := portRegex.FindStringSubmatch(line); len(matches) > 5 {
			slot, _ := strconv.Atoi(matches[1])
			port, _ := strconv.Atoi(matches[2])
			onuCount, _ := strconv.Atoi(matches[5])

			portInfo := cli.PONPortInfo{
				Slot:        slot,
				Port:        port,
				Name:        fmt.Sprintf("%d/%d", slot, port),
				AdminStatus: strings.ToLower(matches[3]),
				Status:      strings.ToLower(matches[4]),
				ONUCount:    onuCount,
			}
			if len(matches) > 6 {
				portInfo.Description = strings.TrimSpace(matches[6])
			}

			// Determine type from output context
			if strings.Contains(strings.ToLower(output), "gpon") {
				portInfo.Type = "gpon"
			} else {
				portInfo.Type = "epon"
			}

			ports = append(ports, portInfo)
		}
	}

	return ports, nil
}

// GetPONPortInfo retrieves information about a specific PON port.
func (d *VSOLCLIDriver) GetPONPortInfo(ctx context.Context, slot, port int) (*cli.PONPortInfo, error) {
	// V-SOL uses "show pon info" in interface mode
	cmd := fmt.Sprintf("interface gpon %d/%d", slot, port)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return nil, fmt.Errorf("failed to enter interface: %w", err)
	}
	defer d.Execute(ctx, "exit")

	output, err := d.Execute(ctx, "show pon info")
	if err != nil {
		return nil, fmt.Errorf("failed to get PON port info: %w", err)
	}

	return parseVSOLPONPortInfo(output, slot, port)
}

// parseVSOLPONPortInfo parses detailed V-Sol PON port info.
func parseVSOLPONPortInfo(output string, slot, port int) (*cli.PONPortInfo, error) {
	info := &cli.PONPortInfo{
		Slot: slot,
		Port: port,
		Name: fmt.Sprintf("%d/%d", slot, port),
	}

	// Parse admin status
	adminRegex := regexp.MustCompile(`(?i)Admin\s*[Ss]tatus\s*:\s*(\S+)`)
	if matches := adminRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.AdminStatus = strings.ToLower(matches[1])
	}

	// Parse oper status
	operRegex := regexp.MustCompile(`(?i)(?:Oper|Link)\s*[Ss]tatus\s*:\s*(\S+)`)
	if matches := operRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.Status = strings.ToLower(matches[1])
	}

	// Parse ONU count
	onuRegex := regexp.MustCompile(`(?i)ONU\s*[Cc]ount\s*:\s*(\d+)`)
	if matches := onuRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.ONUCount, _ = strconv.Atoi(matches[1])
	}

	// Parse max ONUs
	maxRegex := regexp.MustCompile(`(?i)Max\s*ONU[s]?\s*:\s*(\d+)`)
	if matches := maxRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.MaxONUs, _ = strconv.Atoi(matches[1])
	}

	// Parse TX power
	txRegex := regexp.MustCompile(`(?i)TX\s*[Pp]ower\s*:\s*([-\d.]+)`)
	if matches := txRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.TxPower, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse description
	descRegex := regexp.MustCompile(`(?i)Description\s*:\s*(.+)`)
	if matches := descRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.Description = strings.TrimSpace(matches[1])
	}

	// Parse type
	if strings.Contains(strings.ToLower(output), "gpon") {
		info.Type = "gpon"
	} else if strings.Contains(strings.ToLower(output), "epon") {
		info.Type = "epon"
	}

	return info, nil
}

// EnablePONPort enables a PON port.
func (d *VSOLCLIDriver) EnablePONPort(ctx context.Context, slot, port int) error {
	// Enter GPON interface
	cmd := fmt.Sprintf("interface gpon %d/%d", slot, port)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	if _, err := d.Execute(ctx, "no shutdown"); err != nil {
		return fmt.Errorf("failed to enable port: %w", err)
	}

	if _, err := d.Execute(ctx, "exit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// DisablePONPort disables a PON port.
func (d *VSOLCLIDriver) DisablePONPort(ctx context.Context, slot, port int) error {
	// Enter GPON interface
	cmd := fmt.Sprintf("interface gpon %d/%d", slot, port)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	if _, err := d.Execute(ctx, "shutdown"); err != nil {
		return fmt.Errorf("failed to disable port: %w", err)
	}

	if _, err := d.Execute(ctx, "exit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// SetPortDescription sets the description for a port.
func (d *VSOLCLIDriver) SetPortDescription(ctx context.Context, slot, port int, description string) error {
	// Enter GPON interface
	cmd := fmt.Sprintf("interface gpon %d/%d", slot, port)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}

	cmd = fmt.Sprintf("description %s", description)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to set description: %w", err)
	}

	if _, err := d.Execute(ctx, "exit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// =============================================================================
// Batch Operations
// =============================================================================

// BatchProvision provisions multiple ONUs in one operation.
func (d *VSOLCLIDriver) BatchProvision(ctx context.Context, req *cli.BatchProvisionRequest) (*cli.BatchResult, error) {
	start := time.Now()
	result := &cli.BatchResult{
		TotalCount: len(req.ONUs),
		Results:    make([]cli.BatchItemResult, 0, len(req.ONUs)),
	}

	for i, onu := range req.ONUs {
		itemResult := cli.BatchItemResult{
			Index:      i,
			Identifier: onu.SerialNumber,
		}

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

// BatchConfigureVLAN configures VLANs for multiple ONUs.
func (d *VSOLCLIDriver) BatchConfigureVLAN(ctx context.Context, req *cli.BatchVLANRequest) (*cli.BatchResult, error) {
	start := time.Now()
	result := &cli.BatchResult{
		TotalCount: len(req.Assignments),
		Results:    make([]cli.BatchItemResult, 0, len(req.Assignments)),
	}

	for i, assignment := range req.Assignments {
		itemResult := cli.BatchItemResult{
			Index:      i,
			Identifier: fmt.Sprintf("%s/%d", assignment.PonPort, assignment.OnuID),
		}

		err := d.ConfigureVLAN(ctx, &assignment)
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
func (d *VSOLCLIDriver) ExportConfig(ctx context.Context) (*cli.ConfigExport, error) {
	export := &cli.ConfigExport{
		Timestamp:  time.Now(),
		DeviceHost: d.Config().Host,
		DeviceType: "vsol",
	}

	// Collect PON ports info
	ports, err := d.ListPONPorts(ctx)
	if err == nil {
		// Collect ONU info for each port
		for _, port := range ports {
			for onuID := 1; onuID <= port.ONUCount && onuID <= 128; onuID++ {
				info, err := d.GetONUInfo(ctx, port.Name, onuID)
				if err != nil {
					continue
				}
				if info.SerialNumber == "" {
					continue
				}

				req := cli.ONUProvisionRequest{
					PonPort:      info.PonPort,
					OnuID:        info.OnuID,
					SerialNumber: info.SerialNumber,
					Type:         info.Type,
					Description:  info.Description,
				}
				export.ONUs = append(export.ONUs, req)
			}
		}
	}

	// Collect VLANs
	vlans, err := d.ListVLANs(ctx)
	if err == nil {
		export.VLANs = vlans
	}

	// Collect profiles
	lineProfiles, err := d.ListLineProfiles(ctx)
	if err == nil {
		export.Profiles.LineProfiles = lineProfiles
	}

	serviceProfiles, err := d.ListServiceProfiles(ctx)
	if err == nil {
		export.Profiles.ServiceProfiles = serviceProfiles
	}

	trafficProfiles, err := d.ListTrafficProfiles(ctx)
	if err == nil {
		export.Profiles.TrafficProfiles = trafficProfiles
	}

	return export, nil
}

// =============================================================================
// Diagnostics
// =============================================================================

// GetONUDiagnostics retrieves comprehensive diagnostics for an ONU.
func (d *VSOLCLIDriver) GetONUDiagnostics(ctx context.Context, ponPort string, onuID int) (*cli.ONUDiagnostics, error) {
	diag := &cli.ONUDiagnostics{
		PonPort:     ponPort,
		OnuID:       onuID,
		LastUpdated: time.Now(),
	}

	// Get basic ONU info
	info, err := d.GetONUInfo(ctx, ponPort, onuID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ONU info: %w", err)
	}
	diag.SerialNumber = info.SerialNumber
	diag.Status = info.Status

	// Get optical diagnostics
	optical, err := d.GetOpticalDiagnostics(ctx, ponPort, onuID)
	if err == nil {
		diag.Optical = *optical
	}

	// Get performance counters
	counters, err := d.GetONUCounters(ctx, ponPort, onuID)
	if err == nil {
		diag.Counters = *counters
	}

	// Get health info
	health, err := d.getONUHealth(ctx, ponPort, onuID)
	if err == nil {
		diag.Health = *health
	}

	// Get connectivity info
	connectivity, err := d.getONUConnectivity(ctx, ponPort, onuID)
	if err == nil {
		diag.Connectivity = *connectivity
	}

	return diag, nil
}

// getONUHealth retrieves ONU health information.
func (d *VSOLCLIDriver) getONUHealth(ctx context.Context, ponPort string, onuID int) (*cli.DeviceHealth, error) {
	// Enter GPON interface first
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return nil, fmt.Errorf("failed to enter interface: %w", err)
	}
	defer d.Execute(ctx, "exit")

	cmd = fmt.Sprintf("show onu %d detail-info", onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, err
	}

	health := &cli.DeviceHealth{}

	// Parse CPU usage
	cpuRegex := regexp.MustCompile(`(?i)CPU\s*[Uu]sage\s*:\s*([\d.]+)`)
	if matches := cpuRegex.FindStringSubmatch(output); len(matches) > 1 {
		health.CPUUsage, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse memory usage
	memRegex := regexp.MustCompile(`(?i)Memory\s*[Uu]sage\s*:\s*([\d.]+)`)
	if matches := memRegex.FindStringSubmatch(output); len(matches) > 1 {
		health.MemoryUsage, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse temperature
	tempRegex := regexp.MustCompile(`(?i)Temperature\s*:\s*([\d.]+)`)
	if matches := tempRegex.FindStringSubmatch(output); len(matches) > 1 {
		health.Temperature, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse uptime
	uptimeRegex := regexp.MustCompile(`(?i)Uptime\s*:\s*(.+)`)
	if matches := uptimeRegex.FindStringSubmatch(output); len(matches) > 1 {
		health.Uptime = strings.TrimSpace(matches[1])
	}

	// Parse firmware version
	fwRegex := regexp.MustCompile(`(?i)(?:Firmware|Software)\s*[Vv]ersion\s*:\s*(\S+)`)
	if matches := fwRegex.FindStringSubmatch(output); len(matches) > 1 {
		health.FirmwareVer = matches[1]
	}

	// Parse hardware version
	hwRegex := regexp.MustCompile(`(?i)Hardware\s*[Vv]ersion\s*:\s*(\S+)`)
	if matches := hwRegex.FindStringSubmatch(output); len(matches) > 1 {
		health.HardwareVer = matches[1]
	}

	return health, nil
}

// getONUConnectivity retrieves ONU connectivity information.
func (d *VSOLCLIDriver) getONUConnectivity(ctx context.Context, ponPort string, onuID int) (*cli.ConnectivityInfo, error) {
	// Enter GPON interface first
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return nil, fmt.Errorf("failed to enter interface: %w", err)
	}
	defer d.Execute(ctx, "exit")

	cmd = fmt.Sprintf("show onu %d state", onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, err
	}

	conn := &cli.ConnectivityInfo{}

	// Parse distance
	distRegex := regexp.MustCompile(`(?i)Distance\s*:\s*(\d+)`)
	if matches := distRegex.FindStringSubmatch(output); len(matches) > 1 {
		conn.Distance, _ = strconv.Atoi(matches[1])
	}

	// Parse RTT
	rttRegex := regexp.MustCompile(`(?i)RTT\s*:\s*([\d.]+)`)
	if matches := rttRegex.FindStringSubmatch(output); len(matches) > 1 {
		conn.RTT, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse offline reason
	reasonRegex := regexp.MustCompile(`(?i)(?:Offline|Down)\s*[Rr]eason\s*:\s*(.+)`)
	if matches := reasonRegex.FindStringSubmatch(output); len(matches) > 1 {
		conn.OfflineReason = strings.TrimSpace(matches[1])
	}

	// Parse offline count
	countRegex := regexp.MustCompile(`(?i)Offline\s*[Cc]ount\s*:\s*(\d+)`)
	if matches := countRegex.FindStringSubmatch(output); len(matches) > 1 {
		conn.OfflineCount, _ = strconv.Atoi(matches[1])
	}

	return conn, nil
}

// GetONUCounters retrieves performance counters for an ONU.
func (d *VSOLCLIDriver) GetONUCounters(ctx context.Context, ponPort string, onuID int) (*cli.PerformanceCounters, error) {
	// Enter GPON interface first
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return nil, fmt.Errorf("failed to enter interface: %w", err)
	}
	defer d.Execute(ctx, "exit")

	// V-SOL may use various counter commands
	cmd = fmt.Sprintf("show onu %d eth 1 statistics", onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get ONU counters: %w", err)
	}

	return parseVSOLCounters(output)
}

// parseVSOLCounters parses V-Sol ONU counters output.
func parseVSOLCounters(output string) (*cli.PerformanceCounters, error) {
	counters := &cli.PerformanceCounters{}

	// Parse RX bytes
	rxBytesRegex := regexp.MustCompile(`(?i)RX\s*[Bb]ytes\s*:\s*(\d+)`)
	if matches := rxBytesRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.RxBytes, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse TX bytes
	txBytesRegex := regexp.MustCompile(`(?i)TX\s*[Bb]ytes\s*:\s*(\d+)`)
	if matches := txBytesRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.TxBytes, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse RX packets
	rxPktsRegex := regexp.MustCompile(`(?i)RX\s*[Pp]ackets\s*:\s*(\d+)`)
	if matches := rxPktsRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.RxPackets, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse TX packets
	txPktsRegex := regexp.MustCompile(`(?i)TX\s*[Pp]ackets\s*:\s*(\d+)`)
	if matches := txPktsRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.TxPackets, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse RX errors
	rxErrRegex := regexp.MustCompile(`(?i)RX\s*[Ee]rrors\s*:\s*(\d+)`)
	if matches := rxErrRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.RxErrors, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse TX errors
	txErrRegex := regexp.MustCompile(`(?i)TX\s*[Ee]rrors\s*:\s*(\d+)`)
	if matches := txErrRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.TxErrors, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse RX dropped
	rxDropRegex := regexp.MustCompile(`(?i)RX\s*[Dd]ropped\s*:\s*(\d+)`)
	if matches := rxDropRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.RxDropped, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse TX dropped
	txDropRegex := regexp.MustCompile(`(?i)TX\s*[Dd]ropped\s*:\s*(\d+)`)
	if matches := txDropRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.TxDropped, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse CRC errors
	crcRegex := regexp.MustCompile(`(?i)CRC\s*[Ee]rrors\s*:\s*(\d+)`)
	if matches := crcRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.CRCErrors, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	// Parse FCS errors
	fcsRegex := regexp.MustCompile(`(?i)FCS\s*[Ee]rrors\s*:\s*(\d+)`)
	if matches := fcsRegex.FindStringSubmatch(output); len(matches) > 1 {
		counters.FCSErrors, _ = strconv.ParseUint(matches[1], 10, 64)
	}

	return counters, nil
}

// ClearONUCounters clears/resets performance counters for an ONU.
func (d *VSOLCLIDriver) ClearONUCounters(ctx context.Context, ponPort string, onuID int) error {
	// Enter GPON interface first
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to enter interface: %w", err)
	}
	defer d.Execute(ctx, "exit")

	cmd = fmt.Sprintf("clean onu %d statistics", onuID)
	_, err := d.Execute(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to clear ONU counters: %w", err)
	}

	return nil
}

// GetOpticalDiagnostics retrieves optical power readings for an ONU.
func (d *VSOLCLIDriver) GetOpticalDiagnostics(ctx context.Context, ponPort string, onuID int) (*cli.OpticalDiagnostics, error) {
	// Enter GPON interface first
	cmd := fmt.Sprintf("interface gpon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return nil, fmt.Errorf("failed to enter interface: %w", err)
	}
	defer d.Execute(ctx, "exit")

	diag := &cli.OpticalDiagnostics{}

	// V-SOL uses "show pon onu <id> rx-power" and "show pon onu <id> tx-power"
	cmd = fmt.Sprintf("show pon onu %d rx-power", onuID)
	rxOutput, err := d.Execute(ctx, cmd)
	if err == nil {
		// Parse RX power from output like "GPON0/1:1           -28.530(dbm)"
		rxRegex := regexp.MustCompile(`([-\d.]+)\s*\(?\s*dbm\s*\)?`)
		if matches := rxRegex.FindStringSubmatch(rxOutput); len(matches) > 1 {
			diag.RxPower, _ = strconv.ParseFloat(matches[1], 64)
		}
	}

	cmd = fmt.Sprintf("show pon onu %d tx-power", onuID)
	txOutput, err := d.Execute(ctx, cmd)
	if err == nil {
		// Parse TX power
		txRegex := regexp.MustCompile(`([-\d.]+)\s*\(?\s*dbm\s*\)?`)
		if matches := txRegex.FindStringSubmatch(txOutput); len(matches) > 1 {
			diag.TxPower, _ = strconv.ParseFloat(matches[1], 64)
		}
	}

	// Determine status based on thresholds
	diag.RxPowerStatus = determineOpticalStatus(diag.RxPower, -28.0, -25.0)
	diag.TxPowerStatus = determineOpticalStatus(diag.TxPower, -3.0, 0.5)

	return diag, nil
}

// parseVSOLOpticalDiag parses V-Sol optical diagnostics output.
func parseVSOLOpticalDiag(output string) (*cli.OpticalDiagnostics, error) {
	diag := &cli.OpticalDiagnostics{}

	// Parse RX power
	rxRegex := regexp.MustCompile(`(?i)(?:RX|Receive)\s*[Pp]ower\s*:\s*([-\d.]+)`)
	if matches := rxRegex.FindStringSubmatch(output); len(matches) > 1 {
		diag.RxPower, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse TX power
	txRegex := regexp.MustCompile(`(?i)(?:TX|Transmit)\s*[Pp]ower\s*:\s*([-\d.]+)`)
	if matches := txRegex.FindStringSubmatch(output); len(matches) > 1 {
		diag.TxPower, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse OLT RX power
	oltRxRegex := regexp.MustCompile(`(?i)OLT\s*(?:RX|Receive)\s*[Pp]ower\s*:\s*([-\d.]+)`)
	if matches := oltRxRegex.FindStringSubmatch(output); len(matches) > 1 {
		diag.OltRxPower, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse temperature
	tempRegex := regexp.MustCompile(`(?i)Temperature\s*:\s*([\d.]+)`)
	if matches := tempRegex.FindStringSubmatch(output); len(matches) > 1 {
		diag.Temperature, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse voltage
	voltRegex := regexp.MustCompile(`(?i)Voltage\s*:\s*([\d.]+)`)
	if matches := voltRegex.FindStringSubmatch(output); len(matches) > 1 {
		diag.Voltage, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse bias current
	biasRegex := regexp.MustCompile(`(?i)Bias\s*[Cc]urrent\s*:\s*([\d.]+)`)
	if matches := biasRegex.FindStringSubmatch(output); len(matches) > 1 {
		diag.BiasCurrent, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Determine RX power status based on thresholds
	diag.RxPowerStatus = determineOpticalStatus(diag.RxPower, -28.0, -25.0)
	diag.TxPowerStatus = determineOpticalStatus(diag.TxPower, -3.0, 0.5)

	return diag, nil
}

// determineOpticalStatus determines optical power status based on thresholds.
func determineOpticalStatus(power, criticalThreshold, warningThreshold float64) string {
	if power == 0 {
		return "unknown"
	}
	if power < criticalThreshold {
		return "critical"
	}
	if power < warningThreshold {
		return "warning"
	}
	return "normal"
}
