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
// [✓] Created / JSON / OK 稳定输出紧凑 JSON；接口不依赖 request 上下文
// [✓] JSON 拒绝 nil writer、非法状态码、无响应体状态和不可编码值；OK 拒绝 nil writer、nil data 和不可编码值
// [✓] Created 拒绝 nil data、不可编码值和 nil writer
// [✓] JSONBlob 直接原样透传 JSON 字节，不做合法性校验，并拒绝 nil writer、非法状态和无响应体状态
// [✓] NoContent 只写 204 状态，不写 body，也不设置 Content-Type；nil writer 会返回错误
// [✓] writeJSON / writeSuccess 会把编码错误和状态校验错误直接向上返回

type payloadMap map[string]any

// Created 会以 201 状态直接写出 JSON 对象。
func TestCreatedWritesDirectPayload(t *testing.T) {
	rr := httptest.NewRecorder()

	err := Created(rr, map[string]any{"id": "u_1"})
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

// Created 不需要 request 也能安全写出紧凑 JSON。
func TestCreatedWritesCompactJSONWithoutRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := Created(rr, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("Created() error = %v", err)
	}
	if body := rr.Body.String(); body != "{\"id\":\"u_1\"}\n" {
		t.Fatalf("body = %q, want compact JSON", body)
	}
}

// JSON 会按指定成功状态直接写出 JSON 对象。
func TestJSONWritesDirectPayload(t *testing.T) {
	rr := httptest.NewRecorder()

	err := JSON(rr, http.StatusAccepted, map[string]any{"id": "u_1"})
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
	rr := httptest.NewRecorder()

	if err := JSON(rr, http.StatusOK, nil); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if body := rr.Body.String(); body != "null\n" {
		t.Fatalf("body = %q, want %q", body, "null\n")
	}
}

// JSON 不需要 request 也会输出紧凑 JSON。
func TestJSONWritesCompactJSONWithoutRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := JSON(rr, http.StatusOK, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if body := rr.Body.String(); body != "{\"id\":\"u_1\"}\n" {
		t.Fatalf("body = %q, want compact JSON", body)
	}
}

func TestJSONWriterCanCooperateWithHeadLikeWriter(t *testing.T) {
	inner := &headLikeResponseWriter{}
	w := &writeCallbackResponseWriter{ResponseWriter: inner}
	body, err := encodeJSON(map[string]any{"id": "u_1"})
	if err != nil {
		t.Fatalf("encodeJSON() error = %v", err)
	}

	if err := JSON(w, http.StatusAccepted, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if inner.status != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", inner.status, http.StatusAccepted)
	}
	if got := inner.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := inner.Header().Get("Content-Length"); got != "13" {
		t.Fatalf("Content-Length = %q, want %d", got, len(body))
	}
	if w.writeCalls != 1 {
		t.Fatalf("writeCalls = %d, want 1", w.writeCalls)
	}
}

func TestJSONWithoutRequestContextStillValidatesPayload(t *testing.T) {
	rr := httptest.NewRecorder()

	err := JSON(rr, http.StatusOK, make(chan int))
	if err == nil || err.Error() != "json: unsupported type: chan int" {
		t.Fatalf("JSON() error = %v, want unsupported type error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// Created 会拒绝空的 ResponseWriter。
func TestCreatedRejectsNilWriter(t *testing.T) {
	err := Created(nil, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: response writer is nil" {
		t.Fatalf("Created() error = %v, want response writer is nil", err)
	}
}

// JSON 也会拒绝空的 ResponseWriter。
func TestJSONRejectsNilWriter(t *testing.T) {
	err := JSON(nil, http.StatusOK, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: response writer is nil" {
		t.Fatalf("JSON() error = %v, want response writer is nil", err)
	}
}

// JSONBlob 会原样写出已编码好的 JSON 字节。
func TestJSONBlobWritesRawJSONBytes(t *testing.T) {
	rr := httptest.NewRecorder()

	err := JSONBlob(rr, http.StatusAccepted, []byte(`{"id":"u_1"}`))
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

// JSONBlob 不需要 request 也会原样写出 JSON 字节。
func TestJSONBlobWritesRawJSONWithoutRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := JSONBlob(rr, http.StatusAccepted, []byte(`{"id":"u_1"}`)); err != nil {
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

func TestJSONBlobCanCooperateWithHeadLikeWriter(t *testing.T) {
	inner := &headLikeResponseWriter{}
	w := &writeCallbackResponseWriter{ResponseWriter: inner}
	body := []byte(`{"id":"u_1"}`)

	if err := JSONBlob(w, http.StatusAccepted, body); err != nil {
		t.Fatalf("JSONBlob() error = %v", err)
	}
	if inner.status != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", inner.status, http.StatusAccepted)
	}
	if got := inner.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := inner.Header().Get("Content-Length"); got != "12" {
		t.Fatalf("Content-Length = %q, want %d", got, len(body))
	}
	if w.writeCalls != 1 {
		t.Fatalf("writeCalls = %d, want 1", w.writeCalls)
	}
}

func TestJSONBlobRejectsNilWriterWithoutRequest(t *testing.T) {
	err := JSONBlob(nil, http.StatusOK, []byte(`{"id":"u_1"}`))
	if err == nil || err.Error() != "resp: response writer is nil" {
		t.Fatalf("JSONBlob() error = %v, want response writer is nil", err)
	}
}

func TestJSONBlobRejectsInvalidStatusWithoutRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	err := JSONBlob(rr, 1000, []byte(`{"id":"u_1"}`))
	if err == nil || err.Error() != "resp: invalid HTTP status 1000" {
		t.Fatalf("JSONBlob() error = %v, want invalid HTTP status", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

func TestJSONBlobRejectsBodylessStatusWithoutRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	err := JSONBlob(rr, http.StatusNoContent, []byte(`{"id":"u_1"}`))
	if err == nil || err.Error() != "resp: JSON body writers cannot use bodyless status 204" {
		t.Fatalf("JSONBlob() error = %v, want bodyless status error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// JSONBlob 直接透传字节，不负责校验其是否是合法 JSON。
func TestJSONBlobPassesThroughInvalidJSONBytes(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := JSONBlob(rr, http.StatusOK, []byte(`{"id":`)); err != nil {
		t.Fatalf("JSONBlob() error = %v", err)
	}
	if body := rr.Body.String(); body != `{"id":` {
		t.Fatalf("body = %q, want raw invalid JSON bytes", body)
	}
}

// JSONBlob 也会拒绝空的 ResponseWriter。
func TestJSONBlobRejectsNilWriter(t *testing.T) {
	err := JSONBlob(nil, http.StatusOK, []byte(`{"id":"u_1"}`))
	if err == nil || err.Error() != "resp: response writer is nil" {
		t.Fatalf("JSONBlob() error = %v, want response writer is nil", err)
	}
}

// JSONBlob 也必须拒绝不允许响应体的状态码。
func TestJSONBlobRejectsBodylessStatus(t *testing.T) {
	rr := httptest.NewRecorder()

	err := JSONBlob(rr, http.StatusNoContent, []byte(`{"id":"u_1"}`))
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
	rr := httptest.NewRecorder()

	err := JSONBlob(rr, 1000, []byte(`{"id":"u_1"}`))
	if err == nil || err.Error() != "resp: invalid HTTP status 1000" {
		t.Fatalf("JSONBlob() error = %v, want invalid HTTP status", err)
	}
}

// JSON 在编码不支持的值时会直接返回错误。
func TestJSONRejectsUnsupportedValue(t *testing.T) {
	rr := httptest.NewRecorder()

	err := JSON(rr, http.StatusOK, make(chan int))
	if err == nil || err.Error() != "json: unsupported type: chan int" {
		t.Fatalf("JSON() error = %v, want unsupported type error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// 自定义 MarshalJSON 即使 panic，JSON 也应返回错误而不是把 panic 冒出到 handler。
func TestJSONRecoversFromMarshalPanic(t *testing.T) {
	rr := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("JSON() panicked: %v", recovered)
		}
	}()

	err := JSON(rr, http.StatusOK, panicSuccessJSONValue{})
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
	rr := httptest.NewRecorder()

	if err := Created(rr, nil); err == nil || err.Error() != "resp: data must exist and must not encode to null" {
		t.Fatalf("Created() error = %v, want non-null data error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

func TestOKCanCooperateWithHeadLikeWriter(t *testing.T) {
	inner := &headLikeResponseWriter{}
	w := &writeCallbackResponseWriter{ResponseWriter: inner}
	body, err := encodeJSON(map[string]any{"id": "u_1"})
	if err != nil {
		t.Fatalf("encodeJSON() error = %v", err)
	}

	if err := OK(w, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("OK() error = %v", err)
	}
	if inner.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", inner.status, http.StatusOK)
	}
	if got := inner.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := inner.Header().Get("Content-Length"); got != "13" {
		t.Fatalf("Content-Length = %q, want %d", got, len(body))
	}
	if w.writeCalls != 1 {
		t.Fatalf("writeCalls = %d, want 1", w.writeCalls)
	}
}

// Created 在编码不支持的值时会直接返回错误。
func TestCreatedRejectsUnsupportedValue(t *testing.T) {
	rr := httptest.NewRecorder()

	err := Created(rr, make(chan int))
	if err == nil || err.Error() != "json: unsupported type: chan int" {
		t.Fatalf("Created() error = %v, want unsupported type error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// JSON 会拒绝非法的 HTTP 状态码。
func TestJSONRejectsInvalidStatus(t *testing.T) {
	rr := httptest.NewRecorder()

	err := JSON(rr, 1000, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: invalid HTTP status 1000" {
		t.Fatalf("JSON() error = %v, want invalid HTTP status", err)
	}
}

// JSON 不能把 payload 写到 205/204/304 这类不允许响应体的状态上。
func TestJSONRejectsBodylessStatus(t *testing.T) {
	rr := httptest.NewRecorder()

	err := JSON(rr, http.StatusResetContent, map[string]any{"id": "u_1"})
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

// OK 不需要 request 也能安全写出紧凑 JSON。
func TestOKWritesCompactJSONWithoutRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := OK(rr, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("OK() error = %v", err)
	}
	if body := rr.Body.String(); body != "{\"id\":\"u_1\"}\n" {
		t.Fatalf("body = %q, want compact JSON", body)
	}
}

// OK 语义要求显式数据，nil 数据会被拒绝。
func TestOKRejectsNilData(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := OK(rr, nil); err == nil || err.Error() != "resp: data must exist and must not encode to null" {
		t.Fatalf("OK() error = %v, want non-null data error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// OK 在编码不支持的值时会直接返回错误。
func TestOKRejectsUnsupportedValue(t *testing.T) {
	rr := httptest.NewRecorder()

	err := OK(rr, make(chan int))
	if err == nil || err.Error() != "json: unsupported type: chan int" {
		t.Fatalf("OK() error = %v, want unsupported type error", err)
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

// OK 会拒绝空的 ResponseWriter。
func TestOKRejectsNilWriter(t *testing.T) {
	err := OK(nil, map[string]any{"id": "u_1"})
	if err == nil || err.Error() != "resp: response writer is nil" {
		t.Fatalf("OK() error = %v, want response writer is nil", err)
	}
}

// NoContent 只写 204 状态，不产生响应体。
func TestNoContentWritesBodylessStatus(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := NoContent(rr); err != nil {
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
	err := NoContent(nil)
	if err == nil || err.Error() != "resp: response writer is nil" {
		t.Fatalf("NoContent() error = %v, want response writer is nil", err)
	}
}

// NoContent 不需要 request 也能安全返回。
func TestNoContentWithoutRequest(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := NoContent(rr); err != nil {
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
