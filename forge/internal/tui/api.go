// Package tui — Heretic Forge Bubble Tea TUI.
// «Veritas in Crypta. Ordo ab Chao.»
// Internal HTTP client to Forge server.
package tui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/edit"
)

// Client wraps HTTP calls to the Forge server.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient creates a Forge API client with sensible defaults.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

// ChatRequest is the payload for /api/chat.
type ChatRequest struct {
	Message string   `json:"message"`
	Files   []string `json:"files,omitempty"`
	Image   string   `json:"image,omitempty"` // base64 data URI (png/jpg/gif/webp)
}

// ChatResponse is the response from /api/chat.
type ChatResponse struct {
	Response  string `json:"response"`
	Agent     string `json:"agent"`
	Status    string `json:"status"`
	Applied   string `json:"applied,omitempty"`
	Committed string `json:"committed,omitempty"`
	Error     string `json:"error,omitempty"`
}

// AgentInfo describes a registered agent.
type AgentInfo struct {
	Name    string `json:"name"`
	Display string `json:"display"`
	Glyph   string `json:"glyph"`
	Type    string `json:"type"`
	Model   string `json:"model"`
	Context string `json:"context"`
	Rank    string `json:"rank"`
	Active  bool   `json:"active"`
}

// StatusInfo is the system status snapshot.
type StatusInfo struct {
	Model   string `json:"model"`
	Repo    string `json:"repo"`
	Port    int    `json:"port"`
	Uptime  string `json:"uptime"`
	Agents  int    `json:"agents"`
	Session string `json:"session"`
}

// VersionInfo is /api/version response.
type VersionInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// HistoryMessage mirrors server.Message for /api/history.
type HistoryMessage struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	Agent   string    `json:"agent"`
	Time    time.Time `json:"time"`
}

// SessionInfo mirrors the server session list entry.
type SessionInfo struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Messages int       `json:"messages"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
	Current  bool      `json:"current"`
}

// Ping checks whether the Forge server is reachable.
func (c *Client) Ping() error {
	resp, err := c.HTTP.Get(c.BaseURL + "/api/status")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("forge server returned %d", resp.StatusCode)
	}
	return nil
}

// Version fetches /api/version.
func (c *Client) Version() (VersionInfo, error) {
	var v VersionInfo
	err := c.getJSON("/api/version", &v)
	return v, err
}

// Agents fetches the agent roster from /api/agents.
func (c *Client) Agents() ([]AgentInfo, error) {
	var out []AgentInfo
	err := c.getJSON("/api/agents", &out)
	return out, err
}

// Status fetches the system status from /api/status.
func (c *Client) Status() (StatusInfo, error) {
	var s StatusInfo
	err := c.getJSON("/api/status", &s)
	return s, err
}

// Chat sends a chat message and waits for the full response.
func (c *Client) Chat(req ChatRequest) (ChatResponse, error) {
	var resp ChatResponse
	body, _ := json.Marshal(req)
	httpResp, err := c.HTTP.Post(c.BaseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return resp, err
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return resp, err
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return resp, fmt.Errorf("decode: %w (body=%s)", err, string(raw))
	}
	return resp, nil
}

// SendMessageWithImage reads an image file, validates size and format, then
// sends it to /api/chat as a base64 data URI. Only cloud vision-capable agents
// (e.g. @minimax) will receive the image; local models ignore the field.
func (c *Client) SendMessageWithImage(message, imagePath string) (ChatResponse, error) {
	var resp ChatResponse

	info, err := os.Stat(imagePath)
	if err != nil {
		return resp, fmt.Errorf("image stat: %w", err)
	}
	const maxSize = 10 * 1024 * 1024 // 10 MB
	if info.Size() > maxSize {
		return resp, fmt.Errorf("image too large: %d MB > 10 MB limit", info.Size()/(1024*1024))
	}

	ext := strings.ToLower(filepath.Ext(imagePath))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
	default:
		return resp, fmt.Errorf("unsupported image format %q (use png/jpg/gif/webp)", ext)
	}

	data, err := os.ReadFile(imagePath)
	if err != nil {
		return resp, fmt.Errorf("image read: %w", err)
	}

	mime := "image/png"
	switch ext {
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".gif":
		mime = "image/gif"
	case ".webp":
		mime = "image/webp"
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	dataURI := fmt.Sprintf("data:%s;base64,%s", mime, b64)

	req := ChatRequest{
		Message: message,
		Image:   dataURI,
	}
	return c.Chat(req)
}

// Sessions lists sessions from /api/session/list.
func (c *Client) Sessions() ([]map[string]string, error) {
	var out []map[string]string
	err := c.getJSON("/api/session/list", &out)
	return out, err
}

// FetchSessions returns a typed session list from /api/session/list.
func (c *Client) FetchSessions() ([]SessionInfo, error) {
	var out []SessionInfo
	err := c.getJSON("/api/session/list", &out)
	return out, err
}

// FetchHistory loads messages for a specific session from /api/history.
func (c *Client) FetchHistory(sessionID string) ([]HistoryMessage, error) {
	var out []HistoryMessage
	path := "/api/history?session=" + url.QueryEscape(sessionID)
	err := c.getJSON(path, &out)
	return out, err
}

// NewSession starts a new session.
func (c *Client) NewSession() (map[string]string, error) {
	var out map[string]string
	body := strings.NewReader("{}")
	httpResp, err := c.HTTP.Post(c.BaseURL+"/api/session/new", "application/json", body)
	if err != nil {
		return out, err
	}
	defer httpResp.Body.Close()
	raw, _ := io.ReadAll(httpResp.Body)
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode: %w", err)
	}
	return out, nil
}

// Diff shows unsaved changes via /api/diff.
func (c *Client) Diff() (string, error) {
	resp, err := c.HTTP.Get(c.BaseURL + "/api/diff")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return string(raw), nil
}

// ApplyBatch sends a multi-file SEARCH/REPLACE batch to /api/edit/batch.
func (c *Client) ApplyBatch(edits []edit.FileEdit, dryRun bool) (edit.BatchResult, error) {
	var out edit.BatchResult
	payload := map[string]interface{}{
		"edits":    edits,
		"dry_run":  dryRun,
	}
	body, _ := json.Marshal(payload)
	httpResp, err := c.HTTP.Post(c.BaseURL+"/api/edit/batch", "application/json", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	defer httpResp.Body.Close()
	raw, _ := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode != 200 {
		return out, fmt.Errorf("batch edit -> %d: %s", httpResp.StatusCode, string(raw))
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode: %w (body=%s)", err, string(raw))
	}
	return out, nil
}

// Log shows recent git log via /api/log.
func (c *Client) Log() (string, error) {
	resp, err := c.HTTP.Get(c.BaseURL + "/api/log")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return string(raw), nil
}

// RepoMap fetches the repository map via /api/repo-map.
func (c *Client) RepoMap() (string, error) {
	resp, err := c.HTTP.Get(c.BaseURL + "/api/repo-map")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return string(raw), nil
}

// getJSON is a small helper for JSON GETs.
func (c *Client) getJSON(path string, out interface{}) error {
	resp, err := c.HTTP.Get(c.BaseURL + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("GET %s -> %d: %s", path, resp.StatusCode, string(raw))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// StreamEvent is one Server-Sent Event emitted by /api/chat/stream.
type StreamEvent struct {
	Event string          // "status", "token", "tool_call", "tool_result", "done", "error"
	Data  json.RawMessage // raw payload — token events are JSON strings, others are objects
}

// StreamChat opens an SSE connection to /api/chat/stream and returns a channel
// of events. The channel is closed when the stream ends or the context is canceled.
//
// SSE wire format (per server.go::handleChatStream):
//
//	event: status
//	data: {"agent":"magos","status":"thinking"}
//
//	event: token
//	data: "word "
//
//	event: done
//	data: {"agent":"magos","status":"complete"}
func (c *Client) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, <-chan error) {
	out := make(chan StreamEvent, 64)
	errc := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errc)

		body, err := json.Marshal(req)
		if err != nil {
			errc <- fmt.Errorf("marshal: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/chat/stream", bytes.NewReader(body))
		if err != nil {
			errc <- fmt.Errorf("new request: %w", err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")

		resp, err := c.HTTP.Do(httpReq)
		if err != nil {
			errc <- fmt.Errorf("http: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			raw, _ := io.ReadAll(resp.Body)
			errc <- fmt.Errorf("stream -> %d: %s", resp.StatusCode, string(raw))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		var ev StreamEvent
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.Event = strings.TrimPrefix(line, "event: ")
				ev.Data = nil
			case strings.HasPrefix(line, "data: "):
				ev.Data = json.RawMessage(strings.TrimPrefix(line, "data: "))
				// Dispatch complete event (event + data pair)
				select {
				case out <- ev:
				case <-ctx.Done():
					errc <- ctx.Err()
					return
				}
				// Reset for next event
				ev = StreamEvent{}
			case line == "":
				// SSE event separator — ignore
			default:
				// Unknown line (comments start with ":") — ignore
			}
		}
		if err := scanner.Err(); err != nil {
			errc <- fmt.Errorf("scan: %w", err)
		}
	}()

	return out, errc
}

// SwitchSession tells the server to make the given session current.
func (c *Client) SwitchSession(id string) error {
	u := c.BaseURL + "/api/session/switch?id=" + url.QueryEscape(id)
	resp, err := c.HTTP.Post(u, "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("switch session -> %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// DeleteSession removes a session from the server store.
func (c *Client) DeleteSession(id string) error {
	u := c.BaseURL + "/api/session/delete?id=" + url.QueryEscape(id)
	resp, err := c.HTTP.Post(u, "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete session -> %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// ClearSessions wipes the current session history.
func (c *Client) ClearSessions() error {
	resp, err := c.HTTP.Post(c.BaseURL+"/api/session/clear", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clear sessions -> %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}
