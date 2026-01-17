package user

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestNewSessionManager(t *testing.T) {
	sm := NewSessionManager([]byte("test-key"), true, "example.com")
	if sm == nil {
		t.Fatal("expected non-nil session manager")
	}
	if !sm.secure {
		t.Error("expected secure to be true")
	}
	if sm.domain != "example.com" {
		t.Errorf("expected domain 'example.com', got '%s'", sm.domain)
	}
}

func TestSessionManager_SignAndVerify(t *testing.T) {
	sm := NewSessionManager([]byte("secret-key"), false, "")

	tests := []struct {
		name  string
		value string
	}{
		{name: "simple value", value: "hello"},
		{name: "with special chars", value: "user123|csrftoken"},
		{name: "empty", value: ""},
		{name: "unicode", value: "hello 世界"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signed := sm.SignValue(tt.value)
			if !strings.Contains(signed, ".") {
				t.Error("signed value should contain separator '.'")
			}

			verified, err := sm.VerifyValue(signed)
			if err != nil {
				t.Fatalf("verification failed: %v", err)
			}
			if verified != tt.value {
				t.Errorf("expected '%s', got '%s'", tt.value, verified)
			}
		})
	}
}

func TestSessionManager_VerifyValue_Invalid(t *testing.T) {
	sm := NewSessionManager([]byte("secret-key"), false, "")

	tests := []struct {
		name   string
		signed string
	}{
		{name: "no separator", signed: "noseparator"},
		{name: "wrong signature", signed: "dGVzdA==.wrongsig"},
		{name: "invalid base64", signed: "!!!invalid.sig"},
		{name: "empty", signed: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sm.VerifyValue(tt.signed)
			if err == nil {
				t.Error("expected error for invalid signed value")
			}
		})
	}
}

func TestSessionManager_CreateAndGet(t *testing.T) {
	sm := NewSessionManager([]byte("test-key"), false, "")
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	sm.Create(c, "user_123")

	cookies := rec.Result().Cookies()
	if len(cookies) < 2 {
		t.Fatalf("expected at least 2 cookies, got %d", len(cookies))
	}

	var sessionCookie, csrfCookie *http.Cookie
	for _, cookie := range cookies {
		switch cookie.Name {
		case sessionCookieName:
			sessionCookie = cookie
		case csrfCookieName:
			csrfCookie = cookie
		}
	}

	if sessionCookie == nil {
		t.Fatal("session cookie not found")
	}
	if csrfCookie == nil {
		t.Fatal("csrf cookie not found")
	}
	if sessionCookie.HttpOnly != true {
		t.Error("session cookie should be HttpOnly")
	}
	if csrfCookie.HttpOnly != false {
		t.Error("csrf cookie should not be HttpOnly")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(sessionCookie)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)

	userID, csrf, err := sm.Get(c2)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if userID != "user_123" {
		t.Errorf("expected userID 'user_123', got '%s'", userID)
	}
	if csrf == "" {
		t.Error("csrf should not be empty")
	}
	if csrf != csrfCookie.Value {
		t.Error("csrf from session should match csrf cookie")
	}
}

func TestSessionManager_Get_NoCookie(t *testing.T) {
	sm := NewSessionManager([]byte("test-key"), false, "")
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_, _, err := sm.Get(c)
	if err == nil {
		t.Error("expected error when no session cookie")
	}
}

func TestSessionManager_Clear(t *testing.T) {
	sm := NewSessionManager([]byte("test-key"), false, "")
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	sm.Clear(c)

	cookies := rec.Result().Cookies()
	for _, cookie := range cookies {
		if cookie.Name == sessionCookieName || cookie.Name == csrfCookieName {
			if cookie.MaxAge != -1 {
				t.Errorf("cookie %s should have MaxAge -1, got %d", cookie.Name, cookie.MaxAge)
			}
		}
	}
}

func TestSessionManager_RequireCSRF(t *testing.T) {
	sm := NewSessionManager([]byte("test-key"), false, "")
	e := echo.New()

	csrfToken := "test-csrf-token"

	tests := []struct {
		name        string
		header      string
		cookieValue string
		sessionCSRF string
		wantErr     bool
	}{
		{
			name:        "valid csrf",
			header:      csrfToken,
			cookieValue: csrfToken,
			sessionCSRF: csrfToken,
			wantErr:     false,
		},
		{
			name:        "missing header",
			header:      "",
			cookieValue: csrfToken,
			sessionCSRF: csrfToken,
			wantErr:     true,
		},
		{
			name:        "missing cookie",
			header:      csrfToken,
			cookieValue: "",
			sessionCSRF: csrfToken,
			wantErr:     true,
		},
		{
			name:        "header mismatch",
			header:      "wrong",
			cookieValue: csrfToken,
			sessionCSRF: csrfToken,
			wantErr:     true,
		},
		{
			name:        "session csrf mismatch",
			header:      csrfToken,
			cookieValue: csrfToken,
			sessionCSRF: "different",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if tt.header != "" {
				req.Header.Set("X-CSRF-Token", tt.header)
			}
			if tt.cookieValue != "" {
				req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: tt.cookieValue})
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := sm.RequireCSRF(c, tt.sessionCSRF)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSessionManager_GenerateOAuthState(t *testing.T) {
	sm := NewSessionManager([]byte("test-key"), false, "")

	state1 := sm.GenerateOAuthState("")
	state2 := sm.GenerateOAuthState("")
	if state1 == state2 {
		t.Error("states should be unique")
	}

	stateWithURI := sm.GenerateOAuthState("https://example.com/callback")
	if stateWithURI == "" {
		t.Error("state should not be empty")
	}
}

func TestSessionManager_ExtractRedirectURI(t *testing.T) {
	sm := NewSessionManager([]byte("test-key"), false, "")

	tests := []struct {
		name     string
		redirect string
		expected string
	}{
		{
			name:     "with redirect URI",
			redirect: "https://example.com/callback",
			expected: "https://example.com/callback",
		},
		{
			name:     "empty redirect URI",
			redirect: "",
			expected: "",
		},
		{
			name:     "path only",
			redirect: "/dashboard",
			expected: "/dashboard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := sm.GenerateOAuthState(tt.redirect)
			extracted := sm.ExtractRedirectURI(state)
			if extracted != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, extracted)
			}
		})
	}

	invalid := sm.ExtractRedirectURI("invalid.signature")
	if invalid != "" {
		t.Error("should return empty for invalid state")
	}
}
