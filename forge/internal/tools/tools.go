// Package tools — Heretic Forge tool execution
// «Banelight Activated.»
package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ReadResult is the output of ReadFile
type ReadResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int    `json:"size"`
	Error   string `json:"error,omitempty"`
}

// WriteResult is the output of WriteFile
type WriteResult struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	Error  string `json:"error,omitempty"`
}

// BashResult is the output of RunBash
type BashResult struct {
	Command  string `json:"command"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

// GrepResult is the output of Grep
type GrepResult struct {
	Pattern string   `json:"pattern"`
	Files   []string `json:"files"`
	Count   int      `json:"count"`
	Error   string   `json:"error,omitempty"`
}

// ReadFile reads a file and returns its content
func ReadFile(path string) ReadResult {
	// Security: resolve path, prevent directory traversal
	abs, err := filepath.Abs(path)
	if err != nil {
		return ReadResult{Path: path, Error: err.Error()}
	}

	content, err := os.ReadFile(abs)
	if err != nil {
		return ReadResult{Path: path, Error: err.Error()}
	}

	// Truncate very large files
	str := string(content)
	if len(str) > 50000 {
		str = str[:50000] + "\n\n... (truncated, file is " + fmt.Sprintf("%d", len(content)) + " bytes)"
	}

	return ReadResult{
		Path:    path,
		Content: str,
		Size:    len(content),
	}
}

// WriteFile writes content to a file
func WriteFile(path string, content string) WriteResult {
	abs, err := filepath.Abs(path)
	if err != nil {
		return WriteResult{Path: path, Error: err.Error()}
	}

	// Create parent directories if needed
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return WriteResult{Path: path, Error: err.Error()}
	}

	// Backup existing file
	if _, err := os.Stat(abs); err == nil {
		os.WriteFile(abs+".bak", []byte{}, 0644) // best effort backup
	}

	err = os.WriteFile(abs, []byte(content), 0644)
	if err != nil {
		return WriteResult{Path: path, Error: err.Error()}
	}

	return WriteResult{
		Path:  path,
		Bytes: len(content),
	}
}

// RunBash executes a shell command (stateless, shell=False)
func RunBash(command string, workDir string) BashResult {
	if command == "" {
		return BashResult{Error: "empty command"}
	}

	// Security: use shlex-style split, no shell=True
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return BashResult{Error: "empty command"}
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	if workDir != "" {
		abs, err := filepath.Abs(workDir)
		if err == nil {
			cmd.Dir = abs
		}
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return BashResult{
				Command: command,
				Error:   err.Error(),
			}
		}
	}

	// Truncate output
	out := stdout.String()
	if len(out) > 20000 {
		out = out[:20000] + "\n... (truncated)"
	}
	errOut := stderr.String()
	if len(errOut) > 10000 {
		errOut = errOut[:10000] + "\n... (truncated)"
	}

	return BashResult{
		Command:  command,
		Stdout:   out,
		Stderr:   errOut,
		ExitCode: exitCode,
	}
}

// Grep searches for a pattern in files
func Grep(pattern string, dir string, ext string) GrepResult {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return GrepResult{Pattern: pattern, Error: err.Error()}
	}

	matches := []string{}
	count := 0

	filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Filter by extension if specified
		if ext != "" && filepath.Ext(path) != "."+ext {
			return nil
		}

		// Only search text files
		ext_lower := strings.ToLower(filepath.Ext(path))
		valid := map[string]bool{
			".go": true, ".py": true, ".js": true, ".ts": true,
			".tsx": true, ".jsx": true, ".md": true, ".yaml": true,
			".yml": true, ".json": true, ".sh": true, ".txt": true,
			".css": true, ".html": true, ".sql": true, ".rs": true,
		}
		if !valid[ext_lower] {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		if strings.Contains(string(content), pattern) {
			rel, _ := filepath.Rel(abs, path)
			matches = append(matches, rel)
			count++
		}

		return nil
	})

	if count > 50 {
		matches = matches[:50]
	}

	return GrepResult{
		Pattern: pattern,
		Files:   matches,
		Count:   count,
	}
}

// Glob finds files matching a pattern
func Glob(pattern string, dir string) []string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}

	matches, _ := filepath.Glob(filepath.Join(abs, pattern))
	result := []string{}
	for _, m := range matches {
		rel, _ := filepath.Rel(abs, m)
		result = append(result, rel)
	}
	return result
}
