package gateway

import (
	"sync"
	"time"

	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

func APIKeyAuth(auth *Authenticator) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			apiKey := extractAPIKey(c.Request())
			if apiKey == "" {
				return shared.Unauthorized("missing_api_key", "missing api key")
			}

			key, err := auth.ValidateAPIKey(c.Request().Context(), apiKey)
			if err != nil {
				return shared.Unauthorized("invalid_api_key", err.Error())
			}

			if err := auth.ValidateAgentAccess(key, key.OwnerID); err != nil {
				return shared.Forbidden("access_denied", err.Error())
			}

			c.Set("api_key", key)
			return next(c)
		}
	}
}

func GetAPIKey(c echo.Context) *apikey.APIKey {
	if key, ok := c.Get("api_key").(*apikey.APIKey); ok {
		return key
	}
	return nil
}

type RateLimiterConfig struct {
	RequestsPerSecond float64
	Burst             int
	CleanupInterval   time.Duration
}

func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		RequestsPerSecond: 10,
		Burst:             20,
		CleanupInterval:   5 * time.Minute,
	}
}

type rateLimiterStore struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	config   RateLimiterConfig
}

func newRateLimiterStore(cfg RateLimiterConfig) *rateLimiterStore {
	store := &rateLimiterStore{
		limiters: make(map[string]*rate.Limiter),
		config:   cfg,
	}
	go store.cleanupLoop()
	return store
}

func (s *rateLimiterStore) getLimiter(key string) *rate.Limiter {
	s.mu.RLock()
	limiter, exists := s.limiters[key]
	s.mu.RUnlock()

	if exists {
		return limiter
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if limiter, exists = s.limiters[key]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(rate.Limit(s.config.RequestsPerSecond), s.config.Burst)
	s.limiters[key] = limiter
	return limiter
}

func (s *rateLimiterStore) cleanupLoop() {
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		for key := range s.limiters {
			delete(s.limiters, key)
		}
		s.mu.Unlock()
	}
}

func RateLimiter(cfg RateLimiterConfig) echo.MiddlewareFunc {
	store := newRateLimiterStore(cfg)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := c.RealIP()
			if apiKey := GetAPIKey(c); apiKey != nil {
				key = apiKey.OwnerID
			}

			limiter := store.getLimiter(key)
			if !limiter.Allow() {
				return shared.NewAPIError("rate_limit_exceeded", "too many requests").ToHTTP(429)
			}

			return next(c)
		}
	}
}
