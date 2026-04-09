package resp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

// 测试清单：
// [✓] 错误链摘要兼容 nil、普通包装、不可比较 error、typed-nil 和 panic Error()
// [✓] 错误链展开兼容深度限制、errors.Join、多分支 unwrap 和循环引用
// [✓] 诊断起点优先使用 HTTPError 的内部 cause，没有 cause 时回退原始 error
// [✓] 请求日志字段仅在 5xx 场景补充低噪音诊断字段；关联字段与 root cause 诊断保持稳定
// [✓] 错误类型名与错误文本裁剪对 nil、空白、超长输入保持稳定

type cycleTestError struct{}

type multiUnwrapTestError struct {
	errs []error
}

type nonComparableWrappedTestError struct {
	op     string
	frames []string
	err    error
}

type nilUnsafeTestError struct {
	err error
}

type blankMessageTestError struct{}

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

func (blankMessageTestError) Error() string {
	return "   "
}

// 错误链摘要会在 nil 输入时返回零值，并对多层包装提取首尾信息。
func TestBuildErrorChainInfo(t *testing.T) {
	if got := buildErrorChainInfo(nil); got.message != "" || got.errorType != "" || got.rootMessage != "" || got.rootType != "" {
		t.Fatalf("buildErrorChainInfo(nil) = %#v, want zero value fields", got)
	}

	root := errors.New("db timeout")
	info := buildErrorChainInfo(fmt.Errorf("load user: %w", root))
	if got := info.message; got != "load user: db timeout" {
		t.Fatalf("message = %q, want wrapped message", got)
	}
	if got := info.rootMessage; got != "db timeout" {
		t.Fatalf("rootMessage = %q, want db timeout", got)
	}
	if got := info.rootType; got != "*errors.errorString" {
		t.Fatalf("rootType = %q, want *errors.errorString", got)
	}
}

// 不可比较的 error 值不能作为 map key；错误链诊断应安全退化，而不是在去重阶段 panic。
func TestBuildErrorChainInfoWithNonComparableError(t *testing.T) {
	root := errors.New("db timeout")

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("buildErrorChainInfo(non-comparable) panicked: %v", recovered)
		}
	}()

	info := buildErrorChainInfo(nonComparableWrappedTestError{
		op:     "query user",
		frames: []string{"users", "repo"},
		err:    root,
	})
	if got := info.message; got != "query user: db timeout" {
		t.Fatalf("message = %q, want wrapped message", got)
	}
	if got := info.rootMessage; got != "db timeout" {
		t.Fatalf("rootMessage = %q, want db timeout", got)
	}
}

// typed-nil 或不安全的 Error()/Unwrap() 实现不应把日志注解路径打崩。
func TestBuildErrorChainInfoWithTypedNilError(t *testing.T) {
	var err error = (*nilUnsafeTestError)(nil)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("buildErrorChainInfo(typed nil) panicked: %v", recovered)
		}
	}()

	info := buildErrorChainInfo(err)
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

// 错误链展开会兼容 nil、深度限制、errors.Join 和循环引用。
func TestFlattenErrorChain(t *testing.T) {
	if got := flattenErrorChain(nil, maxLoggedErrorChainDepth); got != nil {
		t.Fatalf("flattenErrorChain(nil) = %#v, want nil", got)
	}
	if got := flattenErrorChain(errors.New("x"), 0); got != nil {
		t.Fatalf("flattenErrorChain(limit=0) = %#v, want nil", got)
	}

	joined := errors.Join(errors.New("a"), fmt.Errorf("wrap: %w", errors.New("b")))
	if got := flattenErrorChain(joined, 2); len(got) != 2 {
		t.Fatalf("flattenErrorChain(joined, 2) len = %d, want 2", len(got))
	}

	wideJoin := errors.Join(errors.New("a"), errors.New("b"), errors.New("c"))
	gotWideJoin := flattenErrorChain(wideJoin, 2)
	if len(gotWideJoin) != 2 {
		t.Fatalf("flattenErrorChain(wideJoin, 2) len = %d, want 2", len(gotWideJoin))
	}
	if got := gotWideJoin[1].Error(); got != "a" {
		t.Fatalf("flattenErrorChain(wideJoin, 2)[1] = %q, want first child a", got)
	}

	cycle := &cycleTestError{}
	if got := flattenErrorChain(cycle, maxLoggedErrorChainDepth); len(got) != 1 {
		t.Fatalf("flattenErrorChain(cycle) len = %d, want 1", len(got))
	}

	multi := &multiUnwrapTestError{errs: []error{nil, errors.New("left"), errors.New("right")}}
	if got := flattenErrorChain(multi, maxLoggedErrorChainDepth); len(got) != 3 {
		t.Fatalf("flattenErrorChain(multi) len = %d, want 3", len(got))
	}
}

// unwrapErrors 会统一兼容单个 unwrap、多分支 join 和无下级错误三种场景。
func TestUnwrapErrors(t *testing.T) {
	if got := unwrapErrors(nil); got != nil {
		t.Fatalf("unwrapErrors(nil) = %#v, want nil", got)
	}

	root := errors.New("root")
	single := fmt.Errorf("wrap: %w", root)
	gotSingle := unwrapErrors(single)
	if len(gotSingle) != 1 || !errors.Is(gotSingle[0], root) {
		t.Fatalf("unwrapErrors(single) = %#v, want [root]", gotSingle)
	}

	left := errors.New("left")
	right := errors.New("right")
	gotMulti := unwrapErrors(errors.Join(left, right))
	if len(gotMulti) != 2 || !errors.Is(gotMulti[0], left) || !errors.Is(gotMulti[1], right) {
		t.Fatalf("unwrapErrors(join) = %#v, want [left right]", gotMulti)
	}
}

// 请求日志属性提取会在 nil 输入时返回空，并对 5xx 错误补充低噪音诊断字段。
func TestRequestErrorLogAttrs(t *testing.T) {
	if got := requestErrorLogAttrs(nil, nil); got != nil {
		t.Fatalf("requestErrorLogAttrs(nil, nil) = %#v, want nil", got)
	}

	httpErr := errx.NewHTTPErrorWithCause(http.StatusInternalServerError, "internal_error", "", errors.New("db timeout"))
	attrs := requestErrorLogAttrs(httpErr, httpErr)
	if len(attrs) == 0 {
		t.Fatal("requestErrorLogAttrs() = nil, want attrs")
	}

	values := attrsToMap(attrs)
	if got := values["error.code"]; got != "internal_error" {
		t.Fatalf("error.code = %#v, want internal_error", got)
	}
	if _, exists := values["error.root_message"]; exists {
		t.Fatalf("error.root_message unexpectedly present: %#v", values["error.root_message"])
	}

	clientErr := errx.BadRequest("bad_request", "bad request")
	if got := requestErrorLogAttrs(clientErr, clientErr); got != nil {
		t.Fatalf("requestErrorLogAttrs(4xx) = %#v, want nil", got)
	}
}

func TestDiagnosticErrorLogAttrs(t *testing.T) {
	if got := diagnosticErrorLogAttrs(nil, nil); got != nil {
		t.Fatalf("diagnosticErrorLogAttrs(nil, nil) = %#v, want nil", got)
	}

	canceledErr := errx.NewHTTPErrorWithCause(http.StatusInternalServerError, "internal_error", "", context.Canceled)
	canceledAttrs := attrsToMap(diagnosticErrorLogAttrs(canceledErr, canceledErr))
	if got := canceledAttrs["error.canceled"]; got != true {
		t.Fatalf("error.canceled = %#v, want true", got)
	}

	timeoutCause := fmt.Errorf("query timeout: %w", context.DeadlineExceeded)
	timeoutErr := errx.NewHTTPErrorWithCause(http.StatusInternalServerError, "internal_error", "", timeoutCause)
	timeoutAttrs := attrsToMap(diagnosticErrorLogAttrs(timeoutErr, timeoutErr))
	if got := timeoutAttrs["error.timeout"]; got != true {
		t.Fatalf("error.timeout = %#v, want true", got)
	}
}

func TestLogServerError(t *testing.T) {
	var buf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(previousDefault)

	req := httptest.NewRequest(http.MethodGet, "/users/u_1", nil)
	var responder *ErrorResponder
	responder.logServerError(nil, nil, nil)
	responder.logServerError(nil, errx.BadRequest("bad_request", "bad request"), errors.New("client error"))
	if buf.Len() != 0 {
		t.Fatalf("logServerError() unexpectedly wrote output: %s", buf.Bytes())
	}

	responder.logServerError(req, errx.NewHTTPError(http.StatusInternalServerError, "internal_error", "Internal Server Error"), errors.New("db timeout"))
	if buf.Len() == 0 {
		t.Fatal("logServerError() did not write 5xx output")
	}

	logEntry := decodePayload(t, buf.Bytes())
	if got := logEntry["msg"]; got != "resp: request failed with server error" {
		t.Fatalf("msg = %#v, want resp: request failed with server error", got)
	}
	if got := logEntry["error.code"]; got != "internal_error" {
		t.Fatalf("error.code = %#v, want internal_error", got)
	}
	if got := logEntry["error.message"]; got != "db timeout" {
		t.Fatalf("error.message = %#v, want db timeout", got)
	}
	if got := logEntry["http.response.status_code"]; got != float64(http.StatusInternalServerError) {
		t.Fatalf("http.response.status_code = %#v, want %d", got, http.StatusInternalServerError)
	}
	if got := logEntry["http.request.method"]; got != http.MethodGet {
		t.Fatalf("http.request.method = %#v, want %q", got, http.MethodGet)
	}
	if got := logEntry["url.path"]; got != "/users/u_1" {
		t.Fatalf("url.path = %#v, want /users/u_1", got)
	}
}

func TestLogServerErrorAttrs(t *testing.T) {
	var buf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(previousDefault)

	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	var responder *ErrorResponder
	responder.logServerErrorAttrs(nil, nil, nil)
	responder.logServerErrorAttrs(nil, errx.BadRequest("bad_request", "bad request"), []slog.Attr{
		slog.String("error.code", "bad_request"),
	})
	if buf.Len() != 0 {
		t.Fatalf("logServerErrorAttrs() unexpectedly wrote output: %s", buf.Bytes())
	}

	responder.logServerErrorAttrs(req, errx.NewHTTPError(http.StatusInternalServerError, "internal_error", "Internal Server Error"), []slog.Attr{
		slog.String("error.code", "internal_error"),
	})
	if buf.Len() == 0 {
		t.Fatal("logServerErrorAttrs() did not write 5xx output")
	}

	logEntry := decodePayload(t, buf.Bytes())
	if got := logEntry["msg"]; got != "resp: request failed with server error" {
		t.Fatalf("msg = %#v, want resp: request failed with server error", got)
	}
	if got := logEntry["error.code"]; got != "internal_error" {
		t.Fatalf("error.code = %#v, want internal_error", got)
	}
	if got := logEntry["http.response.status_code"]; got != float64(http.StatusInternalServerError) {
		t.Fatalf("http.response.status_code = %#v, want %d", got, http.StatusInternalServerError)
	}
	if got := logEntry["http.request.method"]; got != http.MethodPost {
		t.Fatalf("http.request.method = %#v, want %q", got, http.MethodPost)
	}
	if got := logEntry["url.path"]; got != "/users" {
		t.Fatalf("url.path = %#v, want /users", got)
	}
}

func TestErrorForDiagnostics(t *testing.T) {
	original := errors.New("original")
	cause := errors.New("db timeout")
	httpErr := errx.NewHTTPErrorWithCause(http.StatusInternalServerError, "", "", cause)

	if got := errorForDiagnostics(original, httpErr); !errors.Is(got, cause) {
		t.Fatalf("errorForDiagnostics() = %v, want cause %v", got, cause)
	}

	withoutCause := errx.NewHTTPError(http.StatusInternalServerError, "", "")
	if got := errorForDiagnostics(original, withoutCause); !errors.Is(got, original) {
		t.Fatalf("errorForDiagnostics() without cause = %v, want original %v", got, original)
	}
}

func TestRequestMetadataAttrs(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/users/u_1", nil)
	attrs := attrsToMap(requestMetadataAttrs(req))
	if got := attrs["http.request.method"]; got != http.MethodDelete {
		t.Fatalf("http.request.method = %#v, want %q", got, http.MethodDelete)
	}
	if got := attrs["url.path"]; got != "/users/u_1" {
		t.Fatalf("url.path = %#v, want /users/u_1", got)
	}
}

// 错误类型名和错误文本裁剪都会对 nil、空白和超长输入给出稳定结果。
func TestErrorTypeNameAndLimitErrorLogString(t *testing.T) {
	if got := errorTypeName(nil); got != "" {
		t.Fatalf("errorTypeName(nil) = %q, want empty", got)
	}
	if got := isComparableError(nil); got {
		t.Fatalf("isComparableError(nil) = true, want false")
	}
	if got := safeErrorString(nil); got != "" {
		t.Fatalf("safeErrorString(nil) = %q, want empty", got)
	}
	if got := safeErrorString(blankMessageTestError{}); got != "" {
		t.Fatalf("safeErrorString(blankMessageTestError) = %q, want empty", got)
	}

	var typedNil error = (*rawTestError)(nil)
	if got := errorTypeName(typedNil); got != "*resp.rawTestError" {
		t.Fatalf("errorTypeName(typed nil) = %q, want *resp.rawTestError", got)
	}

	if got := limitErrorLogString("   "); got != "" {
		t.Fatalf("limitErrorLogString(blank) = %q, want empty", got)
	}
	if got := limitErrorLogString("  keep me  "); got != "keep me" {
		t.Fatalf("limitErrorLogString(trim) = %q, want keep me", got)
	}

	long := strings.Repeat("a", maxLoggedErrorStringBytes+1)
	got := limitErrorLogString(long)
	if !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("limitErrorLogString(long) = %q, want truncated suffix", got)
	}
}

func attrsToMap(attrs []slog.Attr) map[string]any {
	out := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		out[attr.Key] = attr.Value.Any()
	}
	return out
}
