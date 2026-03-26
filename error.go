package hah

import (
	"net/http"

	"github.com/kanata996/hah/errcode"
)

// HTTPError is the standardized public error representation used at the HTTP
// boundary.
type HTTPError struct {
	status  int
	code    string
	message string
	details []any
}

// NewHTTPError constructs a public HTTP error.
func NewHTTPError(status int, code, message string, details ...any) *HTTPError {
	return &HTTPError{
		status:  normalizeStatus(status),
		code:    normalizeCode(status, code),
		message: normalizeMessage(status, message),
		details: cloneDetails(details),
	}
}

// BadRequest constructs a 400 public HTTP error.
func BadRequest(code, message string, details ...any) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, code, message, details...)
}

// Unauthorized constructs a 401 public HTTP error.
func Unauthorized(code, message string, details ...any) *HTTPError {
	return NewHTTPError(http.StatusUnauthorized, code, message, details...)
}

// Forbidden constructs a 403 public HTTP error.
func Forbidden(code, message string, details ...any) *HTTPError {
	return NewHTTPError(http.StatusForbidden, code, message, details...)
}

// NotFound constructs a 404 public HTTP error.
func NotFound(code, message string, details ...any) *HTTPError {
	return NewHTTPError(http.StatusNotFound, code, message, details...)
}

// MethodNotAllowed constructs a 405 public HTTP error.
func MethodNotAllowed(code, message string, details ...any) *HTTPError {
	return NewHTTPError(http.StatusMethodNotAllowed, code, message, details...)
}

// Conflict constructs a 409 public HTTP error.
func Conflict(code, message string, details ...any) *HTTPError {
	return NewHTTPError(http.StatusConflict, code, message, details...)
}

// Gone constructs a 410 public HTTP error.
func Gone(code, message string, details ...any) *HTTPError {
	return NewHTTPError(http.StatusGone, code, message, details...)
}

// UnprocessableEntity constructs a 422 public HTTP error.
func UnprocessableEntity(code, message string, details ...any) *HTTPError {
	return NewHTTPError(http.StatusUnprocessableEntity, code, message, details...)
}

// TooManyRequests constructs a 429 public HTTP error.
func TooManyRequests(code, message string, details ...any) *HTTPError {
	return NewHTTPError(http.StatusTooManyRequests, code, message, details...)
}

func normalizeStatus(status int) int {
	if status < 400 || status > 599 {
		return http.StatusInternalServerError
	}
	return status
}

func normalizeCode(status int, code string) string {
	if code != "" {
		return code
	}

	if status >= 400 && status <= 499 {
		return errcode.ClientError
	}

	return errcode.InternalError
}

func normalizeMessage(status int, message string) string {
	if message != "" {
		return message
	}

	if status >= 400 && status <= 499 {
		return "client error"
	}

	return "internal server error"
}

func cloneDetails(details []any) []any {
	if len(details) == 0 {
		return nil
	}

	cloned := make([]any, len(details))
	copy(cloned, details)
	return cloned
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

// Status returns the public HTTP status code.
func (e *HTTPError) Status() int {
	if e == nil {
		return http.StatusInternalServerError
	}
	return e.status
}

// Code returns the stable machine-readable error code.
func (e *HTTPError) Code() string {
	if e == nil {
		return normalizeCode(http.StatusInternalServerError, "")
	}
	return e.code
}

// Message returns the safe public error message.
func (e *HTTPError) Message() string {
	if e == nil {
		return normalizeMessage(http.StatusInternalServerError, "")
	}
	return e.message
}

// Details returns a copy of the structured error details.
func (e *HTTPError) Details() []any {
	if e == nil || len(e.details) == 0 {
		return nil
	}
	return cloneDetails(e.details)
}
