package voicesession

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/eleven-am/voice-backend/internal/router"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/synthesis"
	"github.com/eleven-am/voice-backend/internal/transcription"
	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/eleven-am/voice-backend/internal/vision"
	"github.com/google/uuid"
)

type VoiceSession struct {
	sessionID string
	userCtx   *transport.UserContext

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
	voiceID     string
	ttsSpeed    float32

	speechCtrl *SpeechController
	router       router.Router
	arbiter      *Arbiter
	agents       []router.AgentInfo
	activeAgents []string
	agentMu      sync.Mutex

	visionAnalyzer *vision.Analyzer
	frameCapturer  *vision.FrameCapturer

	sentenceBuffer *SentenceBuffer
	ttsQueue       *TTSQueue
	ttsBridge      *TTSBridge
}

type Config struct {
	UserContext    *transport.UserContext
	STTConfig      transcription.Config
	STTOptions     transcription.SessionOptions
	TTSConfig      synthesis.Config
	VoiceID        string
	TTSSpeed       float32
	BargeInPolicy  BargeInPolicy
	Agents         []router.AgentInfo
	Router         router.Router
	VisionAnalyzer *vision.Analyzer
	VisionStore    *vision.Store
}

type VideoConnection interface {
	OnVideo(fn func([]byte, string))
	HasVideo() bool
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
		sessionID:      sessionID,
		userCtx:        cfg.UserContext,
		conn:           conn,
		bridge:         bridge,
		ctx:            ctx,
		cancel:         cancel,
		log:            log.With("session_id", sessionID),
		ttsCancelCh:    make(chan struct{}),
		voiceID:        cfg.VoiceID,
		ttsSpeed:       cfg.TTSSpeed,
		speechCtrl:     speechCtrl,
		router:         rtr,
		arbiter:        NewArbiter(),
		agents:         cfg.Agents,
		visionAnalyzer: cfg.VisionAnalyzer,
	}

	if cfg.VisionAnalyzer != nil && cfg.VisionStore != nil {
		if videoConn, ok := conn.(VideoConnection); ok {
			capturer := vision.NewFrameCapturer(vision.CapturerConfig{
				SessionID:   sessionID,
				Store:       cfg.VisionStore,
				CaptureRate: 2 * time.Second,
				Logger:      log,
			})
			s.frameCapturer = capturer

			videoConn.OnVideo(func(payload []byte, mimeType string) {
				capturer.HandleRTPPacket(payload, mimeType)
			})
		}
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

	ttsBridge := NewTTSBridge(TTSBridgeConfig{
		Synth:   ttsClient,
		VoiceID: cfg.VoiceID,
		Speed:   cfg.TTSSpeed,
		Format:  "opus",
		Conn:    conn,
		Log:     log,
	})
	s.ttsBridge = ttsBridge

	ttsQueue := NewTTSQueue(ttsBridge, log)
	ttsQueue.SetCallbacks(
		func() {
			log.Debug("TTS audio start callback", "prev_state", speechCtrl.State())
			speechCtrl.OnTTSAudioStart()
			log.Debug("TTS audio start callback done", "new_state", speechCtrl.State())
		},
		func() {
			log.Debug("TTS audio end callback", "prev_state", speechCtrl.State())
			speechCtrl.OnTTSAudioEnd()
			log.Debug("TTS audio end callback done", "new_state", speechCtrl.State())
		},
	)
	s.ttsQueue = ttsQueue
	s.sentenceBuffer = NewSentenceBuffer(log)

	bridge.SetResponseHandler(s.onAgentResponse)
	if err := bridge.SubscribeToSession(sessionID); err != nil {
		ttsClient.Close()
		sttClient.Close()
		cancel()
		return nil, err
	}

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
	if s.userCtx == nil {
		return ""
	}
	return s.userCtx.UserID
}

func (s *VoiceSession) AgentCount() int {
	return len(s.agents)
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
	state := s.speechCtrl.State()
	s.log.Debug("speech started", "controller_state", state)
	s.sendEvent(transport.MessageTypeSpeechStart, nil)
	actions := s.speechCtrl.OnUserSpeechStart(time.Now())
	s.log.Debug("speech actions", "actions", len(actions), "new_state", s.speechCtrl.State())
	s.executeActions(actions)

	if s.visionAnalyzer != nil {
		s.visionAnalyzer.StartAnalysis(s.ctx, s.sessionID)
	}
}

func (s *VoiceSession) onSpeechEnd() {
	s.log.Debug("speech ended")
	s.sendEvent(transport.MessageTypeSpeechEnd, nil)
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
		s.sendEvent(transport.MessageTypeInterrupt, transport.AgentCancelledEvent{
			AgentID: agentID,
			Reason:  reason,
		})
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

	basePayload := transport.UtterancePayload{
		Text:    evt.Text,
		IsFinal: true,
	}

	if s.visionAnalyzer != nil {
		visionResult := s.visionAnalyzer.GetResult(500 * time.Millisecond)
		if visionResult != nil && visionResult.Available {
			basePayload.Vision = &transport.VisionContext{
				Description: visionResult.Description,
				Timestamp:   visionResult.Timestamp,
				Available:   true,
			}
		}
		s.visionAnalyzer.Reset()
	}

	requestID := uuid.New().String()
	timestamp := time.Now()

	if len(s.agents) > 0 {
		targetAgents := s.router.Route(s.ctx, evt.Text, s.agents)
		if len(targetAgents) == 0 {
			targetAgents = s.allAgentIDs()
		}

		s.agentMu.Lock()
		s.activeAgents = targetAgents
		s.agentMu.Unlock()

		s.arbiter.Start(targetAgents)

		groups := s.groupByScopes(targetAgents)
		for scopeKey, agentIDs := range groups {
			scopes := s.parseScopeKey(scopeKey)
			scopedPayload := s.buildPayload(basePayload, scopes)

			msg := &transport.AgentMessage{
				Type:      transport.MessageTypeUtterance,
				RequestID: requestID,
				SessionID: s.sessionID,
				UserID:    s.UserID(),
				Timestamp: timestamp,
				Payload:   scopedPayload,
			}

			if err := s.bridge.PublishToAgents(s.ctx, agentIDs, msg); err != nil {
				s.log.Error("failed to publish to agents", "scopes", scopeKey, "error", err)
			}
		}
	} else {
		msg := &transport.AgentMessage{
			Type:      transport.MessageTypeUtterance,
			RequestID: requestID,
			SessionID: s.sessionID,
			UserID:    s.UserID(),
			Timestamp: timestamp,
			Payload:   basePayload,
		}
		if err := s.bridge.PublishUtterance(s.ctx, msg); err != nil {
			s.log.Error("failed to publish utterance", "error", err)
		}
	}
}

func (s *VoiceSession) allAgentIDs() []string {
	ids := make([]string, len(s.agents))
	for i, a := range s.agents {
		ids[i] = a.ID
	}
	return ids
}

func (s *VoiceSession) getAgentScopes(agentID string) []string {
	for _, a := range s.agents {
		if a.ID == agentID {
			return a.GrantedScopes
		}
	}
	return nil
}

func (s *VoiceSession) groupByScopes(agentIDs []string) map[string][]string {
	groups := make(map[string][]string)
	for _, id := range agentIDs {
		scopes := s.getAgentScopes(id)
		relevant := s.relevantScopes(scopes)
		slices.Sort(relevant)
		key := s.scopeKey(relevant)
		groups[key] = append(groups[key], id)
	}
	return groups
}

func (s *VoiceSession) relevantScopes(scopes []string) []string {
	relevant := []string{
		shared.ScopeProfile.String(),
		shared.ScopeEmail.String(),
		shared.ScopeLocation.String(),
		shared.ScopeVision.String(),
	}
	var result []string
	for _, scope := range scopes {
		if slices.Contains(relevant, scope) {
			result = append(result, scope)
		}
	}
	return result
}

func (s *VoiceSession) scopeKey(scopes []string) string {
	if len(scopes) == 0 {
		return ""
	}
	key := ""
	for i, scope := range scopes {
		if i > 0 {
			key += ","
		}
		key += scope
	}
	return key
}

func (s *VoiceSession) parseScopeKey(key string) []string {
	if key == "" {
		return nil
	}
	var scopes []string
	start := 0
	for i := 0; i <= len(key); i++ {
		if i == len(key) || key[i] == ',' {
			scopes = append(scopes, key[start:i])
			start = i + 1
		}
	}
	return scopes
}

func (s *VoiceSession) buildPayload(base transport.UtterancePayload, scopes []string) transport.UtterancePayload {
	payload := transport.UtterancePayload{
		Text:    base.Text,
		IsFinal: base.IsFinal,
	}

	if slices.Contains(scopes, shared.ScopeVision.String()) && base.Vision != nil {
		payload.Vision = base.Vision
	}

	if s.userCtx == nil {
		return payload
	}

	var user *transport.UserInfo
	if slices.Contains(scopes, shared.ScopeProfile.String()) && s.userCtx.Name != "" {
		if user == nil {
			user = &transport.UserInfo{}
		}
		user.Name = s.userCtx.Name
	}
	if slices.Contains(scopes, shared.ScopeEmail.String()) && s.userCtx.Email != "" {
		if user == nil {
			user = &transport.UserInfo{}
		}
		user.Email = s.userCtx.Email
	}
	if slices.Contains(scopes, shared.ScopeLocation.String()) && s.userCtx.IP != "" {
		if user == nil {
			user = &transport.UserInfo{}
		}
		user.IP = s.userCtx.IP
	}
	payload.User = user

	return payload
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
	s.sendEvent(transport.MessageTypeError, map[string]string{
		"source":  "stt",
		"message": err.Error(),
	})
}

func (s *VoiceSession) onAgentResponse(sessionID string, msg *transport.AgentMessage) {
	if sessionID != s.sessionID {
		return
	}

	s.log.Debug("agent message received",
		"agent_id", msg.AgentID,
		"type", msg.Type,
		"request_id", msg.RequestID)

	if msg.Type == transport.MessageTypeFrameRequest {
		s.handleFrameRequest(msg)
		return
	}

	agentID := msg.AgentID

	switch msg.Type {
	case transport.MessageTypeResponseDelta:
		s.handleResponseDelta(agentID, msg)
	case transport.MessageTypeResponseDone:
		s.handleResponseDone(agentID, msg)
	case transport.MessageTypeResponse:
		s.handleCompleteResponse(agentID, msg)
	}
}

func (s *VoiceSession) handleResponseDelta(agentID string, msg *transport.AgentMessage) {
	delta := s.extractDelta(msg.Payload)
	if delta == "" {
		s.log.Debug("response delta empty", "agent_id", agentID)
		return
	}

	s.log.Debug("response delta", "agent_id", agentID, "len", len(delta), "text", delta)

	sentences := s.sentenceBuffer.Add(delta)

	if len(sentences) > 0 {
		winner, isNew := s.arbiter.Decide(agentID)

		if winner != agentID {
			s.sentenceBuffer.Reset()
			return
		}

		if isNew {
			s.cancelLosers()
			s.sendEvent(transport.MessageTypeTTSStart, nil)
		}

		for _, sentence := range sentences {
			s.ttsQueue.Enqueue(s.ctx, sentence)
		}
	}
}

func (s *VoiceSession) handleResponseDone(agentID string, msg *transport.AgentMessage) {
	winner := s.arbiter.Winner()
	if winner == "" {
		winner, _ = s.arbiter.Decide(agentID)
	}
	if winner != agentID {
		s.sentenceBuffer.Reset()
		s.log.Debug("response done ignored, not winner", "agent_id", agentID, "winner", winner)
		return
	}

	remaining := s.sentenceBuffer.Flush()

	if remaining != "" {
		s.log.Debug("response done, enqueue remaining", "agent_id", agentID, "len", len(remaining), "text", remaining)
		s.ttsQueue.Enqueue(s.ctx, remaining)
	}

	s.arbiter.Reset()
}

func (s *VoiceSession) handleCompleteResponse(agentID string, msg *transport.AgentMessage) {
	text := s.extractText(msg.Payload)
	if text == "" {
		s.log.Debug("complete response empty", "agent_id", agentID)
		return
	}

	winner, isNew := s.arbiter.Decide(agentID)
	if winner != agentID {
		s.log.Debug("agent response ignored, not winner",
			"agent_id", agentID,
			"winner", winner)
		return
	}

	if isNew {
		s.cancelLosers()
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

func (s *VoiceSession) cancelLosers() {
	losers := s.arbiter.Losers()
	for _, loserID := range losers {
		if err := s.bridge.PublishCancellation(s.ctx, loserID, s.sessionID, "lost_arbitration"); err != nil {
			s.log.Error("failed to cancel loser agent",
				"agent_id", loserID,
				"error", err)
		}
		s.sendEvent(transport.MessageTypeInterrupt, transport.AgentCancelledEvent{
			AgentID: loserID,
			Reason:  "lost_arbitration",
		})
	}
}

func (s *VoiceSession) extractDelta(payload any) string {
	switch p := payload.(type) {
	case transport.ResponseDeltaPayload:
		return p.Delta
	case *transport.ResponseDeltaPayload:
		return p.Delta
	case map[string]any:
		delta, _ := p["delta"].(string)
		return delta
	}
	return ""
}

func (s *VoiceSession) extractText(payload any) string {
	switch p := payload.(type) {
	case transport.ResponsePayload:
		return p.Text
	case *transport.ResponsePayload:
		return p.Text
	case map[string]any:
		text, _ := p["text"].(string)
		return text
	}
	return ""
}

func (s *VoiceSession) synthesizeResponse(text string, cancelCh <-chan struct{}) {
	req := synthesis.Request{
		Text:    text,
		VoiceID: s.voiceID,
		Speed:   s.ttsSpeed,
		Format:  "opus",
		Cancel:  cancelCh,
	}

	s.speechCtrl.OnTTSAudioStart()
	s.sendEvent(transport.MessageTypeTTSStart, nil)

	cb := synthesis.Callbacks{
		OnReady: func(sampleRate uint32, voiceID string) {
			s.log.Debug("TTS ready", "sample_rate", sampleRate, "voice_id", voiceID)
		},
		OnAudio: func(data []byte, format string, sampleRate uint32) {
			s.log.Debug("TTS audio chunk", "bytes", len(data), "format", format, "sample_rate", sampleRate)
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
			s.log.Debug("TTS done, waiting for drain", "audio_duration_ms", audioDurationMs)
			if ctrl, ok := s.conn.(transport.OutputController); ok {
				ctrl.WaitForAudioDrain()
			}
			s.speechCtrl.OnTTSAudioEnd()
			s.sendEvent(transport.MessageTypeTTSEnd, nil)
			s.arbiter.Reset()
		},
		OnError: func(err error) {
			s.log.Error("TTS error", "error", err)
			s.speechCtrl.OnTTSAudioEnd()
			s.sendEvent(transport.MessageTypeError, map[string]string{
				"source":  "tts",
				"message": err.Error(),
			})
		},
	}

	if err := s.tts.Synthesize(s.ctx, req, cb); err != nil {
		s.log.Error("TTS synthesize failed", "error", err)
		s.speechCtrl.OnTTSAudioEnd()
	}
}

func (s *VoiceSession) stopTTS() {
	s.ttsQueue.Clear()
	s.sentenceBuffer.Reset()

	s.ttsMu.Lock()
	if s.ttsCancelCh != nil {
		close(s.ttsCancelCh)
		s.ttsCancelCh = nil
	}
	s.ttsMu.Unlock()

	if ctrl, ok := s.conn.(transport.OutputController); ok {
		ctrl.StopTTS()
	}
	s.conn.FlushAudioQueue()
}

func (s *VoiceSession) sendEvent(msgType transport.MessageType, payload any) {
	evt := transport.ServerEvent{
		Type:    string(msgType),
		Payload: payload,
	}
	if err := s.conn.Send(s.ctx, evt); err != nil {
		s.log.Error("failed to send event", "type", msgType, "error", err)
	}
}

func (s *VoiceSession) handleFrameRequest(msg *transport.AgentMessage) {
	agentScopes := s.getAgentScopes(msg.AgentID)
	if !slices.Contains(agentScopes, shared.ScopeVision.String()) {
		s.sendFrameResponse(msg.AgentID, msg.RequestID, nil, "vision scope not granted")
		return
	}

	if s.visionAnalyzer == nil {
		s.sendFrameResponse(msg.AgentID, msg.RequestID, nil, "vision not available")
		return
	}

	var req transport.FrameRequestPayload
	switch p := msg.Payload.(type) {
	case transport.FrameRequestPayload:
		req = p
	case *transport.FrameRequestPayload:
		req = *p
	case map[string]any:
		if v, ok := p["start_time"].(float64); ok {
			req.StartTime = int64(v)
		}
		if v, ok := p["end_time"].(float64); ok {
			req.EndTime = int64(v)
		}
		if v, ok := p["limit"].(float64); ok {
			req.Limit = int(v)
		}
		if v, ok := p["raw_base64"].(bool); ok {
			req.RawBase64 = v
		}
	default:
		s.sendFrameResponse(msg.AgentID, msg.RequestID, nil, "invalid request payload")
		return
	}

	if req.Limit == 0 {
		req.Limit = 5
	}
	if req.EndTime == 0 {
		req.EndTime = time.Now().UnixMilli()
	}
	if req.StartTime == 0 {
		req.StartTime = req.EndTime - 30000
	}

	frameReq := vision.FrameRequest{
		SessionID: s.sessionID,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Limit:     req.Limit,
		RawBase64: req.RawBase64,
	}

	resp, err := s.visionAnalyzer.GetFrames(s.ctx, frameReq)
	if err != nil {
		s.sendFrameResponse(msg.AgentID, msg.RequestID, nil, err.Error())
		return
	}

	payload := &transport.FrameResponsePayload{}
	if req.RawBase64 {
		payload.Frames = make([]transport.FrameData, len(resp.Frames))
		for i, f := range resp.Frames {
			payload.Frames[i] = transport.FrameData{
				Timestamp: f.Timestamp,
				Base64:    f.Base64,
			}
		}
	} else {
		payload.Descriptions = resp.Descriptions
	}

	s.sendFrameResponse(msg.AgentID, msg.RequestID, payload, "")
}

func (s *VoiceSession) sendFrameResponse(agentID, requestID string, payload *transport.FrameResponsePayload, errMsg string) {
	if payload == nil {
		payload = &transport.FrameResponsePayload{}
	}
	if errMsg != "" {
		payload.Error = errMsg
	}

	msg := &transport.AgentMessage{
		Type:      transport.MessageTypeFrameResponse,
		RequestID: requestID,
		SessionID: s.sessionID,
		AgentID:   agentID,
		UserID:    s.UserID(),
		Timestamp: time.Now(),
		Payload:   payload,
	}

	if err := s.bridge.PublishResponse(s.ctx, msg); err != nil {
		s.log.Error("failed to send frame response", "error", err)
	}
}

func (s *VoiceSession) Close() error {
	s.ttsQueue.Clear()
	s.sentenceBuffer.Reset()
	s.ttsBridge.Stop()

	s.cancel()

	s.bridge.UnsubscribeFromSession(s.sessionID)

	s.wg.Wait()

	if err := s.stt.Close(); err != nil {
		s.log.Error("failed to close STT", "error", err)
	}

	if err := s.tts.Close(); err != nil {
		s.log.Error("failed to close TTS", "error", err)
	}

	if s.frameCapturer != nil {
		s.frameCapturer.Stop()
	}

	if s.visionAnalyzer != nil {
		if err := s.visionAnalyzer.Cleanup(context.Background(), s.sessionID); err != nil {
			s.log.Error("failed to cleanup vision frames", "error", err)
		}
	}

	return s.conn.Close()
}
