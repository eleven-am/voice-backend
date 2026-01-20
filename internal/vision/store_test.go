package vision

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestNewStore_DefaultTTL(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{})
	store := NewStore(redisClient, 0)
	if store == nil {
		t.Fatal("NewStore should not return nil")
	}
	if store.frameTTL != 60*time.Second {
		t.Errorf("expected default TTL 60s, got %v", store.frameTTL)
	}
}

func TestNewStore_CustomTTL(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{})
	store := NewStore(redisClient, 30*time.Second)
	if store.frameTTL != 30*time.Second {
		t.Errorf("expected TTL 30s, got %v", store.frameTTL)
	}
}

func getTestRedisClient(t *testing.T) *redis.Client {
	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}
	return redisClient
}

func TestStore_StoreAndGetLatestFrame(t *testing.T) {
	redisClient := getTestRedisClient(t)
	ctx := context.Background()

	testSessionID := "test-store-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	defer store.DeleteFrames(ctx, testSessionID)

	frame := &Frame{
		SessionID: testSessionID,
		Timestamp: time.Now().UnixMilli(),
		Data:      []byte("test frame data"),
		Width:     1920,
		Height:    1080,
	}

	if err := store.StoreFrame(ctx, frame); err != nil {
		t.Fatalf("StoreFrame failed: %v", err)
	}

	retrieved, err := store.GetLatestFrame(ctx, testSessionID)
	if err != nil {
		t.Fatalf("GetLatestFrame failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected frame to be retrieved")
	}
	if retrieved.SessionID != testSessionID {
		t.Errorf("expected SessionID %s, got %s", testSessionID, retrieved.SessionID)
	}
	if string(retrieved.Data) != "test frame data" {
		t.Errorf("expected Data 'test frame data', got %s", string(retrieved.Data))
	}
	if retrieved.Timestamp != frame.Timestamp {
		t.Errorf("expected Timestamp %d, got %d", frame.Timestamp, retrieved.Timestamp)
	}
}

func TestStore_GetLatestFrame_MultipleFrames(t *testing.T) {
	redisClient := getTestRedisClient(t)
	ctx := context.Background()

	testSessionID := "test-latest-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	defer store.DeleteFrames(ctx, testSessionID)

	now := time.Now().UnixMilli()
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now - 2000, Data: []byte("oldest")})
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now - 1000, Data: []byte("middle")})
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now, Data: []byte("newest")})

	retrieved, err := store.GetLatestFrame(ctx, testSessionID)
	if err != nil {
		t.Fatalf("GetLatestFrame failed: %v", err)
	}
	if string(retrieved.Data) != "newest" {
		t.Errorf("expected 'newest', got %s", string(retrieved.Data))
	}
}

func TestStore_GetLatestFrame_NoFrames(t *testing.T) {
	redisClient := getTestRedisClient(t)
	ctx := context.Background()

	testSessionID := "test-noframes-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)

	retrieved, err := store.GetLatestFrame(ctx, testSessionID)
	if err != nil {
		t.Fatalf("GetLatestFrame failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil for non-existent session")
	}
}

func TestStore_GetFrames_TimeRange(t *testing.T) {
	redisClient := getTestRedisClient(t)
	ctx := context.Background()

	testSessionID := "test-range-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	defer store.DeleteFrames(ctx, testSessionID)

	now := time.Now().UnixMilli()
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now - 3000, Data: []byte("frame1")})
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now - 2000, Data: []byte("frame2")})
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now - 1000, Data: []byte("frame3")})
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now, Data: []byte("frame4")})

	frames, err := store.GetFrames(ctx, testSessionID, now-2500, now-500, 10)
	if err != nil {
		t.Fatalf("GetFrames failed: %v", err)
	}
	if len(frames) != 2 {
		t.Errorf("expected 2 frames in range, got %d", len(frames))
	}
}

func TestStore_GetFrames_WithLimit(t *testing.T) {
	redisClient := getTestRedisClient(t)
	ctx := context.Background()

	testSessionID := "test-limit-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	defer store.DeleteFrames(ctx, testSessionID)

	now := time.Now().UnixMilli()
	for i := range 10 {
		store.StoreFrame(ctx, &Frame{
			SessionID: testSessionID,
			Timestamp: now + int64(i*1000),
			Data:      []byte("frame" + string(rune('0'+i))),
		})
	}

	frames, err := store.GetFrames(ctx, testSessionID, now-1000, now+20000, 3)
	if err != nil {
		t.Fatalf("GetFrames failed: %v", err)
	}
	if len(frames) != 3 {
		t.Errorf("expected 3 frames (limit), got %d", len(frames))
	}
}

func TestStore_GetFrames_NoFrames(t *testing.T) {
	redisClient := getTestRedisClient(t)
	ctx := context.Background()

	testSessionID := "test-empty-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)

	frames, err := store.GetFrames(ctx, testSessionID, 0, time.Now().UnixMilli(), 10)
	if err != nil {
		t.Fatalf("GetFrames failed: %v", err)
	}
	if len(frames) != 0 {
		t.Errorf("expected 0 frames, got %d", len(frames))
	}
}

func TestStore_DeleteFrames(t *testing.T) {
	redisClient := getTestRedisClient(t)
	ctx := context.Background()

	testSessionID := "test-delete-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)

	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: time.Now().UnixMilli(), Data: []byte("test")})

	retrieved, _ := store.GetLatestFrame(ctx, testSessionID)
	if retrieved == nil {
		t.Fatal("frame should exist before delete")
	}

	if err := store.DeleteFrames(ctx, testSessionID); err != nil {
		t.Fatalf("DeleteFrames failed: %v", err)
	}

	retrieved, _ = store.GetLatestFrame(ctx, testSessionID)
	if retrieved != nil {
		t.Error("frame should not exist after delete")
	}
}

func TestStore_DeleteFrames_NonExistent(t *testing.T) {
	redisClient := getTestRedisClient(t)
	ctx := context.Background()

	testSessionID := "test-nonexistent-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)

	err := store.DeleteFrames(ctx, testSessionID)
	if err != nil {
		t.Errorf("DeleteFrames should not error on non-existent session: %v", err)
	}
}

func TestStore_FrameOrdering(t *testing.T) {
	redisClient := getTestRedisClient(t)
	ctx := context.Background()

	testSessionID := "test-order-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	defer store.DeleteFrames(ctx, testSessionID)

	now := time.Now().UnixMilli()
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now + 2000, Data: []byte("third")})
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now, Data: []byte("first")})
	store.StoreFrame(ctx, &Frame{SessionID: testSessionID, Timestamp: now + 1000, Data: []byte("second")})

	frames, err := store.GetFrames(ctx, testSessionID, now-1000, now+3000, 10)
	if err != nil {
		t.Fatalf("GetFrames failed: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
	if string(frames[0].Data) != "first" {
		t.Errorf("expected 'first', got %s", string(frames[0].Data))
	}
	if string(frames[1].Data) != "second" {
		t.Errorf("expected 'second', got %s", string(frames[1].Data))
	}
	if string(frames[2].Data) != "third" {
		t.Errorf("expected 'third', got %s", string(frames[2].Data))
	}
}
