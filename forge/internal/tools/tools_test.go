package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// ═══ TestReadFile ═══════════════════════════════════════════════

func TestReadFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	result := ReadFile(path)
	if result.Error != "" {
		t.Errorf("Error: %s", result.Error)
	}
	if result.Content != "hello world" {
		t.Errorf("Content = %q, want 'hello world'", result.Content)
	}
	if result.Size != 11 {
		t.Errorf("Size = %d, want 11", result.Size)
	}
}

func TestReadFile_Missing(t *testing.T) {
	result := ReadFile("/nonexistent/file.txt")
	if result.Error == "" {
		t.Error("Expected error for missing file")
	}
}

func TestReadFile_Truncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	big := make([]byte, 60000)
	for i := range big {
		big[i] = 'x'
	}
	os.WriteFile(path, big, 0644)

	result := ReadFile(path)
	if result.Error != "" {
		t.Errorf("Error: %s", result.Error)
	}
	if len(result.Content) > 50100 {
		t.Errorf("Content not truncated: %d bytes", len(result.Content))
	}
}

// ═══ TestWriteFile ══════════════════════════════════════════════

func TestWriteFile_New(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	result := WriteFile(path, "hello")
	if result.Error != "" {
		t.Errorf("Error: %s", result.Error)
	}
	if result.Bytes != 5 {
		t.Errorf("Bytes = %d, want 5", result.Bytes)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "hello" {
		t.Errorf("File content = %q, want 'hello'", string(content))
	}
}

func TestWriteFile_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	os.WriteFile(path, []byte("old"), 0644)

	result := WriteFile(path, "new")
	if result.Error != "" {
		t.Errorf("Error: %s", result.Error)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "new" {
		t.Errorf("File content = %q, want 'new'", string(content))
	}
}

func TestWriteFile_MkdirP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "file.txt")

	result := WriteFile(path, "deep")
	if result.Error != "" {
		t.Errorf("Error: %s", result.Error)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "deep" {
		t.Errorf("File content = %q, want 'deep'", string(content))
	}
}

// ═══ TestRunBash ════════════════════════════════════════════════

func TestRunBash_Echo(t *testing.T) {
	result := RunBash("echo hello", "")
	if result.Error != "" {
		t.Errorf("Error: %s", result.Error)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want 'hello\\n'", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestRunBash_InvalidCmd(t *testing.T) {
	result := RunBash("nonexistent_command_xyz", "")
	if result.Error == "" {
		t.Error("Expected error for invalid command")
	}
}

func TestRunBash_ExitCode(t *testing.T) {
	result := RunBash("false", "")
	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
}

func TestRunBash_EmptyCommand(t *testing.T) {
	result := RunBash("", "")
	if result.Error == "" {
		t.Error("Expected error for empty command")
	}
}

// ═══ TestGrep ═══════════════════════════════════════════════════

func TestGrep_Found(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("func hello() {}"), 0644)
	os.WriteFile(filepath.Join(dir, "other.go"), []byte("func world() {}"), 0644)

	result := Grep("hello", dir, "")
	if result.Count != 1 {
		t.Errorf("Count = %d, want 1", result.Count)
	}
	if len(result.Files) != 1 {
		t.Errorf("Files length = %d, want 1", len(result.Files))
	}
}

func TestGrep_NotFound(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("func hello() {}"), 0644)

	result := Grep("nonexistent", dir, "")
	if result.Count != 0 {
		t.Errorf("Count = %d, want 0", result.Count)
	}
}

func TestGrep_Extension(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "test.py"), []byte("hello"), 0644)

	result := Grep("hello", dir, "go")
	if result.Count != 1 {
		t.Errorf("Count = %d, want 1 (filtered by .go)", result.Count)
	}
}

// ═══ TestGlob ═══════════════════════════════════════════════════

func TestGlob_Match(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "c.py"), []byte(""), 0644)

	result := Glob("*.go", dir)
	if len(result) != 2 {
		t.Errorf("Glob(*.go) returned %d files, want 2", len(result))
	}
}

func TestGlob_NoMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0644)

	result := Glob("*.py", dir)
	if len(result) != 0 {
		t.Errorf("Glob(*.py) returned %d files, want 0", len(result))
	}
}

// ═══ TestReadFile_PermissionDenied ══════════════════════════════

func TestReadFile_PermissionDenied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "noread.txt")
	os.WriteFile(path, []byte("secret"), 0000)

	result := ReadFile(path)
	if result.Error == "" {
		t.Error("Expected error for permission denied")
	}
}

// ═══ TestWriteFile_ReadOnlyDir ══════════════════════════════════

func TestWriteFile_ReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	roDir := filepath.Join(dir, "readonly")
	os.Mkdir(roDir, 0555)

	result := WriteFile(filepath.Join(roDir, "file.txt"), "data")
	if result.Error == "" {
		t.Error("Expected error for read-only directory")
	}
}

// ═══ TestRunBash_Timeout ════════════════════════════════════════

func TestRunBash_Timeout(t *testing.T) {
	// sleep 1 should complete quickly
	result := RunBash("sleep 0.1", "")
	if result.Error != "" && result.ExitCode != 0 {
		t.Errorf("sleep 0.1 should succeed: %s", result.Error)
	}
}

// ═══ TestGrep_InvalidRegex ══════════════════════════════════════

func TestGrep_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)

	// Grep uses strings.Contains, not regex — so invalid "regex" still works
	result := Grep("[invalid", dir, "")
	// Should not crash
	if result.Error != "" {
		t.Errorf("Grep should not return error for pattern: %s", result.Error)
	}
}

// ═══ TestGlob_InvalidPattern ════════════════════════════════════

func TestGlob_InvalidPattern(t *testing.T) {
	dir := t.TempDir()
	result := Glob("[invalid", dir)
	// Should return nil or empty, not crash
	if result != nil && len(result) > 0 {
		t.Error("Invalid pattern should return empty results")
	}
}

// ═══ TestReadFile_LargeFile ═════════════════════════════════════

func TestReadFile_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.txt")
	data := make([]byte, 60000)
	for i := range data {
		data[i] = 'A'
	}
	os.WriteFile(path, data, 0644)

	result := ReadFile(path)
	if result.Error != "" {
		t.Errorf("Error: %s", result.Error)
	}
	if len(result.Content) > 50100 {
		t.Errorf("Content not truncated: %d bytes", len(result.Content))
	}
	if result.Size != 60000 {
		t.Errorf("Size = %d, want 60000", result.Size)
	}
}

// ═══ TestWriteFile_Unicode ══════════════════════════════════════

func TestWriteFile_Unicode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unicode.txt")

	result := WriteFile(path, "Привет мир! ☉⚒⚙")
	if result.Error != "" {
		t.Errorf("Error: %s", result.Error)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "Привет мир! ☉⚒⚙" {
		t.Errorf("Unicode content mismatch: %q", string(content))
	}
}

// ═══ TestRunBash_WithWorkDir ════════════════════════════════════

func TestRunBash_WithWorkDir(t *testing.T) {
	dir := t.TempDir()
	result := RunBash("pwd", dir)
	if result.Error != "" {
		t.Errorf("Error: %s", result.Error)
	}
	// stdout should contain the workdir path
	if len(result.Stdout) == 0 {
		t.Error("Expected pwd output")
	}
}
