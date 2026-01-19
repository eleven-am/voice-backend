package voicesession

import (
	"testing"
	"time"
)

func TestNewSpeechController(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{})
	if ctrl == nil {
		t.Fatal("NewSpeechController should not return nil")
	}
	if ctrl.State() != StateIdle {
		t.Errorf("expected initial state %s, got %s", StateIdle, ctrl.State())
	}
}

func TestNewSpeechController_DefaultPolicy(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{})
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

func TestNewSpeechController_CustomPolicy(t *testing.T) {
	policy := BargeInPolicy{
		AllowWhileSpeaking: true,
		MinPartialChars:    5,
		MinSilenceForEnd:   200 * time.Millisecond,
		DebounceMin:        50 * time.Millisecond,
		DebounceMax:        300 * time.Millisecond,
	}
	ctrl := NewSpeechController(policy)
	if ctrl.policy.DebounceMin != 50*time.Millisecond {
		t.Errorf("expected custom DebounceMin 50ms, got %v", ctrl.policy.DebounceMin)
	}
	if ctrl.policy.MinSilenceForEnd != 200*time.Millisecond {
		t.Errorf("expected custom MinSilenceForEnd 200ms, got %v", ctrl.policy.MinSilenceForEnd)
	}
}

func TestSpeechController_OnTTSAudioStart(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{})
	ctrl.OnTTSAudioStart()
	if ctrl.State() != StateSpeaking {
		t.Errorf("expected state %s after TTS start, got %s", StateSpeaking, ctrl.State())
	}
	if !ctrl.ttsActive {
		t.Error("ttsActive should be true after TTS start")
	}
}

func TestSpeechController_OnTTSAudioEnd(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{})
	ctrl.OnTTSAudioStart()
	ctrl.OnTTSAudioEnd()
	if ctrl.State() != StateIdle {
		t.Errorf("expected state %s after TTS end, got %s", StateIdle, ctrl.State())
	}
	if ctrl.ttsActive {
		t.Error("ttsActive should be false after TTS end")
	}
}

func TestSpeechController_OnTTSAudioEnd_FromInterrupted(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{AllowWhileSpeaking: true})
	ctrl.OnTTSAudioStart()
	ctrl.OnUserSpeechStart(time.Now())
	if ctrl.State() != StateInterrupted {
		t.Fatalf("expected state %s, got %s", StateInterrupted, ctrl.State())
	}
	ctrl.OnTTSAudioEnd()
	if ctrl.State() != StateIdle {
		t.Errorf("expected state %s after TTS end from interrupted, got %s", StateIdle, ctrl.State())
	}
}

func TestSpeechController_OnUserSpeechStart_FromIdle(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{})
	now := time.Now()
	actions := ctrl.OnUserSpeechStart(now)
	if ctrl.State() != StateListening {
		t.Errorf("expected state %s, got %s", StateListening, ctrl.State())
	}
	if len(actions) != 0 {
		t.Errorf("expected no actions from idle, got %d", len(actions))
	}
}

func TestSpeechController_OnUserSpeechStart_BargeIn(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{AllowWhileSpeaking: true})
	ctrl.OnTTSAudioStart()
	now := time.Now()
	actions := ctrl.OnUserSpeechStart(now)
	if ctrl.State() != StateInterrupted {
		t.Errorf("expected state %s, got %s", StateInterrupted, ctrl.State())
	}
	if len(actions) != 3 {
		t.Fatalf("expected 3 actions for barge-in, got %d", len(actions))
	}
	expectedActions := []ActionType{ActionStopTTS, ActionCancelAgent, ActionPauseOutput}
	for i, expected := range expectedActions {
		if actions[i].Type != expected {
			t.Errorf("action %d: expected %s, got %s", i, expected, actions[i].Type)
		}
		if actions[i].Reason != "barge_in" {
			t.Errorf("action %d: expected reason 'barge_in', got %s", i, actions[i].Reason)
		}
	}
}

func TestSpeechController_OnUserSpeechStart_NoBargeIn(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{AllowWhileSpeaking: false})
	ctrl.OnTTSAudioStart()
	now := time.Now()
	actions := ctrl.OnUserSpeechStart(now)
	if ctrl.State() != StateSpeaking {
		t.Errorf("expected state to remain %s without barge-in, got %s", StateSpeaking, ctrl.State())
	}
	if len(actions) != 0 {
		t.Errorf("expected no actions when barge-in disabled, got %d", len(actions))
	}
}

func TestSpeechController_OnUserSpeechEnd_FromListening(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{})
	ctrl.OnUserSpeechStart(time.Now())
	actions := ctrl.OnUserSpeechEnd(time.Now())
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[0].Type != ActionEndUtterance {
		t.Errorf("expected first action %s, got %s", ActionEndUtterance, actions[0].Type)
	}
	if actions[1].Type != ActionResumeOutput {
		t.Errorf("expected second action %s, got %s", ActionResumeOutput, actions[1].Type)
	}
}

func TestSpeechController_OnUserSpeechEnd_FromSpeaking(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{})
	ctrl.OnTTSAudioStart()
	actions := ctrl.OnUserSpeechEnd(time.Now())
	if len(actions) != 0 {
		t.Errorf("expected no actions when speaking, got %d", len(actions))
	}
}

func TestSpeechController_OnUserSpeechEnd_FromInterrupted(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{AllowWhileSpeaking: true})
	ctrl.OnTTSAudioStart()
	ctrl.OnUserSpeechStart(time.Now())
	actions := ctrl.OnUserSpeechEnd(time.Now())
	if len(actions) != 0 {
		t.Errorf("expected no actions when interrupted, got %d", len(actions))
	}
}

func TestSpeechController_OnBackpressure_WhileTTSActive(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{})
	ctrl.OnTTSAudioStart()
	actions := ctrl.OnBackpressure()
	if ctrl.State() != StateInterrupted {
		t.Errorf("expected state %s, got %s", StateInterrupted, ctrl.State())
	}
	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(actions))
	}
	for _, action := range actions {
		if action.Reason != "backpressure" {
			t.Errorf("expected reason 'backpressure', got %s", action.Reason)
		}
	}
}

func TestSpeechController_OnBackpressure_WhileTTSInactive(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{})
	actions := ctrl.OnBackpressure()
	if len(actions) != 0 {
		t.Errorf("expected no actions when TTS inactive, got %d", len(actions))
	}
}

func TestSpeechController_ShouldEndBySilence_NoLastSpeech(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{MinSilenceForEnd: 100 * time.Millisecond})
	if ctrl.ShouldEndBySilence(time.Now()) {
		t.Error("should not end by silence without prior speech")
	}
}

func TestSpeechController_ShouldEndBySilence_NotListening(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{MinSilenceForEnd: 100 * time.Millisecond})
	ctrl.OnTTSAudioStart()
	ctrl.OnUserSpeechStart(time.Now())
	if ctrl.ShouldEndBySilence(time.Now().Add(200 * time.Millisecond)) {
		t.Error("should not end by silence when not in listening state")
	}
}

func TestSpeechController_ShouldEndBySilence_NotEnoughSilence(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{MinSilenceForEnd: 500 * time.Millisecond})
	now := time.Now()
	ctrl.OnUserSpeechStart(now)
	if ctrl.ShouldEndBySilence(now.Add(200 * time.Millisecond)) {
		t.Error("should not end by silence before MinSilenceForEnd")
	}
	if ctrl.State() != StateListening {
		t.Errorf("state should remain %s, got %s", StateListening, ctrl.State())
	}
}

func TestSpeechController_ShouldEndBySilence_Success(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{MinSilenceForEnd: 100 * time.Millisecond})
	now := time.Now()
	ctrl.OnUserSpeechStart(now)
	if !ctrl.ShouldEndBySilence(now.Add(150 * time.Millisecond)) {
		t.Error("should end by silence after MinSilenceForEnd")
	}
	if ctrl.State() != StateIdle {
		t.Errorf("state should be %s after silence end, got %s", StateIdle, ctrl.State())
	}
}

func TestSpeechController_State_Concurrent(t *testing.T) {
	ctrl := NewSpeechController(BargeInPolicy{AllowWhileSpeaking: true})
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			ctrl.OnTTSAudioStart()
			ctrl.OnTTSAudioEnd()
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		ctrl.OnUserSpeechStart(time.Now())
		_ = ctrl.State()
		ctrl.OnUserSpeechEnd(time.Now())
	}
	<-done
}
