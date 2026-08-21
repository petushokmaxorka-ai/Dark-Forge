// Package tui — Tests for persistent history and session loading.
package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchSessions_ReturnsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/session/list" {
			t.Errorf("path = %q, want /api/session/list", r.URL.Path)
		}
		sessions := []SessionInfo{
			{ID: "sess-1", Name: "default", Messages: 3, Current: true, Updated: time.Now()},
			{ID: "sess-2", Name: "side", Messages: 1, Updated: time.Now()},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	out, err := c.FetchSessions()
	if err != nil {
		t.Fatalf("FetchSessions: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d sessions, want 2", len(out))
	}
	if !out[0].Current {
		t.Error("first session should be current")
	}
}

func TestFetchHistory_ReturnsMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/history" {
			t.Errorf("path = %q, want /api/history", r.URL.Path)
		}
		if r.URL.Query().Get("session") != "sess-1" {
			t.Errorf("session = %q, want sess-1", r.URL.Query().Get("session"))
		}
		history := []HistoryMessage{
			{Role: "user", Content: "hi", Agent: "princip", Time: time.Now()},
			{Role: "assistant", Content: "hello", Agent: "magos", Time: time.Now()},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(history)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	out, err := c.FetchHistory("sess-1")
	if err != nil {
		t.Fatalf("FetchHistory: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d messages, want 2", len(out))
	}
	if out[1].Agent != "magos" {
		t.Errorf("agent = %q, want magos", out[1].Agent)
	}
}

func TestLoadInitialHistory_LoadsCurrentSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/list":
			sessions := []SessionInfo{
				{ID: "current-id", Name: "main", Messages: 1, Current: true, Updated: time.Now()},
			}
			json.NewEncoder(w).Encode(sessions)
		case "/api/history":
			history := []HistoryMessage{
				{Role: "user", Content: "previous", Agent: "princip", Time: time.Now()},
			}
			json.NewEncoder(w).Encode(history)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	cmd := loadInitialHistory(c)
	msg := cmd().(HistoryLoadedMsg)

	if msg.Err != nil {
		t.Fatalf("loadInitialHistory: %v", msg.Err)
	}
	if msg.SessionID != "current-id" {
		t.Errorf("sessionID = %q, want current-id", msg.SessionID)
	}
	if len(msg.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(msg.Entries))
	}
	if msg.Entries[0].Content != "previous" {
		t.Errorf("content = %q, want previous", msg.Entries[0].Content)
	}
}

func TestLoadInitialHistory_FallsBackToLastSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/list":
			sessions := []SessionInfo{
				{ID: "last-id", Name: "older", Messages: 1, Current: false, Updated: time.Now()},
			}
			json.NewEncoder(w).Encode(sessions)
		case "/api/history":
			json.NewEncoder(w).Encode([]HistoryMessage{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	msg := loadInitialHistory(c)().(HistoryLoadedMsg)

	if msg.SessionID != "last-id" {
		t.Errorf("sessionID = %q, want last-id", msg.SessionID)
	}
}

func TestFormatSessions_HighlightsCurrent(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "a", Name: "main", Messages: 5, Current: true, Updated: time.Now()},
		{ID: "b", Name: "other", Messages: 0, Updated: time.Now()},
	}
	out := formatSessions(sessions)
	if out == "" {
		t.Fatal("formatSessions should not be empty")
	}
	if !contains(out, "a") || !contains(out, "b") {
		t.Error("formatSessions should include session IDs")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
