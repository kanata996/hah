package errx

import (
	"errors"
	"net/http"
	"testing"
)

type panicWriteCause struct{}

type blankWriteCause struct{}

type namedErrorDetail map[string]any
type namedStringMap map[string]string
type namedStringSlice []string

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

// 测试清单：
// [✓] nil 接收者返回安全默认值
// [✓] cause、Error、Unwrap、Detail、Errors 的公开语义稳定
// [✓] Detail 和 Errors 对外暴露标准化后的字段，并在构造时与读取时都做 JSON 容器的防御性深拷贝
// [✓] 常用错误构造器输出稳定的状态码、错误码和公开消息
// [✓] 状态码、错误码、标题、详情标准化包含 499 特例与非常规状态的公开退化语义
// [✓] errors.Is / errors.As 标准 error 链互操作
// [✓] 快捷构造器自定义 code/detail 透传
// [✓] 构造时不传 errors 时 Errors() 返回 nil

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

// Errors 返回给调用方的 JSON 容器也应是递归拷贝，避免调用方修改嵌套结构污染错误对象。
func TestHTTPErrorErrorsReturnsDeepClonedNestedValues(t *testing.T) {
	err := NewHTTPError(
		http.StatusBadRequest,
		"bad_request",
		"bad request",
		map[string]any{
			"field": "name",
			"meta":  []any{map[string]any{"code": "required"}},
		},
	)

	gotErrors := err.Errors()
	gotDetail, ok := gotErrors[0].(map[string]any)
	if !ok {
		t.Fatalf("Errors()[0] type = %T, want map[string]any", gotErrors[0])
	}
	gotDetail["field"] = "changed"
	gotDetail["meta"].([]any)[0].(map[string]any)["code"] = "changed"

	freshErrors := err.Errors()
	freshDetail, ok := freshErrors[0].(map[string]any)
	if !ok {
		t.Fatalf("fresh Errors()[0] type = %T, want map[string]any", freshErrors[0])
	}
	if freshDetail["field"] != "name" {
		t.Fatalf("fresh Errors()[0][field] = %#v, want name", freshDetail["field"])
	}
	freshMeta, ok := freshDetail["meta"].([]any)
	if !ok || len(freshMeta) != 1 {
		t.Fatalf("fresh Errors()[0][meta] = %#v, want one-item []any", freshDetail["meta"])
	}
	freshMetaItem, ok := freshMeta[0].(map[string]any)
	if !ok {
		t.Fatalf("fresh Errors()[0][meta][0] type = %T, want map[string]any", freshMeta[0])
	}
	if freshMetaItem["code"] != "required" {
		t.Fatalf("fresh Errors()[0][meta][0][code] = %#v, want required", freshMetaItem["code"])
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

// 构造 HTTPError 时会递归拷贝公开 JSON 容器，避免嵌套 map/slice 与调用方共享状态。
func TestNewHTTPErrorDeepClonesNestedErrorValues(t *testing.T) {
	inputDetail := map[string]any{
		"field": "name",
		"meta":  []any{map[string]any{"code": "required"}},
	}

	err := NewHTTPError(http.StatusBadRequest, "bad_request", "bad request", inputDetail)

	inputDetail["field"] = "changed"
	inputDetail["meta"].([]any)[0].(map[string]any)["code"] = "changed"

	gotErrors := err.Errors()
	if len(gotErrors) != 1 {
		t.Fatalf("Errors() len = %d, want 1", len(gotErrors))
	}

	gotDetail, ok := gotErrors[0].(map[string]any)
	if !ok {
		t.Fatalf("Errors()[0] type = %T, want map[string]any", gotErrors[0])
	}
	if gotDetail["field"] != "name" {
		t.Fatalf("Errors()[0][field] = %#v, want name", gotDetail["field"])
	}
	gotMeta, ok := gotDetail["meta"].([]any)
	if !ok || len(gotMeta) != 1 {
		t.Fatalf("Errors()[0][meta] = %#v, want one-item []any", gotDetail["meta"])
	}
	gotMetaItem, ok := gotMeta[0].(map[string]any)
	if !ok {
		t.Fatalf("Errors()[0][meta][0] type = %T, want map[string]any", gotMeta[0])
	}
	if gotMetaItem["code"] != "required" {
		t.Fatalf("Errors()[0][meta][0][code] = %#v, want required", gotMetaItem["code"])
	}
}

// 构造 HTTPError 时也应拷贝常见 JSON-safe map/slice 及其命名类型，而不是只处理精确的 []any/map[string]any。
func TestNewHTTPErrorClonesCommonJSONContainerTypes(t *testing.T) {
	nestedLabels := namedStringMap{"code": "required"}
	nestedNames := namedStringSlice{"name"}
	namedDetail := namedErrorDetail{
		"field":  "account",
		"labels": nestedLabels,
		"names":  nestedNames,
	}
	plainLabels := map[string]string{"scope": "query"}
	plainNames := []string{"id"}

	err := NewHTTPError(http.StatusBadRequest, "bad_request", "bad request", namedDetail, plainLabels, plainNames)

	namedDetail["field"] = "changed"
	nestedLabels["code"] = "changed"
	nestedNames[0] = "changed"
	plainLabels["scope"] = "changed"
	plainNames[0] = "changed"

	gotErrors := err.Errors()
	if len(gotErrors) != 3 {
		t.Fatalf("Errors() len = %d, want 3", len(gotErrors))
	}

	gotNamedDetail, ok := gotErrors[0].(namedErrorDetail)
	if !ok {
		t.Fatalf("Errors()[0] type = %T, want namedErrorDetail", gotErrors[0])
	}
	if gotNamedDetail["field"] != "account" {
		t.Fatalf("Errors()[0][field] = %#v, want account", gotNamedDetail["field"])
	}
	gotNestedLabels, ok := gotNamedDetail["labels"].(namedStringMap)
	if !ok {
		t.Fatalf("Errors()[0][labels] type = %T, want namedStringMap", gotNamedDetail["labels"])
	}
	if gotNestedLabels["code"] != "required" {
		t.Fatalf("Errors()[0][labels][code] = %#v, want required", gotNestedLabels["code"])
	}
	gotNestedNames, ok := gotNamedDetail["names"].(namedStringSlice)
	if !ok {
		t.Fatalf("Errors()[0][names] type = %T, want namedStringSlice", gotNamedDetail["names"])
	}
	if gotNestedNames[0] != "name" {
		t.Fatalf("Errors()[0][names][0] = %q, want name", gotNestedNames[0])
	}

	gotPlainLabels, ok := gotErrors[1].(map[string]string)
	if !ok {
		t.Fatalf("Errors()[1] type = %T, want map[string]string", gotErrors[1])
	}
	if gotPlainLabels["scope"] != "query" {
		t.Fatalf("Errors()[1][scope] = %#v, want query", gotPlainLabels["scope"])
	}

	gotPlainNames, ok := gotErrors[2].([]string)
	if !ok {
		t.Fatalf("Errors()[2] type = %T, want []string", gotErrors[2])
	}
	if gotPlainNames[0] != "id" {
		t.Fatalf("Errors()[2][0] = %q, want id", gotPlainNames[0])
	}
}

// 共享同一 backing array 的不同 subslice 也应各自独立克隆，不能因为起始指针相同而复用成同一个结果。
func TestNewHTTPErrorClonesDistinctSubSlicesSharingBackingArray(t *testing.T) {
	source := []string{"first", "second"}
	short := source[:1]
	long := source[:2]

	err := NewHTTPError(http.StatusBadRequest, "bad_request", "bad request", short, long)

	gotErrors := err.Errors()
	if len(gotErrors) != 2 {
		t.Fatalf("Errors() len = %d, want 2", len(gotErrors))
	}

	gotShort, ok := gotErrors[0].([]string)
	if !ok {
		t.Fatalf("Errors()[0] type = %T, want []string", gotErrors[0])
	}
	if len(gotShort) != 1 || gotShort[0] != "first" {
		t.Fatalf("Errors()[0] = %#v, want []string{\"first\"}", gotShort)
	}

	gotLong, ok := gotErrors[1].([]string)
	if !ok {
		t.Fatalf("Errors()[1] type = %T, want []string", gotErrors[1])
	}
	if len(gotLong) != 2 || gotLong[0] != "first" || gotLong[1] != "second" {
		t.Fatalf("Errors()[1] = %#v, want []string{\"first\", \"second\"}", gotLong)
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

// 各个常用错误构造器都会生成稳定的状态码和错误码。
func TestHTTPErrorConstructorsSetStatusAndCode(t *testing.T) {
	for _, tc := range standardHTTPErrorConstructors {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.build("", "", "detail")
			assertHTTPErrorStatusAndCode(t, err, tc.wantStatus, tc.wantCode)
		})
	}
}

// 各个常用错误构造器在未显式传 detail 时都会回退到状态文案。
func TestHTTPErrorConstructorsUseStatusTextPublicMessageByDefault(t *testing.T) {
	for _, tc := range standardHTTPErrorConstructors {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.build("", "", "detail")
			assertHTTPErrorUsesStatusTextPublicMessage(t, err, tc.wantStatus)
		})
	}
}

// 各个常用错误构造器都会保留传入的结构化错误项。
func TestHTTPErrorConstructorsExposeStructuredErrors(t *testing.T) {
	for _, tc := range standardHTTPErrorConstructors {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.build("", "", "detail")
			assertHTTPErrorErrors(t, err, "detail")
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

func TestNewHTTPErrorUsesPublicDefaultsForIrregularStatuses(t *testing.T) {
	testCases := []struct {
		name       string
		status     int
		wantStatus int
	}{
		{name: "success status", status: http.StatusOK, wantStatus: http.StatusInternalServerError},
		{name: "unknown 5xx", status: 509, wantStatus: http.StatusInternalServerError},
		{name: "unknown 7xx", status: 777, wantStatus: http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewHTTPError(tc.status, "", "")
			assertHTTPErrorStatusAndCode(t, err, tc.wantStatus, "internal_error")
			assertHTTPErrorUsesStatusTextPublicMessage(t, err, http.StatusInternalServerError)
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

// 快捷构造器传入自定义 code/detail 时，应透传而非丢弃。
func TestHTTPErrorConstructorWithCustomCodeAndDetail(t *testing.T) {
	err := BadRequest("custom_code", "custom detail", "field error")

	if got := err.Code(); got != "custom_code" {
		t.Fatalf("Code() = %q, want custom_code", got)
	}
	if got := err.Detail(); got != "custom detail" {
		t.Fatalf("Detail() = %q, want %q", got, "custom detail")
	}
}

// 构造时不传 errors，Errors() 应返回 nil 而非空切片。
func TestHTTPErrorErrorsReturnsNilWhenNoErrors(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, "bad_request", "bad request")

	if got := err.Errors(); got != nil {
		t.Fatalf("Errors() = %#v, want nil", got)
	}
}
