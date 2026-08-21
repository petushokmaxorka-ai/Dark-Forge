// Package mcp — Tests for StdioServer JSON-RPC transport.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// startStdio spins up a StdioServer reading from `in` and writing to `out`.
// Returns a channel that closes when Serve() exits.
func startStdio(t *testing.T, s *McpServer, in *bytes.Buffer, out *bytes.Buffer) chan struct{} {
	t.Helper()
	srv := NewStdioServerWithIO(s, in, out)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve()
	}()
	return done
}

// TestStdio_Initialize returns server info.
func TestStdio_Initialize(t *testing.T) {
	s := NewServer()
	s.MustRegister(&stubTool{name: "echo", description: "echoes", returnVal: "ok"})

	in := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	out := &bytes.Buffer{}
	done := startStdio(t, s, in, out)
	<-done

	var resp JSONRPCResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (out=%q)", err, out.String())
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result not a map: %T", resp.Result)
	}
	if result["serverInfo"].(map[string]interface{})["name"] != "heretic-forge-mcp" {
		t.Errorf("serverInfo.name wrong: %v", result["serverInfo"])
	}
}

// TestStdio_ToolsList returns registered tools.
func TestStdio_ToolsList(t *testing.T) {
	s := NewServer()
	s.MustRegister(&stubTool{name: "alpha"})
	s.MustRegister(&stubTool{name: "beta"})

	in := bytes.NewBufferString(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n")
	out := &bytes.Buffer{}
	done := startStdio(t, s, in, out)
	<-done

	var resp JSONRPCResponse
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	result := resp.Result.(map[string]interface{})
	tools := result["tools"].([]interface{})
	if len(tools) != 2 {
		t.Errorf("tools len = %d, want 2", len(tools))
	}
}

// TestStdio_ToolsCall dispatches a tool call.
func TestStdio_ToolsCall(t *testing.T) {
	s := NewServer()
	s.MustRegister(&stubTool{name: "echo", returnVal: map[string]string{"hi": "there"}})

	req := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"x":1}}}`
	in := bytes.NewBufferString(req + "\n")
	out := &bytes.Buffer{}
	done := startStdio(t, s, in, out)
	<-done

	var resp JSONRPCResponse
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	result := resp.Result.(map[string]interface{})
	if result["content"] == nil {
		t.Errorf("missing content: %v", result)
	}
}

// TestStdio_ToolsCall_UnknownTool returns an error.
func TestStdio_ToolsCall_UnknownTool(t *testing.T) {
	s := NewServer()
	req := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"missing","arguments":{}}}`
	in := bytes.NewBufferString(req + "\n")
	out := &bytes.Buffer{}
	done := startStdio(t, s, in, out)
	<-done

	var resp JSONRPCResponse
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error == nil {
		t.Fatal("expected error for unknown tool")
	}
	if resp.Error.Code != RPCErrInternal {
		t.Errorf("error code = %d, want %d", resp.Error.Code, RPCErrInternal)
	}
}

// TestStdio_ParseError returns parse error on invalid JSON.
func TestStdio_ParseError(t *testing.T) {
	s := NewServer()
	in := bytes.NewBufferString("not json\n")
	out := &bytes.Buffer{}
	done := startStdio(t, s, in, out)
	<-done

	var resp JSONRPCResponse
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error == nil {
		t.Fatal("expected parse error")
	}
	if resp.Error.Code != RPCErrParse {
		t.Errorf("error code = %d, want %d", resp.Error.Code, RPCErrParse)
	}
}

// TestStdio_MethodNotFound returns proper error for unknown methods.
func TestStdio_MethodNotFound(t *testing.T) {
	s := NewServer()
	req := `{"jsonrpc":"2.0","id":5,"method":"unknown/method"}`
	in := bytes.NewBufferString(req + "\n")
	out := &bytes.Buffer{}
	done := startStdio(t, s, in, out)
	<-done

	var resp JSONRPCResponse
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error == nil {
		t.Fatal("expected method-not-found error")
	}
	if resp.Error.Code != RPCErrMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, RPCErrMethodNotFound)
	}
}

// TestStdio_MultipleRequests handles multiple requests in one stream.
func TestStdio_MultipleRequests(t *testing.T) {
	s := NewServer()
	s.MustRegister(&stubTool{name: "ping", returnVal: "pong"})

	requests := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ping","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"initialize"}`,
	}, "\n") + "\n"

	in := bytes.NewBufferString(requests)
	out := &bytes.Buffer{}
	done := startStdio(t, s, in, out)
	<-done

	// Should have 3 responses, one per line.
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		var resp JSONRPCResponse
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			t.Errorf("decode resp %d: %v", count, err)
		}
		count++
	}
	if count != 3 {
		t.Errorf("got %d responses, want 3", count)
	}
}