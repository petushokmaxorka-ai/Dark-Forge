// Package mcp — Registry operations for McpServer.
package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Register adds a tool to the server.
// Returns an error if the tool's name is empty or already registered.
func (s *McpServer) Register(tool McpTool) error {
	if tool == nil {
		return fmt.Errorf("mcp: cannot register nil tool")
	}
	name := tool.Name()
	if name == "" {
		return fmt.Errorf("mcp: tool name cannot be empty")
	}
	if _, exists := s.tools[name]; exists {
		return fmt.Errorf("mcp: tool %q already registered", name)
	}
	s.tools[name] = tool
	return nil
}

// MustRegister panics on registration failure. Use for built-ins at init().
func (s *McpServer) MustRegister(tool McpTool) {
	if err := s.Register(tool); err != nil {
		panic(err)
	}
}

// Unregister removes a tool by name. Returns true if removed.
func (s *McpServer) Unregister(name string) bool {
	if _, ok := s.tools[name]; !ok {
		return false
	}
	delete(s.tools, name)
	return true
}

// List returns sorted tool metadata for /api/mcp/tools.
func (s *McpServer) List() []ToolInfo {
	out := make([]ToolInfo, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Schema:      t.Schema(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns the metadata for a single tool, or false if not found.
func (s *McpServer) Get(name string) (ToolInfo, bool) {
	t, ok := s.tools[name]
	if !ok {
		return ToolInfo{}, false
	}
	return ToolInfo{
		Name:        t.Name(),
		Description: t.Description(),
		Schema:      t.Schema(),
	}, true
}

// Execute runs the named tool with the given JSON parameters.
func (s *McpServer) Execute(name string, params json.RawMessage) (interface{}, error) {
	tool, ok := s.tools[name]
	if !ok {
		return nil, fmt.Errorf("mcp: tool %q not found (registered: %v)", name, s.toolNames())
	}
	return tool.Execute(params)
}

// Names returns the list of registered tool names (unsorted, useful for errors).
func (s *McpServer) toolNames() []string {
	names := make([]string, 0, len(s.tools))
	for n := range s.tools {
		names = append(names, n)
	}
	return names
}