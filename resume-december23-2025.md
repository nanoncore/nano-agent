# Nano-Agent OLT Adapter Analysis & Improvement Roadmap

**Date:** December 23, 2025
**Scope:** SNMP collectors for Huawei and V-Sol OLTs
**Codebase:** `/Users/mariano/dev/nano-agent/pkg/snmp/`

---

## Current State Summary

### File Inventory

| File | Lines | Purpose |
|------|-------|---------|
| `huawei.go` | 400 | Huawei XPON MIB collector |
| `vsol.go` | 435 | V-Sol OLT collector |
| `zte.go` | 396 | ZTE OLT collector |
| `fiberhome.go` | 531 | FiberHome OLT collector |
| `collector.go` | 320 | Base collector, helpers, factory |
| `manager.go` | 290 | Multi-device concurrent polling |
| `types.go` | 249 | Shared types and interfaces |
| **Total** | **2,621** | |

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      SNMP Manager                           │
│  - Concurrent polling of multiple OLTs                      │
│  - Configurable interval (default 60s)                      │
│  - OnTelemetry/OnError callbacks                            │
│  - Per-device statistics tracking                           │
├─────────────────────────────────────────────────────────────┤
│  HuaweiCollector │ VSOLCollector │ ZTECollector │ FiberHome │
│  - Vendor OIDs   │ - Vendor OIDs │ - Vendor OIDs│ - Vendor  │
│  - Index decode  │ - Index decode│ - Index dec. │ - OIDs    │
│  - Power units   │ - Power parse │ - Power conv │           │
├─────────────────────────────────────────────────────────────┤
│                     BaseCollector                           │
│  - gosnmp wrapper (SNMPv2c / SNMPv3)                        │
│  - Get, Walk, BulkWalk operations                           │
│  - Value parsing helpers (int, string, MAC, IP)             │
│  - Optical power conversion utilities                       │
└─────────────────────────────────────────────────────────────┘
```

---

## Huawei Adapter Deep Dive

### Enterprise OID Structure

```
1.3.6.1.4.1.2011           = Huawei Enterprise
         .6.128.1.1        = XPON MIB
                  .2.21    = OLT Control Table
                  .2.43    = ONT Info Table
                  .2.46    = ONT Traffic Table
                  .2.51    = ONT DDM (Optical) Table
                  .2.52    = OLT RX Power Table
```

### Implemented Features

| Feature | Method | Data Returned |
|---------|--------|---------------|
| OLT Info | `CollectOLTInfo()` | MAC, software version |
| ONT Discovery | `CollectONUs()` | Serial, model, status, distance, offline cause |
| Optical DDM | `CollectONUOptical()` | RX/TX power, OLT RX, temp, voltage, bias |
| Full Telemetry | `CollectAll()` | All of the above aggregated |

### Huawei-Specific Implementation Details

**1. Community String Padding (R015+ requirement)**
```go
// huawei.go:112-115
if len(config.Community) > 0 && len(config.Community) < 8 {
    config.Community = config.Community + strings.Repeat("_", 8-len(config.Community))
}
```

**2. ifIndex Decoding**
```go
// huawei.go:373-378
func decodeHuaweiIfIndex(ifIndex int) (slotID, portID int) {
    // Format: (4096 * frameId) + (256 * slotId) + (16 * subSlot) + portId
    slotID = (ifIndex / 256) % 16
    portID = ifIndex % 16
    return
}
```

**3. Offline Cause Mapping**
```go
// huawei.go:381-400
causes := map[int]string{
    1:  "unknown",
    2:  "los",           // Loss of Signal
    3:  "lof",           // Loss of Frame
    4:  "lopc_miss",     // Loss of PLOAM Cell
    5:  "dying_gasp",    // Power failure at ONT
    6:  "ont_deregister",
    7:  "ont_reboot",
    8:  "losi",          // Loss of Signal (upstream)
    9:  "lofi",          // Loss of Frame (upstream)
    10: "loami",         // Loss of PLOAM (upstream)
    11: "mem_failure",
    12: "sw_failure",
}
```

**4. Optical Power Conversion**
```go
// Huawei returns power in 1/100 dBm units
opt.RxPowerDBm = ConvertOpticalPower100(ParseInt64(pdu.Value))  // collector.go:251-256
```

---

## V-Sol Adapter Current State

### Enterprise OID Structure

```
1.3.6.1.4.1.37950          = V-Sol Enterprise
             .1.1.5        = OLT Base
                  .10.11   = PON Port Table
                  .10.12   = System Stats (CPU, Memory, Temp)
                  .12.1    = ONU Info Table
                  .12.2    = ONU Auth Mode Table
                  .12.8    = ONU Optical Diagnostics
```

### Implemented Features

| Feature | Status | Notes |
|---------|--------|-------|
| OLT System Info | ✅ Complete | CPU, memory, temperature |
| PON Ports | ✅ Complete | Status, ONU count per port |
| ONU Discovery | ✅ Complete | Serial, MAC, status, distance, model |
| Optical DDM | ✅ Complete | RX/TX power, OLT RX, temp, voltage, bias |
| Traffic Stats | ❌ Missing | OIDs exist but not implemented |
| Offline Cause | ❌ Missing | Need to identify OIDs |
| Unauthorized ONUs | ❌ Stub | Returns nil |

---

## Improvement Plan

### Priority 1: V-Sol Traffic Statistics

**Objective:** Add traffic counters (RX/TX bytes, packets, errors) for V-Sol ONUs.

**Research Required:** Identify V-Sol traffic OIDs. Based on the OLT base pattern, likely candidates:

```go
// Proposed addition to vsol.go
var vsolTrafficOIDs = struct {
    TrafficTable string
    RxBytes      string
    TxBytes      string
    RxPackets    string
    TxPackets    string
    RxErrors     string
    TxErrors     string
}{
    TrafficTable: VSOLOltBase + ".12.9",      // Verify via MIB walk
    RxBytes:      VSOLOltBase + ".12.9.1.2",
    TxBytes:      VSOLOltBase + ".12.9.1.3",
    RxPackets:    VSOLOltBase + ".12.9.1.4",
    TxPackets:    VSOLOltBase + ".12.9.1.5",
    RxErrors:     VSOLOltBase + ".12.9.1.6",
    TxErrors:     VSOLOltBase + ".12.9.1.7",
}
```

**Implementation:**

```go
// Add to vsol.go after CollectONUOptical

// CollectONUTraffic gathers V-Sol ONU traffic statistics.
func (c *VSOLCollector) CollectONUTraffic(ctx context.Context) ([]ONUTraffic, error) {
    trafficMap := make(map[string]*ONUTraffic)
    var mu sync.Mutex

    err := c.Walk(vsolTrafficOIDs.TrafficTable, func(pdu gosnmp.SnmpPDU) error {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }

        indices := ExtractIndex(pdu.Name, vsolTrafficOIDs.TrafficTable)
        if len(indices) < 2 {
            return nil
        }

        ponIdx := indices[len(indices)-2]
        onuIdx := indices[len(indices)-1]
        key := fmt.Sprintf("%d.%d", ponIdx, onuIdx)

        mu.Lock()
        traffic, exists := trafficMap[key]
        if !exists {
            slotID := ponIdx / 256
            portID := ponIdx % 256
            traffic = &ONUTraffic{
                PonIndex: ponIdx,
                OnuIndex: onuIdx,
                OnuID:    fmt.Sprintf("%d/%d/%d", slotID, portID, onuIdx),
            }
            trafficMap[key] = traffic
        }
        mu.Unlock()

        switch {
        case strings.Contains(pdu.Name, ".1.2."):
            traffic.RxBytes = ParseUint64(pdu.Value)
        case strings.Contains(pdu.Name, ".1.3."):
            traffic.TxBytes = ParseUint64(pdu.Value)
        case strings.Contains(pdu.Name, ".1.4."):
            traffic.RxPackets = ParseUint64(pdu.Value)
        case strings.Contains(pdu.Name, ".1.5."):
            traffic.TxPackets = ParseUint64(pdu.Value)
        case strings.Contains(pdu.Name, ".1.6."):
            traffic.RxErrors = ParseUint64(pdu.Value)
        case strings.Contains(pdu.Name, ".1.7."):
            traffic.TxErrors = ParseUint64(pdu.Value)
        }

        return nil
    })

    if err != nil {
        return nil, fmt.Errorf("failed to collect ONU traffic: %w", err)
    }

    result := make([]ONUTraffic, 0, len(trafficMap))
    for _, t := range trafficMap {
        result = append(result, *t)
    }

    return result, nil
}
```

**Update CollectAll:**

```go
// In CollectAll, add after optical collection:
traffic, err := c.CollectONUTraffic(ctx)
if err != nil {
    errors = append(errors, fmt.Sprintf("Traffic: %v", err))
} else {
    telemetry.ONUTraffic = traffic
}
```

**Verification Steps:**
1. SSH to V-Sol OLT and run `snmpwalk -v2c -c <community> <host> 1.3.6.1.4.1.37950.1.1.5.12`
2. Identify traffic-related OIDs from the walk output
3. Adjust OID constants based on actual MIB structure

---

### Priority 2: V-Sol Offline Cause

**Objective:** Report why an ONU went offline (LOS, power failure, deregistration, etc.)

**Research Required:** V-Sol typically stores offline cause in the ONU info table or a separate event table.

**Likely OID Candidates:**

```go
// Add to vsolOnuOIDs struct
OfflineReason: VSOLOltBase + ".12.1.1.14",  // Verify via MIB
OfflineTime:   VSOLOltBase + ".12.1.1.15",
```

**Implementation:**

```go
// Add parsing in CollectONUs switch statement:
case strings.Contains(pdu.Name, ".1.14."):
    onu.OfflineReason = parseVSOLOfflineCause(int(ParseInt64(pdu.Value)))

// Add helper function:
func parseVSOLOfflineCause(cause int) string {
    // V-Sol cause codes (verify against actual MIB)
    causes := map[int]string{
        0: "normal",
        1: "los",
        2: "lof",
        3: "dying_gasp",
        4: "deregistered",
        5: "power_off",
        6: "wire_down",
    }
    if s, ok := causes[cause]; ok {
        return s
    }
    return fmt.Sprintf("unknown(%d)", cause)
}
```

**Verification Steps:**
1. Disconnect an ONU physically and observe SNMP walk changes
2. Power off an ONU and compare the cause code
3. Document cause code mappings from V-Sol documentation or empirical testing

---

### Priority 3: Unauthorized ONU Discovery (Both Vendors)

**Objective:** Detect ONUs that are connected but not yet authorized/provisioned.

**Huawei Implementation:**

```go
// Huawei auto-find table OID
var huaweiAutoFindOIDs = struct {
    Table        string
    SerialNumber string
    MAC          string
    FirstSeen    string
}{
    Table:        HuaweiXPON + ".2.44",  // hwGponDeviceAutoFindTable
    SerialNumber: HuaweiXPON + ".2.44.1.3",
    MAC:          HuaweiXPON + ".2.44.1.4",
    FirstSeen:    HuaweiXPON + ".2.44.1.5",
}

func (c *HuaweiCollector) CollectUnauthONUs(ctx context.Context) ([]UnauthONU, error) {
    var onus []UnauthONU

    err := c.Walk(huaweiAutoFindOIDs.Table, func(pdu gosnmp.SnmpPDU) error {
        // Parse auto-find entries
        // ...
    })

    return onus, err
}
```

**V-Sol Implementation:**

```go
// V-Sol unauth table (verify OID)
var vsolUnauthOIDs = struct {
    Table        string
    SerialNumber string
    MAC          string
}{
    Table:        VSOLOltBase + ".12.3",  // Verify
    SerialNumber: VSOLOltBase + ".12.3.1.2",
    MAC:          VSOLOltBase + ".12.3.1.3",
}
```

---

### Priority 4: CLI/NETCONF Integration for Provisioning

**Current State:** Read-only SNMP monitoring. No provisioning capability.

**Objective:** Enable ONU provisioning (add, delete, modify service profiles).

**Options:**

| Protocol | Huawei Support | V-Sol Support | Complexity |
|----------|---------------|---------------|------------|
| CLI (SSH) | Full | Full | Medium |
| NETCONF | Yes (MA5800+) | No | High |
| SNMP SET | Limited | Limited | Low |

**Recommended Approach:** Start with CLI (SSH) as it works for both vendors.

**Architecture:**

```go
// pkg/southbound/cli/driver.go
type CLIDriver struct {
    host     string
    port     int
    username string
    password string
    conn     *ssh.Client
    session  *ssh.Session
}

func (d *CLIDriver) Connect() error
func (d *CLIDriver) Execute(cmd string) (string, error)
func (d *CLIDriver) Close() error

// pkg/southbound/vendors/huawei/cli.go
type HuaweiCLI struct {
    driver *cli.CLIDriver
}

func (h *HuaweiCLI) AddONU(ponPort string, serialNumber string, profile string) error {
    // ont add 0/1/0 sn-auth <serial> omci ont-lineprofile-id 1 ont-srvprofile-id 1
    cmd := fmt.Sprintf("interface gpon 0/%s", ponPort)
    // ...
}

func (h *HuaweiCLI) DeleteONU(ponPort string, onuID int) error {
    // ont delete 0/1/0 <onuID>
}

func (h *HuaweiCLI) GetONUInfo(ponPort string, onuID int) (*ONUDetails, error) {
    // display ont info 0/1/0 <onuID>
}
```

**V-Sol CLI Differences:**

```go
// V-Sol uses different command syntax
func (v *VSOLCli) AddONU(ponPort string, serialNumber string) error {
    // onu add gpon-olt_1/1/1 sn <serial>
}
```

---

## Testing Recommendations

### Unit Tests

```go
// pkg/snmp/huawei_test.go
func TestDecodeHuaweiIfIndex(t *testing.T) {
    tests := []struct {
        ifIndex  int
        wantSlot int
        wantPort int
    }{
        {4352, 1, 0},   // Frame 1, Slot 1, Port 0
        {4368, 1, 0},   // Verify formula
    }
    for _, tt := range tests {
        slot, port := decodeHuaweiIfIndex(tt.ifIndex)
        assert.Equal(t, tt.wantSlot, slot)
        assert.Equal(t, tt.wantPort, port)
    }
}

func TestParseHuaweiDownCause(t *testing.T) {
    assert.Equal(t, "los", parseHuaweiDownCause(2))
    assert.Equal(t, "dying_gasp", parseHuaweiDownCause(5))
    assert.Equal(t, "unknown(99)", parseHuaweiDownCause(99))
}
```

### Integration Tests

```go
// pkg/snmp/integration_test.go (build tag: integration)
func TestHuaweiCollector_RealDevice(t *testing.T) {
    if os.Getenv("HUAWEI_OLT_HOST") == "" {
        t.Skip("HUAWEI_OLT_HOST not set")
    }

    config := DeviceConfig{
        Host:      os.Getenv("HUAWEI_OLT_HOST"),
        Community: os.Getenv("HUAWEI_COMMUNITY"),
        Vendor:    VendorHuawei,
    }

    collector := NewHuaweiCollector(config)
    require.NoError(t, collector.Connect())
    defer collector.Close()

    telemetry, err := collector.CollectAll(context.Background())
    require.NoError(t, err)

    t.Logf("Collected %d ONUs", len(telemetry.ONUs))
    assert.NotEmpty(t, telemetry.ONUs)
}
```

---

## MIB Discovery Commands

### Huawei

```bash
# Full XPON MIB walk
snmpwalk -v2c -c <community> <host> 1.3.6.1.4.1.2011.6.128.1.1

# ONT info table
snmpwalk -v2c -c <community> <host> 1.3.6.1.4.1.2011.6.128.1.1.2.43

# ONT optical DDM
snmpwalk -v2c -c <community> <host> 1.3.6.1.4.1.2011.6.128.1.1.2.51

# Traffic stats
snmpwalk -v2c -c <community> <host> 1.3.6.1.4.1.2011.6.128.1.1.2.46
```

### V-Sol

```bash
# Full enterprise MIB walk
snmpwalk -v2c -c <community> <host> 1.3.6.1.4.1.37950

# ONU info table
snmpwalk -v2c -c <community> <host> 1.3.6.1.4.1.37950.1.1.5.12.1

# ONU optical diagnostics
snmpwalk -v2c -c <community> <host> 1.3.6.1.4.1.37950.1.1.5.12.8

# Search for traffic-related OIDs
snmpwalk -v2c -c <community> <host> 1.3.6.1.4.1.37950.1.1.5.12 | grep -i "byte\|packet\|traffic"
```

---

## Summary of Gaps

| Feature | Huawei | V-Sol | Priority |
|---------|--------|-------|----------|
| Traffic Stats | ✅ Implemented | ❌ Missing | P1 |
| Offline Cause | ✅ Implemented | ❌ Missing | P2 |
| Unauth ONU Discovery | ❌ Stub | ❌ Stub | P3 |
| CLI Provisioning | ❌ Not started | ❌ Not started | P4 |
| NETCONF | ❌ Not started | N/A | P5 |

---

## Next Steps

1. **Immediate:** Run SNMP walks against V-Sol test device to identify traffic and offline cause OIDs
2. **Week 1:** Implement V-Sol traffic stats collection
3. **Week 2:** Implement V-Sol offline cause parsing
4. **Week 3:** Implement unauthorized ONU discovery for both vendors
5. **Future:** Design CLI driver abstraction for provisioning operations

---

*Document generated during codebase analysis session.*
