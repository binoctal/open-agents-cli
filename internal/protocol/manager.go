package protocol

import (
	"fmt"
	"time"

	"github.com/open-agents/bridge/internal/logger"
)

// Manager manages protocol adapters and auto-detection
type Manager struct {
	adapter  Adapter
	callback func(Message)
}

// NewManager creates a new protocol manager
func NewManager() *Manager {
	return &Manager{}
}

// Connect attempts to connect using the best available protocol
func (m *Manager) Connect(config AdapterConfig) error {
	logger.Info("[Protocol] Auto-detecting protocol for %s", config.Command)

	// Try ACP first
	if err := m.tryACP(config); err == nil {
		logger.Info("[Protocol] Using ACP protocol")
		return nil
	}

	// Fallback to PTY
	logger.Info("[Protocol] ACP failed, falling back to PTY")
	return m.tryPTY(config)
}

// tryACP attempts to connect using ACP protocol
func (m *Manager) tryACP(config AdapterConfig) error {
	adapter := NewACPAdapter()
	adapter.Subscribe(m.callback)

	if err := adapter.Connect(config); err != nil {
		return err
	}

	// Wait for initialization (3 seconds timeout)
	timeout := time.After(3 * time.Second)
	initialized := make(chan bool, 1)

	// Subscribe to messages to detect initialization
	originalCallback := m.callback
	m.callback = func(msg Message) {
		if msg.Type == MessageTypeStatus {
			initialized <- true
		}
		if originalCallback != nil {
			originalCallback(msg)
		}
	}

	select {
	case <-initialized:
		m.adapter = adapter
		m.callback = originalCallback
		return nil
	case <-timeout:
		adapter.Disconnect()
		m.callback = originalCallback
		return fmt.Errorf("ACP initialization timeout")
	}
}

// tryPTY attempts to connect using PTY protocol
func (m *Manager) tryPTY(config AdapterConfig) error {
	adapter := NewPTYAdapter()
	adapter.Subscribe(m.callback)

	if err := adapter.Connect(config); err != nil {
		return err
	}

	m.adapter = adapter
	return nil
}

// Disconnect disconnects the current adapter
func (m *Manager) Disconnect() error {
	if m.adapter == nil {
		return nil
	}
	return m.adapter.Disconnect()
}

// IsConnected returns whether the adapter is connected
func (m *Manager) IsConnected() bool {
	if m.adapter == nil {
		return false
	}
	return m.adapter.IsConnected()
}

// SendMessage sends a message through the current adapter
func (m *Manager) SendMessage(msg Message) error {
	if m.adapter == nil {
		return fmt.Errorf("no adapter connected")
	}
	return m.adapter.SendMessage(msg)
}

// Subscribe sets the message callback
func (m *Manager) Subscribe(callback func(Message)) {
	m.callback = callback
	if m.adapter != nil {
		m.adapter.Subscribe(callback)
	}
}

// GetAdapter returns the current adapter
func (m *Manager) GetAdapter() Adapter {
	return m.adapter
}

// GetProtocolName returns the name of the current protocol
func (m *Manager) GetProtocolName() string {
	if m.adapter == nil {
		return "none"
	}
	return m.adapter.Name()
}
