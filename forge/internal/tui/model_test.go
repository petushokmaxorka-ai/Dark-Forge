// Package tui — Basic tests for Bubble Tea TUI primitives.
package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/theme"
)

// TestModel_Initial verifies the seed model is well-formed.
func TestModel_Initial(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")

	if m.client == nil {
		t.Fatal("client should not be nil")
	}
	if m.client.BaseURL != "http://127.0.0.1:9091" {
		t.Errorf("client.BaseURL = %q, want %q", m.client.BaseURL, "http://127.0.0.1:9091")
	}
	if m.apiURL != "http://127.0.0.1:9091" {
		t.Errorf("apiURL = %q, want %q", m.apiURL, "http://127.0.0.1:9091")
	}
	if m.input.Placeholder == "" {
		t.Error("input should have a placeholder")
	}
	if len(m.entries) != 0 {
		t.Errorf("entries should start empty, got %d", len(m.entries))
	}
	if m.quitting {
		t.Error("model should not be quitting on init")
	}
	if !m.ready {
		// ready flips on first WindowSizeMsg — ok
	}
}

// TestModel_AppendEntry verifies entries are appended and viewport is refreshed.
func TestModel_AppendEntry(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")
	m.AppendEntry("user", "heretic", "hello")
	m.AppendEntry("assistant", "magos", "world")
	m.AppendEntry("system", "", "boot")
	m.AppendEntry("error", "", "bad")
	m.AppendEntry("tool", "", "[read] main.go")

	if got := len(m.entries); got != 5 {
		t.Errorf("entries len = %d, want 5", got)
	}
	for i, e := range m.entries {
		if e.Time.IsZero() {
			t.Errorf("entry %d has zero time", i)
		}
	}
}

// TestModel_StatusLine verifies the status bar renders without panicking.
func TestModel_StatusLine(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")
	m.version = VersionInfo{Name: "HereticArch", Version: "1.0.0"}
	m.status = StatusInfo{Model: "gemma4-v2-Q4_K_M", Agents: 6, Session: "default"}

	s := m.StatusLine()
	if s == "" {
		t.Fatal("status line should not be empty")
	}
	if !strings.Contains(s, "1.0.0") {
		t.Errorf("status line missing version: %q", s)
	}
	if !strings.Contains(s, "gemma4-v2") {
		t.Errorf("status line missing model: %q", s)
	}
	if !strings.Contains(s, "6 agents") {
		t.Errorf("status line missing agents count: %q", s)
	}
}

// TestModel_Update_WindowSize verifies window resize updates layout.
func TestModel_Update_WindowSize(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	nm := updated.(Model)

	if !nm.ready {
		t.Error("model should be ready after WindowSizeMsg")
	}
	if nm.width != 120 {
		t.Errorf("width = %d, want 120", nm.width)
	}
	if nm.height != 40 {
		t.Errorf("height = %d, want 40", nm.height)
	}
	if nm.viewport.Width == 0 {
		t.Error("viewport width should be set")
	}
	if nm.viewport.Height == 0 {
		t.Error("viewport height should be set")
	}
}

// TestModel_Update_ChatResult verifies successful chat responses render correctly.
func TestModel_Update_ChatResult(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")
	m.pending = true

	updated, _ := m.Update(ChatResultMsg{
		Resp: ChatResponse{Response: "Готово.", Agent: "magos"},
	})
	nm := updated.(Model)

	if nm.pending {
		t.Error("pending should be cleared after ChatResultMsg")
	}
	if len(nm.entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(nm.entries))
	}
	if nm.entries[0].Role != "assistant" {
		t.Errorf("entry role = %q, want assistant", nm.entries[0].Role)
	}
	if nm.entries[0].Agent != "magos" {
		t.Errorf("entry agent = %q, want magos", nm.entries[0].Agent)
	}
	if nm.entries[0].Content != "Готово." {
		t.Errorf("entry content = %q, want Готово.", nm.entries[0].Content)
	}
}

// TestModel_Update_ChatError verifies error responses are recorded.
func TestModel_Update_ChatError(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")
	m.pending = true

	updated, _ := m.Update(ChatResultMsg{Err: errFake("timeout")})
	nm := updated.(Model)

	if nm.pending {
		t.Error("pending should be cleared on error")
	}
	if len(nm.entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(nm.entries))
	}
	if nm.entries[0].Role != "error" {
		t.Errorf("entry role = %q, want error", nm.entries[0].Role)
	}
}

// TestModel_View_Banner verifies the welcome view renders without panic.
func TestModel_View_Banner(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")
	// not ready — should fall back to welcome screen
	v := m.View()
	if v == "" {
		t.Error("view should not be empty")
	}
	if !strings.Contains(v, "FORGE") {
		t.Errorf("view missing FORGE: %q", v)
	}
	if !strings.Contains(v, "MMXXVI") {
		t.Errorf("view missing MMXXVI footer: %q", v)
	}
	if !strings.Contains(v, "Omnissiah") {
		t.Errorf("view missing Omnissiah footer: %q", v)
	}
}

// TestRenderEntries_AllRoles verifies all entry roles render without panic.
func TestRenderEntries_AllRoles(t *testing.T) {
	now := time.Now()
	entries := []ChatEntry{
		{Role: "user", Content: "hi", Time: now},
		{Role: "assistant", Agent: "magos", Content: "hello\nworld", Time: now},
		{Role: "tool", Content: "[read] foo.go", Time: now},
		{Role: "system", Content: "ready", Time: now},
		{Role: "error", Content: "boom", Time: now},
	}
	out := renderEntries(entries)
	if !strings.Contains(out, "hi") {
		t.Error("user entry missing")
	}
	if !strings.Contains(out, "hello") {
		t.Error("assistant entry missing")
	}
	if !strings.Contains(out, "[read] foo.go") {
		t.Error("tool entry missing")
	}
	if !strings.Contains(out, "ready") {
		t.Error("system entry missing")
	}
	if !strings.Contains(out, "boom") {
		t.Error("error entry missing")
	}
}

// TestAgentGlyph verifies all known agent names have a glyph mapping.
func TestAgentGlyph(t *testing.T) {
	for _, name := range []string{"magos", "glm", "kimi", "mimo", "qwen", "minimax", "vox_dei", "unknown"} {
		g := theme.AgentGlyph(name)
		if g == "" {
			t.Errorf("AgentGlyph(%q) returned empty", name)
		}
	}
}

// errFake is a minimal error for tests.
type errFake string

func (e errFake) Error() string { return string(e) }

// TestModel_Update_ChatStream_Token verifies token events append assistant content.
func TestModel_Update_ChatStream_Token(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")
	m.pending = true
	m.streaming = true

	updated, _ := m.Update(ChatStreamMsg{Kind: "token", Agent: "magos", Token: "hello "})
	nm := updated.(Model)

	if !nm.streaming {
		t.Error("streaming should remain true after token event")
	}
	if nm.streamBuf.String() != "hello " {
		t.Errorf("streamBuf = %q, want %q", nm.streamBuf.String(), "hello ")
	}
	if len(nm.entries) == 0 {
		t.Fatal("expected assistant entry to be appended")
	}
	last := nm.entries[len(nm.entries)-1]
	if last.Role != "assistant" {
		t.Errorf("last role = %q, want assistant", last.Role)
	}
	if last.Content != "hello " {
		t.Errorf("last content = %q, want %q", last.Content, "hello ")
	}
}

// TestModel_Update_ChatStream_Done verifies done event closes the stream.
func TestModel_Update_ChatStream_Done(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")
	m.pending = true
	m.streaming = true
	m.streamBuf.WriteString("hi")
	ctx, cancel := context.WithCancel(context.Background())
	m.streamCtx = ctx
	m.streamCancel = cancel

	updated, _ := m.Update(ChatStreamMsg{Kind: "done"})
	nm := updated.(Model)

	if nm.streaming {
		t.Error("streaming should be false after done event")
	}
	if nm.pending {
		t.Error("pending should be false after done event")
	}
	if nm.streamBuf.Len() != 0 {
		t.Errorf("streamBuf should be reset, got %q", nm.streamBuf.String())
	}
}

// TestModel_Update_ChatStream_Error verifies error event records the failure.
func TestModel_Update_ChatStream_Error(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")
	m.streaming = true
	m.streamBuf.WriteString("partial")

	updated, _ := m.Update(ChatStreamMsg{Kind: "error", Err: errFake("upstream died")})
	nm := updated.(Model)

	if nm.streaming {
		t.Error("streaming should be false after error")
	}
	if len(nm.entries) == 0 {
		t.Fatal("expected error entry to be appended")
	}
	last := nm.entries[len(nm.entries)-1]
	if last.Role != "error" {
		t.Errorf("last role = %q, want error", last.Role)
	}
	if !strings.Contains(last.Content, "upstream died") {
		t.Errorf("last content = %q, want it to contain 'upstream died'", last.Content)
	}
}

// TestModel_UpsertStreamingAssistant_NewEntry verifies it appends when none exists.
func TestModel_UpsertStreamingAssistant_NewEntry(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")
	m.upsertStreamingAssistant("magos", "first")

	if len(m.entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(m.entries))
	}
	if m.entries[0].Content != "first" {
		t.Errorf("entry content = %q, want %q", m.entries[0].Content, "first")
	}
}

// TestModel_UpsertStreamingAssistant_UpdatesLast verifies it updates in place.
func TestModel_UpsertStreamingAssistant_UpdatesLast(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")
	m.upsertStreamingAssistant("magos", "first")
	m.upsertStreamingAssistant("magos", "first partial")
	m.upsertStreamingAssistant("magos", "first partial stream")

	if len(m.entries) != 1 {
		t.Fatalf("entries len = %d, want 1 (in-place updates)", len(m.entries))
	}
	if m.entries[0].Content != "first partial stream" {
		t.Errorf("entry content = %q, want %q", m.entries[0].Content, "first partial stream")
	}
}

// TestModel_SubmitInput_Stream verifies submitInput starts SSE stream.
func TestModel_SubmitInput_Stream(t *testing.T) {
	m := Initial("http://127.0.0.1:9091", nil, ".")
	m.input.SetValue("hello")

	// Use alt+enter to submit (OpenCode convention: Enter=newline, Alt+Enter=submit)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	nm := updated.(Model)

	if nm.input.Value() != "" {
		t.Errorf("input should be cleared after submit, got %q", nm.input.Value())
	}
	if !nm.pending {
		t.Error("pending should be true after submit")
	}
	if !nm.streaming {
		t.Error("streaming should be true after submit")
	}
	if nm.streamReq.Message != "hello" {
		t.Errorf("streamReq.Message = %q, want %q", nm.streamReq.Message, "hello")
	}
}
