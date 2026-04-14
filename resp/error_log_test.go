package resp

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kanata996/hah/errx"
)

type nonComparableWrappedTestError struct {
	op     string
	frames []string
	err    error
}

type nilUnsafeTestError struct {
	err error
}

func (e nonComparableWrappedTestError) Error() string {
	return fmt.Sprintf("%s: %v", e.op, e.err)
}

func (e nonComparableWrappedTestError) Unwrap() error {
	return e.err
}

func (e *nilUnsafeTestError) Error() string {
	return e.err.Error()
}

func (e *nilUnsafeTestError) Unwrap() error {
	return e.err
}

func TestWriteErrorLogsNonComparableCauseSafely(t *testing.T) {
	var buf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(previousDefault)

	req := httptest.NewRequest(http.MethodGet, "/users/u_1", nil)
	httpErr := errx.NewHTTPErrorWithCause(
		http.StatusInternalServerError,
		"internal_error",
		"",
		nonComparableWrappedTestError{
			op:     "query user",
			frames: []string{"users", "repo"},
			err:    errors.New("db timeout"),
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
	if got := logEntry["error.root_message"]; got != "db timeout" {
		t.Fatalf("error.root_message = %#v, want db timeout", got)
	}
}

// typed-nil 会在公开入口到达日志构建前就经过 errors.As；这里保留一个聚焦 helper 回归，守住日志摘要层的防御性。
func TestBuildErrorChainInfoWithTypedNilError(t *testing.T) {
	var cause error = (*nilUnsafeTestError)(nil)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("buildErrorChainInfo(typed nil) panicked: %v", recovered)
		}
	}()

	info := buildErrorChainInfo(cause)
	if got := info.errorType; got != "*resp.nilUnsafeTestError" {
		t.Fatalf("errorType = %q, want *resp.nilUnsafeTestError", got)
	}
	if got := info.rootType; got != "*resp.nilUnsafeTestError" {
		t.Fatalf("rootType = %q, want *resp.nilUnsafeTestError", got)
	}
	if got := info.message; !strings.Contains(got, "panic calling Error()") {
		t.Fatalf("message = %q, want panic fallback text", got)
	}
	if got := info.rootMessage; !strings.Contains(got, "panic calling Error()") {
		t.Fatalf("rootMessage = %q, want panic fallback text", got)
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
