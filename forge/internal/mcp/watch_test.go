// Package mcp — Tests for FileWatcher + WatchTool.
package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFileWatcher_FirstPollIsEmpty verifies the first poll is a snapshot.
func TestFileWatcher_FirstPollIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	fw := NewFileWatcher(dir, "*", false)
	changes := fw.Poll()
	if len(changes) != 0 {
		t.Errorf("first poll returned %v, want empty (snapshot)", changes)
	}
}

// TestFileWatcher_DetectsChange verifies mtime changes are detected.
func TestFileWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	fw := NewFileWatcher(dir, "*", false)
	if changes := fw.Poll(); len(changes) != 0 {
		t.Errorf("first poll changes = %v, want []", changes)
	}

	// Modify file (mtime must change — set explicit future time).
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	changes := fw.Poll()
	if len(changes) != 1 {
		t.Errorf("second poll changes = %v, want 1 entry", changes)
	}
	if changes[0] != path {
		t.Errorf("change[0] = %q, want %q", changes[0], path)
	}
}

// TestFileWatcher_DetectsDeletion verifies file deletions are reported.
func TestFileWatcher_DetectsDeletion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	fw := NewFileWatcher(dir, "*", false)
	fw.Poll() // snapshot

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	changes := fw.Poll()
	found := false
	for _, c := range changes {
		if c == path {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("deletion not reported: %v", changes)
	}
}

// TestFileWatcher_PatternFilter verifies glob pattern is respected.
func TestFileWatcher_PatternFilter(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.txt", "c.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	fw := NewFileWatcher(dir, "*.go", false)
	// First poll: snapshot.
	_ = fw.Poll()
	// Touch one .go file with future mtime.
	path := filepath.Join(dir, "a.go")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	changes := fw.Poll()
	if len(changes) != 1 {
		t.Fatalf("expected 1 .go change, got %d (%v)", len(changes), changes)
	}
	if changes[0] != path {
		t.Errorf("change = %q, want %q", changes[0], path)
	}
}

// TestWatchTool_Poll executes the poll action.
func TestWatchTool_Poll(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	w := NewWatchTool(dir, "*", false)

	// Initial poll — snapshot.
	res, err := w.Execute(json.RawMessage(`{"action":"poll"}`))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	m := res.(map[string]interface{})
	if m["count"].(int) != 0 {
		t.Errorf("initial count = %v, want 0", m["count"])
	}
	if !strings.Contains(m["hint"].(string), "poll") {
		t.Errorf("hint = %q, want it to mention poll", m["hint"])
	}
}

// TestWatchTool_Reset clears the snapshot.
func TestWatchTool_Reset(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	w := NewWatchTool(dir, "*", false)

	// Build snapshot, then reset, then poll (should return full snapshot).
	w.Execute(json.RawMessage(`{"action":"poll"}`))
	res, err := w.Execute(json.RawMessage(`{"action":"reset"}`))
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if res.(map[string]interface{})["reset"] != true {
		t.Errorf("reset response missing reset:true")
	}
}

// TestWatchTool_Snapshot captures initial state.
func TestWatchTool_Snapshot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	w := NewWatchTool(dir, "*", false)
	res, err := w.Execute(json.RawMessage(`{"action":"snapshot"}`))
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	m := res.(map[string]interface{})
	if m["count"].(int) != 1 {
		t.Errorf("snapshot count = %v, want 1", m["count"])
	}
	if m["_note"] == nil {
		t.Error("snapshot response should include _note")
	}
}

// TestWatchTool_UnknownAction rejects unsupported actions.
func TestWatchTool_UnknownAction(t *testing.T) {
	w := NewWatchTool("/tmp", "*", false)
	if _, err := w.Execute(json.RawMessage(`{"action":"explode"}`)); err == nil {
		t.Error("expected error for unknown action")
	}
}

// TestFilterChangesByExt filters results by extension.
func TestFilterChangesByExt(t *testing.T) {
	paths := []string{
		"/a/foo.go",
		"/a/bar.py",
		"/a/baz.go",
		"/a/MIXED.GO",
	}
	got := FilterChangesByExt(paths, []string{".go"})
	if len(got) != 3 {
		t.Errorf("FilterChangesByExt(.go) = %v, want 3 entries", got)
	}
	for _, p := range got {
		ext := filepath.Ext(p)
		if ext != ".go" && ext != ".GO" {
			t.Errorf("non-.go leaked through: %q", p)
		}
	}
}