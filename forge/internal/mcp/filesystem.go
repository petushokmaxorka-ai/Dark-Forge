// Package mcp — Filesystem stub tool.
// Wraps internal/tools.ReadFile and internal/tools.Glob.
// Adds tree (depth-limited). Watch is now a dedicated tool — see watch.go.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/tools"
)

// FilesystemTool exposes file operations as an MCP tool.
type FilesystemTool struct {
	basePath string // HERETIC_HOME
}

func NewFilesystemTool(basePath string) *FilesystemTool {
	return &FilesystemTool{basePath: basePath}
}

// Name implements McpTool.
func (f *FilesystemTool) Name() string { return "filesystem" }

// Description implements McpTool.
func (f *FilesystemTool) Description() string {
	return "Filesystem operations: read, glob, tree (depth-limited). For watching, use the filesystem.watch tool."
}

func (f *FilesystemTool) Schema() string {
	return `{"type":"object","properties":{"action":{"type":"string","enum":["read","glob","tree"]},"path":{"type":"string"},"pattern":{"type":"string"},"max_depth":{"type":"integer"}},"required":["action"]}`
}

type filesystemParams struct {
	Action   string `json:"action"`
	Path     string `json:"path,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
	MaxDepth int    `json:"max_depth,omitempty"`
}

// Execute implements McpTool.
func (f *FilesystemTool) Execute(params json.RawMessage) (interface{}, error) {
	var p filesystemParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("filesystem: invalid params: %w", err)
	}

	switch p.Action {
	case "read":
		if p.Path == "" {
			return nil, fmt.Errorf("filesystem.read: path is required")
		}
		// Security: resolve under basePath if relative
		target := p.Path
		if !filepath.IsAbs(target) && f.basePath != "" {
			target = filepath.Join(f.basePath, target)
		}
		res := tools.ReadFile(target)
		if res.Error != "" {
			return nil, fmt.Errorf("filesystem.read: %s", res.Error)
		}
		return map[string]interface{}{
			"path":    res.Path,
			"content": res.Content,
			"size":    res.Size,
		}, nil

	case "glob":
		if p.Pattern == "" {
			return nil, fmt.Errorf("filesystem.glob: pattern is required")
		}
		globDir := p.Path
		if globDir == "" {
			globDir = f.basePath
		}
		matches := tools.Glob(p.Pattern, globDir)
		return map[string]interface{}{
			"pattern": p.Pattern,
			"path":    globDir,
			"matches": matches,
			"count":   len(matches),
		}, nil

	case "tree":
		root := p.Path
		if root == "" {
			root = f.basePath
		}
		if root == "" {
			return nil, fmt.Errorf("filesystem.tree: root path is empty")
		}
		maxDepth := p.MaxDepth
		if maxDepth == 0 {
			maxDepth = 3
		}
		if maxDepth > 8 {
			maxDepth = 8
		}
		tree, truncated, err := buildTree(root, maxDepth)
		if err != nil {
			return nil, fmt.Errorf("filesystem.tree: %w", err)
		}
		return map[string]interface{}{
			"root":      root,
			"max_depth": maxDepth,
			"entries":   tree,
			"count":     len(tree),
			"truncated": truncated,
		}, nil

	case "watch":
		return nil, fmt.Errorf("filesystem.watch is a separate tool (action=watch was removed in v1.2; use filesystem.watch tool directly)")

	default:
		return nil, fmt.Errorf("filesystem: unknown action %q (use read, glob, tree, watch)", p.Action)
	}
}

// buildTree walks the directory and returns sorted entries up to maxDepth deep.
// Returns (entries, truncated, err). truncated=true if maxDepth clipped the walk.
func buildTree(root string, maxDepth int) ([]map[string]interface{}, bool, error) {
	var out []map[string]interface{}
	truncated := false

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// skip permission errors etc., don't abort the whole walk
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		if rel == "." {
			out = append(out, map[string]interface{}{
				"path": ".",
				"type": "dir",
			})
			return nil
		}
		depth := strings.Count(rel, string(os.PathSeparator)) + 1
		if depth > maxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			truncated = true
			return nil
		}

		entryType := "file"
		if info.IsDir() {
			entryType = "dir"
		}
		entry := map[string]interface{}{
			"path": rel,
			"type": entryType,
		}
		if !info.IsDir() {
			entry["size"] = info.Size()
		}
		out = append(out, entry)
		return nil
	})
	if err != nil {
		return nil, truncated, err
	}

	sort.Slice(out, func(i, j int) bool {
		// dirs before files at the same prefix; otherwise alpha
		ip, _ := out[i]["path"].(string)
		jp, _ := out[j]["path"].(string)
		return ip < jp
	})
	return out, truncated, nil
}