package errx

import (
	"errors"
	"net/http"
	"testing"
)

type panicWriteCause struct{}

type blankWriteCause struct{}

type typedNilCause struct{}

func (panicWriteCause) Error() string {
	panic("boom")
}

func (blankWriteCause) Error() string {
	return "   "
}

func (*typedNilCause) Error() string {
	return "typed nil cause"
}

type standardHTTPErrorConstructorCase struct {
	name       string
	build      func(string, string) *HTTPError
	wantStatus int
	wantCode   string
}

type normalizedHTTPErrorPublicFieldCase struct {
	name       string
	status     int
	code       string
	detail     string
	wantStatus int
	wantCode   string
	wantTitle  string
	wantDetail string
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

var normalizedHTTPErrorPublicFieldCases = []normalizedHTTPErrorPublicFieldCase{
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
		name:       "unknown client error without status text falls back to client error public message",
		status:     430,
		wantStatus: 430,
		wantCode:   "client_error",
		wantTitle:  "Client Error",
		wantDetail: "Client Error",
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

func assertHTTPErrorStatusAndCode(t *testing.T, err *HTTPError, wantStatus int, wantCode string) {
	t.Helper()

	if got := err.Status(); got != wantStatus {
		t.Fatalf("Status() = %d, want %d", got, wantStatus)
	}
	if got := err.Code(); got != wantCode {
		t.Fatalf("Code() = %q, want %q", got, wantCode)
	}
}

func assertHTTPErrorPublicFields(t *testing.T, err *HTTPError, wantStatus int, wantCode, wantTitle, wantDetail string) {
	t.Helper()

	assertHTTPErrorStatusAndCode(t, err, wantStatus, wantCode)
	if got := err.Title(); got != wantTitle {
		t.Fatalf("Title() = %q, want %q", got, wantTitle)
	}
	if got := err.Detail(); got != wantDetail {
		t.Fatalf("Detail() = %q, want %q", got, wantDetail)
	}
}

func assertHTTPErrorHasNoCause(t *testing.T, err *HTTPError, wantError string) {
	t.Helper()

	if got := err.Error(); got != wantError {
		t.Fatalf("Error() = %q, want %q", got, wantError)
	}
	if got := err.Unwrap(); got != nil {
		t.Fatalf("Unwrap() = %v, want nil", got)
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
	assertHTTPErrorHasNoCause(t, err, want)
}

func assertHTTPErrorErrors(t *testing.T, err *HTTPError, want ...Violation) {
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

// 零值 HTTPError 也应对外暴露稳定、可预测的默认错误契约。
func TestHTTPErrorZeroValueUsesNormalizedPublicContract(t *testing.T) {
	var err HTTPError

	assertHTTPErrorPublicFields(
		t,
		&err,
		http.StatusInternalServerError,
		"internal_error",
		http.StatusText(http.StatusInternalServerError),
		http.StatusText(http.StatusInternalServerError),
	)
	assertHTTPErrorHasNoCause(t, &err, http.StatusText(http.StatusInternalServerError))
	if got := err.Errors(); got != nil {
		t.Fatalf("Errors() = %#v, want nil", got)
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

// typed-nil cause 不应被当成真实错误链保留，否则会污染 Error/Unwrap/Is/As 语义。
func TestNewHTTPErrorWithCauseTreatsTypedNilCauseAsNoCause(t *testing.T) {
	var cause *typedNilCause

	err := NewHTTPErrorWithCause(http.StatusBadRequest, "", "", cause)

	assertHTTPErrorHasNoCause(t, err, http.StatusText(http.StatusBadRequest))
	if errors.Is(err, cause) {
		t.Fatal("errors.Is should not match a typed-nil cause")
	}

	var target *typedNilCause
	if errors.As(err, &target) {
		t.Fatal("errors.As should not expose a typed-nil cause")
	}
}

// WithViolations 会立刻拷贝入参，Errors 也会返回独立副本；有无 cause 时契约一致。
func TestHTTPErrorWithViolationsClonesInputAndReturnedSlices(t *testing.T) {
	testCases := []struct {
		name  string
		cause error
		build func(error, []Violation) *HTTPError
	}{
		{
			name: "without cause",
			build: func(_ error, violations []Violation) *HTTPError {
				return NewHTTPError(http.StatusBadRequest, " invalid_json ", " invalid payload ").WithViolations(violations)
			},
		},
		{
			name:  "with cause",
			cause: errors.New("db timeout"),
			build: func(cause error, violations []Violation) *HTTPError {
				return NewHTTPErrorWithCause(http.StatusConflict, "", "", cause).WithViolations(violations)
			},
		},
	}

	want := []Violation{{Field: "name", In: ViolationInBody, Code: ViolationCodeRequired, Detail: "is required"}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			input := []Violation{{Field: "name", In: ViolationInBody, Code: ViolationCodeRequired, Detail: "is required"}}
			err := tc.build(tc.cause, input)

			input[0].Field = "changed"
			assertHTTPErrorErrors(t, err, want...)

			gotErrors := err.Errors()
			gotErrors[0] = Violation{Field: "changed", In: ViolationInQuery, Code: ViolationCodeInvalid, Detail: "is invalid"}
			assertHTTPErrorErrors(t, err, want...)
		})
	}
}

// WithViolations 应返回携带独立 violations 的新错误对象，不能污染模板对象或兄弟结果。
func TestHTTPErrorWithViolationsDoesNotMutateReceiverOrSiblingResults(t *testing.T) {
	cause := errors.New("db timeout")
	base := NewHTTPErrorWithCause(http.StatusConflict, " account_conflict ", " account already exists ", cause)

	firstWant := []Violation{{Field: "name", In: ViolationInBody, Code: ViolationCodeRequired, Detail: "is required"}}
	secondWant := []Violation{{Field: "email", In: ViolationInQuery, Code: ViolationCodeInvalid, Detail: "is invalid"}}

	first := base.WithViolations(firstWant)
	second := base.WithViolations(secondWant)

	assertHTTPErrorPublicFields(
		t,
		base,
		http.StatusConflict,
		"account_conflict",
		http.StatusText(http.StatusConflict),
		"account already exists",
	)
	if got := base.Error(); got != cause.Error() {
		t.Fatalf("base Error() = %q, want %q", got, cause.Error())
	}
	if got := base.Unwrap(); got != cause {
		t.Fatalf("base Unwrap() = %v, want %v", got, cause)
	}
	assertHTTPErrorErrors(t, base)

	assertHTTPErrorErrors(t, first, firstWant...)
	if got := first.Error(); got != cause.Error() {
		t.Fatalf("first Error() = %q, want %q", got, cause.Error())
	}
	if got := first.Unwrap(); got != cause {
		t.Fatalf("first Unwrap() = %v, want %v", got, cause)
	}

	assertHTTPErrorErrors(t, second, secondWant...)
	if got := second.Error(); got != cause.Error() {
		t.Fatalf("second Error() = %q, want %q", got, cause.Error())
	}
	if got := second.Unwrap(); got != cause {
		t.Fatalf("second Unwrap() = %v, want %v", got, cause)
	}

	// 第二次基于同一模板构造，不能回头覆盖第一次的 violations。
	assertHTTPErrorErrors(t, first, firstWant...)
}

// 即使 detail 为空，Error 也应与公开 Detail 保持一致，不返回空串。
func TestHTTPErrorErrorFallsBackToNormalizedDetail(t *testing.T) {
	err := NewHTTPErrorWithCause(http.StatusBadRequest, "", "", nil)

	assertHTTPErrorHasNoCause(t, err, http.StatusText(http.StatusBadRequest))
}

// 各个常用错误构造器都会生成稳定的公开契约，并透传显式 code/detail。
func TestHTTPErrorConstructorsExposeExpectedPublicContract(t *testing.T) {
	for _, tc := range standardHTTPErrorConstructors {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("defaults", func(t *testing.T) {
				err := tc.build("", "")
				assertHTTPErrorStatusAndCode(t, err, tc.wantStatus, tc.wantCode)
				assertHTTPErrorUsesStatusTextPublicMessage(t, err, tc.wantStatus)
				assertHTTPErrorErrors(t, err)
			})

			t.Run("custom code and detail", func(t *testing.T) {
				err := tc.build("custom_code", "custom detail")
				assertHTTPErrorStatusAndCode(t, err, tc.wantStatus, "custom_code")
				if got := err.Title(); got != http.StatusText(tc.wantStatus) {
					t.Fatalf("Title() = %q, want %q", got, http.StatusText(tc.wantStatus))
				}
				if got := err.Detail(); got != "custom detail" {
					t.Fatalf("Detail() = %q, want %q", got, "custom detail")
				}
				assertHTTPErrorHasNoCause(t, err, "custom detail")
				assertHTTPErrorErrors(t, err)
			})
		})
	}
}

// NewHTTPErrorWithCause 在携带 cause 时也要保持与 NewHTTPError 相同的公开字段标准化。
func TestNewHTTPErrorWithCauseNormalizesPublicFields(t *testing.T) {
	cause := errors.New("db timeout")

	for _, tc := range normalizedHTTPErrorPublicFieldCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewHTTPErrorWithCause(tc.status, tc.code, tc.detail, cause)

			assertHTTPErrorPublicFields(t, err, tc.wantStatus, tc.wantCode, tc.wantTitle, tc.wantDetail)
		})
	}
}

// NewHTTPError 会对外暴露稳定、标准化后的状态码/错误码/标题/详情。
func TestNewHTTPErrorNormalizesPublicFields(t *testing.T) {
	for _, tc := range normalizedHTTPErrorPublicFieldCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewHTTPError(tc.status, tc.code, tc.detail)

			assertHTTPErrorPublicFields(t, err, tc.wantStatus, tc.wantCode, tc.wantTitle, tc.wantDetail)
			assertHTTPErrorHasNoCause(t, err, tc.wantDetail)
		})
	}
}

// HTTPError 实现了 Unwrap，应支持标准库 errors.Is / errors.As 链式查找。
func TestHTTPErrorWorksWithErrorsIsAndAs(t *testing.T) {
	cause := errors.New("db timeout")
	err := NewHTTPErrorWithCause(http.StatusConflict, "", "", cause)

	if got := err.Error(); got != cause.Error() {
		t.Fatalf("Error() = %q, want %q", got, cause.Error())
	}
	if got := err.Unwrap(); got != cause {
		t.Fatalf("Unwrap() = %v, want %v", got, cause)
	}
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

// 构造时不设置 violations，Errors() 应返回 nil 而非空切片。
func TestHTTPErrorErrorsReturnsNilWhenNoErrors(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, "bad_request", "bad request")

	if got := err.Errors(); got != nil {
		t.Fatalf("Errors() = %#v, want nil", got)
	}
}

// nil 接收者调用 WithViolations 应保持 nil-safe，不触发 panic。
func TestHTTPErrorWithViolationsNilReceiverReturnsNil(t *testing.T) {
	var err *HTTPError

	if got := err.WithViolations([]Violation{{Field: "name"}}); got != nil {
		t.Fatalf("WithViolations() = %#v, want nil", got)
	}
}

// 显式传入空切片时，也应收敛为无 violations 的稳定 nil 表示。
func TestHTTPErrorWithViolationsEmptySliceNormalizesToNil(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, "bad_request", "bad request").WithViolations([]Violation{})

	if got := err.Errors(); got != nil {
		t.Fatalf("Errors() = %#v, want nil", got)
	}
}
