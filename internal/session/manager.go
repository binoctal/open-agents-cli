package session

import (
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/open-agents/bridge/internal/protocol"
)

type OutputCallback func(sessionID string, msg protocol.Message)

type Manager struct {
	sessions       map[string]*Session
	mu             sync.RWMutex
	outputCallback OutputCallback
}

type Session struct {
	ID             string
	CLIType        string
	WorkDir        string
	PermissionMode string // "default", "plan", "accept-edits", "accept-all"
	Status         string // "active", "completed", "error"
	Protocol       *protocol.Manager
	CreatedAt      time.Time
}

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

func (m *Manager) SetOutputCallback(callback OutputCallback) {
	m.outputCallback = callback
}

func (m *Manager) Create(cliType, workDir string) (*Session, error) {
	return m.CreateWithID(cliType, workDir, "")
}

func (m *Manager) CreateWithID(cliType, workDir, sessionID string) (*Session, error) {
	return m.CreateWithIDAndSize(cliType, workDir, sessionID, 120, 30, "default")
}

func (m *Manager) CreateWithIDAndSize(cliType, workDir, sessionID string, cols, rows int, permissionMode string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Use provided sessionID or generate a new one
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// Default permission mode
	if permissionMode == "" {
		permissionMode = "default"
	}

	// Create protocol manager
	protocolMgr := protocol.NewManager()

	sess := &Session{
		ID:             sessionID,
		CLIType:        cliType,
		WorkDir:        workDir,
		PermissionMode: permissionMode,
		Status:         "active",
		Protocol:       protocolMgr,
		CreatedAt:      time.Now(),
	}

	// Set up message callback
	if m.outputCallback != nil {
		log.Printf("[SessionManager] Setting up message callback for session %s", sessionID)
		protocolMgr.Subscribe(func(msg protocol.Message) {
			log.Printf("[SessionManager] Message received: type=%s", msg.Type)
			m.outputCallback(sess.ID, msg)
		})
	} else {
		log.Printf("[SessionManager] WARNING: No output callback set for session %s", sessionID)
	}

	// Get CLI command and args
	command, args := m.getCLICommand(cliType)

	// Connect with auto-detection
	config := protocol.AdapterConfig{
		WorkDir: workDir,
		Command: command,
		Args:    args,
		Cols:    cols,
		Rows:    rows,
	}

	// For claude CLI, unset CLAUDECODE to allow nested sessions
	if cliType == "claude" {
		config.CustomEnv = map[string]string{
			"CLAUDECODE": "",
		}
	}

	// Apply permission mode settings
	m.applyPermissionMode(permissionMode, cliType, &config)

	if err := protocolMgr.Connect(config); err != nil {
		return nil, err
	}

	log.Printf("[SessionManager] Session %s connected using protocol: %s", sessionID, protocolMgr.GetProtocolName())

	m.sessions[sess.ID] = sess
	return sess, nil
}

// applyPermissionMode configures the adapter based on permission mode
func (m *Manager) applyPermissionMode(permissionMode, cliType string, config *protocol.AdapterConfig) {
	log.Printf("[SessionManager] Applying permission mode: %s for CLI: %s", permissionMode, cliType)

	// Initialize CustomEnv if nil
	if config.CustomEnv == nil {
		config.CustomEnv = make(map[string]string)
	}

	switch permissionMode {
	case "accept-all":
		// Auto-accept all operations
		switch cliType {
		case "claude":
			config.CustomEnv["CLAUDE_PERMISSION_MODE"] = "accept-all"
			config.Args = append(config.Args, "--dangerously-skip-permissions")
		case "qwen":
			config.CustomEnv["QWEN_PERMISSION_MODE"] = "accept-all"
		case "goose":
			config.CustomEnv["GOOSE_MODE"] = "auto"
		case "gemini":
			config.CustomEnv["GEMINI_PERMISSION_MODE"] = "accept-all"
		}

	case "accept-edits":
		// Auto-accept file edits only
		switch cliType {
		case "claude":
			config.CustomEnv["CLAUDE_PERMISSION_MODE"] = "accept-edits"
		case "qwen":
			config.CustomEnv["QWEN_PERMISSION_MODE"] = "accept-edits"
		case "goose":
			config.CustomEnv["GOOSE_MODE"] = "auto-edit"
		case "gemini":
			config.CustomEnv["GEMINI_PERMISSION_MODE"] = "accept-edits"
		}

	case "plan":
		// Plan mode - show plan before execution
		switch cliType {
		case "claude":
			config.CustomEnv["CLAUDE_PERMISSION_MODE"] = "plan"
			config.Args = append(config.Args, "--plan")
		case "qwen":
			config.CustomEnv["QWEN_PERMISSION_MODE"] = "plan"
		case "goose":
			config.CustomEnv["GOOSE_MODE"] = "plan"
		case "gemini":
			config.CustomEnv["GEMINI_PERMISSION_MODE"] = "plan"
		}

	default:
		// Default mode - ask for confirmation on sensitive operations
		// Most CLIs use this as default, no env vars needed
		log.Printf("[SessionManager] Using default permission mode")
	}
}

func (m *Manager) getCLICommand(cliType string) (string, []string) {
	switch cliType {
	case "claude":
		// Claude Code ACP via npx
		return "npx", []string{"@zed-industries/claude-code-acp"}
	case "qwen":
		return "qwen-code", []string{"--experimental-acp"}
	case "goose":
		return "goose", []string{"acp"}
	case "gemini":
		return "gemini-cli", []string{"--acp"}
	case "kiro":
		return "kiro", []string{"chat"}
	case "cline":
		return "cline", nil
	case "codex":
		return "codex", nil
	default:
		return cliType, nil
	}
}

func (m *Manager) Get(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[id]
	if !ok {
		return nil
	}

	if sess.Protocol != nil {
		sess.Protocol.Disconnect()
	}

	sess.Status = "completed"
	delete(m.sessions, id)
	return nil
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sess := range m.sessions {
		if sess.Protocol != nil {
			sess.Protocol.Disconnect()
		}
	}
	m.sessions = make(map[string]*Session)
}

func (s *Session) Send(input string) error {
	log.Printf("[Session.Send] Called for session %s, input: %q, Protocol nil: %v", s.ID, input, s.Protocol == nil)
	if s.Protocol == nil {
		log.Printf("[Session.Send] ERROR: Protocol is nil for session %s", s.ID)
		return nil
	}
	err := s.Protocol.SendMessage(protocol.Message{
		Type:    protocol.MessageTypeContent,
		Content: input,
	})
	if err != nil {
		log.Printf("[Session.Send] SendMessage error: %v", err)
	}
	return err
}

func (s *Session) Resize(cols, rows int) error {
	// Resize is handled by the protocol adapter
	// For now, we don't expose this in the protocol interface
	// TODO: Add resize support to protocol.Adapter interface if needed
	return nil
}

func (m *Manager) Resize(id string, cols, rows int) error {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()

	if !ok {
		return nil
	}
	return sess.Resize(cols, rows)
}

func (s *Session) GetProtocolName() string {
	if s.Protocol == nil {
		return "none"
	}
	return s.Protocol.GetProtocolName()
}
