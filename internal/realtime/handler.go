package realtime

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"

	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/labstack/echo/v4"
	"github.com/pion/webrtc/v4"
)

type Handler struct {
	manager *Manager
	starter transport.SessionStarter
	auth    transport.AuthFunc
	log     *slog.Logger
}

type HandlerConfig struct {
	Manager *Manager
	Starter transport.SessionStarter
	Auth    transport.AuthFunc
	Log     *slog.Logger
}

func NewHandler(mgr *Manager, starter transport.SessionStarter, auth transport.AuthFunc, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{
		manager: mgr,
		starter: starter,
		auth:    auth,
		log:     log,
	}
}

type OfferRequest struct {
	SDP     string                   `json:"sdp"`
	Session *transport.SessionConfig `json:"session,omitempty"`
}

type OfferResponse struct {
	SessionID  string      `json:"session_id"`
	SDP        string      `json:"sdp"`
	ICEServers []ICEServer `json:"ice_servers,omitempty"`
}

type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type ICECandidateRequest struct {
	Candidate     string  `json:"candidate"`
	SDPMid        *string `json:"sdpMid,omitempty"`
	SDPMLineIndex *uint16 `json:"sdpMLineIndex,omitempty"`
}

type ICEServersResponse struct {
	ICEServers []ICEServer `json:"ice_servers"`
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.POST("/calls", h.HandleOffer)
	g.POST("/calls/:session_id", h.HandleICECandidate)
	g.GET("/calls/:session_id", h.HandleICEStream)
	g.GET("/ice-servers", h.HandleICEServers)
}

func (h *Handler) HandleICECandidate(c echo.Context) error {
	profile, err := h.auth(c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	sessionID := c.Param("session_id")
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing session id")
	}

	session, ok := h.manager.GetSession(sessionID)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}

	if session.UserID() != profile.UserID {
		return echo.NewHTTPError(http.StatusForbidden, "session access denied")
	}

	var req ICECandidateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	candidate := webrtc.ICECandidateInit{
		Candidate:     req.Candidate,
		SDPMid:        req.SDPMid,
		SDPMLineIndex: req.SDPMLineIndex,
	}

	conn := session.Conn()
	if err := conn.peer.AddICECandidate(candidate); err != nil {
		h.log.Error("failed to add ICE candidate", "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, "failed to add candidate")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) HandleICEStream(c echo.Context) error {
	profile, err := h.auth(c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	sessionID := c.Param("session_id")
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing session id")
	}

	session, ok := h.manager.GetSession(sessionID)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}

	if session.UserID() != profile.UserID {
		return echo.NewHTTPError(http.StatusForbidden, "session access denied")
	}

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	ctx := c.Request().Context()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-session.Done():
			return nil
		case candidate, ok := <-session.ICECandidates():
			if !ok {
				return nil
			}

			data, err := json.Marshal(candidate)
			if err != nil {
				continue
			}

			fmt.Fprintf(c.Response(), "event: ice-candidate\ndata: %s\n\n", data)
			c.Response().Flush()
		}
	}
}

func (h *Handler) HandleICEServers(c echo.Context) error {
	servers := h.iceServersResponse()
	return c.JSON(http.StatusOK, map[string][]ICEServer{"ice_servers": servers})
}

func (h *Handler) maxSDPSize() int64 {
	maxSize := h.manager.Config().MaxSDPSize
	if maxSize <= 0 {
		maxSize = 64 * 1024
	}
	return int64(maxSize)
}

func (h *Handler) iceServersResponse() []ICEServer {
	cfgServers := h.manager.ICEServers()
	servers := make([]ICEServer, 0, len(cfgServers))

	for _, s := range cfgServers {
		servers = append(servers, ICEServer(s))
	}

	if len(servers) == 0 {
		servers = append(servers, ICEServer{
			URLs: []string{"stun:stun.l.google.com:19302"},
		})
	}

	return servers
}

func (h *Handler) HandleOffer(c echo.Context) error {
	profile, err := h.auth(c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	clientIP := c.RealIP()
	if clientIP == "" {
		clientIP = c.Request().RemoteAddr
	}

	userCtx := &transport.UserContext{
		UserID: profile.UserID,
		IP:     clientIP,
		Name:   profile.Name,
		Email:  profile.Email,
	}

	sdp, sessionConfig, err := h.extractRequest(c)
	if err != nil {
		h.log.Error("failed to extract request", "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if sdp == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing sdp")
	}

	peer, err := h.manager.NewPeer()
	if err != nil {
		h.log.Error("failed to create peer", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create peer connection")
	}

	if err := peer.SetOffer(sdp); err != nil {
		peer.Close()
		h.log.Error("failed to set offer", "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, "failed to process offer")
	}

	conn, err := NewConn(peer, h.manager.Config(), h.log)
	if err != nil {
		peer.Close()
		h.log.Error("failed to create connection", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create connection")
	}

	peer.OnDataChannel(func(dc *webrtc.DataChannel) {
		conn.SetupDataChannel(dc)
	})

	rtcSession := h.manager.CreateSession(conn, profile.UserID)

	peer.OnICECandidate(func(cand *webrtc.ICECandidate) {
		if cand == nil {
			return
		}
		init := cand.ToJSON()
		rtcSession.SendICE(init)
		conn.SendICECandidate(init)
	})

	answer, err := peer.CreateAnswer()
	if err != nil {
		h.manager.RemoveSession(rtcSession.ID)
		h.log.Error("failed to create answer", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create answer")
	}

	if err := h.starter.Start(transport.StartRequest{
		Conn:        conn,
		UserContext: userCtx,
		Config:      sessionConfig,
	}); err != nil {
		conn.Close()
		h.manager.RemoveSession(rtcSession.ID)
		h.log.Error("session start failed", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to start session")
	}

	c.Response().Header().Set("X-Session-Id", rtcSession.ID)
	c.Response().Header().Set("Content-Type", "application/sdp")
	return c.String(http.StatusOK, answer)
}

func (h *Handler) extractRequest(c echo.Context) (string, *transport.SessionConfig, error) {
	contentType := c.Request().Header.Get("Content-Type")
	mediaType, params, _ := mime.ParseMediaType(contentType)

	switch mediaType {
	case "application/sdp":
		body, err := io.ReadAll(io.LimitReader(c.Request().Body, h.maxSDPSize()))
		if err != nil {
			return "", nil, fmt.Errorf("failed to read SDP body: %w", err)
		}
		return string(body), nil, nil

	case "multipart/form-data":
		boundary := params["boundary"]
		if boundary == "" {
			return "", nil, fmt.Errorf("missing boundary in multipart")
		}
		reader := multipart.NewReader(c.Request().Body, boundary)
		var sdp string
		var sessionConfig *transport.SessionConfig
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", nil, fmt.Errorf("failed to read multipart: %w", err)
			}
			switch part.FormName() {
			case "sdp":
				data, err := io.ReadAll(io.LimitReader(part, h.maxSDPSize()))
				if err != nil {
					return "", nil, fmt.Errorf("failed to read SDP part: %w", err)
				}
				sdp = string(data)
			case "session":
				data, err := io.ReadAll(io.LimitReader(part, h.maxSDPSize()))
				if err != nil {
					return "", nil, fmt.Errorf("failed to read session part: %w", err)
				}
				var cfg transport.SessionConfig
				if err := json.Unmarshal(data, &cfg); err != nil {
					return "", nil, fmt.Errorf("invalid session config: %w", err)
				}
				sessionConfig = &cfg
			}
		}
		if sdp == "" {
			return "", nil, fmt.Errorf("sdp field not found in multipart")
		}
		return sdp, sessionConfig, nil

	case "application/json", "":
		body, err := io.ReadAll(io.LimitReader(c.Request().Body, h.maxSDPSize()))
		if err != nil {
			return "", nil, fmt.Errorf("failed to read request body: %w", err)
		}
		var req OfferRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return "", nil, fmt.Errorf("invalid JSON body: %w", err)
		}
		return req.SDP, req.Session, nil

	default:
		return "", nil, fmt.Errorf("unsupported content type: %s", contentType)
	}
}
