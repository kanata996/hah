package resp

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

func TestNormalizeHTTPErrorCommonErrors(t *testing.T) {
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

			got := asHTTPError(tc.input)
			if tc.wantStatus == 0 {
				if got != nil {
					t.Fatalf("asHTTPError() = %#v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("asHTTPError() = nil")
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

type panicJSONDetail struct{}

func (panicJSONDetail) MarshalJSON() ([]byte, error) {
	panic("panic during MarshalJSON")
}

// WriteError 会把 HTTPError 写成标准 problem JSON。
func TestWriteErrorWritesEnvelope(t *testing.T) {
	rr := httptest.NewRecorder()

	err := WriteError(rr, errx.NewHTTPError(
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
	rr := httptest.NewRecorder()

	err := WriteError(rr, errx.NewHTTPError(
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

// 多个 details 对象应圆整进入 problem JSON 的 errors 数组。
func TestWriteErrorMultipleDetails(t *testing.T) {
	rr := httptest.NewRecorder()

	err := WriteError(rr, errx.NewHTTPError(
		http.StatusUnprocessableEntity,
		"validation_failed",
		"request validation failed",
		map[string]any{"field": "name", "code": "required"},
		map[string]any{"field": "email", "code": "invalid"},
	))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnprocessableEntity)
	}

	body := decodePayload(t, rr.Body.Bytes())
	errors, ok := body["errors"].([]any)
	if !ok || len(errors) != 2 {
		t.Fatalf("errors = %#v, want 2 items", body["errors"])
	}
	assertPublicErrorObject(t, errors[0], map[string]any{
		"field": "name",
		"code":  "required",
	})
	assertPublicErrorObject(t, errors[1], map[string]any{
		"field": "email",
		"code":  "invalid",
	})
}

// 传入 nil 错误时，WriteError 应是纯 no-op。
func TestWriteErrorNilErrorIsNoop(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := WriteError(rr, nil); err != nil {
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

// ResponseWriter 为空时，WriteError 会把底层写回失败作为普通 error 返回。
func TestWriteErrorRejectsNilWriter(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("WriteError() panicked: %v", recovered)
		}
	}()

	err := WriteError(nil, errors.New("db timeout"))
	if err == nil || !strings.Contains(err.Error(), "response writer is nil") {
		t.Fatalf("WriteError() error = %v, want response writer is nil", err)
	}
}

// HEAD 请求写错误时仍走正常 Write 链路，但最终对外只保留状态和头语义。
func TestWriteErrorHeadWritesStatusWithoutBody(t *testing.T) {
	inner := &headLikeResponseWriter{}
	w := &writeCallbackResponseWriter{ResponseWriter: inner}
	httpErr := errx.NewHTTPError(http.StatusBadRequest, "", "", "detail")
	body, err := marshalProblemPayload(httpErr)
	if err != nil {
		t.Fatalf("marshalProblemPayload() error = %v", err)
	}

	err = WriteError(w, httpErr)
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if inner.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", inner.status, http.StatusBadRequest)
	}
	if got := inner.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
	if got := inner.Header().Get("Content-Length"); got != strconv.Itoa(len(body)) {
		t.Fatalf("Content-Length = %q, want %d", got, len(body))
	}
	if w.writeCalls != 1 {
		t.Fatalf("writeCalls = %d, want 1", w.writeCalls)
	}
}

// 非法状态码会被标准化为 500，且内部 cause 不会泄漏到公开响应。
func TestWriteErrorNormalizesInvalidStatusAndHidesCause(t *testing.T) {
	rr := httptest.NewRecorder()

	err := WriteError(rr, errx.NewHTTPErrorWithCause(99, "", "", errors.New("db timeout")))
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
	rr := httptest.NewRecorder()

	err := WriteError(rr, context.Canceled)
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
	rr := httptest.NewRecorder()

	err := WriteError(rr, context.DeadlineExceeded)
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
	rr := httptest.NewRecorder()

	err := WriteError(rr, errors.New("db timeout"))
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

// 公开 details 不可编码时，WriteError 直接返回错误，不再尝试降级写回。
func TestWriteErrorRejectsUnencodableDetails(t *testing.T) {
	rr := httptest.NewRecorder()

	err := WriteError(rr, errx.NewHTTPError(
		http.StatusBadRequest,
		"bad_request",
		"bad request",
		func() {},
	))
	_ = assertUnsupportedTypeError(t, err)
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// details 的自定义 JSON 编码即使发生 panic，也应返回错误而不是把响应路径打崩。
func TestWriteErrorRejectsPanickingDetails(t *testing.T) {
	rr := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("WriteError() panicked: %v", recovered)
		}
	}()

	err := WriteError(rr, errx.NewHTTPError(
		http.StatusBadRequest,
		"bad_request",
		"bad request",
		panicJSONDetail{},
	))
	if err == nil || err.Error() != "resp: marshal problem payload panicked: panic during MarshalJSON" {
		t.Fatalf("WriteError() error = %v, want panic recovery error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// resp 自己的写响应错误再次传回 WriteError 时，应直接原样返回，避免重复写。
func TestWriteErrorReturnsRespWriteErrorWithoutRewrite(t *testing.T) {
	w := &failingWriter{}
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	err := OK(w, map[string]any{"id": "u_1"})
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if w.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.status, http.StatusOK)
	}
	if w.writes != 1 {
		t.Fatalf("writes = %d, want 1", w.writes)
	}

	writtenErr := WriteError(w, err)
	if !errors.Is(writtenErr, err) {
		t.Fatalf("WriteError() error = %v, want original error %v", writtenErr, err)
	}
	if w.writes != 1 {
		t.Fatalf("writes = %d, want still 1", w.writes)
	}
	if defaultBuf.Len() != 0 {
		t.Fatalf("default logger unexpectedly captured output: %s", defaultBuf.Bytes())
	}
}

func TestWriteErrorDoesNotWriteStandaloneLogForServerError(t *testing.T) {
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	rr := httptest.NewRecorder()
	if err := WriteError(rr, errors.New("db timeout")); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if defaultBuf.Len() != 0 {
		t.Fatalf("default logger unexpectedly captured output: %s", defaultBuf.Bytes())
	}
}

func TestWriteErrorWriteFailureDoesNotWriteStandaloneLog(t *testing.T) {
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	w := &failingWriter{}
	err := WriteError(w, errx.NewHTTPError(http.StatusInternalServerError, "internal_error", "Internal Server Error"))
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if w.status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.status, http.StatusInternalServerError)
	}
	if defaultBuf.Len() != 0 {
		t.Fatalf("default logger unexpectedly captured output: %s", defaultBuf.Bytes())
	}
}
