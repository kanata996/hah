package resp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 测试清单：
// [✓] Created / JSON / JSONBlob 按约定写出 JSON、状态码和 Content-Type
// [✓] Created / JSON / OK 稳定输出紧凑 JSON；所有 success writer 在 nil request 下也安全
// [✓] JSON 拒绝 nil writer、非法状态码、无响应体状态和不可编码值；OK 拒绝 nil writer、nil data 和不可编码值
// [✓] Created 拒绝 nil data、不可编码值和 nil writer
// [✓] JSONBlob 直接原样透传 JSON 字节，不做合法性校验，并拒绝 nil writer、非法状态和无响应体状态
// [✓] NoContent 只写 204 状态，不写 body，也不设置 Content-Type；nil writer 会返回错误
// [✓] writeJSON / writeSuccess 会把编码错误和状态校验错误直接向上返回

type payloadMap map[string]any

// Created 会以 201 状态直接写出 JSON 对象。
func TestCreatedWritesDirectPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/accounts", nil)
	rr := httptest.NewRecorder()

	err := Created(rr, req, map[string]any{"id": "u_1"})
	if err != nil {
		t.Fatalf("Created() error = %v", err)
	}

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	payload := decodePayload(t, rr.Body.Bytes())
	if got := payload["id"]; got != "u_1" {
		t.Fatalf("id = %#v, want u_1", got)
	}
}

// Created 在 request 为 nil 时也能安全写出紧凑 JSON。
func TestCreatedAllowsNilRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := Created(rr, nil, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("Created() error = %v", err)
	}
	if body := rr.Body.String(); body != "{\"id\":\"u_1\"}\n" {
		t.Fatalf("body = %q, want compact JSON", body)
	}
}

// JSON 会按指定成功状态直接写出 JSON 对象。
func TestJSONWritesDirectPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := JSON(rr, req, http.StatusAccepted, map[string]any{"id": "u_1"})
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	payload := decodePayload(t, rr.Body.Bytes())
	if got := payload["id"]; got != "u_1" {
		t.Fatalf("id = %#v, want u_1", got)
	}
}

// JSON 显式允许把 nil 数据编码为公开的 null 响应体。
func TestJSONAllowsNilData(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	if err := JSON(rr, req, http.StatusOK, nil); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if body := rr.Body.String(); body != "null\n" {
		t.Fatalf("body = %q, want %q", body, "null\n")
	}
}

// JSON 在 request 为 nil 时也会输出紧凑 JSON。
func TestJSONAllowsNilRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := JSON(rr, nil, http.StatusOK, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if body := rr.Body.String(); body != "{\"id\":\"u_1\"}\n" {
		t.Fatalf("body = %q, want compact JSON", body)
	}
}

// Created 会拒绝空的 ResponseWriter。
func TestCreatedRejectsNilWriter(t *testing.T) {
	err := Created(nil, httptest.NewRequest(http.MethodPost, "/v1/accounts", nil), map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: response writer is nil" {
		t.Fatalf("Created() error = %v, want response writer is nil", err)
	}
}

// JSON 也会拒绝空的 ResponseWriter。
func TestJSONRejectsNilWriter(t *testing.T) {
	err := JSON(nil, nil, http.StatusOK, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: response writer is nil" {
		t.Fatalf("JSON() error = %v, want response writer is nil", err)
	}
}

// JSONBlob 会原样写出已编码好的 JSON 字节。
func TestJSONBlobWritesRawJSONBytes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := JSONBlob(rr, req, http.StatusAccepted, []byte(`{"id":"u_1"}`))
	if err != nil {
		t.Fatalf("JSONBlob() error = %v", err)
	}

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if body := rr.Body.String(); body != `{"id":"u_1"}` {
		t.Fatalf("body = %q, want raw JSON bytes", body)
	}
}

// JSONBlob 在 request 为 nil 时也会原样写出 JSON 字节。
func TestJSONBlobAllowsNilRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := JSONBlob(rr, nil, http.StatusAccepted, []byte(`{"id":"u_1"}`)); err != nil {
		t.Fatalf("JSONBlob() error = %v", err)
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if body := rr.Body.String(); body != `{"id":"u_1"}` {
		t.Fatalf("body = %q, want raw JSON bytes", body)
	}
}

// JSONBlob 直接透传字节，不负责校验其是否是合法 JSON。
func TestJSONBlobPassesThroughInvalidJSONBytes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	if err := JSONBlob(rr, req, http.StatusOK, []byte(`{"id":`)); err != nil {
		t.Fatalf("JSONBlob() error = %v", err)
	}
	if body := rr.Body.String(); body != `{"id":` {
		t.Fatalf("body = %q, want raw invalid JSON bytes", body)
	}
}

// JSONBlob 也会拒绝空的 ResponseWriter。
func TestJSONBlobRejectsNilWriter(t *testing.T) {
	err := JSONBlob(nil, nil, http.StatusOK, []byte(`{"id":"u_1"}`))
	if err == nil || err.Error() != "resp: response writer is nil" {
		t.Fatalf("JSONBlob() error = %v, want response writer is nil", err)
	}
}

// JSONBlob 也必须拒绝不允许响应体的状态码。
func TestJSONBlobRejectsBodylessStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := JSONBlob(rr, req, http.StatusNoContent, []byte(`{"id":"u_1"}`))
	if err == nil || err.Error() != "resp: JSON body writers cannot use bodyless status 204" {
		t.Fatalf("JSONBlob() error = %v, want bodyless status error", err)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
}

// JSONBlob 也会拒绝非法的 HTTP 状态码。
func TestJSONBlobRejectsInvalidStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := JSONBlob(rr, req, 1000, []byte(`{"id":"u_1"}`))
	if err == nil || err.Error() != "resp: invalid HTTP status 1000" {
		t.Fatalf("JSONBlob() error = %v, want invalid HTTP status", err)
	}
}

// JSON 在编码不支持的值时会直接返回错误。
func TestJSONRejectsUnsupportedValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := JSON(rr, req, http.StatusOK, make(chan int))
	if err == nil || err.Error() != "json: unsupported type: chan int" {
		t.Fatalf("JSON() error = %v, want unsupported type error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// 自定义 MarshalJSON 即使 panic，JSON 也应返回错误而不是把 panic 冒出到 handler。
func TestJSONRecoversFromMarshalPanic(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("JSON() panicked: %v", recovered)
		}
	}()

	err := JSON(rr, req, http.StatusOK, panicSuccessJSONValue{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "resp: encode JSON panicked: panic during MarshalJSON" {
		t.Fatalf("JSON() error = %v, want panic recovery error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// Created 语义要求显式数据，nil 数据会被拒绝。
func TestCreatedRejectsNilData(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/accounts", nil)
	rr := httptest.NewRecorder()

	if err := Created(rr, req, nil); err == nil || err.Error() != "resp: data must exist and must not encode to null" {
		t.Fatalf("Created() error = %v, want non-null data error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// Created 在编码不支持的值时会直接返回错误。
func TestCreatedRejectsUnsupportedValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/accounts", nil)
	rr := httptest.NewRecorder()

	err := Created(rr, req, make(chan int))
	if err == nil || err.Error() != "json: unsupported type: chan int" {
		t.Fatalf("Created() error = %v, want unsupported type error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// JSON 会拒绝非法的 HTTP 状态码。
func TestJSONRejectsInvalidStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := JSON(rr, req, 1000, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: invalid HTTP status 1000" {
		t.Fatalf("JSON() error = %v, want invalid HTTP status", err)
	}
}

// JSON 不能把 payload 写到 205/204/304 这类不允许响应体的状态上。
func TestJSONRejectsBodylessStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := JSON(rr, req, http.StatusResetContent, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: JSON body writers cannot use bodyless status 205" {
		t.Fatalf("JSON() error = %v, want bodyless status error", err)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
}

// OK 在 request 为 nil 时也能安全写出紧凑 JSON。
func TestOKAllowsNilRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := OK(rr, nil, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("OK() error = %v", err)
	}
	if body := rr.Body.String(); body != "{\"id\":\"u_1\"}\n" {
		t.Fatalf("body = %q, want compact JSON", body)
	}
}

// OK 语义要求显式数据，nil 数据会被拒绝。
func TestOKRejectsNilData(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	if err := OK(rr, req, nil); err == nil || err.Error() != "resp: data must exist and must not encode to null" {
		t.Fatalf("OK() error = %v, want non-null data error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// OK 在编码不支持的值时会直接返回错误。
func TestOKRejectsUnsupportedValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	err := OK(rr, req, make(chan int))
	if err == nil || err.Error() != "json: unsupported type: chan int" {
		t.Fatalf("OK() error = %v, want unsupported type error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// OK 会拒绝空的 ResponseWriter。
func TestOKRejectsNilWriter(t *testing.T) {
	err := OK(nil, httptest.NewRequest(http.MethodGet, "/", nil), map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: response writer is nil" {
		t.Fatalf("OK() error = %v, want response writer is nil", err)
	}
}

// NoContent 只写 204 状态，不产生响应体。
func TestNoContentWritesBodylessStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rr := httptest.NewRecorder()

	if err := NoContent(rr, req); err != nil {
		t.Fatalf("NoContent() error = %v", err)
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
}

// NoContent 也会拒绝空的 ResponseWriter。
func TestNoContentRejectsNilWriter(t *testing.T) {
	err := NoContent(nil, nil)
	if err == nil || err.Error() != "resp: response writer is nil" {
		t.Fatalf("NoContent() error = %v, want response writer is nil", err)
	}
}

// NoContent 在 request 为 nil 时也能安全返回。
func TestNoContentAllowsNilRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := NoContent(rr, nil); err != nil {
		t.Fatalf("NoContent() error = %v", err)
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
}

// writeJSON 会把底层编码错误直接向上返回。
func TestWriteJSONPropagatesEncodeError(t *testing.T) {
	rr := httptest.NewRecorder()

	err := writeJSON(rr, http.StatusOK, make(chan int))
	if err == nil || err.Error() != "json: unsupported type: chan int" {
		t.Fatalf("writeJSON() error = %v, want unsupported type error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// writeJSON 会把底层状态校验错误直接向上返回。
func TestWriteJSONPropagatesStatusValidationError(t *testing.T) {
	err := writeJSON(httptest.NewRecorder(), 1000, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: invalid HTTP status 1000" {
		t.Fatalf("writeJSON() error = %v, want invalid HTTP status", err)
	}
}

// writeJSON 应先校验响应边界，再进行编码，避免非法状态掩盖更根本的写回错误。
func TestWriteJSONValidatesStatusBeforeEncoding(t *testing.T) {
	rr := httptest.NewRecorder()

	err := writeJSON(rr, http.StatusNoContent, panicSuccessJSONValue{})
	if err == nil || err.Error() != "resp: JSON body writers cannot use bodyless status 204" {
		t.Fatalf("writeJSON() error = %v, want bodyless status error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// writeSuccess 会拒绝非成功状态码。
func TestWriteSuccessRejectsInvalidStatus(t *testing.T) {
	err := writeSuccess(httptest.NewRecorder(), http.StatusBadRequest, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: invalid success status 400" {
		t.Fatalf("writeSuccess() error = %v, want invalid success status", err)
	}
}

// writeSuccess 也会先拒绝非法的 HTTP 状态码数值。
func TestWriteSuccessRejectsInvalidHTTPStatus(t *testing.T) {
	err := writeSuccess(httptest.NewRecorder(), 1000, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: invalid HTTP status 1000" {
		t.Fatalf("writeSuccess() error = %v, want invalid HTTP status", err)
	}
}

// writeSuccess 会拒绝无法携带响应体的状态码。
func TestWriteSuccessRejectsBodylessStatus(t *testing.T) {
	err := writeSuccess(httptest.NewRecorder(), http.StatusNoContent, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: success writers with a body cannot use bodyless status 204" {
		t.Fatalf("writeSuccess() error = %v, want bodyless status error", err)
	}
}

// writeSuccess 会拒绝 1xx informational 状态。
func TestWriteSuccessRejectsInformationalStatus(t *testing.T) {
	err := writeSuccess(httptest.NewRecorder(), http.StatusContinue, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: success writers with a body cannot use informational status 100" {
		t.Fatalf("writeSuccess() error = %v, want informational status error", err)
	}
}

// 写响应体失败时会返回带 responseStarted 标记的包装错误。
func TestWriteJSONBytesReturnsWrappedWriteError(t *testing.T) {
	w := &failingWriter{}

	err := writeJSONBytes(w, http.StatusOK, []byte(`{"id":"u_1"}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var writeErr *responseWriteError
	if !errors.As(err, &writeErr) {
		t.Fatalf("error = %T, want *responseWriteError", err)
	}
	if !writeErr.responseStarted {
		t.Fatal("responseStarted = false, want true")
	}
}

func decodePayload(t *testing.T, body []byte) payloadMap {
	t.Helper()

	var payload payloadMap
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return payload
}

func assertRecorderHasNoBodyOrContentType(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()

	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
}
