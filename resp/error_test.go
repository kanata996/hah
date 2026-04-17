package resp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/kanata996/hah/errx"
)

// WriteError 会把 HTTPError 写成标准 problem JSON。
func TestWriteErrorWritesEnvelope(t *testing.T) {
	rr := httptest.NewRecorder()

	err := WriteError(rr, errx.NewHTTPError(
		http.StatusUnprocessableEntity,
		"",
		"",
	).WithViolations([]errx.Violation{
		{Field: "name", Code: "required", Detail: "is required"},
	}))
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
		"field":  "name",
		"code":   "required",
		"detail": "is required",
	})
}

// 包装后的 HTTPError 也应保留显式公共字段，而不是被默认值覆盖。
func TestWriteErrorPreservesExplicitPublicFieldsFromWrappedHTTPError(t *testing.T) {
	rr := httptest.NewRecorder()

	err := WriteError(rr, errors.Join(
		errors.New("handler failed"),
		errx.NewHTTPError(
			http.StatusBadRequest,
			"invalid_json",
			"payload invalid",
		).WithViolations([]errx.Violation{
			{Field: "name", Code: "required", Detail: "is required"},
		}),
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
		"field":  "name",
		"code":   "required",
		"detail": "is required",
	})
}

// 多个 violation 对象应圆整进入 problem JSON 的 errors 数组。
func TestWriteErrorMultipleDetails(t *testing.T) {
	rr := httptest.NewRecorder()

	err := WriteError(rr, errx.NewHTTPError(
		http.StatusUnprocessableEntity,
		"validation_failed",
		"request validation failed",
	).WithViolations([]errx.Violation{
		{Field: "name", Code: "required", Detail: "is required"},
		{Field: "email", Code: "invalid", Detail: "is invalid"},
	}))
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
		"field":  "name",
		"code":   "required",
		"detail": "is required",
	})
	assertPublicErrorObject(t, errors[1], map[string]any{
		"field":  "email",
		"code":   "invalid",
		"detail": "is invalid",
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

	if err := WriteError(nil, errors.New("db timeout")); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// HEAD 请求写错误时仍走正常 Write 链路，但最终对外只保留状态和头语义。
func TestWriteErrorHeadWritesStatusWithoutBody(t *testing.T) {
	expected := httptest.NewRecorder()
	httpErr := errx.NewHTTPError(http.StatusBadRequest, "", "").WithViolations([]errx.Violation{
		{Field: "name", Code: "required", Detail: "is required"},
	})
	if err := WriteError(expected, httpErr); err != nil {
		t.Fatalf("WriteError() expected recorder error = %v", err)
	}

	inner := &headLikeResponseWriter{}
	w := &writeCallbackResponseWriter{ResponseWriter: inner}

	err := WriteError(w, httpErr)
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if inner.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", inner.status, http.StatusBadRequest)
	}
	if got := inner.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
	if got := inner.Header().Get("Content-Length"); got != strconv.Itoa(expected.Body.Len()) {
		t.Fatalf("Content-Length = %q, want %d", got, expected.Body.Len())
	}
	if w.writeCalls != 1 {
		t.Fatalf("writeCalls = %d, want 1", w.writeCalls)
	}
}

func TestWriteErrorRespectsWrappedWriterContentLength(t *testing.T) {
	inner := &headLikeResponseWriter{}
	w := &transformingResponseWriter{
		ResponseWriter: inner,
		suffix:         []byte("\n"),
	}
	httpErr := errx.NewHTTPError(http.StatusBadRequest, "", "").WithViolations([]errx.Violation{
		{Field: "name", Code: "required", Detail: "is required"},
	})

	if err := WriteError(w, httpErr); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if inner.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", inner.status, http.StatusBadRequest)
	}
	if got := inner.Header().Get("Content-Length"); got != strconv.Itoa(len(w.lastWrite)) {
		t.Fatalf("Content-Length = %q, want %d", got, len(w.lastWrite))
	}
	if w.writeCalls != 1 {
		t.Fatalf("writeCalls = %d, want 1", w.writeCalls)
	}
}

func TestWriteErrorClearsStaleContentLengthOnRealHTTPServer(t *testing.T) {
	httpErr := errx.NewHTTPError(http.StatusBadRequest, "", "").WithViolations([]errx.Violation{
		{Field: "name", Code: "required", Detail: "is required"},
	})

	expected := httptest.NewRecorder()
	if err := WriteError(expected, httpErr); err != nil {
		t.Fatalf("WriteError() expected recorder error = %v", err)
	}

	result := roundTripOverHTTP(t, func(w http.ResponseWriter, _ *http.Request) error {
		w.Header().Set("Content-Length", "100")
		return WriteError(w, httpErr)
	})

	if result.handlerErr != nil {
		t.Fatalf("handler error = %v", result.handlerErr)
	}
	if result.response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", result.response.StatusCode, http.StatusBadRequest)
	}
	if got := result.response.Header.Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want %q", got, "application/problem+json")
	}
	if result.readErr != nil {
		t.Fatalf("ReadAll() error = %v", result.readErr)
	}
	if got := string(result.body); got != expected.Body.String() {
		t.Fatalf("body = %q, want %q", got, expected.Body.String())
	}
	if got := result.response.Header.Get("Content-Length"); got != "" && got != strconv.Itoa(len(result.body)) {
		t.Fatalf("Content-Length = %q, want empty or %d", got, len(result.body))
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
	if _, exists := body["detail"]; exists {
		t.Fatalf("detail unexpectedly present: %#v", body["detail"])
	}
	if _, exists := body["errors"]; exists {
		t.Fatalf("errors unexpectedly present: %#v", body["errors"])
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
	if _, exists := body["detail"]; exists {
		t.Fatalf("detail unexpectedly present: %#v", body["detail"])
	}
	if _, exists := body["errors"]; exists {
		t.Fatalf("errors unexpectedly present: %#v", body["errors"])
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
	if _, exists := body["detail"]; exists {
		t.Fatalf("detail unexpectedly present: %#v", body["detail"])
	}
	if _, exists := body["errors"]; exists {
		t.Fatalf("errors unexpectedly present: %#v", body["errors"])
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("db timeout")) {
		t.Fatalf("body leaked internal cause: %q", rr.Body.String())
	}
}

func TestWriteErrorReturnsWriteFailureAfterFirstCommit(t *testing.T) {
	cause := errors.New("socket closed")
	w := &failingWriter{cause: cause}
	err := WriteError(w, errx.NewHTTPError(http.StatusInternalServerError, "internal_error", "Internal Server Error"))
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false, want true")
	}
	if w.status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.status, http.StatusInternalServerError)
	}
	if got := w.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
	if w.writes != 1 {
		t.Fatalf("writes = %d, want 1", w.writes)
	}
}
