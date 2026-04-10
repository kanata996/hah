package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `BindAndValidate*` 包装器会优先执行 bind，并在公开返回值中暴露稳定的成功/失败语义。
// - [✓] `BindAndValidate*` 在各来源下都会返回请求 tag 字段名、规范化 header 名与稳定的 violation 包络。
// - [✓] 内部 `postBindValidate`、`validateStruct`、`validateTarget` 会维持稳定目标契约。
// - [✓] validator 初始化、字段别名、tag 优先级与 panic 分支都会产出稳定结果。
// - [✓] 嵌套 struct 校验失败时 violation 字段路径会包含完整的嵌套层级。
// - [✓] 仅实现 Normalizer 不实现 RequestValidator 的 DTO 在绑定校验时只执行 normalize 不触发请求级规则。

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/kanata996/hah/bind"
)

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
	namespace  string
	structNS   string
	field      string
	structName string
	value      any
	param      string
	typ        reflect.Type
}

func (f fakeFieldError) Tag() string             { return f.tag }
func (f fakeFieldError) ActualTag() string       { return f.tag }
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

// 各个 BindAndValidate 包装器会优先返回绑定阶段错误。
func TestBindAndValidateWrappersReturnBindErrors(t *testing.T) {
	var dst struct{}

	if err := BindAndValidate(nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("BindAndValidate() error = %v", err)
	}
	if err := BindAndValidateBody(nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("BindAndValidateBody() error = %v", err)
	}
	if err := BindAndValidateQuery(nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("BindAndValidateQuery() error = %v", err)
	}
	if err := BindAndValidatePath(nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("BindAndValidatePath() error = %v", err)
	}
	if err := BindAndValidateHeaders(nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("BindAndValidateHeaders() error = %v", err)
	}
}

func TestBindAndValidateWrappersRejectNilDestination(t *testing.T) {
	req := newJSONRequest(http.MethodPost, "/", `{}`)

	if err := BindAndValidate(req, nil); err == nil || err.Error() != "reqx: destination must not be nil" {
		t.Fatalf("BindAndValidate(nil target) error = %v", err)
	}
	if err := BindAndValidateBody(req, nil); err == nil || err.Error() != "reqx: destination must not be nil" {
		t.Fatalf("BindAndValidateBody(nil target) error = %v", err)
	}
	if err := BindAndValidateQuery(req, nil); err == nil || err.Error() != "reqx: destination must not be nil" {
		t.Fatalf("BindAndValidateQuery(nil target) error = %v", err)
	}
	if err := BindAndValidatePath(req, nil); err == nil || err.Error() != "reqx: destination must not be nil" {
		t.Fatalf("BindAndValidatePath(nil target) error = %v", err)
	}
	if err := BindAndValidateHeaders(req, nil); err == nil || err.Error() != "reqx: destination must not be nil" {
		t.Fatalf("BindAndValidateHeaders(nil target) error = %v", err)
	}
}

func TestBindAndValidateWrappersReturnBindingErrorsFromBindPackage(t *testing.T) {
	t.Run("body invalid json", func(t *testing.T) {
		type bodyRequest struct {
			Name int `json:"name"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"oops"}`)
		gotErr := BindAndValidateBody(req, &bodyRequest{})

		wantReq := newJSONRequest(http.MethodPost, "/", `{"name":"oops"}`)
		wantErr := bind.BindBody(wantReq, &bodyRequest{})

		_ = assertSameHTTPError(t, gotErr, wantErr)
		_ = assertHTTPError(t, gotErr, http.StatusBadRequest, bind.CodeInvalidJSON, "request body must be valid JSON")
	})

	t.Run("body unsupported media type", func(t *testing.T) {
		type bodyRequest struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
		req.Header.Set("Content-Type", "text/plain")
		gotErr := BindAndValidateBody(req, &bodyRequest{})

		wantReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
		wantReq.Header.Set("Content-Type", "text/plain")
		wantErr := bind.BindBody(wantReq, &bodyRequest{})

		_ = assertSameHTTPError(t, gotErr, wantErr)
		_ = assertHTTPError(t, gotErr, http.StatusUnsupportedMediaType, bind.CodeUnsupportedMediaType, "Content-Type must be application/json")
	})

	t.Run("query parse error", func(t *testing.T) {
		type requestQuery struct {
			Page int `query:"page"`
		}

		req := newJSONRequest(http.MethodGet, "/?page=oops", "")
		req.ContentLength = 0
		gotErr := BindAndValidateQuery(req, &requestQuery{})

		wantReq := newJSONRequest(http.MethodGet, "/?page=oops", "")
		wantReq.ContentLength = 0
		wantErr := bind.BindQueryParams(wantReq, &requestQuery{})

		_ = assertSameHTTPError(t, gotErr, wantErr)
		_ = assertHTTPError(t, gotErr, http.StatusBadRequest, "bad_request", "Bad Request")
	})

	t.Run("path parse error", func(t *testing.T) {
		type requestPath struct {
			ID int `param:"id"`
		}

		req := requestWithPathParams(map[string][]string{"id": {"oops"}})
		gotErr := BindAndValidatePath(req, &requestPath{})

		wantReq := requestWithPathParams(map[string][]string{"id": {"oops"}})
		wantErr := bind.BindPathValues(wantReq, &requestPath{})

		_ = assertSameHTTPError(t, gotErr, wantErr)
		_ = assertHTTPError(t, gotErr, http.StatusBadRequest, "bad_request", "Bad Request")
	})

	t.Run("header parse error", func(t *testing.T) {
		type requestHeader struct {
			Retry int `header:"x-retry"`
		}

		req := newJSONRequest(http.MethodGet, "/", "")
		req.ContentLength = 0
		req.Header.Set("X-Retry", "oops")
		gotErr := BindAndValidateHeaders(req, &requestHeader{})

		wantReq := newJSONRequest(http.MethodGet, "/", "")
		wantReq.ContentLength = 0
		wantReq.Header.Set("X-Retry", "oops")
		wantErr := bind.BindHeaders(wantReq, &requestHeader{})

		_ = assertSameHTTPError(t, gotErr, wantErr)
		_ = assertHTTPError(t, gotErr, http.StatusBadRequest, "bad_request", "Bad Request")
	})

	t.Run("mixed request bind error", func(t *testing.T) {
		type mixedRequest struct {
			ID int `param:"id"`
		}

		req := requestWithPathParams(map[string][]string{"id": {"oops"}})
		gotErr := BindAndValidate(req, &mixedRequest{})

		wantReq := requestWithPathParams(map[string][]string{"id": {"oops"}})
		wantErr := bind.Bind(wantReq, &mixedRequest{})

		_ = assertSameHTTPError(t, gotErr, wantErr)
		_ = assertHTTPError(t, gotErr, http.StatusBadRequest, "bad_request", "Bad Request")
	})
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
	if bodyDst.Name != "kanata" {
		t.Fatalf("bodyDst = %#v, want bound body field", bodyDst)
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
	if requestDst.ID != "route-id" {
		t.Fatalf("requestDst = %#v, want bound path field", requestDst)
	}

	type queryRequest struct {
		Cursor string `query:"cursor" validate:"required"`
	}
	queryReq := httptest.NewRequest(http.MethodGet, "/?cursor=abc", nil)
	var queryDst queryRequest
	if err := BindAndValidateQuery(queryReq, &queryDst); err != nil {
		t.Fatalf("BindAndValidateQuery() error = %v", err)
	}
	if queryDst.Cursor != "abc" {
		t.Fatalf("queryDst = %#v, want bound query field", queryDst)
	}

	type pathRequest struct {
		UUID string `param:"uuid" validate:"required"`
	}
	pathReq := requestWithPathParams(map[string][]string{"uuid": {"u_1"}})
	var pathDst pathRequest
	if err := BindAndValidatePath(pathReq, &pathDst); err != nil {
		t.Fatalf("BindAndValidatePath() error = %v", err)
	}
	if pathDst.UUID != "u_1" {
		t.Fatalf("pathDst = %#v, want bound path field", pathDst)
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
	if headerDst.RequestID != "req-1" {
		t.Fatalf("headerDst = %#v, want bound header field", headerDst)
	}
}

// 当 DTO 未实现 RequestValidator 时，综合绑定下的空 body 会继续沿用 binding no-op，再由字段校验处理。
func TestBindAndValidate_EmptyMixedSourceBodyDefersToValidation(t *testing.T) {
	type request struct {
		OrgID string `param:"org_id" validate:"required"`
		Name  string `json:"name" validate:"required"`
	}

	req := requestWithPathParams(map[string][]string{"org_id": {"org_1"}})
	req.Method = http.MethodPost
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = -1

	var dst request
	err := BindAndValidate(req, &dst)
	httpErr := assertHTTPError(t, err, http.StatusUnprocessableEntity, CodeInvalidRequest, "request contains invalid fields")
	if got := len(httpErr.Errors()); got != 1 {
		t.Fatalf("errors len = %d, want 1", got)
	}
	if !reflect.DeepEqual(dst, request{OrgID: "org_1"}) {
		t.Fatalf("dst = %#v, want path value preserved before validation failure", dst)
	}
}

// 各个 BindAndValidate 包装器会把各来源的字段别名和 violation 包络直接暴露给调用方。
func TestBindAndValidateWrappersReturnPublicValidationViolations(t *testing.T) {
	t.Run("request uses request aliases", func(t *testing.T) {
		type request struct {
			OrgID       string `param:"org_id" validate:"required"`
			DisplayName string `json:"display_name" validate:"required,nospace"`
		}

		req := requestWithPathParams(map[string][]string{
			"org_id": {"org_1"},
		})
		req.Method = http.MethodPost
		req.Header.Set("Content-Type", "application/json")
		req.Body = io.NopCloser(strings.NewReader(`{"display_name":"bad value"}`))
		req.ContentLength = int64(len(`{"display_name":"bad value"}`))

		var dst request
		violation := assertSingleViolation(t, BindAndValidate(req, &dst))
		want := Violation{
			Field:  "display_name",
			In:     ViolationInRequest,
			Code:   ViolationCodeInvalid,
			Detail: "is invalid",
		}
		if violation != want {
			t.Fatalf("violation = %#v, want %#v", violation, want)
		}
		if dst.OrgID != "org_1" || dst.DisplayName != "bad value" {
			t.Fatalf("dst = %#v, want bound values preserved on validation failure", dst)
		}
	})

	t.Run("body uses json tag", func(t *testing.T) {
		type request struct {
			DisplayName string `json:"display_name" validate:"required,nospace"`
		}

		var dst request
		violation := assertSingleViolation(t, BindAndValidateBody(newJSONRequest(http.MethodPost, "/", `{"display_name":"bad value"}`), &dst))
		want := Violation{
			Field:  "display_name",
			In:     ViolationInBody,
			Code:   ViolationCodeInvalid,
			Detail: "is invalid",
		}
		if violation != want {
			t.Fatalf("violation = %#v, want %#v", violation, want)
		}
		if dst.DisplayName != "bad value" {
			t.Fatalf("dst = %#v, want bound body value preserved", dst)
		}
	})

	t.Run("query uses query tag", func(t *testing.T) {
		type request struct {
			CursorToken string `query:"cursor_token" validate:"required,nospace"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?cursor_token=bad%20value", nil)
		var dst request
		violation := assertSingleViolation(t, BindAndValidateQuery(req, &dst))
		want := Violation{
			Field:  "cursor_token",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "is invalid",
		}
		if violation != want {
			t.Fatalf("violation = %#v, want %#v", violation, want)
		}
		if dst.CursorToken != "bad value" {
			t.Fatalf("dst = %#v, want bound query value preserved", dst)
		}
	})

	t.Run("path uses param tag", func(t *testing.T) {
		type request struct {
			AccountUUID string `param:"account_uuid" validate:"required,nospace"`
		}

		req := requestWithPathParams(map[string][]string{
			"account_uuid": {"bad value"},
		})
		var dst request
		violation := assertSingleViolation(t, BindAndValidatePath(req, &dst))
		want := Violation{
			Field:  "account_uuid",
			In:     ViolationInPath,
			Code:   ViolationCodeInvalid,
			Detail: "is invalid",
		}
		if violation != want {
			t.Fatalf("violation = %#v, want %#v", violation, want)
		}
		if dst.AccountUUID != "bad value" {
			t.Fatalf("dst = %#v, want bound path value preserved", dst)
		}
	})

	t.Run("headers use canonical header tag", func(t *testing.T) {
		type request struct {
			TraceID string `header:"x-trace-id" validate:"required,nospace"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Trace-Id", "bad value")

		var dst request
		violation := assertSingleViolation(t, BindAndValidateHeaders(req, &dst))
		want := Violation{
			Field:  "X-Trace-Id",
			In:     ViolationInHeader,
			Code:   ViolationCodeInvalid,
			Detail: "is invalid",
		}
		if violation != want {
			t.Fatalf("violation = %#v, want %#v", violation, want)
		}
		if dst.TraceID != "bad value" {
			t.Fatalf("dst = %#v, want bound header value preserved", dst)
		}
	})
}

func TestBindAndValidate_UsesRequestFieldAliases(t *testing.T) {
	var dst struct {
		ID      string `param:"id" validate:"required"`
		Cursor  string `query:"cursor" validate:"required"`
		Name    string `json:"name" validate:"required"`
		TraceID string `header:"x-trace-id" validate:"required"`
		Plain   string `validate:"required"`
	}

	req := requestWithPathParams(nil)
	req.Method = http.MethodGet

	violations := assertViolations(t, BindAndValidate(req, &dst))
	if len(violations) != 5 {
		t.Fatalf("violations len = %d, want 5", len(violations))
	}

	got := map[string]Violation{}
	for _, violation := range violations {
		got[violation.Field] = violation
	}

	want := map[string]string{
		"id":         ViolationInRequest,
		"cursor":     ViolationInRequest,
		"name":       ViolationInRequest,
		"X-Trace-Id": ViolationInRequest,
		"Plain":      ViolationInRequest,
	}
	for field, wantIn := range want {
		violation, ok := got[field]
		if !ok {
			t.Fatalf("missing violation for %q in %#v", field, got)
		}
		if violation.In != wantIn || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation[%q] = %#v", field, violation)
		}
	}
}

func TestPostBindValidateRejectsInvalidTarget(t *testing.T) {
	if err := postBindValidate(newJSONRequest(http.MethodPost, "/", `{}`), 1, sourceBody); err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
		t.Fatalf("postBindValidate(non-struct) error = %v", err)
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

// validateFields 透传 validateStruct 的 InvalidValidationError。
func TestValidateFieldsReturnsInvalidValidationError(t *testing.T) {
	err := validateFields(1, sourceBody)
	if err == nil {
		t.Fatal("validateFields() error = nil")
	}

	var invalidErr *validator.InvalidValidationError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("error = %T, want *validator.InvalidValidationError", err)
	}
}

func TestReqxValidationHelperBranches(t *testing.T) {
	if !validateNoSpace(fakeFieldLevel{field: reflect.ValueOf("kanata")}) {
		t.Fatal("validateNoSpace(string without space) = false, want true")
	}
	if validateNoSpace(fakeFieldLevel{field: reflect.ValueOf("kana ta")}) {
		t.Fatal("validateNoSpace(string with space) = true, want false")
	}
	if validateNoSpace(fakeFieldLevel{field: reflect.ValueOf(1)}) {
		t.Fatal("validateNoSpace(non-string) = true, want false")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("mustRegisterValidation() did not panic")
		}
	}()
	mustRegisterValidation(validator.New(), "", validateNoSpace)
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

func TestViolationsFromValidationAndFieldPathBranches(t *testing.T) {
	if got := violationsFromValidation(sourceBody, nil); got != nil {
		t.Fatalf("violationsFromValidation(nil) = %#v, want nil", got)
	}

	errs := validator.ValidationErrors{
		fakeFieldError{tag: "required", namespace: "Req.z", field: "z", typ: reflect.TypeOf("")},
		fakeFieldError{tag: "min", namespace: "Req.a", field: "a", typ: reflect.TypeOf("")},
		fakeFieldError{tag: "required", namespace: "Req.a", field: "a", typ: reflect.TypeOf("")},
	}
	violations := violationsFromValidation(sourceRequest, errs)
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

func TestViolationInputHelpers(t *testing.T) {

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

func TestTagValueAdditionalBranches(t *testing.T) {
	type request struct {
		NoTag    string
		BlankTag string `json:"   "`
		SkipTag  string `json:"-"`
	}

	noTagField, _ := reflect.TypeOf(request{}).FieldByName("NoTag")
	blankTagField, _ := reflect.TypeOf(request{}).FieldByName("BlankTag")
	skipTagField, _ := reflect.TypeOf(request{}).FieldByName("SkipTag")

	if got := tagValue(noTagField, "json"); got != "" {
		t.Fatalf("tagValue(no tag) = %q, want empty", got)
	}
	if got := tagValue(blankTagField, "json"); got != "" {
		t.Fatalf("tagValue(blank tag) = %q, want empty", got)
	}
	if got := tagValue(skipTagField, "json"); got != "" {
		t.Fatalf("tagValue(skip tag) = %q, want empty", got)
	}
}

// 嵌套 struct 校验失败时，violation 字段路径应包含完整嵌套层级。
func TestBindAndValidateBody_NestedStructViolationFieldPath(t *testing.T) {
	type address struct {
		City string `json:"city" validate:"required"`
	}
	type request struct {
		Item struct {
			Addr address `json:"addr"`
		} `json:"item"`
	}

	req := newJSONRequest(http.MethodPost, "/", `{"item":{"addr":{}}}`)
	var dst request
	violation := assertSingleViolation(t, BindAndValidateBody(req, &dst))
	want := Violation{
		Field:  "item.addr.city",
		In:     ViolationInBody,
		Code:   ViolationCodeRequired,
		Detail: "is required",
	}
	if violation != want {
		t.Fatalf("violation = %#v, want %#v", violation, want)
	}
}

// 仅实现 Normalizer 不实现 RequestValidator 的 DTO 只触发 normalize，不触发请求级规则。
func TestBindAndValidateBody_NormalizerOnlyWithoutRequestValidator(t *testing.T) {
	var events []string
	dst := normalizerOnlyRequest{events: &events, Name: ""}
	req := newJSONRequest(http.MethodPost, "/", `{"name":"  kanata  "}`)

	err := BindAndValidateBody(req, &dst)
	if err != nil {
		t.Fatalf("BindAndValidateBody() error = %v", err)
	}
	if dst.Name != "kanata" {
		t.Fatalf("name = %q, want kanata", dst.Name)
	}
	if !reflect.DeepEqual(events, []string{"normalize"}) {
		t.Fatalf("events = %#v, want [normalize] only", events)
	}
}

type normalizerOnlyRequest struct {
	Name   string    `json:"name" validate:"required"`
	events *[]string `json:"-"`
}

func (r *normalizerOnlyRequest) Normalize() {
	*r.events = append(*r.events, "normalize")
	r.Name = strings.TrimSpace(r.Name)
}
