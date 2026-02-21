package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/open-agents/bridge/internal/logger"
)

// ACPAdapter implements the Agent Client Protocol (ACP)
type ACPAdapter struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	connected atomic.Bool
	callback  func(Message)
	requestID atomic.Int64
	mu        sync.Mutex
	sessionID string
}

// NewACPAdapter creates a new ACP adapter
func NewACPAdapter() *ACPAdapter {
	return &ACPAdapter{}
}

func (a *ACPAdapter) Name() string {
	return "acp"
}

func (a *ACPAdapter) Version() string {
	return "1.0.0"
}

func (a *ACPAdapter) Connect(config AdapterConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	logger.Info("[ACP] Connecting to %s in %s", config.Command, config.WorkDir)

	// Start CLI process
	a.cmd = exec.Command(config.Command, config.Args...)
	a.cmd.Dir = config.WorkDir
	a.cmd.Env = os.Environ()

	// Add custom env vars
	for k, v := range config.Env {
		a.cmd.Env = append(a.cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range config.CustomEnv {
		a.cmd.Env = append(a.cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Setup pipes
	var err error
	a.stdin, err = a.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	a.stdout, err = a.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	a.stderr, err = a.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start process
	if err := a.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	logger.Info("[ACP] Process started (PID: %d)", a.cmd.Process.Pid)
	a.connected.Store(true)

	// Start reading messages
	go a.readMessages()
	go a.readErrors()

	// Send initialize request
	if err := a.initialize(); err != nil {
		a.Disconnect()
		return fmt.Errorf("failed to initialize: %w", err)
	}

	return nil
}

func (a *ACPAdapter) Disconnect() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.connected.Load() {
		return nil
	}

	logger.Info("[ACP] Disconnecting")
	a.connected.Store(false)

	if a.stdin != nil {
		a.stdin.Close()
	}
	if a.cmd != nil && a.cmd.Process != nil {
		a.cmd.Process.Kill()
	}

	return nil
}

func (a *ACPAdapter) IsConnected() bool {
	return a.connected.Load()
}

func (a *ACPAdapter) SendMessage(msg Message) error {
	// Convert unified message to ACP JSON-RPC format
	// This will be implemented based on message type
	return nil
}

func (a *ACPAdapter) ReceiveMessage() (Message, error) {
	// Not used in callback mode
	return Message{}, fmt.Errorf("not implemented")
}

func (a *ACPAdapter) Subscribe(callback func(Message)) {
	a.callback = callback
}

func (a *ACPAdapter) Capabilities() []string {
	return []string{"permissions", "file_ops", "tool_calls", "streaming"}
}

func (a *ACPAdapter) SupportsPermissions() bool {
	return true
}

func (a *ACPAdapter) SupportsFileOps() bool {
	return true
}

func (a *ACPAdapter) SupportsToolCalls() bool {
	return true
}

// initialize sends the initialize request
func (a *ACPAdapter) initialize() error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      a.nextRequestID(),
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "1.0.0",
			"clientInfo": map[string]string{
				"name":    "open-agents-bridge",
				"version": "1.0.0",
			},
		},
	}

	return a.sendJSONRPC(req)
}

// readMessages reads JSON-RPC messages from stdout
func (a *ACPAdapter) readMessages() {
	scanner := bufio.NewScanner(a.stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		if !a.connected.Load() {
			break
		}

		line := scanner.Text()
		logger.Debug("[ACP] Received: %s", line)

		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			logger.Error("[ACP] Failed to parse JSON: %v", err)
			continue
		}

		a.handleMessage(msg)
	}

	if err := scanner.Err(); err != nil {
		logger.Error("[ACP] Scanner error: %v", err)
	}
}

// readErrors reads stderr
func (a *ACPAdapter) readErrors() {
	scanner := bufio.NewScanner(a.stderr)
	for scanner.Scan() {
		if !a.connected.Load() {
			break
		}
		logger.Warn("[ACP] stderr: %s", scanner.Text())
	}
}

// handleMessage processes incoming JSON-RPC messages
func (a *ACPAdapter) handleMessage(msg map[string]interface{}) {
	method, _ := msg["method"].(string)

	switch method {
	case "session/update":
		a.handleSessionUpdate(msg)
	case "session/request_permission":
		a.handlePermissionRequest(msg)
	case "fs/read_text_file":
		a.handleFileRead(msg)
	case "fs/write_text_file":
		a.handleFileWrite(msg)
	default:
		// Handle responses
		if _, ok := msg["result"]; ok {
			a.handleResponse(msg)
		} else if _, ok := msg["error"]; ok {
			a.handleError(msg)
		}
	}
}

// handleSessionUpdate processes session/update notifications
func (a *ACPAdapter) handleSessionUpdate(msg map[string]interface{}) {
	params, ok := msg["params"].(map[string]interface{})
	if !ok {
		return
	}

	updates, ok := params["updates"].([]interface{})
	if !ok {
		return
	}

	for _, update := range updates {
		u, ok := update.(map[string]interface{})
		if !ok {
			continue
		}

		updateType, _ := u["type"].(string)

		switch updateType {
		case "agent_message_chunk":
			a.emitMessage(Message{
				Type:    MessageTypeContent,
				Content: u["content"],
				Meta: map[string]interface{}{
					"protocol": "acp",
				},
			})

		case "agent_thought_chunk":
			a.emitMessage(Message{
				Type:    MessageTypeThought,
				Content: u["content"],
				Meta: map[string]interface{}{
					"protocol": "acp",
				},
			})

		case "tool_call":
			a.emitMessage(Message{
				Type: MessageTypeToolCall,
				Content: ToolCall{
					ID:     u["id"].(string),
					Name:   u["name"].(string),
					Input:  u["input"].(map[string]interface{}),
					Status: "pending",
				},
				Meta: map[string]interface{}{
					"protocol": "acp",
				},
			})

		case "tool_call_update":
			status, _ := u["status"].(string)
			a.emitMessage(Message{
				Type: MessageTypeToolCall,
				Content: ToolCall{
					ID:     u["id"].(string),
					Status: status,
					Result: u["result"],
				},
				Meta: map[string]interface{}{
					"protocol": "acp",
				},
			})

		case "end_turn":
			a.emitMessage(Message{
				Type:    MessageTypeStatus,
				Content: StatusIdle,
				Meta: map[string]interface{}{
					"protocol": "acp",
				},
			})
		}
	}
}

// handlePermissionRequest processes permission requests
func (a *ACPAdapter) handlePermissionRequest(msg map[string]interface{}) {
	params, ok := msg["params"].(map[string]interface{})
	if !ok {
		return
	}

	id, _ := msg["id"].(string)
	toolName, _ := params["tool_name"].(string)
	toolInput, _ := params["tool_input"].(map[string]interface{})
	description, _ := params["description"].(string)
	risk, _ := params["risk"].(string)
	options, _ := params["options"].([]interface{})

	optionStrs := make([]string, len(options))
	for i, opt := range options {
		optionStrs[i], _ = opt.(string)
	}

	a.emitMessage(Message{
		Type: MessageTypePermission,
		Content: PermissionRequest{
			ID:          id,
			ToolName:    toolName,
			ToolInput:   toolInput,
			Description: description,
			Risk:        risk,
			Options:     optionStrs,
		},
		Meta: map[string]interface{}{
			"protocol": "acp",
		},
	})
}

// handleFileRead processes file read requests
func (a *ACPAdapter) handleFileRead(msg map[string]interface{}) {
	// TODO: Implement file read
	logger.Debug("[ACP] File read request: %v", msg)
}

// handleFileWrite processes file write requests
func (a *ACPAdapter) handleFileWrite(msg map[string]interface{}) {
	// TODO: Implement file write
	logger.Debug("[ACP] File write request: %v", msg)
}

// handleResponse processes JSON-RPC responses
func (a *ACPAdapter) handleResponse(msg map[string]interface{}) {
	result, _ := msg["result"].(map[string]interface{})
	
	// Handle initialize response
	if sessionID, ok := result["sessionId"].(string); ok {
		a.sessionID = sessionID
		logger.Info("[ACP] Session initialized: %s", sessionID)
	}
}

// handleError processes JSON-RPC errors
func (a *ACPAdapter) handleError(msg map[string]interface{}) {
	errObj, _ := msg["error"].(map[string]interface{})
	code, _ := errObj["code"].(float64)
	message, _ := errObj["message"].(string)

	logger.Error("[ACP] Error %d: %s", int(code), message)

	a.emitMessage(Message{
		Type:    MessageTypeError,
		Content: message,
		Meta: map[string]interface{}{
			"protocol": "acp",
			"code":     int(code),
		},
	})
}

// emitMessage sends a message to the callback
func (a *ACPAdapter) emitMessage(msg Message) {
	if a.callback != nil {
		a.callback(msg)
	}
}

// sendJSONRPC sends a JSON-RPC message
func (a *ACPAdapter) sendJSONRPC(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	logger.Debug("[ACP] Sending: %s", string(data))

	_, err = a.stdin.Write(append(data, '\n'))
	return err
}

// nextRequestID generates the next request ID
func (a *ACPAdapter) nextRequestID() int64 {
	return a.requestID.Add(1)
}
