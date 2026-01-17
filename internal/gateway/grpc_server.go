package gateway

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/eleven-am/voice-backend/internal/agent"
	pb "github.com/eleven-am/voice-backend/internal/gateway/proto"
	"github.com/eleven-am/voice-backend/internal/shared"
)

type GRPCServer struct {
	pb.UnimplementedVoiceGatewayServer
	bridge     *Bridge
	agentStore *agent.Store
	logger     *slog.Logger
	sessions   map[string]*voiceSession
	mu         sync.RWMutex
}

func NewGRPCServer(bridge *Bridge, agentStore *agent.Store, logger *slog.Logger) *GRPCServer {
	server := &GRPCServer{
		bridge:     bridge,
		agentStore: agentStore,
		logger:     logger.With("component", "grpc_server"),
		sessions:   make(map[string]*voiceSession),
	}

	bridge.SetResponseHandler(server.handleResponse)
	return server
}

func (s *GRPCServer) Session(stream pb.VoiceGateway_SessionServer) error {
	var session *voiceSession

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			if session != nil {
				s.cleanupSession(session)
			}
			return nil
		}
		if err != nil {
			s.logger.Error("receive error", "error", err)
			if session != nil {
				s.cleanupSession(session)
			}
			return err
		}

		switch payload := msg.Payload.(type) {
		case *pb.ClientMessage_SessionStart:
			session, err = s.handleSessionStart(stream, msg.RequestId, payload.SessionStart)
			if err != nil {
				return err
			}

		case *pb.ClientMessage_Utterance:
			if session == nil {
				s.sendError(stream, msg.RequestId, pb.ErrorCode_ERROR_CODE_INVALID_REQUEST, "session not started")
				continue
			}
			s.handleUtterance(session, msg.RequestId, payload.Utterance)

		case *pb.ClientMessage_SessionEnd:
			if session != nil {
				s.cleanupSession(session)
				session = nil
			}
		}
	}
}

type voiceSession struct {
	id       string
	userID   string
	roomID   string
	stream   pb.VoiceGateway_SessionServer
	agents   []*agent.Agent
	mu       sync.Mutex
	canceled bool
}

func (s *GRPCServer) handleSessionStart(stream pb.VoiceGateway_SessionServer, requestID string, start *pb.SessionStart) (*voiceSession, error) {
	sessionID := shared.NewID("sess_")

	agents, err := s.agentStore.GetInstalledAgents(context.Background(), start.UserId)
	if err != nil {
		s.logger.Error("get installed agents", "error", err)
		s.sendError(stream, requestID, pb.ErrorCode_ERROR_CODE_INTERNAL, "failed to get agents")
		return nil, err
	}

	session := &voiceSession{
		id:     sessionID,
		userID: start.UserId,
		roomID: start.RoomId,
		stream: stream,
		agents: agents,
	}

	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	s.bridge.SubscribeToSession(sessionID)

	var pbAgents []*pb.Agent
	for _, a := range agents {
		online := s.isAgentOnline(a.ID)
		pbAgents = append(pbAgents, &pb.Agent{
			Id:           a.ID,
			Name:         a.Name,
			Description:  a.Description,
			Online:       online,
			Capabilities: []string{},
		})
	}

	err = stream.Send(&pb.ServerMessage{
		RequestId: requestID,
		Timestamp: time.Now().UnixMilli(),
		Payload: &pb.ServerMessage_SessionConfig{
			SessionConfig: &pb.SessionConfig{
				Agents: pbAgents,
			},
		},
	})

	if err != nil {
		s.logger.Error("send session config", "error", err)
		return nil, err
	}

	s.logger.Info("session started",
		"session_id", sessionID,
		"user_id", start.UserId,
		"room_id", start.RoomId,
		"agents", len(agents))

	return session, nil
}

func (s *GRPCServer) handleUtterance(session *voiceSession, requestID string, utterance *pb.Utterance) {
	agentID := utterance.AgentId
	if agentID == "" {
		s.sendError(session.stream, requestID, pb.ErrorCode_ERROR_CODE_INVALID_REQUEST, "missing agent_id")
		return
	}

	if !s.isAgentOnline(agentID) {
		s.sendError(session.stream, requestID, pb.ErrorCode_ERROR_CODE_AGENT_UNAVAILABLE, "agent is offline")
		return
	}

	msg := &GatewayMessage{
		Type:      MessageTypeUtterance,
		RequestID: requestID,
		SessionID: session.id,
		AgentID:   agentID,
		UserID:    session.userID,
		RoomID:    session.roomID,
		Timestamp: time.Now(),
		Payload: UtterancePayload{
			Text:    utterance.Text,
			IsFinal: utterance.IsFinal,
		},
	}

	if err := s.bridge.PublishUtterance(context.Background(), msg); err != nil {
		s.logger.Error("publish utterance", "error", err)
		s.sendError(session.stream, requestID, pb.ErrorCode_ERROR_CODE_INTERNAL, "failed to route utterance")
	}
}

func (s *GRPCServer) handleResponse(sessionID string, msg *GatewayMessage) {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()

	if !ok || session == nil {
		s.logger.Warn("response for unknown session", "session_id", sessionID)
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.canceled {
		return
	}

	switch msg.Type {
	case MessageTypeResponse:
		payload, ok := msg.Payload.(map[string]interface{})
		if !ok {
			s.logger.Error("invalid response payload type")
			return
		}

		text, _ := payload["text"].(string)
		fromAgent, _ := payload["from_agent"].(string)

		err := session.stream.Send(&pb.ServerMessage{
			RequestId: msg.RequestID,
			Timestamp: time.Now().UnixMilli(),
			Payload: &pb.ServerMessage_Response{
				Response: &pb.Response{
					Text:      text,
					FromAgent: fromAgent,
				},
			},
		})
		if err != nil {
			s.logger.Error("send response", "error", err)
		}

	case MessageTypeAgentStatus:
		payload, ok := msg.Payload.(map[string]interface{})
		if !ok {
			return
		}

		agentID, _ := payload["agent_id"].(string)
		online, _ := payload["online"].(bool)

		err := session.stream.Send(&pb.ServerMessage{
			RequestId: msg.RequestID,
			Timestamp: time.Now().UnixMilli(),
			Payload: &pb.ServerMessage_AgentStatus{
				AgentStatus: &pb.AgentStatusUpdate{
					AgentId: agentID,
					Online:  online,
				},
			},
		})
		if err != nil {
			s.logger.Error("send agent status", "error", err)
		}

	case MessageTypeError:
		payload, ok := msg.Payload.(map[string]interface{})
		if !ok {
			return
		}

		message, _ := payload["message"].(string)
		s.sendError(session.stream, msg.RequestID, pb.ErrorCode_ERROR_CODE_INTERNAL, message)
	}
}

func (s *GRPCServer) cleanupSession(session *voiceSession) {
	session.mu.Lock()
	session.canceled = true
	session.mu.Unlock()

	s.mu.Lock()
	delete(s.sessions, session.id)
	s.mu.Unlock()

	s.bridge.UnsubscribeFromSession(session.id)
	s.logger.Info("session ended", "session_id", session.id)
}

func (s *GRPCServer) isAgentOnline(agentID string) bool {
	_, ok := s.bridge.GetAgent(agentID)
	return ok
}

func (s *GRPCServer) sendError(stream pb.VoiceGateway_SessionServer, requestID string, code pb.ErrorCode, message string) {
	err := stream.Send(&pb.ServerMessage{
		RequestId: requestID,
		Timestamp: time.Now().UnixMilli(),
		Payload: &pb.ServerMessage_Error{
			Error: &pb.Error{
				Code:    code,
				Message: message,
			},
		},
	})
	if err != nil {
		s.logger.Error("send error", "error", err)
	}
}
