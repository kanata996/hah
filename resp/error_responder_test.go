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

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `NewErrorResponder` 与零值 `ErrorResponder` 的默认 fallback 契约。
// - [✓] `ErrorResponder` 的自定义 `Logger` / `AsHTTPError` / `ContextAttrs` / `RequestLogAttrs` / `AnnotateRequestLog` 会通过 `Respond` 生效。
// - [✓] `ErrorResponder` 仅在 5xx 请求日志补低噪音 `error.*` 字段，并对 canceled / timeout 保持稳定标记；4xx 不补这些字段也不额外打独立错误日志。

func TestNewErrorResponderRespondUsesDefaultFallbacks(t *testing.T) {
	responder := NewErrorResponder()
	if responder == nil {
		t.Fatal("NewErrorResponder() = nil")
	}

	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	req := httptest.NewRequest(http.MethodGet, "/timeout", nil)
	rr := httptest.NewRecorder()

	if err := responder.Respond(rr, req, context.DeadlineExceeded); err != nil {
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

	if defaultBuf.Len() == 0 {
		t.Fatal("default logger did not capture output")
	}

	logEntry := decodePayload(t, defaultBuf.Bytes())
	if got := logEntry["msg"]; got != "resp: request failed with server error" {
		t.Fatalf("msg = %#v, want resp: request failed with server error", got)
	}
	if got := logEntry["error.code"]; got != "timeout" {
		t.Fatalf("error.code = %#v, want timeout", got)
	}
	if got := logEntry["error.timeout"]; got != true {
		t.Fatalf("error.timeout = %#v, want true", got)
	}
}

func TestZeroValueErrorResponderRespondUsesDefaultFallbacks(t *testing.T) {
	var responder ErrorResponder

	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rr := httptest.NewRecorder()

	if err := responder.Respond(rr, req, errors.New("boom")); err != nil {
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

	if defaultBuf.Len() == 0 {
		t.Fatal("default logger did not capture output")
	}

	logEntry := decodePayload(t, defaultBuf.Bytes())
	if got := logEntry["msg"]; got != "resp: request failed with server error" {
		t.Fatalf("msg = %#v, want resp: request failed with server error", got)
	}
	if got := logEntry["error.code"]; got != "internal_error" {
		t.Fatalf("error.code = %#v, want internal_error", got)
	}
}

func TestErrorResponderRespondUsesCustomHooks(t *testing.T) {
	type requestContextKey struct{}

	inputErr := errors.New("boom")
	customHTTPError := errx.NewHTTPError(http.StatusBadGateway, "upstream_failure", "upstream failure")
	captured := &capturedRequestLog{}

	var customBuf bytes.Buffer
	customLogger := slog.New(slog.NewJSONHandler(&customBuf, nil))

	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	responder := &ErrorResponder{
		Logger: customLogger,
		AsHTTPError: func(err error) *errx.HTTPError {
			if !errors.Is(err, inputErr) {
				t.Fatalf("AsHTTPError() err = %v, want %v", err, inputErr)
			}
			return customHTTPError
		},
		ContextAttrs: func(ctx context.Context) []slog.Attr {
			if got := ctx.Value(requestContextKey{}); got != "trace-123" {
				t.Fatalf("ContextAttrs() context value = %#v, want trace-123", got)
			}
			return []slog.Attr{slog.String("traceId", "trace-123")}
		},
		AnnotateRequestLog: captured.annotate,
		RequestLogAttrs: func(err error, httpErr *errx.HTTPError) []slog.Attr {
			if !errors.Is(err, inputErr) {
				t.Fatalf("RequestLogAttrs() err = %v, want %v", err, inputErr)
			}
			if httpErr != customHTTPError {
				t.Fatalf("RequestLogAttrs() httpErr = %p, want %p", httpErr, customHTTPError)
			}
			return []slog.Attr{slog.String("service", "resp")}
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/custom", nil).WithContext(
		context.WithValue(context.Background(), requestContextKey{}, "trace-123"),
	)
	rr := httptest.NewRecorder()

	if err := responder.Respond(rr, req, inputErr); err != nil {
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

	if captured.req != req {
		t.Fatalf("annotated request = %p, want %p", captured.req, req)
	}
	requestLogEntry := attrsToMap(captured.attrs)
	if got := requestLogEntry["service"]; got != "resp" {
		t.Fatalf("request log service = %#v, want resp", got)
	}
	if _, exists := requestLogEntry["error.code"]; exists {
		t.Fatalf("request log unexpectedly used default attrs: %#v", requestLogEntry)
	}

	if defaultBuf.Len() != 0 {
		t.Fatalf("default logger unexpectedly captured output: %s", defaultBuf.Bytes())
	}
	if customBuf.Len() == 0 {
		t.Fatal("custom logger did not capture output")
	}

	logEntry := decodePayload(t, customBuf.Bytes())
	if got := logEntry["msg"]; got != "resp: request failed with server error" {
		t.Fatalf("msg = %#v, want resp: request failed with server error", got)
	}
	if got := logEntry["error.code"]; got != "upstream_failure" {
		t.Fatalf("error.code = %#v, want upstream_failure", got)
	}
	if got := logEntry["traceId"]; got != "trace-123" {
		t.Fatalf("traceId = %#v, want trace-123", got)
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
