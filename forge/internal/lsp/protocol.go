// Package lsp — Language Server Protocol integration for Forge.
// «Knowledge is power. Guard it well.»
//
// JSON-RPC 2.0 over stdio with Content-Length framing.
// Supports gopls (Go), pylsp (Python), rust-analyzer (Rust).
package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// RPCRequest is a JSON-RPC 2.0 request.
type RPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// RPCNotification is a JSON-RPC 2.0 notification (no ID).
type RPCNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// RPCResponse is a JSON-RPC 2.0 response.
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// encodeMessage serialises a JSON-RPC message with Content-Length header.
func encodeMessage(msg interface{}) ([]byte, error) {
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal rpc: %w", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	return []byte(header + string(body)), nil
}

// decodeMessage reads one JSON-RPC message with Content-Length framing.
func decodeMessage(r *bufferedReader) (json.RawMessage, error) {
	// Read headers until empty line
	contentLength := 0
	for {
		line, err := r.ReadLine()
		if err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			fmt.Sscanf(line, "Content-Length: %d", &contentLength)
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("no Content-Length header")
	}
	// Read body
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return json.RawMessage(body), nil
}

// bufferedReader wraps a reader with ReadLine support.
type bufferedReader struct {
	r   io.Reader
	buf []byte
}

func (b *bufferedReader) Read(p []byte) (int, error) {
	return b.r.Read(p)
}

// ReadLine reads one line (including \n).
func (b *bufferedReader) ReadLine() (string, error) {
	// Simple line reader — reads byte by byte until \n
	var line strings.Builder
	oneByte := make([]byte, 1)
	for {
		n, err := b.r.Read(oneByte)
		if err != nil {
			return line.String(), err
		}
		if n == 0 {
			continue
		}
		line.WriteByte(oneByte[0])
		if oneByte[0] == '\n' {
			return line.String(), nil
		}
	}
}
