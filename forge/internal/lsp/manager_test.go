package lsp

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestLSPManager_New(t *testing.T) {
	m := NewLSPManager()
	if len(m.servers) != 0 {
		t.Errorf("new manager has %d servers, want 0", len(m.servers))
	}
}

func TestLSPManager_Get_NotFound(t *testing.T) {
	m := NewLSPManager()
	if s := m.Get("go"); s != nil {
		t.Error("expected nil for unknown language")
	}
}

func TestServerCommand_Go(t *testing.T) {
	binary, args, err := serverCommand("go")
	if err != nil {
		t.Fatalf("serverCommand(go): %v", err)
	}
	if binary != "gopls" {
		t.Errorf("binary = %s, want gopls", binary)
	}
	if len(args) == 0 {
		t.Error("expected args for gopls")
	}
}

func TestServerCommand_Python(t *testing.T) {
	binary, _, err := serverCommand("python")
	if err != nil {
		t.Fatalf("serverCommand(python): %v", err)
	}
	if binary != "pylsp" {
		t.Errorf("binary = %s, want pylsp", binary)
	}
}

func TestServerCommand_Rust(t *testing.T) {
	binary, _, err := serverCommand("rust")
	if err != nil {
		t.Fatalf("serverCommand(rust): %v", err)
	}
	if binary != "rust-analyzer" {
		t.Errorf("binary = %s, want rust-analyzer", binary)
	}
}

func TestServerCommand_Unknown(t *testing.T) {
	_, _, err := serverCommand("cobol")
	if err == nil {
		t.Error("expected error for unknown language")
	}
}

func TestLanguageID(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"script.py", "python"},
		{"main.rs", "rust"},
		{"app.ts", "typescript"},
		{"app.js", "javascript"},
		{"config.json", "json"},
		{"config.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"README.md", "markdown"},
		{"unknown.xyz", "plaintext"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := languageID(tt.path); got != tt.want {
				t.Errorf("languageID(%s) = %s, want %s", tt.path, got, tt.want)
			}
		})
	}
}

func TestPathToURI(t *testing.T) {
	uri := pathToURI("/home/user/main.go")
	if uri == "" {
		t.Error("URI should not be empty")
	}
	if uri[:7] != "file://" {
		t.Errorf("URI should start with file://, got %s", uri[:7])
	}
}

func TestEncodeMessage(t *testing.T) {
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test",
	}
	data, err := encodeMessage(req)
	if err != nil {
		t.Fatalf("encodeMessage: %v", err)
	}
	s := string(data)
	if s[:15] != "Content-Length:" {
		t.Error("should start with Content-Length header")
	}
}

func TestRPCResponse_Unmarshal(t *testing.T) {
	raw := json.RawMessage(`{"jsonrpc":"2.0","id":1,"result":{"x":42}}`)
	var resp RPCResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != 1 {
		t.Errorf("ID = %d, want 1", resp.ID)
	}
}

func TestRPCResponse_Error(t *testing.T) {
	raw := json.RawMessage(`{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"Method not found"}}`)
	var resp RPCResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("should have error")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("code = %d, want -32601", resp.Error.Code)
	}
}

func TestDiagnostic_JSON(t *testing.T) {
	d := Diagnostic{
		Line:     5,
		Col:      10,
		Message:  "undefined: fmt",
		Severity: "error",
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var d2 Diagnostic
	if err := json.Unmarshal(b, &d2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d2.Message != d.Message {
		t.Errorf("message = %s, want %s", d2.Message, d.Message)
	}
}

func TestLSPManager_StartGo(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}
	m := NewLSPManager()
	defer m.StopAll()
	if err := m.Start("go", "../../"); err != nil {
		t.Fatalf("Start(go): %v", err)
	}
	s := m.Get("go")
	if s == nil {
		t.Fatal("server not found after start")
	}
}

func TestLSPServer_Initialize(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}
	m := NewLSPManager()
	defer m.StopAll()
	if err := m.Start("go", "../../"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	s := m.Get("go")
	if err := s.Initialize("../../"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
}

func TestLSPServer_Diagnostics_Empty(t *testing.T) {
	s := &LSPServer{
		diagStore: make(map[string][]Diagnostic),
	}
	diags := s.Diagnostics("test.go")
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestEnsureRunning_Idempotent(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}
	m := NewLSPManager()
	defer m.StopAll()
	if err := m.EnsureRunning("go", "../../"); err != nil {
		t.Fatalf("first EnsureRunning: %v", err)
	}
	if err := m.EnsureRunning("go", "../../"); err != nil {
		t.Fatalf("second EnsureRunning: %v", err)
	}
}
