package voicesession

import (
	"sync"
	"time"
)

type ActionType string

const (
	ActionStopTTS      ActionType = "stop_tts"
	ActionCancelAgent  ActionType = "cancel_agent"
	ActionPauseOutput  ActionType = "pause_output"
	ActionResumeOutput ActionType = "resume_output"
	ActionEndUtterance ActionType = "end_utterance"
)

type Action struct {
	Type   ActionType
	Reason string
}

type SpeechState string

const (
	StateIdle        SpeechState = "idle"
	StateListening   SpeechState = "listening"
	StateSpeaking    SpeechState = "speaking"
	StateInterrupted SpeechState = "interrupted"
)

type BargeInPolicy struct {
	AllowWhileSpeaking bool
	MinPartialChars    int
	MinSilenceForEnd   time.Duration
	DebounceMin        time.Duration
	DebounceMax        time.Duration
}

type SpeechController struct {
	mu         sync.Mutex
	state      SpeechState
	policy     BargeInPolicy
	ttsActive  bool
	lastSpeech time.Time
}

func NewSpeechController(policy BargeInPolicy) *SpeechController {
	if policy.DebounceMin == 0 {
		policy.DebounceMin = 100 * time.Millisecond
	}
	if policy.DebounceMax == 0 {
		policy.DebounceMax = 500 * time.Millisecond
	}
	if policy.MinSilenceForEnd == 0 {
		policy.MinSilenceForEnd = 400 * time.Millisecond
	}
	return &SpeechController{
		state:  StateIdle,
		policy: policy,
	}
}

func (c *SpeechController) OnTTSAudioStart() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttsActive = true
	c.state = StateSpeaking
}

func (c *SpeechController) OnTTSAudioEnd() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttsActive = false
	if c.state == StateSpeaking || c.state == StateInterrupted {
		c.state = StateIdle
	}
}

func (c *SpeechController) OnUserSpeechStart(now time.Time) []Action {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSpeech = now
	switch c.state {
	case StateSpeaking:
		if c.policy.AllowWhileSpeaking {
			c.state = StateInterrupted
			return []Action{
				{Type: ActionStopTTS, Reason: "barge_in"},
				{Type: ActionCancelAgent, Reason: "barge_in"},
				{Type: ActionPauseOutput, Reason: "barge_in"},
			}
		}
	case StateIdle:
		c.state = StateListening
	}
	return nil
}

func (c *SpeechController) OnUserSpeechEnd(now time.Time) []Action {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSpeech = now
	if c.state == StateInterrupted || c.state == StateSpeaking {
		return nil
	}
	return []Action{{Type: ActionEndUtterance, Reason: "speech_end"}, {Type: ActionResumeOutput, Reason: "speech_end"}}
}

func (c *SpeechController) OnBackpressure() []Action {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ttsActive {
		c.state = StateInterrupted
		return []Action{
			{Type: ActionStopTTS, Reason: "backpressure"},
			{Type: ActionCancelAgent, Reason: "backpressure"},
			{Type: ActionPauseOutput, Reason: "backpressure"},
		}
	}
	return nil
}

func (c *SpeechController) ShouldEndBySilence(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastSpeech.IsZero() {
		return false
	}
	if c.state != StateListening {
		return false
	}
	if now.Sub(c.lastSpeech) >= c.policy.MinSilenceForEnd {
		c.lastSpeech = time.Time{}
		c.state = StateIdle
		return true
	}
	return false
}

func (c *SpeechController) State() SpeechState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}
