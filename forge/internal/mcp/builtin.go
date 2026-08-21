// Package mcp — Built-in tool registration.
// Called from server.New() so every forge process starts with the default
// MCP tool set wired in.
package mcp

import (
	"os"
)

// RegisterBuiltins wires the default tool set into the given server.
//   - github:        live GitHub REST API client (stub if GITHUB_TOKEN unset)
//   - filesystem:    read/glob/tree operations
//   - filesystem.watch: polling-based file change watcher
//
// Environment variables:
//   - HERETIC_REPO     overrides default repo for github tool (owner/name)
//   - HERETIC_HOME     overrides filesystem root
//   - HERETIC_WATCH_PATTERN overrides the watch glob (default "*")
func RegisterBuiltins(s *McpServer) {
	repo := os.Getenv("HERETIC_REPO")
	if repo == "" {
		repo = "petushokmaxorka-ai/Heretic-OS"
	}
	s.MustRegister(NewGitHubTool(repo))

	home := os.Getenv("DARKFORGE_HOME")
	if home == "" {
		home = os.Getenv("HERETIC_HOME")
	}
	if home == "" {
		if cwd, err := os.Getwd(); err == nil {
			home = cwd
		} else {
			home = "."
		}
	}
	s.MustRegister(NewFilesystemTool(home))

	watchPattern := os.Getenv("HERETIC_WATCH_PATTERN")
	if watchPattern == "" {
		watchPattern = "*"
	}
	s.MustRegister(NewWatchTool(home, watchPattern, true))
}