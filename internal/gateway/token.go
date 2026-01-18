package gateway

import (
	"time"

	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/livekit/protocol/auth"
)

type TokenService struct {
	apiKey    string
	apiSecret string
	url       string
}

func NewTokenService(apiKey, apiSecret, url string) *TokenService {
	return &TokenService{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		url:       url,
	}
}

func (s *TokenService) URL() string {
	return s.url
}

func (s *TokenService) GenerateToken(identity, room string) (string, error) {
	at := auth.NewAccessToken(s.apiKey, s.apiSecret)

	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     room,
	}

	at.SetIdentity(identity).
		SetValidFor(24 * time.Hour).
		SetVideoGrant(grant)

	return at.ToJWT()
}

func (s *TokenService) GenerateRoomName() string {
	return "room_" + shared.NewID("")
}
