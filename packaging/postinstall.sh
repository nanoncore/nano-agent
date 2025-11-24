#!/bin/bash
set -e

# Reload systemd
systemctl daemon-reload

echo ""
echo "============================================"
echo "  Nanoncore Edge Agent installed!"
echo "============================================"
echo ""
echo "To enroll this node with the control plane:"
echo ""
echo "  sudo nano-agent enroll \\"
echo "    --api https://api.nanoncore.com \\"
echo "    --token YOUR_ENROLLMENT_TOKEN \\"
echo "    --node-id \$(hostname) \\"
echo "    --labels \"pop=YOUR_POP,role=bng\""
echo ""
echo "Then start the agent service:"
echo ""
echo "  sudo systemctl enable --now nano-agent"
echo ""
echo "Check status with:"
echo ""
echo "  sudo nano-agent status"
echo "  sudo systemctl status nano-agent"
echo ""
