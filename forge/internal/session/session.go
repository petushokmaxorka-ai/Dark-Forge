// Package session — Heretic Forge session management
// «Memoria non moritur.»
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Message is a single message in a session
type Message struct {
	Role    string    `json:"role"`    // user, assistant, system
	Content string    `json:"content"`
	Agent   string    `json:"agent"`   // magos, servitor, skitarii
	Time    time.Time `json:"time"`
}

// Session is a conversation session
type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Messages  []Message `json:"messages"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	ContextFiles []string `json:"context_files"`
}

// Manager handles session persistence
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	current  string
	saveDir  string
}

// NewManager creates a session manager
func NewManager(saveDir string) *Manager {
	m := &Manager{
		sessions: make(map[string]*Session),
		saveDir:  filepath.Join(saveDir, ".forge_sessions"),
	}
	os.MkdirAll(m.saveDir, 0755)
	m.LoadAll()

	// Ensure a current session always exists.
	if _, ok := m.sessions[m.current]; !ok {
		if len(m.sessions) > 0 {
			for id := range m.sessions {
				m.current = id
				break
			}
		} else {
			m.New("default")
		}
	}

	return m
}

// New creates a new session
func (m *Manager) New(name string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := &Session{
		ID:      generateID(),
		Name:    name,
		Messages: []Message{},
		Created: time.Now(),
		Updated: time.Now(),
	}
	m.sessions[s.ID] = s
	m.current = s.ID
	m.save(s)
	return s
}

// Current returns the active session
func (m *Manager) Current() *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[m.current]; ok {
		return s
	}
	return nil
}

// Switch changes the active session
func (m *Manager) Switch(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; ok {
		m.current = id
		return true
	}
	return false
}

// AddMessage appends a message to the current session
func (m *Manager) AddMessage(role, content, agent string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[m.current]
	if !ok {
		return
	}
	s.Messages = append(s.Messages, Message{
		Role:    role,
		Content: content,
		Agent:   agent,
		Time:    time.Now(),
	})
	s.Updated = time.Now()
	m.save(s)
}

// SetContext updates context files for current session
func (m *Manager) SetContext(files []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[m.current]; ok {
		s.ContextFiles = files
		m.save(s)
	}
}

// List returns all sessions
func (m *Manager) List() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

// Delete removes a session
func (m *Manager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	path := filepath.Join(m.saveDir, id+".json")
	os.Remove(path)
	if m.current == id && len(m.sessions) > 0 {
		for k := range m.sessions {
			m.current = k
			break
		}
	}
}

// Clear empties current session messages
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[m.current]; ok {
		s.Messages = []Message{}
		s.Updated = time.Now()
		m.save(s)
	}
}

// SaveAll persists all sessions
func (m *Manager) SaveAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		m.save(s)
	}
}

// LoadAll loads sessions from disk
func (m *Manager) LoadAll() {
	entries, err := os.ReadDir(m.saveDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(m.saveDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		s := &Session{}
		if err := json.Unmarshal(data, s); err != nil {
			continue
		}
		m.sessions[s.ID] = s
	}
}

// save writes a single session to disk
func (m *Manager) save(s *Session) {
	data, _ := json.MarshalIndent(s, "", "  ")
	path := filepath.Join(m.saveDir, s.ID+".json")
	os.WriteFile(path, data, 0644)
}

func generateID() string {
	return time.Now().Format("20060102-150405.000000000")
}
