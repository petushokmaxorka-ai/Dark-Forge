// Package mcp — Filesystem watcher via polling (no fsnotify dependency).
// Polls modtimes every Poll() call and returns paths whose mtime changed
// since the last successful Poll.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileWatcher tracks file modtimes under a root path and reports changes.
type FileWatcher struct {
	mu          sync.Mutex
	root        string
	last        map[string]int64 // path → modtime unix nano
	recursive   bool
	pattern     string // optional glob filter (e.g. "*.go"); empty = all
	initialized bool   // false until first Poll() snapshots the tree
}

// NewFileWatcher creates a watcher rooted at `root`.
// `pattern` is an optional filepath.Match filter (e.g. "*.go").
// `recursive` controls whether subdirectories are walked.
func NewFileWatcher(root, pattern string, recursive bool) *FileWatcher {
	return &FileWatcher{
		root:      root,
		last:      make(map[string]int64),
		recursive: recursive,
		pattern:   pattern,
	}
}

// Root returns the watched root path.
func (fw *FileWatcher) Root() string { return fw.root }

// pollOnce scans the watched tree once, returns paths whose mtime changed.
// First call after NewFileWatcher returns an empty slice (it just snapshots).
func (fw *FileWatcher) pollOnce() []string {
	root := fw.root
	if root == "" {
		return nil
	}

	current := make(map[string]int64)
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip permission errors etc.
		}
		if info.IsDir() {
			if path != root && !fw.recursive && !isSameLevel(root, path) {
				return filepath.SkipDir
			}
			// Skip well-known noise directories.
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		// File
		if fw.pattern != "" {
			matched, _ := filepath.Match(fw.pattern, filepath.Base(path))
			if !matched {
				return nil
			}
		}
		current[path] = info.ModTime().UnixNano()
		return nil
	})
	if walkErr != nil {
		return nil
	}

	fw.mu.Lock()
	defer fw.mu.Unlock()

	// First poll: snapshot only, no changes reported.
	if !fw.initialized {
		fw.last = current
		fw.initialized = true
		return []string{}
	}

	changes := make([]string, 0)
	for path, mtime := range current {
		prev, ok := fw.last[path]
		if !ok || mtime != prev {
			changes = append(changes, path)
		}
	}
	for path := range fw.last {
		if _, exists := current[path]; !exists {
			changes = append(changes, path) // deleted
		}
	}
	fw.last = current
	sort.Strings(changes)
	return changes
}

// Poll performs one polling cycle and returns the list of changed files.
// On the first call, returns an empty slice (it just builds the initial snapshot).
// Subsequent calls return files whose mtime changed OR that were deleted.
func (fw *FileWatcher) Poll() []string {
	return fw.pollOnce()
}

// snapshot walks the tree once and returns ALL files (no diff logic).
// Useful for `filesystem.watch snapshot` action that wants a full listing
// while still seeding the watcher's internal modtime map.
func (fw *FileWatcher) snapshot() []string {
	root := fw.root
	if root == "" {
		return nil
	}

	out := make([]string, 0)
	current := make(map[string]int64)

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if path != root && !fw.recursive && !isSameLevel(root, path) {
				return filepath.SkipDir
			}
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if fw.pattern != "" {
			matched, _ := filepath.Match(fw.pattern, filepath.Base(path))
			if !matched {
				return nil
			}
		}
		current[path] = info.ModTime().UnixNano()
		out = append(out, path)
		return nil
	})

	fw.mu.Lock()
	fw.last = current
	fw.initialized = true
	fw.mu.Unlock()

	sort.Strings(out)
	return out
}

// Reset clears the modtime snapshot. Next Poll will rebuild from scratch.
func (fw *FileWatcher) Reset() {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.last = make(map[string]int64)
	fw.initialized = false
}

// WatchTool exposes the FileWatcher as an MCP tool.
type WatchTool struct {
	watcher *FileWatcher
}

// NewWatchTool wraps a FileWatcher as an McpTool.
func NewWatchTool(root, pattern string, recursive bool) *WatchTool {
	return &WatchTool{watcher: NewFileWatcher(root, pattern, recursive)}
}

// Name implements McpTool.
func (w *WatchTool) Name() string { return "filesystem.watch" }

// Description implements McpTool.
func (w *WatchTool) Description() string {
	return "Poll filesystem for changes (mtime-based). Returns paths that changed since last poll."
}

func (w *WatchTool) Schema() string {
	return `{"type":"object","properties":{"action":{"type":"string","enum":["poll","reset","snapshot"]},"pattern":{"type":"string"},"recursive":{"type":"boolean"}},"required":["action"]}`
}

type watchParams struct {
	Action    string `json:"action"`
	Pattern   string `json:"pattern,omitempty"`
	Recursive *bool  `json:"recursive,omitempty"`
}

// Execute implements McpTool.
func (w *WatchTool) Execute(params json.RawMessage) (interface{}, error) {
	var p watchParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("filesystem.watch: invalid params: %w", err)
	}
	switch p.Action {
	case "poll":
		changes := w.watcher.Poll()
		return map[string]interface{}{
			"root":    w.watcher.Root(),
			"changes": changes,
			"count":   len(changes),
			"polled":  time.Now().UTC().Format(time.RFC3339),
			"hint":    "call poll repeatedly with 1-5s interval for live updates",
		}, nil
	case "reset":
		w.watcher.Reset()
		return map[string]interface{}{
			"reset":  true,
			"polled": time.Now().UTC().Format(time.RFC3339),
		}, nil
	case "snapshot":
		// Walk the tree and return the full file list, also seeding the
		// watcher's internal snapshot so the next "poll" returns deltas.
		files := w.watcher.snapshot()
		return map[string]interface{}{
			"root":   w.watcher.Root(),
			"files":  files,
			"count":  len(files),
			"_note":  "snapshot taken; subsequent polls return deltas",
			"polled": time.Now().UTC().Format(time.RFC3339),
		}, nil
	default:
		return nil, fmt.Errorf("filesystem.watch: unknown action %q (use poll, reset, snapshot)", p.Action)
	}
}

// isSameLevel reports whether path is a direct child of root (one level deep).
func isSameLevel(root, path string) bool {
	parent := filepath.Dir(path)
	return parent == root
}

// FilterChangesByExt returns only paths whose file extension is in `exts` (case-insensitive).
// Used as a helper for callers that want to filter watch results.
func FilterChangesByExt(paths []string, exts []string) []string {
	if len(exts) == 0 {
		return paths
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		ext := strings.ToLower(filepath.Ext(p))
		for _, want := range exts {
			if ext == strings.ToLower(want) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}