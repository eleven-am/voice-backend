package user

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eleven-am/voice-backend/internal/auth"
	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/labstack/echo/v4"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func newTestHandler(t *testing.T) (*Handler, *Store) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	store := NewStore(db)
	store.Migrate()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(store, logger)
	return h, store
}

func setAuthClaims(c echo.Context, userID, email, name string) {
	claims := &auth.Claims{
		UserID: userID,
		Email:  email,
		Name:   name,
	}
	auth.SetClaimsForTest(c, claims)
}

func TestNewHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(nil, logger)

	if h == nil {
		t.Fatal("handler should not be nil")
	}
}

func TestHandler_Me_NotAuthenticated(t *testing.T) {
	h, _ := newTestHandler(t)
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
	h, _ := newTestHandler(t)
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

func TestHandler_RegisterRoutes(t *testing.T) {
	h, _ := newTestHandler(t)
	e := echo.New()
	g := e.Group("/auth")

	h.RegisterRoutes(g)

	routes := e.Routes()
	expectedPaths := []string{
		"/auth/me",
		"/auth/me/developer",
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

	if !json.Valid(data) {
		t.Error("response should be valid JSON")
	}
}

func TestHandler_Me_Success(t *testing.T) {
	h, store := newTestHandler(t)
	ctx := context.Background()

	user := &User{
		ID:          "user_me_test",
		Provider:    "better-auth",
		ProviderSub: "user_me_test",
		Email:       "me@example.com",
		Name:        "Me User",
		AvatarURL:   "https://example.com/me.png",
		IsDeveloper: true,
	}
	store.Create(ctx, user)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	setAuthClaims(c, "user_me_test", "me@example.com", "Me User")

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
	h, _ := newTestHandler(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	setAuthClaims(c, "nonexistent_user", "test@example.com", "Test User")

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
	h, store := newTestHandler(t)
	ctx := context.Background()

	user := &User{
		ID:          "user_dev_test",
		Provider:    "better-auth",
		ProviderSub: "user_dev_test",
		IsDeveloper: false,
	}
	store.Create(ctx, user)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/me/developer", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	setAuthClaims(c, "user_dev_test", "test@example.com", "Test User")

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
	h, _ := newTestHandler(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/me/developer", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	setAuthClaims(c, "nonexistent_user", "test@example.com", "Test User")

	err := h.BecomeDeveloper(c)
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", httpErr.Code)
	}
}
