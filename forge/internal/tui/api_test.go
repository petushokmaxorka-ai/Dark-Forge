// Package tui — Tests for HTTP client (SSE streaming + session subcommands).
package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestStreamEventToMsg_Token verifies a token event parses correctly.
func TestStreamEventToMsg_Token(t *testing.T) {
	raw := []byte(`"hello "`)
	msg := streamEventToMsg(StreamEvent{Event: "token", Data: raw})

	if msg.Kind != "token" {
		t.Errorf("Kind = %q, want token", msg.Kind)
	}
	if msg.Token != "hello " {
		t.Errorf("Token = %q, want %q", msg.Token, "hello ")
	}
}

// TestStreamEventToMsg_Status verifies status events parse the agent field.
func TestStreamEventToMsg_Status(t *testing.T) {
	raw := []byte(`{"agent":"magos","status":"thinking"}`)
	msg := streamEventToMsg(StreamEvent{Event: "status", Data: raw})

	if msg.Kind != "status" {
		t.Errorf("Kind = %q, want status", msg.Kind)
	}
	if msg.Agent != "magos" {
		t.Errorf("Agent = %q, want magos", msg.Agent)
	}
}

// TestStreamEventToMsg_Done verifies done events have agent.
func TestStreamEventToMsg_Done(t *testing.T) {
	raw := []byte(`{"agent":"glm","status":"complete"}`)
	msg := streamEventToMsg(StreamEvent{Event: "done", Data: raw})

	if msg.Kind != "done" {
		t.Errorf("Kind = %q, want done", msg.Kind)
	}
	if msg.Agent != "glm" {
		t.Errorf("Agent = %q, want glm", msg.Agent)
	}
}

// TestStreamEventToMsg_Error verifies error events populate Err.
func TestStreamEventToMsg_Error(t *testing.T) {
	raw := []byte(`{"error":"upstream timeout"}`)
	msg := streamEventToMsg(StreamEvent{Event: "error", Data: raw})

	if msg.Kind != "error" {
		t.Errorf("Kind = %q, want error", msg.Kind)
	}
	if msg.Err == nil {
		t.Fatal("Err should not be nil for error event")
	}
	if !strings.Contains(msg.Err.Error(), "upstream timeout") {
		t.Errorf("Err = %q, want it to contain 'upstream timeout'", msg.Err.Error())
	}
}

// TestStreamEventToMsg_ToolCall verifies tool_call events parse tool+path.
func TestStreamEventToMsg_ToolCall(t *testing.T) {
	raw := []byte(`{"tool":"read","path":"main.go"}`)
	msg := streamEventToMsg(StreamEvent{Event: "tool_call", Data: raw})

	if msg.Kind != "tool_call" {
		t.Errorf("Kind = %q, want tool_call", msg.Kind)
	}
	if msg.Token != "read main.go" {
		t.Errorf("Token = %q, want %q", msg.Token, "read main.go")
	}
}

// TestStreamChat_Integration runs against an in-process SSE server.
func TestStreamChat_Integration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)

		fmt.Fprintf(w, "event: status\ndata: {\"agent\":\"magos\",\"status\":\"thinking\"}\n\n")
		flusher.Flush()

		for _, word := range []string{"hello", " ", "world", "."} {
			fmt.Fprintf(w, "event: token\ndata: %q\n\n", word)
			flusher.Flush()
		}

		fmt.Fprintf(w, "event: done\ndata: {\"agent\":\"magos\",\"status\":\"complete\"}\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, errc := c.StreamChat(ctx, ChatRequest{Message: "hi"})

	var tokens []string
	for ev := range events {
		if ev.Event == "token" {
			tokens = append(tokens, string(ev.Data))
		}
		if ev.Event == "done" {
			break
		}
	}
	select {
	case e := <-errc:
		if e != nil {
			t.Errorf("errc: %v", e)
		}
	default:
	}

	if len(tokens) != 4 {
		t.Errorf("got %d tokens, want 4", len(tokens))
	}
}

// TestSwitchSession_OK checks that a 200 from the server is treated as success.
func TestSwitchSession_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/session/switch" {
			t.Errorf("path = %q, want /api/session/switch", r.URL.Path)
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			t.Error("id query param missing")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","id":"` + id + `"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.SwitchSession("abc-123"); err != nil {
		t.Errorf("SwitchSession: %v", err)
	}
}

// TestSwitchSession_NotFound checks that 404 surfaces as error.
func TestSwitchSession_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "session not found", 404)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.SwitchSession("missing")
	if err == nil {
		t.Error("expected error for 404, got nil")
	}
}

// TestDeleteSession_OK checks the delete endpoint.
func TestDeleteSession_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/session/delete" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.DeleteSession("xyz"); err != nil {
		t.Errorf("DeleteSession: %v", err)
	}
}

// TestClearSessions_OK checks the clear endpoint.
func TestClearSessions_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/session/clear" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.ClearSessions(); err != nil {
		t.Errorf("ClearSessions: %v", err)
	}
}
