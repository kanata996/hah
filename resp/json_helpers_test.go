package resp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// 测试清单：
// [✓] responseWriteError 在 nil、普通错误、panic Error()、空白错误文本下都稳定
// [✓] writeJSONBytes 校验 writer、状态码，以及不允许响应体的状态
// [✓] writeStatus 校验 writer、状态码，并只写状态不写 body / Content-Type
// [✓] encodeJSON 对不支持的值和 MarshalJSON panic 都返回普通 error
// [✓] HTTP 状态码校验接受标准范围并拒绝越界值

type panicSuccessJSONValue struct{}
type panicWriteCause struct{}
type blankWriteCause struct{}
type trackingResponseWriter struct {
	header           http.Header
	writeHeaderCalls int
	writeCalls       int
	status           int
}
type headLikeResponseWriter struct {
	header           http.Header
	writeHeaderCalls int
	writeCalls       int
	status           int
}
type writeCallbackResponseWriter struct {
	http.ResponseWriter
	writeCalls int
}

func (panicSuccessJSONValue) MarshalJSON() ([]byte, error) {
	panic("panic during MarshalJSON")
}

func (panicWriteCause) Error() string {
	panic("panic during Error")
}

func (blankWriteCause) Error() string {
	return "   "
}

func (w *trackingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *trackingResponseWriter) WriteHeader(status int) {
	w.writeHeaderCalls++
	w.status = status
}

func (w *trackingResponseWriter) Write(p []byte) (int, error) {
	w.writeCalls++
	return len(p), nil
}

func (w *headLikeResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *headLikeResponseWriter) WriteHeader(status int) {
	w.writeHeaderCalls++
	w.status = status
}

func (w *headLikeResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	w.writeCalls++
	if w.Header().Get("Content-Length") == "" {
		w.Header().Set("Content-Length", strconv.Itoa(len(p)))
	}
	return len(p), nil
}

func (w *writeCallbackResponseWriter) Write(p []byte) (int, error) {
	w.writeCalls++
	return w.ResponseWriter.Write(p)
}

// responseWriteError 在 nil 接收者和普通错误场景下都应提供稳定的错误语义。
func TestResponseWriteErrorMethods(t *testing.T) {
	var nilErr *responseWriteError
	if got := nilErr.Error(); got != "resp: write response failed" {
		t.Fatalf("nil Error() = %q", got)
	}
	if got := nilErr.Unwrap(); got != nil {
		t.Fatalf("nil Unwrap() = %v, want nil", got)
	}

	cause := errors.New("socket closed")
	err := &responseWriteError{cause: cause}
	if got := err.Error(); got != "resp: write response failed: socket closed" {
		t.Fatalf("Error() = %q", got)
	}
	if got := err.Unwrap(); !errors.Is(got, cause) {
		t.Fatalf("Unwrap() = %v, want %v", got, cause)
	}
}

// 即使底层写错误本身的 Error() 实现不安全，responseWriteError 也不应再 panic。
func TestResponseWriteErrorErrorRecoversFromCausePanic(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("responseWriteError.Error() panicked: %v", recovered)
		}
	}()

	got := (&responseWriteError{cause: panicWriteCause{}}).Error()
	if !strings.Contains(got, "resp: write response failed: panic calling Error()") {
		t.Fatalf("Error() = %q, want panic fallback text", got)
	}
}

// 底层错误文本为空白时，responseWriteError 也应回退到稳定默认文案。
func TestResponseWriteErrorErrorFallsBackOnBlankCause(t *testing.T) {
	if got := (&responseWriteError{cause: blankWriteCause{}}).Error(); got != "resp: write response failed" {
		t.Fatalf("Error() = %q, want fallback text", got)
	}
}

// 底层写 JSON 字节时会拒绝空的 ResponseWriter。
func TestWriteJSONBytesRejectsNilWriter(t *testing.T) {
	err := writeJSONBytes(nil, http.StatusOK, []byte(`{"ok":true}`))
	assertErrorContainsAll(t, err, "response writer is nil")
}

// 底层写 JSON 字节时会校验 HTTP 状态码合法性。
func TestWriteJSONBytesRejectsInvalidStatus(t *testing.T) {
	rr := httptest.NewRecorder()

	err := writeJSONBytes(rr, 1000, []byte(`{"ok":true}`))
	assertErrorContainsAll(t, err, "invalid HTTP status", "1000")
}

// 带 body 的 JSON 写辅助函数应在写出前拒绝 1xx/204/205/304 这类不允许响应体的状态。
func TestWriteJSONBytesRejectsStatusesThatCannotHaveBody(t *testing.T) {
	testCases := []struct {
		status     int
		wantErrHas []string
	}{
		{status: http.StatusContinue, wantErrHas: []string{"JSON body writers", "informational status", "100"}},
		{status: http.StatusNoContent, wantErrHas: []string{"JSON body writers", "bodyless status", "204"}},
		{status: http.StatusResetContent, wantErrHas: []string{"JSON body writers", "bodyless status", "205"}},
		{status: http.StatusNotModified, wantErrHas: []string{"JSON body writers", "bodyless status", "304"}},
	}

	for _, tc := range testCases {
		w := &trackingResponseWriter{}
		err := writeJSONBytes(w, tc.status, []byte(`{"ok":true}`))
		assertErrorContainsAll(t, err, tc.wantErrHas...)
		if w.writeHeaderCalls != 0 {
			t.Fatalf("writeJSONBytes(status=%d) wrote header %d times, want 0", tc.status, w.writeHeaderCalls)
		}
		if w.writeCalls != 0 {
			t.Fatalf("writeJSONBytes(status=%d) wrote body %d times, want 0", tc.status, w.writeCalls)
		}
		if got := w.Header().Get("Content-Type"); got != "" {
			t.Fatalf("writeJSONBytes(status=%d) Content-Type = %q, want empty", tc.status, got)
		}
	}
}

// 仅写状态码的辅助函数也会拒绝空的 ResponseWriter。
func TestWriteStatusRejectsNilWriter(t *testing.T) {
	err := writeStatus(nil, http.StatusNoContent)
	assertErrorContainsAll(t, err, "response writer is nil")
}

// 仅写状态码的辅助函数会校验 HTTP 状态码合法性。
func TestWriteStatusRejectsInvalidStatus(t *testing.T) {
	rr := httptest.NewRecorder()

	err := writeStatus(rr, 1000)
	assertErrorContainsAll(t, err, "invalid HTTP status", "1000")
}

// 仅写状态码的辅助函数成功时不写 body，也不擅自设置 Content-Type。
func TestWriteStatusWritesStatusWithoutBodyOrContentType(t *testing.T) {
	w := &trackingResponseWriter{}

	if err := writeStatus(w, http.StatusNoContent); err != nil {
		t.Fatalf("writeStatus() error = %v", err)
	}
	if w.writeHeaderCalls != 1 {
		t.Fatalf("writeHeaderCalls = %d, want 1", w.writeHeaderCalls)
	}
	if w.writeCalls != 0 {
		t.Fatalf("writeCalls = %d, want 0", w.writeCalls)
	}
	if w.status != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.status, http.StatusNoContent)
	}
	if got := w.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
}

// JSON 编码阶段遇到不支持的值时直接返回编码错误。
func TestEncodeJSONRejectsUnsupportedValue(t *testing.T) {
	_, err := encodeJSON(make(chan int))
	assertErrorContainsAll(t, err, "unsupported type", "chan int")
}

// 自定义 MarshalJSON 即使 panic，编码辅助函数也应恢复为普通 error。
func TestEncodeJSONRecoversFromMarshalPanic(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("encodeJSON() panicked: %v", recovered)
		}
	}()

	_, err := encodeJSON(panicSuccessJSONValue{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "resp: encode JSON panicked") {
		t.Fatalf("encodeJSON() error = %v, want panic recovery error", err)
	}
}

// HTTP 状态码校验会接受标准范围并拒绝越界值。
func TestValidateHTTPStatus(t *testing.T) {
	for _, status := range []int{http.StatusContinue, http.StatusOK, 999} {
		if err := validateHTTPStatus(status); err != nil {
			t.Fatalf("validateHTTPStatus(%d) error = %v, want nil", status, err)
		}
	}

	testCases := []int{99, 1000}
	for _, status := range testCases {
		assertErrorContainsAll(t, validateHTTPStatus(status), "invalid HTTP status", strconv.Itoa(status))
	}
}

func assertErrorContainsAll(t *testing.T, err error, wantParts ...string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	message := err.Error()
	for _, part := range wantParts {
		if !strings.Contains(message, part) {
			t.Fatalf("error = %q, want substring %q", message, part)
		}
	}
}
