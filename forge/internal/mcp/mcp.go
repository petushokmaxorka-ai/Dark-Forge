// Package mcp — Model Context Protocol stub for Heretic Forge.
// «Veritas in Crypta. Ordo ab Chao.»
// External tool registry: interface, server, built-in tools.
//
// This is a STUB. It defines the McpTool contract and the McpServer registry
// so the agent can SEE tools and CALL them. Real protocol adapters (stdio /
// HTTP / SSE transports) and real tool implementations land in v1.2+.
package mcp

import (
	"encoding/json"
)

// McpTool is the interface every MCP-compatible tool implements.
type McpTool interface {
	// Name returns the unique tool name (used in registry + execute calls).
	Name() string
	// Description is a one-line summary shown to the agent.
	Description() string
	// Schema returns a JSON Schema for the tool's parameters (may be empty).
	Schema() string
	// Execute runs the tool with the given JSON parameters.
	// Returns the result as interface{} or an error.
	Execute(params json.RawMessage) (interface{}, error)
}

// McpServer holds the registered tools.
type McpServer struct {
	tools map[string]McpTool
}

// ToolInfo is the public metadata exposed via /api/mcp/tools.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Schema      string `json:"schema,omitempty"`
}

// NewServer creates an empty McpServer.
func NewServer() *McpServer {
	return &McpServer{
		tools: make(map[string]McpTool),
	}
}

// Count returns the number of registered tools.
func (s *McpServer) Count() int {
	return len(s.tools)
}

// Has reports whether a tool with the given name is registered.
func (s *McpServer) Has(name string) bool {
	_, ok := s.tools[name]
	return ok
}