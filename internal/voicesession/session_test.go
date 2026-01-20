package voicesession

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eleven-am/voice-backend/internal/router"
	"github.com/eleven-am/voice-backend/internal/synthesis"
	"github.com/eleven-am/voice-backend/internal/transcription"
	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/eleven-am/voice-backend/internal/vision"
)

type mockConnection struct {
	mu             sync.Mutex
	audioInCh      chan []byte
	messagesCh     chan transport.ClientEnvelope
	sentEvents     []transport.ServerEvent
	sentAudio      []transport.AudioChunk
	connected      bool
	closed         bool
	flushedCount   int
	backpressureCB transport.BackpressureCallback
	videoCallback  func([]byte, string)
	hasVideo       bool
}

func newMockConnection() *mockConnection {
	return &mockConnection{
		audioInCh:  make(chan []byte, 100),
		messagesCh: make(chan transport.ClientEnvelope, 100),
		connected:  true,
	}
}

func (m *mockConnection) Send(ctx context.Context, event transport.ServerEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentEvents = append(m.sentEvents, event)
	return nil
}

func (m *mockConnection) SendAudio(ctx context.Context, chunk transport.AudioChunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentAudio = append(m.sentAudio, chunk)
	return nil
}

func (m *mockConnection) Messages() <-chan transport.ClientEnvelope {
	return m.messagesCh
}

func (m *mockConnection) AudioIn() <-chan []byte {
	return m.audioInCh
}

func (m *mockConnection) AudioFormat() transport.AudioFormat {
	return transport.AudioFormatOpus
}

func (m *mockConnection) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

func (m *mockConnection) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.connected = false
	return nil
}

func (m *mockConnection) FlushAudioQueue() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushedCount++
	return 0
}

func (m *mockConnection) SetBackpressureCallback(cb transport.BackpressureCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backpressureCB = cb
}

func (m *mockConnection) OnVideo(fn func([]byte, string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.videoCallback = fn
}

func (m *mockConnection) HasVideo() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hasVideo
}

func (m *mockConnection) PauseOutput()  {}
func (m *mockConnection) ResumeOutput() {}
func (m *mockConnection) StopTTS()      {}

type mockBridge struct {
	mu                sync.Mutex
	responseHandler   func(sessionID string, msg *transport.AgentMessage)
	publishedMsgs     []*transport.AgentMessage
	cancelledAgents   []string
	subscribedSession string
	subscribeErr      error
}

func newMockBridge() *mockBridge {
	return &mockBridge{}
}

func (m *mockBridge) PublishUtterance(ctx context.Context, msg *transport.AgentMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedMsgs = append(m.publishedMsgs, msg)
	return nil
}

func (m *mockBridge) PublishToAgents(ctx context.Context, agentIDs []string, msg *transport.AgentMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedMsgs = append(m.publishedMsgs, msg)
	return nil
}

func (m *mockBridge) PublishCancellation(ctx context.Context, agentID, sessionID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelledAgents = append(m.cancelledAgents, agentID)
	return nil
}

func (m *mockBridge) PublishResponse(ctx context.Context, msg *transport.AgentMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedMsgs = append(m.publishedMsgs, msg)
	return nil
}

func (m *mockBridge) SubscribeToSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.subscribeErr != nil {
		return m.subscribeErr
	}
	m.subscribedSession = sessionID
	return nil
}

func (m *mockBridge) UnsubscribeFromSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribedSession = ""
}

func (m *mockBridge) SetResponseHandler(handler func(sessionID string, msg *transport.AgentMessage)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseHandler = handler
}

type mockTranscriber struct {
	mu        sync.Mutex
	closed    bool
	closeErr  error
	frames    [][]byte
	callbacks transcription.Callbacks
}

func newMockTranscriber(callbacks transcription.Callbacks) *mockTranscriber {
	return &mockTranscriber{callbacks: callbacks}
}

func (m *mockTranscriber) SendOpusFrame(data []byte, sampleRate uint32, channels int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frames = append(m.frames, data)
	return nil
}

func (m *mockTranscriber) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return m.closeErr
}

type mockSynthesizer struct {
	mu       sync.Mutex
	closed   bool
	closeErr error
	requests []synthesis.Request
}

func newMockSynthesizer() *mockSynthesizer {
	return &mockSynthesizer{}
}

func (m *mockSynthesizer) Synthesize(ctx context.Context, req synthesis.Request, cb synthesis.Callbacks) error {
	m.mu.Lock()
	m.requests = append(m.requests, req)
	m.mu.Unlock()

	if cb.OnReady != nil {
		cb.OnReady(48000, "voice-1")
	}
	if cb.OnAudio != nil {
		cb.OnAudio([]byte{0x01, 0x02}, "opus", 48000)
	}
	if cb.OnDone != nil {
		cb.OnDone(100, 50, 10)
	}
	return nil
}

func (m *mockSynthesizer) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return m.closeErr
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		UserID: "user-1",
		Agents: []router.AgentInfo{
			{ID: "agent-1", Name: "Agent 1"},
		},
		BargeInPolicy: BargeInPolicy{AllowWhileSpeaking: true},
	}

	if cfg.UserID != "user-1" {
		t.Errorf("expected UserID 'user-1', got %s", cfg.UserID)
	}
	if len(cfg.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(cfg.Agents))
	}
	if !cfg.BargeInPolicy.AllowWhileSpeaking {
		t.Error("expected AllowWhileSpeaking to be true")
	}
}

func TestVideoConnection_Interface(t *testing.T) {
	conn := newMockConnection()
	conn.hasVideo = true

	var vc VideoConnection = conn
	if !vc.HasVideo() {
		t.Error("expected HasVideo to be true")
	}

	called := false
	vc.OnVideo(func(data []byte, mime string) {
		called = true
	})

	conn.mu.Lock()
	if conn.videoCallback == nil {
		t.Error("video callback should be set")
	}
	conn.videoCallback([]byte{0x01}, "video/VP8")
	conn.mu.Unlock()

	if !called {
		t.Error("video callback should have been called")
	}
}

func TestSpeechController_Integration(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{
		AllowWhileSpeaking: true,
		MinSilenceForEnd:   200 * time.Millisecond,
	})

	if ctrl.State() != StateIdle {
		t.Fatalf("expected initial state Idle, got %s", ctrl.State())
	}

	ctrl.OnTTSAudioStart()
	if ctrl.State() != StateSpeaking {
		t.Fatalf("expected state Speaking after TTS start, got %s", ctrl.State())
	}

	now := time.Now()
	actions := ctrl.OnUserSpeechStart(now)
	if ctrl.State() != StateInterrupted {
		t.Fatalf("expected state Interrupted after user speech during TTS, got %s", ctrl.State())
	}
	if len(actions) != 3 {
		t.Errorf("expected 3 barge-in actions, got %d", len(actions))
	}

	ctrl.OnTTSAudioEnd()
	if ctrl.State() != StateIdle {
		t.Errorf("expected state Idle after TTS end, got %s", ctrl.State())
	}
}

func TestArbiter_Integration(t *testing.T) {
	arbiter := NewArbiter()
	agents := []string{"agent-1", "agent-2", "agent-3"}

	arbiter.Start(agents)

	winner1, isNew1 := arbiter.Decide("agent-2")
	if winner1 != "agent-2" {
		t.Errorf("expected winner agent-2, got %s", winner1)
	}
	if !isNew1 {
		t.Error("expected isNew to be true for first decision")
	}

	winner2, isNew2 := arbiter.Decide("agent-1")
	if winner2 != "agent-2" {
		t.Errorf("expected winner to remain agent-2, got %s", winner2)
	}
	if isNew2 {
		t.Error("expected isNew to be false for subsequent decision")
	}

	losers := arbiter.Losers()
	if len(losers) != 2 {
		t.Errorf("expected 2 losers, got %d", len(losers))
	}

	arbiter.Reset()
	if arbiter.Winner() != "" {
		t.Error("expected empty winner after reset")
	}
}

func TestMockConnection_Implementation(t *testing.T) {
	conn := newMockConnection()

	if !conn.IsConnected() {
		t.Error("should be connected initially")
	}

	if conn.AudioFormat() != transport.AudioFormatOpus {
		t.Error("should return Opus format")
	}

	ctx := context.Background()
	err := conn.Send(ctx, transport.ServerEvent{Type: "test"})
	if err != nil {
		t.Errorf("Send should not error: %v", err)
	}
	if len(conn.sentEvents) != 1 {
		t.Errorf("expected 1 sent event, got %d", len(conn.sentEvents))
	}

	err = conn.SendAudio(ctx, transport.AudioChunk{Data: []byte{0x01}})
	if err != nil {
		t.Errorf("SendAudio should not error: %v", err)
	}
	if len(conn.sentAudio) != 1 {
		t.Errorf("expected 1 sent audio, got %d", len(conn.sentAudio))
	}

	conn.FlushAudioQueue()
	if conn.flushedCount != 1 {
		t.Errorf("expected flushedCount 1, got %d", conn.flushedCount)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
	if !conn.closed {
		t.Error("should be closed")
	}
	if conn.IsConnected() {
		t.Error("should not be connected after close")
	}
}

func TestMockBridge_Implementation(t *testing.T) {
	bridge := newMockBridge()
	ctx := context.Background()

	msg := &transport.AgentMessage{
		Type:      transport.MessageTypeUtterance,
		SessionID: "session-1",
	}

	err := bridge.PublishUtterance(ctx, msg)
	if err != nil {
		t.Errorf("PublishUtterance should not error: %v", err)
	}

	err = bridge.PublishToAgents(ctx, []string{"a1", "a2"}, msg)
	if err != nil {
		t.Errorf("PublishToAgents should not error: %v", err)
	}

	err = bridge.PublishCancellation(ctx, "agent-1", "session-1", "test")
	if err != nil {
		t.Errorf("PublishCancellation should not error: %v", err)
	}

	if len(bridge.cancelledAgents) != 1 {
		t.Errorf("expected 1 cancelled agent, got %d", len(bridge.cancelledAgents))
	}

	err = bridge.SubscribeToSession("session-1")
	if err != nil {
		t.Errorf("SubscribeToSession should not error: %v", err)
	}
	if bridge.subscribedSession != "session-1" {
		t.Errorf("expected subscribed session 'session-1', got %s", bridge.subscribedSession)
	}

	bridge.UnsubscribeFromSession("session-1")
	if bridge.subscribedSession != "" {
		t.Error("should have unsubscribed")
	}
}

func TestMockBridge_SubscribeError(t *testing.T) {
	bridge := newMockBridge()
	bridge.subscribeErr = errors.New("subscribe failed")

	err := bridge.SubscribeToSession("session-1")
	if err == nil {
		t.Error("expected error")
	}
}

func TestMockTranscriber_Implementation(t *testing.T) {
	stt := newMockTranscriber(transcription.Callbacks{})

	err := stt.SendOpusFrame([]byte{0x01, 0x02}, 48000, 1)
	if err != nil {
		t.Errorf("SendOpusFrame should not error: %v", err)
	}
	if len(stt.frames) != 1 {
		t.Errorf("expected 1 frame, got %d", len(stt.frames))
	}

	err = stt.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
	if !stt.closed {
		t.Error("should be closed")
	}
}

func TestMockSynthesizer_Implementation(t *testing.T) {
	tts := newMockSynthesizer()
	ctx := context.Background()

	readyCalled := false
	audioCalled := false
	doneCalled := false

	req := synthesis.Request{Text: "Hello"}
	cb := synthesis.Callbacks{
		OnReady: func(sampleRate uint32, voiceID string) {
			readyCalled = true
		},
		OnAudio: func(data []byte, format string, sampleRate uint32) {
			audioCalled = true
		},
		OnDone: func(audioDurationMs, processingDurationMs, textLength uint64) {
			doneCalled = true
		},
	}

	err := tts.Synthesize(ctx, req, cb)
	if err != nil {
		t.Errorf("Synthesize should not error: %v", err)
	}

	if !readyCalled {
		t.Error("OnReady should have been called")
	}
	if !audioCalled {
		t.Error("OnAudio should have been called")
	}
	if !doneCalled {
		t.Error("OnDone should have been called")
	}

	if len(tts.requests) != 1 {
		t.Errorf("expected 1 request, got %d", len(tts.requests))
	}

	err = tts.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
	if !tts.closed {
		t.Error("should be closed")
	}
}

func TestBargeInPolicy_Defaults(t *testing.T) {
	policy := BargeInPolicy{}
	ctrl := NewSpeechController(policy)

	if ctrl.policy.DebounceMin != 100*time.Millisecond {
		t.Errorf("expected default DebounceMin 100ms, got %v", ctrl.policy.DebounceMin)
	}
	if ctrl.policy.DebounceMax != 500*time.Millisecond {
		t.Errorf("expected default DebounceMax 500ms, got %v", ctrl.policy.DebounceMax)
	}
	if ctrl.policy.MinSilenceForEnd != 400*time.Millisecond {
		t.Errorf("expected default MinSilenceForEnd 400ms, got %v", ctrl.policy.MinSilenceForEnd)
	}
}

func TestStateTransitions_FullCycle(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{
		AllowWhileSpeaking: true,
		MinSilenceForEnd:   100 * time.Millisecond,
	})

	if ctrl.State() != StateIdle {
		t.Fatalf("initial state should be Idle")
	}

	now := time.Now()
	ctrl.OnUserSpeechStart(now)
	if ctrl.State() != StateListening {
		t.Fatalf("state should be Listening after user speech start")
	}

	actions := ctrl.OnUserSpeechEnd(now)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions from speech end, got %d", len(actions))
	}

	if ctrl.ShouldEndBySilence(now.Add(150 * time.Millisecond)) {
		if ctrl.State() != StateIdle {
			t.Fatalf("state should be Idle after silence timeout")
		}
	}

	ctrl.OnTTSAudioStart()
	if ctrl.State() != StateSpeaking {
		t.Fatalf("state should be Speaking after TTS start")
	}

	ctrl.OnUserSpeechStart(time.Now())
	if ctrl.State() != StateInterrupted {
		t.Fatalf("state should be Interrupted after user speech during Speaking")
	}

	ctrl.OnTTSAudioEnd()
	if ctrl.State() != StateIdle {
		t.Fatalf("state should be Idle after TTS end from Interrupted")
	}
}

func TestVisionConfig_Integration(t *testing.T) {
	cfg := Config{
		VisionAnalyzer: nil,
		VisionStore:    nil,
	}

	if cfg.VisionAnalyzer != nil {
		t.Error("VisionAnalyzer should be nil")
	}
	if cfg.VisionStore != nil {
		t.Error("VisionStore should be nil")
	}
}

func TestVisionConfig_WithComponents(t *testing.T) {
	analyzer := vision.NewAnalyzer(nil, nil, nil)

	cfg := Config{
		VisionAnalyzer: analyzer,
	}

	if cfg.VisionAnalyzer == nil {
		t.Error("VisionAnalyzer should not be nil")
	}
}
