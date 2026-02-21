#!/bin/bash

echo "=== Open Agents Bridge Diagnostic ==="
echo ""

# Check if bridge is running
echo "1. Bridge Process:"
ps aux | grep '[o]pen-agents start' || echo "   ❌ Bridge not running"
echo ""

# Check bridge binary
echo "2. Bridge Binary:"
if [ -f "./build/open-agents" ]; then
    echo "   ✅ Binary exists"
    ls -lh ./build/open-agents
else
    echo "   ❌ Binary not found"
fi
echo ""

# Check logs
echo "3. Recent Logs:"
if [ -f "$HOME/.open-agents/logs/bridge.log" ]; then
    echo "   Last 10 lines:"
    tail -10 "$HOME/.open-agents/logs/bridge.log"
else
    echo "   ❌ Log file not found"
fi
echo ""

# Check config
echo "4. Config:"
if [ -f "$HOME/.open-agents/config.yaml" ]; then
    echo "   ✅ Config exists"
else
    echo "   ❌ Config not found"
fi
echo ""

echo "=== Recommendations ==="
echo "1. Rebuild: cd bridge && go build -o build/open-agents ./cmd/open-agents/"
echo "2. Restart: ./scripts/restart.sh"
echo "3. Check logs: tail -f ~/.open-agents/logs/bridge.log"
