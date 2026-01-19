package gateway

import (
	"context"
	"errors"

	"github.com/eleven-am/voice-backend/internal/apikey"
)

var (
	ErrUnauthorized    = errors.New("unauthorized")
	ErrInvalidAPIKey   = errors.New("invalid api key")
	ErrAgentNotAllowed = errors.New("agent not allowed for this api key")
)

type Authenticator struct {
	apiKeyStore *apikey.Store
}

func NewAuthenticator(store *apikey.Store) *Authenticator {
	return &Authenticator{apiKeyStore: store}
}

func (a *Authenticator) ValidateAPIKey(ctx context.Context, secret string) (*apikey.APIKey, error) {
	key, err := a.apiKeyStore.Validate(ctx, secret)
	if err != nil {
		return nil, ErrInvalidAPIKey
	}
	return key, nil
}

func (a *Authenticator) ValidateAgentAccess(key *apikey.APIKey, agentID string) error {
	if key.OwnerType == apikey.OwnerTypeAgent && key.OwnerID == agentID {
		return nil
	}

	for _, scope := range key.Scopes {
		if scope == "agent:*" || scope == "agent:"+agentID {
			return nil
		}
	}

	return ErrAgentNotAllowed
}
