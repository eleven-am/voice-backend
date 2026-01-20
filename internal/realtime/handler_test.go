package realtime

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/labstack/echo/v4"
)

func TestNewHandler(t *testing.T) {
	mgr, _ := NewManager(Config{})
	h := NewHandler(mgr, nil, nil, nil, nil)
	if h == nil {
		t.Fatal("NewHandler should not return nil")
	}
	if h.manager != mgr {
		t.Error("handler should use provided manager")
	}
	if h.log == nil {
		t.Error("handler should have default logger")
	}
}

func TestNewHandler_WithLogger(t *testing.T) {
	mgr, _ := NewManager(Config{})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(mgr, nil, nil, nil, logger)
	if h.log == nil {
		t.Error("handler should have provided logger")
	}
}

func TestNewHandler_WithAuth(t *testing.T) {
	mgr, _ := NewManager(Config{})
	auth := func(r *http.Request) (*transport.UserProfile, error) {
		return &transport.UserProfile{UserID: "user-123"}, nil
	}
	h := NewHandler(mgr, nil, auth, nil, nil)
	if h.auth == nil {
		t.Error("handler should have auth func")
	}
}

func TestHandler_maxSDPSize_Default(t *testing.T) {
	mgr, _ := NewManager(Config{})
	h := NewHandler(mgr, nil, nil, nil, nil)
	maxSize := h.maxSDPSize()
	if maxSize != 64*1024 {
		t.Errorf("expected default max SDP size 64KB, got %d", maxSize)
	}
}

func TestHandler_maxSDPSize_Custom(t *testing.T) {
	mgr, _ := NewManager(Config{MaxSDPSize: 128 * 1024})
	h := NewHandler(mgr, nil, nil, nil, nil)
	maxSize := h.maxSDPSize()
	if maxSize != 128*1024 {
		t.Errorf("expected custom max SDP size 128KB, got %d", maxSize)
	}
}

func TestHandler_iceServersResponse_Default(t *testing.T) {
	mgr, _ := NewManager(Config{})
	h := NewHandler(mgr, nil, nil, nil, nil)
	servers := h.iceServersResponse()
	if len(servers) != 1 {
		t.Fatalf("expected 1 default server, got %d", len(servers))
	}
	if servers[0].URLs[0] != "stun:stun.l.google.com:19302" {
		t.Errorf("expected default STUN server, got %s", servers[0].URLs[0])
	}
}

func TestHandler_iceServersResponse_Custom(t *testing.T) {
	mgr, _ := NewManager(Config{
		ICEServers: []ICEServerConfig{
			{URLs: []string{"stun:stun.example.com"}},
			{URLs: []string{"turn:turn.example.com"}, Username: "user", Credential: "pass"},
		},
	})
	h := NewHandler(mgr, nil, nil, nil, nil)
	servers := h.iceServersResponse()
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	if servers[1].Username != "user" {
		t.Errorf("expected username 'user', got %s", servers[1].Username)
	}
}

func TestHandler_HandleICEServers(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{
		ICEServers: []ICEServerConfig{
			{URLs: []string{"stun:stun.example.com"}},
		},
	})
	h := NewHandler(mgr, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/ice-servers", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.HandleICEServers(c)
	if err != nil {
		t.Fatalf("HandleICEServers should not error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var response map[string][]ICEServer
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	servers := response["ice_servers"]
	if len(servers) != 1 {
		t.Errorf("expected 1 server in response, got %d", len(servers))
	}
}

func TestHandler_extractSDP_JSON(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	h := NewHandler(mgr, nil, nil, nil, nil)

	body := `{"sdp":"v=0\r\n..."}`
	req := httptest.NewRequest(http.MethodPost, "/calls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	sdp, err := h.extractSDP(c)
	if err != nil {
		t.Fatalf("extractSDP should not error: %v", err)
	}
	if sdp != "v=0\r\n..." {
		t.Errorf("expected SDP 'v=0\\r\\n...', got %s", sdp)
	}
}

func TestHandler_extractSDP_EmptyContentType(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	h := NewHandler(mgr, nil, nil, nil, nil)

	body := `{"sdp":"test-sdp"}`
	req := httptest.NewRequest(http.MethodPost, "/calls", strings.NewReader(body))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	sdp, err := h.extractSDP(c)
	if err != nil {
		t.Fatalf("extractSDP should not error: %v", err)
	}
	if sdp != "test-sdp" {
		t.Errorf("expected SDP 'test-sdp', got %s", sdp)
	}
}

func TestHandler_extractSDP_ApplicationSDP(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	h := NewHandler(mgr, nil, nil, nil, nil)

	body := "v=0\r\no=- 123 456 IN IP4 127.0.0.1\r\n"
	req := httptest.NewRequest(http.MethodPost, "/calls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/sdp")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	sdp, err := h.extractSDP(c)
	if err != nil {
		t.Fatalf("extractSDP should not error: %v", err)
	}
	if sdp != body {
		t.Errorf("expected raw SDP body, got %s", sdp)
	}
}

func TestHandler_extractSDP_Multipart(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	h := NewHandler(mgr, nil, nil, nil, nil)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormField("sdp")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte("multipart-sdp-content"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/calls", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	sdp, err := h.extractSDP(c)
	if err != nil {
		t.Fatalf("extractSDP should not error: %v", err)
	}
	if sdp != "multipart-sdp-content" {
		t.Errorf("expected multipart SDP, got %s", sdp)
	}
}

func TestHandler_extractSDP_MultipartNoSDP(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	h := NewHandler(mgr, nil, nil, nil, nil)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormField("other")
	part.Write([]byte("not-sdp"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/calls", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_, err := h.extractSDP(c)
	if err == nil {
		t.Error("extractSDP should error when sdp field not found")
	}
}

func TestHandler_extractSDP_UnsupportedContentType(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	h := NewHandler(mgr, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/calls", strings.NewReader("data"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_, err := h.extractSDP(c)
	if err == nil {
		t.Error("extractSDP should error for unsupported content type")
	}
}

func TestHandler_extractSDP_InvalidJSON(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	h := NewHandler(mgr, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/calls", strings.NewReader("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_, err := h.extractSDP(c)
	if err == nil {
		t.Error("extractSDP should error for invalid JSON")
	}
}

func TestHandler_HandleICECandidate_Unauthorized(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	auth := func(r *http.Request) (*transport.UserProfile, error) {
		return nil, echo.NewHTTPError(http.StatusUnauthorized)
	}
	h := NewHandler(mgr, nil, auth, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/calls/session-123", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues("session-123")

	err := h.HandleICECandidate(c)
	if err == nil {
		t.Error("expected unauthorized error")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 error, got %v", err)
	}
}

func TestHandler_HandleICECandidate_MissingSessionID(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	auth := func(r *http.Request) (*transport.UserProfile, error) {
		return &transport.UserProfile{UserID: "user-123"}, nil
	}
	h := NewHandler(mgr, nil, auth, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/calls/", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.HandleICECandidate(c)
	if err == nil {
		t.Error("expected bad request error")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 error, got %v", err)
	}
}

func TestHandler_HandleICECandidate_SessionNotFound(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	auth := func(r *http.Request) (*transport.UserProfile, error) {
		return &transport.UserProfile{UserID: "user-123"}, nil
	}
	h := NewHandler(mgr, nil, auth, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/calls/nonexistent", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues("nonexistent")

	err := h.HandleICECandidate(c)
	if err == nil {
		t.Error("expected not found error")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusNotFound {
		t.Errorf("expected 404 error, got %v", err)
	}
}

func TestHandler_HandleICEStream_Unauthorized(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	auth := func(r *http.Request) (*transport.UserProfile, error) {
		return nil, echo.NewHTTPError(http.StatusUnauthorized)
	}
	h := NewHandler(mgr, nil, auth, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/calls/session-123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues("session-123")

	err := h.HandleICEStream(c)
	if err == nil {
		t.Error("expected unauthorized error")
	}
}

func TestHandler_HandleICEStream_SessionNotFound(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	auth := func(r *http.Request) (*transport.UserProfile, error) {
		return &transport.UserProfile{UserID: "user-123"}, nil
	}
	h := NewHandler(mgr, nil, auth, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/calls/nonexistent", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues("nonexistent")

	err := h.HandleICEStream(c)
	if err == nil {
		t.Error("expected not found error")
	}
}

func TestHandler_HandleOffer_Unauthorized(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	auth := func(r *http.Request) (*transport.UserProfile, error) {
		return nil, echo.NewHTTPError(http.StatusUnauthorized)
	}
	h := NewHandler(mgr, nil, auth, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/calls", strings.NewReader(`{"sdp":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.HandleOffer(c)
	if err == nil {
		t.Error("expected unauthorized error")
	}
}

func TestHandler_HandleOffer_MissingSDP(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	auth := func(r *http.Request) (*transport.UserProfile, error) {
		return &transport.UserProfile{UserID: "user-123"}, nil
	}
	h := NewHandler(mgr, nil, auth, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/calls", strings.NewReader(`{"sdp":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.HandleOffer(c)
	if err == nil {
		t.Error("expected bad request error for missing SDP")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 error, got %v", err)
	}
}

func TestHandler_RegisterRoutes(t *testing.T) {
	e := echo.New()
	mgr, _ := NewManager(Config{})
	h := NewHandler(mgr, nil, nil, nil, nil)
	g := e.Group("/v1/voice")
	h.RegisterRoutes(g)

	routes := e.Routes()
	expectedPaths := map[string]bool{
		"/v1/voice/calls":             false,
		"/v1/voice/calls/:session_id": false,
		"/v1/voice/ice-servers":       false,
	}

	for _, r := range routes {
		if _, exists := expectedPaths[r.Path]; exists {
			expectedPaths[r.Path] = true
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("route %s not registered", path)
		}
	}
}

func TestOfferRequest_JSON(t *testing.T) {
	req := OfferRequest{SDP: "v=0\r\n..."}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var parsed OfferRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if parsed.SDP != req.SDP {
		t.Errorf("expected SDP %s, got %s", req.SDP, parsed.SDP)
	}
}

func TestOfferResponse_JSON(t *testing.T) {
	resp := OfferResponse{
		SessionID: "session-123",
		SDP:       "v=0\r\n...",
		ICEServers: []ICEServer{
			{URLs: []string{"stun:stun.example.com"}},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var parsed OfferResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if parsed.SessionID != resp.SessionID {
		t.Errorf("expected SessionID %s, got %s", resp.SessionID, parsed.SessionID)
	}
}

func TestICECandidateRequest_JSON(t *testing.T) {
	sdpMid := "0"
	var sdpMLineIndex uint16 = 0
	req := ICECandidateRequest{
		Candidate:     "candidate:123",
		SDPMid:        &sdpMid,
		SDPMLineIndex: &sdpMLineIndex,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var parsed ICECandidateRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if parsed.Candidate != req.Candidate {
		t.Errorf("expected Candidate %s, got %s", req.Candidate, parsed.Candidate)
	}
}

func TestICEServer_JSON(t *testing.T) {
	server := ICEServer{
		URLs:       []string{"turn:turn.example.com"},
		Username:   "user",
		Credential: "pass",
	}
	data, err := json.Marshal(server)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var parsed ICEServer
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if parsed.Username != server.Username {
		t.Errorf("expected Username %s, got %s", server.Username, parsed.Username)
	}
}

func TestICEServersResponse_JSON(t *testing.T) {
	resp := ICEServersResponse{
		ICEServers: []ICEServer{
			{URLs: []string{"stun:stun.example.com"}},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var parsed ICEServersResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(parsed.ICEServers) != 1 {
		t.Errorf("expected 1 ICE server, got %d", len(parsed.ICEServers))
	}
}

func TestHandlerConfig(t *testing.T) {
	mgr, _ := NewManager(Config{})
	cfg := HandlerConfig{
		Manager: mgr,
		Log:     slog.Default(),
	}
	if cfg.Manager != mgr {
		t.Error("config should have manager")
	}
	if cfg.Log == nil {
		t.Error("config should have logger")
	}
}
