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
	ID        string
	CLIType   string
	WorkDir   string
	Status    string // "active", "completed", "error"
	Protocol  *protocol.Manager
	CreatedAt time.Time
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
	return m.CreateWithIDAndSize(cliType, workDir, sessionID, 120, 30)
}

func (m *Manager) CreateWithIDAndSize(cliType, workDir, sessionID string, cols, rows int) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Use provided sessionID or generate a new one
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// Create protocol manager
	protocolMgr := protocol.NewManager()

	sess := &Session{
		ID:        sessionID,
		CLIType:   cliType,
		WorkDir:   workDir,
		Status:    "active",
		Protocol:  protocolMgr,
		CreatedAt: time.Now(),
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

	if err := protocolMgr.Connect(config); err != nil {
		return nil, err
	}

	log.Printf("[SessionManager] Session %s connected using protocol: %s", sessionID, protocolMgr.GetProtocolName())

	m.sessions[sess.ID] = sess
	return sess, nil
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
	if s.Protocol == nil {
		return nil
	}
	return s.Protocol.SendMessage(protocol.Message{
		Type:    protocol.MessageTypeContent,
		Content: input,
	})
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
