package gateway

import (
	"io"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestProvideBridge(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	bridge := ProvideBridge(redisClient, logger)
	if bridge == nil {
		t.Fatal("ProvideBridge should not return nil")
	}
	defer bridge.Close()
}

func TestProvideAuthenticator(t *testing.T) {
	auth := ProvideAuthenticator(nil)
	if auth == nil {
		t.Fatal("ProvideAuthenticator should not return nil")
	}
}

func TestProvideAgentHandler(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	bridge := NewBridge(redisClient, logger)
	defer bridge.Close()

	mock := &mockAPIKeyValidator{}
	auth := NewAuthenticator(mock)

	handler := ProvideAgentHandler(auth, bridge, logger)
	if handler == nil {
		t.Fatal("ProvideAgentHandler should not return nil")
	}
}
