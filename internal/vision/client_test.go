package vision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient_Defaults(t *testing.T) {
	cfg := Config{
		OllamaURL: "http://localhost:11434",
		Model:     "qwen2.5vl",
	}
	client := NewClient(cfg)
	if client == nil {
		t.Fatal("NewClient should not return nil")
	}
	if client.baseURL != cfg.OllamaURL {
		t.Errorf("expected baseURL %s, got %s", cfg.OllamaURL, client.baseURL)
	}
	if client.model != cfg.Model {
		t.Errorf("expected model %s, got %s", cfg.Model, client.model)
	}
	if client.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
}

func TestNewClient_CustomTimeout(t *testing.T) {
	cfg := Config{
		OllamaURL: "http://localhost:11434",
		Model:     "qwen2.5vl",
		Timeout:   10 * time.Second,
	}
	client := NewClient(cfg)
	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", client.httpClient.Timeout)
	}
}

func TestClient_Analyze_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/generate" {
			t.Errorf("expected /api/generate, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json")
		}

		var req ollamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Model != "test-model" {
			t.Errorf("expected model 'test-model', got %s", req.Model)
		}
		if len(req.Images) != 1 {
			t.Errorf("expected 1 image, got %d", len(req.Images))
		}
		if req.Stream {
			t.Error("stream should be false")
		}

		resp := ollamaResponse{
			Response: "A test description",
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		OllamaURL: server.URL,
		Model:     "test-model",
	})

	frame := &Frame{
		SessionID: "session-1",
		Timestamp: 1234567890,
		Data:      []byte("test image data"),
	}

	resp, err := client.Analyze(context.Background(), AnalyzeRequest{
		Frame: frame,
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if resp.Description != "A test description" {
		t.Errorf("expected description 'A test description', got %s", resp.Description)
	}
	if resp.Timestamp != frame.Timestamp {
		t.Errorf("expected timestamp %d, got %d", frame.Timestamp, resp.Timestamp)
	}
}

func TestClient_Analyze_CustomPrompt(t *testing.T) {
	var receivedPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedPrompt = req.Prompt
		json.NewEncoder(w).Encode(ollamaResponse{Response: "ok", Done: true})
	}))
	defer server.Close()

	client := NewClient(Config{OllamaURL: server.URL, Model: "m"})
	client.Analyze(context.Background(), AnalyzeRequest{
		Frame:  &Frame{Data: []byte("x")},
		Prompt: "Custom prompt here",
	})

	if receivedPrompt != "Custom prompt here" {
		t.Errorf("expected custom prompt, got %s", receivedPrompt)
	}
}

func TestClient_Analyze_NoFrame(t *testing.T) {
	client := NewClient(Config{OllamaURL: "http://localhost", Model: "m"})

	_, err := client.Analyze(context.Background(), AnalyzeRequest{Frame: nil})
	if err == nil {
		t.Error("expected error for nil frame")
	}
}

func TestClient_Analyze_EmptyFrameData(t *testing.T) {
	client := NewClient(Config{OllamaURL: "http://localhost", Model: "m"})

	_, err := client.Analyze(context.Background(), AnalyzeRequest{
		Frame: &Frame{Data: []byte{}},
	})
	if err == nil {
		t.Error("expected error for empty frame data")
	}
}

func TestClient_Analyze_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Config{OllamaURL: server.URL, Model: "m"})
	_, err := client.Analyze(context.Background(), AnalyzeRequest{
		Frame: &Frame{Data: []byte("x")},
	})
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestClient_Analyze_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(Config{OllamaURL: server.URL, Model: "m"})
	_, err := client.Analyze(context.Background(), AnalyzeRequest{
		Frame: &Frame{Data: []byte("x")},
	})
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestClient_Analyze_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		json.NewEncoder(w).Encode(ollamaResponse{Response: "ok", Done: true})
	}))
	defer server.Close()

	client := NewClient(Config{OllamaURL: server.URL, Model: "m"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Analyze(ctx, AnalyzeRequest{
		Frame: &Frame{Data: []byte("x")},
	})
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestClient_IsAvailable_True(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/tags" {
			t.Errorf("expected /api/tags, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(Config{OllamaURL: server.URL, Model: "m"})
	if !client.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable to return true")
	}
}

func TestClient_IsAvailable_ServerDown(t *testing.T) {
	client := NewClient(Config{OllamaURL: "http://localhost:99999", Model: "m"})
	if client.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable to return false for unreachable server")
	}
}

func TestClient_IsAvailable_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(Config{OllamaURL: server.URL, Model: "m"})
	if client.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable to return false for 503")
	}
}
