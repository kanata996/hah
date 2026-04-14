package resp

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

func TestAsHTTPErrorNormalizesCommonErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		input      error
		wantStatus int
		wantCode   string
		wantTitle  string
		wantDetail string
	}{
		{
			name:       "nil",
			input:      nil,
			wantStatus: 0,
		},
		{
			name:       "wrapped http error",
			input:      errors.Join(errors.New("handler failed"), errx.NewHTTPError(http.StatusBadRequest, "bad_request", "bad request")),
			wantStatus: http.StatusBadRequest,
			wantCode:   "bad_request",
			wantTitle:  http.StatusText(http.StatusBadRequest),
			wantDetail: "bad request",
		},
		{
			name:       "context canceled",
			input:      context.Canceled,
			wantStatus: 499,
			wantCode:   "client_closed_request",
			wantTitle:  "Client Closed Request",
			wantDetail: "Client Closed Request",
		},
		{
			name:       "deadline exceeded",
			input:      context.DeadlineExceeded,
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "timeout",
			wantTitle:  http.StatusText(http.StatusGatewayTimeout),
			wantDetail: http.StatusText(http.StatusGatewayTimeout),
		},
		{
			name:       "generic error",
			input:      errors.New("db timeout"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			wantTitle:  http.StatusText(http.StatusInternalServerError),
			wantDetail: http.StatusText(http.StatusInternalServerError),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := AsHTTPError(tc.input)
			if tc.wantStatus == 0 {
				if got != nil {
					t.Fatalf("AsHTTPError() = %#v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("AsHTTPError() = nil")
			}
			if got.Status() != tc.wantStatus {
				t.Fatalf("status = %d, want %d", got.Status(), tc.wantStatus)
			}
			if got.Code() != tc.wantCode {
				t.Fatalf("code = %q, want %q", got.Code(), tc.wantCode)
			}
			if got.Title() != tc.wantTitle {
				t.Fatalf("title = %q, want %q", got.Title(), tc.wantTitle)
			}
			if got.Detail() != tc.wantDetail {
				t.Fatalf("detail = %q, want %q", got.Detail(), tc.wantDetail)
			}
		})
	}
}

func TestNewErrorResponderRespondUsesDefaultFallbacksWithoutLogging(t *testing.T) {
	responder := NewErrorResponder()
	if responder == nil {
		t.Fatal("NewErrorResponder() = nil")
	}

	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	rr := httptest.NewRecorder()

	if err := responder.Respond(rr, context.DeadlineExceeded); err != nil {
		t.Fatalf("Respond() error = %v", err)
	}

	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusGatewayTimeout)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "timeout" {
		t.Fatalf("code = %#v, want timeout", got)
	}
	if got := body["title"]; got != http.StatusText(http.StatusGatewayTimeout) {
		t.Fatalf("title = %#v, want %q", got, http.StatusText(http.StatusGatewayTimeout))
	}
	if got := body["detail"]; got != http.StatusText(http.StatusGatewayTimeout) {
		t.Fatalf("detail = %#v, want %q", got, http.StatusText(http.StatusGatewayTimeout))
	}
	if defaultBuf.Len() != 0 {
		t.Fatalf("default logger unexpectedly captured output: %s", defaultBuf.Bytes())
	}
}

func TestZeroValueErrorResponderRespondUsesDefaultFallbacksWithoutLogging(t *testing.T) {
	var responder ErrorResponder

	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	rr := httptest.NewRecorder()

	if err := responder.Respond(rr, errors.New("boom")); err != nil {
		t.Fatalf("Respond() error = %v", err)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "internal_error" {
		t.Fatalf("code = %#v, want internal_error", got)
	}
	if got := body["title"]; got != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("title = %#v, want %q", got, http.StatusText(http.StatusInternalServerError))
	}
	if got := body["detail"]; got != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("detail = %#v, want %q", got, http.StatusText(http.StatusInternalServerError))
	}
	if defaultBuf.Len() != 0 {
		t.Fatalf("default logger unexpectedly captured output: %s", defaultBuf.Bytes())
	}
}

func TestErrorResponderRespondUsesCustomAsHTTPError(t *testing.T) {
	inputErr := errors.New("boom")
	customHTTPError := errx.NewHTTPError(http.StatusBadGateway, "upstream_failure", "upstream failure")
	responder := &ErrorResponder{
		AsHTTPError: func(err error) *errx.HTTPError {
			if !errors.Is(err, inputErr) {
				t.Fatalf("AsHTTPError() err = %v, want %v", err, inputErr)
			}
			return customHTTPError
		},
	}

	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	rr := httptest.NewRecorder()

	if err := responder.Respond(rr, inputErr); err != nil {
		t.Fatalf("Respond() error = %v", err)
	}

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadGateway)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "upstream_failure" {
		t.Fatalf("code = %#v, want upstream_failure", got)
	}
	if got := body["title"]; got != http.StatusText(http.StatusBadGateway) {
		t.Fatalf("title = %#v, want %q", got, http.StatusText(http.StatusBadGateway))
	}
	if got := body["detail"]; got != "upstream failure" {
		t.Fatalf("detail = %#v, want upstream failure", got)
	}
	if defaultBuf.Len() != 0 {
		t.Fatalf("default logger unexpectedly captured output: %s", defaultBuf.Bytes())
	}
}

func TestErrorResponderRespondFallsBackWhenCustomAsHTTPErrorReturnsNil(t *testing.T) {
	responder := &ErrorResponder{
		AsHTTPError: func(error) *errx.HTTPError {
			return nil
		},
	}

	rr := httptest.NewRecorder()

	if err := responder.Respond(rr, errors.New("boom")); err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "internal_error" {
		t.Fatalf("code = %#v, want internal_error", got)
	}
}

func TestErrorResponderRespondPropagatesAsHTTPErrorPanics(t *testing.T) {
	responder := &ErrorResponder{
		AsHTTPError: func(error) *errx.HTTPError {
			panic("boom")
		},
	}

	rr := httptest.NewRecorder()

	assertPanicsWithValue(t, "boom", func() {
		_ = responder.Respond(rr, errors.New("db timeout"))
	})
}

func TestErrorResponderRespondNilErrorIsNoop(t *testing.T) {
	responder := NewErrorResponder()
	rr := httptest.NewRecorder()

	if err := responder.Respond(rr, nil); err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want recorder default %d", rr.Code, http.StatusOK)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
}

func TestErrorResponderRespondRejectsNilWriter(t *testing.T) {
	responder := NewErrorResponder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Respond() panicked: %v", recovered)
		}
	}()

	err := responder.Respond(nil, errors.New("db timeout"))
	if err == nil || !strings.Contains(err.Error(), "response writer is nil") {
		t.Fatalf("Respond() error = %v, want response writer is nil", err)
	}
}

func assertPanicsWithValue(t *testing.T, want any, fn func()) {
	t.Helper()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic, got nil")
		}
		if recovered != want {
			t.Fatalf("panic = %#v, want %#v", recovered, want)
		}
	}()

	fn()
}
