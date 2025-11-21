# nano-agent
Nanoncore edge agent for BNG (Broadband Network Gateway) data plane nodes.

`nano-agent` runs on bare-metal edge nodes and handles:
  - **Enrollment** with the Nanoncore control plane
  - **Configuration sync** from the hosted control plane
  - **Telemetry reporting** (VPP stats, interface counters)
  - **Health monitoring** and heartbeats

  The control plane (UI, API, policy store) is hosted by Nanoncore. You run the edge nodes
  with VPP/DPDK for subscriber traffic.

  ## Installation

  ### Debian/Ubuntu (amd64)

  ```bash
  curl -L
  "https://github.com/nanoncore/nano-agent/releases/latest/download/nano-agent_amd64.deb" -o
   nano-agent.deb
  sudo dpkg -i nano-agent.deb

  Debian/Ubuntu (arm64)

  curl -L
  "https://github.com/nanoncore/nano-agent/releases/latest/download/nano-agent_arm64.deb" -o
   nano-agent.deb
  sudo dpkg -i nano-agent.deb

  Binary (manual)

  curl -L
  "https://github.com/nanoncore/nano-agent/releases/latest/download/nano-agent-linux-amd64"
  -o nano-agent
  chmod +x nano-agent
  sudo mv nano-agent /usr/local/bin/

  Quick Start

  1. Enroll with the control plane:

  sudo nano-agent enroll \
    --api "https://api.nanoncore.com" \
    --token "YOUR_ENROLLMENT_TOKEN" \
    --node-id "$(hostname)" \
    --labels "pop=paris,role=bng"

  2. Check status:

  sudo nano-agent status

  3. Start the service (if installed via .deb):

  sudo systemctl enable --now nano-agent

  Commands

  | Command  | Description                                      |
  |----------|--------------------------------------------------|
  | enroll   | Register this node with the control plane        |
  | status   | Show enrollment status, VPP status, connectivity |
  | unenroll | Remove registration and clear local config       |
  | version  | Print version information                        |

  Configuration

  Configuration is stored in /etc/nano-agent/ after enrollment:
  - config.json - API URL, node ID, labels, certificate paths
  - state.json - Enrollment status, last sync time

  Requirements

  - Linux (amd64 or arm64)
  - Root access (for VPP integration)
  - Network access to api.nanoncore.com

  Recommended (for full BNG functionality)

  - VPP with DPDK plugin
  - SR-IOV capable NICs
  - IOMMU enabled

  Documentation

  - https://docs.nanoncore.com/docs/getting-started
  - https://docs.nanoncore.com/docs/edge-deployment
  - https://docs.nanoncore.com/docs/architecture

  Support

  - Documentation: https://docs.nanoncore.com
  - Issues: https://github.com/nanoncore/nano-agent/issues

  License

  Copyright © 2025 Nanoncore. All rights reserved.
  ```
