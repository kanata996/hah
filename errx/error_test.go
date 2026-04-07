package errx

import (
	"errors"
	"net/http"
	"testing"
)

type panicWriteCause struct{}

type blankWriteCause struct{}

func (panicWriteCause) Error() string {
	panic("boom")
}

func (blankWriteCause) Error() string {
	return "   "
}

// 测试清单：
// [✓] nil 接收者返回安全默认值
// [✓] cause、Error、Unwrap、Detail、Errors 的公开语义稳定
// [✓] Detail 和 Errors 对外暴露标准化后的字段，并在构造时与读取时都做防御性切片拷贝
// [✓] 常用错误构造器输出稳定的状态码、错误码和公开消息
// [✓] 状态码、错误码、标题、详情标准化包含 499 特例

// nil 的 HTTPError 接收者应返回一组安全默认值，避免调用方二次判空。
func TestHTTPErrorNilReceiverUsesSafeDefaults(t *testing.T) {
	var err *HTTPError

	if got := err.Error(); got != "" {
		t.Fatalf("Error() = %q, want empty", got)
	}
	if got := err.Unwrap(); got != nil {
		t.Fatalf("Unwrap() = %v, want nil", got)
	}
	if got := err.Status(); got != http.StatusInternalServerError {
		t.Fatalf("Status() = %d, want %d", got, http.StatusInternalServerError)
	}
	if got := err.Code(); got != "internal_error" {
		t.Fatalf("Code() = %q, want internal_error", got)
	}
	if got := err.Title(); got != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("Title() = %q, want %q", got, http.StatusText(http.StatusInternalServerError))
	}
	if got := err.Detail(); got != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("Detail() = %q, want %q", got, http.StatusText(http.StatusInternalServerError))
	}
	if got := err.Errors(); got != nil {
		t.Fatalf("Errors() = %#v, want nil", got)
	}
}

// HTTPError 会优先暴露底层 cause，并对 details 做防御性拷贝。
func TestHTTPErrorUsesCauseAndClonesDetails(t *testing.T) {
	cause := errors.New("db timeout")
	err := NewHTTPErrorWithCause(http.StatusConflict, "", "", cause, "detail")

	if got := err.Error(); got != cause.Error() {
		t.Fatalf("Error() = %q, want %q", got, cause.Error())
	}
	if got := err.Unwrap(); !errors.Is(got, cause) {
		t.Fatalf("Unwrap() = %v, want %v", got, cause)
	}
	if got := err.Status(); got != http.StatusConflict {
		t.Fatalf("Status() = %d, want %d", got, http.StatusConflict)
	}
	if got := err.Code(); got != "conflict" {
		t.Fatalf("Code() = %q, want conflict", got)
	}
	if got := err.Detail(); got != http.StatusText(http.StatusConflict) {
		t.Fatalf("Detail() = %q, want %q", got, http.StatusText(http.StatusConflict))
	}

	details := err.Errors()
	if len(details) != 1 || details[0] != "detail" {
		t.Fatalf("Errors() = %#v, want [detail]", details)
	}
	details[0] = "changed"
	if got := err.Errors()[0]; got != "detail" {
		t.Fatalf("Errors() after mutation = %#v, want detail", got)
	}
}

// cause 的 Error() 实现不安全时，HTTPError.Error 也应退化到稳定公开文案。
func TestHTTPErrorErrorFallsBackWhenCausePanics(t *testing.T) {
	err := NewHTTPErrorWithCause(http.StatusBadRequest, "", "", panicWriteCause{})

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Error() panicked: %v", recovered)
		}
	}()

	if got := err.Error(); got != http.StatusText(http.StatusBadRequest) {
		t.Fatalf("Error() = %q, want %q", got, http.StatusText(http.StatusBadRequest))
	}
}

// cause 文本为空白时，HTTPError.Error 也不应返回空串。
func TestHTTPErrorErrorFallsBackWhenCauseMessageBlank(t *testing.T) {
	err := NewHTTPErrorWithCause(http.StatusBadRequest, "", "", blankWriteCause{})

	if got := err.Error(); got != http.StatusText(http.StatusBadRequest) {
		t.Fatalf("Error() = %q, want %q", got, http.StatusText(http.StatusBadRequest))
	}
}

// Detail/Errors 会暴露公共字段，并返回独立的切片副本给调用方修改。
func TestHTTPErrorDetailAndErrorsExposePublicFields(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, " invalid_json ", " invalid payload ", "detail")

	if got := err.Detail(); got != "invalid payload" {
		t.Fatalf("Detail() = %q, want %q", got, "invalid payload")
	}

	gotErrors := err.Errors()
	if len(gotErrors) != 1 || gotErrors[0] != "detail" {
		t.Fatalf("Errors() = %#v, want [detail]", gotErrors)
	}
	gotErrors[0] = "changed"
	if got := err.Errors()[0]; got != "detail" {
		t.Fatalf("Errors() after mutation = %#v, want detail", got)
	}
}

// 构造 HTTPError 时会立刻拷贝 errors 入参，避免调用方后续修改原切片影响错误对象。
func TestNewHTTPErrorClonesInputErrorsSlice(t *testing.T) {
	input := []any{"detail"}
	err := NewHTTPError(http.StatusBadRequest, "bad_request", "bad request", input...)

	input[0] = "changed"

	gotErrors := err.Errors()
	if len(gotErrors) != 1 || gotErrors[0] != "detail" {
		t.Fatalf("Errors() = %#v, want original [detail]", gotErrors)
	}
}

// 没有 cause 时，HTTPError.Error 会回退为公开消息本身。
func TestHTTPErrorErrorReturnsMessageWithoutCause(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, "bad_request", "bad request")

	if got := err.Error(); got != "bad request" {
		t.Fatalf("Error() = %q, want %q", got, "bad request")
	}
}

// 即使 detail 为空，Error 也应与公开 Detail 保持一致，不返回空串。
func TestHTTPErrorErrorFallsBackToNormalizedDetail(t *testing.T) {
	err := NewHTTPErrorWithCause(http.StatusBadRequest, "", "", nil)

	if got := err.Error(); got != http.StatusText(http.StatusBadRequest) {
		t.Fatalf("Error() = %q, want %q", got, http.StatusText(http.StatusBadRequest))
	}
}

// 各个常用错误构造器都会生成稳定的状态码、错误码和公开消息。
func TestHTTPErrorConstructors(t *testing.T) {
	testCases := []struct {
		name       string
		build      func() *HTTPError
		wantStatus int
		wantCode   string
		wantTitle  string
		wantDetail string
	}{
		{
			name:       "bad request",
			build:      func() *HTTPError { return BadRequest("", "", "detail") },
			wantStatus: http.StatusBadRequest,
			wantCode:   "bad_request",
			wantTitle:  http.StatusText(http.StatusBadRequest),
			wantDetail: http.StatusText(http.StatusBadRequest),
		},
		{
			name:       "unauthorized",
			build:      func() *HTTPError { return Unauthorized("", "", "detail") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthorized",
			wantTitle:  http.StatusText(http.StatusUnauthorized),
			wantDetail: http.StatusText(http.StatusUnauthorized),
		},
		{
			name:       "forbidden",
			build:      func() *HTTPError { return Forbidden("", "", "detail") },
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
			wantTitle:  http.StatusText(http.StatusForbidden),
			wantDetail: http.StatusText(http.StatusForbidden),
		},
		{
			name:       "not found",
			build:      func() *HTTPError { return NotFound("", "", "detail") },
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
			wantTitle:  http.StatusText(http.StatusNotFound),
			wantDetail: http.StatusText(http.StatusNotFound),
		},
		{
			name:       "method not allowed",
			build:      func() *HTTPError { return MethodNotAllowed("", "", "detail") },
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
			wantTitle:  http.StatusText(http.StatusMethodNotAllowed),
			wantDetail: http.StatusText(http.StatusMethodNotAllowed),
		},
		{
			name:       "conflict",
			build:      func() *HTTPError { return Conflict("", "", "detail") },
			wantStatus: http.StatusConflict,
			wantCode:   "conflict",
			wantTitle:  http.StatusText(http.StatusConflict),
			wantDetail: http.StatusText(http.StatusConflict),
		},
		{
			name:       "unprocessable entity",
			build:      func() *HTTPError { return UnprocessableEntity("", "", "detail") },
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "unprocessable_entity",
			wantTitle:  http.StatusText(http.StatusUnprocessableEntity),
			wantDetail: http.StatusText(http.StatusUnprocessableEntity),
		},
		{
			name:       "too many requests",
			build:      func() *HTTPError { return TooManyRequests("", "", "detail") },
			wantStatus: http.StatusTooManyRequests,
			wantCode:   "too_many_requests",
			wantTitle:  http.StatusText(http.StatusTooManyRequests),
			wantDetail: http.StatusText(http.StatusTooManyRequests),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.build()
			if got := err.Status(); got != tc.wantStatus {
				t.Fatalf("Status() = %d, want %d", got, tc.wantStatus)
			}
			if got := err.Code(); got != tc.wantCode {
				t.Fatalf("Code() = %q, want %q", got, tc.wantCode)
			}
			if got := err.Title(); got != tc.wantTitle {
				t.Fatalf("Title() = %q, want %q", got, tc.wantTitle)
			}
			if got := err.Detail(); got != tc.wantDetail {
				t.Fatalf("Detail() = %q, want %q", got, tc.wantDetail)
			}
			if got := err.Error(); got != tc.wantDetail {
				t.Fatalf("Error() = %q, want %q", got, tc.wantDetail)
			}
			if got := err.Errors(); len(got) != 1 || got[0] != "detail" {
				t.Fatalf("Errors() = %#v, want [detail]", got)
			}
		})
	}
}

// 错误码标准化会裁剪显式值，并按状态码回退到约定错误码。
func TestNormalizeErrorCode(t *testing.T) {
	testCases := []struct {
		name   string
		status int
		code   string
		want   string
	}{
		{name: "explicit", status: http.StatusBadRequest, code: " invalid_json ", want: "invalid_json"},
		{name: "bad request", status: http.StatusBadRequest, want: "bad_request"},
		{name: "unauthorized", status: http.StatusUnauthorized, want: "unauthorized"},
		{name: "forbidden", status: http.StatusForbidden, want: "forbidden"},
		{name: "not found", status: http.StatusNotFound, want: "not_found"},
		{name: "method not allowed", status: http.StatusMethodNotAllowed, want: "method_not_allowed"},
		{name: "conflict", status: http.StatusConflict, want: "conflict"},
		{name: "unprocessable entity", status: http.StatusUnprocessableEntity, want: "unprocessable_entity"},
		{name: "too many requests", status: http.StatusTooManyRequests, want: "too_many_requests"},
		{name: "service unavailable", status: http.StatusServiceUnavailable, want: "service_unavailable"},
		{name: "gateway timeout", status: http.StatusGatewayTimeout, want: "timeout"},
		{name: "client closed request", status: 499, want: "client_closed_request"},
		{name: "internal error", status: http.StatusInternalServerError, want: "internal_error"},
		{name: "other client error", status: http.StatusTeapot, want: "client_error"},
		{name: "invalid status normalized to internal", status: 200, want: "internal_error"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeErrorCode(tc.status, tc.code); got != tc.want {
				t.Fatalf("normalizeErrorCode(%d, %q) = %q, want %q", tc.status, tc.code, got, tc.want)
			}
		})
	}
}

func TestNormalizeErrorTitleSupports499(t *testing.T) {
	if got := normalizeErrorTitle(499); got != "Client Closed Request" {
		t.Fatalf("normalizeErrorTitle(499) = %q, want Client Closed Request", got)
	}
	if got := normalizeErrorTitle(509); got != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("normalizeErrorTitle(509) = %q, want %q", got, http.StatusText(http.StatusInternalServerError))
	}
}

func TestNormalizeErrorDetail(t *testing.T) {
	testCases := []struct {
		name   string
		status int
		detail string
		want   string
	}{
		{name: "explicit", status: http.StatusBadRequest, detail: " invalid payload ", want: "invalid payload"},
		{name: "status text fallback", status: http.StatusBadRequest, want: http.StatusText(http.StatusBadRequest)},
		{name: "client closed request", status: 499, want: "Client Closed Request"},
		{name: "invalid status fallback", status: 777, want: http.StatusText(http.StatusInternalServerError)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeErrorDetail(tc.status, tc.detail); got != tc.want {
				t.Fatalf("normalizeErrorDetail(%d, %q) = %q, want %q", tc.status, tc.detail, got, tc.want)
			}
		})
	}
}

func TestNewHTTPErrorSupports499Defaults(t *testing.T) {
	err := NewHTTPError(499, "", "")

	if got := err.Status(); got != 499 {
		t.Fatalf("Status() = %d, want 499", got)
	}
	if got := err.Code(); got != "client_closed_request" {
		t.Fatalf("Code() = %q, want client_closed_request", got)
	}
	if got := err.Title(); got != "Client Closed Request" {
		t.Fatalf("Title() = %q, want Client Closed Request", got)
	}
	if got := err.Detail(); got != "Client Closed Request" {
		t.Fatalf("Detail() = %q, want Client Closed Request", got)
	}
}
