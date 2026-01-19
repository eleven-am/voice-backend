package auth

import (
	"context"
	"strings"

	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/labstack/echo/v4"
)

type contextKey string

const claimsKey contextKey = "jwt_claims"

type UserSyncer interface {
	SyncFromJWT(ctx context.Context, userID, email, name, avatar string) error
}

type Middleware struct {
	validator  *JWTValidator
	userSyncer UserSyncer
}

func NewMiddleware(validator *JWTValidator, userSyncer UserSyncer) *Middleware {
	return &Middleware{
		validator:  validator,
		userSyncer: userSyncer,
	}
}

func (m *Middleware) Authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		authHeader := c.Request().Header.Get("Authorization")
		if authHeader == "" {
			return shared.Unauthorized("missing_token", "authorization header required")
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			return shared.Unauthorized("invalid_token", "bearer token required")
		}

		claims, err := m.validator.Validate(authHeader)
		if err != nil {
			if err == ErrExpiredToken {
				return shared.Unauthorized("token_expired", "token has expired")
			}
			return shared.Unauthorized("invalid_token", "invalid or malformed token")
		}

		ctx := context.WithValue(c.Request().Context(), claimsKey, claims)
		c.SetRequest(c.Request().WithContext(ctx))

		if m.userSyncer != nil {
			_ = m.userSyncer.SyncFromJWT(ctx, claims.UserID, claims.Email, claims.Name, claims.AvatarURL)
		}

		return next(c)
	}
}

func (m *Middleware) OptionalAuthenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		authHeader := c.Request().Header.Get("Authorization")
		if authHeader == "" {
			return next(c)
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			return next(c)
		}

		claims, err := m.validator.Validate(authHeader)
		if err != nil {
			return next(c)
		}

		ctx := context.WithValue(c.Request().Context(), claimsKey, claims)
		c.SetRequest(c.Request().WithContext(ctx))

		return next(c)
	}
}

func GetClaims(c echo.Context) *Claims {
	claims, ok := c.Request().Context().Value(claimsKey).(*Claims)
	if !ok {
		return nil
	}
	return claims
}

func RequireAuth(c echo.Context) (string, error) {
	claims := GetClaims(c)
	if claims == nil {
		return "", shared.Unauthorized("auth_required", "authentication required")
	}
	return claims.UserID, nil
}

func MiddlewareFunc(validator *JWTValidator, userSyncer UserSyncer) echo.MiddlewareFunc {
	m := NewMiddleware(validator, userSyncer)
	return m.Authenticate
}

func OptionalMiddlewareFunc(validator *JWTValidator) echo.MiddlewareFunc {
	m := &Middleware{validator: validator}
	return m.OptionalAuthenticate
}

func SetClaimsForTest(c echo.Context, claims *Claims) {
	ctx := context.WithValue(c.Request().Context(), claimsKey, claims)
	c.SetRequest(c.Request().WithContext(ctx))
}
