package gateway

import (
	"context"
	"errors"

	"github.com/eleven-am/voice-backend/internal/apikey"
)

var (
	ErrUnauthorized    = errors.New("unauthorized")
	ErrInvalidAPIKey   = errors.New("invalid api key")
	ErrNotAgentKey     = errors.New("api key is not an agent key")
	ErrAgentIDMismatch = errors.New("agent id does not match api key owner")
)

type APIKeyValidator interface {
	Validate(ctx context.Context, secret string) (*apikey.APIKey, error)
}

type Authenticator struct {
	apiKeyStore APIKeyValidator
}

func NewAuthenticator(store APIKeyValidator) *Authenticator {
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
	if key.OwnerType != apikey.OwnerTypeAgent {
		return ErrNotAgentKey
	}
	if key.OwnerID != agentID {
		return ErrAgentIDMismatch
	}
	return nil
}
