// Package edit — Batch multi-file SEARCH/REPLACE operations.
package edit

import (
	"fmt"
	"os"
	"strings"
)

// FileEdit is a single file + SEARCH/REPLACE pair.
type FileEdit struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

// BatchEdit is a collection of file edits applied atomically.
type BatchEdit struct {
	Edits []FileEdit `json:"edits"`
}

// EditResult describes the outcome for one file.
type EditResult struct {
	Path  string `json:"path"`
	Error string `json:"error,omitempty"`
	Diff  string `json:"diff,omitempty"`
}

// BatchResult is the aggregate outcome of a batch operation.
type BatchResult struct {
	Applied []EditResult `json:"applied"`
	Failed  []EditResult `json:"failed"`
	DryRun  bool         `json:"dry_run"`
	Edits   []FileEdit   `json:"edits,omitempty"`
}

// ApplyBatch validates and applies a batch of file edits atomically.
// If dryRun is true, previews are returned and no files are modified.
// If any edit is invalid, nothing is written and all failures are returned.
func ApplyBatch(batch BatchEdit, dryRun bool) BatchResult {
	result := BatchResult{
		DryRun:  dryRun,
		Edits:   batch.Edits,
		Applied: []EditResult{}, // always non-nil so JSON marshals as [] not null
		Failed:  []EditResult{},
	}

	if len(batch.Edits) == 0 {
		return result
	}

	// Phase 1: validate every edit (file readable, old_text unique and present).
	validated := make([]struct {
		edit    FileEdit
		orig    string
		updated string
	}, 0, len(batch.Edits))

	// Per-file working buffer — so consecutive edits on the same file see
	// the result of the previous edit instead of all re-reading the
	// original. Without this, only the LAST edit's `updated` is ever
	// written in Phase 3 and earlier edits silently disappear.
	fileState := map[string]string{} // path -> current text buffer
	for _, e := range batch.Edits {
		if strings.TrimSpace(e.Path) == "" {
			result.Failed = append(result.Failed, EditResult{
				Path:  e.Path,
				Error: "empty path",
			})
			continue
		}
		if strings.TrimSpace(e.OldText) == "" {
			result.Failed = append(result.Failed, EditResult{
				Path:  e.Path,
				Error: "empty old_text",
			})
			continue
		}

		var text string
		if buf, ok := fileState[e.Path]; ok {
			text = buf // subsequent edit on this file
		} else {
			content, err := os.ReadFile(e.Path)
			if err != nil {
				result.Failed = append(result.Failed, EditResult{
					Path:  e.Path,
					Error: fmt.Sprintf("read file: %v", err),
				})
				continue
			}
			text = string(content)
			fileState[e.Path] = text
		}

		count := strings.Count(text, e.OldText)
		if count == 0 {
			result.Failed = append(result.Failed, EditResult{
				Path:  e.Path,
				Error: "old_text not found",
			})
			continue
		}
		if count > 1 {
			result.Failed = append(result.Failed, EditResult{
				Path:  e.Path,
				Error: fmt.Sprintf("old_text found %d times (must be unique)", count),
			})
			continue
		}

		updated := strings.Replace(text, e.OldText, e.NewText, 1)
		fileState[e.Path] = updated // make next edit see this
		validated = append(validated, struct {
			edit    FileEdit
			orig    string
			updated string
		}{edit: e, orig: text, updated: updated})
	}

	// If any validation failed, abort without writing anything.
	if len(result.Failed) > 0 {
		// Still generate previews for valid edits so the user sees what would happen.
		for _, v := range validated {
			result.Applied = append(result.Applied, EditResult{
				Path: v.edit.Path,
				Diff: DiffPreview(v.edit.Path, v.orig, v.updated),
			})
		}
		return result
	}

	// Phase 2: generate previews.
	for _, v := range validated {
		result.Applied = append(result.Applied, EditResult{
			Path: v.edit.Path,
			Diff: DiffPreview(v.edit.Path, v.orig, v.updated),
		})
	}

	if dryRun {
		return result
	}

	// Phase 3: write all files atomically-ish: if a write fails, we do not roll
	// back earlier writes, but we record the failure. Callers can use dry-run first.
	for i, v := range validated {
		if err := os.WriteFile(v.edit.Path, []byte(v.updated), 0644); err != nil {
			// Move from applied to failed.
			result.Applied = append(result.Applied[:i], result.Applied[i+1:]...)
			result.Failed = append(result.Failed, EditResult{
				Path:  v.edit.Path,
				Error: fmt.Sprintf("write file: %v", err),
			})
		}
	}

	return result
}

// DiffStats returns the number of added and removed lines for a diff string.
func DiffStats(diff string) (added, removed int) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return
}
