package snmp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

// FiberHome Enterprise OID and base paths
const (
	FiberHomeEnterprise = "1.3.6.1.4.1.5875"
	FHOltData           = FiberHomeEnterprise + ".800.3"
)

// FiberHome System OIDs
var fhSystemOIDs = struct {
	CPUUtilization     string
	MemUtilization     string
	CurrentTemperature string
	SysIP              string
	SysMAC             string
	SysSoftVersion     string
	SysHardVersion     string
}{
	CPUUtilization:     FHOltData + ".8.6.1.1",
	MemUtilization:     FHOltData + ".8.6.1.2",
	CurrentTemperature: FHOltData + ".8.6.1.3",
	SysIP:              FHOltData + ".9.4.1",
	SysMAC:             FHOltData + ".9.4.2",
	SysSoftVersion:     FHOltData + ".9.4.3",
	SysHardVersion:     FHOltData + ".9.4.4",
}

// FiberHome Card OIDs
var fhCardOIDs = struct {
	CardInfoTable string
	CardType      string
	CardStatus    string
	CardCpuUtil   string
	CardMemUtil   string
}{
	CardInfoTable: FHOltData + ".9.2.1",
	CardType:      FHOltData + ".9.2.1.1.2",
	CardStatus:    FHOltData + ".9.2.1.1.3",
	CardCpuUtil:   FHOltData + ".9.2.1.1.8",
	CardMemUtil:   FHOltData + ".9.2.1.1.9",
}

// FiberHome PON OIDs
var fhPonOIDs = struct {
	PonInfoTable    string
	PonEnable       string
	PonOnuCount     string
	PonTxPower      string
	PonRxPowerTable string
	PonRxPower      string
}{
	PonInfoTable:    FHOltData + ".9.3.4",
	PonEnable:       FHOltData + ".2.3.1.1.1",
	PonOnuCount:     FHOltData + ".9.3.4.1.4",
	PonTxPower:      FHOltData + ".9.3.4.1.8",
	PonRxPowerTable: FHOltData + ".9.3.7",
	PonRxPower:      FHOltData + ".9.3.7.1.2",
}

// FiberHome ONU OIDs
var fhOnuOIDs = struct {
	AuthOnuListTable string
	AuthOnuMac       string
	AuthOnuSn        string
	AuthOnuLoid      string
	AuthOnuType      string
	AuthOnuDesc      string
	AuthOnuAuthMode  string
	AuthOnuStatus    string
	AuthOnuDistance  string
	AuthOnuSoftVer   string
	AuthOnuHardVer   string
	UnauthOnuTable   string
}{
	AuthOnuListTable: FHOltData + ".10.1",
	AuthOnuMac:       FHOltData + ".10.1.1.3",
	AuthOnuSn:        FHOltData + ".10.1.1.4",
	AuthOnuLoid:      FHOltData + ".10.1.1.6",
	AuthOnuType:      FHOltData + ".10.1.1.8",
	AuthOnuDesc:      FHOltData + ".10.1.1.9",
	AuthOnuAuthMode:  FHOltData + ".10.1.1.10",
	AuthOnuStatus:    FHOltData + ".10.1.1.11",
	AuthOnuDistance:  FHOltData + ".10.1.1.12",
	AuthOnuSoftVer:   FHOltData + ".10.1.1.15",
	AuthOnuHardVer:   FHOltData + ".10.1.1.16",
	UnauthOnuTable:   FHOltData + ".11.1",
}

// FiberHome Optical OIDs
var fhOpticalOIDs = struct {
	OnuOpticalTable string
	OnuRxPower      string
	OnuTxPower      string
	OnuTemperature  string
	OnuVoltage      string
	OnuCurrent      string
}{
	OnuOpticalTable: FHOltData + ".9.3.3",
	OnuRxPower:      FHOltData + ".9.3.3.1.6",
	OnuTxPower:      FHOltData + ".9.3.3.1.7",
	OnuTemperature:  FHOltData + ".9.3.3.1.5",
	OnuVoltage:      FHOltData + ".9.3.3.1.3",
	OnuCurrent:      FHOltData + ".9.3.3.1.4",
}

// FiberHome Traffic OIDs
var fhTrafficOIDs = struct {
	OnuTrafficTable string
	RxBytes         string
	TxBytes         string
	RxPackets       string
	TxPackets       string
}{
	OnuTrafficTable: FHOltData + ".12.1",
	RxBytes:         FHOltData + ".12.1.1.3",
	TxBytes:         FHOltData + ".12.1.1.4",
	RxPackets:       FHOltData + ".12.1.1.5",
	TxPackets:       FHOltData + ".12.1.1.6",
}

// FiberHome ONU Status values
const (
	FHOnuStatusOffline = 0
	FHOnuStatusOnline  = 1
)

// FiberHomeCollector implements SNMP collection for FiberHome OLTs.
type FiberHomeCollector struct {
	*BaseCollector
	thresholds OpticalThresholds
}

// NewFiberHomeCollector creates a new FiberHome SNMP collector.
func NewFiberHomeCollector(config DeviceConfig) *FiberHomeCollector {
	return &FiberHomeCollector{
		BaseCollector: NewBaseCollector(config),
		thresholds:    DefaultOpticalThresholds(),
	}
}

// Vendor returns the vendor type.
func (c *FiberHomeCollector) Vendor() Vendor {
	return VendorFiberHome
}

// CollectOLTInfo gathers FiberHome OLT system information.
func (c *FiberHomeCollector) CollectOLTInfo(ctx context.Context) (*OLTInfo, error) {
	oids := []string{
		fhSystemOIDs.CPUUtilization + ".0",
		fhSystemOIDs.MemUtilization + ".0",
		fhSystemOIDs.CurrentTemperature + ".0",
		fhSystemOIDs.SysIP + ".0",
		fhSystemOIDs.SysMAC + ".0",
		fhSystemOIDs.SysSoftVersion + ".0",
		fhSystemOIDs.SysHardVersion + ".0",
	}

	result, err := c.Get(oids)
	if err != nil {
		return nil, fmt.Errorf("failed to get OLT info: %w", err)
	}

	info := &OLTInfo{
		Host:        c.config.Host,
		Vendor:      VendorFiberHome,
		CollectedAt: time.Now(),
	}

	for _, variable := range result.Variables {
		if variable.Type == gosnmp.NoSuchObject || variable.Type == gosnmp.NoSuchInstance {
			continue
		}

		switch {
		case strings.Contains(variable.Name, fhSystemOIDs.CPUUtilization):
			info.CPUPercent = float64(ParseInt64(variable.Value))
		case strings.Contains(variable.Name, fhSystemOIDs.MemUtilization):
			info.MemoryPercent = float64(ParseInt64(variable.Value))
		case strings.Contains(variable.Name, fhSystemOIDs.CurrentTemperature):
			info.Temperature = float64(ParseInt64(variable.Value))
		case strings.Contains(variable.Name, fhSystemOIDs.SysIP):
			info.IP = ParseIP(variable.Value)
		case strings.Contains(variable.Name, fhSystemOIDs.SysMAC):
			info.MAC = ParseMAC(variable.Value)
		case strings.Contains(variable.Name, fhSystemOIDs.SysSoftVersion):
			info.SoftwareVersion = ParseString(variable.Value)
		case strings.Contains(variable.Name, fhSystemOIDs.SysHardVersion):
			info.HardwareVersion = ParseString(variable.Value)
		}
	}

	return info, nil
}

// CollectPONPorts gathers FiberHome PON port information.
func (c *FiberHomeCollector) CollectPONPorts(ctx context.Context) ([]PONPort, error) {
	portMap := make(map[int]*PONPort)
	var mu sync.Mutex

	err := c.Walk(fhPonOIDs.PonInfoTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ponIdx := ExtractLastIndex(pdu.Name)
		
		mu.Lock()
		port, exists := portMap[ponIdx]
		if !exists {
			slotID, portID := decodeFHPonIndex(ponIdx)
			port = &PONPort{
				Index:  ponIdx,
				SlotID: slotID,
				PortID: portID,
				Name:   fmt.Sprintf("PON %d/%d", slotID, portID),
			}
			portMap[ponIdx] = port
		}
		mu.Unlock()

		baseSuffix := pdu.Name[len(fhPonOIDs.PonInfoTable):]
		switch {
		case strings.HasPrefix(baseSuffix, ".1.4."):
			port.ONUCount = int(ParseInt64(pdu.Value))
		case strings.HasPrefix(baseSuffix, ".1.8."):
			port.TxPowerDBm = ConvertOpticalPower100(ParseInt64(pdu.Value))
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

// CollectONUs gathers FiberHome authorized ONU information.
func (c *FiberHomeCollector) CollectONUs(ctx context.Context) ([]ONUInfo, error) {
	onuMap := make(map[string]*ONUInfo)
	var mu sync.Mutex

	err := c.Walk(fhOnuOIDs.AuthOnuListTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// FiberHome index format: ponIndex.onuIndex
		indices := ExtractIndex(pdu.Name, fhOnuOIDs.AuthOnuListTable)
		if len(indices) < 2 {
			return nil
		}

		ponIdx := indices[len(indices)-2]
		onuIdx := indices[len(indices)-1]
		key := fmt.Sprintf("%d.%d", ponIdx, onuIdx)

		mu.Lock()
		onu, exists := onuMap[key]
		if !exists {
			slotID, portID := decodeFHPonIndex(ponIdx)
			onu = &ONUInfo{
				PonIndex: ponIdx,
				OnuIndex: onuIdx,
				OnuID:    fmt.Sprintf("%d/%d/%d", slotID, portID, onuIdx),
			}
			onuMap[key] = onu
		}
		mu.Unlock()

		baseSuffix := pdu.Name[len(fhOnuOIDs.AuthOnuListTable):]
		switch {
		case strings.HasPrefix(baseSuffix, ".1.3."):
			onu.MAC = ParseMAC(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.4."):
			onu.SerialNumber = ParseString(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.8."):
			onu.Type = ParseString(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.9."):
			onu.Description = ParseString(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.10."):
			onu.AuthMode = parseFHAuthMode(int(ParseInt64(pdu.Value)))
		case strings.HasPrefix(baseSuffix, ".1.11."):
			status := int(ParseInt64(pdu.Value))
			if status == FHOnuStatusOnline {
				onu.Status = "online"
			} else {
				onu.Status = "offline"
			}
		case strings.HasPrefix(baseSuffix, ".1.12."):
			onu.Distance = int(ParseInt64(pdu.Value))
		case strings.HasPrefix(baseSuffix, ".1.15."):
			onu.SoftwareVersion = ParseString(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.16."):
			onu.HardwareVersion = ParseString(pdu.Value)
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

// CollectUnauthONUs gathers FiberHome unauthorized ONUs.
func (c *FiberHomeCollector) CollectUnauthONUs(ctx context.Context) ([]UnauthONU, error) {
	var unauthOnus []UnauthONU
	onuMap := make(map[string]*UnauthONU)
	var mu sync.Mutex

	err := c.Walk(fhOnuOIDs.UnauthOnuTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		indices := ExtractIndex(pdu.Name, fhOnuOIDs.UnauthOnuTable)
		if len(indices) < 2 {
			return nil
		}

		ponIdx := indices[len(indices)-2]
		onuIdx := indices[len(indices)-1]
		key := fmt.Sprintf("%d.%d", ponIdx, onuIdx)

		mu.Lock()
		onu, exists := onuMap[key]
		if !exists {
			onu = &UnauthONU{
				PonIndex: ponIdx,
				OnuIndex: onuIdx,
			}
			onuMap[key] = onu
		}
		mu.Unlock()

		baseSuffix := pdu.Name[len(fhOnuOIDs.UnauthOnuTable):]
		switch {
		case strings.HasPrefix(baseSuffix, ".1.3."):
			onu.MAC = ParseMAC(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.4."):
			onu.SerialNumber = ParseString(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.5."):
			onu.Type = ParseString(pdu.Value)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to collect unauthorized ONUs: %w", err)
	}

	for _, onu := range onuMap {
		unauthOnus = append(unauthOnus, *onu)
	}

	return unauthOnus, nil
}

// CollectONUOptical gathers FiberHome ONU optical power readings.
func (c *FiberHomeCollector) CollectONUOptical(ctx context.Context) ([]ONUOptical, error) {
	optMap := make(map[string]*ONUOptical)
	var mu sync.Mutex

	err := c.Walk(fhOpticalOIDs.OnuOpticalTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		indices := ExtractIndex(pdu.Name, fhOpticalOIDs.OnuOpticalTable)
		if len(indices) < 2 {
			return nil
		}

		ponIdx := indices[len(indices)-2]
		onuIdx := indices[len(indices)-1]
		key := fmt.Sprintf("%d.%d", ponIdx, onuIdx)

		mu.Lock()
		opt, exists := optMap[key]
		if !exists {
			slotID, portID := decodeFHPonIndex(ponIdx)
			opt = &ONUOptical{
				PonIndex: ponIdx,
				OnuIndex: onuIdx,
				OnuID:    fmt.Sprintf("%d/%d/%d", slotID, portID, onuIdx),
			}
			optMap[key] = opt
		}
		mu.Unlock()

		baseSuffix := pdu.Name[len(fhOpticalOIDs.OnuOpticalTable):]
		// FiberHome returns optical power in 0.01 dBm units
		switch {
		case strings.HasPrefix(baseSuffix, ".1.3."):
			opt.Voltage = float64(ParseInt64(pdu.Value)) / 1000.0
		case strings.HasPrefix(baseSuffix, ".1.4."):
			opt.BiasCurrent = float64(ParseInt64(pdu.Value)) / 1000.0
		case strings.HasPrefix(baseSuffix, ".1.5."):
			opt.Temperature = float64(ParseInt64(pdu.Value)) / 100.0
		case strings.HasPrefix(baseSuffix, ".1.6."):
			opt.RxPowerDBm = ConvertOpticalPower100(ParseInt64(pdu.Value))
		case strings.HasPrefix(baseSuffix, ".1.7."):
			opt.TxPowerDBm = ConvertOpticalPower100(ParseInt64(pdu.Value))
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

// CollectAll gathers complete FiberHome OLT telemetry.
func (c *FiberHomeCollector) CollectAll(ctx context.Context) (*OLTTelemetry, error) {
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

	// Collect unauthorized ONUs
	unauthOnus, err := c.CollectUnauthONUs(ctx)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Unauth ONUs: %v", err))
	} else {
		telemetry.UnauthONUs = unauthOnus
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

// decodeFHPonIndex decodes FiberHome PON index to slot/port
// FiberHome uses: ponIndex = (slotId * 65536) + portId
func decodeFHPonIndex(ponIdx int) (slotID, portID int) {
	slotID = ponIdx / 65536
	portID = ponIdx % 65536
	return
}

func parseFHAuthMode(mode int) string {
	modes := map[int]string{
		1: "mac",
		2: "sn",
		3: "loid",
		4: "password",
		5: "hybrid",
	}
	if s, ok := modes[mode]; ok {
		return s
	}
	return fmt.Sprintf("unknown(%d)", mode)
}
