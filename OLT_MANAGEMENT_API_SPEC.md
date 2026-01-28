# OLT Management API Specification

## Overview

This document outlines the API endpoints needed to enable OLT (Optical Line Terminal) management through nano-agents. These endpoints would allow the control plane (app.nanoncore.com) to:

1. Configure agents to monitor OLTs
2. Trigger ONU discovery
3. Retrieve metrics and status from OLTs
4. Manage OLT equipment in the network

---

## Architecture

```
┌──────────────────┐         HTTPS          ┌──────────────────────┐
│  Web UI/API      │ ────────────────────> │  Control Plane API   │
│  (User/Postman)  │ <──────────────────── │  (app.nanoncore.com) │
└──────────────────┘                        └──────────────────────┘
                                                      │
                                                      │ Config Push/Pull
                                                      ▼
                                            ┌──────────────────────┐
                                            │   nano-agent         │
                                            │   (Docker/Local)     │
                                            └──────────────────────┘
                                                      │
                                                      │ SNMP/SSH/gNMI
                                                      ▼
                                            ┌──────────────────────┐
                                            │   OLT Simulator      │
                                            │   (172.25.0.2)       │
                                            └──────────────────────┘
```

---

## Current Status

### ✅ What Works
- Agent enrollment
- Agent heartbeats (every 30s)
- Agent shows as ONLINE in control plane
- Agent can manually query OLT via CLI commands

### ❌ What's Missing
- `/api/v1/nodes/{nodeId}/config` endpoint (agent gets 404)
- Equipment management in control plane
- Agent doesn't receive OLT monitoring instructions
- No automatic ONU discovery
- No metrics collection/reporting

---

## Required API Endpoints

### 1. Agent Configuration

#### GET `/api/v1/nodes/{nodeId}/config`

**Purpose**: Agent pulls its configuration (OLTs to monitor, polling intervals, etc.)

**Authentication**: Agent API Key

**Response Example**:
```json
{
  "nodeId": "agent-1",
  "version": 1,
  "olts": [
    {
      "id": "olt-1",
      "name": "OLT Simulator Dev",
      "vendor": "huawei",
      "model": "ma5800",
      "address": "172.25.0.2",
      "protocols": {
        "snmp": {
          "enabled": true,
          "port": 161,
          "community": "public",
          "version": "2c"
        },
        "ssh": {
          "enabled": true,
          "port": 2222,
          "username": "admin",
          "password": "admin"
        }
      },
      "polling": {
        "enabled": true,
        "interval": 300,
        "metrics": ["onu-status", "optical-power", "traffic", "errors"]
      },
      "discovery": {
        "enabled": true,
        "interval": 3600,
        "ponPorts": ["0/0/1", "0/0/2", "0/0/3", "0/0/4", "0/0/5"]
      }
    }
  ]
}
```

**Agent Behavior**:
- Agent calls this endpoint every 5 minutes
- Compares version number to detect configuration changes
- Starts/stops monitoring based on config
- Updates polling intervals dynamically

---

#### PUT `/api/v1/nodes/{nodeId}/config`

**Purpose**: Control plane updates agent configuration

**Authentication**: User session (admin only)

**Request Body**: Same as GET response above

**Response**:
```json
{
  "success": true,
  "version": 2,
  "updated": "2025-12-28T09:30:00Z"
}
```

---

### 2. Equipment Management

#### POST `/api/networks/{networkId}/equipment`

**Purpose**: Add new OLT equipment to the network

**Authentication**: User session (admin only)

**Request**:
```json
{
  "agentId": "cmjpgunzf0007hk0jxs4t5mr1",
  "name": "OLT Simulator Dev",
  "vendor": "huawei",
  "model": "ma5800",
  "type": "olt",
  "address": "172.25.0.2",
  "protocols": {
    "snmp": {
      "enabled": true,
      "port": 161,
      "community": "public"
    },
    "ssh": {
      "enabled": true,
      "port": 2222,
      "username": "admin",
      "password": "admin"
    }
  },
  "location": {
    "site": "Lab",
    "rack": "A1",
    "position": "U42"
  }
}
```

**Response**:
```json
{
  "id": "olt-1",
  "status": "pending",
  "message": "Equipment added. Agent will begin monitoring on next config sync."
}
```

**Backend Actions**:
1. Store equipment in database
2. Update agent config version
3. Agent picks up new config within 5 minutes
4. Agent starts monitoring OLT

---

#### GET `/api/networks/{networkId}/equipment`

**Purpose**: List all equipment in network

**Query Parameters**:
- `type`: Filter by equipment type (`olt`, `onu`, `switch`, etc.)
- `agentId`: Filter by assigned agent
- `status`: Filter by status (`online`, `offline`, `pending`, `error`)

**Response**:
```json
{
  "equipment": [
    {
      "id": "olt-1",
      "name": "OLT Simulator Dev",
      "type": "olt",
      "vendor": "huawei",
      "model": "ma5800",
      "status": "online",
      "agentId": "cmjpgunzf0007hk0jxs4t5mr1",
      "address": "172.25.0.2",
      "lastSeen": "2025-12-28T09:35:00Z",
      "metrics": {
        "uptime": 172800,
        "totalOnus": 40,
        "onlineOnus": 38
      }
    }
  ],
  "total": 1
}
```

**Examples**:
- `GET /api/networks/{networkId}/equipment` - All equipment
- `GET /api/networks/{networkId}/equipment?type=olt` - Only OLTs
- `GET /api/networks/{networkId}/equipment?status=online` - Only online equipment

---

#### GET `/api/networks/{networkId}/equipment/{equipmentId}`

**Purpose**: Get details for specific equipment (OLT, ONU, etc.)

**Response**:
```json
{
  "id": "olt-1",
  "name": "OLT Simulator Dev",
  "type": "olt",
  "vendor": "huawei",
  "model": "ma5800",
  "status": "online",
  "agentId": "cmjpgunzf0007hk0jxs4t5mr1",
  "address": "172.25.0.2",
  "protocols": {
    "snmp": {
      "enabled": true,
      "port": 161,
      "community": "public"
    },
    "ssh": {
      "enabled": true,
      "port": 2222,
      "username": "admin"
    }
  },
  "location": {
    "site": "Lab",
    "rack": "A1",
    "position": "U42"
  },
  "lastSeen": "2025-12-28T09:35:00Z",
  "metrics": {
    "uptime": 172800,
    "cpu": 15.5,
    "memory": 45.2,
    "totalOnus": 40,
    "onlineOnus": 38,
    "offlineOnus": 2
  }
}
```

---

### 3. ONU Discovery & Management

#### POST `/api/networks/{networkId}/agents/{agentId}/discover`

**Purpose**: Trigger immediate ONU discovery

**Request**:
```json
{
  "oltId": "olt-1",
  "ponPorts": ["0/0/1", "0/0/2"],
  "immediate": true
}
```

**Response**:
```json
{
  "jobId": "discovery-12345",
  "status": "queued",
  "message": "Discovery request sent to agent"
}
```

**Agent Implementation**:
- Agent listens for discovery commands via config or websocket
- Executes `nano-agent discover` command
- Reports results back to control plane

---

#### GET `/api/networks/{networkId}/equipment/{oltId}/onus`

**Purpose**: List all ONUs on an OLT

**Response**:
```json
{
  "oltId": "olt-1",
  "onus": [
    {
      "id": "onu-1",
      "serial": "HWTC00000101",
      "model": "HG8245H",
      "status": "online",
      "ponPort": "0/0/1",
      "onuId": 1,
      "distance": 1250,
      "rxPower": -18.5,
      "txPower": 2.3,
      "lastSeen": "2025-12-28T09:35:00Z"
    }
  ],
  "total": 40
}
```

---

### 4. Metrics & Monitoring

#### GET `/api/networks/{networkId}/equipment/{oltId}/metrics`

**Purpose**: Get real-time metrics from OLT

**Query Parameters**:
- `interval`: `1h`, `6h`, `24h`, `7d`
- `metrics`: `cpu,memory,traffic,errors`

**Response**:
```json
{
  "oltId": "olt-1",
  "timestamp": "2025-12-28T09:35:00Z",
  "metrics": {
    "system": {
      "uptime": 172800,
      "cpu": 15.5,
      "memory": 45.2
    },
    "pon": {
      "activePorts": 5,
      "totalOnus": 40,
      "onlineOnus": 38,
      "offlineOnus": 2
    },
    "traffic": {
      "upstream": 125000000,
      "downstream": 850000000,
      "errors": 12
    }
  }
}
```

---

## Implementation Priority

### Phase 1: Configuration Pull (Highest Priority)
1. ✅ Implement `/api/v1/nodes/{nodeId}/config` GET endpoint
2. ✅ Store equipment configuration in database
3. ✅ Agent config sync working without 404 errors

### Phase 2: Equipment Management
1. ✅ Add Equipment CRUD endpoints
2. ✅ Link equipment to agents
3. ✅ Agent starts monitoring based on config

### Phase 3: Discovery & Metrics
1. ✅ Trigger ONU discovery from API
2. ✅ Store discovered ONUs in database
3. ✅ Metrics collection and reporting

### Phase 4: Real-time Updates
1. ✅ WebSocket/Pusher for live updates
2. ✅ Real-time ONU status changes
3. ✅ Alerts and notifications

---

## Database Schema Additions

### Equipment Table
```sql
CREATE TABLE equipment (
  id TEXT PRIMARY KEY,
  network_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL, -- 'olt', 'onu', 'switch', etc.
  vendor TEXT NOT NULL,
  model TEXT,
  address TEXT NOT NULL,
  protocols JSONB,
  status TEXT NOT NULL, -- 'pending', 'online', 'offline', 'error'
  last_seen TIMESTAMP,
  metadata JSONB,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  FOREIGN KEY (network_id) REFERENCES network(id),
  FOREIGN KEY (agent_id) REFERENCES agent(id)
);
```

### ONUs Table
```sql
CREATE TABLE onus (
  id TEXT PRIMARY KEY,
  olt_id TEXT NOT NULL,
  serial TEXT NOT NULL UNIQUE,
  model TEXT,
  pon_port TEXT NOT NULL,
  onu_id INTEGER,
  status TEXT NOT NULL, -- 'online', 'offline', 'pending'
  distance INTEGER,
  rx_power DECIMAL,
  tx_power DECIMAL,
  last_seen TIMESTAMP,
  metadata JSONB,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  FOREIGN KEY (olt_id) REFERENCES equipment(id)
);
```

### Agent Config Table
```sql
CREATE TABLE agent_configs (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL UNIQUE,
  version INTEGER NOT NULL DEFAULT 1,
  config JSONB NOT NULL,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  FOREIGN KEY (agent_id) REFERENCES agent(id)
);
```

---

## Testing Workflow

1. **Add OLT via API** (Postman/UI)
   ```
   POST /api/networks/{networkId}/equipment
   ```

2. **Agent pulls config** (automatic, every 5 min)
   ```
   GET /api/v1/nodes/agent-1/config
   ```

3. **Agent starts monitoring OLT** (automatic)
   - Connects via SNMP/SSH to 172.25.0.2
   - Polls metrics every 5 minutes
   - Discovers ONUs every hour

4. **Trigger discovery manually** (Postman/UI)
   ```
   POST /api/networks/{networkId}/agents/{agentId}/discover
   ```

5. **View discovered ONUs** (Postman/UI)
   ```
   GET /api/networks/{networkId}/equipment/olt-1/onus
   ```

6. **View metrics** (Postman/UI)
   ```
   GET /api/networks/{networkId}/equipment/olt-1/metrics
   ```

---

## Current Working Test Data

**Network ID**: `cmjpgulnd0001hk0jw1jggjhz`
**Agent ID**: `cmjpgunzf0007hk0jxs4t5mr1`
**Agent API Key**: `na_XYrpvy_9n232FnPtrluKVqNMq0nJ-gF6B-0h5wOCfyM`
**OLT IP**: `172.25.0.2` (Docker network)
**OLT Ports**: SNMP 161, SSH 2222

---

## Next Steps

1. Import updated Postman collection
2. Implement `/api/v1/nodes/{nodeId}/config` endpoint first
3. Test agent config pull works
4. Implement equipment management endpoints
5. Test full workflow: Add OLT → Agent monitors → View data

