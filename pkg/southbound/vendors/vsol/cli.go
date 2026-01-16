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

	// V-Sol typically uses enable -> configure terminal
	if _, err := d.Execute(ctx, "enable"); err != nil {
		return fmt.Errorf("failed to enter enable mode: %w", err)
	}

	if _, err := d.Execute(ctx, "configure terminal"); err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}

	return nil
}

// AddONU provisions a new ONU on V-Sol OLT.
func (d *VSOLCLIDriver) AddONU(ctx context.Context, req *cli.ONUProvisionRequest) error {
	if req.SerialNumber == "" {
		return fmt.Errorf("serial number is required")
	}

	// Enter EPON/GPON interface
	// V-Sol uses different interface naming depending on technology
	cmd := fmt.Sprintf("interface epon %s", req.PonPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		// Try GPON interface if EPON fails
		cmd = fmt.Sprintf("interface gpon-olt %s", req.PonPort)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to enter interface: %w", err)
		}
	}

	// Build ONU authorization command
	cmdParts := []string{
		fmt.Sprintf("onu %d", req.OnuID),
	}

	if req.Type != "" {
		cmdParts = append(cmdParts, fmt.Sprintf("type %s", req.Type))
	}

	cmdParts = append(cmdParts, fmt.Sprintf("sn %s", req.SerialNumber))

	cmd = strings.Join(cmdParts, " ")
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to add ONU: %w", err)
	}

	if strings.Contains(strings.ToLower(output), "error") ||
		strings.Contains(strings.ToLower(output), "fail") {
		return fmt.Errorf("ONU add failed: %s", output)
	}

	// Configure description if provided
	if req.Description != "" {
		cmd = fmt.Sprintf("onu %d description %s", req.OnuID, req.Description)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to set description: %w", err)
		}
	}

	// Configure native VLAN if specified
	if req.NativeVLAN > 0 {
		cmd = fmt.Sprintf("onu %d vlan %d", req.OnuID, req.NativeVLAN)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to configure VLAN: %w", err)
		}
	}

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
	// Enter interface
	cmd := fmt.Sprintf("interface epon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		// Try GPON interface
		cmd = fmt.Sprintf("interface gpon-olt %s", ponPort)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to enter interface: %w", err)
		}
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

// GetONUInfo retrieves ONU information via CLI.
func (d *VSOLCLIDriver) GetONUInfo(ctx context.Context, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
	cmd := fmt.Sprintf("show onu %s/%d info", ponPort, onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		// Try alternate command format
		cmd = fmt.Sprintf("show epon onu-info %s %d", ponPort, onuID)
		output, err = d.Execute(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("failed to get ONU info: %w", err)
		}
	}

	return parseVSOLONUInfo(output, ponPort, onuID)
}

// parseVSOLONUInfo parses V-Sol ONU info output.
func parseVSOLONUInfo(output string, ponPort string, onuID int) (*cli.ONUCLIInfo, error) {
	info := &cli.ONUCLIInfo{
		PonPort: ponPort,
		OnuID:   onuID,
	}

	// Parse serial number
	snRegex := regexp.MustCompile(`(?i)Serial\s*[Nn]umber\s*:\s*(\S+)`)
	if matches := snRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.SerialNumber = matches[1]
	}

	// Parse MAC address
	macRegex := regexp.MustCompile(`(?i)MAC\s*[Aa]ddress\s*:\s*(\S+)`)
	if matches := macRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.MAC = matches[1]
	}

	// Parse status
	statusRegex := regexp.MustCompile(`(?i)Status\s*:\s*(\S+)`)
	if matches := statusRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.Status = strings.ToLower(matches[1])
	}

	// Parse type/model
	typeRegex := regexp.MustCompile(`(?i)(?:Type|Model)\s*:\s*(\S+)`)
	if matches := typeRegex.FindStringSubmatch(output); len(matches) > 1 {
		info.Type = matches[1]
	}

	// Parse distance
	distRegex := regexp.MustCompile(`(?i)Distance\s*:\s*(\d+)`)
	if matches := distRegex.FindStringSubmatch(output); len(matches) > 1 {
		if d, err := strconv.Atoi(matches[1]); err == nil {
			info.Distance = d
		}
	}

	// Parse RX power
	rxRegex := regexp.MustCompile(`(?i)(?:RX|Receive)\s*[Pp]ower\s*:\s*([-\d.]+)`)
	if matches := rxRegex.FindStringSubmatch(output); len(matches) > 1 {
		if rx, err := strconv.ParseFloat(matches[1], 64); err == nil {
			info.RxPower = rx
		}
	}

	// Parse description
	descRegex := regexp.MustCompile(`(?i)Description\s*:\s*(.+)`)
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

// RebootONU reboots a specific ONU.
func (d *VSOLCLIDriver) RebootONU(ctx context.Context, ponPort string, onuID int) error {
	cmd := fmt.Sprintf("interface epon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		cmd = fmt.Sprintf("interface gpon-olt %s", ponPort)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to enter interface: %w", err)
		}
	}

	cmd = fmt.Sprintf("onu %d reboot", onuID)
	if _, err := d.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to reboot ONU: %w", err)
	}

	if _, err := d.Execute(ctx, "exit"); err != nil {
		return fmt.Errorf("failed to exit interface: %w", err)
	}

	return nil
}

// =============================================================================
// VLAN Management
// =============================================================================

// ConfigureVLAN configures VLAN settings for an ONU.
func (d *VSOLCLIDriver) ConfigureVLAN(ctx context.Context, config *cli.VLANConfig) error {
	// Enter interface
	cmd := fmt.Sprintf("interface epon %s", config.PonPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		cmd = fmt.Sprintf("interface gpon-olt %s", config.PonPort)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to enter interface: %w", err)
		}
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
	cmd := fmt.Sprintf("show onu %s/%d vlan", ponPort, onuID)
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
	// Enter interface
	cmd := fmt.Sprintf("interface epon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		cmd = fmt.Sprintf("interface gpon-olt %s", ponPort)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to enter interface: %w", err)
		}
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
	// Enter interface
	cmd := fmt.Sprintf("interface epon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		cmd = fmt.Sprintf("interface gpon-olt %s", ponPort)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to enter interface: %w", err)
		}
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
func parseVSOLVLANList(output string) ([]cli.VLANInfo, error) {
	var vlans []cli.VLANInfo

	// V-Sol format typically:
	// VLAN ID    Name                    Description
	// -------    ----                    -----------
	// 1          default                 Default VLAN
	// 100        DATA                    Data VLAN
	vlanRegex := regexp.MustCompile(`^\s*(\d+)\s+(\S+)\s*(.*)$`)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if matches := vlanRegex.FindStringSubmatch(line); len(matches) > 2 {
			id, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			vlans = append(vlans, cli.VLANInfo{
				ID:          id,
				Name:        strings.TrimSpace(matches[2]),
				Description: strings.TrimSpace(matches[3]),
			})
		}
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
	// Enter interface
	cmd := fmt.Sprintf("interface epon %s", ponPort)
	if _, err := d.Execute(ctx, cmd); err != nil {
		cmd = fmt.Sprintf("interface gpon-olt %s", ponPort)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to enter interface: %w", err)
		}
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
func (d *VSOLCLIDriver) ListPONPorts(ctx context.Context) ([]cli.PONPortInfo, error) {
	output, err := d.Execute(ctx, "show interface epon brief")
	if err != nil {
		// Try GPON
		output, err = d.Execute(ctx, "show interface gpon brief")
		if err != nil {
			return nil, fmt.Errorf("failed to list PON ports: %w", err)
		}
	}

	return parseVSOLPONPorts(output)
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
	cmd := fmt.Sprintf("show interface epon %d/%d", slot, port)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		cmd = fmt.Sprintf("show interface gpon %d/%d", slot, port)
		output, err = d.Execute(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("failed to get PON port info: %w", err)
		}
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
	cmd := fmt.Sprintf("interface epon %d/%d", slot, port)
	if _, err := d.Execute(ctx, cmd); err != nil {
		cmd = fmt.Sprintf("interface gpon %d/%d", slot, port)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to enter interface: %w", err)
		}
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
	cmd := fmt.Sprintf("interface epon %d/%d", slot, port)
	if _, err := d.Execute(ctx, cmd); err != nil {
		cmd = fmt.Sprintf("interface gpon %d/%d", slot, port)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to enter interface: %w", err)
		}
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
	cmd := fmt.Sprintf("interface epon %d/%d", slot, port)
	if _, err := d.Execute(ctx, cmd); err != nil {
		cmd = fmt.Sprintf("interface gpon %d/%d", slot, port)
		if _, err := d.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("failed to enter interface: %w", err)
		}
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
	cmd := fmt.Sprintf("show onu %s/%d status", ponPort, onuID)
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
	cmd := fmt.Sprintf("show onu %s/%d connectivity", ponPort, onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		// Try alternate command
		cmd = fmt.Sprintf("show onu %s/%d info", ponPort, onuID)
		output, err = d.Execute(ctx, cmd)
		if err != nil {
			return nil, err
		}
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
	cmd := fmt.Sprintf("show onu %s/%d counters", ponPort, onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		// Try alternate command
		cmd = fmt.Sprintf("show epon onu-counters %s %d", ponPort, onuID)
		output, err = d.Execute(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("failed to get ONU counters: %w", err)
		}
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
	cmd := fmt.Sprintf("clear onu %s/%d counters", ponPort, onuID)
	_, err := d.Execute(ctx, cmd)
	if err != nil {
		// Try alternate command
		cmd = fmt.Sprintf("clear epon onu-counters %s %d", ponPort, onuID)
		_, err = d.Execute(ctx, cmd)
		if err != nil {
			return fmt.Errorf("failed to clear ONU counters: %w", err)
		}
	}

	return nil
}

// GetOpticalDiagnostics retrieves optical power readings for an ONU.
func (d *VSOLCLIDriver) GetOpticalDiagnostics(ctx context.Context, ponPort string, onuID int) (*cli.OpticalDiagnostics, error) {
	cmd := fmt.Sprintf("show onu %s/%d optical", ponPort, onuID)
	output, err := d.Execute(ctx, cmd)
	if err != nil {
		// Try alternate command
		cmd = fmt.Sprintf("show epon onu-optical %s %d", ponPort, onuID)
		output, err = d.Execute(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("failed to get optical diagnostics: %w", err)
		}
	}

	return parseVSOLOpticalDiag(output)
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
