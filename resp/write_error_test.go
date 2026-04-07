package resp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/go-chi/traceid"
	"github.com/kanata996/hah/errx"
)

// 测试清单：
// [✓] WriteError 会把 HTTPError 写成稳定的 problem JSON，并保留显式公共字段
// [✓] nil error 是 no-op；nil request 与 nil writer 都能安全退化而不 panic
// [✓] HEAD 请求只写状态和头；非法状态和未知错误会统一收敛且不泄漏内部 cause
// [✓] context canceled / deadline exceeded 会映射为公开错误语义
// [✓] 公开 errors 无法编码或 panic 时会降级丢弃该字段，并把降级信息返回给调用方
// [✓] 响应已经开始写出时不会被二次改写，但 5xx 仍会输出独立错误日志
// [✓] asHTTPError 会保留 HTTPError，并把 context/普通 error 收敛为稳定公共语义
// [✓] problem payload 会按 includeErrors 开关决定是否暴露公开 errors
// [✓] 5xx 请求日志只补充低噪音关联字段并输出独立错误日志，4xx 不会；错误响应写出失败会记录独立日志

type failingWriter struct {
	header http.Header
	status int
	writes int
}

type rawTestError struct {
	message string
}

func (e *rawTestError) Error() string {
	return e.message
}

type wrappedTestError struct {
	op  string
	err error
}

type countingError struct {
	calls *int
}

type panicJSONDetail struct{}

func (panicJSONDetail) MarshalJSON() ([]byte, error) {
	panic("panic during MarshalJSON")
}

func (e *wrappedTestError) Error() string {
	return fmt.Sprintf("%s: %v", e.op, e.err)
}

func (e *wrappedTestError) Unwrap() error {
	return e.err
}

func (e *countingError) Error() string {
	if e != nil && e.calls != nil {
		*e.calls = *e.calls + 1
	}
	return "counting error"
}

func (w *failingWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingWriter) WriteHeader(status int) {
	w.status = status
}

func (w *failingWriter) Write(_ []byte) (int, error) {
	w.writes++
	return 0, errors.New("socket closed")
}

// WriteError 会把 HTTPError 写成标准 problem JSON。
func TestWriteErrorWritesEnvelope(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := WriteError(rr, req, errx.NewHTTPError(
		http.StatusUnprocessableEntity,
		"",
		"",
		map[string]any{"field": "name", "code": "required"},
	))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnprocessableEntity)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "unprocessable_entity" {
		t.Fatalf("code = %#v, want unprocessable_entity", got)
	}
	if got := body["title"]; got != "Unprocessable Entity" {
		t.Fatalf("title = %#v, want Unprocessable Entity", got)
	}
	if got := body["status"]; got != float64(http.StatusUnprocessableEntity) {
		t.Fatalf("status = %#v, want %d", got, http.StatusUnprocessableEntity)
	}
	if got := body["detail"]; got != "Unprocessable Entity" {
		t.Fatalf("detail = %#v, want Unprocessable Entity", got)
	}
	errors, ok := body["errors"].([]any)
	if !ok || len(errors) != 1 {
		t.Fatalf("errors = %#v, want 1 item", body["errors"])
	}
	assertPublicErrorObject(t, errors[0], map[string]any{
		"field": "name",
		"code":  "required",
	})
}

// 显式传入的公共 code/detail/errors 应原样进入 problem JSON，而不是被默认值覆盖。
func TestWriteErrorPreservesExplicitPublicFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	rr := httptest.NewRecorder()

	err := WriteError(rr, req, errx.NewHTTPError(
		http.StatusBadRequest,
		"invalid_json",
		"payload invalid",
		map[string]any{"field": "name", "code": "required"},
	))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "invalid_json" {
		t.Fatalf("code = %#v, want invalid_json", got)
	}
	if got := body["title"]; got != http.StatusText(http.StatusBadRequest) {
		t.Fatalf("title = %#v, want %q", got, http.StatusText(http.StatusBadRequest))
	}
	if got := body["detail"]; got != "payload invalid" {
		t.Fatalf("detail = %#v, want payload invalid", got)
	}
	errors, ok := body["errors"].([]any)
	if !ok || len(errors) != 1 {
		t.Fatalf("errors = %#v, want 1 item", body["errors"])
	}
	assertPublicErrorObject(t, errors[0], map[string]any{
		"field": "name",
		"code":  "required",
	})
}

// 传入 nil 错误时，WriteError 应是纯 no-op。
func TestWriteErrorNilErrorIsNoop(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := WriteError(rr, httptest.NewRequest(http.MethodGet, "/", nil), nil); err != nil {
		t.Fatalf("WriteError() error = %v", err)
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
	if len(rr.Header()) != 0 {
		t.Fatalf("headers = %#v, want empty", rr.Header())
	}
}

// request 为空时，WriteError 也应写出稳定的公共错误响应而不是 panic。
func TestWriteErrorNilRequestStillWritesErrorResponse(t *testing.T) {
	rr := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("WriteError() panicked: %v", recovered)
		}
	}()

	if err := WriteError(rr, nil, errors.New("db timeout")); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "internal_error" {
		t.Fatalf("code = %#v, want internal_error", got)
	}
	if got := body["title"]; got != "Internal Server Error" {
		t.Fatalf("title = %#v, want Internal Server Error", got)
	}
	if got := body["detail"]; got != "Internal Server Error" {
		t.Fatalf("detail = %#v, want Internal Server Error", got)
	}
}

// ResponseWriter 为空时，WriteError 会把底层写回失败作为普通 error 返回。
func TestWriteErrorRejectsNilWriter(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("WriteError() panicked: %v", recovered)
		}
	}()

	err := WriteError(nil, httptest.NewRequest(http.MethodGet, "/", nil), errors.New("db timeout"))
	if err == nil || !strings.Contains(err.Error(), "response writer is nil") {
		t.Fatalf("WriteError() error = %v, want response writer is nil", err)
	}
}

// HEAD 请求写错误时只写状态和头，不写响应体。
func TestWriteErrorHeadWritesStatusWithoutBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rr := httptest.NewRecorder()

	err := WriteError(rr, req, errx.NewHTTPError(http.StatusBadRequest, "", "", "detail"))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
}

// 非法状态码会被标准化为 500，且内部 cause 不会泄漏到公开响应。
func TestWriteErrorNormalizesInvalidStatusAndHidesCause(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := WriteError(rr, req, errx.NewHTTPErrorWithCause(99, "", "", errors.New("db timeout")))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "internal_error" {
		t.Fatalf("code = %#v, want internal_error", got)
	}
	if got := body["title"]; got != "Internal Server Error" {
		t.Fatalf("title = %#v, want Internal Server Error", got)
	}
	if _, exists := body["errors"]; exists {
		t.Fatalf("errors unexpectedly present: %#v", body["errors"])
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("db timeout")) {
		t.Fatalf("body leaked internal cause: %q", rr.Body.String())
	}
}

// context.Canceled 会映射为对外可见的 client closed request 错误。
func TestWriteErrorMapsContextCanceled(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := WriteError(rr, req, context.Canceled)
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if rr.Code != 499 {
		t.Fatalf("status = %d, want 499", rr.Code)
	}
	if got := body["code"]; got != "client_closed_request" {
		t.Fatalf("code = %#v, want client_closed_request", got)
	}
	if got := body["title"]; got != "Client Closed Request" {
		t.Fatalf("title = %#v, want Client Closed Request", got)
	}
}

// context.DeadlineExceeded 会映射为对外可见的超时错误。
func TestWriteErrorMapsContextDeadlineExceeded(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := WriteError(rr, req, context.DeadlineExceeded)
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusGatewayTimeout)
	}
	if got := body["code"]; got != "timeout" {
		t.Fatalf("code = %#v, want timeout", got)
	}
	if got := body["title"]; got != http.StatusText(http.StatusGatewayTimeout) {
		t.Fatalf("title = %#v, want %q", got, http.StatusText(http.StatusGatewayTimeout))
	}
	if got := body["detail"]; got != http.StatusText(http.StatusGatewayTimeout) {
		t.Fatalf("detail = %#v, want %q", got, http.StatusText(http.StatusGatewayTimeout))
	}
}

// 未知普通错误会统一降级为 500 internal_error。
func TestWriteErrorMapsUnknownErrorToInternalError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := WriteError(rr, req, errors.New("db timeout"))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "internal_error" {
		t.Fatalf("code = %#v, want internal_error", got)
	}
	if got := body["title"]; got != "Internal Server Error" {
		t.Fatalf("title = %#v, want Internal Server Error", got)
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("db timeout")) {
		t.Fatalf("body leaked internal cause: %q", rr.Body.String())
	}
}

// 公开 details 不可编码时，会丢弃 details 并返回降级写回错误。
func TestWriteErrorDropsUnencodableDetails(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := WriteError(rr, req, errx.NewHTTPError(
		http.StatusBadRequest,
		"bad_request",
		"bad request",
		func() {},
	))
	if err == nil {
		t.Fatal("expected degraded write error, got nil")
	}

	var degraded *ErrorWriteDegraded
	if !errors.As(err, &degraded) || degraded == nil {
		t.Fatalf("WriteError() error = %T, want *ErrorWriteDegraded", err)
	}
	if !degraded.PreservedPublicResponse {
		t.Fatal("degraded.PreservedPublicResponse = false, want true")
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
	if _, exists := body["errors"]; exists {
		t.Fatalf("errors unexpectedly present: %#v", body["errors"])
	}
}

// details 的自定义 JSON 编码即使发生 panic，也应降级丢弃而不是把响应路径打崩。
func TestWriteErrorDropsPanickingDetails(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("WriteError() panicked: %v", recovered)
		}
	}()

	err := WriteError(rr, req, errx.NewHTTPError(
		http.StatusBadRequest,
		"bad_request",
		"bad request",
		panicJSONDetail{},
	))
	if err == nil {
		t.Fatal("expected degraded write error, got nil")
	}

	var degraded *ErrorWriteDegraded
	if !errors.As(err, &degraded) || degraded == nil {
		t.Fatalf("WriteError() error = %T, want *ErrorWriteDegraded", err)
	}
	if !degraded.PreservedPublicResponse {
		t.Fatal("degraded.PreservedPublicResponse = false, want true")
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
	if _, exists := body["errors"]; exists {
		t.Fatalf("errors unexpectedly present: %#v", body["errors"])
	}
}

func TestMarshalProblemPayloadNilHTTPError(t *testing.T) {
	body, err := marshalProblemPayload(nil)
	if err != nil {
		t.Fatalf("marshalProblemPayload(nil) error = %v", err)
	}
	payload := decodePayload(t, body)
	if got := payload["title"]; got != "" {
		t.Fatalf("title = %#v, want empty", got)
	}
	if got := payload["status"]; got != float64(0) {
		t.Fatalf("status = %#v, want 0", got)
	}
	if got := payload["detail"]; got != "" {
		t.Fatalf("detail = %#v, want empty", got)
	}
	if got := payload["code"]; got != "" {
		t.Fatalf("code = %#v, want empty", got)
	}
	if _, exists := payload["errors"]; exists {
		t.Fatalf("errors unexpectedly present: %#v", payload["errors"])
	}
}

func TestProblemPayloadFromHTTPErrorNil(t *testing.T) {
	if got := problemPayloadFromHTTPError(nil, true); got.Title != "" || got.Status != 0 || got.Detail != "" || got.Code != "" || got.Errors != nil {
		t.Fatalf("problemPayloadFromHTTPError(nil) = %#v, want zero value", got)
	}
}

// ErrorWriteDegraded 在 nil 接收者和普通错误场景下都应提供稳定的错误语义。
func TestErrorWriteDegradedMethods(t *testing.T) {
	var nilErr *ErrorWriteDegraded
	if got := nilErr.Error(); got != "resp: error response errors were dropped" {
		t.Fatalf("nil Error() = %q", got)
	}
	if got := nilErr.Unwrap(); got != nil {
		t.Fatalf("nil Unwrap() = %v, want nil", got)
	}

	cause := errors.New("json: unsupported type: func()")
	err := &ErrorWriteDegraded{Cause: cause}
	if got := err.Error(); got != "resp: error response errors were dropped: "+cause.Error() {
		t.Fatalf("Error() = %q", got)
	}
	if got := err.Unwrap(); !errors.Is(got, cause) {
		t.Fatalf("Unwrap() = %v, want %v", got, cause)
	}
}

// asHTTPError 会保留已有 HTTPError，并对 nil 输入保持安全。
func TestAsHTTPError(t *testing.T) {
	if got := asHTTPError(nil); got != nil {
		t.Fatalf("asHTTPError(nil) = %#v, want nil", got)
	}

	httpErr := errx.NewHTTPError(http.StatusBadRequest, "bad_request", "bad request")
	if got := asHTTPError(httpErr); got != httpErr {
		t.Fatalf("asHTTPError(httpErr) = %#v, want same pointer", got)
	}
}

func TestAsHTTPErrorMapsContextAndUnknownErrors(t *testing.T) {
	canceled := asHTTPError(context.Canceled)
	if got := canceled.Status(); got != 499 {
		t.Fatalf("canceled.Status() = %d, want 499", got)
	}
	if got := canceled.Code(); got != "client_closed_request" {
		t.Fatalf("canceled.Code() = %q, want client_closed_request", got)
	}

	timeout := asHTTPError(context.DeadlineExceeded)
	if got := timeout.Status(); got != http.StatusGatewayTimeout {
		t.Fatalf("timeout.Status() = %d, want %d", got, http.StatusGatewayTimeout)
	}
	if got := timeout.Code(); got != "timeout" {
		t.Fatalf("timeout.Code() = %q, want timeout", got)
	}

	root := errors.New("db timeout")
	internal := asHTTPError(root)
	if got := internal.Status(); got != http.StatusInternalServerError {
		t.Fatalf("internal.Status() = %d, want %d", got, http.StatusInternalServerError)
	}
	if !errors.Is(internal.Unwrap(), root) {
		t.Fatalf("internal.Unwrap() = %v, want %v", internal.Unwrap(), root)
	}
}

// 底层写 HTTP 错误时，空的 HTTPError 会直接视为 no-op。
func TestWriteHTTPErrorNilHTTPErrorIsNoop(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := writeHTTPError(rr, httptest.NewRequest(http.MethodGet, "/", nil), nil); err != nil {
		t.Fatalf("writeHTTPError() error = %v, want nil", err)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want recorder default %d", rr.Code, http.StatusOK)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
}

func TestProblemPayloadFromHTTPErrorIncludeErrorsToggle(t *testing.T) {
	httpErr := errx.NewHTTPError(http.StatusBadRequest, "bad_request", "bad request", map[string]any{"field": "name"})

	withErrors := problemPayloadFromHTTPError(httpErr, true)
	if len(withErrors.Errors) != 1 {
		t.Fatalf("problemPayloadFromHTTPError(includeErrors=true).Errors = %#v, want 1 item", withErrors.Errors)
	}
	assertPublicErrorObject(t, withErrors.Errors[0], map[string]any{
		"field": "name",
	})

	withoutErrors := problemPayloadFromHTTPError(httpErr, false)
	if withoutErrors.Errors != nil {
		t.Fatalf("problemPayloadFromHTTPError(includeErrors=false).Errors = %#v, want nil", withoutErrors.Errors)
	}
}

// details 降级后如果补写响应也失败，调用方会同时拿到降级错误和写出错误。
func TestWriteErrorPayloadReturnsJoinedErrorWhenFallbackWriteFails(t *testing.T) {
	w := &failingWriter{}

	err := writeErrorPayload(w, errx.NewHTTPError(http.StatusBadRequest, "bad_request", "bad request", func() {}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var degraded *ErrorWriteDegraded
	if !errors.As(err, &degraded) {
		t.Fatalf("error = %T, want joined *ErrorWriteDegraded", err)
	}

	var writeErr *responseWriteError
	if !errors.As(err, &writeErr) {
		t.Fatalf("error = %T, want joined *responseWriteError", err)
	}
	if w.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.status, http.StatusBadRequest)
	}
	if w.writes != 1 {
		t.Fatalf("writes = %d, want 1", w.writes)
	}
}

// 响应一旦已经开始写出，WriteError 不会再次重写响应。
func TestWriteErrorSkipsRewriteAfterResponseStarted(t *testing.T) {
	w := &failingWriter{}
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	err := OK(w, nil, map[string]any{"id": "u_1"})
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if w.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.status, http.StatusOK)
	}
	if w.writes != 1 {
		t.Fatalf("writes = %d, want 1", w.writes)
	}

	writtenErr := WriteError(w, httptest.NewRequest(http.MethodGet, "/", nil), err)
	if !errors.Is(writtenErr, err) {
		t.Fatalf("WriteError() error = %v, want original error %v", writtenErr, err)
	}
	if w.writes != 1 {
		t.Fatalf("writes = %d, want still 1", w.writes)
	}
	if defaultBuf.Len() == 0 {
		t.Fatal("default logger did not capture output")
	}

	logEntry := decodePayload(t, defaultBuf.Bytes())
	if got := logEntry["msg"]; got != "resp: request failed with server error" {
		t.Fatalf("msg = %#v, want resp: request failed with server error", got)
	}
	if got := logEntry["http.response.status_code"]; got != float64(http.StatusInternalServerError) {
		t.Fatalf("http.response.status_code = %#v, want %d", got, http.StatusInternalServerError)
	}
}

// 显式暴露状态的包装 ResponseWriter 一旦已经发出状态或字节，WriteError 不应再改写响应。
func TestWriteErrorSkipsRewriteAfterWrappedResponseStarted(t *testing.T) {
	rr := httptest.NewRecorder()
	w := chimiddleware.NewWrapResponseWriter(rr, 1)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusAccepted)
	if _, err := w.Write([]byte("partial")); err != nil {
		t.Fatalf("w.Write() error = %v", err)
	}

	cause := errors.New("boom")
	err := WriteError(w, httptest.NewRequest(http.MethodGet, "/", nil), cause)
	if !errors.Is(err, cause) {
		t.Errorf("WriteError() error = %v, want original error %v", err, cause)
	}
	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", got)
	}
	if got := rr.Body.String(); got != "partial" {
		t.Errorf("body = %q, want partial", got)
	}
}

// 5xx 错误会给请求日志补充诊断字段，但不写入公开 details。
func TestWriteErrorEnrichesRequestLog(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(traceid.Middleware)
	r.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))
	r.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		rawErr := &rawTestError{message: "db timeout"}
		err := errx.NewHTTPErrorWithCause(
			http.StatusInternalServerError,
			"internal_error",
			"",
			&wrappedTestError{op: "load user", err: rawErr},
			map[string]any{"field": "name", "code": "required"},
		)
		_ = WriteError(w, r, err)
	})
	req := httptest.NewRequest(http.MethodGet, "/users/u_123", nil)
	req.Header.Set(chimiddleware.RequestIDHeader, "req-123")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	logEntry := decodePayload(t, buf.Bytes())
	if got := logEntry["error.code"]; got != "internal_error" {
		t.Fatalf("error.code = %#v, want internal_error", got)
	}
	if got := logEntry["request.id"]; got != "req-123" {
		t.Fatalf("request.id = %#v, want req-123", got)
	}
	if got, ok := logEntry["traceId"].(string); !ok || got == "" {
		t.Fatalf("traceId = %#v, want non-empty string", logEntry["traceId"])
	}
	if _, exists := logEntry["error.message"]; exists {
		t.Fatalf("error.message unexpectedly present: %#v", logEntry["error.message"])
	}
	if _, exists := logEntry["error.type"]; exists {
		t.Fatalf("error.type unexpectedly present: %#v", logEntry["error.type"])
	}
	if _, exists := logEntry["error.root_message"]; exists {
		t.Fatalf("error.root_message unexpectedly present: %#v", logEntry["error.root_message"])
	}
	if _, exists := logEntry["error.root_type"]; exists {
		t.Fatalf("error.root_type unexpectedly present: %#v", logEntry["error.root_type"])
	}
	if _, exists := logEntry["error.details"]; exists {
		t.Fatalf("error.details unexpectedly present: %#v", logEntry["error.details"])
	}
	if _, exists := logEntry["error.chain"]; exists {
		t.Fatalf("error.chain unexpectedly present: %#v", logEntry["error.chain"])
	}
	if _, exists := logEntry["error.chain_types"]; exists {
		t.Fatalf("error.chain_types unexpectedly present: %#v", logEntry["error.chain_types"])
	}
	if _, exists := logEntry["error.public_message"]; exists {
		t.Fatalf("error.public_message unexpectedly present: %#v", logEntry["error.public_message"])
	}
	if _, exists := logEntry["error.expected"]; exists {
		t.Fatalf("error.expected unexpectedly present: %#v", logEntry["error.expected"])
	}
	if _, exists := logEntry["error.category"]; exists {
		t.Fatalf("error.category unexpectedly present: %#v", logEntry["error.category"])
	}
	if _, exists := logEntry["error.details_count"]; exists {
		t.Fatalf("error.details_count unexpectedly present: %#v", logEntry["error.details_count"])
	}
}

// request log 只保留低噪音字段，不会镜像包装错误文本或类型。
func TestWriteErrorEnrichesRequestLogFromWrappedHTTPErrorWithoutCause(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r := chi.NewRouter()
	r.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))
	r.Get("/wrapped", func(w http.ResponseWriter, r *http.Request) {
		err := fmt.Errorf("handler failed: %w", errx.NewHTTPError(
			http.StatusInternalServerError,
			"internal_error",
			"",
		))
		_ = WriteError(w, r, err)
	})
	req := httptest.NewRequest(http.MethodGet, "/wrapped", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	logEntry := decodePayload(t, buf.Bytes())
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
func TestWriteErrorEnrichesRequestLogWithTimeoutFlag(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(traceid.Middleware)
	r.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))
	r.Get("/timeout", func(w http.ResponseWriter, r *http.Request) {
		err := errx.NewHTTPErrorWithCause(
			http.StatusInternalServerError,
			"internal_error",
			"",
			context.DeadlineExceeded,
		)
		_ = WriteError(w, r, err)
	})
	req := httptest.NewRequest(http.MethodGet, "/timeout", nil)
	req.Header.Set(chimiddleware.RequestIDHeader, "req-timeout")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	logEntry := decodePayload(t, buf.Bytes())
	if got := logEntry["error.timeout"]; got != true {
		t.Fatalf("error.timeout = %#v, want true", got)
	}
	if _, exists := logEntry["error.canceled"]; exists {
		t.Fatalf("error.canceled unexpectedly present: %#v", logEntry["error.canceled"])
	}
	if got := logEntry["request.id"]; got != "req-timeout" {
		t.Fatalf("request.id = %#v, want req-timeout", got)
	}
	if got, ok := logEntry["traceId"].(string); !ok || got == "" {
		t.Fatalf("traceId = %#v, want non-empty string", logEntry["traceId"])
	}
}

// 5xx 请求日志会显式标记 canceled 错误，即使公开响应仍是 500。
func TestWriteErrorEnrichesRequestLogWithCanceledFlag(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(traceid.Middleware)
	r.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))
	r.Get("/canceled", func(w http.ResponseWriter, r *http.Request) {
		err := errx.NewHTTPErrorWithCause(
			http.StatusInternalServerError,
			"internal_error",
			"",
			context.Canceled,
		)
		_ = WriteError(w, r, err)
	})
	req := httptest.NewRequest(http.MethodGet, "/canceled", nil)
	req.Header.Set(chimiddleware.RequestIDHeader, "req-canceled")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	logEntry := decodePayload(t, buf.Bytes())
	if got := logEntry["error.canceled"]; got != true {
		t.Fatalf("error.canceled = %#v, want true", got)
	}
	if _, exists := logEntry["error.timeout"]; exists {
		t.Fatalf("error.timeout unexpectedly present: %#v", logEntry["error.timeout"])
	}
	if got := logEntry["request.id"]; got != "req-canceled" {
		t.Fatalf("request.id = %#v, want req-canceled", got)
	}
	if got, ok := logEntry["traceId"].(string); !ok || got == "" {
		t.Fatalf("traceId = %#v, want non-empty string", logEntry["traceId"])
	}
}

// 4xx 错误不会污染请求日志的 error.* 诊断字段。
func TestWriteErrorDoesNotEnrichRequestLogFor4xx(t *testing.T) {
	var buf bytes.Buffer
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r := chi.NewRouter()
	r.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))
	r.Use(chimiddleware.RequestID)
	r.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		err := errx.BadRequest("bad_request", "bad request", map[string]any{
			"field": "name",
			"code":  "required",
		})
		_ = WriteError(w, r, err)
	})
	req := httptest.NewRequest(http.MethodGet, "/users/u_123", nil)
	req.Header.Set(chimiddleware.RequestIDHeader, "req-456")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	logEntry := decodePayload(t, buf.Bytes())
	if got := logEntry["http.response.status_code"]; got != float64(http.StatusBadRequest) {
		t.Fatalf("http.response.status_code = %#v, want %d", got, http.StatusBadRequest)
	}
	for key := range logEntry {
		if strings.HasPrefix(key, "error.") {
			t.Fatalf("unexpected error log field for 4xx: %s=%#v", key, logEntry[key])
		}
	}
	if defaultBuf.Len() != 0 {
		t.Fatalf("default logger unexpectedly captured output: %s", defaultBuf.Bytes())
	}
}

// 4xx 不会为了独立错误日志去预先展开内部诊断错误链。
func TestWriteErrorDoesNotBuildDiagnosticAttrsFor4xx(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users/u_123", nil)
	rr := httptest.NewRecorder()

	calls := 0
	err := WriteError(rr, req, errx.NewHTTPErrorWithCause(
		http.StatusBadRequest,
		"bad_request",
		"bad request",
		&countingError{calls: &calls},
	))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if calls != 0 {
		t.Fatalf("countingError.Error() calls = %d, want 0", calls)
	}
}

// 5xx 会通过 slog.Default() 额外记录一条独立错误日志，便于脱离 access log 排查问题。
func TestWriteErrorLogsServerErrorToDefaultLogger(t *testing.T) {
	var requestBuf bytes.Buffer
	requestLogger := slog.New(slog.NewJSONHandler(&requestBuf, nil))

	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(traceid.Middleware)
	r.Use(httplog.RequestLogger(requestLogger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))
	r.Get("/failure", func(w http.ResponseWriter, r *http.Request) {
		_ = WriteError(w, r, errors.New("db timeout"))
	})
	req := httptest.NewRequest(http.MethodGet, "/failure", nil)
	req.Header.Set(chimiddleware.RequestIDHeader, "req-server")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if requestBuf.Len() == 0 {
		t.Fatal("request logger did not capture access log")
	}
	if defaultBuf.Len() == 0 {
		t.Fatal("default logger did not capture output")
	}

	accessLog := decodePayload(t, requestBuf.Bytes())
	if got := accessLog["error.code"]; got != "internal_error" {
		t.Fatalf("access log error.code = %#v, want internal_error", got)
	}
	if got := accessLog["request.id"]; got != "req-server" {
		t.Fatalf("access log request.id = %#v, want req-server", got)
	}
	if got, ok := accessLog["traceId"].(string); !ok || got == "" {
		t.Fatalf("access log traceId = %#v, want non-empty string", accessLog["traceId"])
	}
	if _, exists := accessLog["error.message"]; exists {
		t.Fatalf("access log error.message unexpectedly present: %#v", accessLog["error.message"])
	}

	logEntry := decodePayload(t, defaultBuf.Bytes())
	if got := logEntry["msg"]; got != "resp: request failed with server error" {
		t.Fatalf("msg = %#v, want resp: request failed with server error", got)
	}
	if got := logEntry["error.code"]; got != "internal_error" {
		t.Fatalf("error.code = %#v, want internal_error", got)
	}
	if got := logEntry["error.message"]; got != "db timeout" {
		t.Fatalf("error.message = %#v, want db timeout", got)
	}
	if got := logEntry["request.id"]; got != "req-server" {
		t.Fatalf("request.id = %#v, want req-server", got)
	}
	if got, ok := logEntry["traceId"].(string); !ok || got == "" {
		t.Fatalf("traceId = %#v, want non-empty string", logEntry["traceId"])
	}
	if got := logEntry["http.response.status_code"]; got != float64(http.StatusInternalServerError) {
		t.Fatalf("http.response.status_code = %#v, want %d", got, http.StatusInternalServerError)
	}
}

// request 为空且错误响应写出失败时，独立错误日志仍会回退到默认 logger。
func TestWriteErrorWithNilRequestLogsWriteFailureToDefaultLogger(t *testing.T) {
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	w := &failingWriter{}
	err := WriteError(w, nil, errx.NewHTTPError(http.StatusInternalServerError, "internal_error", "Internal Server Error"))
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if w.status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.status, http.StatusInternalServerError)
	}

	if defaultBuf.Len() == 0 {
		t.Fatal("default logger did not capture output")
	}

	lines := bytes.Split(bytes.TrimSpace(defaultBuf.Bytes()), []byte{'\n'})
	if len(lines) != 2 {
		t.Fatalf("default logger lines = %d, want 2", len(lines))
	}

	serverLog := decodePayload(t, lines[0])
	if got := serverLog["msg"]; got != "resp: request failed with server error" {
		t.Fatalf("msg = %#v, want resp: request failed with server error", got)
	}
	if got := serverLog["error.code"]; got != "internal_error" {
		t.Fatalf("error.code = %#v, want internal_error", got)
	}
}

// 错误响应写出失败时，独立错误日志会回退到 slog.Default。
func TestLogErrorResponseWriteFailureFallsBackToDefaultLogger(t *testing.T) {
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	req := httptest.NewRequest(http.MethodGet, "/failure", nil)
	logErrorResponseWriteFailure(
		req,
		errx.NewHTTPError(http.StatusInternalServerError, "internal_error", "Internal Server Error"),
		errors.New("socket closed"),
	)

	if defaultBuf.Len() == 0 {
		t.Fatal("default logger did not capture output")
	}

	logEntry := decodePayload(t, defaultBuf.Bytes())
	if got := logEntry["msg"]; got != "resp: failed to write error response" {
		t.Fatalf("msg = %#v, want resp: failed to write error response", got)
	}
	if got := logEntry["error.code"]; got != "internal_error" {
		t.Fatalf("error.code = %#v, want internal_error", got)
	}
	if got := logEntry["error.message"]; got != "socket closed" {
		t.Fatalf("error.message = %#v, want socket closed", got)
	}
	if got := logEntry["http.response.status_code"]; got != float64(http.StatusInternalServerError) {
		t.Fatalf("http.response.status_code = %#v, want %d", got, http.StatusInternalServerError)
	}
}

func assertPublicErrorObject(t *testing.T, got any, want map[string]any) {
	t.Helper()

	gotMap, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("error item = %#v, want object", got)
	}
	for key, wantValue := range want {
		if gotValue := gotMap[key]; gotValue != wantValue {
			t.Fatalf("error item %q = %#v, want %#v", key, gotValue, wantValue)
		}
	}
	if len(gotMap) != len(want) {
		t.Fatalf("error item = %#v, want only %#v", gotMap, want)
	}
}
