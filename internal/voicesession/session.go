package voicesession

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/eleven-am/voice-backend/internal/router"
	"github.com/eleven-am/voice-backend/internal/synthesis"
	"github.com/eleven-am/voice-backend/internal/transcription"
	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/google/uuid"
)

type VoiceSession struct {
	sessionID string
	userID    string
	agentID   string

	conn   transport.Connection
	stt    transcription.Transcriber
	tts    synthesis.Synthesizer
	bridge transport.Bridge

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	log    *slog.Logger

	ttsCancelCh chan struct{}
	ttsMu       sync.Mutex

	speechCtrl   *SpeechController
	router       router.Router
	arbiter      *Arbiter
	agents       []router.AgentInfo
	activeAgents []string
	agentMu      sync.Mutex
}

type Config struct {
	AgentID       string
	UserID        string
	STTConfig     transcription.Config
	STTOptions    transcription.SessionOptions
	TTSConfig     synthesis.Config
	VoiceID       string
	TTSModelID    string
	TTSLanguage   string
	TTSSpeed      float32
	TTSFormat     string
	BargeInPolicy BargeInPolicy
	Agents        []router.AgentInfo
	Router        router.Router
}

func New(conn transport.Connection, bridge transport.Bridge, cfg Config, log *slog.Logger) (*VoiceSession, error) {
	if log == nil {
		log = slog.Default()
	}

	sessionID := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())

	speechCtrl := NewSpeechController(cfg.BargeInPolicy)

	var rtr router.Router
	if cfg.Router != nil {
		rtr = cfg.Router
	} else {
		rtr = router.NewSmartRouter()
	}

	s := &VoiceSession{
		sessionID:   sessionID,
		userID:      cfg.UserID,
		agentID:     cfg.AgentID,
		conn:        conn,
		bridge:      bridge,
		ctx:         ctx,
		cancel:      cancel,
		log:         log.With("session_id", sessionID),
		ttsCancelCh: make(chan struct{}),
		speechCtrl:  speechCtrl,
		router:      rtr,
		arbiter:     NewArbiter(),
		agents:      cfg.Agents,
	}

	sttClient, err := transcription.New(cfg.STTConfig, cfg.STTOptions, transcription.Callbacks{
		OnReady:       s.onSTTReady,
		OnSpeechStart: s.onSpeechStart,
		OnSpeechEnd:   s.onSpeechEnd,
		OnTranscript:  s.onTranscript,
		OnError:       s.onSTTError,
	})
	if err != nil {
		cancel()
		return nil, err
	}
	s.stt = sttClient

	ttsClient, err := synthesis.New(cfg.TTSConfig)
	if err != nil {
		sttClient.Close()
		cancel()
		return nil, err
	}
	s.tts = ttsClient

	bridge.SetResponseHandler(s.onAgentResponse)
	bridge.SubscribeToSession(sessionID)

	return s, nil
}

func (s *VoiceSession) Start() {
	s.wg.Add(1)
	go s.audioInLoop()
}

func (s *VoiceSession) SessionID() string {
	return s.sessionID
}

func (s *VoiceSession) UserID() string {
	return s.userID
}

func (s *VoiceSession) audioInLoop() {
	defer s.wg.Done()

	audioIn := s.conn.AudioIn()
	for {
		select {
		case <-s.ctx.Done():
			return
		case opusData, ok := <-audioIn:
			if !ok {
				return
			}
			if err := s.stt.SendOpusFrame(opusData, 48000, 1); err != nil {
				s.log.Error("failed to send opus frame to STT", "error", err)
			}
		}
	}
}

func (s *VoiceSession) onSTTReady() {
	s.log.Info("STT ready")
}

func (s *VoiceSession) onSpeechStart() {
	s.log.Debug("speech started")
	actions := s.speechCtrl.OnUserSpeechStart(time.Now())
	s.executeActions(actions)
}

func (s *VoiceSession) onSpeechEnd() {
	s.log.Debug("speech ended")
	actions := s.speechCtrl.OnUserSpeechEnd(time.Now())
	s.executeActions(actions)
}

func (s *VoiceSession) executeActions(actions []Action) {
	for _, action := range actions {
		switch action.Type {
		case ActionStopTTS:
			s.stopTTS()
		case ActionCancelAgent:
			s.cancelActiveAgents(action.Reason)
		case ActionPauseOutput:
			if ctrl, ok := s.conn.(transport.OutputController); ok {
				ctrl.PauseOutput()
			}
		case ActionResumeOutput:
			if ctrl, ok := s.conn.(transport.OutputController); ok {
				ctrl.ResumeOutput()
			}
		case ActionEndUtterance:
			s.log.Debug("end utterance action triggered")
		}
	}
}

func (s *VoiceSession) cancelActiveAgents(reason string) {
	s.agentMu.Lock()
	agents := s.activeAgents
	s.activeAgents = nil
	s.agentMu.Unlock()

	for _, agentID := range agents {
		if err := s.bridge.PublishCancellation(s.ctx, agentID, s.sessionID, reason); err != nil {
			s.log.Error("failed to cancel agent", "agent_id", agentID, "error", err)
		}
	}

	s.arbiter.Reset()
}

func (s *VoiceSession) onTranscript(evt transcription.TranscriptEvent) {
	s.log.Info("transcript received", "text", evt.Text, "partial", evt.IsPartial)

	if evt.IsPartial {
		s.sendPartialTranscript(evt)
		return
	}

	if evt.Text == "" {
		return
	}

	msg := &transport.AgentMessage{
		Type:      transport.MessageTypeUtterance,
		RequestID: uuid.New().String(),
		SessionID: s.sessionID,
		AgentID:   s.agentID,
		UserID:    s.userID,
		Timestamp: time.Now(),
		Payload: transport.UtterancePayload{
			Text:    evt.Text,
			IsFinal: true,
		},
	}

	if len(s.agents) > 0 {
		targetAgents := s.router.Route(s.ctx, evt.Text, s.agents)
		if len(targetAgents) == 0 {
			targetAgents = []string{s.agentID}
		}

		s.agentMu.Lock()
		s.activeAgents = targetAgents
		s.agentMu.Unlock()

		s.arbiter.Start(targetAgents)

		if err := s.bridge.PublishToAgents(s.ctx, targetAgents, msg); err != nil {
			s.log.Error("failed to publish to agents", "error", err)
		}
	} else {
		if err := s.bridge.PublishUtterance(s.ctx, msg); err != nil {
			s.log.Error("failed to publish utterance", "error", err)
		}
	}
}

func (s *VoiceSession) sendPartialTranscript(evt transcription.TranscriptEvent) {
	partialEvt := transport.ServerEvent{
		Type: string(transport.MessageTypeTranscript),
		Payload: transport.PartialTranscriptEvent{
			Text:      evt.Text,
			IsFinal:   !evt.IsPartial,
			Timestamp: time.Now(),
		},
	}
	if err := s.conn.Send(s.ctx, partialEvt); err != nil {
		s.log.Error("failed to send partial transcript", "error", err)
	}
}

func (s *VoiceSession) onSTTError(err error) {
	s.log.Error("STT error", "error", err)
}

func (s *VoiceSession) onAgentResponse(sessionID string, msg *transport.AgentMessage) {
	if sessionID != s.sessionID {
		return
	}

	if msg.Type != transport.MessageTypeResponse {
		return
	}

	payload, ok := msg.Payload.(map[string]any)
	if !ok {
		return
	}

	text, _ := payload["text"].(string)
	if text == "" {
		return
	}

	agentID := msg.AgentID
	winner, isNew := s.arbiter.Decide(agentID)

	if winner != agentID {
		s.log.Debug("agent response ignored, not winner",
			"agent_id", agentID,
			"winner", winner)
		return
	}

	if isNew {
		losers := s.arbiter.Losers()
		for _, loserID := range losers {
			if err := s.bridge.PublishCancellation(s.ctx, loserID, s.sessionID, "lost_arbitration"); err != nil {
				s.log.Error("failed to cancel loser agent",
					"agent_id", loserID,
					"error", err)
			}
		}
	}

	s.log.Info("agent response received",
		"agent_id", agentID,
		"text", text,
		"is_winner", isNew)

	s.ttsMu.Lock()
	cancelCh := make(chan struct{})
	s.ttsCancelCh = cancelCh
	s.ttsMu.Unlock()

	go s.synthesizeResponse(text, cancelCh)
}

func (s *VoiceSession) synthesizeResponse(text string, cancelCh <-chan struct{}) {
	req := synthesis.Request{
		Text:   text,
		Cancel: cancelCh,
	}

	s.speechCtrl.OnTTSAudioStart()

	cb := synthesis.Callbacks{
		OnReady: func(sampleRate uint32, voiceID string) {
			s.log.Debug("TTS ready", "sample_rate", sampleRate, "voice_id", voiceID)
		},
		OnAudio: func(data []byte, format string, sampleRate uint32) {
			chunk := transport.AudioChunk{
				Data:       data,
				Format:     format,
				SampleRate: sampleRate,
			}
			if err := s.conn.SendAudio(s.ctx, chunk); err != nil {
				s.log.Error("failed to send audio", "error", err)
			}
		},
		OnDone: func(audioDurationMs, processingDurationMs, textLength uint64) {
			s.log.Debug("TTS done", "audio_duration_ms", audioDurationMs)
			s.speechCtrl.OnTTSAudioEnd()
			s.arbiter.Reset()
		},
		OnError: func(err error) {
			s.log.Error("TTS error", "error", err)
			s.speechCtrl.OnTTSAudioEnd()
		},
	}

	if err := s.tts.Synthesize(s.ctx, req, cb); err != nil {
		s.log.Error("TTS synthesize failed", "error", err)
		s.speechCtrl.OnTTSAudioEnd()
	}
}

func (s *VoiceSession) stopTTS() {
	s.ttsMu.Lock()
	if s.ttsCancelCh != nil {
		close(s.ttsCancelCh)
		s.ttsCancelCh = make(chan struct{})
	}
	s.ttsMu.Unlock()

	if ctrl, ok := s.conn.(transport.OutputController); ok {
		ctrl.StopTTS()
	}
	s.conn.FlushAudioQueue()
}

func (s *VoiceSession) Close() error {
	s.cancel()

	s.bridge.UnsubscribeFromSession(s.sessionID)

	s.wg.Wait()

	if err := s.stt.Close(); err != nil {
		s.log.Error("failed to close STT", "error", err)
	}

	if err := s.tts.Close(); err != nil {
		s.log.Error("failed to close TTS", "error", err)
	}

	return s.conn.Close()
}
