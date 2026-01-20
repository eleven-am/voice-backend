package realtime

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestNewSession(t *testing.T) {
	session := NewSession(nil, "user-123", 0, nil)
	if session == nil {
		t.Fatal("NewSession should not return nil")
	}
	if session.ID == "" {
		t.Error("session ID should not be empty")
	}
	if len(session.ID) != 32 {
		t.Errorf("expected session ID length 32, got %d", len(session.ID))
	}
	if session.userID != "user-123" {
		t.Errorf("expected userID 'user-123', got %s", session.userID)
	}
	if session.log == nil {
		t.Error("logger should not be nil (default)")
	}
}

func TestNewSession_DefaultICEBufferSize(t *testing.T) {
	session := NewSession(nil, "user", 0, nil)
	if cap(session.iceCh) != 128 {
		t.Errorf("expected default ICE buffer size 128, got %d", cap(session.iceCh))
	}
}

func TestNewSession_NegativeICEBufferSize(t *testing.T) {
	session := NewSession(nil, "user", -10, nil)
	if cap(session.iceCh) != 128 {
		t.Errorf("expected default ICE buffer size 128 for negative, got %d", cap(session.iceCh))
	}
}

func TestNewSession_CustomICEBufferSize(t *testing.T) {
	session := NewSession(nil, "user", 64, nil)
	if cap(session.iceCh) != 64 {
		t.Errorf("expected ICE buffer size 64, got %d", cap(session.iceCh))
	}
}

func TestSession_UserID(t *testing.T) {
	session := NewSession(nil, "test-user", 0, nil)
	if session.UserID() != "test-user" {
		t.Errorf("expected UserID 'test-user', got %s", session.UserID())
	}
}

func TestSession_Conn(t *testing.T) {
	session := NewSession(nil, "user", 0, nil)
	if session.Conn() != nil {
		t.Error("expected Conn to be nil when created with nil")
	}
}

func TestSession_CreatedAt(t *testing.T) {
	before := time.Now()
	session := NewSession(nil, "user", 0, nil)
	after := time.Now()

	createdAt := session.CreatedAt()
	if createdAt.Before(before) {
		t.Error("CreatedAt should not be before session creation")
	}
	if createdAt.After(after) {
		t.Error("CreatedAt should not be after session creation")
	}
}

func TestSession_Done(t *testing.T) {
	session := NewSession(nil, "user", 0, nil)
	done := session.Done()
	if done == nil {
		t.Fatal("Done channel should not be nil")
	}

	select {
	case <-done:
		t.Error("Done channel should not be closed initially")
	default:
	}
}

func TestSession_ICECandidates(t *testing.T) {
	session := NewSession(nil, "user", 10, nil)
	ch := session.ICECandidates()
	if ch == nil {
		t.Fatal("ICECandidates channel should not be nil")
	}
}

func TestSession_SendICE(t *testing.T) {
	session := NewSession(nil, "user", 10, nil)
	candidate := webrtc.ICECandidateInit{
		Candidate: "candidate:123",
	}
	session.SendICE(candidate)

	select {
	case received := <-session.ICECandidates():
		if received.Candidate != candidate.Candidate {
			t.Errorf("expected candidate '%s', got '%s'", candidate.Candidate, received.Candidate)
		}
	default:
		t.Error("expected to receive ICE candidate")
	}
}

func TestSession_SendICE_BufferFull(t *testing.T) {
	session := NewSession(nil, "user", 1, nil)

	session.SendICE(webrtc.ICECandidateInit{Candidate: "1"})
	session.SendICE(webrtc.ICECandidateInit{Candidate: "2"})

	select {
	case <-session.ICECandidates():
	default:
		t.Error("expected at least one candidate")
	}

	select {
	case <-session.ICECandidates():
		t.Error("second candidate should have been dropped")
	default:
	}
}

func TestSession_UniqueIDs(t *testing.T) {
	session1 := NewSession(nil, "user", 0, nil)
	session2 := NewSession(nil, "user", 0, nil)

	if session1.ID == session2.ID {
		t.Error("session IDs should be unique")
	}
}

func TestSession_WithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	session := NewSession(nil, "user", 0, logger)
	if session.log == nil {
		t.Error("logger should not be nil")
	}
}

func TestSession_IDLength(t *testing.T) {
	session := NewSession(nil, "user", 0, nil)
	if len(session.ID) != 32 {
		t.Errorf("expected session ID length 32 (16 bytes hex encoded), got %d", len(session.ID))
	}
}

func TestSession_IDHexFormat(t *testing.T) {
	session := NewSession(nil, "user", 0, nil)
	for _, c := range session.ID {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("session ID should be hex encoded, got character %c", c)
		}
	}
}

func TestSession_MultipleICECandidates(t *testing.T) {
	session := NewSession(nil, "user", 5, nil)
	for i := 0; i < 5; i++ {
		session.SendICE(webrtc.ICECandidateInit{Candidate: string(rune('a' + i))})
	}

	received := 0
	for received < 5 {
		select {
		case <-session.ICECandidates():
			received++
		default:
			t.Fatalf("expected 5 candidates, got %d", received)
		}
	}
}
