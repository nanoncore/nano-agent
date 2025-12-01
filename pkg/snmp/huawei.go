package snmp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

// Huawei Enterprise OID and base paths
const (
	HuaweiEnterprise = "1.3.6.1.4.1.2011"
	HuaweiXPON       = HuaweiEnterprise + ".6.128.1.1"
)

// Huawei OLT Control OIDs
var huaweiOltOIDs = struct {
	ControlTable    string
	MacAddress      string
	SoftwareVersion string
	RunStatus       string
}{
	ControlTable:    HuaweiXPON + ".2.21",
	MacAddress:      HuaweiXPON + ".2.21.1.3",
	SoftwareVersion: HuaweiXPON + ".2.21.1.5",
	RunStatus:       HuaweiXPON + ".2.21.1.7",
}

// Huawei ONT Info OIDs
var huaweiOntOIDs = struct {
	InfoTable       string
	SerialNumber    string
	Password        string
	RunStatus       string
	Distance        string
	LastUpTime      string
	LastDownTime    string
	LastDownCause   string
	SoftwareVersion string
	Model           string
}{
	InfoTable:       HuaweiXPON + ".2.43",
	SerialNumber:    HuaweiXPON + ".2.43.1.3",
	Password:        HuaweiXPON + ".2.43.1.4",
	RunStatus:       HuaweiXPON + ".2.43.1.6",
	Distance:        HuaweiXPON + ".2.43.1.12",
	LastUpTime:     HuaweiXPON + ".2.43.1.8",
	LastDownTime:    HuaweiXPON + ".2.43.1.9",
	LastDownCause:   HuaweiXPON + ".2.43.1.10",
	SoftwareVersion: HuaweiXPON + ".2.43.1.14",
	Model:           HuaweiXPON + ".2.43.1.5",
}

// Huawei ONT Optical DDM OIDs
var huaweiOpticalOIDs = struct {
	DdmTable    string
	Temperature string
	BiasCurrent string
	TxPower     string
	RxPower     string
	Voltage     string
}{
	DdmTable:    HuaweiXPON + ".2.51",
	Temperature: HuaweiXPON + ".2.51.1.1",
	BiasCurrent: HuaweiXPON + ".2.51.1.2",
	TxPower:     HuaweiXPON + ".2.51.1.3",
	RxPower:     HuaweiXPON + ".2.51.1.4",
	Voltage:     HuaweiXPON + ".2.51.1.5",
}

// Huawei OLT Optical OIDs
var huaweiOltOpticalOIDs = struct {
	RxPowerTable string
	RxPower      string
}{
	RxPowerTable: HuaweiXPON + ".2.52",
	RxPower:      HuaweiXPON + ".2.52.1.1",
}

// Huawei Traffic OIDs
var huaweiTrafficOIDs = struct {
	TrafficTable string
	RxBytes      string
	TxBytes      string
	RxCrcErrors  string
	TxCrcErrors  string
}{
	TrafficTable: HuaweiXPON + ".2.46",
	RxBytes:      HuaweiXPON + ".2.46.1.1",
	TxBytes:      HuaweiXPON + ".2.46.1.2",
	RxCrcErrors:  HuaweiXPON + ".2.46.1.7",
	TxCrcErrors:  HuaweiXPON + ".2.46.1.8",
}

// Huawei ONT Status values
const (
	HuaweiOntStatusOnline  = 1
	HuaweiOntStatusOffline = 2
)

// HuaweiCollector implements SNMP collection for Huawei OLTs.
type HuaweiCollector struct {
	*BaseCollector
	thresholds OpticalThresholds
}

// NewHuaweiCollector creates a new Huawei SNMP collector.
func NewHuaweiCollector(config DeviceConfig) *HuaweiCollector {
	// Huawei R015+ requires 8+ character community string
	if len(config.Community) > 0 && len(config.Community) < 8 {
		config.Community = config.Community + strings.Repeat("_", 8-len(config.Community))
	}
	
	return &HuaweiCollector{
		BaseCollector: NewBaseCollector(config),
		thresholds:    DefaultOpticalThresholds(),
	}
}

// Vendor returns the vendor type.
func (c *HuaweiCollector) Vendor() Vendor {
	return VendorHuawei
}

// CollectOLTInfo gathers Huawei OLT system information.
func (c *HuaweiCollector) CollectOLTInfo(ctx context.Context) (*OLTInfo, error) {
	info := &OLTInfo{
		Host:        c.config.Host,
		Vendor:      VendorHuawei,
		CollectedAt: time.Now(),
	}

	// Walk OLT control table for basic info
	err := c.Walk(huaweiOltOIDs.ControlTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		switch {
		case strings.HasSuffix(pdu.Name, ".3"):
			info.MAC = ParseMAC(pdu.Value)
		case strings.HasSuffix(pdu.Name, ".5"):
			info.SoftwareVersion = ParseString(pdu.Value)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get OLT info: %w", err)
	}

	return info, nil
}

// CollectPONPorts gathers Huawei PON port information.
func (c *HuaweiCollector) CollectPONPorts(ctx context.Context) ([]PONPort, error) {
	// Huawei PON ports are typically enumerated via ifIndex
	// This is a simplified implementation
	var ports []PONPort
	return ports, nil
}

// CollectONUs gathers Huawei authorized ONT information.
func (c *HuaweiCollector) CollectONUs(ctx context.Context) ([]ONUInfo, error) {
	onuMap := make(map[string]*ONUInfo)
	var mu sync.Mutex

	err := c.Walk(huaweiOntOIDs.InfoTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Huawei index format: ifIndex.ontId
		indices := ExtractIndex(pdu.Name, huaweiOntOIDs.InfoTable)
		if len(indices) < 2 {
			return nil
		}

		ifIndex := indices[len(indices)-2]
		ontId := indices[len(indices)-1]
		key := fmt.Sprintf("%d.%d", ifIndex, ontId)

		mu.Lock()
		onu, exists := onuMap[key]
		if !exists {
			// Decode ifIndex to slot/port (Huawei specific)
			slotID, portID := decodeHuaweiIfIndex(ifIndex)
			onu = &ONUInfo{
				PonIndex: ifIndex,
				OnuIndex: ontId,
				OnuID:    fmt.Sprintf("%d/%d/%d", slotID, portID, ontId),
			}
			onuMap[key] = onu
		}
		mu.Unlock()

		// Extract field based on OID suffix
		baseSuffix := pdu.Name[len(huaweiOntOIDs.InfoTable):]
		switch {
		case strings.HasPrefix(baseSuffix, ".1.3."):
			onu.SerialNumber = ParseString(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.5."):
			onu.Model = ParseString(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.6."):
			status := int(ParseInt64(pdu.Value))
			if status == HuaweiOntStatusOnline {
				onu.Status = "online"
			} else {
				onu.Status = "offline"
			}
		case strings.HasPrefix(baseSuffix, ".1.10."):
			onu.OfflineReason = parseHuaweiDownCause(int(ParseInt64(pdu.Value)))
		case strings.HasPrefix(baseSuffix, ".1.12."):
			onu.Distance = int(ParseInt64(pdu.Value))
		case strings.HasPrefix(baseSuffix, ".1.14."):
			onu.SoftwareVersion = ParseString(pdu.Value)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to collect ONTs: %w", err)
	}

	onus := make([]ONUInfo, 0, len(onuMap))
	for _, onu := range onuMap {
		onus = append(onus, *onu)
	}

	return onus, nil
}

// CollectUnauthONUs gathers Huawei unauthorized ONTs.
func (c *HuaweiCollector) CollectUnauthONUs(ctx context.Context) ([]UnauthONU, error) {
	// Huawei has a separate auto-find table
	return nil, nil
}

// CollectONUOptical gathers Huawei ONT optical power readings.
func (c *HuaweiCollector) CollectONUOptical(ctx context.Context) ([]ONUOptical, error) {
	optMap := make(map[string]*ONUOptical)
	var mu sync.Mutex

	// Collect ONT-side DDM info
	err := c.Walk(huaweiOpticalOIDs.DdmTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		indices := ExtractIndex(pdu.Name, huaweiOpticalOIDs.DdmTable)
		if len(indices) < 2 {
			return nil
		}

		ifIndex := indices[len(indices)-2]
		ontId := indices[len(indices)-1]
		key := fmt.Sprintf("%d.%d", ifIndex, ontId)

		mu.Lock()
		opt, exists := optMap[key]
		if !exists {
			slotID, portID := decodeHuaweiIfIndex(ifIndex)
			opt = &ONUOptical{
				PonIndex: ifIndex,
				OnuIndex: ontId,
				OnuID:    fmt.Sprintf("%d/%d/%d", slotID, portID, ontId),
			}
			optMap[key] = opt
		}
		mu.Unlock()

		baseSuffix := pdu.Name[len(huaweiOpticalOIDs.DdmTable):]
		// Huawei returns optical power in 1/100 dBm units
		switch {
		case strings.HasPrefix(baseSuffix, ".1.1."):
			opt.Temperature = float64(ParseInt64(pdu.Value)) / 100.0
		case strings.HasPrefix(baseSuffix, ".1.2."):
			opt.BiasCurrent = float64(ParseInt64(pdu.Value)) / 1000.0
		case strings.HasPrefix(baseSuffix, ".1.3."):
			opt.TxPowerDBm = ConvertOpticalPower100(ParseInt64(pdu.Value))
		case strings.HasPrefix(baseSuffix, ".1.4."):
			opt.RxPowerDBm = ConvertOpticalPower100(ParseInt64(pdu.Value))
		case strings.HasPrefix(baseSuffix, ".1.5."):
			opt.Voltage = float64(ParseInt64(pdu.Value)) / 1000.0
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to collect ONT optical: %w", err)
	}

	// Collect OLT-side RX power
	err = c.Walk(huaweiOltOpticalOIDs.RxPowerTable, func(pdu gosnmp.SnmpPDU) error {
		indices := ExtractIndex(pdu.Name, huaweiOltOpticalOIDs.RxPowerTable)
		if len(indices) < 2 {
			return nil
		}

		ifIndex := indices[len(indices)-2]
		ontId := indices[len(indices)-1]
		key := fmt.Sprintf("%d.%d", ifIndex, ontId)

		mu.Lock()
		if opt, exists := optMap[key]; exists {
			opt.OltRxDBm = ConvertOpticalPower100(ParseInt64(pdu.Value))
		}
		mu.Unlock()

		return nil
	})

	opticals := make([]ONUOptical, 0, len(optMap))
	for _, opt := range optMap {
		opt.Status = EvaluateOpticalStatus(opt.RxPowerDBm, c.thresholds)
		opticals = append(opticals, *opt)
	}

	return opticals, nil
}

// CollectAll gathers complete Huawei OLT telemetry.
func (c *HuaweiCollector) CollectAll(ctx context.Context) (*OLTTelemetry, error) {
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

func decodeHuaweiIfIndex(ifIndex int) (slotID, portID int) {
	// Huawei ifIndex encoding varies by platform
	// Common format: (4096 * frameId) + (256 * slotId) + (16 * subSlot) + portId
	slotID = (ifIndex / 256) % 16
	portID = ifIndex % 16
	return
}

func parseHuaweiDownCause(cause int) string {
	causes := map[int]string{
		1:  "unknown",
		2:  "los",
		3:  "lof",
		4:  "lopc_miss",
		5:  "dying_gasp",
		6:  "ont_deregister",
		7:  "ont_reboot",
		8:  "losi",
		9:  "lofi",
		10: "loami",
		11: "mem_failure",
		12: "sw_failure",
	}
	if s, ok := causes[cause]; ok {
		return s
	}
	return fmt.Sprintf("unknown(%d)", cause)
}
