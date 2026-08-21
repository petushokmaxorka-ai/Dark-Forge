// Package edit — Aider-style SEARCH/REPLACE diff parsing and application.
package edit

import (
	"fmt"
	"os"
	"strings"
)

// Edit represents a single SEARCH/REPLACE operation.
type Edit struct {
	Search  string `json:"search"`
	Replace string `json:"replace"`
}

// ParseDiffs parses SEARCH/REPLACE blocks from a model response.
func ParseDiffs(response string) ([]Edit, string) {
	var edits []Edit
	var explanation strings.Builder

	lines := strings.Split(response, "\n")
	inBlock := false
	inSearch := false
	inReplace := false
	var searchBuf, replaceBuf strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "<<<<<<< SEARCH" {
			inBlock = true
			inSearch = true
			inReplace = false
			searchBuf.Reset()
			replaceBuf.Reset()
			continue
		}
		if trimmed == "=======" && inBlock {
			inSearch = false
			inReplace = true
			continue
		}
		if trimmed == ">>>>>>> REPLACE" && inBlock {
			edits = append(edits, Edit{
				Search:  searchBuf.String(),
				Replace: replaceBuf.String(),
			})
			inBlock = false
			inSearch = false
			inReplace = false
			continue
		}

		if inSearch {
			searchBuf.WriteString(line)
			searchBuf.WriteString("\n")
		} else if inReplace {
			replaceBuf.WriteString(line)
			replaceBuf.WriteString("\n")
		} else if !inBlock {
			explanation.WriteString(line)
			explanation.WriteString("\n")
		}
	}

	return edits, strings.TrimSpace(explanation.String())
}

// ApplyEdits applies edits to a file. Returns error if Search not found.
func ApplyEdits(filePath string, edits []Edit) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	text := string(content)
	for i, edit := range edits {
		search := strings.TrimSpace(edit.Search)
		replace := strings.TrimSpace(edit.Replace)
		if !strings.Contains(text, search) {
			return fmt.Errorf("edit %d: SEARCH text not found in %s", i, filePath)
		}
		text = strings.Replace(text, search, replace, 1)
	}

	return os.WriteFile(filePath, []byte(text), 0644)
}

// ApplyEditsWithBackup applies edits, creating a .bak file first.
func ApplyEditsWithBackup(filePath string, edits []Edit) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Create backup
	bakPath := filePath + ".bak"
	if err := os.WriteFile(bakPath, content, 0644); err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	return ApplyEdits(filePath, edits)
}

// Preview returns the diff-like preview of applying edits without writing.
func Preview(filePath string, edits []Edit) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	updated := string(content)

	for i, e := range edits {
		count := strings.Count(updated, e.Search)
		if count == 0 {
			return "", fmt.Errorf("edit %d: SEARCH block not found", i+1)
		}
		if count > 1 {
			return "", fmt.Errorf("edit %d: SEARCH block found %d times", i+1, count)
		}
		updated = strings.Replace(updated, e.Search, e.Replace, 1)
	}
	return DiffPreview(filePath, string(content), updated), nil
}

// DiffPreview returns a unified-style diff between old and new content.
func DiffPreview(filePath, oldText, newText string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	var out strings.Builder
	out.WriteString(fmt.Sprintf("--- %s\n+++ %s\n", filePath, filePath))
	i, j := 0, 0
	for i < len(oldLines) || j < len(newLines) {
		if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
			out.WriteString(" " + oldLines[i] + "\n")
			i++
			j++
		} else if i < len(oldLines) && (j >= len(newLines) || !containsLine(newLines, oldLines[i])) {
			out.WriteString("-" + oldLines[i] + "\n")
			i++
		} else {
			out.WriteString("+" + newLines[j] + "\n")
			j++
		}
	}
	return strings.TrimSpace(out.String())
}

func containsLine(lines []string, line string) bool {
	for _, l := range lines {
		if l == line {
			return true
		}
	}
	return false
}
