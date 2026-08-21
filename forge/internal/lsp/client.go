// Package lsp — LSP client methods (Initialize, DidOpen, Hover, etc.)
package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Diagnostic represents one LSP diagnostic (error/warning).
type Diagnostic struct {
	Line     int    `json:"line"`     // 0-based
	Col      int    `json:"col"`      // 0-based
	Message  string `json:"message"`
	Severity string `json:"severity"` // "error", "warning", "info", "hint"
	Source   string `json:"source,omitempty"`
}

// Hover represents hover information at a position.
type Hover struct {
	Contents string `json:"contents"`
	Range    *Range `json:"range,omitempty"`
}

// Range represents a text range.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position represents a 0-based line/character position.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Location represents a location in a file.
type Location struct {
	URI   string `json:"uri"`
	Range Range `json:"range"`
}

// pathToURI converts a filesystem path to an LSP URI.
func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return "file://" + abs
}

// Initialize performs the LSP initialize handshake.
func (s *LSPServer) Initialize(workspace string) error {
	absWS, _ := filepath.Abs(workspace)
	params := map[string]interface{}{
		"processId":    os.Getpid(),
		"rootUri":      "file://" + absWS,
		"capabilities": map[string]interface{}{},
		"workspaceFolders": []map[string]string{
			{"uri": "file://" + absWS, "name": filepath.Base(absWS)},
		},
	}
	_, err := s.Call("initialize", params)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	// Send initialized notification
	s.Notify("initialized", map[string]interface{}{})
	return nil
}

// DidOpen notifies the server about an opened file.
func (s *LSPServer) DidOpen(path, language, content string) error {
	uri := pathToURI(path)
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":       uri,
			"languageId": languageID(path),
			"version":    1,
			"text":       content,
		},
	}
	return s.Notify("textDocument/didOpen", params)
}

// Diagnostics returns cached diagnostics for a file.
func (s *LSPServer) Diagnostics(path string) []Diagnostic {
	uri := pathToURI(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	diags := s.diagStore[uri]
	if diags == nil {
		return []Diagnostic{}
	}
	return diags
}

// Hover requests hover information at a position.
func (s *LSPServer) Hover(path string, line, col int) (*Hover, error) {
	uri := pathToURI(path)
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uri},
		"position":     map[string]int{"line": line, "character": col},
	}
	result, err := s.Call("textDocument/hover", params)
	if err != nil {
		return nil, fmt.Errorf("hover: %w", err)
	}
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}
	// Parse result — could be {contents: {value: "..."}} or {contents: "..."}
	var raw struct {
		Contents struct {
			Value string `json:"value"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(result, &raw); err != nil {
		// Try simple string
		var s string
		if json.Unmarshal(result, &s) == nil {
			return &Hover{Contents: s}, nil
		}
		return nil, nil
	}
	return &Hover{Contents: raw.Contents.Value}, nil
}

// Definition requests go-to-definition.
func (s *LSPServer) Definition(path string, line, col int) (*Location, error) {
	uri := pathToURI(path)
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uri},
		"position":     map[string]int{"line": line, "character": col},
	}
	result, err := s.Call("textDocument/definition", params)
	if err != nil {
		return nil, fmt.Errorf("definition: %w", err)
	}
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}
	// Could be a single Location or array
	var loc Location
	if err := json.Unmarshal(result, &loc); err == nil && loc.URI != "" {
		return &loc, nil
	}
	var locs []Location
	if err := json.Unmarshal(result, &locs); err == nil && len(locs) > 0 {
		return &locs[0], nil
	}
	return nil, nil
}

// Call sends a JSON-RPC request and waits for the response.
func (s *LSPServer) Call(method string, params interface{}) (json.RawMessage, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, err := encodeMessage(req)
	if err != nil {
		return nil, err
	}

	if _, err := s.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Read response (simplified — reads next message that matches ID)
	// In production, this would use channels for async matching
	timeout := time.After(10 * time.Second)
	_ = timeout // We use blocking read for simplicity

	for {
		raw, err := decodeMessage(s.stdout)
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		var resp RPCResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			continue
		}
		if resp.ID == id {
			if resp.Error != nil {
				return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
			}
			return resp.Result, nil
		}
		// Not our response — might be a notification, skip
	}
}

// Notify sends a JSON-RPC notification (no response expected).
func (s *LSPServer) Notify(method string, params interface{}) error {
	notif := RPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, err := encodeMessage(notif)
	if err != nil {
		return err
	}
	_, err = s.stdin.Write(data)
	return err
}

// languageID returns the LSP language ID for a file extension.
func languageID(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".ts":
		return "typescript"
	case ".js":
		return "javascript"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".md":
		return "markdown"
	default:
		return "plaintext"
	}
}
