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

type standardHTTPErrorConstructorCase struct {
	name       string
	build      func(string, string, ...any) *HTTPError
	wantStatus int
	wantCode   string
}

var standardHTTPErrorConstructors = []standardHTTPErrorConstructorCase{
	{
		name:       "bad request",
		build:      BadRequest,
		wantStatus: http.StatusBadRequest,
		wantCode:   "bad_request",
	},
	{
		name:       "unauthorized",
		build:      Unauthorized,
		wantStatus: http.StatusUnauthorized,
		wantCode:   "unauthorized",
	},
	{
		name:       "forbidden",
		build:      Forbidden,
		wantStatus: http.StatusForbidden,
		wantCode:   "forbidden",
	},
	{
		name:       "not found",
		build:      NotFound,
		wantStatus: http.StatusNotFound,
		wantCode:   "not_found",
	},
	{
		name:       "method not allowed",
		build:      MethodNotAllowed,
		wantStatus: http.StatusMethodNotAllowed,
		wantCode:   "method_not_allowed",
	},
	{
		name:       "conflict",
		build:      Conflict,
		wantStatus: http.StatusConflict,
		wantCode:   "conflict",
	},
	{
		name:       "unprocessable entity",
		build:      UnprocessableEntity,
		wantStatus: http.StatusUnprocessableEntity,
		wantCode:   "unprocessable_entity",
	},
	{
		name:       "too many requests",
		build:      TooManyRequests,
		wantStatus: http.StatusTooManyRequests,
		wantCode:   "too_many_requests",
	},
}

func assertHTTPErrorStatusAndCode(t *testing.T, err *HTTPError, wantStatus int, wantCode string) {
	t.Helper()

	if got := err.Status(); got != wantStatus {
		t.Fatalf("Status() = %d, want %d", got, wantStatus)
	}
	if got := err.Code(); got != wantCode {
		t.Fatalf("Code() = %q, want %q", got, wantCode)
	}
}

func assertHTTPErrorUsesStatusTextPublicMessage(t *testing.T, err *HTTPError, wantStatus int) {
	t.Helper()

	want := http.StatusText(wantStatus)
	if got := err.Title(); got != want {
		t.Fatalf("Title() = %q, want %q", got, want)
	}
	if got := err.Detail(); got != want {
		t.Fatalf("Detail() = %q, want %q", got, want)
	}
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func assertHTTPErrorErrors(t *testing.T, err *HTTPError, want ...string) {
	t.Helper()

	got := err.Errors()
	if len(got) != len(want) {
		t.Fatalf("Errors() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Errors() = %#v, want %#v", got, want)
		}
	}
}

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

// 各个常用错误构造器都会生成稳定的公开契约，并透传显式 code/detail。
func TestHTTPErrorConstructorsExposeExpectedPublicContract(t *testing.T) {
	for _, tc := range standardHTTPErrorConstructors {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("defaults", func(t *testing.T) {
				err := tc.build("", "", "detail")
				assertHTTPErrorStatusAndCode(t, err, tc.wantStatus, tc.wantCode)
				assertHTTPErrorUsesStatusTextPublicMessage(t, err, tc.wantStatus)
				assertHTTPErrorErrors(t, err, "detail")
			})

			t.Run("custom code and detail", func(t *testing.T) {
				err := tc.build("custom_code", "custom detail", "detail")
				assertHTTPErrorStatusAndCode(t, err, tc.wantStatus, "custom_code")
				if got := err.Title(); got != http.StatusText(tc.wantStatus) {
					t.Fatalf("Title() = %q, want %q", got, http.StatusText(tc.wantStatus))
				}
				if got := err.Detail(); got != "custom detail" {
					t.Fatalf("Detail() = %q, want %q", got, "custom detail")
				}
				if got := err.Error(); got != "custom detail" {
					t.Fatalf("Error() = %q, want %q", got, "custom detail")
				}
				assertHTTPErrorErrors(t, err, "detail")
			})
		})
	}
}

// NewHTTPError 会对外暴露稳定、标准化后的状态码/错误码/标题/详情。
func TestNewHTTPErrorNormalizesPublicFields(t *testing.T) {
	testCases := []struct {
		name       string
		status     int
		code       string
		detail     string
		wantStatus int
		wantCode   string
		wantTitle  string
		wantDetail string
	}{
		{
			name:       "explicit code and detail are trimmed",
			status:     http.StatusBadRequest,
			code:       " invalid_json ",
			detail:     " invalid payload ",
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_json",
			wantTitle:  http.StatusText(http.StatusBadRequest),
			wantDetail: "invalid payload",
		},
		{
			name:       "gateway timeout uses timeout code",
			status:     http.StatusGatewayTimeout,
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "timeout",
			wantTitle:  http.StatusText(http.StatusGatewayTimeout),
			wantDetail: http.StatusText(http.StatusGatewayTimeout),
		},
		{
			name:       "service unavailable uses service unavailable code",
			status:     http.StatusServiceUnavailable,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "service_unavailable",
			wantTitle:  http.StatusText(http.StatusServiceUnavailable),
			wantDetail: http.StatusText(http.StatusServiceUnavailable),
		},
		{
			name:       "client closed request supports 499",
			status:     499,
			wantStatus: 499,
			wantCode:   "client_closed_request",
			wantTitle:  "Client Closed Request",
			wantDetail: "Client Closed Request",
		},
		{
			name:       "other client error falls back to client error code",
			status:     http.StatusTeapot,
			wantStatus: http.StatusTeapot,
			wantCode:   "client_error",
			wantTitle:  http.StatusText(http.StatusTeapot),
			wantDetail: http.StatusText(http.StatusTeapot),
		},
		{
			name:       "invalid status falls back to internal server error",
			status:     http.StatusOK,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			wantTitle:  http.StatusText(http.StatusInternalServerError),
			wantDetail: http.StatusText(http.StatusInternalServerError),
		},
		{
			name:       "unknown 5xx preserves status and falls back to internal error public message",
			status:     509,
			wantStatus: 509,
			wantCode:   "internal_error",
			wantTitle:  http.StatusText(http.StatusInternalServerError),
			wantDetail: http.StatusText(http.StatusInternalServerError),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewHTTPError(tc.status, tc.code, tc.detail)

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
			if got := err.Errors(); got != nil {
				t.Fatalf("Errors() = %#v, want nil", got)
			}
		})
	}
}

// HTTPError 实现了 Unwrap，应支持标准库 errors.Is / errors.As 链式查找。
func TestHTTPErrorWorksWithErrorsIsAndAs(t *testing.T) {
	cause := errors.New("db timeout")
	err := NewHTTPErrorWithCause(http.StatusConflict, "", "", cause)

	if !errors.Is(err, cause) {
		t.Fatal("errors.Is should find cause through Unwrap chain")
	}

	var target *HTTPError
	if !errors.As(err, &target) {
		t.Fatal("errors.As should match *HTTPError")
	}
	if target.Status() != http.StatusConflict {
		t.Fatalf("matched HTTPError Status() = %d, want %d", target.Status(), http.StatusConflict)
	}
}

// 构造时不传 errors，Errors() 应返回 nil 而非空切片。
func TestHTTPErrorErrorsReturnsNilWhenNoErrors(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, "bad_request", "bad request")

	if got := err.Errors(); got != nil {
		t.Fatalf("Errors() = %#v, want nil", got)
	}
}
