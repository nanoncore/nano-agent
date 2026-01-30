package snmp

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

// BaseCollector provides common SNMP functionality for all vendors.
type BaseCollector struct {
	config DeviceConfig
	client *gosnmp.GoSNMP
	mu     sync.Mutex
}

// NewBaseCollector creates a new base SNMP collector.
func NewBaseCollector(config DeviceConfig) *BaseCollector {
	if config.Port == 0 {
		config.Port = 161
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.Retries == 0 {
		config.Retries = 2
	}
	if config.Version == "" {
		config.Version = SNMPv2c
	}

	return &BaseCollector{
		config: config,
	}
}

// Connect establishes the SNMP connection.
func (c *BaseCollector) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		return nil // Already connected
	}

	client := &gosnmp.GoSNMP{
		Target:    c.config.Host,
		Port:      c.config.Port,
		Community: c.config.Community,
		Timeout:   c.config.Timeout,
		Retries:   c.config.Retries,
	}

	switch c.config.Version {
	case SNMPv2c:
		client.Version = gosnmp.Version2c
	case SNMPv3:
		client.Version = gosnmp.Version3
		client.SecurityModel = gosnmp.UserSecurityModel
		client.MsgFlags = gosnmp.AuthPriv
		client.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 c.config.Username,
			AuthenticationProtocol:   parseAuthProtocol(c.config.AuthProtocol),
			AuthenticationPassphrase: c.config.AuthPassword,
			PrivacyProtocol:          parsePrivProtocol(c.config.PrivProtocol),
			PrivacyPassphrase:        c.config.PrivPassword,
		}
	default:
		client.Version = gosnmp.Version2c
	}

	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect to %s: %w", c.config.Host, err)
	}

	c.client = client
	return nil
}

// Close terminates the SNMP connection.
func (c *BaseCollector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil && c.client.Conn != nil {
		err := c.client.Conn.Close()
		c.client = nil
		return err
	}
	return nil
}

// Config returns the device configuration.
func (c *BaseCollector) Config() DeviceConfig {
	return c.config
}

// Get performs an SNMP GET operation.
func (c *BaseCollector) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}

	return c.client.Get(oids)
}

// Walk performs an SNMP walk operation.
func (c *BaseCollector) Walk(rootOid string, walkFn gosnmp.WalkFunc) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return fmt.Errorf("not connected")
	}

	return c.client.Walk(rootOid, walkFn)
}

// BulkWalk performs an SNMP bulk walk operation (more efficient).
func (c *BaseCollector) BulkWalk(rootOid string, walkFn gosnmp.WalkFunc) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return fmt.Errorf("not connected")
	}

	return c.client.BulkWalk(rootOid, walkFn)
}

// Helper functions for parsing SNMP values

// ParseInt64 extracts an int64 from an SNMP value.
func ParseInt64(value interface{}) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case uint:
		return int64(v)
	case uint64:
		return int64(v)
	case uint32:
		return int64(v)
	case int32:
		return int64(v)
	default:
		return gosnmp.ToBigInt(value).Int64()
	}
}

// ParseUint64 extracts a uint64 from an SNMP value.
func ParseUint64(value interface{}) uint64 {
	switch v := value.(type) {
	case uint:
		return uint64(v)
	case uint64:
		return v
	case uint32:
		return uint64(v)
	case int:
		return uint64(v)
	case int64:
		return uint64(v)
	default:
		return gosnmp.ToBigInt(value).Uint64()
	}
}

// ParseString extracts a string from an SNMP value.
func ParseString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ParseMAC extracts a MAC address from an SNMP value.
func ParseMAC(value interface{}) string {
	switch v := value.(type) {
	case []byte:
		if len(v) == 6 {
			return net.HardwareAddr(v).String()
		}
		return fmt.Sprintf("%x", v)
	case string:
		return v
	default:
		return ""
	}
}

// ParseIP extracts an IP address from an SNMP value.
func ParseIP(value interface{}) net.IP {
	switch v := value.(type) {
	case []byte:
		if len(v) == 4 || len(v) == 16 {
			return net.IP(v)
		}
	case string:
		return net.ParseIP(v)
	}
	return nil
}

// ExtractIndex extracts numeric indices from an OID suffix.
func ExtractIndex(oid, baseOid string) []int {
	suffix := strings.TrimPrefix(oid, baseOid)
	suffix = strings.TrimPrefix(suffix, ".")

	if suffix == "" {
		return nil
	}

	parts := strings.Split(suffix, ".")
	indices := make([]int, 0, len(parts))
	for _, p := range parts {
		if idx, err := strconv.Atoi(p); err == nil {
			indices = append(indices, idx)
		}
	}
	return indices
}

// ExtractLastIndex gets the last numeric index from an OID.
func ExtractLastIndex(oid string) int {
	parts := strings.Split(oid, ".")
	if len(parts) == 0 {
		return 0
	}
	idx, _ := strconv.Atoi(parts[len(parts)-1])
	return idx
}

// Optical power conversion helpers

// ConvertOpticalPower100 converts optical power from 1/100 dBm units.
func ConvertOpticalPower100(raw int64) float64 {
	if raw == -32768 || raw == 0x7FFF || raw == 0xFFFF {
		return -40.0 // No signal / invalid
	}
	return float64(raw) / 100.0
}

// ConvertOpticalPower1000 converts optical power from 0.001 dBm units.
func ConvertOpticalPower1000(raw int64) float64 {
	if raw == -32768 || raw == 0x7FFF || raw == 0x7FFFFFFF {
		return -40.0 // No signal / invalid
	}
	return float64(raw) / 1000.0
}

// ConvertMWtoDBm converts milliwatts to dBm.
func ConvertMWtoDBm(mw float64) float64 {
	if mw <= 0 {
		return -40.0
	}
	return 10 * math.Log10(mw)
}

// Protocol parsing helpers

func parseAuthProtocol(proto string) gosnmp.SnmpV3AuthProtocol {
	switch strings.ToUpper(proto) {
	case "MD5":
		return gosnmp.MD5
	case "SHA":
		return gosnmp.SHA
	case "SHA256":
		return gosnmp.SHA256
	case "SHA512":
		return gosnmp.SHA512
	default:
		return gosnmp.NoAuth
	}
}

func parsePrivProtocol(proto string) gosnmp.SnmpV3PrivProtocol {
	switch strings.ToUpper(proto) {
	case "DES":
		return gosnmp.DES
	case "AES":
		return gosnmp.AES
	case "AES192":
		return gosnmp.AES192
	case "AES256":
		return gosnmp.AES256
	default:
		return gosnmp.NoPriv
	}
}

// NewCollector creates a vendor-specific collector based on configuration.
// Note: V-SOL SNMP is handled via nano-southbound, not this legacy package.
func NewCollector(config DeviceConfig) (Collector, error) {
	switch config.Vendor {
	case VendorHuawei:
		return NewHuaweiCollector(config), nil
	case VendorZTE:
		return NewZTECollector(config), nil
	case VendorFiberHome:
		return NewFiberHomeCollector(config), nil
	default:
		return nil, fmt.Errorf("unsupported vendor: %s", config.Vendor)
	}
}
