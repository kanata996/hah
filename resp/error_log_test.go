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

// 测试清单：
// [✓] 通过公开 `WriteError` 契约验证诊断日志对常见包装错误的摘要行为
// [✓] typed-nil / 不安全 Error() 实现会安全退化为稳定日志文本，而不是 panic
// [✓] 超长诊断文本会在公开日志输出里被截断，并保持 UTF-8 有效

type nonComparableWrappedTestError struct {
	op     string
	frames []string
	err    error
}

type nilUnsafeTestError struct {
	err error
}

type cycleTestError struct{}

type multiUnwrapTestError struct {
	errs []error
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

func (e *cycleTestError) Error() string {
	return "cycle"
}

func (e *cycleTestError) Unwrap() error {
	return e
}

func (e *multiUnwrapTestError) Error() string {
	return "multi"
}

func (e *multiUnwrapTestError) Unwrap() []error {
	return e.errs
}

func TestWriteErrorLogsWrappedCauseSummary(t *testing.T) {
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	req := httptest.NewRequest(http.MethodGet, "/users/u_1", nil)
	rr := httptest.NewRecorder()
	cause := nonComparableWrappedTestError{
		op:     "query user",
		frames: []string{"users", "repo"},
		err:    errors.New("db timeout"),
	}

	if err := WriteError(rr, req, errx.NewHTTPErrorWithCause(
		http.StatusInternalServerError,
		"internal_error",
		"",
		cause,
	)); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if defaultBuf.Len() == 0 {
		t.Fatal("default logger did not capture output")
	}

	logEntry := decodePayload(t, bytes.TrimSpace(defaultBuf.Bytes()))
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

func TestWriteErrorLogsTypedNilCauseSafely(t *testing.T) {
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	req := httptest.NewRequest(http.MethodGet, "/users/u_1", nil)
	rr := httptest.NewRecorder()
	var cause error = (*nilUnsafeTestError)(nil)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("WriteError() panicked: %v", recovered)
		}
	}()

	if err := WriteError(rr, req, errx.NewHTTPErrorWithCause(
		http.StatusInternalServerError,
		"internal_error",
		"",
		cause,
	)); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if defaultBuf.Len() == 0 {
		t.Fatal("default logger did not capture output")
	}

	logEntry := decodePayload(t, bytes.TrimSpace(defaultBuf.Bytes()))
	if got := logEntry["error.type"]; got != "*resp.nilUnsafeTestError" {
		t.Fatalf("error.type = %#v, want *resp.nilUnsafeTestError", got)
	}
	if got := logEntry["error.root_type"]; got != "*resp.nilUnsafeTestError" {
		t.Fatalf("error.root_type = %#v, want *resp.nilUnsafeTestError", got)
	}
	message, ok := logEntry["error.message"].(string)
	if !ok || !strings.Contains(message, "panic calling Error()") {
		t.Fatalf("error.message = %#v, want panic fallback text", logEntry["error.message"])
	}
	rootMessage, ok := logEntry["error.root_message"].(string)
	if !ok || !strings.Contains(rootMessage, "panic calling Error()") {
		t.Fatalf("error.root_message = %#v, want panic fallback text", logEntry["error.root_message"])
	}
}

func TestWriteErrorTruncatesLongDiagnosticMessages(t *testing.T) {
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	req := httptest.NewRequest(http.MethodGet, "/users/u_1", nil)
	rr := httptest.NewRecorder()
	longMessage := strings.Repeat("a", 5000) + "世"

	if err := WriteError(rr, req, errx.NewHTTPErrorWithCause(
		http.StatusInternalServerError,
		"internal_error",
		"",
		errors.New(longMessage),
	)); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	logEntry := decodePayload(t, bytes.TrimSpace(defaultBuf.Bytes()))
	message, ok := logEntry["error.message"].(string)
	if !ok {
		t.Fatalf("error.message = %#v, want string", logEntry["error.message"])
	}
	if !strings.HasSuffix(message, "...(truncated)") {
		t.Fatalf("error.message = %q, want truncated suffix", message)
	}
	if !utf8.ValidString(message) {
		t.Fatalf("error.message is not valid UTF-8: %q", message)
	}
	if strings.Contains(message, "世") {
		t.Fatalf("error.message = %q, want multibyte tail trimmed", message)
	}
}

func TestLogStartedServerError(t *testing.T) {
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	var responder *ErrorResponder
	responder.logStartedServerError(nil, nil, nil, nil)
	responder.logStartedServerError(nil, nil, errx.BadRequest("bad_request", "bad request"), errors.New("client error"))
	if defaultBuf.Len() != 0 {
		t.Fatalf("logStartedServerError() unexpectedly wrote output: %s", defaultBuf.Bytes())
	}

	req := httptest.NewRequest(http.MethodGet, "/started", nil)
	startedInner := &stateTrackingWriter{inner: httptest.NewRecorder()}
	startedInner.WriteHeader(http.StatusAccepted)
	responder.logStartedServerError(
		req,
		startedInner,
		errx.NewHTTPError(http.StatusInternalServerError, "internal_error", "Internal Server Error"),
		errors.New("db timeout"),
	)
	if defaultBuf.Len() == 0 {
		t.Fatal("logStartedServerError() did not write 5xx output")
	}

	logEntry := decodePayload(t, defaultBuf.Bytes())
	if got := logEntry["msg"]; got != "resp: request failed with server error" {
		t.Fatalf("msg = %#v, want resp: request failed with server error", got)
	}
	if got := logEntry["http.response.status_code"]; got != float64(http.StatusAccepted) {
		t.Fatalf("http.response.status_code = %#v, want %d", got, http.StatusAccepted)
	}
}

func TestLogStartedServerErrorNoopOnNilAnd4xx(t *testing.T) {
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	var responder *ErrorResponder
	responder.logStartedServerError(nil, nil, nil, nil)
	responder.logStartedServerError(nil, nil, errx.BadRequest("bad_request", "bad request"), errors.New("client error"))

	if defaultBuf.Len() != 0 {
		t.Fatalf("logStartedServerError() unexpectedly wrote output: %s", defaultBuf.Bytes())
	}
}

func TestLogServerErrorWithAttrsNoopOnNilAnd4xx(t *testing.T) {
	var defaultBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&defaultBuf, nil)))
	defer slog.SetDefault(previousDefault)

	var responder *ErrorResponder
	responder.logServerErrorWithAttrs(nil, nil, nil, nil)
	responder.logServerErrorWithAttrs(nil, errx.BadRequest("bad_request", "bad request"), nil, nil)

	if defaultBuf.Len() != 0 {
		t.Fatalf("logServerErrorWithAttrs() unexpectedly wrote output: %s", defaultBuf.Bytes())
	}
}

func TestErrorLogHelpersDefensiveNilPaths(t *testing.T) {
	if got := requestErrorLogAttrs(nil, nil); got != nil {
		t.Fatalf("requestErrorLogAttrs(nil, nil) = %#v, want nil", got)
	}
	if got := diagnosticErrorLogAttrsWithSource(nil, nil, true); got != nil {
		t.Fatalf("diagnosticErrorLogAttrsWithSource(nil, nil) = %#v, want nil", got)
	}

	var responder *ErrorResponder
	responder.logServerErrorAttrs(nil, errx.BadRequest("bad_request", "bad request"), nil)

	if got := buildErrorChainInfo(nil); got != (errorChainInfo{}) {
		t.Fatalf("buildErrorChainInfo(nil) = %#v, want zero value", got)
	}
	if got := isComparableError(nil); got {
		t.Fatal("isComparableError(nil) = true, want false")
	}
	if got := errorTypeName(nil); got != "" {
		t.Fatalf("errorTypeName(nil) = %q, want empty", got)
	}
	if got := safeErrorString(nil); got != "" {
		t.Fatalf("safeErrorString(nil) = %q, want empty", got)
	}
}

func TestFlattenErrorChainFocusedEdgeCases(t *testing.T) {
	if got := flattenErrorChain(nil, maxLoggedErrorChainDepth); got != nil {
		t.Fatalf("flattenErrorChain(nil) = %#v, want nil", got)
	}
	if got := flattenErrorChain(errors.New("x"), 0); got != nil {
		t.Fatalf("flattenErrorChain(limit=0) = %#v, want nil", got)
	}

	wideJoin := errors.Join(errors.New("a"), errors.New("b"), errors.New("c"))
	gotWideJoin := flattenErrorChain(wideJoin, 2)
	if len(gotWideJoin) != 2 {
		t.Fatalf("flattenErrorChain(wideJoin, 2) len = %d, want 2", len(gotWideJoin))
	}
	if got := gotWideJoin[1].Error(); got != "a" {
		t.Fatalf("flattenErrorChain(wideJoin, 2)[1] = %q, want first child a", got)
	}

	multi := &multiUnwrapTestError{errs: []error{nil, errors.New("left"), errors.New("right")}}
	gotMulti := flattenErrorChain(multi, maxLoggedErrorChainDepth)
	if len(gotMulti) != 3 {
		t.Fatalf("flattenErrorChain(multi) len = %d, want 3", len(gotMulti))
	}

	cycle := &cycleTestError{}
	gotCycle := flattenErrorChain(cycle, maxLoggedErrorChainDepth)
	if len(gotCycle) != 1 {
		t.Fatalf("flattenErrorChain(cycle) len = %d, want 1", len(gotCycle))
	}

	wrapped := fmt.Errorf("wrap: %w", errors.New("root"))
	gotWrapped := flattenErrorChain(wrapped, 1)
	if len(gotWrapped) != 1 {
		t.Fatalf("flattenErrorChain(wrapped, 1) len = %d, want 1", len(gotWrapped))
	}

	var typedNil error = (*nilUnsafeTestError)(nil)
	if got := unwrapErrors(typedNil); got != nil {
		t.Fatalf("unwrapErrors(typed nil) = %#v, want nil after panic recovery", got)
	}
}

func TestLimitErrorLogStringPreservesUTF8Boundary(t *testing.T) {
	if got := limitErrorLogString("   "); got != "" {
		t.Fatalf("limitErrorLogString(blank) = %q, want empty", got)
	}

	padding := strings.Repeat("a", maxLoggedErrorStringBytes-1)
	input := padding + "世"
	got := limitErrorLogString(input)

	if !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("limitErrorLogString() = %q, want truncated suffix", got)
	}
	core := strings.TrimSuffix(got, "...(truncated)")
	if !utf8.ValidString(core) {
		t.Fatalf("truncated core is not valid UTF-8: %q", core)
	}
	if strings.Contains(core, "世") {
		t.Fatalf("truncated core contains partial multibyte char: %q", core)
	}
}

func attrsToMap(attrs []slog.Attr) map[string]any {
	out := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		out[attr.Key] = attr.Value.Any()
	}
	return out
}
