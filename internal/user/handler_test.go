package user

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/labstack/echo/v4"
)

func newTestHandler() (*Handler, *SessionManager) {
	sm := NewSessionManager([]byte("test-key"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(nil, nil, nil, sm, []string{"myapp"}, logger)
	return h, sm
}

func TestNewHandler(t *testing.T) {
	sm := NewSessionManager([]byte("key"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(nil, nil, nil, sm, []string{"MyApp", " custom ", "HTTPS"}, logger)

	if h == nil {
		t.Fatal("handler should not be nil")
	}
	if len(h.schemes) != 3 {
		t.Errorf("expected 3 schemes, got %d", len(h.schemes))
	}
	if _, ok := h.schemes["myapp"]; !ok {
		t.Error("myapp scheme should be normalized to lowercase")
	}
	if _, ok := h.schemes["custom"]; !ok {
		t.Error("custom scheme should be trimmed and lowercased")
	}
}

func TestHandler_sanitizeRedirectURI(t *testing.T) {
	h, _ := newTestHandler()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "relative path",
			input:    "/dashboard",
			expected: "/dashboard",
		},
		{
			name:     "https url",
			input:    "https://example.com/path",
			expected: "https://example.com/path",
		},
		{
			name:     "localhost http",
			input:    "http://localhost:3000/callback",
			expected: "http://localhost:3000/callback",
		},
		{
			name:     "127.0.0.1 http",
			input:    "http://127.0.0.1:8080/",
			expected: "http://127.0.0.1:8080/",
		},
		{
			name:     "custom scheme allowed",
			input:    "myapp://callback",
			expected: "myapp://callback",
		},
		{
			name:     "http non-local rejected",
			input:    "http://example.com/",
			expected: "",
		},
		{
			name:     "unknown scheme rejected",
			input:    "ftp://example.com/",
			expected: "",
		},
		{
			name:     "invalid url",
			input:    "://invalid",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := h.sanitizeRedirectURI(tt.input)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}


func TestHandler_handleCallback_MissingStateCookie(t *testing.T) {
	h, _ := newTestHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/callback?state=abc&code=xyz", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.handleCallback(c, &mockProvider{})
	if err == nil {
		t.Fatal("expected error when state cookie is missing")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, httpErr.Code)
	}
}

func TestHandler_handleCallback_StateMismatch(t *testing.T) {
	h, sm := newTestHandler()
	e := echo.New()

	state := sm.SignValue("random|")
	req := httptest.NewRequest(http.MethodGet, "/callback?state=different&code=xyz", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.handleCallback(c, &mockProvider{})
	if err == nil {
		t.Fatal("expected error when state doesn't match")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, httpErr.Code)
	}
}

func TestHandler_handleCallback_MissingCode(t *testing.T) {
	h, sm := newTestHandler()
	e := echo.New()

	state := sm.SignValue("random|")
	req := httptest.NewRequest(http.MethodGet, "/callback?state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.handleCallback(c, &mockProvider{})
	if err == nil {
		t.Fatal("expected error when code is missing")
	}
}

func TestHandler_handleCallback_OAuthError(t *testing.T) {
	h, sm := newTestHandler()
	e := echo.New()

	state := sm.SignValue("random|")
	req := httptest.NewRequest(http.MethodGet, "/callback?state="+state+"&error=access_denied", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.handleCallback(c, &mockProvider{})
	if err == nil {
		t.Fatal("expected error when oauth error param present")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, httpErr.Code)
	}
}

func TestHandler_Me_NotAuthenticated(t *testing.T) {
	h, _ := newTestHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Me(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestHandler_BecomeDeveloper_NotAuthenticated(t *testing.T) {
	h, _ := newTestHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/auth/me/developer", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.BecomeDeveloper(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestHandler_Logout_NotAuthenticated(t *testing.T) {
	h, _ := newTestHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Logout(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
}

func TestHandler_RegisterRoutes(t *testing.T) {
	h, _ := newTestHandler()
	e := echo.New()
	g := e.Group("/auth")

	h.RegisterRoutes(g)

	routes := e.Routes()
	expectedPaths := []string{
		"/auth/google",
		"/auth/google/callback",
		"/auth/github",
		"/auth/github/callback",
		"/auth/me",
		"/auth/me/developer",
		"/auth/logout",
	}

	routePaths := make(map[string]bool)
	for _, r := range routes {
		routePaths[r.Path] = true
	}

	for _, path := range expectedPaths {
		if !routePaths[path] {
			t.Errorf("expected route %s to be registered", path)
		}
	}
}

func TestMeResponse_JSON(t *testing.T) {
	resp := dto.MeResponse{
		ID:          "user_123",
		Email:       "test@example.com",
		Name:        "Test User",
		AvatarURL:   "https://example.com/avatar.png",
		IsDeveloper: true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	expected := `"id":"user_123"`
	if !strings.Contains(string(data), expected) {
		t.Errorf("expected JSON to contain %s", expected)
	}
}

type mockProvider struct {
	name        string
	authURL     string
	user        *ProviderUser
	exchangeErr error
}

func (m *mockProvider) Name() string {
	if m.name == "" {
		return "mock"
	}
	return m.name
}

func (m *mockProvider) AuthURL(state string) string {
	if m.authURL == "" {
		return "https://mock.provider/auth?state=" + state
	}
	return m.authURL
}

func (m *mockProvider) Exchange(ctx context.Context, code string) (*ProviderUser, error) {
	if m.exchangeErr != nil {
		return nil, m.exchangeErr
	}
	if m.user != nil {
		return m.user, nil
	}
	return &ProviderUser{
		Sub:       "mock_sub",
		Email:     "mock@example.com",
		Name:      "Mock User",
		AvatarURL: "https://example.com/mock.png",
	}, nil
}
