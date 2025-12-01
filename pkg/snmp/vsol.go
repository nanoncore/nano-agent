package snmp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

// V-SOL Enterprise OID and base paths
const (
	VSOLEnterprise = "1.3.6.1.4.1.37950"
	VSOLOltBase    = VSOLEnterprise + ".1.1.5"
)

// V-SOL System OIDs
var vsolSystemOIDs = struct {
	CPULoad     string
	MemoryLoad  string
	Temperature string
}{
	CPULoad:     VSOLOltBase + ".10.12.3",
	MemoryLoad:  VSOLOltBase + ".10.12.4",
	Temperature: VSOLOltBase + ".10.12.5.9",
}

// V-SOL PON Port OIDs
var vsolPonOIDs = struct {
	PortTable   string
	PortStatus  string
	PortOnuCount string
}{
	PortTable:    VSOLOltBase + ".10.11",
	PortStatus:   VSOLOltBase + ".10.11.1.3",
	PortOnuCount: VSOLOltBase + ".10.11.1.5",
}

// V-SOL ONU OIDs
var vsolOnuOIDs = struct {
	InfoTable       string
	SerialNumber    string
	MAC             string
	Status          string
	Distance        string
	Model           string
	SoftwareVersion string
	LastOnline      string
	LastOffline     string
}{
	InfoTable:       VSOLOltBase + ".12.1",
	SerialNumber:    VSOLOltBase + ".12.1.1.3",
	MAC:             VSOLOltBase + ".12.1.1.4",
	Status:          VSOLOltBase + ".12.1.1.5",
	Distance:        VSOLOltBase + ".12.1.1.8",
	Model:           VSOLOltBase + ".12.1.1.6",
	SoftwareVersion: VSOLOltBase + ".12.1.1.10",
	LastOnline:      VSOLOltBase + ".12.1.1.12",
	LastOffline:     VSOLOltBase + ".12.1.1.13",
}

// V-SOL Optical Power OIDs
var vsolOpticalOIDs = struct {
	DiagTable   string
	OnuRxPower  string
	OnuTxPower  string
	OltRxPower  string
	Temperature string
	Voltage     string
	BiasCurrent string
}{
	DiagTable:   VSOLOltBase + ".12.8",
	OnuRxPower:  VSOLOltBase + ".12.8.1.4",
	OnuTxPower:  VSOLOltBase + ".12.8.1.5",
	OltRxPower:  VSOLOltBase + ".12.8.1.6",
	Temperature: VSOLOltBase + ".12.8.1.2",
	Voltage:     VSOLOltBase + ".12.8.1.3",
	BiasCurrent: VSOLOltBase + ".12.8.1.7",
}

// V-SOL Authentication Mode OIDs  
var vsolAuthOIDs = struct {
	ModeTable string
	AuthMode  string
}{
	ModeTable: VSOLOltBase + ".12.2",
	AuthMode:  VSOLOltBase + ".12.2.1.2",
}

// V-SOL ONU Status values
const (
	VSOLOnuStatusOnline  = 1
	VSOLOnuStatusOffline = 0
)

// VSOLCollector implements SNMP collection for V-SOL OLTs.
type VSOLCollector struct {
	*BaseCollector
	thresholds OpticalThresholds
}

// NewVSOLCollector creates a new V-SOL SNMP collector.
func NewVSOLCollector(config DeviceConfig) *VSOLCollector {
	return &VSOLCollector{
		BaseCollector: NewBaseCollector(config),
		thresholds:    DefaultOpticalThresholds(),
	}
}

// Vendor returns the vendor type.
func (c *VSOLCollector) Vendor() Vendor {
	return VendorVSOL
}

// CollectOLTInfo gathers V-SOL OLT system information.
func (c *VSOLCollector) CollectOLTInfo(ctx context.Context) (*OLTInfo, error) {
	oids := []string{
		vsolSystemOIDs.CPULoad,
		vsolSystemOIDs.MemoryLoad,
		vsolSystemOIDs.Temperature,
	}

	result, err := c.Get(oids)
	if err != nil {
		return nil, fmt.Errorf("failed to get OLT info: %w", err)
	}

	info := &OLTInfo{
		Host:        c.config.Host,
		Vendor:      VendorVSOL,
		CollectedAt: time.Now(),
	}

	for _, variable := range result.Variables {
		if variable.Type == gosnmp.NoSuchObject || variable.Type == gosnmp.NoSuchInstance {
			continue
		}

		switch {
		case strings.HasPrefix(variable.Name, vsolSystemOIDs.CPULoad):
			info.CPUPercent = float64(ParseInt64(variable.Value))
		case strings.HasPrefix(variable.Name, vsolSystemOIDs.MemoryLoad):
			info.MemoryPercent = float64(ParseInt64(variable.Value))
		case strings.HasPrefix(variable.Name, vsolSystemOIDs.Temperature):
			info.Temperature = float64(ParseInt64(variable.Value))
		}
	}

	return info, nil
}

// CollectPONPorts gathers V-SOL PON port information.
func (c *VSOLCollector) CollectPONPorts(ctx context.Context) ([]PONPort, error) {
	portMap := make(map[int]*PONPort)
	var mu sync.Mutex

	err := c.Walk(vsolPonOIDs.PortTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		indices := ExtractIndex(pdu.Name, vsolPonOIDs.PortTable)
		if len(indices) < 2 {
			return nil
		}

		portIdx := indices[len(indices)-1]
		
		mu.Lock()
		port, exists := portMap[portIdx]
		if !exists {
			port = &PONPort{
				Index:  portIdx,
				SlotID: portIdx / 256,
				PortID: portIdx % 256,
				Name:   fmt.Sprintf("PON %d/%d", portIdx/256, portIdx%256),
			}
			portMap[portIdx] = port
		}
		mu.Unlock()

		switch {
		case strings.Contains(pdu.Name, ".3."):
			port.Status = parseVSOLPortStatus(int(ParseInt64(pdu.Value)))
			port.Enabled = port.Status == "up"
		case strings.Contains(pdu.Name, ".5."):
			port.ONUCount = int(ParseInt64(pdu.Value))
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to collect PON ports: %w", err)
	}

	ports := make([]PONPort, 0, len(portMap))
	for _, port := range portMap {
		ports = append(ports, *port)
	}

	return ports, nil
}

// CollectONUs gathers V-SOL authorized ONU information.
func (c *VSOLCollector) CollectONUs(ctx context.Context) ([]ONUInfo, error) {
	onuMap := make(map[string]*ONUInfo)
	var mu sync.Mutex

	err := c.Walk(vsolOnuOIDs.InfoTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		indices := ExtractIndex(pdu.Name, vsolOnuOIDs.InfoTable)
		if len(indices) < 2 {
			return nil
		}

		ponIdx := indices[len(indices)-2]
		onuIdx := indices[len(indices)-1]
		key := fmt.Sprintf("%d.%d", ponIdx, onuIdx)

		mu.Lock()
		onu, exists := onuMap[key]
		if !exists {
			slotID := ponIdx / 256
			portID := ponIdx % 256
			onu = &ONUInfo{
				PonIndex: ponIdx,
				OnuIndex: onuIdx,
				OnuID:    fmt.Sprintf("%d/%d/%d", slotID, portID, onuIdx),
			}
			onuMap[key] = onu
		}
		mu.Unlock()

		switch {
		case strings.Contains(pdu.Name, vsolOnuOIDs.SerialNumber[len(vsolOnuOIDs.InfoTable):]):
			onu.SerialNumber = ParseString(pdu.Value)
		case strings.Contains(pdu.Name, vsolOnuOIDs.MAC[len(vsolOnuOIDs.InfoTable):]):
			onu.MAC = ParseMAC(pdu.Value)
		case strings.Contains(pdu.Name, vsolOnuOIDs.Status[len(vsolOnuOIDs.InfoTable):]):
			status := int(ParseInt64(pdu.Value))
			if status == VSOLOnuStatusOnline {
				onu.Status = "online"
			} else {
				onu.Status = "offline"
			}
		case strings.Contains(pdu.Name, vsolOnuOIDs.Distance[len(vsolOnuOIDs.InfoTable):]):
			onu.Distance = int(ParseInt64(pdu.Value))
		case strings.Contains(pdu.Name, vsolOnuOIDs.Model[len(vsolOnuOIDs.InfoTable):]):
			onu.Model = ParseString(pdu.Value)
		case strings.Contains(pdu.Name, vsolOnuOIDs.SoftwareVersion[len(vsolOnuOIDs.InfoTable):]):
			onu.SoftwareVersion = ParseString(pdu.Value)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to collect ONUs: %w", err)
	}

	onus := make([]ONUInfo, 0, len(onuMap))
	for _, onu := range onuMap {
		onus = append(onus, *onu)
	}

	return onus, nil
}

// CollectUnauthONUs gathers V-SOL unauthorized ONUs.
func (c *VSOLCollector) CollectUnauthONUs(ctx context.Context) ([]UnauthONU, error) {
	// V-SOL uses same table with different status filtering
	// This is a simplified implementation
	return nil, nil
}

// CollectONUOptical gathers V-SOL ONU optical power readings.
func (c *VSOLCollector) CollectONUOptical(ctx context.Context) ([]ONUOptical, error) {
	optMap := make(map[string]*ONUOptical)
	var mu sync.Mutex

	err := c.Walk(vsolOpticalOIDs.DiagTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		indices := ExtractIndex(pdu.Name, vsolOpticalOIDs.DiagTable)
		if len(indices) < 2 {
			return nil
		}

		ponIdx := indices[len(indices)-2]
		onuIdx := indices[len(indices)-1]
		key := fmt.Sprintf("%d.%d", ponIdx, onuIdx)

		mu.Lock()
		opt, exists := optMap[key]
		if !exists {
			slotID := ponIdx / 256
			portID := ponIdx % 256
			opt = &ONUOptical{
				PonIndex: ponIdx,
				OnuIndex: onuIdx,
				OnuID:    fmt.Sprintf("%d/%d/%d", slotID, portID, onuIdx),
			}
			optMap[key] = opt
		}
		mu.Unlock()

		// V-SOL returns optical power directly in dBm
		switch {
		case strings.Contains(pdu.Name, ".4."):
			opt.RxPowerDBm = parseVSOLOpticalPower(pdu.Value)
		case strings.Contains(pdu.Name, ".5."):
			opt.TxPowerDBm = parseVSOLOpticalPower(pdu.Value)
		case strings.Contains(pdu.Name, ".6."):
			opt.OltRxDBm = parseVSOLOpticalPower(pdu.Value)
		case strings.Contains(pdu.Name, ".2."):
			opt.Temperature = float64(ParseInt64(pdu.Value)) / 100.0
		case strings.Contains(pdu.Name, ".3."):
			opt.Voltage = float64(ParseInt64(pdu.Value)) / 1000.0
		case strings.Contains(pdu.Name, ".7."):
			opt.BiasCurrent = float64(ParseInt64(pdu.Value)) / 1000.0
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to collect ONU optical: %w", err)
	}

	opticals := make([]ONUOptical, 0, len(optMap))
	for _, opt := range optMap {
		opt.Status = EvaluateOpticalStatus(opt.RxPowerDBm, c.thresholds)
		opticals = append(opticals, *opt)
	}

	return opticals, nil
}

// CollectAll gathers complete V-SOL OLT telemetry.
func (c *VSOLCollector) CollectAll(ctx context.Context) (*OLTTelemetry, error) {
	start := time.Now()
	telemetry := &OLTTelemetry{
		CollectedAt: start,
	}
	var errors []string

	// Collect OLT info
	oltInfo, err := c.CollectOLTInfo(ctx)
	if err != nil {
		errors = append(errors, fmt.Sprintf("OLT info: %v", err))
	} else {
		telemetry.OLTInfo = *oltInfo
	}

	// Collect PON ports
	ponPorts, err := c.CollectPONPorts(ctx)
	if err != nil {
		errors = append(errors, fmt.Sprintf("PON ports: %v", err))
	} else {
		telemetry.PONPorts = ponPorts
	}

	// Collect ONUs
	onus, err := c.CollectONUs(ctx)
	if err != nil {
		errors = append(errors, fmt.Sprintf("ONUs: %v", err))
	} else {
		telemetry.ONUs = onus
	}

	// Collect optical power
	optical, err := c.CollectONUOptical(ctx)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Optical: %v", err))
	} else {
		telemetry.ONUOptical = optical
	}

	telemetry.Duration = time.Since(start)
	telemetry.Errors = errors

	return telemetry, nil
}

// Helper functions

func parseVSOLPortStatus(status int) string {
	switch status {
	case 1:
		return "up"
	case 2:
		return "down"
	default:
		return "unknown"
	}
}

func parseVSOLOpticalPower(value interface{}) float64 {
	// V-SOL may return as OCTET STRING with dBm value or as integer
	switch v := value.(type) {
	case []byte:
		// Try to parse as string (e.g., "-12.5")
		s := strings.TrimSpace(string(v))
		var dbm float64
		if _, err := fmt.Sscanf(s, "%f", &dbm); err == nil {
			return dbm
		}
		// Fall through to integer parsing
		if len(v) >= 2 {
			raw := int16(v[0])<<8 | int16(v[1])
			return float64(raw) / 100.0
		}
	case int, int64, int32, uint, uint64, uint32:
		raw := ParseInt64(v)
		if raw == -32768 || raw == 0x7FFF {
			return -40.0
		}
		return float64(raw) / 100.0
	}
	return -40.0
}
