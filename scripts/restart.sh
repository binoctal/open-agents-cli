#!/bin/bash

echo "🔄 Restarting Bridge..."

# Find and kill existing bridge process
PID=$(ps aux | grep '[o]pen-agents start' | awk '{print $2}')
if [ ! -z "$PID" ]; then
    echo "🛑 Stopping old Bridge (PID: $PID)"
    kill $PID
    sleep 2
fi

# Start new bridge
echo "🚀 Starting new Bridge..."
cd "$(dirname "$0")/.."
./build/open-agents start &

echo "✅ Bridge restarted"
echo "📋 Check logs: tail -f ~/.open-agents/logs/bridge.log"
