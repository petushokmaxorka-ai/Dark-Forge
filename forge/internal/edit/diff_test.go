package edit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDiffs(t *testing.T) {
	response := `Here is the fix:

<<<<<<< SEARCH
func old() {
	return "old"
}
=======
func new() {
	return "new"
}
>>>>>>> REPLACE
`
	edits, explanation := ParseDiffs(response)
	if len(edits) != 1 {
		t.Fatalf("Expected 1 edit, got %d", len(edits))
	}
	if edits[0].Search == "" {
		t.Error("Search is empty")
	}
	if edits[0].Replace == "" {
		t.Error("Replace is empty")
	}
	if explanation == "" {
		t.Error("Explanation is empty")
	}
}

func TestParseDiffs_Multiple(t *testing.T) {
	response := `Two changes:

<<<<<<< SEARCH
old1
=======
new1
>>>>>>> REPLACE

<<<<<<< SEARCH
old2
=======
new2
>>>>>>> REPLACE
`
	edits, _ := ParseDiffs(response)
	if len(edits) != 2 {
		t.Fatalf("Expected 2 edits, got %d", len(edits))
	}
}

func TestApplyEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world\nfoo bar\n"), 0644)

	edits := []Edit{
		{Search: "hello world", Replace: "goodbye world"},
	}
	if err := ApplyEdits(path, edits); err != nil {
		t.Fatalf("ApplyEdits() error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "goodbye world\nfoo bar\n" {
		t.Errorf("Unexpected content: %q", string(content))
	}
}

func TestApplyEdits_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world\n"), 0644)

	edits := []Edit{
		{Search: "nonexistent text", Replace: "replacement"},
	}
	err := ApplyEdits(path, edits)
	if err == nil {
		t.Error("Expected error for non-existent search text")
	}
}

func TestApplyEdits_Backup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	original := "original content\n"
	os.WriteFile(path, []byte(original), 0644)

	edits := []Edit{
		{Search: "original", Replace: "modified"},
	}
	if err := ApplyEditsWithBackup(path, edits); err != nil {
		t.Fatalf("ApplyEditsWithBackup() error: %v", err)
	}

	// Check backup exists
	bak, _ := os.ReadFile(path + ".bak")
	if string(bak) != original {
		t.Errorf("Backup content mismatch: %q", string(bak))
	}

	// Check file was modified
	content, _ := os.ReadFile(path)
	if string(content) != "modified content\n" {
		t.Errorf("File not modified: %q", string(content))
	}
}

func TestParseDiffs_EmptyResponse(t *testing.T) {
	edits, _ := ParseDiffs("no diffs here")
	if len(edits) != 0 {
		t.Errorf("Expected 0 edits, got %d", len(edits))
	}
}

func TestParseDiffs_PreservesIndentation(t *testing.T) {
	response := `<<<<<<< SEARCH
	if true {
		return nil
	}
=======
	if false {
		return fmt.Errorf("nope")
	}
>>>>>>> REPLACE
`
	edits, _ := ParseDiffs(response)
	if len(edits) != 1 {
		t.Fatalf("Expected 1 edit, got %d", len(edits))
	}
	// Check indentation preserved
	if edits[0].Search == "" || edits[0].Replace == "" {
		t.Error("Indentation not preserved")
	}
}

func TestApplyEdits_MultipleEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0644)

	edits := []Edit{
		{Search: "alpha", Replace: "ALPHA"},
		{Search: "gamma", Replace: "GAMMA"},
	}
	if err := ApplyEdits(path, edits); err != nil {
		t.Fatalf("ApplyEdits() error: %v", err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != "ALPHA\nbeta\nGAMMA\n" {
		t.Errorf("Unexpected content: %q", string(content))
	}
}

func TestApplyEdits_FileNotFound(t *testing.T) {
	edits := []Edit{{Search: "a", Replace: "b"}}
	err := ApplyEdits("/nonexistent/file.txt", edits)
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestApplyEditsWithBackup_FileNotFound(t *testing.T) {
	edits := []Edit{{Search: "a", Replace: "b"}}
	err := ApplyEditsWithBackup("/nonexistent/file.txt", edits)
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestParseDiffs_NoNewlines(t *testing.T) {
	response := `<<<<<<< SEARCH
old
=======
new
>>>>>>> REPLACE`
	edits, _ := ParseDiffs(response)
	if len(edits) != 1 {
		t.Fatalf("Expected 1 edit, got %d", len(edits))
	}
	if edits[0].Search == "" {
		t.Error("Search is empty")
	}
}

func TestApplyEdits_SameSearchTwice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("aaa bbb aaa\n"), 0644)

	edits := []Edit{
		{Search: "aaa", Replace: "AAA"},
	}
	if err := ApplyEdits(path, edits); err != nil {
		t.Fatalf("ApplyEdits() error: %v", err)
	}
	content, _ := os.ReadFile(path)
	// Should replace first occurrence only
	if string(content) != "AAA bbb aaa\n" {
		t.Errorf("Unexpected content: %q", string(content))
	}
}

func TestEdit_Struct(t *testing.T) {
	e := Edit{Search: "a", Replace: "b"}
	if e.Search != "a" || e.Replace != "b" {
		t.Error("Edit struct fields incorrect")
	}
}
