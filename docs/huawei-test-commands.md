# Huawei ONU Management - Test Commands

This document contains step-by-step instructions for testing all nano-agent ONU management features using Docker containers.

---

## Prerequisites

- Docker Desktop installed and running
- Access to both `nano-agent` and `olt-simulator` repositories

---

## Step 1: Start OLT Simulator

```bash
cd /Users/marksonrebelomarcolino/nanoncore/olt-simulator
docker build -t olt-simulator:latest .
docker stop olt-sim 2>/dev/null; docker rm olt-sim 2>/dev/null
docker run -d --name olt-sim \
  -p 161:161/udp \
  -p 2222:2222 \
  -p 8081:8080 \
  olt-simulator:latest
sleep 5
docker ps --filter name=olt-sim
```

---

## Step 2: Build nano-agent Docker Image

```bash
cd /Users/marksonrebelomarcolino/nanoncore/nano-agent
CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -o nano-agent ./cmd/nano-agent
docker build -t nano-agent:latest .
```

---

## Step 3: Run Test Commands

Each command below can be copied and run directly from your terminal.

### 3.1 List Existing ONUs (Before Provisioning)

```bash
docker run --rm nano-agent:latest onu-list \
  --vendor huawei \
  --address host.docker.internal \
  --port 161 \
  --protocol snmp \
  --username admin \
  --password admin
```

### 3.2 Provision New ONU

```bash
docker run --rm nano-agent:latest onu-provision \
  --serial HWTCAABB1234 \
  --pon-port 0/0/1 \
  --onu-id 55 \
  --vlan 100 \
  --bandwidth-down 100 \
  --bandwidth-up 50 \
  --vendor huawei \
  --address host.docker.internal \
  --port 2222 \
  --username admin \
  --password admin
```

Expected output:
```
ONU Provisioning
================
...
Provisioning Complete
---------------------
  Subscriber ID:   HWTCAABB1234
  Session ID:      ont-0/0/1-55
  Status:          Success
```

### 3.3 List ONUs (After Provisioning)

```bash
docker run --rm nano-agent:latest onu-list \
  --vendor huawei \
  --address host.docker.internal \
  --port 161 \
  --protocol snmp \
  --username admin \
  --password admin
```

Verify `HWTCAABB1234` appears in the list.

### 3.4 Get ONU Info by Serial

```bash
docker run --rm nano-agent:latest onu-info \
  --serial HWTCAABB1234 \
  --vendor huawei \
  --address host.docker.internal \
  --port 161 \
  --protocol snmp \
  --username admin \
  --password admin
```

Expected output:
```
Registration
------------
  Serial:          HWTCAABB1234
  PON Port:        0/0/1
  ONU ID:          55
```

### 3.5 Get ONU Info by Port/ID

```bash
docker run --rm nano-agent:latest onu-info \
  --pon-port 0/0/1 \
  --onu-id 55 \
  --vendor huawei \
  --address host.docker.internal \
  --port 161 \
  --protocol snmp \
  --username admin \
  --password admin
```

### 3.6 Reboot ONU

```bash
docker run --rm nano-agent:latest onu-reboot \
  --serial HWTCAABB1234 \
  --vendor huawei \
  --address host.docker.internal \
  --port 2222 \
  --username admin \
  --password admin
```

Expected output:
```
ONU reboot initiated
  PON Port: 0/0/1
  ONU ID:   55
```

### 3.7 Verify ONU Still Exists After Reboot

```bash
docker run --rm nano-agent:latest onu-info \
  --serial HWTCAABB1234 \
  --vendor huawei \
  --address host.docker.internal \
  --port 161 \
  --protocol snmp \
  --username admin \
  --password admin
```

### 3.8 Delete ONU

```bash
docker run --rm nano-agent:latest onu-delete \
  --serial HWTCAABB1234 \
  --force \
  --vendor huawei \
  --address host.docker.internal \
  --port 2222 \
  --username admin \
  --password admin
```

Expected output:
```
ONU deleted successfully
  PON Port: 0/0/1
  ONU ID:   55
```

### 3.9 Verify ONU Deleted

```bash
docker run --rm nano-agent:latest onu-info \
  --serial HWTCAABB1234 \
  --vendor huawei \
  --address host.docker.internal \
  --port 161 \
  --protocol snmp \
  --username admin \
  --password admin
```

Expected output:
```
Looking up ONU by serial... NOT FOUND
Error: ONU with serial HWTCAABB1234 not found
```

### 3.10 List ONUs (After Deletion)

```bash
docker run --rm nano-agent:latest onu-list \
  --vendor huawei \
  --address host.docker.internal \
  --port 161 \
  --protocol snmp \
  --username admin \
  --password admin
```

Verify `HWTCAABB1234` no longer appears in the list.

---

## Step 4: Additional Test Scenarios

### Query Pre-existing ONU

```bash
docker run --rm nano-agent:latest onu-info \
  --serial HWTC00000101 \
  --vendor huawei \
  --address host.docker.internal \
  --port 161 \
  --protocol snmp \
  --username admin \
  --password admin
```

### JSON Output

```bash
docker run --rm nano-agent:latest onu-info \
  --serial HWTC00000101 \
  --vendor huawei \
  --address host.docker.internal \
  --port 161 \
  --protocol snmp \
  --username admin \
  --password admin \
  --json
```

### Dry Run (Preview Provisioning)

```bash
docker run --rm nano-agent:latest onu-provision \
  --serial HWTCTEST9999 \
  --pon-port 0/0/1 \
  --onu-id 200 \
  --vlan 100 \
  --vendor huawei \
  --address host.docker.internal \
  --port 2222 \
  --username admin \
  --password admin \
  --dry-run
```

---

## Step 5: Cleanup

```bash
docker stop olt-sim
docker rm olt-sim
```

---

## Troubleshooting

### Container Not Starting
```bash
docker logs olt-sim
```

### SNMP Timeout
```bash
docker run --rm --entrypoint snmpget nano-agent:latest -v2c -c public host.docker.internal:161 1.3.6.1.2.1.1.1.0
```

### SSH Connection Failed
```bash
ssh -p 2222 admin@localhost
# Password: admin
```

### ONU ID Range
The OLT simulator restricts ONU IDs to range **0-255**.

### Serial Number Format
Serial numbers must be exactly **12 characters**: 4 uppercase letters + 8 hex digits.
- Valid: `HWTC12345678`, `ZTEGAABBCCDD`
- Invalid: `HWTC123`, `hwtc12345678`

---

## Quick Reference

| Service | Host Port | Protocol |
|---------|-----------|----------|
| SNMP    | 161       | UDP      |
| SSH/CLI | 2222      | TCP      |
| REST API| 8081      | TCP      |
