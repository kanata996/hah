package errx

import (
	"errors"
	"net/http"
	"strings"
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
		name:       "explicit code and detail are trimmed only",
		status:     http.StatusBadRequest,
		code:       " Mixed_Code  V1 ",
		detail:     " Mixed  Detail ",
		wantStatus: http.StatusBadRequest,
		wantCode:   "Mixed_Code  V1",
		wantTitle:  http.StatusText(http.StatusBadRequest),
		wantDetail: "Mixed  Detail",
	},
	{
		name:       "gateway timeout uses timeout code",
		status:     http.StatusGatewayTimeout,
		wantStatus: http.StatusGatewayTimeout,
		wantCode:   "timeout",
		wantTitle:  http.StatusText(http.StatusGatewayTimeout),
		wantDetail: "timeout",
	},
	{
		name:       "service unavailable uses service unavailable code",
		status:     http.StatusServiceUnavailable,
		wantStatus: http.StatusServiceUnavailable,
		wantCode:   "service_unavailable",
		wantTitle:  http.StatusText(http.StatusServiceUnavailable),
		wantDetail: "service unavailable",
	},
	{
		name:       "client closed request supports 499",
		status:     499,
		wantStatus: 499,
		wantCode:   "client_closed_request",
		wantTitle:  "Client Closed Request",
		wantDetail: "client closed request",
	},
	{
		name:       "other client error falls back to client error code",
		status:     http.StatusTeapot,
		wantStatus: http.StatusTeapot,
		wantCode:   "client_error",
		wantTitle:  http.StatusText(http.StatusTeapot),
		wantDetail: "client error",
	},
	{
		name:       "unknown client error without status text falls back to client error public message",
		status:     430,
		wantStatus: 430,
		wantCode:   "client_error",
		wantTitle:  "Client Error",
		wantDetail: "client error",
	},
	{
		name:       "invalid status falls back to internal server error",
		status:     http.StatusOK,
		wantStatus: http.StatusInternalServerError,
		wantCode:   "internal_error",
		wantTitle:  http.StatusText(http.StatusInternalServerError),
		wantDetail: "internal error",
	},
	{
		name:       "unknown 5xx preserves status and falls back to internal error public message",
		status:     509,
		wantStatus: 509,
		wantCode:   "internal_error",
		wantTitle:  http.StatusText(http.StatusInternalServerError),
		wantDetail: "internal error",
	},
	{
		name:       "blank detail falls back to humanized explicit code",
		status:     http.StatusBadRequest,
		code:       " Mixed_Code  V1 ",
		wantStatus: http.StatusBadRequest,
		wantCode:   "Mixed_Code  V1",
		wantTitle:  http.StatusText(http.StatusBadRequest),
		wantDetail: "mixed code v1",
	},
	{
		name:       "blank detail falls back to trimmed code when humanized explicit code is empty",
		status:     http.StatusBadRequest,
		code:       " _-_ ",
		wantStatus: http.StatusBadRequest,
		wantCode:   "_-_",
		wantTitle:  http.StatusText(http.StatusBadRequest),
		wantDetail: "_-_",
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

func assertHTTPErrorUsesDefaultCodeDerivedDetail(t *testing.T, err *HTTPError, wantStatus int, wantCode string) {
	t.Helper()

	wantTitle := http.StatusText(wantStatus)
	if got := err.Title(); got != wantTitle {
		t.Fatalf("Title() = %q, want %q", got, wantTitle)
	}
	wantDetail := expectedHumanizedCode(wantCode)
	if got := err.Detail(); got != wantDetail {
		t.Fatalf("Detail() = %q, want %q", got, wantDetail)
	}
	assertHTTPErrorHasNoCause(t, err, wantDetail)
}

func assertHTTPErrorPreservesCause(t *testing.T, err *HTTPError, wantDetail string, wantCause error) {
	t.Helper()

	if got := err.Error(); got != wantDetail {
		t.Fatalf("Error() = %q, want %q", got, wantDetail)
	}
	if got := err.Unwrap(); got != wantCause {
		t.Fatalf("Unwrap() = %v, want %v", got, wantCause)
	}
}

func assertHTTPErrorErrors(t *testing.T, err *HTTPError, want ...FieldError) {
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

// 零值 HTTPError 也应对外暴露稳定、可预测的默认错误契约。
func TestHTTPErrorZeroValueUsesNormalizedPublicContract(t *testing.T) {
	var err HTTPError

	assertHTTPErrorPublicFields(
		t,
		&err,
		http.StatusInternalServerError,
		"internal_error",
		http.StatusText(http.StatusInternalServerError),
		"internal error",
	)
	assertHTTPErrorHasNoCause(t, &err, "internal error")
	if got := err.Errors(); got != nil {
		t.Fatalf("Errors() = %#v, want nil", got)
	}
}

// Error() 必须只返回公开 Detail，不得依赖 cause.Error()，即使 cause 本身不安全也不能 panic。
func TestHTTPErrorErrorIgnoresCauseWhenCausePanics(t *testing.T) {
	err := NewHTTPErrorWithCause(http.StatusBadRequest, "", "", panicWriteCause{})

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Error() panicked: %v", recovered)
		}
	}()

	if got := err.Error(); got != "bad request" {
		t.Fatalf("Error() = %q, want %q", got, "bad request")
	}
}

// 即使 cause 的 Error() 文本为空白，HTTPError.Error 也必须保持等于公开 Detail。
func TestHTTPErrorErrorIgnoresBlankCauseMessage(t *testing.T) {
	err := NewHTTPErrorWithCause(http.StatusBadRequest, "", "", blankWriteCause{})

	if got := err.Error(); got != "bad request" {
		t.Fatalf("Error() = %q, want %q", got, "bad request")
	}
}

// typed-nil cause 不应被当成真实错误链保留，否则会污染公开错误链语义。
func TestNewHTTPErrorWithCauseTreatsTypedNilCauseAsNoCause(t *testing.T) {
	var cause *typedNilCause

	err := NewHTTPErrorWithCause(http.StatusBadRequest, "", "", cause)

	assertHTTPErrorHasNoCause(t, err, "bad request")
	if errors.Is(err, cause) {
		t.Fatal("errors.Is should not match a typed-nil cause")
	}

	var target *typedNilCause
	if errors.As(err, &target) {
		t.Fatal("errors.As should not expose a typed-nil cause")
	}
}

// nil receiver 的 Unwrap 必须安全返回 nil，避免 typed-nil *HTTPError 污染错误链扫描。
func TestHTTPErrorUnwrapNilReceiverReturnsNil(t *testing.T) {
	var err *HTTPError

	if got := err.Unwrap(); got != nil {
		t.Fatalf("Unwrap() = %v, want nil", got)
	}
}

// WithFieldErrors 的结果不应受入参切片或 Errors() 返回结果后续修改影响；有无 cause 时契约一致。
func TestHTTPErrorWithFieldErrorsClonesInputAndReturnedSlices(t *testing.T) {
	testCases := []struct {
		name  string
		cause error
		build func(error, []FieldError) *HTTPError
	}{
		{
			name: "without cause",
			build: func(_ error, fieldErrors []FieldError) *HTTPError {
				return NewHTTPError(http.StatusBadRequest, " invalid_json ", " invalid payload ").WithFieldErrors(fieldErrors)
			},
		},
		{
			name:  "with cause",
			cause: errors.New("db timeout"),
			build: func(cause error, fieldErrors []FieldError) *HTTPError {
				return NewHTTPErrorWithCause(http.StatusConflict, "", "", cause).WithFieldErrors(fieldErrors)
			},
		},
	}

	want := []FieldError{{Field: "name", In: InBody, Code: CodeRequired, Detail: "is required"}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			input := []FieldError{{Field: "name", In: InBody, Code: CodeRequired, Detail: "is required"}}
			err := tc.build(tc.cause, input)

			input[0].Field = "changed"
			assertHTTPErrorErrors(t, err, want...)

			gotErrors := err.Errors()
			gotErrors[0] = FieldError{Field: "changed", In: InQuery, Code: CodeInvalid, Detail: "is invalid"}
			assertHTTPErrorErrors(t, err, want...)
		})
	}
}

// WithFieldErrors 应返回携带独立 field errors 的新错误对象，不能污染模板对象或兄弟结果。
func TestHTTPErrorWithFieldErrorsDoesNotMutateReceiverOrSiblingResults(t *testing.T) {
	cause := errors.New("db timeout")
	baseWant := []FieldError{{Field: "base", In: InHeader, Code: CodeUnknown, Detail: "is duplicated"}}
	base := NewHTTPErrorWithCause(http.StatusConflict, " account_conflict ", " account already exists ", cause).WithFieldErrors(baseWant)

	firstWant := []FieldError{{Field: "name", In: InBody, Code: CodeRequired, Detail: "is required"}}
	secondWant := []FieldError{{Field: "email", In: InQuery, Code: CodeInvalid, Detail: "is invalid"}}

	first := base.WithFieldErrors(firstWant)
	second := base.WithFieldErrors(secondWant)

	assertHTTPErrorPublicFields(
		t,
		base,
		http.StatusConflict,
		"account_conflict",
		http.StatusText(http.StatusConflict),
		"account already exists",
	)
	assertHTTPErrorPreservesCause(t, base, "account already exists", cause)
	assertHTTPErrorErrors(t, base, baseWant...)

	assertHTTPErrorErrors(t, first, firstWant...)
	assertHTTPErrorPreservesCause(t, first, "account already exists", cause)

	assertHTTPErrorErrors(t, second, secondWant...)
	assertHTTPErrorPreservesCause(t, second, "account already exists", cause)

	// 第二次基于同一模板构造，不能回头覆盖第一次的 field errors。
	assertHTTPErrorErrors(t, first, firstWant...)
}

// 基于已有 field errors 再传入 nil/空切片时，应显式清空旧 field errors，而不是保留或合并。
func TestHTTPErrorWithFieldErrorsNilAndEmptyInputReplaceExistingFieldErrors(t *testing.T) {
	cause := errors.New("db timeout")
	baseWant := []FieldError{{Field: "name", In: InBody, Code: CodeRequired, Detail: "is required"}}
	base := NewHTTPErrorWithCause(http.StatusConflict, " account_conflict ", " account already exists ", cause).WithFieldErrors(baseWant)

	testCases := []struct {
		name        string
		fieldErrors []FieldError
	}{
		{name: "nil input", fieldErrors: nil},
		{name: "empty input", fieldErrors: []FieldError{}},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := base.WithFieldErrors(tc.fieldErrors)

			assertHTTPErrorPublicFields(
				t,
				err,
				http.StatusConflict,
				"account_conflict",
				http.StatusText(http.StatusConflict),
				"account already exists",
			)
			assertHTTPErrorPreservesCause(t, err, "account already exists", cause)
			if got := err.Errors(); got != nil {
				t.Fatalf("Errors() = %#v, want nil", got)
			}

			// 原模板必须保持不变，证明这里是“替换”为 nil，而不是污染 receiver。
			assertHTTPErrorErrors(t, base, baseWant...)
		})
	}
}

// 即使 detail 为空，Error 也应与公开 Detail 保持一致，不返回空串。
func TestHTTPErrorErrorFallsBackToNormalizedDetail(t *testing.T) {
	err := NewHTTPErrorWithCause(http.StatusBadRequest, "", "", nil)

	assertHTTPErrorHasNoCause(t, err, "bad request")
}

func TestHTTPErrorPreservesExplicitDetailEvenWhenItMatchesTitle(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, "invalid_json", "Bad Request")

	assertHTTPErrorPublicFields(
		t,
		err,
		http.StatusBadRequest,
		"invalid_json",
		http.StatusText(http.StatusBadRequest),
		"Bad Request",
	)
	assertHTTPErrorHasNoCause(t, err, "Bad Request")
}

// 各个常用错误构造器都会生成稳定的公开契约，并透传显式 code/detail。
func TestHTTPErrorConstructorsExposeExpectedPublicContract(t *testing.T) {
	for _, tc := range standardHTTPErrorConstructors {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("defaults", func(t *testing.T) {
				err := tc.build("", "")
				assertHTTPErrorStatusAndCode(t, err, tc.wantStatus, tc.wantCode)
				assertHTTPErrorUsesDefaultCodeDerivedDetail(t, err, tc.wantStatus, tc.wantCode)
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
			assertHTTPErrorPreservesCause(t, err, tc.wantDetail, cause)
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

// HTTPError 实现了 Unwrap，应支持标准库 errors.Is / errors.As 链式查找，且 Error() 始终等于公开 Detail。
func TestHTTPErrorWorksWithErrorsIsAndAs(t *testing.T) {
	cause := errors.New("db timeout")
	err := NewHTTPErrorWithCause(http.StatusConflict, "", "", cause)

	assertHTTPErrorPreservesCause(t, err, "conflict", cause)
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

// 构造时不设置 field errors，Errors() 应返回 nil 而非空切片。
func TestHTTPErrorErrorsReturnsNilWhenNoErrors(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, "bad_request", "bad request")

	if got := err.Errors(); got != nil {
		t.Fatalf("Errors() = %#v, want nil", got)
	}
}

// nil 与空切片输入都应收敛为无 field errors 的稳定 nil 表示。
func TestHTTPErrorWithFieldErrorsNilAndEmptySliceNormalizeToNil(t *testing.T) {
	testCases := []struct {
		name        string
		fieldErrors []FieldError
	}{
		{name: "nil input", fieldErrors: nil},
		{name: "empty input", fieldErrors: []FieldError{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewHTTPError(http.StatusBadRequest, "bad_request", "bad request").WithFieldErrors(tc.fieldErrors)

			if got := err.Errors(); got != nil {
				t.Fatalf("Errors() = %#v, want nil", got)
			}
		})
	}
}

// 默认 code 表属于锁版公开契约。
func TestHTTPErrorDefaultCodeTable(t *testing.T) {
	testCases := []struct {
		status   int
		wantCode string
	}{
		{status: http.StatusBadRequest, wantCode: "bad_request"},
		{status: http.StatusUnauthorized, wantCode: "unauthorized"},
		{status: http.StatusForbidden, wantCode: "forbidden"},
		{status: http.StatusNotFound, wantCode: "not_found"},
		{status: http.StatusMethodNotAllowed, wantCode: "method_not_allowed"},
		{status: http.StatusConflict, wantCode: "conflict"},
		{status: http.StatusUnprocessableEntity, wantCode: "unprocessable_entity"},
		{status: http.StatusTooManyRequests, wantCode: "too_many_requests"},
		{status: 499, wantCode: "client_closed_request"},
		{status: http.StatusServiceUnavailable, wantCode: "service_unavailable"},
		{status: http.StatusGatewayTimeout, wantCode: "timeout"},
		{status: http.StatusTeapot, wantCode: "client_error"},
		{status: 509, wantCode: "internal_error"},
	}

	for _, tc := range testCases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			err := NewHTTPError(tc.status, "", "")
			if got := err.Code(); got != tc.wantCode {
				t.Fatalf("Code() = %q, want %q", got, tc.wantCode)
			}
		})
	}
}

// 默认 title 表属于锁版公开契约。
func TestHTTPErrorDefaultTitleTable(t *testing.T) {
	testCases := []struct {
		name       string
		status     int
		wantStatus int
		wantTitle  string
	}{
		{
			name:       "standard client error uses net/http status text",
			status:     http.StatusBadRequest,
			wantStatus: http.StatusBadRequest,
			wantTitle:  http.StatusText(http.StatusBadRequest),
		},
		{
			name:       "499 uses explicit client closed request title",
			status:     499,
			wantStatus: 499,
			wantTitle:  "Client Closed Request",
		},
		{
			name:       "other client error with status text uses that text",
			status:     http.StatusTeapot,
			wantStatus: http.StatusTeapot,
			wantTitle:  http.StatusText(http.StatusTeapot),
		},
		{
			name:       "other client error without status text falls back to client error",
			status:     430,
			wantStatus: 430,
			wantTitle:  "Client Error",
		},
		{
			name:       "standard server error uses net/http status text",
			status:     http.StatusServiceUnavailable,
			wantStatus: http.StatusServiceUnavailable,
			wantTitle:  http.StatusText(http.StatusServiceUnavailable),
		},
		{
			name:       "other server error without status text falls back to internal server error",
			status:     509,
			wantStatus: 509,
			wantTitle:  http.StatusText(http.StatusInternalServerError),
		},
		{
			name:       "non error status normalizes to internal server error title",
			status:     http.StatusOK,
			wantStatus: http.StatusInternalServerError,
			wantTitle:  http.StatusText(http.StatusInternalServerError),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewHTTPError(tc.status, "", "")

			if got := err.Status(); got != tc.wantStatus {
				t.Fatalf("Status() = %d, want %d", got, tc.wantStatus)
			}
			if got := err.Title(); got != tc.wantTitle {
				t.Fatalf("Title() = %q, want %q", got, tc.wantTitle)
			}
			wantDetail := expectedHumanizedCode(defaultErrorCode(tc.wantStatus))
			if got := err.Detail(); got != wantDetail {
				t.Fatalf("Detail() = %q, want %q", got, wantDetail)
			}
		})
	}
}

func expectedHumanizedCode(code string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ")
	return strings.ToLower(strings.Join(strings.Fields(replacer.Replace(code)), " "))
}

// 共享 field error 常量的 string 值属于公开可观察语义。
func TestFieldErrorSharedConstantsExposeStableValues(t *testing.T) {
	codeCases := []struct {
		name string
		got  FieldErrorCode
		want string
	}{
		{name: "invalid", got: CodeInvalid, want: "invalid"},
		{name: "required", got: CodeRequired, want: "required"},
		{name: "unknown", got: CodeUnknown, want: "unknown"},
		{name: "type", got: CodeType, want: "type"},
		{name: "multiple", got: CodeMultiple, want: "multiple"},
	}

	for _, tc := range codeCases {
		t.Run("code/"+tc.name, func(t *testing.T) {
			if got := string(tc.got); got != tc.want {
				t.Fatalf("string(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}

	inCases := []struct {
		name string
		got  FieldErrorIn
		want string
	}{
		{name: "body", got: InBody, want: "body"},
		{name: "query", got: InQuery, want: "query"},
		{name: "path", got: InPath, want: "path"},
		{name: "header", got: InHeader, want: "header"},
	}

	for _, tc := range inCases {
		t.Run("in/"+tc.name, func(t *testing.T) {
			if got := string(tc.got); got != tc.want {
				t.Fatalf("string(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// WithFieldErrors 必须保留顺序和重复项，不排序、不去重。
func TestHTTPErrorWithFieldErrorsPreservesOrderAndDuplicates(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, "", "").WithFieldErrors([]FieldError{
		{Field: "name", In: InBody, Code: CodeInvalid, Detail: "is invalid"},
		{Field: "name", In: InBody, Code: CodeInvalid, Detail: "is invalid"},
		{Field: "email", In: InQuery, Code: CodeRequired, Detail: "is required"},
	})

	assertHTTPErrorErrors(
		t,
		err,
		FieldError{Field: "name", In: InBody, Code: CodeInvalid, Detail: "is invalid"},
		FieldError{Field: "name", In: InBody, Code: CodeInvalid, Detail: "is invalid"},
		FieldError{Field: "email", In: InQuery, Code: CodeRequired, Detail: "is required"},
	)
}
