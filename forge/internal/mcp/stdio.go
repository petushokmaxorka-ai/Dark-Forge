// Package mcp — JSON-RPC over stdio transport for MCP.
// Allows external clients (OpenCode, Cursor, Claude Desktop) to invoke
// MCP tools via subprocess stdio.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// JSONRPCRequest is the wire format for MCP over stdio.
// Spec: https://www.jsonrpc.org/specification
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"` // "2.0"
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is the wire format for MCP responses.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError is the error envelope for JSON-RPC.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes.
const (
	RPCErrParse          = -32700
	RPCErrInvalidRequest = -32600
	RPCErrMethodNotFound = -32601
	RPCErrInvalidParams  = -32602
	RPCErrInternal       = -32603
)

// StdioServer serves MCP requests over stdin/stdout using newline-delimited JSON.
type StdioServer struct {
	server *McpServer
	in     io.Reader
	out    io.Writer
	mu     sync.Mutex // serializes writes to out
}

// NewStdioServer creates a stdio MCP server using os.Stdin / os.Stdout.
func NewStdioServer(s *McpServer) *StdioServer {
	return &StdioServer{
		server: s,
		in:     os.Stdin,
		out:    os.Stdout,
	}
}

// NewStdioServerWithIO is for testing — caller provides in/out streams.
func NewStdioServerWithIO(s *McpServer, in io.Reader, out io.Writer) *StdioServer {
	return &StdioServer{server: s, in: in, out: out}
}

// Serve reads newline-delimited JSON-RPC requests until EOF, dispatching
// each to the MCP server. Writes newline-delimited JSON-RPC responses.
//
// Convention: input is one JSON object per line. Output is one JSON object
// per line. No logs to stdout (logs go to stderr).
func (s *StdioServer) Serve() error {
	scanner := bufio.NewScanner(s.in)
	// Allow large params (some MCP payloads are big).
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(nil, RPCErrParse, "parse error: "+err.Error(), nil)
			continue
		}
		if req.JSONRPC != "2.0" {
			s.writeError(req.ID, RPCErrInvalidRequest, "jsonrpc must be \"2.0\"", nil)
			continue
		}
		if req.Method == "" {
			s.writeError(req.ID, RPCErrInvalidRequest, "method is required", nil)
			continue
		}

		resp := s.dispatch(req)
		s.writeResponse(resp)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("stdio: scan: %w", err)
	}
	return nil
}

// dispatch routes a JSON-RPC request to the MCP server.
// Supported methods:
//   - "tools/list"             → List of ToolInfo
//   - "tools/call"             → Execute tool (params = {name, arguments})
//   - "initialize"             → MCP handshake (returns server info)
func (s *StdioServer) dispatch(req JSONRPCRequest) JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]interface{}{
					"name":    "heretic-forge-mcp",
					"version": "1.2.0",
				},
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
			},
		}

	case "tools/list":
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"tools": s.server.List(),
			},
		}

	case "tools/call":
		var callParams struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &callParams); err != nil {
			return s.errResponse(req.ID, RPCErrInvalidParams, "params must be {name, arguments}: "+err.Error())
		}
		if callParams.Name == "" {
			return s.errResponse(req.ID, RPCErrInvalidParams, "name is required")
		}
		result, err := s.server.Execute(callParams.Name, callParams.Arguments)
		if err != nil {
			return JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &RPCError{
					Code:    RPCErrInternal,
					Message: err.Error(),
				},
			}
		}
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "json", "data": result},
				},
			},
		}

	case "notifications/initialized", "ping":
		// No-op notifications / heartbeat.
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"ok": true},
		}

	default:
		return s.errResponse(req.ID, RPCErrMethodNotFound, "method not found: "+req.Method)
	}
}

func (s *StdioServer) errResponse(id interface{}, code int, msg string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}
}

// writeResponse serializes resp as JSON + newline to out.
func (s *StdioServer) writeResponse(resp JSONRPCResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.Marshal(resp)
	if err != nil {
		// Fall back to a minimal error envelope.
		fallback := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      resp.ID,
			Error:   &RPCError{Code: RPCErrInternal, Message: "marshal: " + err.Error()},
		}
		raw, _ = json.Marshal(fallback)
	}
	_, _ = s.out.Write(raw)
	_, _ = s.out.Write([]byte("\n"))
}

// writeError is a shortcut for parse / validation errors.
func (s *StdioServer) writeError(id interface{}, code int, msg string, data interface{}) {
	s.writeResponse(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg, Data: data},
	})
}