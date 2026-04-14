package resp

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type panicSuccessJSONValue struct{}
type panicWriteCause struct{}
type blankWriteCause struct{}
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
