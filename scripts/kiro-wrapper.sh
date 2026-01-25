#!/bin/bash

# Kiro CLI Wrapper for Open Agents Bridge
# This script intercepts kiro-cli commands and sends permission requests

SOCKET_PATH="/tmp/open-agents.sock"
REAL_KIRO=$(which kiro-cli 2>/dev/null || echo "/usr/local/bin/kiro-cli")

# Check if Bridge is running
if [ ! -S "$SOCKET_PATH" ]; then
  echo "⚠️  Open Agents Bridge not running. Starting kiro-cli directly..."
  exec "$REAL_KIRO" "$@"
fi

# Parse command to detect tool usage
COMMAND="$*"
SESSION_ID="session_$(date +%s)"

# Detect if this is a tool-using command (simple heuristic)
if echo "$COMMAND" | grep -qE "file|write|read|execute|aws|bash"; then
  echo "🔒 Requesting permission from Open Agents..."
  
  # Send permission request to Bridge
  REQUEST=$(cat <<EOF
{
  "type": "tool_request",
  "toolName": "kiro_command",
  "toolInput": {
    "command": "$COMMAND"
  },
  "sessionId": "$SESSION_ID"
}
EOF
)
  
  # Send to Unix socket and wait for response
  RESPONSE=$(echo "$REQUEST" | nc -U "$SOCKET_PATH" -W 30)
  
  if echo "$RESPONSE" | grep -q '"approved":true'; then
    echo "✅ Permission approved. Executing..."
  else
    echo "❌ Permission denied."
    exit 1
  fi
fi

# Execute real kiro-cli
exec "$REAL_KIRO" "$@"
