package session

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/open-agents/bridge/internal/adapter"
)

type OutputCallback func(sessionID, outputType, content string)

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
	Adapter   adapter.Adapter
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

	// Set up output callback
	if m.outputCallback != nil {
		adp.OnOutput(func(event adapter.OutputEvent) {
			m.outputCallback(sess.ID, event.Type, event.Content)
		})
	}

	if err := adp.Start(workDir, nil); err != nil {
		return nil, err
	}

	m.sessions[sess.ID] = sess
	return sess, nil
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
