// Package agent — Heretic Forge AI agents (Servitor, Skitarii, Magos).
// Dark Mechanicus style. Per Ignis, per Ferrum, per Codicem.
package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	edit "github.com/petushokmaxorka-ai/dark-forge/forge/internal/edit"
)

// ═══════════════════════════════════════════════════════════════════════
// SERVITOR — 8B Editor Agent (Ferrum Flagellum)
// ═══════════════════════════════════════════════════════════════════════

// Servitor is the 8B editor agent that applies code changes.
// Magos (Vox Dei memory/general) plans. Servitor (8B) executes.
type Servitor struct {
	modelURL string // http://127.0.0.1:11435 (Symbiont-Bridge)
	modelID  string // "Ferrum Flagellum" (8B Llama)
}

// NewServitor creates a new Servitor agent.
func NewServitor(url, model string) *Servitor {
	return &Servitor{modelURL: url, modelID: model}
}

// ApplyPlan applies a plan to a file by generating SEARCH/REPLACE blocks via 8B model.
func (s *Servitor) ApplyPlan(plan string, filePath string) (string, error) {
	// If the plan already contains SEARCH/REPLACE blocks, apply directly (offline mode).
	edits, explanation := edit.ParseDiffs(plan)
	if len(edits) > 0 {
		if err := edit.ApplyEditsWithBackup(filePath, edits); err != nil {
			return "", fmt.Errorf("apply edits: %w", err)
		}
		return explanation, nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	// Build prompt for 8B model
	prompt := fmt.Sprintf(`Вот файл %s:
%s

Вот план изменений:
%s

Сгенерируй SEARCH/REPLACE блоки в формате:
<<<<<<< SEARCH
old code here
=======
new code here
>>>>>>> REPLACE

Только блоки. Без объяснений.`, filePath, string(content), plan)

	// Call 8B model via Symbiont-Bridge
	response, err := s.callModel(prompt)
	if err != nil {
		return "", fmt.Errorf("model call: %w", err)
	}

	// Parse diffs from model response
	edits, explanation = edit.ParseDiffs(response)
	if len(edits) == 0 {
		return "", fmt.Errorf("no edits found in model response")
	}

	// Apply edits with backup
	if err := edit.ApplyEditsWithBackup(filePath, edits); err != nil {
		return "", fmt.Errorf("apply edits: %w", err)
	}

	return explanation, nil
}

// callModel calls the 8B model via Symbiont-Bridge OpenAI-compatible API.
func (s *Servitor) callModel(prompt string) (string, error) {
	payload := map[string]interface{}{
		"model":       s.modelID,
		"prompt":      prompt,
		"max_tokens":  2000,
		"temperature": 0.3,
		"top_p":       0.9,
		"stop":        []string{"<<<<<<< SEARCH"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(s.modelURL+"/v1/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body2, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body2, &result); err != nil {
		return string(body2), nil
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	choice := choices[0].(map[string]interface{})
	text, _ := choice["text"].(string)
	return strings.TrimSpace(text), nil
}

// Lint checks a file for syntax errors.
func (s *Servitor) Lint(filePath string) (bool, string) {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".go":
		dir := filepath.Dir(filePath)
		cmd := exec.Command("go", "vet", "./...")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return false, string(out)
		}
		return true, ""
	case ".py":
		cmd := exec.Command("python3", "-m", "py_compile", filePath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return false, string(out)
		}
		return true, ""
	case ".ts":
		cmd := exec.Command("tsc", "--noEmit", filePath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return false, string(out)
		}
		return true, ""
	default:
		return true, "" // unknown type — assume OK
	}
}

// ═══════════════════════════════════════════════════════════════════════
// SKITARII — 1.5B Explorer Agent (Vox Minor)
// ═══════════════════════════════════════════════════════════════════════

// Skitarii is the 1.5B read-only explorer agent.
// Fast recon. Read-only. No writes.
type Skitarii struct {
	modelURL string // http://127.0.0.1:11436 (или 11435)
	modelID  string // "Vox Minor" (1.5B)
}

// NewSkitarii creates a new Skitarii agent.
func NewSkitarii(url, model string) *Skitarii {
	return &Skitarii{modelURL: url, modelID: model}
}

// Explore reads a file and returns a summary via 1.5B model.
func (s *Skitarii) Explore(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	// Truncate to ~4000 tokens (~16000 chars)
	text := string(content)
	if len(text) > 16000 {
		text = text[:16000] + "\n... [truncated]"
	}

	// Build prompt for 1.5B model
	prompt := fmt.Sprintf(`Вот файл %s:
%s

Кратко опиши:
1. Основные функции/классы
2. Возможные баги
3. Структура

Ответ: 200-500 токенов. По-русски.`, filePath, text)

	// Call 1.5B model
	response, err := s.callModel(prompt)
	if err != nil {
		// Fallback: return basic info
		return fmt.Sprintf("File: %s (%d bytes, %d lines)",
			filepath.Base(filePath), len(content), strings.Count(text, "\n")), nil
	}

	return response, nil
}

// callModel calls the 1.5B model via Symbiont-Bridge.
func (s *Skitarii) callModel(prompt string) (string, error) {
	payload := map[string]interface{}{
		"model":       s.modelID,
		"prompt":      prompt,
		"max_tokens":  500,
		"temperature": 0.5,
		"top_p":       0.9,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(s.modelURL+"/v1/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body2, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body2, &result); err != nil {
		return string(body2), nil
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	choice := choices[0].(map[string]interface{})
	text, _ := choice["text"].(string)
	return strings.TrimSpace(text), nil
}

// SearchResult represents a grep match.
type SearchResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Context string `json:"context"`
}

// Search performs a grep-like search across source files.
func (s *Skitarii) Search(query string, repoPath string) ([]SearchResult, error) {
	cmd := exec.Command("grep", "-rn", "--include=*.go", "--include=*.py", "--include=*.ts",
		query, repoPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil // no matches
	}
	var results []SearchResult
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) >= 3 {
			lineNum, _ := strconv.Atoi(parts[1])
			results = append(results, SearchResult{
				File:    parts[0],
				Line:    lineNum,
				Context: parts[2],
			})
		}
		if len(results) >= 10 {
			break
		}
	}
	return results, nil
}
