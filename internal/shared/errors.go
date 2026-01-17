package shared

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
)

type APIError struct {
	Code    string `json:"code" example:"invalid_request"`
	Message string `json:"message" example:"Invalid request body"`
	Details any    `json:"details,omitempty" swaggertype:"object"`
}

func NewAPIError(code, message string) *APIError {
	return &APIError{
		Code:    code,
		Message: message,
	}
}

func (e *APIError) WithDetails(details any) *APIError {
	e.Details = details
	return e
}

func (e *APIError) ToHTTP(status int) *echo.HTTPError {
	return echo.NewHTTPError(status, e)
}

func BadRequest(code, message string) *echo.HTTPError {
	return NewAPIError(code, message).ToHTTP(http.StatusBadRequest)
}

func Unauthorized(code, message string) *echo.HTTPError {
	return NewAPIError(code, message).ToHTTP(http.StatusUnauthorized)
}

func Forbidden(code, message string) *echo.HTTPError {
	return NewAPIError(code, message).ToHTTP(http.StatusForbidden)
}

func NotFound(code, message string) *echo.HTTPError {
	return NewAPIError(code, message).ToHTTP(http.StatusNotFound)
}

func Conflict(code, message string) *echo.HTTPError {
	return NewAPIError(code, message).ToHTTP(http.StatusConflict)
}

func InternalError(code, message string) *echo.HTTPError {
	return NewAPIError(code, message).ToHTTP(http.StatusInternalServerError)
}
