// Package tui — Tests for image attachments (/image command).
package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendMessageWithImage_ValidPng(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "test.png")
	if err := os.WriteFile(imgPath, []byte("fake-png-bytes"), 0644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	var received ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %q, want /api/chat", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ChatResponse{Response: "image received", Agent: "minimax"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	resp, err := c.SendMessageWithImage("Опиши изображение.", imgPath)
	if err != nil {
		t.Fatalf("SendMessageWithImage: %v", err)
	}
	if resp.Agent != "minimax" {
		t.Errorf("agent = %q, want minimax", resp.Agent)
	}
	if received.Message != "Опиши изображение." {
		t.Errorf("message = %q, want prompt", received.Message)
	}
	if !strings.HasPrefix(received.Image, "data:image/png;base64,") {
		t.Errorf("image data URI prefix missing, got %q", received.Image[:min(len(received.Image), 30)])
	}
}

func TestSendMessageWithImage_TooLarge(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "big.png")
	big := make([]byte, 11*1024*1024) // 11 MB
	if err := os.WriteFile(imgPath, big, 0644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	c := NewClient("http://127.0.0.1:1")
	_, err := c.SendMessageWithImage("prompt", imgPath)
	if err == nil {
		t.Fatal("expected error for oversized image")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error = %q, want 'too large'", err.Error())
	}
}

func TestSendMessageWithImage_InvalidFormat(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(imgPath, []byte("text"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := NewClient("http://127.0.0.1:1")
	_, err := c.SendMessageWithImage("prompt", imgPath)
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "unsupported image format") {
		t.Errorf("error = %q, want 'unsupported image format'", err.Error())
	}
}

func TestSendMessageWithImage_MissingFile(t *testing.T) {
	c := NewClient("http://127.0.0.1:1")
	_, err := c.SendMessageWithImage("prompt", "/nonexistent/file.png")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
