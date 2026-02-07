# nano-agent

Nanoncore edge agent for BNG (Broadband Network Gateway) data plane nodes.

`nano-agent` runs on bare-metal edge nodes and handles:
- **Enrollment** with the Nanoncore control plane
- **Configuration sync** from the hosted control plane
- **Telemetry reporting** (VPP stats, interface counters)
- **Health monitoring** and heartbeats

The control plane (UI, API, policy store) is hosted by Nanoncore. You run the edge nodes with VPP/DPDK for subscriber traffic.

## Supported Architectures

| Architecture | Binary | .deb Package | Status |
|--------------|--------|--------------|--------|
| linux/amd64  | ✅     | ✅           | Stable |
| linux/arm64  | ✅     | ✅           | Stable |
| linux/riscv64| ✅     | ✅           | Stable |

## Installation

### Debian/Ubuntu (amd64)

```bash
curl -LO "https://github.com/nanoncore/nano-agent/releases/latest/download/nano-agent_amd64.deb"
sudo dpkg -i nano-agent_amd64.deb
```

### Debian/Ubuntu (arm64)

```bash
curl -LO "https://github.com/nanoncore/nano-agent/releases/latest/download/nano-agent_arm64.deb"
sudo dpkg -i nano-agent_arm64.deb
```

### Debian/Ubuntu (riscv64)

```bash
curl -LO "https://github.com/nanoncore/nano-agent/releases/latest/download/nano-agent_riscv64.deb"
sudo dpkg -i nano-agent_riscv64.deb
```

### Binary (manual)

```bash
# amd64
curl -LO "https://github.com/nanoncore/nano-agent/releases/latest/download/nano-agent-linux-amd64"

# arm64
curl -LO "https://github.com/nanoncore/nano-agent/releases/latest/download/nano-agent-linux-arm64"

# riscv64
curl -LO "https://github.com/nanoncore/nano-agent/releases/latest/download/nano-agent-linux-riscv64"

chmod +x nano-agent-linux-*
sudo mv nano-agent-linux-* /usr/local/bin/nano-agent
```

## Quick Start

1. Enroll with the control plane:

```bash
sudo nano-agent enroll \
  --api "https://api.nanoncore.com" \
  --token "YOUR_ENROLLMENT_TOKEN" \
  --node-id "$(hostname)" \
  --labels "pop=paris,role=bng"
```

2. Check status:

```bash
sudo nano-agent status
```

3. Start the service (if installed via .deb):

```bash
sudo systemctl enable --now nano-agent
```

## Commands

| Command  | Description                                      |
|----------|--------------------------------------------------|
| enroll   | Register this node with the control plane        |
| status   | Show enrollment status, VPP status, connectivity |
| unenroll | Remove registration and clear local config       |
| version  | Print version information                        |

## Configuration

Configuration is stored in `/etc/nano-agent/` after enrollment:
- `config.json` - API URL, node ID, labels, certificate paths
- `state.json` - Enrollment status, last sync time

## Requirements

- Linux (amd64, arm64, or riscv64)
- Root access (for VPP integration)
- Network access to api.nanoncore.com

### Recommended (for full BNG functionality)

- VPP with DPDK plugin
- SR-IOV capable NICs
- IOMMU enabled

## Documentation

- https://docs.nanoncore.com/docs/getting-started
- https://docs.nanoncore.com/docs/edge-deployment
- https://docs.nanoncore.com/docs/architecture

## Security Scanning (Local)

Run the same Trivy scan used in CI:

```bash
./scripts/trivy.sh
```

## Support

- Documentation: https://docs.nanoncore.com
- Issues: https://github.com/nanoncore/nano-agent/issues

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
