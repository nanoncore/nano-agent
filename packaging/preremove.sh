#!/bin/bash
set -e

# Stop and disable the service if running
if systemctl is-active --quiet nano-agent; then
    systemctl stop nano-agent
fi

if systemctl is-enabled --quiet nano-agent 2>/dev/null; then
    systemctl disable nano-agent
fi

echo "Nanoncore Edge Agent service stopped and disabled"
