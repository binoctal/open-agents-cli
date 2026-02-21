package protocol

// MessageType represents the type of message
type MessageType string

const (
	MessageTypeContent    MessageType = "content"     // AI response text
	MessageTypeThought    MessageType = "thought"     // AI thinking process
	MessageTypeToolCall   MessageType = "tool_call"   // Tool invocation
	MessageTypePermission MessageType = "permission"  // Permission request
	MessageTypeStatus     MessageType = "status"      // Agent status change
	MessageTypePlan       MessageType = "plan"        // Task plan
	MessageTypeError      MessageType = "error"       // Error message
)

// AgentStatus represents the current state of the agent
type AgentStatus string

const (
	StatusIdle              AgentStatus = "idle"
	StatusThinking          AgentStatus = "thinking"
	StatusStreaming         AgentStatus = "streaming"
	StatusToolExecuting     AgentStatus = "tool_executing"
	StatusPermissionPending AgentStatus = "permission_pending"
)

// Message represents a unified message format across all protocols
type Message struct {
	Type    MessageType            `json:"type"`
	Content interface{}            `json:"content"`
	Meta    map[string]interface{} `json:"meta,omitempty"`
}

// PermissionRequest represents a permission request
type PermissionRequest struct {
	ID          string                 `json:"id"`
	ToolName    string                 `json:"tool_name"`
	ToolInput   map[string]interface{} `json:"tool_input"`
	Description string                 `json:"description"`
	Risk        string                 `json:"risk"` // "low", "medium", "high"
	Options     []string               `json:"options"`
}

// PermissionResponse represents the user's response
type PermissionResponse struct {
	ID       string `json:"id"`
	OptionID string `json:"option_id"` // "allow_once", "allow_always", "reject_once", "reject_always"
}

// ToolCall represents a tool invocation
type ToolCall struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	Input  map[string]interface{} `json:"input"`
	Status string                 `json:"status"` // "pending", "in_progress", "completed", "failed"
	Result interface{}            `json:"result,omitempty"`
}
