package session

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// ═══ TestNewSession ═════════════════════════════════════════════

func TestNewSession(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	s := m.Current()
	if s == nil {
		t.Fatal("Current() returned nil after NewManager")
	}
	if s.ID == "" {
		t.Error("Session ID is empty")
	}
	if len(s.Messages) != 0 {
		t.Errorf("Messages length = %d, want 0", len(s.Messages))
	}
}

// ═══ TestAddMessage ═════════════════════════════════════════════

func TestAddMessage(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	before := time.Now()
	m.AddMessage("user", "Привет", "magos")

	s := m.Current()
	if len(s.Messages) != 1 {
		t.Errorf("Messages length = %d, want 1", len(s.Messages))
	}
	if s.Messages[0].Content != "Привет" {
		t.Errorf("Message content = %q, want 'Привет'", s.Messages[0].Content)
	}
	if s.Messages[0].Role != "user" {
		t.Errorf("Message role = %q, want 'user'", s.Messages[0].Role)
	}
	if s.Messages[0].Agent != "magos" {
		t.Errorf("Message agent = %q, want 'magos'", s.Messages[0].Agent)
	}
	if s.Updated.Before(before) {
		t.Error("Updated time not changed after AddMessage")
	}
}

// ═══ TestSwitchSession ══════════════════════════════════════════

func TestSwitchSession(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	first := m.New("first")
	second := m.New("second")

	if m.Current().ID != second.ID {
		t.Errorf("Current() = %q, want %q", m.Current().ID, second.ID)
	}

	if !m.Switch(first.ID) {
		t.Error("Switch(first) returned false")
	}
	if m.Current().ID != first.ID {
		t.Errorf("Current() = %q, want %q", m.Current().ID, first.ID)
	}

	// Switch to non-existent
	if m.Switch("nonexistent") {
		t.Error("Switch(nonexistent) returned true")
	}
}

// ═══ TestDeleteSession ══════════════════════════════════════════

func TestDeleteSession(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	s := m.New("to-delete")
	id := s.ID

	m.Delete(id)

	// List should not contain deleted session
	for _, s := range m.List() {
		if s.ID == id {
			t.Errorf("Deleted session %q still in List()", id)
		}
	}
}

// ═══ TestClearSession ═══════════════════════════════════════════

func TestClearSession(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	m.AddMessage("user", "msg1", "magos")
	m.AddMessage("assistant", "msg2", "magos")
	m.AddMessage("user", "msg3", "magos")

	if len(m.Current().Messages) != 3 {
		t.Errorf("Messages length = %d, want 3", len(m.Current().Messages))
	}

	m.Clear()

	if len(m.Current().Messages) != 0 {
		t.Errorf("Messages length after Clear = %d, want 0", len(m.Current().Messages))
	}
}

// ═══ TestSessionPersistence ═════════════════════════════════════

func TestSessionPersistence(t *testing.T) {
	dir := t.TempDir()

	// Create session with messages
	m1 := NewManager(dir)
	m1.AddMessage("user", "Привет", "magos")
	m1.AddMessage("assistant", "Привет! Занесено.", "magos")
	m1.AddMessage("user", "Как дела?", "magos")

	// Create new manager (reload from disk)
	m2 := NewManager(dir)

	// Find the session with messages
	var found *Session
	for _, s := range m2.List() {
		if len(s.Messages) == 3 {
			found = s
			break
		}
	}
	if found == nil {
		t.Fatal("No session with 3 messages found after reload")
	}
	if found.Messages[0].Content != "Привет" {
		t.Errorf("First message = %q, want 'Привет'", found.Messages[0].Content)
	}
}

// ═══ TestSessionPersistence_MultipleSessions ════════════════════

func TestSessionPersistence_MultipleSessions(t *testing.T) {
	dir := t.TempDir()

	m1 := NewManager(dir)
	s1 := m1.New("session-1")
	m1.AddMessage("user", "msg1", "magos")

	m1.New("session-2")
	m1.AddMessage("user", "msg2", "magos")

	// Reload
	m2 := NewManager(dir)
	list := m2.List()

	// Should have at least 2 sessions
	if len(list) < 2 {
		t.Errorf("List() length = %d, want >= 2", len(list))
	}

	// Find session-1
	found := false
	for _, s := range list {
		if s.ID == s1.ID {
			found = true
			if len(s.Messages) != 1 {
				t.Errorf("session-1 messages = %d, want 1", len(s.Messages))
			}
		}
	}
	if !found {
		t.Error("session-1 not found after reload")
	}
}

// ═══ TestSetContext ═════════════════════════════════════════════

func TestSetContext(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	m.SetContext([]string{"main.go", "server.go"})

	s := m.Current()
	if len(s.ContextFiles) != 2 {
		t.Errorf("ContextFiles length = %d, want 2", len(s.ContextFiles))
	}
	if s.ContextFiles[0] != "main.go" {
		t.Errorf("ContextFiles[0] = %q, want 'main.go'", s.ContextFiles[0])
	}
}

// ═══ TestList ═══════════════════════════════════════════════════

func TestList(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	m.New("session-a")
	m.New("session-b")
	m.New("session-c")

	list := m.List()
	// Should have default + 3 new = 4
	if len(list) < 4 {
		t.Errorf("List() length = %d, want >= 4", len(list))
	}
}

// ═══ TestGenerateID ═════════════════════════════════════════════

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	time.Sleep(time.Millisecond * 10)
	id2 := generateID()

	if id1 == "" {
		t.Error("generateID() returned empty string")
	}
	if id1 == id2 {
		t.Errorf("generateID() returned same ID twice: %q", id1)
	}
}

// ═══ TestSaveDir ════════════════════════════════════════════════

func TestSaveDir(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	expectedDir := dir + "/.forge_sessions"
	if m.saveDir != expectedDir {
		t.Errorf("saveDir = %q, want %q", m.saveDir, expectedDir)
	}

	// Check directory was created
	if _, err := os.Stat(m.saveDir); os.IsNotExist(err) {
		t.Errorf("saveDir %q does not exist", m.saveDir)
	}
}

// ═══ TestAddMessage_Agent ═══════════════════════════════════════

func TestAddMessage_Agent(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	m.AddMessage("user", "test", "servitor")
	m.AddMessage("assistant", "done", "magos")

	s := m.Current()
	if s.Messages[0].Agent != "servitor" {
		t.Errorf("Messages[0].Agent = %q, want 'servitor'", s.Messages[0].Agent)
	}
	if s.Messages[1].Agent != "magos" {
		t.Errorf("Messages[1].Agent = %q, want 'magos'", s.Messages[1].Agent)
	}
}

// ═══ TestSwitchSession_Invalid ══════════════════════════════════

func TestSwitchSession_Invalid(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	original := m.Current().ID
	if m.Switch("nonexistent-id") {
		t.Error("Switch(nonexistent) returned true")
	}
	if m.Current().ID != original {
		t.Error("Current changed after failed switch")
	}
}

// ═══ TestDeleteSession_Current ══════════════════════════════════

func TestDeleteSession_Current(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	first := m.Current()
	m.New("second")
	second := m.Current()

	// Delete the current session
	m.Delete(second.ID)

	// Should switch to first
	if m.Current().ID != first.ID {
		t.Errorf("Current after delete = %q, want %q", m.Current().ID, first.ID)
	}
}

// ═══ TestNewSession_HasTimestamp ════════════════════════════════

func TestNewSession_HasTimestamp(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	s := m.Current()
	if s.Created.IsZero() {
		t.Error("Created timestamp is zero")
	}
	if s.Updated.IsZero() {
		t.Error("Updated timestamp is zero")
	}
}

// ═══ TestConcurrentAccess_NoRace ═══════════════════════════════

func TestConcurrentAccess_NoRace(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.AddMessage("user", fmt.Sprintf("msg-%d", n), "magos")
			_ = m.Current()
			_ = m.List()
		}(i)
	}
	wg.Wait()

	if len(m.Current().Messages) != 10 {
		t.Errorf("Messages = %d, want 10", len(m.Current().Messages))
	}
}

// ═══ TestAddMessage_PreservesOrder ══════════════════════════════

func TestAddMessage_PreservesOrder(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	for i := 0; i < 5; i++ {
		m.AddMessage("user", fmt.Sprintf("msg-%d", i), "magos")
	}

	s := m.Current()
	for i, msg := range s.Messages {
		expected := fmt.Sprintf("msg-%d", i)
		if msg.Content != expected {
			t.Errorf("Messages[%d].Content = %q, want %q", i, msg.Content, expected)
		}
	}
}
