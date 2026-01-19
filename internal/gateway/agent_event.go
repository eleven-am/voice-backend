package gateway

type AgentEventType string

const (
	AgentEventResponseCreated    AgentEventType = "response.created"
	AgentEventResponseTextDelta  AgentEventType = "response.text.delta"
	AgentEventResponseTextDone   AgentEventType = "response.text.done"
	AgentEventResponseAudioDelta AgentEventType = "response.audio.delta"
	AgentEventResponseAudioDone  AgentEventType = "response.audio.done"
	AgentEventResponseDone       AgentEventType = "response.done"
	AgentEventResponseStatus     AgentEventType = "response.status"
	AgentEventError              AgentEventType = "error"
	AgentEventDisconnected       AgentEventType = "agent.disconnected"
	AgentEventUtterance          AgentEventType = "utterance"
	AgentEventInterrupt          AgentEventType = "interrupt"
)

type AgentEvent struct {
	Type      AgentEventType `json:"type"`
	SessionID string         `json:"session_id"`
	AgentID   string         `json:"agent_id,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	Payload   any            `json:"payload,omitempty"`
}

type UtteranceEventPayload struct {
	Text    string `json:"text"`
	IsFinal bool   `json:"is_final"`
	UserID  string `json:"user_id,omitempty"`
}

type InterruptEventPayload struct {
	Reason string `json:"reason"`
}

type TextDeltaEventPayload struct {
	ResponseID string `json:"response_id,omitempty"`
	Delta      string `json:"delta"`
}

type TextDoneEventPayload struct {
	ResponseID string `json:"response_id,omitempty"`
	Text       string `json:"text"`
}

type AudioDeltaEventPayload struct {
	ResponseID string `json:"response_id,omitempty"`
	Delta      []byte `json:"delta"`
	Format     string `json:"format,omitempty"`
	SampleRate uint32 `json:"sample_rate,omitempty"`
}

type ResponseDoneEventPayload struct {
	ResponseID string `json:"response_id,omitempty"`
	Status     string `json:"status"`
}

type AgentErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type AgentOutput struct {
	Type  AgentOutputType
	Text  string
	Audio []byte
	Err   error
}

type AgentOutputType int

const (
	AgentOutputText AgentOutputType = iota
	AgentOutputAudio
	AgentOutputDone
	AgentOutputError
)

func eventToAgentOutput(event AgentEvent) *AgentOutput {
	switch event.Type {
	case AgentEventResponseTextDelta:
		if p, ok := event.Payload.(map[string]any); ok {
			delta, _ := p["delta"].(string)
			return &AgentOutput{Type: AgentOutputText, Text: delta}
		}
		if p, ok := event.Payload.(*TextDeltaEventPayload); ok {
			return &AgentOutput{Type: AgentOutputText, Text: p.Delta}
		}
	case AgentEventResponseTextDone:
		if p, ok := event.Payload.(map[string]any); ok {
			text, _ := p["text"].(string)
			return &AgentOutput{Type: AgentOutputText, Text: text}
		}
		if p, ok := event.Payload.(*TextDoneEventPayload); ok {
			return &AgentOutput{Type: AgentOutputText, Text: p.Text}
		}
	case AgentEventResponseAudioDelta:
		if p, ok := event.Payload.(map[string]any); ok {
			if delta, ok := p["delta"].([]byte); ok {
				return &AgentOutput{Type: AgentOutputAudio, Audio: delta}
			}
		}
		if p, ok := event.Payload.(*AudioDeltaEventPayload); ok {
			return &AgentOutput{Type: AgentOutputAudio, Audio: p.Delta}
		}
	case AgentEventResponseDone:
		return &AgentOutput{Type: AgentOutputDone}
	case AgentEventError:
		if p, ok := event.Payload.(map[string]any); ok {
			msg, _ := p["message"].(string)
			return &AgentOutput{Type: AgentOutputError, Err: &agentError{message: msg}}
		}
		if p, ok := event.Payload.(*AgentErrorPayload); ok {
			return &AgentOutput{Type: AgentOutputError, Err: &agentError{message: p.Message}}
		}
	}
	return nil
}

type agentError struct {
	message string
}

func (e *agentError) Error() string {
	return e.message
}
