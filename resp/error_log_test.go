package resp

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kanata996/hah/errx"
)

type nilUnsafeTestError struct {
	err error
}

type panicUnwrapTestError struct{}

func (e *nilUnsafeTestError) Error() string {
	return e.err.Error()
}

func (e *nilUnsafeTestError) Unwrap() error {
	return e.err
}

func (panicUnwrapTestError) Error() string {
	return "unwrap panic"
}

func (panicUnwrapTestError) Unwrap() error {
	panic("boom")
}

func TestWriteErrorLogsWrappedCauseSummary(t *testing.T) {
	var buf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(previousDefault)

	req := httptest.NewRequest(http.MethodGet, "/users/u_1", nil)
	httpErr := errx.NewHTTPErrorWithCause(
		http.StatusInternalServerError,
		"internal_error",
		"",
		&wrappedTestError{
			op:  "query user",
			err: &rawTestError{message: "db timeout"},
		},
	)

	if err := WriteError(httptest.NewRecorder(), req, httpErr); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	logEntry := decodePayload(t, buf.Bytes())
	if got := logEntry["msg"]; got != "resp: request failed with server error" {
		t.Fatalf("msg = %#v, want resp: request failed with server error", got)
	}
	if got := logEntry["error.code"]; got != "internal_error" {
		t.Fatalf("error.code = %#v, want internal_error", got)
	}
	if got := logEntry["error.message"]; got != "query user: db timeout" {
		t.Fatalf("error.message = %#v, want query user: db timeout", got)
	}
	if got := logEntry["error.type"]; got != "*resp.wrappedTestError" {
		t.Fatalf("error.type = %#v, want *resp.wrappedTestError", got)
	}
	if got := logEntry["error.root_message"]; got != "db timeout" {
		t.Fatalf("error.root_message = %#v, want db timeout", got)
	}
	if got := logEntry["error.root_type"]; got != "*resp.rawTestError" {
		t.Fatalf("error.root_type = %#v, want *resp.rawTestError", got)
	}
}

// typed-nil 和不安全 Error() 实现都不应把日志摘要路径打崩。
func TestSummarizeDiagnosticErrorWithTypedNilError(t *testing.T) {
	var cause error = (*nilUnsafeTestError)(nil)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("summarizeDiagnosticError(typed nil) panicked: %v", recovered)
		}
	}()

	summary := summarizeDiagnosticError(cause)
	if got := summary.errorType; got != "*resp.nilUnsafeTestError" {
		t.Fatalf("errorType = %q, want *resp.nilUnsafeTestError", got)
	}
	if got := summary.rootType; got != "*resp.nilUnsafeTestError" {
		t.Fatalf("rootType = %q, want *resp.nilUnsafeTestError", got)
	}
	if got := summary.message; !strings.Contains(got, "panic calling Error()") {
		t.Fatalf("message = %q, want panic fallback text", got)
	}
	if got := summary.rootMessage; !strings.Contains(got, "panic calling Error()") {
		t.Fatalf("rootMessage = %q, want panic fallback text", got)
	}
}

// 不安全 Unwrap() 实现会被降级为停止下钻，而不是反向影响错误响应主流程。
func TestSummarizeDiagnosticErrorStopsAfterUnwrapPanic(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("summarizeDiagnosticError(unwrap panic) panicked: %v", recovered)
		}
	}()

	summary := summarizeDiagnosticError(panicUnwrapTestError{})
	if got := summary.message; got != "unwrap panic" {
		t.Fatalf("message = %q, want unwrap panic", got)
	}
	if got := summary.errorType; got != "resp.panicUnwrapTestError" {
		t.Fatalf("errorType = %q, want resp.panicUnwrapTestError", got)
	}
	if got := summary.rootMessage; got != "unwrap panic" {
		t.Fatalf("rootMessage = %q, want unwrap panic", got)
	}
	if got := summary.rootType; got != "resp.panicUnwrapTestError" {
		t.Fatalf("rootType = %q, want resp.panicUnwrapTestError", got)
	}
}

func TestWriteErrorLogTruncatesAtUTF8Boundary(t *testing.T) {
	var buf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(previousDefault)

	padding := strings.Repeat("a", maxLoggedErrorStringBytes-1)
	input := padding + "世"
	req := httptest.NewRequest(http.MethodGet, "/truncated", nil)

	if err := WriteError(httptest.NewRecorder(), req, errors.New(input)); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	logEntry := decodePayload(t, buf.Bytes())
	for _, key := range []string{"error.message", "error.root_message"} {
		got, ok := logEntry[key].(string)
		if !ok {
			t.Fatalf("%s = %#v, want string", key, logEntry[key])
		}
		if !strings.HasSuffix(got, "...(truncated)") {
			t.Fatalf("%s = %q, want truncated suffix", key, got)
		}

		core := strings.TrimSuffix(got, "...(truncated)")
		if !utf8.ValidString(core) {
			t.Fatalf("%s core is not valid UTF-8: %q", key, core)
		}
		if strings.Contains(core, "世") {
			t.Fatalf("%s core contains truncated multibyte rune: %q", key, core)
		}
	}
}

func attrsToMap(attrs []slog.Attr) map[string]any {
	out := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		out[attr.Key] = attr.Value.Any()
	}
	return out
}
