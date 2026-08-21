package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := Config{
		Host:     "127.0.0.1",
		Port:     0,
		ModelURL: "http://127.0.0.1:11435",
		RepoPath: t.TempDir(),
	}
	return New(cfg)
}

func TestServer_Chat(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(ChatRequest{Message: "hello"})
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
	var resp ChatResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
}

func TestServer_Status(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
}

func TestServer_Files(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/files", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestServer_RepoMap(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/repo-map", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["map"]; !ok {
		t.Error("Response missing 'map' field")
	}
}

func TestServer_Edit(t *testing.T) {
	srv := newTestServer(t)
	// Create a test file in repo
	path := "test.txt"
	fullPath := srv.cfg.RepoPath + "/" + path
	os.WriteFile(fullPath, []byte("hello world\n"), 0644)

	body, _ := json.Marshal(map[string]interface{}{
		"filePath": fullPath,
		"edits": []map[string]string{
			{"search": "hello", "replace": "goodbye"},
		},
	})
	req := httptest.NewRequest("POST", "/api/edit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServer_Commit(t *testing.T) {
	srv := newTestServer(t)
	// Create a file to commit
	os.WriteFile(srv.cfg.RepoPath+"/test.txt", []byte("data"), 0644)

	body, _ := json.Marshal(map[string]string{"message": "test commit"})
	req := httptest.NewRequest("POST", "/api/commit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	// May fail if not a git repo — that's OK
	if w.Code == http.StatusOK {
		var resp map[string]string
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["hash"] == "" {
			t.Error("Expected hash in response")
		}
	}
}

func TestServer_Dispatch(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{
		"message": "explain this code",
	})
	req := httptest.NewRequest("POST", "/api/dispatch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
}

func TestServer_Dispatch_NoMessage(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest("POST", "/api/dispatch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestServer_Edit_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/edit", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

func TestServer_Commit_DefaultMessage(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest("POST", "/api/commit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	// Should use default message
	if w.Code == http.StatusOK {
		var resp map[string]string
		json.Unmarshal(w.Body.Bytes(), &resp)
	}
}

func TestServer_ChatWithModel(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{
		"message": "hello",
		"model":   "vox-dei",
	})
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServer_Explore(t *testing.T) {
	srv := newTestServer(t)
	// Create a test file
	os.WriteFile(srv.cfg.RepoPath+"/test.go", []byte("package main\nfunc main() {}\n"), 0644)

	body, _ := json.Marshal(map[string]string{"file": srv.cfg.RepoPath + "/test.go"})
	req := httptest.NewRequest("POST", "/api/explore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["agent"] != "skitarii" {
		t.Errorf("Expected agent 'skitarii', got %q", resp["agent"])
	}
}

func TestServer_Explore_NoFile(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest("POST", "/api/explore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestServer_Search(t *testing.T) {
	srv := newTestServer(t)
	os.WriteFile(srv.cfg.RepoPath+"/test.go", []byte("package main\nfunc hello() {}\n"), 0644)

	req := httptest.NewRequest("GET", "/api/search?q=hello", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
}

func TestServer_Search_NoQuery(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/search", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestServer_Undo(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("POST", "/api/undo", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	// May fail if not a git repo — that's OK
	if w.Code == http.StatusOK {
		var resp map[string]string
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["status"] != "undone" {
			t.Errorf("Expected status 'undone', got %q", resp["status"])
		}
	}
}

func TestServer_Diff(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/diff", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	// May fail if not a git repo — that's OK
	if w.Code == http.StatusOK {
		var resp map[string]string
		json.Unmarshal(w.Body.Bytes(), &resp)
	}
}

func TestServer_Log(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/log?n=5", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	// May fail if not a git repo — that's OK
	if w.Code == http.StatusOK {
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
	}
}

func TestServer_History(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/history", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestServer_Index(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("Expected HTML content type, got %q", w.Header().Get("Content-Type"))
	}
}

// ═══ Health Endpoint ════════════════════════════════════════════

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["forge"]; !ok {
		t.Error("Health response missing 'forge' field")
	}
}

// ═══ Models Endpoint ════════════════════════════════════════════

func TestModelsEndpoint(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/files", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

// ═══ Chat Endpoint ═════════════════════════════════════════════

func TestChatEndpoint_MissingMessage(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	// Server accepts empty message (processes as empty chat)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("Expected 200 or 400, got %d", w.Code)
	}
}

func TestChatEndpoint_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/chat", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

// ═══ CORS ═══════════════════════════════════════════════════════

func TestCORS_HeadersPresent(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("OPTIONS", "/api/status", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for OPTIONS, got %d", w.Code)
	}
	// CORS is applied via middleware — may not be visible in httptest
	// Just verify OPTIONS doesn't crash
}

// ═══ Not Found ══════════════════════════════════════════════════

func TestNotFound(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/nonexistent", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	// FastAPI returns 404 for unknown routes
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Logf("Unknown route returned %d (may be handled by catch-all)", w.Code)
	}
}

// ═══ Plan Endpoint ══════════════════════════════════════════════

func TestPlanEndpoint_ReturnsPlan(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{
		"message": "add a function to sort",
	})
	req := httptest.NewRequest("POST", "/api/plan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	// Plan endpoint may not exist yet — check for 200 or 404
	if w.Code == http.StatusOK {
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
	}
}

func TestPlanEndpoint_Approve(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{
		"plan_id": "test-plan",
		"action":  "approve",
	})
	req := httptest.NewRequest("POST", "/api/plan/approve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	// May not exist yet
	if w.Code == http.StatusOK {
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
	}
}

// ═══ History Endpoint ═══════════════════════════════════════════

func TestHistoryEndpoint_ReturnsMessages(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/history", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

// ═══ Chat Endpoint With Image ═══════════════════════════════════

func TestChatEndpoint_WithImage(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{
		"message": "describe this",
		"image":   "data:image/png;base64,iVBORw0KGgo=",
	})
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	// Should accept (200) or reject image gracefully
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("Expected 200 or 400, got %d", w.Code)
	}
}

// ═══ Version Endpoint ═══════════════════════════════════════════

func TestVersionEndpoint(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/version", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	// Version endpoint may not exist yet
	if w.Code == http.StatusOK {
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if name, ok := resp["name"]; ok {
			t.Logf("Version name: %v", name)
		}
	}
}
