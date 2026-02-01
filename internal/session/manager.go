package session

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/open-agents/bridge/internal/adapter"
)

type OutputCallback func(sessionID, outputType, content string)
type ChatCallback func(sessionID, content string, codeBlock *CodeBlock)

type CodeBlock struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

type Manager struct {
	sessions       map[string]*Session
	mu             sync.RWMutex
	outputCallback OutputCallback
	chatCallback   ChatCallback
}

type Session struct {
	ID           string
	CLIType      string
	WorkDir      string
	Status       string // "active", "completed", "error"
	Adapter      adapter.Adapter
	CreatedAt    time.Time
	outputBuffer strings.Builder
	inCodeBlock  bool
	codeLanguage string
	codeBuffer   strings.Builder
}

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

func (m *Manager) SetOutputCallback(callback OutputCallback) {
	m.outputCallback = callback
}

func (m *Manager) SetChatCallback(callback ChatCallback) {
	m.chatCallback = callback
}

func (m *Manager) Create(cliType, workDir string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	adp, err := adapter.Get(cliType)
	if err != nil {
		return nil, err
	}

	sess := &Session{
		ID:        uuid.New().String(),
		CLIType:   cliType,
		WorkDir:   workDir,
		Status:    "active",
		Adapter:   adp,
		CreatedAt: time.Now(),
	}

	// Set up output callback - parse for chat responses
	if m.outputCallback != nil || m.chatCallback != nil {
		adp.OnOutput(func(event adapter.OutputEvent) {
			// Forward raw output
			if m.outputCallback != nil {
				m.outputCallback(sess.ID, event.Type, event.Content)
			}

			// Parse for chat responses
			if m.chatCallback != nil && event.Type == "stdout" {
				m.parseOutput(sess, event.Content)
			}
		})
	}

	// Set up exit callback
	adp.OnExit(func(code int) {
		m.mu.Lock()
		sess.Status = "completed"
		m.mu.Unlock()

		// Flush any remaining output
		if m.chatCallback != nil && sess.outputBuffer.Len() > 0 {
			m.chatCallback(sess.ID, sess.outputBuffer.String(), nil)
		}
	})

	if err := adp.Start(workDir, nil); err != nil {
		return nil, err
	}

	m.sessions[sess.ID] = sess
	return sess, nil
}

// parseOutput parses CLI output for chat messages and code blocks
func (m *Manager) parseOutput(sess *Session, line string) {
	trimmed := strings.TrimSpace(line)

	// Detect code block start
	if strings.HasPrefix(trimmed, "```") {
		if !sess.inCodeBlock {
			// Start of code block
			sess.inCodeBlock = true
			sess.codeLanguage = strings.TrimPrefix(trimmed, "```")
			sess.codeBuffer.Reset()

			// Flush text before code block
			if sess.outputBuffer.Len() > 0 {
				m.chatCallback(sess.ID, sess.outputBuffer.String(), nil)
				sess.outputBuffer.Reset()
			}
		} else {
			// End of code block
			sess.inCodeBlock = false
			m.chatCallback(sess.ID, "", &CodeBlock{
				Language: sess.codeLanguage,
				Code:     sess.codeBuffer.String(),
			})
		}
		return
	}

	if sess.inCodeBlock {
		if sess.codeBuffer.Len() > 0 {
			sess.codeBuffer.WriteString("\n")
		}
		sess.codeBuffer.WriteString(line)
	} else {
		// Accumulate text output
		if sess.outputBuffer.Len() > 0 {
			sess.outputBuffer.WriteString("\n")
		}
		sess.outputBuffer.WriteString(line)

		// Flush on empty line or after accumulating enough
		if trimmed == "" || sess.outputBuffer.Len() > 500 {
			m.chatCallback(sess.ID, sess.outputBuffer.String(), nil)
			sess.outputBuffer.Reset()
		}
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

	if sess.Adapter != nil {
		sess.Adapter.Stop()
	}

	sess.Status = "completed"
	delete(m.sessions, id)
	return nil
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sess := range m.sessions {
		if sess.Adapter != nil {
			sess.Adapter.Stop()
		}
	}
	m.sessions = make(map[string]*Session)
}

func (s *Session) Send(input string) error {
	if s.Adapter == nil {
		return nil
	}
	return s.Adapter.Send(input)
}
