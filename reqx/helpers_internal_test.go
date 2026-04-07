package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] 内部 body 辅助会处理空 body、读取失败、Content-Type 校验、未知字段与 JSON 解码错误映射。
// - [✓] 内部 path/query/header 值绑定辅助会处理标签解析、解码计划缓存、未知字段、重复值与不支持字段类型。
// - [✓] 内部目标处理与校验辅助会维持深拷贝隔离，并对非法目标、非法注册和不支持来源给出稳定错误或 panic。
// - [✓] 本文件只覆盖关键内部辅助契约，不把这些断言记成独立公开能力验收。

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
)

type failingReadCloser struct {
	err error
}

func (r failingReadCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r failingReadCloser) Close() error {
	return nil
}

type fakeFieldLevel struct {
	field reflect.Value
}

func (f fakeFieldLevel) Top() reflect.Value      { return reflect.Value{} }
func (f fakeFieldLevel) Parent() reflect.Value   { return reflect.Value{} }
func (f fakeFieldLevel) Field() reflect.Value    { return f.field }
func (f fakeFieldLevel) FieldName() string       { return "" }
func (f fakeFieldLevel) StructFieldName() string { return "" }
func (f fakeFieldLevel) Param() string           { return "" }
func (f fakeFieldLevel) GetTag() string          { return "" }
func (f fakeFieldLevel) ExtractType(v reflect.Value) (reflect.Value, reflect.Kind, bool) {
	return v, v.Kind(), false
}
func (f fakeFieldLevel) GetStructFieldOK() (reflect.Value, reflect.Kind, bool) {
	return reflect.Value{}, reflect.Invalid, false
}
func (f fakeFieldLevel) GetStructFieldOKAdvanced(reflect.Value, string) (reflect.Value, reflect.Kind, bool) {
	return reflect.Value{}, reflect.Invalid, false
}
func (f fakeFieldLevel) GetStructFieldOK2() (reflect.Value, reflect.Kind, bool, bool) {
	return reflect.Value{}, reflect.Invalid, false, false
}
func (f fakeFieldLevel) GetStructFieldOKAdvanced2(reflect.Value, string) (reflect.Value, reflect.Kind, bool, bool) {
	return reflect.Value{}, reflect.Invalid, false, false
}

type fakeFieldError struct {
	tag        string
	actualTag  string
	namespace  string
	structNS   string
	field      string
	structName string
	value      any
	param      string
	typ        reflect.Type
}

func (f fakeFieldError) Tag() string             { return f.tag }
func (f fakeFieldError) ActualTag() string       { return f.actualTag }
func (f fakeFieldError) Namespace() string       { return f.namespace }
func (f fakeFieldError) StructNamespace() string { return f.structNS }
func (f fakeFieldError) Field() string           { return f.field }
func (f fakeFieldError) StructField() string     { return f.structName }
func (f fakeFieldError) Value() interface{}      { return f.value }
func (f fakeFieldError) Param() string           { return f.param }
func (f fakeFieldError) Kind() reflect.Kind {
	if f.typ == nil {
		return reflect.Invalid
	}
	return f.typ.Kind()
}
func (f fakeFieldError) Type() reflect.Type             { return f.typ }
func (f fakeFieldError) Translate(ut.Translator) string { return f.Error() }
func (f fakeFieldError) Error() string                  { return "fake field error" }

// Bind、BindBody 和底层值绑定都会拒绝空目标对象。
func TestBindAndBindBodyRejectNilDestination(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	var dst struct{}
	if err := Bind[struct{}](nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("Bind(nil) error = %v, want request must not be nil", err)
	}

	if err := Bind[struct{}](req, nil); err == nil || err.Error() != "reqx: destination must not be nil" {
		t.Fatalf("Bind() error = %v, want destination must not be nil", err)
	}

	req = newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)
	if err := BindBody[struct{}](req, nil); err == nil || err.Error() != "reqx: destination must not be nil" {
		t.Fatalf("BindBody() error = %v, want destination must not be nil", err)
	}
}

// 非结构体目标不允许参与标签值绑定。
func TestBindTaggedValuesRejectsNonStructDestination(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	value := 1

	err := bindTaggedValues(req, &value, querySource, bindValuesConfig{})
	if err == nil || err.Error() != "reqx: destination must point to a struct" {
		t.Fatalf("bindTaggedValues() error = %v, want destination must point to a struct", err)
	}
}

// readBody 会覆盖空 body 和底层读取失败分支。
func TestReadBodyBranches(t *testing.T) {
	data, err := readBody(nil, 10)
	if err != nil || data != nil {
		t.Fatalf("readBody(nil) = (%v, %v), want (nil, nil)", data, err)
	}

	wantErr := errors.New("read failed")
	_, err = readBody(failingReadCloser{err: wantErr}, 10)
	if !errors.Is(err, wantErr) {
		t.Fatalf("readBody() error = %v, want %v", err, wantErr)
	}
}

// headerValues 会忽略规范化后为空的 header key，并保留合法键值。
func TestHeaderValuesSkipsBlankHeaderKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header = http.Header{
		" ":      {"ignored"},
		"x-name": {"kanata"},
	}

	values := headerValues(req)
	if got := values.Get("X-Name"); got != "kanata" {
		t.Fatalf("X-Name = %q, want kanata", got)
	}
	if _, exists := values[""]; exists {
		t.Fatalf("blank header key unexpectedly present: %#v", values[""])
	}
}

// deepCloneValue 会覆盖 invalid、指针、接口、切片、数组、map 和 nil 容器分支。
func TestDeepCloneValueBranches(t *testing.T) {
	t.Run("invalid source", func(t *testing.T) {
		dst := reflect.New(reflect.TypeOf(0)).Elem()
		deepCloneValue(dst, reflect.Value{})
		if got := dst.Int(); got != 0 {
			t.Fatalf("dst = %d, want 0", got)
		}
	})

	t.Run("nested containers", func(t *testing.T) {
		type inner struct {
			Value int
		}
		type sample struct {
			Ptr      *inner
			Any      any
			Slice    []int
			Array    [2]*inner
			Map      map[string][]int
			NilPtr   *inner
			NilAny   any
			NilSlice []int
			NilMap   map[string]int
		}

		src := sample{
			Ptr:   &inner{Value: 1},
			Any:   []int{2, 3},
			Slice: []int{4, 5},
			Array: [2]*inner{&inner{Value: 6}, &inner{Value: 7}},
			Map: map[string][]int{
				"k": []int{8, 9},
			},
		}

		cloned := cloneBindingTarget(&src)

		src.Ptr.Value = 10
		src.Any.([]int)[0] = 11
		src.Slice[0] = 12
		src.Array[0].Value = 13
		src.Map["k"][0] = 14

		if cloned.Ptr == src.Ptr || cloned.Ptr.Value != 1 {
			t.Fatalf("cloned.Ptr = %#v, want independent copy", cloned.Ptr)
		}
		if got := cloned.Any.([]int)[0]; got != 2 {
			t.Fatalf("cloned.Any[0] = %d, want 2", got)
		}
		if got := cloned.Slice[0]; got != 4 {
			t.Fatalf("cloned.Slice[0] = %d, want 4", got)
		}
		if cloned.Array[0] == src.Array[0] || cloned.Array[0].Value != 6 {
			t.Fatalf("cloned.Array[0] = %#v, want independent copy", cloned.Array[0])
		}
		if got := cloned.Map["k"][0]; got != 8 {
			t.Fatalf("cloned.Map[\"k\"][0] = %d, want 8", got)
		}
		if cloned.NilPtr != nil || cloned.NilAny != nil || cloned.NilSlice != nil || cloned.NilMap != nil {
			t.Fatalf("nil containers = %#v, want preserved nils", cloned)
		}
	})

	t.Run("nan map key preserves value", func(t *testing.T) {
		src := map[float64]string{
			math.NaN(): "present-in-source",
		}

		var dst map[float64]string
		deepCloneValue(reflect.ValueOf(&dst).Elem(), reflect.ValueOf(src))

		if len(dst) != 1 {
			t.Fatalf("len(dst) = %d, want 1", len(dst))
		}
		for _, value := range dst {
			if value != "present-in-source" {
				t.Fatalf("map value = %q, want present-in-source", value)
			}
		}
	})
}

// 空 body 且不允许为空时返回空 body 错误。
func TestBindJSONWithConfigRejectsEmptyBodyWhenNotAllowed(t *testing.T) {
	req := newJSONRequest(http.MethodPost, "/", "")

	var dst struct {
		Name string `json:"name"`
	}
	err := bindJSONWithConfig(req, &dst, bindBodyConfig{}, bodyBindMode{})
	_ = assertHTTPError(t, err, http.StatusBadRequest, CodeInvalidJSON, "request body must not be empty")
}

// 空 body 场景下也可以按配置校验 Content-Type。
func TestBindJSONWithConfigRejectsInvalidContentTypeOnEmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.Header.Set("Content-Type", "text/plain")

	var dst struct {
		Name string `json:"name"`
	}
	err := bindJSONWithConfig(req, &dst, bindBodyConfig{allowEmptyBody: true}, bodyBindMode{
		validateContentTypeOnEmpty: true,
	})
	_ = assertHTTPError(t, err, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "Content-Type must be application/json or application/*+json")
}

// body 读取失败时直接透传底层错误。
func TestBindJSONWithConfigPropagatesReadError(t *testing.T) {
	wantErr := errors.New("read failed")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = failingReadCloser{err: wantErr}

	var dst struct {
		Name string `json:"name"`
	}
	err := bindJSONWithConfig(req, &dst, bindBodyConfig{}, bodyBindMode{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("bindJSONWithConfig() error = %v, want %v", err, wantErr)
	}
}

// 禁止未知字段时会返回未知字段 violation。
func TestBindJSONWithConfigRejectsUnknownFieldWhenDisabled(t *testing.T) {
	req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","extra":1}`)

	var dst struct {
		Name string `json:"name"`
	}
	err := bindJSONWithConfig(req, &dst, bindBodyConfig{allowUnknownFields: false}, bodyBindMode{})
	violation := assertSingleViolation(t, err)
	if violation.Field != "extra" || violation.Code != ViolationCodeUnknown || violation.Detail != "unknown field" {
		t.Fatalf("violation = %#v", violation)
	}
}

// validateJSONContentType 会接受空值、标准 JSON 和 +json 后缀，并正确裁剪首尾空白。
func TestValidateJSONContentType_AllowsSupportedMediaTypes(t *testing.T) {
	testCases := []struct {
		name        string
		contentType string
	}{
		{name: "empty", contentType: ""},
		{name: "application json", contentType: "application/json"},
		{name: "parameterized application json", contentType: "application/json; charset=utf-8"},
		{name: "json suffix", contentType: "application/merge-patch+json"},
		{name: "trimmed json suffix", contentType: " application/problem+json ; charset=utf-8 "},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateJSONContentType(tc.contentType); err != nil {
				t.Fatalf("validateJSONContentType() error = %v", err)
			}
		})
	}
}

// mapDecodeError 会覆盖语法、类型、EOF、未知字段等分支。
func TestMapDecodeErrorBranches(t *testing.T) {
	t.Run("syntax", func(t *testing.T) {
		err := mapDecodeError(&json.SyntaxError{})
		_ = assertHTTPError(t, err, http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
	})

	t.Run("type", func(t *testing.T) {
		err := mapDecodeError(&json.UnmarshalTypeError{
			Field: "age",
			Type:  reflect.TypeOf(0),
		})
		violation := assertSingleViolation(t, err)
		if violation.Field != "age" || violation.Code != ViolationCodeType || violation.Detail != "must be number" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("invalid unmarshal", func(t *testing.T) {
		wantErr := &json.InvalidUnmarshalError{Type: reflect.TypeOf(0)}
		if got := mapDecodeError(wantErr); got != wantErr {
			t.Fatalf("mapDecodeError() = %v, want same error", got)
		}
	})

	t.Run("eof", func(t *testing.T) {
		err := mapDecodeError(io.EOF)
		_ = assertHTTPError(t, err, http.StatusBadRequest, CodeInvalidJSON, "request body must not be empty")
	})

	t.Run("unknown field", func(t *testing.T) {
		err := mapDecodeError(errors.New(`json: unknown field "extra"`))
		violation := assertSingleViolation(t, err)
		if violation.Field != "extra" || violation.Code != ViolationCodeUnknown || violation.Detail != "unknown field" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("malformed unknown field message", func(t *testing.T) {
		err := mapDecodeError(errors.New(`json: unknown field "extra`))
		_ = assertHTTPError(t, err, http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
	})

	t.Run("default", func(t *testing.T) {
		err := mapDecodeError(errors.New("boom"))
		_ = assertHTTPError(t, err, http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
	})
}

// parseUnknownField 只解析标准未知字段错误消息。
func TestParseUnknownField(t *testing.T) {
	if field, ok := parseUnknownField(errors.New(`json: unknown field "extra"`)); !ok || field != "extra" {
		t.Fatalf("parseUnknownField() = (%q, %v), want (extra, true)", field, ok)
	}
	if field, ok := parseUnknownField(errors.New("boom")); ok || field != "" {
		t.Fatalf("parseUnknownField() = (%q, %v), want empty false", field, ok)
	}
}

// describeJSONType 会把 Go 类型映射为对外错误描述。
func TestDescribeJSONType(t *testing.T) {
	testCases := []struct {
		name string
		typ  reflect.Type
		want string
	}{
		{name: "nil", typ: nil, want: "valid value"},
		{name: "bool", typ: reflect.TypeOf(true), want: "boolean"},
		{name: "int", typ: reflect.TypeOf(1), want: "number"},
		{name: "pointer int", typ: reflect.TypeOf(new(int)), want: "number"},
		{name: "uint", typ: reflect.TypeOf(uint(1)), want: "number"},
		{name: "float", typ: reflect.TypeOf(1.5), want: "number"},
		{name: "string", typ: reflect.TypeOf(""), want: "string"},
		{name: "array", typ: reflect.TypeOf([2]string{}), want: "array"},
		{name: "slice", typ: reflect.TypeOf([]string{}), want: "array"},
		{name: "map", typ: reflect.TypeOf(map[string]string{}), want: "object"},
		{name: "struct", typ: reflect.TypeOf(struct{}{}), want: "object"},
		{name: "default", typ: reflect.TypeOf(complex64(1)), want: "complex64"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeJSONType(tc.typ); got != tc.want {
				t.Fatalf("describeJSONType() = %q, want %q", got, tc.want)
			}
		})
	}
}

// emptyBodyError 会生成标准空 body HTTP 错误。
func TestEmptyBodyError(t *testing.T) {
	err := emptyBodyError()
	_ = assertHTTPError(t, err, http.StatusBadRequest, CodeInvalidJSON, "request body must not be empty")
}

// pathValuesForPlan 在空请求、空计划或无匹配 pattern 时返回空结果。
func TestPathValuesForPlanBranches(t *testing.T) {
	plan := &valueDecodePlan{
		fields: []valueFieldSpec{{name: "id"}},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := pathValuesForPlan(nil, plan); len(got) != 0 {
		t.Fatalf("pathValuesForPlan(nil, plan) = %#v, want empty", got)
	}
	if got := pathValuesForPlan(req, nil); len(got) != 0 {
		t.Fatalf("pathValuesForPlan(req, nil) = %#v, want empty", got)
	}
	if got := pathValuesForPlan(req, plan); len(got) != 0 {
		t.Fatalf("pathValuesForPlan(no pattern) = %#v, want empty", got)
	}
}

func TestPathWildcardHelpers(t *testing.T) {
	names := pathWildcardNames("GET /users/{user_id}/files/{path...}/{$}")
	if !reflect.DeepEqual(names, []string{"user_id", "path"}) {
		t.Fatalf("pathWildcardNames() = %#v", names)
	}
	if got := pathWildcardNames("/users/{user_id"); len(got) != 0 {
		t.Fatalf("pathWildcardNames(invalid pattern) = %#v, want empty", got)
	}

	if !pathWildcardExists("/users/{user_id}", "user_id") {
		t.Fatal("pathWildcardExists() = false, want true")
	}
	if pathWildcardExists("/users/{user_id}", "account_id") {
		t.Fatal("pathWildcardExists() = true, want false")
	}
	if pathWildcardExists("/users/{user_id}", "   ") {
		t.Fatal("pathWildcardExists(blank name) = true, want false")
	}
}

func TestPathParamValues(t *testing.T) {
	if got, ok := pathParamValues(nil, "id"); ok || got != nil {
		t.Fatalf("pathParamValues(nil) = (%#v, %v), want (nil, false)", got, ok)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", " 42 ")
	if got, ok := pathParamValues(req, "id"); !ok || !reflect.DeepEqual(got, []string{"42"}) {
		t.Fatalf("pathParamValues(non-empty) = (%#v, %v), want ([\"42\"], true)", got, ok)
	}
}

// 单个空白 path 参数值会被视为缺失。
func TestRequiredPathParamValuesRejectsSingleEmptyValue(t *testing.T) {
	req := requestWithPathParams(map[string][]string{
		"id": {"   "},
	})

	_, err := requiredPathParamValues(req, "id")
	violation := assertSingleViolation(t, err)
	if violation.Field != "id" || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
		t.Fatalf("violation = %#v", violation)
	}
}

// 解码计划会写入并复用缓存。
func TestLoadValueDecodePlanUsesCache(t *testing.T) {
	type request struct {
		Page int `query:"page"`
	}

	plan1, err := loadValueDecodePlan(reflect.TypeOf(request{}), querySource)
	if err != nil {
		t.Fatalf("loadValueDecodePlan() error = %v", err)
	}
	plan2, err := loadValueDecodePlan(reflect.TypeOf(request{}), querySource)
	if err != nil {
		t.Fatalf("loadValueDecodePlan() second error = %v", err)
	}
	if plan1 != plan2 {
		t.Fatal("loadValueDecodePlan() did not reuse cached plan")
	}
}

// 字段标签解析会裁剪空白并忽略跳过标签。
func TestValueFieldNameAndTagValue(t *testing.T) {
	type request struct {
		Page int `query:" page ,omitempty "`
		Skip int `query:"-"`
	}

	pageField, _ := reflect.TypeOf(request{}).FieldByName("Page")
	skipField, _ := reflect.TypeOf(request{}).FieldByName("Skip")

	if name, ok := valueFieldName(pageField, querySource); !ok || name != "page" {
		t.Fatalf("valueFieldName(page) = (%q, %v), want (page, true)", name, ok)
	}
	if name, ok := valueFieldName(skipField, querySource); ok || name != "" {
		t.Fatalf("valueFieldName(skip) = (%q, %v), want empty false", name, ok)
	}

	if got := tagValue(pageField, "query"); got != "page" {
		t.Fatalf("tagValue() = %q, want page", got)
	}
	if got := tagValue(skipField, "query"); got != "" {
		t.Fatalf("tagValue(skip) = %q, want empty", got)
	}
}

// 字段类型校验和 TextUnmarshaler 识别覆盖常见分支。
func TestValidateValueFieldTypeAndSupportsTextUnmarshaler(t *testing.T) {
	if err := validateValueFieldType(reflect.TypeOf(new(int)), "Age", "query"); err != nil {
		t.Fatalf("validateValueFieldType(pointer) error = %v", err)
	}
	if err := validateValueFieldType(reflect.TypeOf([]uint{}), "IDs", "query"); err != nil {
		t.Fatalf("validateValueFieldType(slice) error = %v", err)
	}
	if !supportsTextUnmarshaler(reflect.TypeOf(queryState(""))) {
		t.Fatal("supportsTextUnmarshaler(queryState) = false, want true")
	}
	if !supportsTextUnmarshaler(reflect.TypeOf((*queryState)(nil))) {
		t.Fatal("supportsTextUnmarshaler(*queryState) = false, want true")
	}
	if supportsTextUnmarshaler(reflect.TypeOf("")) {
		t.Fatal("supportsTextUnmarshaler(string) = true, want false")
	}
	if supportsTextUnmarshaler(reflect.TypeOf((*string)(nil))) {
		t.Fatal("supportsTextUnmarshaler(*string) = true, want false")
	}
}

// 底层 query 解码会覆盖未知字段、空值、切片和指针分支。
func TestDecodeValuesIntoAndDecodeQueryFieldBranches(t *testing.T) {
	type request struct {
		Page int `query:"page"`
	}

	dst := reflect.ValueOf(&request{}).Elem()
	plan := &valueDecodePlan{
		fields:      []valueFieldSpec{{index: 0, name: "page"}},
		knownFields: map[string]struct{}{"page": {}},
	}

	violations, err := decodeValuesInto(dst, url.Values{"extra": {"1"}}, plan, bindValuesConfig{})
	if err != nil {
		t.Fatalf("decodeValuesInto() error = %v", err)
	}
	if len(violations) != 1 || violations[0].Field != "extra" || violations[0].Code != ViolationCodeUnknown {
		t.Fatalf("violations = %#v", violations)
	}

	violations, err = decodeValuesInto(dst, url.Values{"extra": {"1"}}, plan, bindValuesConfig{allowUnknownFields: true})
	if err != nil {
		t.Fatalf("decodeValuesInto(allow unknown) error = %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want empty", violations)
	}

	var value string
	violation, err := decodeQueryField(reflect.ValueOf(&value).Elem(), nil, "name")
	if err != nil || violation != nil {
		t.Fatalf("decodeQueryField(empty) = (%v, %v), want (nil, nil)", violation, err)
	}

	var state queryState
	violation, err = decodeQueryField(reflect.ValueOf(&state).Elem(), []string{"active", "disabled"}, "state")
	if err != nil {
		t.Fatalf("decodeQueryField(text repeated) error = %v", err)
	}
	if violation == nil || violation.Code != ViolationCodeMultiple {
		t.Fatalf("violation = %#v, want multiple", violation)
	}

	var ids []int
	violation, err = decodeQueryField(reflect.ValueOf(&ids).Elem(), []string{"1", "oops"}, "id")
	if err != nil {
		t.Fatalf("decodeQueryField(slice item) error = %v", err)
	}
	if violation == nil || violation.Field != "id" || violation.Code != ViolationCodeType {
		t.Fatalf("violation = %#v, want type violation for id", violation)
	}

	var page *int
	violation, err = decodeQueryField(reflect.ValueOf(&page).Elem(), []string{"oops"}, "page")
	if err != nil {
		t.Fatalf("decodeQueryField(pointer invalid) error = %v", err)
	}
	if violation == nil || violation.Field != "page" || violation.Code != ViolationCodeType {
		t.Fatalf("violation = %#v, want type violation for page", violation)
	}
}

// sourceValues 对不支持的值来源会 panic，避免默默回退成错误输入源。
func TestSourceValues_PanicsOnUnsupportedSource(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("sourceValues() did not panic")
		}
	}()

	_ = sourceValues(httptest.NewRequest(http.MethodGet, "/", nil), valueSource{tag: "unsupported"}, nil)
}

// 解码到不支持的字段类型时，decodeValuesInto 会返回底层错误。
func TestDecodeValuesIntoReturnsDecodeError(t *testing.T) {
	type request struct {
		Data struct{}
	}

	dst := reflect.ValueOf(&request{}).Elem()
	plan := &valueDecodePlan{
		fields:      []valueFieldSpec{{index: 0, name: "data"}},
		knownFields: map[string]struct{}{"data": {}},
	}

	violations, err := decodeValuesInto(dst, url.Values{"data": {"x"}}, plan, bindValuesConfig{})
	if err == nil {
		t.Fatal("decodeValuesInto() error = nil")
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want empty", violations)
	}
	if got := err.Error(); !strings.Contains(got, "unsupported destination type") {
		t.Fatalf("error = %q, want unsupported destination type", got)
	}
}

// 计划命中缓存后，如果解码阶段失败，bindTaggedValues 会直接返回错误。
func TestBindTaggedValuesReturnsDecodeErrorFromCachedPlan(t *testing.T) {
	type request struct {
		Data struct{} `query:"data"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?data=x", nil)
	var dst request

	var cache sync.Map
	cache.Store(reflect.TypeOf(request{}), valueDecodePlanResult{
		plan: &valueDecodePlan{
			fields:      []valueFieldSpec{{index: 0, name: "data"}},
			knownFields: map[string]struct{}{"data": {}},
		},
	})

	err := bindTaggedValues(req, &dst, valueSource{
		name:  "query",
		tag:   querySource.tag,
		cache: &cache,
	}, bindValuesConfig{})
	if err == nil {
		t.Fatal("bindTaggedValues() error = nil")
	}
	if got := err.Error(); !strings.Contains(got, "unsupported destination type") {
		t.Fatalf("error = %q, want unsupported destination type", got)
	}
}

// 标量解码会覆盖布尔、无符号整数、浮点数和不支持类型分支。
func TestDecodeScalarValueBranches(t *testing.T) {
	var boolValue bool
	if violation, err := decodeScalarValue(reflect.ValueOf(&boolValue).Elem(), "maybe", "enabled"); err != nil {
		t.Fatalf("decodeScalarValue(invalid bool) error = %v", err)
	} else if violation == nil || violation.Field != "enabled" || violation.Code != ViolationCodeType {
		t.Fatalf("decodeScalarValue(invalid bool) violation = %#v", violation)
	}

	var uintValue uint
	if violation, err := decodeScalarValue(reflect.ValueOf(&uintValue).Elem(), "42", "id"); err != nil || violation != nil || uintValue != 42 {
		t.Fatalf("decodeScalarValue(uint) = (%v, %v), value=%d", violation, err, uintValue)
	}

	if violation, err := decodeScalarValue(reflect.ValueOf(&uintValue).Elem(), "-1", "id"); err != nil {
		t.Fatalf("decodeScalarValue(invalid uint) error = %v", err)
	} else if violation == nil || violation.Field != "id" || violation.Code != ViolationCodeType {
		t.Fatalf("decodeScalarValue(invalid uint) violation = %#v", violation)
	}

	var score float64
	if violation, err := decodeScalarValue(reflect.ValueOf(&score).Elem(), "oops", "score"); err != nil {
		t.Fatalf("decodeScalarValue(invalid float) error = %v", err)
	} else if violation == nil || violation.Field != "score" || violation.Code != ViolationCodeType {
		t.Fatalf("decodeScalarValue(invalid float) violation = %#v", violation)
	}

	var unsupported struct{}
	_, err := decodeScalarValue(reflect.ValueOf(&unsupported).Elem(), "x", "field")
	if err == nil || !strings.Contains(err.Error(), "unsupported destination type") {
		t.Fatalf("decodeScalarValue(unsupported) error = %v", err)
	}
}

// nospace 校验只接受不含空格的字符串。
func TestValidateNoSpace(t *testing.T) {
	if !validateNoSpace(fakeFieldLevel{field: reflect.ValueOf("kanata")}) {
		t.Fatal("validateNoSpace(string without space) = false, want true")
	}
	if validateNoSpace(fakeFieldLevel{field: reflect.ValueOf("kana ta")}) {
		t.Fatal("validateNoSpace(string with space) = true, want false")
	}
	if validateNoSpace(fakeFieldLevel{field: reflect.ValueOf(1)}) {
		t.Fatal("validateNoSpace(non-string) = true, want false")
	}
}

// validateTarget 和 errorsf 会返回标准化错误信息。
func TestValidateTargetAndErrorsf(t *testing.T) {
	if err := validateTarget(nil); err == nil || err.Error() != "reqx: target must not be nil" {
		t.Fatalf("validateTarget(nil) error = %v", err)
	}

	var nilStruct *struct{}
	if err := validateTarget(nilStruct); err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
		t.Fatalf("validateTarget(nil struct ptr) error = %v", err)
	}
	if err := validateTarget(struct{}{}); err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
		t.Fatalf("validateTarget(non-pointer) error = %v", err)
	}
	if err := errorsf("boom %d", 1); err.Error() != "reqx: boom 1" {
		t.Fatalf("errorsf() = %v", err)
	}
}

// validateStruct 在非法输入上返回 InvalidValidationError。
func TestValidateStructInvalidValidationError(t *testing.T) {
	_, err := validateStruct(1, sourceBody)
	var invalidValidationErr *validator.InvalidValidationError
	if !errors.As(err, &invalidValidationErr) {
		t.Fatalf("validateStruct() error = %T, want *validator.InvalidValidationError", err)
	}
}

// 注册非法校验器名称时会触发 panic。
func TestMustRegisterValidationPanicsOnInvalidRegistration(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("mustRegisterValidation() did not panic")
		}
	}()

	mustRegisterValidation(validator.New(), "", validateNoSpace)
}

// 校验辅助函数会处理去重、排序和字段路径回退。
func TestValidationHelpers(t *testing.T) {
	if got := violationsFromValidation(sourceBody, nil, nil); got != nil {
		t.Fatalf("violationsFromValidation(nil) = %#v, want nil", got)
	}

	errs := validator.ValidationErrors{
		fakeFieldError{tag: "required", namespace: "Req.z", field: "z", typ: reflect.TypeOf("")},
		fakeFieldError{tag: "min", namespace: "Req.a", field: "a", typ: reflect.TypeOf("")},
		fakeFieldError{tag: "required", namespace: "Req.a", field: "a", typ: reflect.TypeOf("")},
	}
	violations := violationsFromValidation(sourceRequest, nil, errs)
	if len(violations) != 2 {
		t.Fatalf("violations len = %d, want 2", len(violations))
	}
	if violations[0].Field != "a" || violations[0].Code != ViolationCodeInvalid {
		t.Fatalf("violations[0] = %#v", violations[0])
	}
	if violations[1].Field != "z" || violations[1].Code != ViolationCodeRequired {
		t.Fatalf("violations[1] = %#v", violations[1])
	}

	if got := validationFieldPath(sourceBody, fakeFieldError{namespace: " Req.body.name ", typ: reflect.TypeOf("")}); got != "body.name" {
		t.Fatalf("validationFieldPath(namespace) = %q, want body.name", got)
	}
	if got := validationFieldPath(sourceBody, fakeFieldError{field: "display_name", typ: reflect.TypeOf("")}); got != "display_name" {
		t.Fatalf("validationFieldPath(field) = %q, want display_name", got)
	}
	if got := validationFieldPath(sourceBody, fakeFieldError{}); got != "body" {
		t.Fatalf("validationFieldPath(body fallback) = %q, want body", got)
	}
	if got := validationFieldPath(sourceRequest, fakeFieldError{}); got != "request" {
		t.Fatalf("validationFieldPath(request fallback) = %q, want request", got)
	}
}

// fieldAlias 会按来源选择别名并回退到字段名。
func TestFieldAlias(t *testing.T) {
	type request struct {
		RequestID string `header:"x-request-id"`
		Name      string
	}

	requestIDField, _ := reflect.TypeOf(request{}).FieldByName("RequestID")
	nameField, _ := reflect.TypeOf(request{}).FieldByName("Name")

	if got := fieldAlias(requestIDField, sourceHeader); got != "X-Request-Id" {
		t.Fatalf("fieldAlias(header) = %q, want X-Request-Id", got)
	}
	if got := fieldAlias(nameField, sourceBody); got != "Name" {
		t.Fatalf("fieldAlias(fallback) = %q, want Name", got)
	}
}
