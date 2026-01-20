package vision

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestNewAnalyzer(t *testing.T) {
	client := NewClient(Config{OllamaURL: "http://localhost", Model: "m"})
	store := NewStore(redis.NewClient(&redis.Options{}), 60*time.Second)

	analyzer := NewAnalyzer(client, store, nil)
	if analyzer == nil {
		t.Fatal("NewAnalyzer should not return nil")
	}
	if analyzer.client != client {
		t.Error("client should match")
	}
	if analyzer.store != store {
		t.Error("store should match")
	}
	if analyzer.logger == nil {
		t.Error("logger should not be nil (default)")
	}
}

func TestNewAnalyzer_WithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	analyzer := NewAnalyzer(nil, nil, logger)
	if analyzer.logger == nil {
		t.Error("logger should be set")
	}
}

func TestAnalyzer_Reset(t *testing.T) {
	analyzer := NewAnalyzer(nil, nil, nil)
	analyzer.lastResult = &AnalyzeResponse{Description: "test"}

	analyzer.Reset()

	if analyzer.lastResult != nil {
		t.Error("lastResult should be nil after Reset")
	}
}

func TestAnalyzer_GetResult_NoAnalysis(t *testing.T) {
	analyzer := NewAnalyzer(nil, nil, nil)

	result := analyzer.GetResult(10 * time.Millisecond)
	if result == nil {
		t.Fatal("GetResult should not return nil")
	}
	if result.Available {
		t.Error("Available should be false when no analysis done")
	}
}

func TestAnalyzer_GetResult_WithCachedResult(t *testing.T) {
	analyzer := NewAnalyzer(nil, nil, nil)
	analyzer.lastResult = &AnalyzeResponse{
		Description: "Cached description",
		Timestamp:   1234567890,
	}

	result := analyzer.GetResult(10 * time.Millisecond)
	if !result.Available {
		t.Error("Available should be true")
	}
	if result.Description != "Cached description" {
		t.Errorf("expected 'Cached description', got %s", result.Description)
	}
	if result.Timestamp != 1234567890 {
		t.Errorf("expected timestamp 1234567890, got %d", result.Timestamp)
	}
}

func TestAnalyzer_StartAnalysis_NoFrames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call ollama when no frames")
	}))
	defer server.Close()

	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}

	testSessionID := "test-no-frames-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	client := NewClient(Config{OllamaURL: server.URL, Model: "m"})
	analyzer := NewAnalyzer(client, store, nil)

	analyzer.StartAnalysis(ctx, testSessionID)
	time.Sleep(50 * time.Millisecond)

	result := analyzer.GetResult(10 * time.Millisecond)
	if result.Available {
		t.Error("should not have result when no frames exist")
	}
}

func TestAnalyzer_StartAnalysis_Concurrent(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		time.Sleep(50 * time.Millisecond)
		json.NewEncoder(w).Encode(ollamaResponse{Response: "desc", Done: true})
	}))
	defer server.Close()

	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}

	testSessionID := "test-concurrent-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: time.Now().UnixMilli(), Data: []byte("test")})
	defer store.DeleteFrames(ctx, testSessionID)

	client := NewClient(Config{OllamaURL: server.URL, Model: "m"})
	analyzer := NewAnalyzer(client, store, nil)

	analyzer.StartAnalysis(ctx, testSessionID)
	analyzer.StartAnalysis(ctx, testSessionID)
	analyzer.StartAnalysis(ctx, testSessionID)

	time.Sleep(200 * time.Millisecond)

	if callCount > 1 {
		t.Errorf("expected at most 1 call during concurrent starts, got %d", callCount)
	}
}

func TestAnalyzer_GetResult_WaitsForCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		json.NewEncoder(w).Encode(ollamaResponse{Response: "waited result", Done: true})
	}))
	defer server.Close()

	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}

	testSessionID := "test-wait-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: time.Now().UnixMilli(), Data: []byte("test")})
	defer store.DeleteFrames(ctx, testSessionID)

	client := NewClient(Config{OllamaURL: server.URL, Model: "m"})
	analyzer := NewAnalyzer(client, store, nil)

	analyzer.StartAnalysis(ctx, testSessionID)

	result := analyzer.GetResult(500 * time.Millisecond)
	if !result.Available {
		t.Error("expected result to be available after waiting")
	}
	if result.Description != "waited result" {
		t.Errorf("expected 'waited result', got %s", result.Description)
	}
}

func TestAnalyzer_GetResult_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		json.NewEncoder(w).Encode(ollamaResponse{Response: "too slow", Done: true})
	}))
	defer server.Close()

	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}

	testSessionID := "test-timeout-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: time.Now().UnixMilli(), Data: []byte("test")})
	defer store.DeleteFrames(ctx, testSessionID)

	client := NewClient(Config{OllamaURL: server.URL, Model: "m"})
	analyzer := NewAnalyzer(client, store, nil)

	analyzer.StartAnalysis(ctx, testSessionID)

	result := analyzer.GetResult(10 * time.Millisecond)
	if result.Available {
		t.Error("expected timeout, result should not be available")
	}
}

func TestAnalyzer_GetFrames_RawBase64(t *testing.T) {
	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}

	testSessionID := "test-getframes-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)

	now := time.Now().UnixMilli()
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now, Data: []byte("frame1")})
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now + 1000, Data: []byte("frame2")})
	defer store.DeleteFrames(ctx, testSessionID)

	analyzer := NewAnalyzer(nil, store, nil)

	resp, err := analyzer.GetFrames(ctx, FrameRequest{
		SessionID: testSessionID,
		StartTime: now - 1000,
		EndTime:   now + 2000,
		Limit:     10,
		RawBase64: true,
	})
	if err != nil {
		t.Fatalf("GetFrames failed: %v", err)
	}

	if len(resp.Frames) != 2 {
		t.Errorf("expected 2 frames, got %d", len(resp.Frames))
	}
}

func TestAnalyzer_GetFrames_Descriptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ollamaResponse{Response: "analyzed", Done: true})
	}))
	defer server.Close()

	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}

	testSessionID := "test-desc-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)

	now := time.Now().UnixMilli()
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now, Data: []byte("frame1")})
	defer store.DeleteFrames(ctx, testSessionID)

	client := NewClient(Config{OllamaURL: server.URL, Model: "m"})
	analyzer := NewAnalyzer(client, store, nil)

	resp, err := analyzer.GetFrames(ctx, FrameRequest{
		SessionID: testSessionID,
		StartTime: now - 1000,
		EndTime:   now + 1000,
		Limit:     10,
		RawBase64: false,
	})
	if err != nil {
		t.Fatalf("GetFrames failed: %v", err)
	}

	if len(resp.Descriptions) != 1 {
		t.Errorf("expected 1 description, got %d", len(resp.Descriptions))
	}
	if resp.Descriptions[0] != "analyzed" {
		t.Errorf("expected 'analyzed', got %s", resp.Descriptions[0])
	}
}

func TestAnalyzer_Cleanup(t *testing.T) {
	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}

	testSessionID := "test-cleanup-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)

	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: time.Now().UnixMilli(), Data: []byte("test")})

	analyzer := NewAnalyzer(nil, store, nil)
	if err := analyzer.Cleanup(ctx, testSessionID); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	frame, _ := store.GetLatestFrame(ctx, testSessionID)
	if frame != nil {
		t.Error("frame should be deleted after Cleanup")
	}
}

func TestAnalyzer_StoreFrame(t *testing.T) {
	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}

	testSessionID := "test-storeframe-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	analyzer := NewAnalyzer(nil, store, nil)

	frame := &Frame{
		SessionID: testSessionID,
		Timestamp: time.Now().UnixMilli(),
		Data:      []byte("test frame data"),
	}

	if err := analyzer.StoreFrame(ctx, frame); err != nil {
		t.Fatalf("StoreFrame failed: %v", err)
	}
	defer store.DeleteFrames(ctx, testSessionID)

	retrieved, err := store.GetLatestFrame(ctx, testSessionID)
	if err != nil {
		t.Fatalf("GetLatestFrame failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected frame to be stored")
	}
	if string(retrieved.Data) != "test frame data" {
		t.Errorf("expected 'test frame data', got %s", string(retrieved.Data))
	}
}
