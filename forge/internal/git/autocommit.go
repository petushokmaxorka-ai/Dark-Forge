// Package git — Git operations for Heretic Forge auto-commit.
package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// Commit represents a git commit.
type Commit struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
}

// GitOps provides git operations on a repository.
type GitOps struct {
	repoPath string
}

// RepoPath returns the repository root path.
func (g *GitOps) RepoPath() string {
	return g.repoPath
}

// NewGitOps creates a new GitOps for the given repo path.
func NewGitOps(repoPath string) *GitOps {
	return &GitOps{repoPath: repoPath}
}

// AutoCommit stages all changes and commits with the given message.
func (g *GitOps) AutoCommit(message string) (string, error) {
	if err := g.run("add", "-A"); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	if err := g.run("commit", "-m", message); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}
	hash, err := g.output("rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(hash), nil
}

// Undo resets the last commit (git reset --hard HEAD~1).
func (g *GitOps) Undo() error {
	return g.run("reset", "--hard", "HEAD~1")
}

// Diff returns the current unstaged diff.
func (g *GitOps) Diff() (string, error) {
	return g.output("diff")
}

// Status returns git status --short.
func (g *GitOps) Status() (string, error) {
	return g.output("status", "--short")
}

// Log returns the last n commits.
func (g *GitOps) Log(n int) ([]Commit, error) {
	out, err := g.output("log", "--oneline", fmt.Sprintf("-%d", n))
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		hash := parts[0]
		msg := ""
		if len(parts) > 1 {
			msg = parts[1]
		}
		commits = append(commits, Commit{Hash: hash, Message: msg})
	}
	return commits, nil
}

func (g *GitOps) run(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), string(out))
	}
	return nil
}

func (g *GitOps) output(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}
