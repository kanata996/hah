package hah

import (
	"errors"
	"net/http"
	"testing"

	"github.com/kanata996/hah/internal/errx"
)

func TestNewErrorDefaultsClientResponses(t *testing.T) {
	err := NewHTTPError(http.StatusConflict, "", "")

	if got := err.Status(); got != http.StatusConflict {
		t.Fatalf("status = %d, want %d", got, http.StatusConflict)
	}
	if got := err.Code(); got != "client_error" {
		t.Fatalf("code = %q, want client_error", got)
	}
	if got := err.Message(); got != "client error" {
		t.Fatalf("message = %q, want client error", got)
	}
	if got := err.Error(); got != "client error" {
		t.Fatalf("error string = %q, want client error", got)
	}
}

func TestNewErrorDefaultsInternalResponses(t *testing.T) {
	err := NewHTTPError(200, "", "")

	if got := err.Status(); got != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", got, http.StatusInternalServerError)
	}
	if got := err.Code(); got != "internal_error" {
		t.Fatalf("code = %q, want internal_error", got)
	}
	if got := err.Message(); got != "internal server error" {
		t.Fatalf("message = %q, want internal server error", got)
	}
}

func TestCommonHTTPErrorHelpers(t *testing.T) {
	tests := []struct {
		name   string
		build  func(code, message string, details ...any) *HTTPError
		status int
	}{
		{name: "bad request", build: BadRequest, status: http.StatusBadRequest},
		{name: "unauthorized", build: Unauthorized, status: http.StatusUnauthorized},
		{name: "forbidden", build: Forbidden, status: http.StatusForbidden},
		{name: "not found", build: NotFound, status: http.StatusNotFound},
		{name: "method not allowed", build: MethodNotAllowed, status: http.StatusMethodNotAllowed},
		{name: "conflict", build: Conflict, status: http.StatusConflict},
		{name: "gone", build: Gone, status: http.StatusGone},
		{name: "unprocessable entity", build: UnprocessableEntity, status: http.StatusUnprocessableEntity},
		{name: "too many requests", build: TooManyRequests, status: http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.build("sample_code", "sample message", map[string]any{"field": "value"})

			if got := err.Status(); got != tt.status {
				t.Fatalf("status = %d, want %d", got, tt.status)
			}
			if got := err.Code(); got != "sample_code" {
				t.Fatalf("code = %q, want sample_code", got)
			}
			if got := err.Message(); got != "sample message" {
				t.Fatalf("message = %q, want sample message", got)
			}
			if details := err.Details(); len(details) != 1 {
				t.Fatalf("details len = %d, want 1", len(details))
			}
		})
	}
}

func TestErrorDetailsReturnsCopy(t *testing.T) {
	err := NewHTTPError(http.StatusConflict, "conflict", "conflict", map[string]any{"field": "email"})

	details := err.Details()
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}

	details[0] = "mutated"

	again := err.Details()
	if got, ok := again[0].(map[string]any); !ok || got["field"] != "email" {
		t.Fatalf("details should be copied, got %#v", again[0])
	}
}

func TestNilErrorAccessorsUseInternalDefaults(t *testing.T) {
	var err *HTTPError

	if got := err.Status(); got != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", got, http.StatusInternalServerError)
	}
	if got := err.Code(); got != "internal_error" {
		t.Fatalf("code = %q, want internal_error", got)
	}
	if got := err.Message(); got != "internal server error" {
		t.Fatalf("message = %q, want internal server error", got)
	}
	if got := err.Error(); got != "" {
		t.Fatalf("error string = %q, want empty", got)
	}
	if got := err.Details(); got != nil {
		t.Fatalf("details = %#v, want nil", got)
	}
}

func TestWithStageWrapsBoundaryErrorWithoutChangingPublicError(t *testing.T) {
	base := NewHTTPError(http.StatusConflict, "conflict", "conflict")
	tagged := errx.WithStage(base, errx.StageProcessing)

	var boundaryErr *HTTPError
	if !errors.As(tagged, &boundaryErr) {
		t.Fatalf("errors.As(tagged, *HTTPError) = false, want true")
	}
	if boundaryErr != base {
		t.Fatalf("boundaryErr = %#v, want original error %#v", boundaryErr, base)
	}
	if got := errx.From(tagged).Stage; got != errx.StageProcessing {
		t.Fatalf("observed stage = %q, want processing", got)
	}
	if got := errx.From(base).Stage; got != "" {
		t.Fatalf("base stage = %q, want empty", got)
	}
}
