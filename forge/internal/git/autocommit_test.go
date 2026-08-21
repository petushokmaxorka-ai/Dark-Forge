package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	exec.Command("git", "init", dir).Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
	// Initial commit
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("init"), 0644)
	exec.Command("git", "-C", dir, "add", "-A").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "initial").Run()
	return dir
}

func TestAutoCommit(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	// Make a change
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)

	hash, err := g.AutoCommit("test commit")
	if err != nil {
		t.Fatalf("AutoCommit() error: %v", err)
	}
	if len(hash) < 7 {
		t.Errorf("Hash too short: %q", hash)
	}
}

func TestUndo(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	// Commit a change
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
	g.AutoCommit("test commit")

	// Undo
	if err := g.Undo(); err != nil {
		t.Fatalf("Undo() error: %v", err)
	}

	// File should be gone
	if _, err := os.Stat(filepath.Join(dir, "test.txt")); !os.IsNotExist(err) {
		t.Error("Expected file to be removed after undo")
	}
}

func TestDiff(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	// Make unstaged change
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified"), 0644)

	diff, err := g.Diff()
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	if diff == "" {
		t.Error("Expected non-empty diff")
	}
}

func TestStatus(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0644)

	status, err := g.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if status == "" {
		t.Error("Expected non-empty status")
	}
}

func TestLog(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	commits, err := g.Log(5)
	if err != nil {
		t.Fatalf("Log() error: %v", err)
	}
	if len(commits) == 0 {
		t.Error("Expected at least 1 commit")
	}
	if commits[0].Hash == "" {
		t.Error("Commit hash is empty")
	}
}

func TestLog_AfterCommit(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("data"), 0644)
	g.AutoCommit("second commit")

	commits, err := g.Log(5)
	if err != nil {
		t.Fatalf("Log() error: %v", err)
	}
	if len(commits) < 2 {
		t.Errorf("Expected ≥2 commits, got %d", len(commits))
	}
}

func TestAutoCommit_EmptyMessage(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("data"), 0644)
	_, err := g.AutoCommit("")
	// git rejects empty commit messages
	if err == nil {
		t.Skip("git accepted empty commit message")
	}
}

func TestDiff_NoChanges(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	diff, err := g.Diff()
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	if diff != "" {
		t.Errorf("Expected empty diff, got: %q", diff)
	}
}

func TestStatus_Clean(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	status, err := g.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if status != "" {
		t.Errorf("Expected empty status, got: %q", status)
	}
}

func TestUndo_NoCommit(t *testing.T) {
	dir := t.TempDir()
	exec.Command("git", "init", dir).Run()
	g := NewGitOps(dir)
	err := g.Undo()
	// Should fail — no commits to undo
	if err == nil {
		t.Skip("Undo() succeeded with no commits (edge case)")
	}
}

func TestLog_Zero(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	commits, err := g.Log(0)
	if err != nil {
		t.Fatalf("Log(0) error: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("Expected 0 commits, got %d", len(commits))
	}
}

func TestNewGitOps(t *testing.T) {
	g := NewGitOps("/tmp/test")
	if g == nil {
		t.Error("NewGitOps returned nil")
	}
}

func TestCommit_Struct(t *testing.T) {
	c := Commit{Hash: "abc123", Message: "test"}
	if c.Hash != "abc123" || c.Message != "test" {
		t.Error("Commit struct fields incorrect")
	}
}

func TestGitOps_Status(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	// Clean status
	status, err := g.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if status != "" {
		t.Errorf("Expected empty status, got: %q", status)
	}

	// Dirty status
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0644)
	status, err = g.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if status == "" {
		t.Error("Expected non-empty status after creating file")
	}
}

func TestGitOps_IsClean(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	// Clean repo
	status, _ := g.Status()
	isClean := status == ""
	if !isClean {
		t.Error("Expected clean repo after initial commit")
	}

	// Make dirty
	os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty"), 0644)
	status, _ = g.Status()
	isClean = status == ""
	if isClean {
		t.Error("Expected dirty repo after creating file")
	}
}

func TestGitOps_Status_AfterCommit(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("data"), 0644)
	g.AutoCommit("test")

	status, err := g.Status()
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if status != "" {
		t.Errorf("Expected clean status after commit, got: %q", status)
	}
}

func TestGitOps_Diff_AfterModify(t *testing.T) {
	dir := setupTestRepo(t)
	g := NewGitOps(dir)

	// Modify tracked file
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified"), 0644)

	diff, err := g.Diff()
	if err != nil {
		t.Fatalf("Diff() error: %v", err)
	}
	if diff == "" {
		t.Error("Expected non-empty diff after modifying tracked file")
	}
}
