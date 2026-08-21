package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestDispatcher(t *testing.T) *TaskDispatcher {
	t.Helper()
	dir := t.TempDir()
	// Create a test file
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	return NewTaskDispatcher("http://127.0.0.1:11435", dir)
}

func TestTaskDispatcher_Handle_NoFiles(t *testing.T) {
	td := newTestDispatcher(t)

	resp, err := td.Handle("explain the codebase", nil)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp == nil {
		t.Fatal("Handle() returned nil response")
	}
	if resp.Agent != "magos" {
		t.Errorf("Expected agent 'magos', got %q", resp.Agent)
	}
	if resp.Applied {
		t.Error("Expected Applied=false when no files")
	}
}

func TestTaskDispatcher_Handle_WithFiles(t *testing.T) {
	td := newTestDispatcher(t)
	dir := td.RepoMap.AllSymbols()
	_ = dir // repo path is in TempDir

	// Get the test file path
	testFile := filepath.Join(td.Git.RepoPath(), "test.go")

	resp, err := td.Handle("add a comment", []string{testFile})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp == nil {
		t.Fatal("Handle() returned nil response")
	}
	// Agent should be servitor (applied) or magos (plan only)
	if resp.Agent == "" {
		t.Error("Agent field is empty")
	}
}

func TestTaskDispatcher_Search(t *testing.T) {
	td := newTestDispatcher(t)

	results, err := td.Search("main")
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) == 0 {
		t.Error("Search() returned no results")
	}
}

func TestTaskDispatcher_Search_NoMatch(t *testing.T) {
	td := newTestDispatcher(t)

	results, _ := td.Search("nonexistent_function_xyz_12345")
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestTaskDispatcher_Explore(t *testing.T) {
	td := newTestDispatcher(t)
	testFile := filepath.Join(td.Git.RepoPath(), "test.go")

	summary, err := td.Explore(testFile)
	if err != nil {
		t.Fatalf("Explore() error: %v", err)
	}
	if summary == "" {
		t.Error("Explore() returned empty summary")
	}
}

func TestTaskDispatcher_Explore_Nonexistent(t *testing.T) {
	td := newTestDispatcher(t)

	_, err := td.Explore("/nonexistent/file.go")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestNewTaskDispatcher(t *testing.T) {
	dir := t.TempDir()
	td := NewTaskDispatcher("http://127.0.0.1:11435", dir)
	if td == nil {
		t.Error("NewTaskDispatcher returned nil")
	}
	if td.Servitor == nil {
		t.Error("Servitor is nil")
	}
	if td.Skitarii == nil {
		t.Error("Skitarii is nil")
	}
	if td.RepoMap == nil {
		t.Error("RepoMap is nil")
	}
	if td.Git == nil {
		t.Error("Git is nil")
	}
}

func TestResponse_Struct(t *testing.T) {
	r := Response{
		Text:      "test",
		Applied:   true,
		Committed: true,
		Agent:     "servitor",
	}
	if r.Text != "test" || !r.Applied || !r.Committed || r.Agent != "servitor" {
		t.Error("Response struct fields incorrect")
	}
}
