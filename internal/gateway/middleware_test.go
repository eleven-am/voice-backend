package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

func TestAPIKeyAuth(t *testing.T) {
	tests := []struct {
		name           string
		authHeader     string
		mockKey        *apikey.APIKey
		mockErr        error
		wantStatusCode int
		wantErr        bool
	}{
		{
			name:           "missing api key",
			authHeader:     "",
			mockKey:        nil,
			mockErr:        nil,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
		},
		{
			name:           "invalid api key",
			authHeader:     "Bearer sk-invalid",
			mockKey:        nil,
			mockErr:        errors.New("not found"),
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
		},
		{
			name:       "valid agent key",
			authHeader: "Bearer sk-valid",
			mockKey: &apikey.APIKey{
				ID:        "key_123",
				OwnerID:   "agent_456",
				OwnerType: apikey.OwnerTypeAgent,
			},
			mockErr:        nil,
			wantStatusCode: http.StatusOK,
			wantErr:        false,
		},
		{
			name:       "user key forbidden",
			authHeader: "Bearer sk-user",
			mockKey: &apikey.APIKey{
				ID:        "key_789",
				OwnerID:   "user_123",
				OwnerType: apikey.OwnerTypeUser,
			},
			mockErr:        nil,
			wantStatusCode: http.StatusForbidden,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			mock := &mockAPIKeyValidator{
				validateFunc: func(ctx context.Context, secret string) (*apikey.APIKey, error) {
					return tt.mockKey, tt.mockErr
				},
			}
			auth := NewAuthenticator(mock)

			middleware := APIKeyAuth(auth)
			handler := middleware(func(c echo.Context) error {
				return c.String(http.StatusOK, "success")
			})

			err := handler(c)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				he, ok := err.(*echo.HTTPError)
				if !ok {
					t.Errorf("expected *echo.HTTPError, got %T", err)
					return
				}
				if he.Code != tt.wantStatusCode {
					t.Errorf("error code = %d, want %d", he.Code, tt.wantStatusCode)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if rec.Code != tt.wantStatusCode {
					t.Errorf("status code = %d, want %d", rec.Code, tt.wantStatusCode)
				}
			}
		})
	}
}

func TestGetAPIKey(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if key := GetAPIKey(c); key != nil {
		t.Error("expected nil key when not set")
	}

	expectedKey := &apikey.APIKey{ID: "key_123", OwnerID: "agent_456"}
	c.Set("api_key", expectedKey)

	key := GetAPIKey(c)
	if key == nil {
		t.Error("expected key to be returned")
	}
	if key.ID != expectedKey.ID {
		t.Errorf("key ID = %q, want %q", key.ID, expectedKey.ID)
	}
}

func TestDefaultRateLimiterConfig(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	if cfg.RequestsPerSecond != 10 {
		t.Errorf("RequestsPerSecond = %f, want 10", cfg.RequestsPerSecond)
	}
	if cfg.Burst != 20 {
		t.Errorf("Burst = %d, want 20", cfg.Burst)
	}
	if cfg.CleanupInterval != 5*time.Minute {
		t.Errorf("CleanupInterval = %v, want 5m", cfg.CleanupInterval)
	}
}

func TestRateLimiterStore_GetLimiter(t *testing.T) {
	store := &rateLimiterStore{
		limiters: make(map[string]*rate.Limiter),
		config: RateLimiterConfig{
			RequestsPerSecond: 10,
			Burst:             20,
		},
	}

	limiter1 := store.getLimiter("key1")
	if limiter1 == nil {
		t.Error("expected limiter to be created")
	}

	limiter2 := store.getLimiter("key1")
	if limiter1 != limiter2 {
		t.Error("expected same limiter to be returned")
	}

	limiter3 := store.getLimiter("key2")
	if limiter1 == limiter3 {
		t.Error("expected different limiter for different key")
	}
}

func TestRateLimiter_AllowsRequests(t *testing.T) {
	e := echo.New()

	cfg := RateLimiterConfig{
		RequestsPerSecond: 100,
		Burst:             100,
		CleanupInterval:   time.Hour,
	}
	middleware := RateLimiter(cfg)

	handler := middleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler(c); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRateLimiter_BlocksExcessiveRequests(t *testing.T) {
	e := echo.New()

	cfg := RateLimiterConfig{
		RequestsPerSecond: 1,
		Burst:             1,
		CleanupInterval:   time.Hour,
	}
	middleware := RateLimiter(cfg)

	handler := middleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	})

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler(c)
		if i == 0 {
			if err != nil {
				t.Errorf("first request should succeed, got error: %v", err)
			}
		} else {
			if err == nil {
				continue
			}
			he, ok := err.(*echo.HTTPError)
			if !ok {
				t.Errorf("expected echo.HTTPError, got %T", err)
				continue
			}
			if he.Code != 429 {
				t.Errorf("request %d status = %d, want 429", i+1, he.Code)
			}
		}
	}
}

func TestRateLimiter_UsesAPIKeyOwner(t *testing.T) {
	e := echo.New()

	cfg := RateLimiterConfig{
		RequestsPerSecond: 1,
		Burst:             1,
		CleanupInterval:   time.Hour,
	}
	middleware := RateLimiter(cfg)

	handler := middleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	})

	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.Set("api_key", &apikey.APIKey{OwnerID: "owner1"})

	if err := handler(c1); err != nil {
		t.Errorf("first request for owner1 should succeed: %v", err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.Set("api_key", &apikey.APIKey{OwnerID: "owner2"})

	if err := handler(c2); err != nil {
		t.Errorf("first request for owner2 should succeed: %v", err)
	}
}
