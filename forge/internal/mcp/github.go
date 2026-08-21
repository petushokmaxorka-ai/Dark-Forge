// Package mcp — GitHub REST API client.
// Real implementation (no external deps, uses net/http).
// Falls back to stub when GITHUB_TOKEN is unset.
package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// GitHubTool is a real GitHub REST API client.
// Auth via GITHUB_TOKEN env var (Personal Access Token or GitHub App token).
// If GITHUB_TOKEN is unset, the tool operates in STUB mode (returns _stub:true).
type GitHubTool struct {
	defaultRepo string // "owner/name"
	token       string // from $GITHUB_TOKEN; empty → stub mode
	apiBase     string // override for GitHub Enterprise
	httpClient  *http.Client
}

// NewGitHubTool creates a GitHub tool. Token is read from $GITHUB_TOKEN.
func NewGitHubTool(defaultRepo string) *GitHubTool {
	apiBase := os.Getenv("GITHUB_API_BASE")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	return &GitHubTool{
		defaultRepo: defaultRepo,
		token:       os.Getenv("GITHUB_TOKEN"),
		apiBase:     apiBase,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// NewGitHubToolWithToken creates a GitHub tool with an explicit token (useful for tests).
func NewGitHubToolWithToken(defaultRepo, token, apiBase string) *GitHubTool {
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	return &GitHubTool{
		defaultRepo: defaultRepo,
		token:       token,
		apiBase:     apiBase,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Name implements McpTool.
func (g *GitHubTool) Name() string { return "github" }

// Description implements McpTool.
func (g *GitHubTool) Description() string {
	if g.token != "" {
		return "GitHub REST API: list_issues, get_issue, create_pr (LIVE — using GITHUB_TOKEN)"
	}
	return "GitHub REST API: list_issues, get_issue, create_pr (STUB — set GITHUB_TOKEN env to enable)"
}

func (g *GitHubTool) Schema() string {
	return `{"type":"object","properties":{"action":{"type":"string","enum":["list_issues","get_issue","create_pr"]},"repo":{"type":"string"},"number":{"type":"integer"},"title":{"type":"string"},"body":{"type":"string"}},"required":["action"]}`
}

// IsStub reports whether the tool is in stub mode (no token).
func (g *GitHubTool) IsStub() bool { return g.token == "" }

type githubParams struct {
	Action string `json:"action"`
	Repo   string `json:"repo,omitempty"`
	Number int    `json:"number,omitempty"`
	Title  string `json:"title,omitempty"`
	Body   string `json:"body,omitempty"`
}

// Execute implements McpTool.
func (g *GitHubTool) Execute(params json.RawMessage) (interface{}, error) {
	var p githubParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("github: invalid params: %w", err)
	}
	if p.Repo == "" {
		p.Repo = g.defaultRepo
	}
	if p.Repo == "" {
		return nil, fmt.Errorf("github: repo is required (set default or pass repo param)")
	}

	// Stub fallback when no token configured.
	if g.IsStub() {
		return g.stubExecute(p)
	}

	switch p.Action {
	case "list_issues":
		return g.listIssues(p.Repo)
	case "get_issue":
		if p.Number == 0 {
			return nil, fmt.Errorf("github.get_issue: number is required")
		}
		return g.getIssue(p.Repo, p.Number)
	case "create_pr":
		if p.Title == "" {
			return nil, fmt.Errorf("github.create_pr: title is required")
		}
		return g.createPR(p.Repo, p.Title, p.Body)
	default:
		return nil, fmt.Errorf("github: unknown action %q (use list_issues, get_issue, create_pr)", p.Action)
	}
}

// ─────────── STUB implementation ───────────

func (g *GitHubTool) stubExecute(p githubParams) (interface{}, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	switch p.Action {
	case "list_issues":
		return map[string]interface{}{
			"repo":    p.Repo,
			"count":   0,
			"issues":  []interface{}{},
			"_stub":   true,
			"_note":   "set GITHUB_TOKEN env var to call real GitHub API",
			"stub_at": now,
		}, nil
	case "get_issue":
		if p.Number == 0 {
			return nil, fmt.Errorf("github.get_issue: number is required")
		}
		return map[string]interface{}{
			"repo":    p.Repo,
			"number":  p.Number,
			"title":   "",
			"body":    "",
			"state":   "unknown",
			"_stub":   true,
			"stub_at": now,
		}, nil
	case "create_pr":
		if p.Title == "" {
			return nil, fmt.Errorf("github.create_pr: title is required")
		}
		return map[string]interface{}{
			"repo":    p.Repo,
			"title":   p.Title,
			"body":    p.Body,
			"url":     "",
			"_stub":   true,
			"stub_at": now,
		}, nil
	default:
		return nil, fmt.Errorf("github: unknown action %q (use list_issues, get_issue, create_pr)", p.Action)
	}
}

// ─────────── REAL GitHub REST client ───────────

func (g *GitHubTool) authHeaders() map[string]string {
	h := map[string]string{
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
		"User-Agent":           "heretic-forge-mcp/1.2",
	}
	if g.token != "" {
		h["Authorization"] = "Bearer " + g.token
	}
	return h
}

func (g *GitHubTool) doRequest(method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("github: marshal body: %w", err)
		}
		reqBody = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, g.apiBase+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("github: new request: %w", err)
	}
	for k, v := range g.authHeaders() {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("github: http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("github: read body: %w", err)
	}
	return raw, resp.StatusCode, nil
}

// splitRepo returns owner + name from "owner/name" or fails.
func splitRepo(repo string) (string, string, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("github: invalid repo %q (want owner/name)", repo)
	}
	return parts[0], parts[1], nil
}

// listIssues returns up to 30 open issues.
func (g *GitHubTool) listIssues(repo string) (interface{}, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	raw, status, err := g.doRequest("GET", "/repos/"+owner+"/"+name+"/issues?state=open&per_page=30", nil)
	if err != nil {
		return nil, err
	}
	if status == 401 {
		return nil, fmt.Errorf("github: 401 unauthorized (check GITHUB_TOKEN validity)")
	}
	if status == 404 {
		return nil, fmt.Errorf("github: 404 repo %q not found", repo)
	}
	if status >= 400 {
		return nil, fmt.Errorf("github: list_issues %d: %s", status, truncate(string(raw), 200))
	}
	var issues []map[string]interface{}
	if err := json.Unmarshal(raw, &issues); err != nil {
		return nil, fmt.Errorf("github: decode issues: %w", err)
	}
	// Filter out pull requests (GitHub returns PRs in /issues endpoint).
	filtered := make([]map[string]interface{}, 0, len(issues))
	for _, iss := range issues {
		if _, isPR := iss["pull_request"]; !isPR {
			filtered = append(filtered, iss)
		}
	}
	return map[string]interface{}{
		"repo":   repo,
		"count":  len(filtered),
		"issues": filtered,
	}, nil
}

// getIssue fetches a single issue by number.
func (g *GitHubTool) getIssue(repo string, number int) (interface{}, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	raw, status, err := g.doRequest("GET", fmt.Sprintf("/repos/%s/%s/issues/%d", owner, name, number), nil)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, fmt.Errorf("github: 404 issue #%d not found in %s", number, repo)
	}
	if status >= 400 {
		return nil, fmt.Errorf("github: get_issue %d: %s", status, truncate(string(raw), 200))
	}
	var issue map[string]interface{}
	if err := json.Unmarshal(raw, &issue); err != nil {
		return nil, fmt.Errorf("github: decode issue: %w", err)
	}
	return issue, nil
}

// createPR creates a new pull request. Requires base + head branches.
// NOTE: GitHub API needs `head`, `base`, `title`, `body`. The MCP interface
// only accepts title + body for now. Caller must set defaults in `body` JSON
// or future iteration can add head/base fields.
func (g *GitHubTool) createPR(repo, title, body string) (interface{}, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	payload := map[string]interface{}{
		"title": title,
		"body":  body,
		"head":  "feature", // default; caller can override in body JSON
		"base":  "main",    // default; caller can override in body JSON
	}
	// Allow advanced callers to encode head/base in body JSON as hints.
	// (Body field is the issue body, but we also peek for __head/__base markers.)
	if strings.Contains(body, "__head:") {
		// best-effort override; production would extend params
		payload["head"] = extractHint(body, "__head:")
	}
	if strings.Contains(body, "__base:") {
		payload["base"] = extractHint(body, "__base:")
	}

	raw, status, err := g.doRequest("POST", "/repos/"+owner+"/"+name+"/pulls", payload)
	if err != nil {
		return nil, err
	}
	if status == 422 {
		return nil, fmt.Errorf("github: 422 PR validation failed: %s", truncate(string(raw), 300))
	}
	if status >= 400 {
		return nil, fmt.Errorf("github: create_pr %d: %s", status, truncate(string(raw), 200))
	}
	var pr map[string]interface{}
	if err := json.Unmarshal(raw, &pr); err != nil {
		return nil, fmt.Errorf("github: decode pr: %w", err)
	}
	return pr, nil
}

// extractHint pulls a "hint" value from a body string (format: __key:value).
func extractHint(body, prefix string) string {
	idx := strings.Index(body, prefix)
	if idx < 0 {
		return ""
	}
	rest := body[idx+len(prefix):]
	if nl := strings.Index(rest, "\n"); nl >= 0 {
		rest = rest[:nl]
	}
	return strings.TrimSpace(rest)
}

// truncate clamps a string to n bytes for error messages.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}