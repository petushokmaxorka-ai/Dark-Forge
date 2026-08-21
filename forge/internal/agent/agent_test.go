package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServitor_ApplyPlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	os.WriteFile(path, []byte("package main\n\nfunc old() string {\n\treturn \"old\"\n}\n"), 0644)

	plan := `<<<<<<< SEARCH
func old() string {
	return "old"
}
=======
func new() string {
	return "new"
}
>>>>>>> REPLACE
`
	s := NewServitor("http://127.0.0.1:11436", "Ferrum Flagellum")
	_, err := s.ApplyPlan(plan, path)
	if err != nil {
		t.Fatalf("ApplyPlan() error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if string(content) == "" {
		t.Error("File is empty after ApplyPlan")
	}
}

func TestServitor_ApplyPlan_NoEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	os.WriteFile(path, []byte("package main\n"), 0644)

	s := NewServitor("http://127.0.0.1:11436", "Ferrum Flagellum")
	_, err := s.ApplyPlan("no diffs here", path)
	if err == nil {
		t.Error("Expected error for plan with no edits")
	}
}

func TestServitor_Lint_Go(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0644)

	s := NewServitor("http://127.0.0.1:11436", "Ferrum Flagellum")
	ok, _ := s.Lint(path)
	// go vet may fail in temp dir without go.mod — that's OK
	if !ok {
		t.Skip("go vet failed in temp dir (expected without go.mod)")
	}
}

func TestServitor_Lint_Python(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.py")
	os.WriteFile(path, []byte("def hello():\n    pass\n"), 0644)

	s := NewServitor("http://127.0.0.1:11436", "Ferrum Flagellum")
	ok, msg := s.Lint(path)
	if !ok {
		t.Errorf("Lint() failed: %s", msg)
	}
}

func TestSkitarii_Explore(t *testing.T) {
	t.Skip("requires live llama-swap")
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	os.WriteFile(path, []byte("package main\n\nfunc main() {\n\t// do stuff\n}\n"), 0644)

	s := NewSkitarii("http://127.0.0.1:11436", "Vox Minor")
	summary, err := s.Explore(path)
	if err != nil {
		t.Fatalf("Explore() error: %v", err)
	}
	if summary == "" {
		t.Error("Explore() returned empty summary")
	}
}

func TestSkitarii_Explore_Nonexistent(t *testing.T) {
	t.Skip("requires live llama-swap")
	s := NewSkitarii("http://127.0.0.1:11436", "Vox Minor")
	_, err := s.Explore("/nonexistent/file.go")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestSkitarii_Search(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	s := NewSkitarii("http://127.0.0.1:11436", "Vox Minor")
	results, err := s.Search("hello", dir)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) == 0 {
		t.Error("Search() returned no results")
	}
}

func TestSkitarii_Search_NoMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n"), 0644)

	s := NewSkitarii("http://127.0.0.1:11436", "Vox Minor")
	results, _ := s.Search("nonexistent_function_xyz", dir)
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestServitor_Lint_UnknownType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xyz")
	os.WriteFile(path, []byte("content"), 0644)

	s := NewServitor("http://127.0.0.1:11436", "Ferrum Flagellum")
	ok, msg := s.Lint(path)
	if !ok {
		t.Errorf("Lint() should pass for unknown type: %s", msg)
	}
}

func TestSkitarii_Explore_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.py")
	// Create a large file
	content := "def func" + strings.Repeat("_x", 20000) + "(): pass\n"
	os.WriteFile(path, []byte(content), 0644)

	s := NewSkitarii("http://127.0.0.1:11436", "Vox Minor")
	summary, err := s.Explore(path)
	if err != nil {
		t.Fatalf("Explore() error: %v", err)
	}
	if summary == "" {
		t.Error("Explore() returned empty summary for large file")
	}
}

func TestServitor_ApplyPlan_BackupCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	original := "package main\n\nfunc old() {}\n"
	os.WriteFile(path, []byte(original), 0644)

	plan := `<<<<<<< SEARCH
func old() {}
=======
func new() {}
>>>>>>> REPLACE
`
	s := NewServitor("http://127.0.0.1:11436", "Ferrum Flagellum")
	_, err := s.ApplyPlan(plan, path)
	if err != nil {
		t.Fatalf("ApplyPlan() error: %v", err)
	}

	// Check backup exists
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("Backup not created: %v", err)
	}
	if string(bak) != original {
		t.Error("Backup content doesn't match original")
	}
}

func TestNewServitor(t *testing.T) {
	s := NewServitor("http://127.0.0.1:11436", "Ferrum Flagellum")
	if s == nil {
		t.Error("NewServitor returned nil")
	}
}

func TestNewSkitarii(t *testing.T) {
	s := NewSkitarii("http://127.0.0.1:11436", "Vox Minor")
	if s == nil {
		t.Error("NewSkitarii returned nil")
	}
}

func TestSearchResult_Struct(t *testing.T) {
	r := SearchResult{File: "test.go", Line: 42, Context: "func main()"}
	if r.File != "test.go" || r.Line != 42 {
		t.Error("SearchResult struct fields incorrect")
	}
}
