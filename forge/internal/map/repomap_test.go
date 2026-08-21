package repomap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoMap_Build(t *testing.T) {
	dir := t.TempDir()
	// Create a test Go file
	content := `package main

func hello() string {
	return "world"
}

type Config struct {
	Host string
	Port int
}
`
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte(content), 0644)

	rm := NewRepoMap(dir)
	if err := rm.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	syms := rm.AllSymbols()
	if len(syms) == 0 {
		t.Fatal("Expected symbols, got none")
	}
	fileSyms := syms[path]
	if len(fileSyms) < 2 {
		t.Errorf("Expected ≥2 symbols, got %d", len(fileSyms))
	}
}

func TestRepoMap_Render(t *testing.T) {
	dir := t.TempDir()
	content := `package main

func main() {}
func helper() {}

type Server struct{}
`
	path := filepath.Join(dir, "app.go")
	os.WriteFile(path, []byte(content), 0644)

	rm := NewRepoMap(dir)
	rm.Build()
	result := rm.Render(512)
	if result == "" {
		t.Fatal("Render() returned empty string")
	}
	if len(result) < 10 {
		t.Errorf("Render() too short: %q", result)
	}
}

func TestRepoMap_PageRank(t *testing.T) {
	dir := t.TempDir()
	// File with many symbols should rank higher
	content1 := `package main

func a() {}
func b() {}
func c() {}
func d() {}
func e() {}
`
	content2 := `package util

func single() {}
`
	os.WriteFile(filepath.Join(dir, "big.go"), []byte(content1), 0644)
	os.WriteFile(filepath.Join(dir, "small.go"), []byte(content2), 0644)

	rm := NewRepoMap(dir)
	rm.Build()
	result := rm.Render(512)
	// big.go should appear before small.go (more symbols = higher rank)
	bigIdx := -1
	smallIdx := -1
	for i, line := range splitLines(result) {
		if contains(line, "big.go") && bigIdx < 0 {
			bigIdx = i
		}
		if contains(line, "small.go") && smallIdx < 0 {
			smallIdx = i
		}
	}
	if bigIdx >= 0 && smallIdx >= 0 && bigIdx > smallIdx {
		t.Errorf("big.go should rank before small.go: big=%d small=%d", bigIdx, smallIdx)
	}
}

func TestRepoMap_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	rm := NewRepoMap(dir)
	if err := rm.Build(); err != nil {
		t.Fatalf("Build() on empty dir: %v", err)
	}
	if len(rm.AllSymbols()) != 0 {
		t.Errorf("Expected 0 symbols, got %d", len(rm.AllSymbols()))
	}
}

func TestRepoMap_PythonSymbols(t *testing.T) {
	dir := t.TempDir()
	content := `def hello():
    pass

def world():
    pass
`
	os.WriteFile(filepath.Join(dir, "test.py"), []byte(content), 0644)

	rm := NewRepoMap(dir)
	rm.Build()
	syms := rm.AllSymbols()
	if len(syms) == 0 {
		t.Fatal("Expected Python symbols")
	}
}

func TestRepoMap_MixedFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "util.py"), []byte("def helper():\n    pass\n"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Project\n"), 0644)

	rm := NewRepoMap(dir)
	rm.Build()
	syms := rm.AllSymbols()
	if len(syms) < 2 {
		t.Errorf("Expected symbols from multiple files, got %d", len(syms))
	}
}

func TestRepoMap_SymbolsMethod(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	os.WriteFile(path, []byte("package main\nfunc foo() {}\ntype Bar struct{}\n"), 0644)

	rm := NewRepoMap(dir)
	rm.Build()
	syms := rm.Symbols(path)
	if len(syms) == 0 {
		t.Error("Symbols() returned empty for file with symbols")
	}
}

func TestRepoMap_RenderTokenBudget(t *testing.T) {
	dir := t.TempDir()
	// Create many symbols
	var content string
	for i := 0; i < 20; i++ {
		content += "func func" + string(rune('A'+i)) + "() {}\n"
	}
	os.WriteFile(filepath.Join(dir, "many.go"), []byte("package main\n"+content), 0644)

	rm := NewRepoMap(dir)
	rm.Build()
	// Small token budget should truncate
	short := rm.Render(10)
	long := rm.Render(1000)
	if len(short) > len(long) {
		t.Error("Small budget should produce shorter output")
	}
}

func TestRepoMap_SymbolTypes(t *testing.T) {
	dir := t.TempDir()
	content := `package main

func Function() {}
type MyStruct struct {}
type MyInterface interface {}
const Constant = 42
var Variable = "test"
`
	os.WriteFile(filepath.Join(dir, "types.go"), []byte(content), 0644)

	rm := NewRepoMap(dir)
	rm.Build()
	syms := rm.AllSymbols()
	if len(syms) == 0 {
		t.Fatal("Expected symbols")
	}
	// Check we found at least function and struct
	found := false
	for _, s := range syms[filepath.Join(dir, "types.go")] {
		if s.Type == "function" || s.Type == "struct" {
			found = true
		}
	}
	if !found {
		t.Error("Expected function or struct symbols")
	}
}

func TestRepoMap_NestedDirectories(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "pkg", "sub")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "deep.go"), []byte("package sub\nfunc Deep() {}\n"), 0644)

	rm := NewRepoMap(dir)
	rm.Build()
	syms := rm.AllSymbols()
	if len(syms) == 0 {
		t.Error("Expected symbols from nested directory")
	}
}

func splitLines(s string) []string {
	result := make([]string, 0)
	current := ""
	for _, c := range s {
		if c == '\n' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
