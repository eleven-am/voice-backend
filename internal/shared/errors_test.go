package shared

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestNewAPIError(t *testing.T) {
	err := NewAPIError("test_code", "test message")
	if err.Code != "test_code" {
		t.Errorf("expected code 'test_code', got '%s'", err.Code)
	}
	if err.Message != "test message" {
		t.Errorf("expected message 'test message', got '%s'", err.Message)
	}
	if err.Details != nil {
		t.Errorf("expected nil details, got %v", err.Details)
	}
}

func TestAPIError_WithDetails(t *testing.T) {
	err := NewAPIError("code", "message")
	details := map[string]string{"field": "value"}
	err = err.WithDetails(details)

	if err.Details == nil {
		t.Fatal("expected details to be set")
	}
	d, ok := err.Details.(map[string]string)
	if !ok {
		t.Fatal("expected details to be map[string]string")
	}
	if d["field"] != "value" {
		t.Errorf("expected field 'value', got '%s'", d["field"])
	}
}

func TestAPIError_ToHTTP(t *testing.T) {
	apiErr := NewAPIError("code", "message")
	httpErr := apiErr.ToHTTP(http.StatusBadRequest)

	if httpErr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, httpErr.Code)
	}
	msg, ok := httpErr.Message.(*APIError)
	if !ok {
		t.Fatal("expected message to be *APIError")
	}
	if msg.Code != "code" {
		t.Errorf("expected code 'code', got '%s'", msg.Code)
	}
}

func TestBadRequest(t *testing.T) {
	err := BadRequest("bad", "bad request")
	assertHTTPError(t, err, http.StatusBadRequest, "bad", "bad request")
}

func TestUnauthorized(t *testing.T) {
	err := Unauthorized("auth", "unauthorized")
	assertHTTPError(t, err, http.StatusUnauthorized, "auth", "unauthorized")
}

func TestForbidden(t *testing.T) {
	err := Forbidden("forbid", "forbidden")
	assertHTTPError(t, err, http.StatusForbidden, "forbid", "forbidden")
}

func TestNotFound(t *testing.T) {
	err := NotFound("notfound", "not found")
	assertHTTPError(t, err, http.StatusNotFound, "notfound", "not found")
}

func TestConflict(t *testing.T) {
	err := Conflict("conflict", "conflict error")
	assertHTTPError(t, err, http.StatusConflict, "conflict", "conflict error")
}

func TestInternalError(t *testing.T) {
	err := InternalError("internal", "internal error")
	assertHTTPError(t, err, http.StatusInternalServerError, "internal", "internal error")
}

func assertHTTPError(t *testing.T, err *echo.HTTPError, expectedStatus int, expectedCode, expectedMessage string) {
	t.Helper()
	if err.Code != expectedStatus {
		t.Errorf("expected status %d, got %d", expectedStatus, err.Code)
	}
	apiErr, ok := err.Message.(*APIError)
	if !ok {
		t.Fatal("expected message to be *APIError")
	}
	if apiErr.Code != expectedCode {
		t.Errorf("expected code '%s', got '%s'", expectedCode, apiErr.Code)
	}
	if apiErr.Message != expectedMessage {
		t.Errorf("expected message '%s', got '%s'", expectedMessage, apiErr.Message)
	}
}
