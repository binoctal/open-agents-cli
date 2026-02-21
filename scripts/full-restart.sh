#!/bin/bash

echo "=== Open Agents Bridge - Full Restart ==="
echo ""

cd "$(dirname "$0")/.."

# 1. Stop old process
echo "🛑 Step 1: Stopping old Bridge..."
pkill -f "open-agents start"
sleep 2

# Check if stopped
if ps aux | grep -q '[o]pen-agents start'; then
    echo "   ⚠️  Process still running, force killing..."
    pkill -9 -f "open-agents start"
    sleep 1
fi
echo "   ✅ Old Bridge stopped"
echo ""

# 2. Rebuild
echo "🔨 Step 2: Rebuilding Bridge..."
if go build -o build/open-agents ./cmd/open-agents/; then
    echo "   ✅ Build successful"
else
    echo "   ❌ Build failed"
    exit 1
fi
echo ""

# 3. Start new process
echo "🚀 Step 3: Starting new Bridge..."
./build/open-agents start > /dev/null 2>&1 &
NEW_PID=$!
sleep 2

# Check if started
if ps -p $NEW_PID > /dev/null; then
    echo "   ✅ Bridge started (PID: $NEW_PID)"
else
    echo "   ❌ Failed to start Bridge"
    exit 1
fi
echo ""

# 4. Verify
echo "📋 Step 4: Verification"
echo "   Process:"
ps aux | grep '[o]pen-agents start' | head -1
echo ""
echo "   Binary:"
ls -lh build/open-agents
echo ""

echo "=== Restart Complete ==="
echo ""
echo "📝 Next steps:"
echo "   1. Check logs: tail -f ~/.open-agents/logs/bridge.log"
echo "   2. Test in Web UI"
echo "   3. Look for [Protocol] and [ACP]/[PTY] in logs"
echo ""
