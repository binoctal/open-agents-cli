package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/open-agents/bridge/internal/protocol"
)

func main() {
	// Create protocol manager
	manager := protocol.NewManager()

	// Subscribe to messages
	manager.Subscribe(func(msg protocol.Message) {
		switch msg.Type {
		case protocol.MessageTypeContent:
			fmt.Printf("[Content] %v\n", msg.Content)

		case protocol.MessageTypeThought:
			fmt.Printf("[Thought] %v\n", msg.Content)

		case protocol.MessageTypeToolCall:
			toolCall := msg.Content.(protocol.ToolCall)
			fmt.Printf("[Tool] %s: %s\n", toolCall.Name, toolCall.Status)

		case protocol.MessageTypePermission:
			permReq := msg.Content.(protocol.PermissionRequest)
			fmt.Printf("[Permission] %s wants to %s\n", permReq.ToolName, permReq.Description)
			// TODO: Forward to Web UI

		case protocol.MessageTypeStatus:
			status := msg.Content.(protocol.AgentStatus)
			fmt.Printf("[Status] %s\n", status)

		case protocol.MessageTypeError:
			fmt.Printf("[Error] %v\n", msg.Content)
		}

		// Show protocol info
		if proto, ok := msg.Meta["protocol"].(string); ok {
			fmt.Printf("  (via %s)\n", proto)
		}
	})

	// Connect with auto-detection
	config := protocol.AdapterConfig{
		WorkDir: ".",
		Command: "claude", // or "qwen-code", "goose", etc.
		Args:    []string{"--experimental-acp"},
		Cols:    120,
		Rows:    30,
	}

	if err := manager.Connect(config); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	fmt.Printf("Connected using protocol: %s\n", manager.GetProtocolName())

	// Send a message
	manager.SendMessage(protocol.Message{
		Type:    protocol.MessageTypeContent,
		Content: "Hello, can you help me?",
	})

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nDisconnecting...")
	manager.Disconnect()
}
