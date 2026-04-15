package resp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

type payloadMap map[string]any
type panicSuccessJSONValue struct{}
type panicWriteCause struct{}
type blankWriteCause struct{}
type failingWriter struct {
	header http.Header
	status int
	writes int
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

func assertWrappedResponseWriteError(t *testing.T, err error) *responseWriteError {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var writeErr *responseWriteError
	if !errors.As(err, &writeErr) {
		t.Fatalf("error = %T, want *responseWriteError", err)
	}
	return writeErr
}

func assertUnsupportedTypeError(t *testing.T, err error) *json.UnsupportedTypeError {
	t.Helper()

	if err == nil {
		t.Fatal("expected unsupported type error, got nil")
	}

	var unsupportedErr *json.UnsupportedTypeError
	if !errors.As(err, &unsupportedErr) {
		t.Fatalf("error = %T, want *json.UnsupportedTypeError", err)
	}
	return unsupportedErr
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

func mustEncodeJSON(t *testing.T, data any) []byte {
	t.Helper()

	body, err := encodeJSON(data)
	if err != nil {
		t.Fatalf("encodeJSON() error = %v", err)
	}
	return body
}

func stringLen(body []byte) string {
	return strconv.Itoa(len(body))
}
