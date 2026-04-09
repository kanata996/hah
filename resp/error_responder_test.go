package resp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kanata996/hah/errx"
)

func TestErrorResponderContextAttrs(t *testing.T) {
	var responder *ErrorResponder
	if got := responder.contextAttrs(nilContext()); got != nil {
		t.Fatalf("contextAttrs(nil responder) = %#v, want nil", got)
	}

	responder = &ErrorResponder{
		ContextAttrs: func(context.Context) []slog.Attr {
			return []slog.Attr{
				slog.String("traceId", "trace-123"),
			}
		},
	}

	attrs := attrsToMap(responder.contextAttrs(context.Background()))
	if got := attrs["traceId"]; got != "trace-123" {
		t.Fatalf("traceId = %#v, want trace-123", got)
	}
}

func TestNewErrorResponderAndOverrides(t *testing.T) {
	responder := NewErrorResponder()
	if responder == nil {
		t.Fatal("NewErrorResponder() = nil")
	}

	defaultLogger := slog.Default()
	if got := responder.logger(); got != defaultLogger {
		t.Fatalf("logger() = %p, want default %p", got, defaultLogger)
	}

	customLogger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	responder.Logger = customLogger
	if got := responder.logger(); got != customLogger {
		t.Fatalf("logger() = %p, want custom %p", got, customLogger)
	}

	cause := errors.New("boom")
	customHTTPError := errx.BadRequest("bad_request", "bad request")
	responder.AsHTTPError = func(err error) *errx.HTTPError {
		if !errors.Is(err, cause) {
			t.Fatalf("AsHTTPError() err = %v, want %v", err, cause)
		}
		return customHTTPError
	}
	if got := responder.httpError(cause); got != customHTTPError {
		t.Fatalf("httpError() = %p, want %p", got, customHTTPError)
	}

	customAttrs := []slog.Attr{slog.String("service", "resp")}
	responder.RequestLogAttrs = func(err error, httpErr *errx.HTTPError) []slog.Attr {
		if !errors.Is(err, cause) {
			t.Fatalf("RequestLogAttrs err = %v, want %v", err, cause)
		}
		if httpErr != customHTTPError {
			t.Fatalf("RequestLogAttrs httpErr = %p, want %p", httpErr, customHTTPError)
		}
		return customAttrs
	}
	if got := responder.requestLogAttrs(cause, customHTTPError); len(got) != 1 || got[0].Key != "service" {
		t.Fatalf("requestLogAttrs() = %#v, want custom attrs", got)
	}
}

// 5xx 错误会给请求日志补充低噪音诊断字段，但不写入关联字段或详细诊断链。
func TestErrorResponderEnrichesRequestLog(t *testing.T) {
	captured := &capturedRequestLog{}
	responder := &ErrorResponder{
		AnnotateRequestLog: captured.annotate,
	}

	req := httptest.NewRequest(http.MethodGet, "/users/u_123", nil)
	rr := httptest.NewRecorder()

	rawErr := &rawTestError{message: "db timeout"}
	err := errx.NewHTTPErrorWithCause(
		http.StatusInternalServerError,
		"internal_error",
		"",
		&wrappedTestError{op: "load user", err: rawErr},
		map[string]any{"field": "name", "code": "required"},
	)
	if got := responder.Respond(rr, req, err); got != nil {
		t.Fatalf("Respond() error = %v", got)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if captured.req != req {
		t.Fatalf("annotated request = %p, want %p", captured.req, req)
	}

	logEntry := attrsToMap(captured.attrs)
	if got := logEntry["error.code"]; got != "internal_error" {
		t.Fatalf("error.code = %#v, want internal_error", got)
	}
	for _, key := range []string{
		"request.id",
		"traceId",
		"error.timeout",
		"error.canceled",
		"error.message",
		"error.type",
		"error.root_message",
		"error.root_type",
		"error.details",
		"error.chain",
		"error.chain_types",
		"error.public_message",
		"error.expected",
		"error.category",
		"error.details_count",
	} {
		if _, exists := logEntry[key]; exists {
			t.Fatalf("%s unexpectedly present: %#v", key, logEntry[key])
		}
	}
}

// request log 只保留低噪音字段，不会镜像包装错误文本或类型。
func TestErrorResponderEnrichesRequestLogFromWrappedHTTPErrorWithoutCause(t *testing.T) {
	captured := &capturedRequestLog{}
	responder := &ErrorResponder{
		AnnotateRequestLog: captured.annotate,
	}

	req := httptest.NewRequest(http.MethodGet, "/wrapped", nil)
	rr := httptest.NewRecorder()
	err := fmt.Errorf("handler failed: %w", errx.NewHTTPError(
		http.StatusInternalServerError,
		"internal_error",
		"",
	))
	if got := responder.Respond(rr, req, err); got != nil {
		t.Fatalf("Respond() error = %v", got)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	logEntry := attrsToMap(captured.attrs)
	if got := logEntry["error.code"]; got != "internal_error" {
		t.Fatalf("error.code = %#v, want internal_error", got)
	}
	for _, key := range []string{"error.message", "error.type", "error.root_message", "error.root_type"} {
		if _, exists := logEntry[key]; exists {
			t.Fatalf("%s unexpectedly present: %#v", key, logEntry[key])
		}
	}
}

// 5xx 请求日志会显式标记超时错误，便于和普通内部错误区分。
func TestErrorResponderEnrichesRequestLogWithTimeoutFlag(t *testing.T) {
	captured := &capturedRequestLog{}
	responder := &ErrorResponder{
		AnnotateRequestLog: captured.annotate,
	}

	req := httptest.NewRequest(http.MethodGet, "/timeout", nil)
	rr := httptest.NewRecorder()
	err := errx.NewHTTPErrorWithCause(
		http.StatusInternalServerError,
		"internal_error",
		"",
		context.DeadlineExceeded,
	)
	if got := responder.Respond(rr, req, err); got != nil {
		t.Fatalf("Respond() error = %v", got)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	logEntry := attrsToMap(captured.attrs)
	if got := logEntry["error.timeout"]; got != true {
		t.Fatalf("error.timeout = %#v, want true", got)
	}
	if _, exists := logEntry["error.canceled"]; exists {
		t.Fatalf("error.canceled unexpectedly present: %#v", logEntry["error.canceled"])
	}
}

// 5xx 请求日志会显式标记 canceled 错误，即使公开响应仍是 500。
func TestErrorResponderEnrichesRequestLogWithCanceledFlag(t *testing.T) {
	captured := &capturedRequestLog{}
	responder := &ErrorResponder{
		AnnotateRequestLog: captured.annotate,
	}

	req := httptest.NewRequest(http.MethodGet, "/canceled", nil)
	rr := httptest.NewRecorder()
	err := errx.NewHTTPErrorWithCause(
		http.StatusInternalServerError,
		"internal_error",
		"",
		context.Canceled,
	)
	if got := responder.Respond(rr, req, err); got != nil {
		t.Fatalf("Respond() error = %v", got)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	logEntry := attrsToMap(captured.attrs)
	if got := logEntry["error.canceled"]; got != true {
		t.Fatalf("error.canceled = %#v, want true", got)
	}
	if _, exists := logEntry["error.timeout"]; exists {
		t.Fatalf("error.timeout unexpectedly present: %#v", logEntry["error.timeout"])
	}
}

// 4xx 错误不会污染请求日志的 error.* 诊断字段，也不会额外打独立 error log。
func TestErrorResponderDoesNotEnrichRequestLogFor4xx(t *testing.T) {
	captured := &capturedRequestLog{}

	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	responder := &ErrorResponder{
		AnnotateRequestLog: captured.annotate,
	}

	req := httptest.NewRequest(http.MethodGet, "/users/u_123", nil)
	rr := httptest.NewRecorder()
	err := errx.BadRequest("bad_request", "bad request", map[string]any{
		"field": "name",
		"code":  "required",
	})
	if got := responder.Respond(rr, req, err); got != nil {
		t.Fatalf("Respond() error = %v", got)
	}

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if len(captured.attrs) != 0 {
		t.Fatalf("request log attrs = %#v, want none", captured.attrs)
	}
	if defaultBuf.Len() != 0 {
		t.Fatalf("default logger unexpectedly captured output: %s", defaultBuf.Bytes())
	}
}
