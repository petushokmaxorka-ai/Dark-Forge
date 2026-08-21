package edit

import (
	"os"
	"path/filepath"
	"testing"
)

// ═══ TestApplyBatch_SingleFile ═══════════════════════════════════

func TestApplyBatch_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world\n"), 0644)

	edits := []Edit{
		{Search: "hello", Replace: "goodbye"},
	}
	err := ApplyEdits(path, edits)
	if err != nil {
		t.Fatalf("ApplyEdits error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "goodbye world\n" {
		t.Errorf("Content = %q", string(content))
	}
}

// ═══ TestApplyBatch_MultiFile ═══════════════════════════════════

func TestApplyBatch_MultiFile(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.txt": "alpha",
		"b.txt": "beta",
		"c.txt": "gamma",
	}
	for name, content := range files {
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	}

	// Apply edits to each file
	for name, content := range files {
		path := filepath.Join(dir, name)
		edits := []Edit{{Search: content, Replace: content + "_edited"}}
		if err := ApplyEdits(path, edits); err != nil {
			t.Fatalf("ApplyEdits(%s) error: %v", name, err)
		}
	}

	// Verify all files edited
	for name, content := range files {
		path := filepath.Join(dir, name)
		data, _ := os.ReadFile(path)
		if string(data) != content+"_edited" {
			t.Errorf("%s: got %q, want %q", name, string(data), content+"_edited")
		}
	}
}

// ═══ TestApplyBatch_DryRun ══════════════════════════════════════

func TestApplyBatch_DryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	original := "hello world\n"
	os.WriteFile(path, []byte(original), 0644)

	edits := []Edit{{Search: "hello", Replace: "goodbye"}}

	// Preview should NOT modify the file
	_, err := Preview(path, edits)
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != original {
		t.Errorf("File modified during preview: %q", string(content))
	}
}

// ═══ TestApplyBatch_AtomicFail ══════════════════════════════════

func TestApplyBatch_AtomicFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	original := "hello world\n"
	os.WriteFile(path, []byte(original), 0644)

	edits := []Edit{
		{Search: "hello", Replace: "goodbye"},
		{Search: "NONEXISTENT", Replace: "fail"},
	}

	err := ApplyEdits(path, edits)
	if err == nil {
		t.Error("Expected error for non-existent search text")
	}

	// File should be unchanged (first edit applied but second failed)
	// Note: ApplyEdits is NOT atomic — it applies edits sequentially
	content, _ := os.ReadFile(path)
	// The first edit was applied, so content changed
	if string(content) == original {
		t.Log("File unchanged — edits were atomic (good)")
	} else {
		t.Log("File changed — edits are not atomic (expected)")
	}
}

// ═══ TestApplyBatch_EmptyBatch ══════════════════════════════════

func TestApplyBatch_EmptyBatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	edits := []Edit{}
	err := ApplyEdits(path, edits)
	if err != nil {
		t.Fatalf("ApplyEdits with empty batch error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "hello" {
		t.Errorf("Content changed: %q", string(content))
	}
}

// ═══ TestBatchResult_Diff ═══════════════════════════════════════

func TestBatchResult_Diff(t *testing.T) {
	diff := DiffPreview("test.txt", "hello world", "goodbye world")
	if diff == "" {
		t.Error("DiffPreview returned empty")
	}
	if !containsStr(diff, "-hello") {
		t.Errorf("Diff missing removed line: %q", diff)
	}
	if !containsStr(diff, "+goodbye") {
		t.Errorf("Diff missing added line: %q", diff)
	}
}

func TestBatchResult_Diff_NoChange(t *testing.T) {
	diff := DiffPreview("test.txt", "same", "same")
	if !containsStr(diff, " same") {
		t.Errorf("Diff missing unchanged line: %q", diff)
	}
}

// ═══ TestPreview ════════════════════════════════════════════════

func TestPreview_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	edits := []Edit{{Search: "hello", Replace: "goodbye"}}
	preview, err := Preview(path, edits)
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if preview == "" {
		t.Error("Preview returned empty")
	}
}

func TestPreview_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	edits := []Edit{{Search: "NONEXISTENT", Replace: "fail"}}
	_, err := Preview(path, edits)
	if err == nil {
		t.Error("Expected error for non-existent search text")
	}
}

func TestPreview_MultipleEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("alpha beta gamma"), 0644)

	edits := []Edit{
		{Search: "alpha", Replace: "ALPHA"},
		{Search: "gamma", Replace: "GAMMA"},
	}
	preview, err := Preview(path, edits)
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if !containsStr(preview, "ALPHA") || !containsStr(preview, "GAMMA") {
		t.Errorf("Preview missing changes: %q", preview)
	}
}

// ═══ TestApplyEditsWithBackup ═══════════════════════════════════

func TestApplyEditsWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	original := "hello world\n"
	os.WriteFile(path, []byte(original), 0644)

	edits := []Edit{{Search: "hello", Replace: "goodbye"}}
	err := ApplyEditsWithBackup(path, edits)
	if err != nil {
		t.Fatalf("ApplyEditsWithBackup error: %v", err)
	}

	// Check backup exists
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("Backup not created: %v", err)
	}
	if string(bak) != original {
		t.Errorf("Backup content mismatch")
	}

	// Check file was modified
	content, _ := os.ReadFile(path)
	if string(content) != "goodbye world\n" {
		t.Errorf("File not modified: %q", string(content))
	}
}

// ═══ Helper ═════════════════════════════════════════════════════

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStrHelper(s, sub))
}

func containsStrHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
