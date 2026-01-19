package realtime

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

type Session struct {
	ID        string
	userID    string
	conn      *Conn
	iceCh     chan webrtc.ICECandidateInit
	done      chan struct{}
	createdAt time.Time
	closeOnce sync.Once
	log       *slog.Logger
}

func NewSession(conn *Conn, userID string, iceBufSize int, log *slog.Logger) *Session {
	if iceBufSize <= 0 {
		iceBufSize = 128
	}

	if log == nil {
		log = slog.Default()
	}

	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}

	return &Session{
		ID:        hex.EncodeToString(idBytes[:]),
		userID:    userID,
		conn:      conn,
		iceCh:     make(chan webrtc.ICECandidateInit, iceBufSize),
		done:      make(chan struct{}),
		createdAt: time.Now(),
		log:       log,
	}
}

func (s *Session) UserID() string {
	return s.userID
}

func (s *Session) Conn() *Conn {
	return s.conn
}

func (s *Session) SendICE(candidate webrtc.ICECandidateInit) {
	select {
	case s.iceCh <- candidate:
	case <-s.done:
	default:
		s.log.Warn("ICE candidate dropped, buffer full", "session_id", s.ID)
	}
}

func (s *Session) ICECandidates() <-chan webrtc.ICECandidateInit {
	return s.iceCh
}

func (s *Session) Done() <-chan struct{} {
	return s.done
}

func (s *Session) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		close(s.iceCh)
		s.conn.Close()
	})
}

func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}
