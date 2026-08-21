// Package mcp — Tests for McpServer registry and built-in tools.
package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// stubTool is a minimal McpTool for registry tests.
type stubTool struct {
	name        string
	description string
	schema      string
	gotParams   string
	returnVal   interface{}
	returnErr   error
}

func (s *stubTool) Name() string        { return s.name }
func (s *stubTool) Description() string { return s.description }
func (s *stubTool) Schema() string      { return s.schema }
func (s *stubTool) Execute(params json.RawMessage) (interface{}, error) {
	s.gotParams = string(params)
	return s.returnVal, s.returnErr
}

// TestNewServer_Empty verifies a fresh server has no tools.
func TestNewServer_Empty(t *testing.T) {
	s := NewServer()
	if s.Count() != 0 {
		t.Errorf("Count = %d, want 0", s.Count())
	}
	if s.List() == nil {
		t.Error("List should return empty slice, not nil")
	}
	if len(s.List()) != 0 {
		t.Errorf("List len = %d, want 0", len(s.List()))
	}
}

// TestRegister_Valid verifies a tool can be registered.
func TestRegister_Valid(t *testing.T) {
	s := NewServer()
	tool := &stubTool{name: "echo", description: "echoes input", returnVal: "ok"}

	if err := s.Register(tool); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if s.Count() != 1 {
		t.Errorf("Count = %d, want 1", s.Count())
	}
	if !s.Has("echo") {
		t.Error("Has(echo) should be true")
	}
}

// TestRegister_EmptyName rejects tools without a name.
func TestRegister_EmptyName(t *testing.T) {
	s := NewServer()
	tool := &stubTool{name: "", description: "no-name"}
	if err := s.Register(tool); err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

// TestRegister_Duplicate rejects re-registration of the same name.
func TestRegister_Duplicate(t *testing.T) {
	s := NewServer()
	s.MustRegister(&stubTool{name: "dup", description: "first"})
	err := s.Register(&stubTool{name: "dup", description: "second"})
	if err == nil {
		t.Fatal("expected duplicate registration error")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("error = %q, want it to mention 'already registered'", err.Error())
	}
}

// TestRegister_Nil rejects nil tools.
func TestRegister_Nil(t *testing.T) {
	s := NewServer()
	if err := s.Register(nil); err == nil {
		t.Error("expected error for nil tool, got nil")
	}
}

// TestMustRegister_PanicOnDuplicate confirms MustRegister panics.
func TestMustRegister_PanicOnDuplicate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustRegister should panic on duplicate")
		}
	}()
	s := NewServer()
	s.MustRegister(&stubTool{name: "x"})
	s.MustRegister(&stubTool{name: "x"})
}

// TestList_Sorted verifies tool names are returned in sorted order.
func TestList_Sorted(t *testing.T) {
	s := NewServer()
	s.MustRegister(&stubTool{name: "zebra"})
	s.MustRegister(&stubTool{name: "alpha"})
	s.MustRegister(&stubTool{name: "mango"})

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("List len = %d, want 3", len(list))
	}
	want := []string{"alpha", "mango", "zebra"}
	for i, ti := range list {
		if ti.Name != want[i] {
			t.Errorf("list[%d].Name = %q, want %q", i, ti.Name, want[i])
		}
	}
}

// TestGet_FoundAndNotFound covers both branches.
func TestGet_FoundAndNotFound(t *testing.T) {
	s := NewServer()
	s.MustRegister(&stubTool{name: "x", description: "ex tool", schema: `{"type":"object"}`})

	if ti, ok := s.Get("x"); !ok {
		t.Error("Get(x) should be found")
	} else if ti.Description != "ex tool" {
		t.Errorf("Description = %q, want %q", ti.Description, "ex tool")
	}

	if _, ok := s.Get("missing"); ok {
		t.Error("Get(missing) should NOT be found")
	}
}

// TestExecute_DispatchesParams checks that Execute calls the right tool with raw params.
func TestExecute_DispatchesParams(t *testing.T) {
	s := NewServer()
	tool := &stubTool{
		name:      "echo",
		returnVal: map[string]string{"got": "ok"},
	}
	s.MustRegister(tool)

	params := json.RawMessage(`{"x":1,"y":"two"}`)
	res, err := s.Execute("echo", params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !reflect.DeepEqual(res, map[string]string{"got": "ok"}) {
		t.Errorf("result = %v, want {got:ok}", res)
	}
	if tool.gotParams != string(params) {
		t.Errorf("tool got params %q, want %q", tool.gotParams, string(params))
	}
}

// TestExecute_UnknownName returns a helpful error listing registered tools.
func TestExecute_UnknownName(t *testing.T) {
	s := NewServer()
	s.MustRegister(&stubTool{name: "alpha"})

	_, err := s.Execute("missing", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error should mention missing tool: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "alpha") {
		t.Errorf("error should list registered tools: %q", err.Error())
	}
}

// TestExecute_PropagatesError confirms tool errors flow through.
func TestExecute_PropagatesError(t *testing.T) {
	s := NewServer()
	s.MustRegister(&stubTool{name: "fail", returnErr: os.ErrNotExist})

	_, err := s.Execute("fail", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !os.IsNotExist(err) {
		t.Errorf("error = %v, want os.ErrNotExist", err)
	}
}

// TestUnregister_RoundTrip verifies Unregister returns true for existing tools.
func TestUnregister_RoundTrip(t *testing.T) {
	s := NewServer()
	s.MustRegister(&stubTool{name: "temp"})

	if !s.Unregister("temp") {
		t.Error("Unregister(temp) should return true")
	}
	if s.Has("temp") {
		t.Error("tool should be gone")
	}
	if s.Unregister("temp") {
		t.Error("Unregister(temp) on missing tool should return false")
	}
}

// ─────────────────────────── GitHub stub ───────────────────────────

// TestGitHub_NameDescription checks identity of the github tool.
func TestGitHub_NameDescription(t *testing.T) {
	g := NewGitHubTool("owner/repo")
	if g.Name() != "github" {
		t.Errorf("Name = %q, want github", g.Name())
	}
	if !strings.Contains(g.Description(), "GitHub") {
		t.Errorf("Description = %q, want it to mention GitHub", g.Description())
	}
	if g.Schema() == "" {
		t.Error("Schema should not be empty")
	}
}

// TestGitHub_ListIssues_Stub returns synthetic empty result.
func TestGitHub_ListIssues_Stub(t *testing.T) {
	g := NewGitHubTool("owner/repo")
	res, err := g.Execute(json.RawMessage(`{"action":"list_issues"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	m, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("result not a map: %T", res)
	}
	if m["_stub"] != true {
		t.Error("expected _stub=true marker")
	}
	if m["count"].(int) != 0 {
		t.Errorf("count = %v, want 0", m["count"])
	}
}

// TestGitHub_GetIssue_RequiresNumber rejects missing number.
func TestGitHub_GetIssue_RequiresNumber(t *testing.T) {
	g := NewGitHubTool("owner/repo")
	if _, err := g.Execute(json.RawMessage(`{"action":"get_issue"}`)); err == nil {
		t.Error("expected error for missing number")
	}
}

// TestGitHub_CreatePR_RequiresTitle rejects missing title.
func TestGitHub_CreatePR_RequiresTitle(t *testing.T) {
	g := NewGitHubTool("owner/repo")
	if _, err := g.Execute(json.RawMessage(`{"action":"create_pr"}`)); err == nil {
		t.Error("expected error for missing title")
	}
}

// TestGitHub_UnknownAction rejects unsupported actions.
func TestGitHub_UnknownAction(t *testing.T) {
	g := NewGitHubTool("owner/repo")
	if _, err := g.Execute(json.RawMessage(`{"action":"merge_pr"}`)); err == nil {
		t.Error("expected error for unknown action")
	}
}

// TestGitHub_InvalidParams rejects malformed JSON.
func TestGitHub_InvalidParams(t *testing.T) {
	g := NewGitHubTool("owner/repo")
	if _, err := g.Execute(json.RawMessage(`not json`)); err == nil {
		t.Error("expected error for invalid JSON params")
	}
}

// ─────────────────────────── Filesystem stub ───────────────────────────

// TestFilesystem_NameDescription checks identity of the filesystem tool.
func TestFilesystem_NameDescription(t *testing.T) {
	f := NewFilesystemTool("/tmp")
	if f.Name() != "filesystem" {
		t.Errorf("Name = %q, want filesystem", f.Name())
	}
	if !strings.Contains(f.Description(), "Filesystem") {
		t.Errorf("Description = %q, want it to mention Filesystem", f.Description())
	}
}

// TestFilesystem_GlobUsesBasePath verifies glob with a known directory.
func TestFilesystem_GlobUsesBasePath(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	f := NewFilesystemTool(dir)
	res, err := f.Execute(json.RawMessage(`{"action":"glob","pattern":"*.go"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	m := res.(map[string]interface{})
	if m["count"].(int) != 2 {
		t.Errorf("count = %v, want 2", m["count"])
	}
}

// TestFilesystem_TreeDepthLimit verifies depth limit is honored.
func TestFilesystem_TreeDepthLimit(t *testing.T) {
	dir := t.TempDir()
	for i, sub := range []string{"a", "a/b", "a/b/c", "a/b/c/d"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
		_ = i
	}
	f := NewFilesystemTool(dir)
	res, err := f.Execute(json.RawMessage(`{"action":"tree","max_depth":2}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	m := res.(map[string]interface{})
	if m["max_depth"].(int) != 2 {
		t.Errorf("max_depth = %v, want 2", m["max_depth"])
	}
	entries := m["entries"].([]map[string]interface{})
	if len(entries) == 0 {
		t.Error("expected some tree entries")
	}
	// Should NOT include "a/b/c/d" (depth 4)
	for _, e := range entries {
		if strings.HasPrefix(e["path"].(string), "a/b/c/d") {
			t.Errorf("tree leaked beyond depth: %v", e)
		}
	}
}

// TestFilesystem_UnknownAction rejects unsupported actions.
func TestFilesystem_UnknownAction(t *testing.T) {
	f := NewFilesystemTool("/tmp")
	if _, err := f.Execute(json.RawMessage(`{"action":"delete_everything"}`)); err == nil {
		t.Error("expected error for unknown action")
	}
}

// TestFilesystem_ReadMissingFile returns wrapped error.
func TestFilesystem_ReadMissingFile(t *testing.T) {
	f := NewFilesystemTool("/tmp")
	_, err := f.Execute(json.RawMessage(`{"action":"read","path":"/nonexistent_xyz_12345"}`))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// ─────────────────────────── Builtin registration ───────────────────────────

// TestRegisterBuiltins confirms built-in tools are added.
func TestRegisterBuiltins(t *testing.T) {
	s := NewServer()
	RegisterBuiltins(s)
	if s.Count() != 3 {
		t.Errorf("Count = %d, want 3 (github + filesystem + filesystem.watch)", s.Count())
	}
	if !s.Has("github") {
		t.Error("github should be registered")
	}
	if !s.Has("filesystem") {
		t.Error("filesystem should be registered")
	}
	if !s.Has("filesystem.watch") {
		t.Error("filesystem.watch should be registered")
	}
}