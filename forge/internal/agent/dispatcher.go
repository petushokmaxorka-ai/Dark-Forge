// Package agent — TaskDispatcher orchestrates Magos → Servitor → Skitarii.
// Dark Mechanicus style. Per Ignis, per Ferrum, per Codicem.
package agent

import (
	"fmt"
	"log"
	"strings"

	"github.com/petushokmaxorka-ai/dark-forge/forge/internal/git"
	forgeMap "github.com/petushokmaxorka-ai/dark-forge/forge/internal/map"
)

// ═══════════════════════════════════════════════════════════════════════
// TASK DISPATCHER — Orchestration Layer
// ═══════════════════════════════════════════════════════════════════════

// TaskDispatcher orchestrates the three agents: Magos (plan), Servitor (edit), Skitarii (explore).
type TaskDispatcher struct {
 Magos    *MagosAgent // Vox Dei planner (local swarm)
	Servitor *Servitor   // 8B editor
	Skitarii *Skitarii   // 1.5B explorer
	RepoMap  *forgeMap.RepoMap
	Git      *git.GitOps
}

// Response represents the result of a dispatched task.
type Response struct {
	Text      string `json:"text"`      // Response text
	Applied   bool   `json:"applied"`   // Were edits applied?
	Committed bool   `json:"committed"` // Were edits committed?
	Agent     string `json:"agent"`     // Which agent responded
	Error     string `json:"error,omitempty"`
}

// NewTaskDispatcher creates a new dispatcher with all agents.
func NewTaskDispatcher(modelURL, repoPath string) *TaskDispatcher {
	return &TaskDispatcher{

		Servitor: NewServitor(modelURL, "Ferrum Flagellum"), // 8B
		Skitarii: NewSkitarii(modelURL, "Vox Minor"),        // 1.5B
		RepoMap:  forgeMap.NewRepoMap(repoPath),
		Git:      git.NewGitOps(repoPath),
	}
}

// Handle processes a request through the agent pipeline.
// Flow: Skitarii (explore) → Magos (plan) → Servitor (apply) → Lint → Git
func (td *TaskDispatcher) Handle(request string, files []string) (*Response, error) {
	log.Printf("⚒ TaskDispatcher: %s", truncate(request, 80))

	// Step 1: Skitarii explores relevant files (if needed)
	var exploration string
	if len(files) > 0 {
		var summaries []string
		for _, f := range files {
			summary, err := td.Skitarii.Explore(f)
			if err != nil {
				log.Printf("☠ Skitarii explore error: %v", err)
				continue
			}
			summaries = append(summaries, fmt.Sprintf("=== %s ===\n%s", f, summary))
		}
		exploration = strings.Join(summaries, "\n\n")
	}

	// Step 2: Magos plans changes (Vox Dei memory/general, with repo map in context)
	// Note: Magos is called externally via server.go callMagos
	// Here we just prepare the context
	repoMapStr := ""
	if td.RepoMap != nil {
		repoMapStr = td.RepoMap.Render(1024)
	}

	plan := fmt.Sprintf(`Request: %s

Exploration:
%s

Repo Map:
%s

Plan your changes. Output SEARCH/REPLACE blocks.`, request, exploration, repoMapStr)

	// Step 3: Servitor applies diffs (8B, with plan + file)
	if len(files) > 0 {
		for _, f := range files {
			explanation, err := td.Servitor.ApplyPlan(plan, f)
			if err != nil {
				log.Printf("☠ Servitor apply error: %v", err)
				return &Response{
					Text:    fmt.Sprintf("[Коррупция: %v]", err),
					Applied: false,
					Agent:   "servitor",
					Error:   err.Error(),
				}, nil
			}

			// Step 4: Lint check
			ok, lintErr := td.Servitor.Lint(f)
			if !ok {
				log.Printf("☠ Lint failed: %s", lintErr)
				// Undo changes
				if undoErr := td.Git.Undo(); undoErr != nil {
					log.Printf("☠ Undo error: %v", undoErr)
				}
				return &Response{
					Text:    fmt.Sprintf("[Lint failed: %s]", lintErr),
					Applied: false,
					Agent:   "servitor",
					Error:   lintErr,
				}, nil
			}

			// Step 5: Git auto-commit
			commitMsg := fmt.Sprintf("feat(forge): %s", truncate(request, 50))
			commitHash, err := td.Git.AutoCommit(commitMsg)
			if err != nil {
				log.Printf("☠ Git commit error: %v", err)
				return &Response{
					Text:      explanation,
					Applied:   true,
					Committed: false,
					Agent:     "servitor",
					Error:     err.Error(),
				}, nil
			}

			log.Printf("✓ Committed: %s", commitHash[:8])
			return &Response{
				Text:      fmt.Sprintf("%s\n\n✓ Committed: %s", explanation, commitHash[:8]),
				Applied:   true,
				Committed: true,
				Agent:     "servitor",
			}, nil
		}
	}

	// No files to edit — just return Magos plan
	return &Response{
		Text:    plan,
		Applied: false,
		Agent:   "magos",
	}, nil
}

// Explore delegates to Skitarii for read-only exploration.
func (td *TaskDispatcher) Explore(filePath string) (string, error) {
	return td.Skitarii.Explore(filePath)
}

// Search delegates to Skitarii for grep search.
func (td *TaskDispatcher) Search(query string) ([]SearchResult, error) {
	return td.Skitarii.Search(query, td.Git.RepoPath())
}

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
