package hah

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// 测试清单：
// [✓] 根包 facade 会把 reqx / resp 的核心能力稳定透传出来
// [✓] 根包 facade 会把 resp 的成功响应与错误响应 helper 稳定透传出来
// [✓] 根包 facade 公开常用绑定入口：BindBody、BindQuery
// [✓] 根包 facade 继续暴露统一错误响应写回与 invalid_request helper

type rootPayloadMap map[string]any

type decodedRootFieldError struct {
	Field  string
	In     string
	Code   string
	Detail string
}

// BindBody 只从 JSON body 绑定数据。
func TestBindBody_DelegatesToReqx(t *testing.T) {
	req := newJSONRequest(http.MethodPost, "/accounts", `{"name":"kanata"}`)

	var dst struct {
		Name string `json:"name"`
	}

	if err := BindBody(req, &dst); err != nil {
		t.Fatalf("BindBody() error = %v", err)
	}
	if dst.Name != "kanata" {
		t.Fatalf("name = %q, want kanata", dst.Name)
	}
}

// BindQuery 只从 query 参数绑定数据。
func TestBindQuery_DelegatesToReqx(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts?cursor=next", nil)

	var dst struct {
		Cursor string `query:"cursor"`
	}

	if err := BindQuery(req, &dst); err != nil {
		t.Fatalf("BindQuery() error = %v", err)
	}
	if dst.Cursor != "next" {
		t.Fatalf("cursor = %q, want next", dst.Cursor)
	}
}

// Path 会通过根包 facade 暴露 reqx 的 path 单参数校验 builder。
func TestPath_DelegatesToReqx(t *testing.T) {
	want := uuid.New()
	req := newRouteRequest(http.MethodGet, "/accounts/"+want.String(), "id", want.String())

	id, err := Path(req, "id").UUID().Required().Get()
	if err != nil {
		t.Fatalf("Path().UUID().Required().Get() error = %v", err)
	}
	if id != want {
		t.Fatalf("id = %v, want %v", id, want)
	}
}

// Query 会通过根包 facade 暴露 reqx 的 query 单参数校验 builder。
func TestQuery_DelegatesToReqx(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts?at=2026-04-12T08:30:00Z", nil)

	at, err := Query(req, "at").Time().Required().Get()
	if err != nil {
		t.Fatalf("Query().Time().Required().Get() error = %v", err)
	}
	if got := at.UTC().Format(time.RFC3339); got != "2026-04-12T08:30:00Z" {
		t.Fatalf("at = %q, want 2026-04-12T08:30:00Z", got)
	}
}

// Query 也会通过根包 facade 暴露 query 专用的多值读取 builder。
func TestQuery_ValuesDelegatesToReqx(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts?tag=a&tag=b", nil)

	tags, err := Query(req, "tag").Values().Required().Get()
	if err != nil {
		t.Fatalf("Query().Values().Required().Get() error = %v", err)
	}
	if got := strings.Join(tags, ","); got != "a,b" {
		t.Fatalf("tags = %q, want a,b", got)
	}
}

// InvalidRequest 会通过根包 facade 暴露统一的 invalid_request 错误构造。
func TestInvalidRequest_DelegatesToReqx(t *testing.T) {
	fieldError := assertSingleRootFieldError(t, InvalidRequest(FieldError{
		Field: "name",
		In:    InBody,
		Code:  CodeRequired,
	}))

	if fieldError.Field != "name" || fieldError.In != string(InBody) || fieldError.Code != string(CodeRequired) || fieldError.Detail != "is required" {
		t.Fatalf("fieldError = %#v", fieldError)
	}
}

// NewHTTPError 会通过根包 facade 暴露共享公共错误模型。
func TestNewHTTPError_DelegatesToErrx(t *testing.T) {
	err := NewHTTPError(http.StatusConflict, "account_conflict", "account already exists").WithFieldErrors([]FieldError{
		{
			Field:  "name",
			In:     InBody,
			Code:   CodeInvalid,
			Detail: "is invalid",
		},
	})

	if got := err.Status(); got != http.StatusConflict {
		t.Fatalf("Status() = %d, want %d", got, http.StatusConflict)
	}
	if got := err.Code(); got != "account_conflict" {
		t.Fatalf("Code() = %q, want account_conflict", got)
	}
	if got := err.Detail(); got != "account already exists" {
		t.Fatalf("Detail() = %q, want account already exists", got)
	}

	fieldError := assertSingleRootFieldError(t, err)
	if fieldError.Field != "name" || fieldError.In != string(InBody) || fieldError.Code != string(CodeInvalid) || fieldError.Detail != "is invalid" {
		t.Fatalf("fieldError = %#v", fieldError)
	}
}

// NewHTTPErrorWithCause 会通过根包 facade 暴露保留 cause 的公共错误构造。
func TestNewHTTPErrorWithCause_DelegatesToErrx(t *testing.T) {
	cause := errors.New("db timeout")
	err := NewHTTPErrorWithCause(99, "", "", cause)

	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false, want true")
	}
	if got := err.Status(); got != http.StatusInternalServerError {
		t.Fatalf("Status() = %d, want %d", got, http.StatusInternalServerError)
	}
	if got := err.Code(); got != "internal_error" {
		t.Fatalf("Code() = %q, want internal_error", got)
	}
	if got := err.Error(); got != "internal error" {
		t.Fatalf("Error() = %q, want internal error", got)
	}
}

// 根包也会直接暴露常用 HTTP 错误快捷构造器与 field error 常量。
func TestRootErrorHelpersExposePublicFieldErrorSurface(t *testing.T) {
	err := UnprocessableEntity("invalid_account", "account is invalid").WithFieldErrors([]FieldError{
		{
			Field:  "name",
			In:     InBody,
			Code:   CodeRequired,
			Detail: "is required",
		},
	})

	if got := err.Status(); got != http.StatusUnprocessableEntity {
		t.Fatalf("Status() = %d, want %d", got, http.StatusUnprocessableEntity)
	}
	if got := err.Code(); got != "invalid_account" {
		t.Fatalf("Code() = %q, want invalid_account", got)
	}

	fieldError := assertSingleRootFieldError(t, err)
	if fieldError.Field != "name" || fieldError.In != string(InBody) || fieldError.Code != string(CodeRequired) || fieldError.Detail != "is required" {
		t.Fatalf("fieldError = %#v", fieldError)
	}
}

func TestRootErrorHelpers_CommonStatuses(t *testing.T) {
	tests := []struct {
		name       string
		build      func(code, detail string) *HTTPError
		wantStatus int
	}{
		{name: "bad request", build: BadRequest, wantStatus: http.StatusBadRequest},
		{name: "unauthorized", build: Unauthorized, wantStatus: http.StatusUnauthorized},
		{name: "forbidden", build: Forbidden, wantStatus: http.StatusForbidden},
		{name: "not found", build: NotFound, wantStatus: http.StatusNotFound},
		{name: "method not allowed", build: MethodNotAllowed, wantStatus: http.StatusMethodNotAllowed},
		{name: "conflict", build: Conflict, wantStatus: http.StatusConflict},
		{name: "unprocessable entity", build: UnprocessableEntity, wantStatus: http.StatusUnprocessableEntity},
		{name: "too many requests", build: TooManyRequests, wantStatus: http.StatusTooManyRequests},
		{name: "internal server", build: InternalServer, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.build("custom_code", "custom detail")

			if got := err.Status(); got != tt.wantStatus {
				t.Fatalf("Status() = %d, want %d", got, tt.wantStatus)
			}
			if got := err.Code(); got != "custom_code" {
				t.Fatalf("Code() = %q, want custom_code", got)
			}
			if got := err.Detail(); got != "custom detail" {
				t.Fatalf("Detail() = %q, want custom detail", got)
			}
		})
	}
}

// WriteError 会通过根包 facade 写出统一的公开错误包络。
func TestWriteError_DelegatesToResp(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := WriteError(rr, context.DeadlineExceeded, 50412); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusGatewayTimeout)
	}

	body := decodeRootPayload(t, rr.Body.Bytes())
	if got := body["code"]; got != float64(50412) {
		t.Fatalf("code = %#v, want 50412", got)
	}
	if got := body["message"]; got != "timeout" {
		t.Fatalf("message = %#v, want timeout", got)
	}

	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	if got := errorValue["reason"]; got != "timeout" {
		t.Fatalf("error.reason = %#v, want timeout", got)
	}
}

func TestNormalizeError_DelegatesToErrx(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		if got := NormalizeError(nil); got != nil {
			t.Fatalf("NormalizeError(nil) = %#v, want nil", got)
		}
	})

	t.Run("invalid request keeps public error semantics", func(t *testing.T) {
		err := NormalizeError(InvalidRequest(FieldError{
			Field: "name",
			In:    InBody,
			Code:  CodeRequired,
		}))
		if err == nil {
			t.Fatal("NormalizeError() = nil, want value")
		}
		if err.Status() != http.StatusUnprocessableEntity {
			t.Fatalf("Status() = %d, want %d", err.Status(), http.StatusUnprocessableEntity)
		}
		if err.Code() != "invalid_request" {
			t.Fatalf("Code() = %q, want invalid_request", err.Code())
		}
		if err.Detail() != "request contains invalid fields" {
			t.Fatalf("Detail() = %q, want request contains invalid fields", err.Detail())
		}
		if got := err.Errors(); len(got) != 1 || got[0].Field != "name" || got[0].In != InBody || got[0].Code != CodeRequired || got[0].Detail != "is required" {
			t.Fatalf("Errors() = %#v", got)
		}
	})

	t.Run("context deadline exceeded maps to timeout", func(t *testing.T) {
		err := NormalizeError(context.DeadlineExceeded)
		if err == nil {
			t.Fatal("NormalizeError() = nil, want value")
		}
		if err.Status() != http.StatusGatewayTimeout {
			t.Fatalf("Status() = %d, want %d", err.Status(), http.StatusGatewayTimeout)
		}
		if err.Code() != "timeout" {
			t.Fatalf("Code() = %q, want timeout", err.Code())
		}
		if err.Detail() != "timeout" {
			t.Fatalf("Detail() = %q, want timeout", err.Detail())
		}
	})

	t.Run("wrapped unknown error becomes internal error", func(t *testing.T) {
		err := NormalizeError(errors.New("db timeout"))
		if err == nil {
			t.Fatal("NormalizeError() = nil, want value")
		}
		if err.Status() != http.StatusInternalServerError {
			t.Fatalf("Status() = %d, want %d", err.Status(), http.StatusInternalServerError)
		}
		if err.Code() != "internal_error" {
			t.Fatalf("Code() = %q, want internal_error", err.Code())
		}
		if err.Detail() != "internal error" {
			t.Fatalf("Detail() = %q, want internal error", err.Detail())
		}
	})
}

// OK 会通过根包 facade 写回标准 200 JSON 响应。
func TestOK_DelegatesToResp(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := OK(rr, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("OK() error = %v", err)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	payload := decodeRootPayload(t, rr.Body.Bytes())
	if got := payload["code"]; got != float64(0) {
		t.Fatalf("code = %#v, want 0", got)
	}
	if got := payload["message"]; got != "success" {
		t.Fatalf("message = %#v, want success", got)
	}

	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want object", payload["data"])
	}
	if data["id"] != "u_1" {
		t.Fatalf("data.id = %#v, want u_1", data["id"])
	}
}

// JSON 会通过根包 facade 直接写回紧凑 JSON。
func TestJSON_DelegatesToResp(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := JSON(rr, http.StatusAccepted, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rr.Code)
	}
	if body := rr.Body.String(); body != "{\"id\":\"u_1\"}\n" {
		t.Fatalf("body = %q, want compact JSON", body)
	}
}

// Accepted 会通过根包 facade 写回标准 202 JSON 响应。
func TestAccepted_DelegatesToResp(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := Accepted(rr, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("Accepted() error = %v", err)
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}

	payload := decodeRootPayload(t, rr.Body.Bytes())
	if got := payload["code"]; got != float64(0) {
		t.Fatalf("code = %#v, want 0", got)
	}
	if got := payload["message"]; got != "success" {
		t.Fatalf("message = %#v, want success", got)
	}

	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want object", payload["data"])
	}
	if data["id"] != "u_1" {
		t.Fatalf("data.id = %#v, want u_1", data["id"])
	}
}

// Created 会通过根包 facade 写回标准 201 JSON 响应。
func TestCreated_DelegatesToResp(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := Created(rr, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("Created() error = %v", err)
	}
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}

	payload := decodeRootPayload(t, rr.Body.Bytes())
	if got := payload["code"]; got != float64(0) {
		t.Fatalf("code = %#v, want 0", got)
	}
	if got := payload["message"]; got != "success" {
		t.Fatalf("message = %#v, want success", got)
	}

	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want object", payload["data"])
	}
	if data["id"] != "u_1" {
		t.Fatalf("data.id = %#v, want u_1", data["id"])
	}
}

// NoContent 会通过根包 facade 写回标准 204 无响应体成功响应。
func TestNoContent_DelegatesToResp(t *testing.T) {
	rr := httptest.NewRecorder()
	rr.Header().Set("X-Trace-ID", "trace-1")
	rr.Header().Set("Content-Type", "application/json")
	rr.Header().Set("Content-Length", "999")

	if err := NoContent(rr); err != nil {
		t.Fatalf("NoContent() error = %v", err)
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if got := rr.Header().Get("X-Trace-ID"); got != "trace-1" {
		t.Fatalf("X-Trace-ID = %q, want trace-1", got)
	}
	if got := rr.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
	if got := rr.Header().Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length = %q, want empty", got)
	}
}

func newJSONRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newRouteRequest(method, target, name, value string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return withRouteParam(req, name, value)
}

func withRouteParam(req *http.Request, name, value string) *http.Request {
	req.Pattern = "/{" + name + "}"
	req.SetPathValue(name, value)
	return req
}

func decodeRootPayload(t *testing.T, body []byte) rootPayloadMap {
	t.Helper()

	var payload rootPayloadMap
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return payload
}

func assertSingleRootFieldError(t *testing.T, err error) decodedRootFieldError {
	t.Helper()

	payload := decodeRootPayload(t, mustWriteRootError(t, err))
	errorValue, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", payload["error"])
	}

	details, ok := errorValue["details"].([]any)
	if !ok || len(details) != 1 {
		t.Fatalf("details = %#v, want single field error", errorValue["details"])
	}

	fieldErrorMap, ok := details[0].(map[string]any)
	if !ok {
		t.Fatalf("field error type = %T, want map[string]any", details[0])
	}

	return decodedRootFieldError{
		Field:  stringValue(fieldErrorMap["field"]),
		In:     stringValue(fieldErrorMap["in"]),
		Code:   stringValue(fieldErrorMap["code"]),
		Detail: stringValue(fieldErrorMap["detail"]),
	}
}

func mustWriteRootError(t *testing.T, err error) []byte {
	t.Helper()

	rr := httptest.NewRecorder()
	if writeErr := WriteError(rr, err); writeErr != nil {
		t.Fatalf("WriteError() error = %v", writeErr)
	}
	return rr.Body.Bytes()
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
