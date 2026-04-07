package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `BindAndValidate*` 包装器会优先返回绑定错误，并与“单源绑定 + validate”组合保持一致。
// - [✓] 内部 `validate`、`validateStruct`、`validateTarget` 会对 typed nil、非 struct 和 validator 非法输入维持稳定契约。
// - [✓] 内部 violation 规范化、字段解析、来源推断与请求标签优先级会产出稳定结果。
// - [✓] 对不支持的来源和非法注册，内部 helper 会明确 panic，而不是静默回退。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
)

// 各个 BindAndValidate 包装器会优先返回绑定阶段错误。
func TestBindAndValidateWrappersReturnBindErrors(t *testing.T) {
	var dst struct{}

	if err := BindAndValidate[struct{}](nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("BindAndValidate() error = %v", err)
	}
	if err := BindAndValidateBody[struct{}](nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("BindAndValidateBody() error = %v", err)
	}
	if err := BindAndValidateQuery[struct{}](nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("BindAndValidateQuery() error = %v", err)
	}
	if err := BindAndValidatePath[struct{}](nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("BindAndValidatePath() error = %v", err)
	}
	if err := BindAndValidateHeaders[struct{}](nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("BindAndValidateHeaders() error = %v", err)
	}
}

// 各个 BindAndValidate 包装器在正常输入下都能顺利通过。
func TestBindAndValidateWrappersSuccessPaths(t *testing.T) {
	type bodyRequest struct {
		Name string `json:"name" validate:"required"`
	}
	bodyReq := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)
	var bodyDst bodyRequest
	if err := BindAndValidateBody(bodyReq, &bodyDst); err != nil {
		t.Fatalf("BindAndValidateBody() error = %v", err)
	}

	type requestRequest struct {
		ID string `param:"id" validate:"required"`
	}
	req := requestWithPathParams(map[string][]string{"id": {"route-id"}})
	req.Method = http.MethodGet
	req.URL.RawQuery = "ignored=1"
	var requestDst requestRequest
	if err := BindAndValidate(req, &requestDst); err != nil {
		t.Fatalf("BindAndValidate() error = %v", err)
	}

	type queryRequest struct {
		Cursor string `query:"cursor" validate:"required"`
	}
	queryReq := httptest.NewRequest(http.MethodGet, "/?cursor=abc", nil)
	var queryDst queryRequest
	if err := BindAndValidateQuery(queryReq, &queryDst); err != nil {
		t.Fatalf("BindAndValidateQuery() error = %v", err)
	}

	type pathRequest struct {
		UUID string `param:"uuid" validate:"required"`
	}
	pathReq := requestWithPathParams(map[string][]string{"uuid": {"u_1"}})
	var pathDst pathRequest
	if err := BindAndValidatePath(pathReq, &pathDst); err != nil {
		t.Fatalf("BindAndValidatePath() error = %v", err)
	}

	type headerRequest struct {
		RequestID string `header:"x-request-id" validate:"required"`
	}
	headerReq := httptest.NewRequest(http.MethodGet, "/", nil)
	headerReq.Header.Set("X-Request-Id", "req-1")
	var headerDst headerRequest
	if err := BindAndValidateHeaders(headerReq, &headerDst); err != nil {
		t.Fatalf("BindAndValidateHeaders() error = %v", err)
	}
}

// 各个 BindAndValidate 包装器的结果应与“先绑定，再按对应来源校验”的组合一致。
func TestBindAndValidateWrappersMatchBindPlusValidate(t *testing.T) {
	t.Run("request success", func(t *testing.T) {
		type request struct {
			ID   string `param:"id" validate:"required,uuid"`
			Name string `query:"name" validate:"required,nospace"`
		}

		newReq := func() *http.Request {
			req := requestWithPathParams(map[string][]string{
				"id": {"550e8400-e29b-41d4-a716-446655440000"},
			})
			req.Method = http.MethodGet
			req.URL.RawQuery = "name=kanata"
			return req
		}

		var got request
		gotErr := BindAndValidate(newReq(), &got)

		var want request
		wantErr := Bind(newReq(), &want)
		if wantErr == nil {
			wantErr = validate(&want, sourceRequest)
		}

		if gotErr != nil || wantErr != nil {
			t.Fatalf("gotErr = %v, wantErr = %v", gotErr, wantErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %#v, want %#v", got, want)
		}
	})

	t.Run("request validation failure", func(t *testing.T) {
		type request struct {
			ID   string `param:"id" validate:"required"`
			Name string `query:"name" validate:"required,nospace"`
		}

		newReq := func() *http.Request {
			req := requestWithPathParams(map[string][]string{
				"id": {"route-id"},
			})
			req.Method = http.MethodGet
			req.URL.RawQuery = "name=bad%20value"
			return req
		}

		var got request
		gotErr := BindAndValidate(newReq(), &got)

		var want request
		wantErr := Bind(newReq(), &want)
		if wantErr == nil {
			wantErr = validate(&want, sourceRequest)
		}

		_ = assertSameHTTPError(t, gotErr, wantErr)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %#v, want %#v", got, want)
		}
	})

	t.Run("body success", func(t *testing.T) {
		type request struct {
			Name string `json:"name" validate:"required,nospace"`
		}

		newReq := func() *http.Request {
			return newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)
		}

		var got request
		gotErr := BindAndValidateBody(newReq(), &got)

		var want request
		wantErr := BindBody(newReq(), &want)
		if wantErr == nil {
			wantErr = validate(&want, sourceBody)
		}

		if gotErr != nil || wantErr != nil {
			t.Fatalf("gotErr = %v, wantErr = %v", gotErr, wantErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %#v, want %#v", got, want)
		}
	})

	t.Run("body validation failure", func(t *testing.T) {
		type request struct {
			Name string `json:"name" validate:"required,nospace"`
		}

		newReq := func() *http.Request {
			return newJSONRequest(http.MethodPost, "/", `{"name":"bad value"}`)
		}

		var got request
		gotErr := BindAndValidateBody(newReq(), &got)

		var want request
		wantErr := BindBody(newReq(), &want)
		if wantErr == nil {
			wantErr = validate(&want, sourceBody)
		}

		_ = assertSameHTTPError(t, gotErr, wantErr)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %#v, want %#v", got, want)
		}
	})

	t.Run("query success", func(t *testing.T) {
		type request struct {
			Cursor string `query:"cursor" validate:"required,nospace"`
		}

		newReq := func() *http.Request {
			return httptest.NewRequest(http.MethodGet, "/?cursor=abc", nil)
		}

		var got request
		gotErr := BindAndValidateQuery(newReq(), &got)

		var want request
		wantErr := BindQueryParams(newReq(), &want)
		if wantErr == nil {
			wantErr = validate(&want, sourceQuery)
		}

		if gotErr != nil || wantErr != nil {
			t.Fatalf("gotErr = %v, wantErr = %v", gotErr, wantErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %#v, want %#v", got, want)
		}
	})

	t.Run("query validation failure", func(t *testing.T) {
		type request struct {
			Cursor string `query:"cursor" validate:"required,nospace"`
		}

		newReq := func() *http.Request {
			return httptest.NewRequest(http.MethodGet, "/?cursor=bad%20value", nil)
		}

		var got request
		gotErr := BindAndValidateQuery(newReq(), &got)

		var want request
		wantErr := BindQueryParams(newReq(), &want)
		if wantErr == nil {
			wantErr = validate(&want, sourceQuery)
		}

		_ = assertSameHTTPError(t, gotErr, wantErr)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %#v, want %#v", got, want)
		}
	})

	t.Run("path success", func(t *testing.T) {
		type request struct {
			ID string `param:"id" validate:"required,nospace"`
		}

		newReq := func() *http.Request {
			return requestWithPathParams(map[string][]string{
				"id": {"route-id"},
			})
		}

		var got request
		gotErr := BindAndValidatePath(newReq(), &got)

		var want request
		wantErr := BindPathValues(newReq(), &want)
		if wantErr == nil {
			wantErr = validate(&want, sourcePath)
		}

		if gotErr != nil || wantErr != nil {
			t.Fatalf("gotErr = %v, wantErr = %v", gotErr, wantErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %#v, want %#v", got, want)
		}
	})

	t.Run("path validation failure", func(t *testing.T) {
		type request struct {
			ID string `param:"id" validate:"required,nospace"`
		}

		newReq := func() *http.Request {
			return requestWithPathParams(map[string][]string{
				"id": {"bad value"},
			})
		}

		var got request
		gotErr := BindAndValidatePath(newReq(), &got)

		var want request
		wantErr := BindPathValues(newReq(), &want)
		if wantErr == nil {
			wantErr = validate(&want, sourcePath)
		}

		_ = assertSameHTTPError(t, gotErr, wantErr)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %#v, want %#v", got, want)
		}
	})

	t.Run("header success", func(t *testing.T) {
		type request struct {
			RequestID string `header:"x-request-id" validate:"required,nospace"`
		}

		newReq := func() *http.Request {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("X-Request-Id", "req-1")
			return req
		}

		var got request
		gotErr := BindAndValidateHeaders(newReq(), &got)

		var want request
		wantErr := BindHeaders(newReq(), &want)
		if wantErr == nil {
			wantErr = validate(&want, sourceHeader)
		}

		if gotErr != nil || wantErr != nil {
			t.Fatalf("gotErr = %v, wantErr = %v", gotErr, wantErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %#v, want %#v", got, want)
		}
	})

	t.Run("header validation failure", func(t *testing.T) {
		type request struct {
			RequestID string `header:"x-request-id" validate:"required,nospace"`
		}

		newReq := func() *http.Request {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("X-Request-Id", "bad value")
			return req
		}

		var got request
		gotErr := BindAndValidateHeaders(newReq(), &got)

		var want request
		wantErr := BindHeaders(newReq(), &want)
		if wantErr == nil {
			wantErr = validate(&want, sourceHeader)
		}

		_ = assertSameHTTPError(t, gotErr, wantErr)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %#v, want %#v", got, want)
		}
	})
}

// validate 会覆盖成功、typed nil 和非结构体目标分支。
func TestValidateBranches(t *testing.T) {
	type request struct {
		Name string `validate:"required"`
	}

	if err := validate(&request{Name: "ok"}, sourceBody); err != nil {
		t.Fatalf("validate(success) error = %v", err)
	}

	var nilTarget *request
	if err := validate(nilTarget, sourceBody); err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
		t.Fatalf("validate(typed nil) error = %v", err)
	}

	value := 1
	if err := validate(&value, sourceBody); err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
		t.Fatalf("validate(non-struct) error = %v", err)
	}
}

// validateStruct 返回的校验错误会被转换为 violation 列表。
func TestValidateStructValidationErrors(t *testing.T) {
	target := &struct {
		Name string `json:"name" validate:"required"`
	}{}

	violations, err := validateStruct(target, sourceBody)
	if err != nil {
		t.Fatalf("validateStruct() error = %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("violations len = %d, want 1", len(violations))
	}
	if violations[0].Field != "name" || violations[0].In != ViolationInBody || violations[0].Code != ViolationCodeRequired || violations[0].Detail != "is required" {
		t.Fatalf("violations[0] = %#v", violations[0])
	}
}

// 直接传入 nil 接口值时返回空目标错误。
func TestValidateTargetRejectsNilTarget(t *testing.T) {
	err := validateTarget(nil)
	if err == nil {
		t.Fatal("validateTarget() error = nil")
	}
	if got := err.Error(); got != "reqx: target must not be nil" {
		t.Fatalf("error = %q", got)
	}
}

// 非法的校验目标会透传 validator 的 InvalidValidationError。
func TestValidateStructReturnsInvalidValidationError(t *testing.T) {
	_, err := validateStruct(1, sourceBody)
	if err == nil {
		t.Fatal("validateStruct() error = nil")
	}

	var invalidErr *validator.InvalidValidationError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("error = %T, want *validator.InvalidValidationError", err)
	}
}

// validator 拒绝 time.Time 时，validate 会直接透传该错误。
func TestValidateReturnsInvalidValidationError(t *testing.T) {
	now := time.Now()

	err := validate(&now, sourceBody)
	if err == nil {
		t.Fatal("validate() error = nil")
	}

	var invalidErr *validator.InvalidValidationError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("error = %T, want *validator.InvalidValidationError", err)
	}
}

// 内部校验器和标签优先级 helper 对不支持的来源会 panic。
func TestValidatorHelpers_PanicOnUnsupportedSource(t *testing.T) {
	t.Run("validatorFor", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("validatorFor() did not panic")
			}
		}()

		_ = validatorFor(sourceKind("unsupported"))
	})

	t.Run("sourceTagPriority", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("sourceTagPriority() did not panic")
			}
		}()

		_ = sourceTagPriority(sourceKind("unsupported"))
	})
}

// body 来源的标签优先级顺序固定，用于字段别名解析。
func TestSourceTagPriority_UsesBodyPriority(t *testing.T) {
	got := sourceTagPriority(sourceBody)
	want := []string{"json", "query", "param", "header"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sourceTagPriority(sourceBody) = %#v, want %#v", got, want)
	}
}

// normalizeViolation 会按错误码补齐默认错误信息。
func TestNormalizeViolationBranches(t *testing.T) {
	testCases := []struct {
		name string
		in   Violation
		want Violation
	}{
		{
			name: "required",
			in:   Violation{Field: "name", Code: ViolationCodeRequired},
			want: Violation{Field: "name", Code: ViolationCodeRequired, Detail: "is required"},
		},
		{
			name: "unknown",
			in:   Violation{Field: "name", Code: ViolationCodeUnknown},
			want: Violation{Field: "name", Code: ViolationCodeUnknown, Detail: "unknown field"},
		},
		{
			name: "type",
			in:   Violation{Field: "name", Code: ViolationCodeType},
			want: Violation{Field: "name", Code: ViolationCodeType, Detail: "has invalid type"},
		},
		{
			name: "multiple",
			in:   Violation{Field: "name", Code: ViolationCodeMultiple},
			want: Violation{Field: "name", Code: ViolationCodeMultiple, Detail: "must not be repeated"},
		},
		{
			name: "default",
			in:   Violation{Field: "name"},
			want: Violation{Field: "name", Code: ViolationCodeInvalid, Detail: "is invalid"},
		},
		{
			name: "explicit detail",
			in:   Violation{Field: "name", Code: ViolationCodeInvalid, Detail: "custom"},
			want: Violation{Field: "name", Code: ViolationCodeInvalid, Detail: "custom"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeViolation(tc.in); got != tc.want {
				t.Fatalf("normalizeViolation() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestViolationInputHelpers(t *testing.T) {
	if got := violationInForValueSource(valueSource{tag: querySource.tag}); got != ViolationInQuery {
		t.Fatalf("violationInForValueSource(query) = %q", got)
	}
	if got := violationInForValueSource(valueSource{tag: pathSource.tag}); got != ViolationInPath {
		t.Fatalf("violationInForValueSource(path) = %q", got)
	}
	if got := violationInForValueSource(valueSource{tag: headerSource.tag}); got != ViolationInHeader {
		t.Fatalf("violationInForValueSource(header) = %q", got)
	}
	if got := violationInForValueSource(valueSource{tag: "unknown"}); got != ViolationInRequest {
		t.Fatalf("violationInForValueSource(default) = %q", got)
	}

	testTags := map[string]string{
		"json":    ViolationInBody,
		"query":   ViolationInQuery,
		"param":   ViolationInPath,
		"header":  ViolationInHeader,
		"unknown": ViolationInRequest,
	}
	for tag, want := range testTags {
		if got := violationInForTag(tag); got != want {
			t.Fatalf("violationInForTag(%q) = %q, want %q", tag, got, want)
		}
	}

	testSources := map[sourceKind]string{
		sourceBody:      ViolationInBody,
		sourceQuery:     ViolationInQuery,
		sourcePath:      ViolationInPath,
		sourceHeader:    ViolationInHeader,
		sourceRequest:   ViolationInRequest,
		sourceKind("x"): ViolationInRequest,
	}
	for source, want := range testSources {
		if got := violationInForSource(source); got != want {
			t.Fatalf("violationInForSource(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestResolveValidationFieldPathAndNamespaceParsing(t *testing.T) {
	type nestedItem struct {
		Value string `header:"x-value"`
	}
	type request struct {
		Nested []*nestedItem `json:"nested"`
		Plain  string        `json:"plain"`
	}

	fields, ok := resolveValidationFieldPath(&request{}, "request.Nested[0].Value")
	if !ok {
		t.Fatal("resolveValidationFieldPath() = false, want true")
	}
	if len(fields) != 2 {
		t.Fatalf("len(fields) = %d, want 2", len(fields))
	}
	if fields[0].Name != "Nested" || fields[1].Name != "Value" {
		t.Fatalf("fields = %#v, want [Nested Value]", fields)
	}

	testCases := []struct {
		name      string
		target    any
		namespace string
	}{
		{name: "nil target", target: nil, namespace: "request.Name"},
		{name: "empty namespace", target: &request{}, namespace: ""},
		{name: "root only", target: &request{}, namespace: "request"},
		{name: "missing field", target: &request{}, namespace: "request.Missing"},
		{name: "missing intermediate field", target: &request{}, namespace: "request.Nested[0].Missing.Value"},
		{name: "non-struct intermediate", target: &request{}, namespace: "request.Plain.Value"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := resolveValidationFieldPath(tc.target, tc.namespace); ok {
				t.Fatalf("resolveValidationFieldPath(%#v, %q) = true, want false", tc.target, tc.namespace)
			}
		})
	}

	if got := parseStructNamespace(" request.Nested[0].Value "); !reflect.DeepEqual(got, []string{"Nested", "Value"}) {
		t.Fatalf("parseStructNamespace() = %#v", got)
	}
	if got := parseStructNamespace(""); got != nil {
		t.Fatalf("parseStructNamespace(empty) = %#v, want nil", got)
	}
	if got := parseStructNamespace("request"); got != nil {
		t.Fatalf("parseStructNamespace(root) = %#v, want nil", got)
	}
	if got := parseStructNamespace("request..Nested[0]..Value"); !reflect.DeepEqual(got, []string{"Nested", "Value"}) {
		t.Fatalf("parseStructNamespace(skip empty) = %#v", got)
	}
}

func TestValidationInputForRequestUsesTagPriorityAndFallback(t *testing.T) {
	type request struct {
		ID      string `param:"id" validate:"required"`
		Cursor  string `query:"cursor" validate:"required"`
		Name    string `json:"name" validate:"required"`
		TraceID string `header:"x-trace-id" validate:"required"`
		Plain   string `validate:"required"`
	}

	target := &request{}
	err := validatorFor(sourceRequest).Struct(target)
	if err == nil {
		t.Fatal("validatorFor(sourceRequest).Struct() error = nil")
	}

	validationErrs := err.(validator.ValidationErrors)
	violations := violationsFromValidation(sourceRequest, target, validationErrs)
	got := map[string]string{}
	for _, violation := range violations {
		got[violation.Field] = violation.In
	}

	want := map[string]string{
		"id":         ViolationInPath,
		"cursor":     ViolationInQuery,
		"name":       ViolationInBody,
		"X-Trace-Id": ViolationInHeader,
		"Plain":      ViolationInRequest,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request violations = %#v, want %#v", got, want)
	}

	if got := validationInput(sourceRequest, 1, validationErrs[0]); got != ViolationInRequest {
		t.Fatalf("validationInput(fallback) = %q, want request", got)
	}
}
