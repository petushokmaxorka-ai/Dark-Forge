// Package lsp — LSP server lifecycle management.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
)

// LSPManager manages language servers for multiple languages.
type LSPManager struct {
	mu      sync.Mutex
	servers map[string]*LSPServer
}

// LSPServer wraps a running language server process.
type LSPServer struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *bufferedReader
	nextID     int
	mu         sync.Mutex
	diagStore  map[string][]Diagnostic // uri → diagnostics (from notifications)
	workspace  string
	language   string
}

// NewLSPManager creates a new empty manager.
func NewLSPManager() *LSPManager {
	return &LSPManager{
		servers: make(map[string]*LSPServer),
	}
}

// Start launches a language server for the given language.
func (m *LSPManager) Start(language, workspace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.servers[language]; exists {
		return nil // already running
	}

	binary, args, err := serverCommand(language)
	if err != nil {
		return err
	}

	cmd := exec.Command(binary, args...)
	cmd.Dir = workspace

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", binary, err)
	}

	server := &LSPServer{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    &bufferedReader{r: bufio.NewReader(stdoutPipe)},
		diagStore: make(map[string][]Diagnostic),
		workspace: workspace,
		language:  language,
	}

	// Start background reader for notifications
	go server.readLoop()

	m.servers[language] = server
	log.Printf("⚙ LSP %s started (pid %d)", language, cmd.Process.Pid)
	return nil
}

// StopAll stops all running language servers.
func (m *LSPManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for lang, s := range m.servers {
		s.stdin.Close()
		s.cmd.Process.Kill()
		s.cmd.Wait()
		delete(m.servers, lang)
		log.Printf("⚙ LSP %s stopped", lang)
	}
}

// EnsureRunning starts the server if not already running.
func (m *LSPManager) EnsureRunning(language, workspace string) error {
	m.mu.Lock()
	_, exists := m.servers[language]
	m.mu.Unlock()
	if exists {
		return nil
	}
	return m.Start(language, workspace)
}

// Get returns the server for a language, or nil.
func (m *LSPManager) Get(language string) *LSPServer {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.servers[language]
}

// serverCommand returns the binary and args for a language server.
func serverCommand(language string) (string, []string, error) {
	switch language {
	case "go", "golang":
		return "gopls", []string{"-rpc.trace", "serve"}, nil
	case "python", "py":
		return "pylsp", nil, nil
	case "rust", "rs":
		return "rust-analyzer", nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported language: %s", language)
	}
}

// readLoop reads messages from the server stdout in background.
// Captures diagnostics notifications and stores them.
func (s *LSPServer) readLoop() {
	for {
		raw, err := decodeMessage(s.stdout)
		if err != nil {
			if err != io.EOF {
				log.Printf("⚙ LSP %s read error: %v", s.language, err)
			}
			return
		}

		// Try to parse as response (has "id")
		var resp RPCResponse
		if err := json.Unmarshal(raw, &resp); err == nil && resp.ID > 0 {
			// It's a response — handled by Call() via channel
			// For simplicity we store the latest response
			continue
		}

		// Try to parse as notification
		var notif RPCNotification
		if err := json.Unmarshal(raw, &notif); err == nil {
			if notif.Method == "textDocument/publishDiagnostics" {
				s.handleDiagnostics(notif.Params)
			}
		}
	}
}

// handleDiagnostics extracts diagnostics from a notification.
func (s *LSPServer) handleDiagnostics(params interface{}) {
	b, err := json.Marshal(params)
	if err != nil {
		return
	}
	var data struct {
		URI         string       `json:"uri"`
		Diagnostics []Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return
	}
	s.mu.Lock()
	s.diagStore[data.URI] = data.Diagnostics
	s.mu.Unlock()
}
