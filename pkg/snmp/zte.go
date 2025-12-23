package snmp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

// ZTE Enterprise OID and base paths
const (
	ZTEEnterprise = "1.3.6.1.4.1.3902"
	ZTEC300GPON   = ZTEEnterprise + ".1012" // GPON
	ZTEC300EPON   = ZTEEnterprise + ".1015" // EPON
	ZTEAccessNode = ZTEEnterprise + ".1082" // Titan/AccessNode series
)

// ZTE System OIDs
var zteSystemOIDs = struct {
	Temperature string
	CPUUsage    string
	MemUsage    string
}{
	Temperature: ZTEAccessNode + ".500.1.2.2.1.8",
	CPUUsage:    ZTEAccessNode + ".500.1.2.2.1.9",
	MemUsage:    ZTEAccessNode + ".500.1.2.2.1.10",
}

// ZTE Card OIDs
var zteCardOIDs = struct {
	CardTable       string
	CardType        string
	CardStatus      string
	CardTemperature string
}{
	CardTable:       ZTEAccessNode + ".500.1.2.1",
	CardType:        ZTEAccessNode + ".500.1.2.1.1.2",
	CardStatus:      ZTEAccessNode + ".500.1.2.1.1.5",
	CardTemperature: ZTEAccessNode + ".500.1.2.1.1.8",
}

// ZTE ONU OIDs
var zteOnuOIDs = struct {
	InfoTable         string
	SerialNumber      string
	MAC               string
	RunStatus         string
	Distance          string
	Type              string
	SoftwareVersion   string
	LastOnlineTime    string
	LastOfflineTime   string
	LastOfflineReason string
}{
	InfoTable:         ZTEAccessNode + ".500.1.2.4.1",
	SerialNumber:      ZTEAccessNode + ".500.1.2.4.1.1.3",
	MAC:               ZTEAccessNode + ".500.1.2.4.1.1.4",
	RunStatus:         ZTEAccessNode + ".500.1.2.4.1.1.5",
	Distance:          ZTEAccessNode + ".500.1.2.4.1.1.10",
	Type:              ZTEAccessNode + ".500.1.2.4.1.1.6",
	SoftwareVersion:   ZTEAccessNode + ".500.1.2.4.1.1.8",
	LastOnlineTime:    ZTEAccessNode + ".500.1.2.4.1.1.11",
	LastOfflineTime:   ZTEAccessNode + ".500.1.2.4.1.1.12",
	LastOfflineReason: ZTEAccessNode + ".500.1.2.4.1.1.13",
}

// ZTE Optical OIDs
var zteOpticalOIDs = struct {
	RxPowerTable string
	RxPower      string
	TxPowerTable string
	TxPower      string
	OltRxPower   string
}{
	RxPowerTable: ZTEAccessNode + ".500.1.2.4.2",
	RxPower:      ZTEAccessNode + ".500.1.2.4.2.1.2",
	TxPowerTable: ZTEAccessNode + ".500.1.2.4.3",
	TxPower:      ZTEAccessNode + ".500.1.2.4.3.1.2",
	OltRxPower:   ZTEAccessNode + ".500.1.2.4.2.1.3",
}

// ZTE Traffic OIDs
var zteTrafficOIDs = struct {
	TrafficTable string
	RxBytes      string
	TxBytes      string
	RxPackets    string
	TxPackets    string
}{
	TrafficTable: ZTEAccessNode + ".500.1.2.4.5",
	RxBytes:      ZTEAccessNode + ".500.1.2.4.5.1.2",
	TxBytes:      ZTEAccessNode + ".500.1.2.4.5.1.3",
	RxPackets:    ZTEAccessNode + ".500.1.2.4.5.1.4",
	TxPackets:    ZTEAccessNode + ".500.1.2.4.5.1.5",
}

// ZTE ONU Status values
const (
	ZTEOnuStatusOnline      = 1
	ZTEOnuStatusOffline     = 2
	ZTEOnuStatusRegistering = 3
)

// ZTECollector implements SNMP collection for ZTE OLTs.
type ZTECollector struct {
	*BaseCollector
	thresholds OpticalThresholds
}

// NewZTECollector creates a new ZTE SNMP collector.
func NewZTECollector(config DeviceConfig) *ZTECollector {
	return &ZTECollector{
		BaseCollector: NewBaseCollector(config),
		thresholds:    DefaultOpticalThresholds(),
	}
}

// Vendor returns the vendor type.
func (c *ZTECollector) Vendor() Vendor {
	return VendorZTE
}

// CollectOLTInfo gathers ZTE OLT system information.
func (c *ZTECollector) CollectOLTInfo(ctx context.Context) (*OLTInfo, error) {
	oids := []string{
		zteSystemOIDs.Temperature + ".0",
		zteSystemOIDs.CPUUsage + ".0",
		zteSystemOIDs.MemUsage + ".0",
	}

	result, err := c.Get(oids)
	if err != nil {
		return nil, fmt.Errorf("failed to get OLT info: %w", err)
	}

	info := &OLTInfo{
		Host:        c.config.Host,
		Vendor:      VendorZTE,
		CollectedAt: time.Now(),
	}

	for _, variable := range result.Variables {
		if variable.Type == gosnmp.NoSuchObject || variable.Type == gosnmp.NoSuchInstance {
			continue
		}

		switch {
		case strings.Contains(variable.Name, zteSystemOIDs.Temperature):
			info.Temperature = float64(ParseInt64(variable.Value))
		case strings.Contains(variable.Name, zteSystemOIDs.CPUUsage):
			info.CPUPercent = float64(ParseInt64(variable.Value))
		case strings.Contains(variable.Name, zteSystemOIDs.MemUsage):
			info.MemoryPercent = float64(ParseInt64(variable.Value))
		}
	}

	return info, nil
}

// CollectPONPorts gathers ZTE PON port information.
func (c *ZTECollector) CollectPONPorts(ctx context.Context) ([]PONPort, error) {
	var ports []PONPort
	// ZTE PON port enumeration via card table
	return ports, nil
}

// CollectONUs gathers ZTE authorized ONU information.
func (c *ZTECollector) CollectONUs(ctx context.Context) ([]ONUInfo, error) {
	onuMap := make(map[string]*ONUInfo)
	var mu sync.Mutex

	err := c.Walk(zteOnuOIDs.InfoTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// ZTE index format: rack.shelf.slot.port.onu
		indices := ExtractIndex(pdu.Name, zteOnuOIDs.InfoTable)
		if len(indices) < 4 {
			return nil
		}

		// Use last 4 indices: slot.port.onu (simplified)
		slot := indices[len(indices)-3]
		port := indices[len(indices)-2]
		onuId := indices[len(indices)-1]
		ponIdx := slot*1000 + port
		key := fmt.Sprintf("%d.%d", ponIdx, onuId)

		mu.Lock()
		onu, exists := onuMap[key]
		if !exists {
			onu = &ONUInfo{
				PonIndex: ponIdx,
				OnuIndex: onuId,
				OnuID:    fmt.Sprintf("%d/%d/%d", slot, port, onuId),
			}
			onuMap[key] = onu
		}
		mu.Unlock()

		baseSuffix := pdu.Name[len(zteOnuOIDs.InfoTable):]
		switch {
		case strings.HasPrefix(baseSuffix, ".1.3."):
			onu.SerialNumber = ParseString(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.4."):
			onu.MAC = ParseMAC(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.5."):
			status := int(ParseInt64(pdu.Value))
			switch status {
			case ZTEOnuStatusOnline:
				onu.Status = "online"
			case ZTEOnuStatusRegistering:
				onu.Status = "registering"
			default:
				onu.Status = "offline"
			}
		case strings.HasPrefix(baseSuffix, ".1.6."):
			onu.Type = ParseString(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.8."):
			onu.SoftwareVersion = ParseString(pdu.Value)
		case strings.HasPrefix(baseSuffix, ".1.10."):
			onu.Distance = int(ParseInt64(pdu.Value))
		case strings.HasPrefix(baseSuffix, ".1.13."):
			onu.OfflineReason = parseZTEDownCause(int(ParseInt64(pdu.Value)))
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

// CollectUnauthONUs gathers ZTE unauthorized ONUs.
func (c *ZTECollector) CollectUnauthONUs(ctx context.Context) ([]UnauthONU, error) {
	return nil, nil
}

// CollectONUOptical gathers ZTE ONU optical power readings.
func (c *ZTECollector) CollectONUOptical(ctx context.Context) ([]ONUOptical, error) {
	optMap := make(map[string]*ONUOptical)
	var mu sync.Mutex

	// Collect ONU RX power
	err := c.Walk(zteOpticalOIDs.RxPowerTable, func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		indices := ExtractIndex(pdu.Name, zteOpticalOIDs.RxPowerTable)
		if len(indices) < 4 {
			return nil
		}

		slot := indices[len(indices)-3]
		port := indices[len(indices)-2]
		onuId := indices[len(indices)-1]
		ponIdx := slot*1000 + port
		key := fmt.Sprintf("%d.%d", ponIdx, onuId)

		mu.Lock()
		opt, exists := optMap[key]
		if !exists {
			opt = &ONUOptical{
				PonIndex: ponIdx,
				OnuIndex: onuId,
				OnuID:    fmt.Sprintf("%d/%d/%d", slot, port, onuId),
			}
			optMap[key] = opt
		}
		mu.Unlock()

		baseSuffix := pdu.Name[len(zteOpticalOIDs.RxPowerTable):]
		// ZTE returns optical power in 0.001 dBm units
		switch {
		case strings.HasPrefix(baseSuffix, ".1.2."):
			opt.RxPowerDBm = ConvertOpticalPower1000(ParseInt64(pdu.Value))
		case strings.HasPrefix(baseSuffix, ".1.3."):
			opt.OltRxDBm = ConvertOpticalPower1000(ParseInt64(pdu.Value))
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to collect ONU optical RX: %w", err)
	}

	// Collect ONU TX power
	err = c.Walk(zteOpticalOIDs.TxPowerTable, func(pdu gosnmp.SnmpPDU) error {
		indices := ExtractIndex(pdu.Name, zteOpticalOIDs.TxPowerTable)
		if len(indices) < 4 {
			return nil
		}

		slot := indices[len(indices)-3]
		port := indices[len(indices)-2]
		onuId := indices[len(indices)-1]
		ponIdx := slot*1000 + port
		key := fmt.Sprintf("%d.%d", ponIdx, onuId)

		mu.Lock()
		if opt, exists := optMap[key]; exists {
			baseSuffix := pdu.Name[len(zteOpticalOIDs.TxPowerTable):]
			if strings.HasPrefix(baseSuffix, ".1.2.") {
				opt.TxPowerDBm = ConvertOpticalPower1000(ParseInt64(pdu.Value))
			}
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

// CollectAll gathers complete ZTE OLT telemetry.
func (c *ZTECollector) CollectAll(ctx context.Context) (*OLTTelemetry, error) {
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

func parseZTEDownCause(cause int) string {
	causes := map[int]string{
		1:  "power_off",
		2:  "los",
		3:  "lof",
		4:  "loam",
		5:  "dying_gasp",
		6:  "deregister",
		7:  "onu_deactive",
		8:  "ranging_fail",
		9:  "rogue_onu",
		10: "upstream_fail",
	}
	if s, ok := causes[cause]; ok {
		return s
	}
	return fmt.Sprintf("unknown(%d)", cause)
}
