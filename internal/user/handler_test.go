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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
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

func TestHandler_handleLogin(t *testing.T) {
	h, _ := newTestHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.handleLogin(c, &mockProvider{})
	if err != nil {
		t.Fatalf("handleLogin failed: %v", err)
	}

	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected redirect status, got %d", rec.Code)
	}

	cookies := rec.Result().Cookies()
	found := false
	for _, cookie := range cookies {
		if cookie.Name == "oauth_state" {
			found = true
			break
		}
	}
	if !found {
		t.Error("oauth_state cookie should be set")
	}
}

func TestHandler_handleLogin_NilProvider(t *testing.T) {
	h, _ := newTestHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.handleLogin(c, nil)
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", httpErr.Code)
	}
}

func TestHandler_handleCallback_NilProvider(t *testing.T) {
	h, _ := newTestHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/callback", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.handleCallback(c, nil)
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", httpErr.Code)
	}
}

func TestHandler_handleLogin_WithRedirectURI(t *testing.T) {
	h, _ := newTestHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/login?redirect_uri=/dashboard", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.handleLogin(c, &mockProvider{})
	if err != nil {
		t.Fatalf("handleLogin failed: %v", err)
	}

	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected redirect status, got %d", rec.Code)
	}
}

func TestHandler_handleCallback_InvalidStateSignature(t *testing.T) {
	h, sm := newTestHandler()
	e := echo.New()

	state := "invalid.signature"
	req := httptest.NewRequest(http.MethodGet, "/callback?state="+state+"&code=xyz", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.handleCallback(c, &mockProvider{})
	if err == nil {
		t.Fatal("expected error for invalid state signature")
	}

	_ = sm
}

func TestHandler_handleCallback_ExchangeError(t *testing.T) {
	h, sm := newTestHandler()
	e := echo.New()

	state := sm.SignValue("random|")
	req := httptest.NewRequest(http.MethodGet, "/callback?state="+state+"&code=xyz", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	provider := &mockProvider{
		exchangeErr: echo.NewHTTPError(http.StatusInternalServerError, "exchange failed"),
	}

	err := h.handleCallback(c, provider)
	if err == nil {
		t.Fatal("expected error for exchange failure")
	}
}


func TestHandler_Me_CSRFFailure(t *testing.T) {
	h, sm := newTestHandler()
	e := echo.New()

	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec1)
	sm.Create(c1, "user_123")

	cookies := rec1.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == sessionCookieName {
			sessionCookie = cookie
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Me(c)
	if err == nil {
		t.Fatal("expected error for missing CSRF")
	}
}

func newTestHandlerWithStore(t *testing.T) (*Handler, *SessionManager, *Store) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	store := NewStore(db)
	store.Migrate()
	sm := NewSessionManager([]byte("test-key"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(store, nil, nil, sm, []string{"myapp"}, logger)
	return h, sm, store
}

func createSessionCookies(t *testing.T, sm *SessionManager, userID string) (sessionCookie, csrfCookie *http.Cookie, csrfToken string) {
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	sm.Create(c, userID)

	cookies := rec.Result().Cookies()
	for _, cookie := range cookies {
		switch cookie.Name {
		case sessionCookieName:
			sessionCookie = cookie
		case csrfCookieName:
			csrfCookie = cookie
			csrfToken = cookie.Value
		}
	}
	return
}

func TestHandler_Me_Success(t *testing.T) {
	h, sm, store := newTestHandlerWithStore(t)
	ctx := context.Background()

	user := &User{
		ID:          "user_me_test",
		Provider:    "google",
		ProviderSub: "sub_me",
		Email:       "me@example.com",
		Name:        "Me User",
		AvatarURL:   "https://example.com/me.png",
		IsDeveloper: true,
	}
	store.Create(ctx, user)

	sessionCookie, csrfCookie, csrfToken := createSessionCookies(t, sm, "user_me_test")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Me(c)
	if err != nil {
		t.Fatalf("Me failed: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp dto.MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.ID != "user_me_test" {
		t.Errorf("expected user_me_test, got %s", resp.ID)
	}
	if resp.Email != "me@example.com" {
		t.Errorf("expected me@example.com, got %s", resp.Email)
	}
	if !resp.IsDeveloper {
		t.Error("expected IsDeveloper to be true")
	}
}

func TestHandler_Me_UserNotFound(t *testing.T) {
	h, sm, _ := newTestHandlerWithStore(t)

	sessionCookie, csrfCookie, csrfToken := createSessionCookies(t, sm, "nonexistent_user")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Me(c)
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", httpErr.Code)
	}
}

func TestHandler_BecomeDeveloper_Success(t *testing.T) {
	h, sm, store := newTestHandlerWithStore(t)
	ctx := context.Background()

	user := &User{
		ID:          "user_dev_test",
		Provider:    "google",
		ProviderSub: "sub_dev",
		IsDeveloper: false,
	}
	store.Create(ctx, user)

	sessionCookie, csrfCookie, csrfToken := createSessionCookies(t, sm, "user_dev_test")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/me/developer", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.BecomeDeveloper(c)
	if err != nil {
		t.Fatalf("BecomeDeveloper failed: %v", err)
	}

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}

	updated, _ := store.GetByID(ctx, "user_dev_test")
	if !updated.IsDeveloper {
		t.Error("user should be developer after call")
	}
}

func TestHandler_BecomeDeveloper_UserNotFound(t *testing.T) {
	h, sm, _ := newTestHandlerWithStore(t)

	sessionCookie, csrfCookie, csrfToken := createSessionCookies(t, sm, "nonexistent_user")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/me/developer", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.BecomeDeveloper(c)
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", httpErr.Code)
	}
}

func TestHandler_BecomeDeveloper_CSRFFailure(t *testing.T) {
	h, sm, _ := newTestHandlerWithStore(t)

	sessionCookie, _, _ := createSessionCookies(t, sm, "user_123")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/me/developer", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.BecomeDeveloper(c)
	if err == nil {
		t.Fatal("expected error for missing CSRF")
	}
}

func TestHandler_Logout_Success(t *testing.T) {
	h, sm, _ := newTestHandlerWithStore(t)

	sessionCookie, csrfCookie, csrfToken := createSessionCookies(t, sm, "user_logout_test")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Logout(c)
	if err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}

	cookies := rec.Result().Cookies()
	for _, cookie := range cookies {
		if cookie.Name == sessionCookieName || cookie.Name == csrfCookieName {
			if cookie.MaxAge != -1 {
				t.Errorf("cookie %s should be cleared with MaxAge -1", cookie.Name)
			}
		}
	}
}

func TestHandler_Logout_CSRFFailure(t *testing.T) {
	h, sm, _ := newTestHandlerWithStore(t)

	sessionCookie, _, _ := createSessionCookies(t, sm, "user_123")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Logout(c)
	if err == nil {
		t.Fatal("expected error for missing CSRF")
	}
}

func TestHandler_handleCallback_Success(t *testing.T) {
	h, sm, store := newTestHandlerWithStore(t)
	e := echo.New()

	state := sm.SignValue("random|/dashboard")
	req := httptest.NewRequest(http.MethodGet, "/callback?state="+state+"&code=xyz", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	provider := &mockProvider{
		user: &ProviderUser{
			Sub:       "mock_sub_123",
			Email:     "callback@example.com",
			Name:      "Callback User",
			AvatarURL: "https://example.com/callback.png",
		},
	}

	err := h.handleCallback(c, provider)
	if err != nil {
		t.Fatalf("handleCallback failed: %v", err)
	}

	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected 307, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != "/dashboard" {
		t.Errorf("expected redirect to /dashboard, got %s", location)
	}

	user, err := store.GetByProvider(context.Background(), "mock", "mock_sub_123")
	if err != nil {
		t.Fatalf("user should be created: %v", err)
	}
	if user.Email != "callback@example.com" {
		t.Errorf("expected callback@example.com, got %s", user.Email)
	}
}
