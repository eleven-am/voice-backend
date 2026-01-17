package user

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	sessionCookieName = "voice_session"
	csrfCookieName    = "voice_csrf"
	sessionMaxAge     = 7 * 24 * 60 * 60
)

type SessionManager struct {
	hmacKey []byte
	secure  bool
	domain  string
}

func NewSessionManager(hmacKey []byte, secure bool, domain string) *SessionManager {
	return &SessionManager{
		hmacKey: hmacKey,
		secure:  secure,
		domain:  domain,
	}
}

func (s *SessionManager) Get(c echo.Context) (userID, csrf string, err error) {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil {
		return "", "", err
	}

	payload, err := s.VerifyValue(cookie.Value)
	if err != nil {
		return "", "", err
	}

	parts := strings.SplitN(payload, "|", 2)
	if len(parts) != 2 {
		return "", "", errors.New("invalid session format")
	}

	return parts[0], parts[1], nil
}

func (s *SessionManager) Create(c echo.Context, userID string) {
	csrf := s.generateCSRF()
	payload := userID + "|" + csrf
	signed := s.SignValue(payload)

	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    signed,
		Path:     "/",
		Domain:   s.domain,
		MaxAge:   sessionMaxAge,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})

	c.SetCookie(&http.Cookie{
		Name:     csrfCookieName,
		Value:    csrf,
		Path:     "/",
		Domain:   s.domain,
		MaxAge:   sessionMaxAge,
		HttpOnly: false,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *SessionManager) Clear(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   s.domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})

	c.SetCookie(&http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		Domain:   s.domain,
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *SessionManager) SignValue(value string) string {
	mac := hmac.New(sha256.New, s.hmacKey)
	mac.Write([]byte(value))
	sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	return base64.URLEncoding.EncodeToString([]byte(value)) + "." + sig
}

func (s *SessionManager) VerifyValue(signed string) (string, error) {
	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		return "", errors.New("invalid signature format")
	}

	payload, err := base64.URLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", err
	}

	mac := hmac.New(sha256.New, s.hmacKey)
	mac.Write(payload)
	expectedSig := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[1]), []byte(expectedSig)) {
		return "", errors.New("invalid signature")
	}

	return string(payload), nil
}

func (s *SessionManager) generateCSRF() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.URLEncoding.EncodeToString(b)
}

func (s *SessionManager) RequireCSRF(c echo.Context, sessionCSRF string) error {
	header := c.Request().Header.Get("X-CSRF-Token")
	if header == "" {
		return echo.NewHTTPError(http.StatusForbidden, "missing csrf token")
	}

	csrfCookie, err := c.Cookie(csrfCookieName)
	if err != nil || csrfCookie.Value == "" {
		return echo.NewHTTPError(http.StatusForbidden, "missing csrf cookie")
	}

	if csrfCookie.Value != header || sessionCSRF != header {
		return echo.NewHTTPError(http.StatusForbidden, "invalid csrf token")
	}

	return nil
}

func (s *SessionManager) GenerateOAuthState(redirectURI string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}

	state := base64.URLEncoding.EncodeToString(b)
	if redirectURI != "" {
		state += "|" + redirectURI
	}

	return s.SignValue(state)
}

func (s *SessionManager) ExtractRedirectURI(state string) string {
	payload, err := s.VerifyValue(state)
	if err != nil {
		return ""
	}

	parts := strings.SplitN(payload, "|", 2)
	if len(parts) < 2 {
		return ""
	}

	return parts[1]
}
