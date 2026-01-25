package adapter

import (
	"github.com/open-agents/bridge/internal/acp"
	"github.com/open-agents/bridge/internal/hook"
)

// AdapterType defines the type of CLI adapter
type AdapterType string

const (
	AdapterTypeWrapper AdapterType = "wrapper"  // Wrapper script
	AdapterTypeHook    AdapterType = "hook"     // Hook/Plugin
	AdapterTypeACP     AdapterType = "acp"      // ACP Protocol
)

// Adapter interface for different CLI integration methods
type Adapter interface {
	Start() error
	Stop() error
	SendMessage(content string) error
	OnToolCall(handler func(toolName string, input map[string]interface{}))
}

// WrapperAdapter uses wrapper script
type WrapperAdapter struct {
	// Implemented in existing code
}

// HookAdapter uses hook server
type HookAdapter struct {
	server *hook.HookServer
}

// ACPAdapter uses ACP protocol
type ACPAdapter struct {
	client *acp.ACPClient
}

// NewAdapter creates appropriate adapter based on CLI capabilities
func NewAdapter(cliType string, config map[string]interface{}) (Adapter, error) {
	// Auto-detect best adapter type
	adapterType := detectAdapterType(cliType, config)
	
	switch adapterType {
	case AdapterTypeACP:
		return NewACPAdapter(config)
	case AdapterTypeHook:
		return NewHookAdapter(config)
	default:
		return NewWrapperAdapter(config)
	}
}

func detectAdapterType(cliType string, config map[string]interface{}) AdapterType {
	// Check if CLI supports ACP
	if supportsACP, ok := config["supportsACP"].(bool); ok && supportsACP {
		return AdapterTypeACP
	}
	
	// Check if CLI supports hooks
	if supportsHooks, ok := config["supportsHooks"].(bool); ok && supportsHooks {
		return AdapterTypeHook
	}
	
	// Default to wrapper
	return AdapterTypeWrapper
}

func NewACPAdapter(config map[string]interface{}) (*ACPAdapter, error) {
	command := config["command"].(string)
	args := config["args"].([]string)
	
	client, err := acp.NewACPClient(command, args)
	if err != nil {
		return nil, err
	}
	
	if err := client.Initialize(); err != nil {
		return nil, err
	}
	
	return &ACPAdapter{client: client}, nil
}

func (a *ACPAdapter) Start() error {
	return nil // Already started in NewACPAdapter
}

func (a *ACPAdapter) Stop() error {
	return a.client.Close()
}

func (a *ACPAdapter) SendMessage(content string) error {
	return a.client.SendMessage(content)
}

func (a *ACPAdapter) OnToolCall(handler func(string, map[string]interface{})) {
	a.client.OnToolCall(func(msg acp.ACPMessage) {
		toolName := msg.Params["tool"].(string)
		input := msg.Params["input"].(map[string]interface{})
		handler(toolName, input)
	})
}

func NewHookAdapter(config map[string]interface{}) (*HookAdapter, error) {
	server := hook.NewHookServer(func(event hook.HookEvent) {
		// Handle hook events
	})
	
	if err := server.Start(); err != nil {
		return nil, err
	}
	
	return &HookAdapter{server: server}, nil
}

func (a *HookAdapter) Start() error {
	return nil // Already started
}

func (a *HookAdapter) Stop() error {
	return a.server.Stop()
}

func (a *HookAdapter) SendMessage(content string) error {
	// Not applicable for hook adapter
	return nil
}

func (a *HookAdapter) OnToolCall(handler func(string, map[string]interface{})) {
	// Hook events are handled in server callback
}

func NewWrapperAdapter(config map[string]interface{}) (*WrapperAdapter, error) {
	return &WrapperAdapter{}, nil
}

func (a *WrapperAdapter) Start() error {
	return nil
}

func (a *WrapperAdapter) Stop() error {
	return nil
}

func (a *WrapperAdapter) SendMessage(content string) error {
	return nil
}

func (a *WrapperAdapter) OnToolCall(handler func(string, map[string]interface{})) {
	// Handled by permission hook
}
