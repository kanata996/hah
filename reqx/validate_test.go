package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `BindAndValidate*` 包装器会优先执行 bind，并与“先 bind，再按来源 validate”保持一致。
// - [✓] 内部 `validate`、`postBindValidate`、`validateStruct`、`validateTarget` 会维持稳定目标契约。
// - [✓] validator 初始化、字段别名、tag 优先级、来源推断与 panic 分支都会产出稳定结果。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

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

// 对未实现 RequestValidator 的 DTO，各个 BindAndValidate 包装器的结果应与“先绑定，再按对应来源校验”的组合一致。
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
		wantErr := bind.Bind(newReq(), &want)
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
		wantErr := bind.Bind(newReq(), &want)
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
		wantErr := bind.BindBody(newReq(), &want)
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
		wantErr := bind.BindBody(newReq(), &want)
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
		wantErr := bind.BindQueryParams(newReq(), &want)
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
		wantErr := bind.BindQueryParams(newReq(), &want)
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
		wantErr := bind.BindPathValues(newReq(), &want)
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
		wantErr := bind.BindPathValues(newReq(), &want)
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
		wantErr := bind.BindHeaders(newReq(), &want)
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
		wantErr := bind.BindHeaders(newReq(), &want)
		if wantErr == nil {
			wantErr = validate(&want, sourceHeader)
		}

		_ = assertSameHTTPError(t, gotErr, wantErr)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got = %#v, want %#v", got, want)
		}
	})
}

func TestPostBindValidateRejectsInvalidTarget(t *testing.T) {
	if err := postBindValidate(newJSONRequest(http.MethodPost, "/", `{}`), 1, sourceBody); err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
		t.Fatalf("postBindValidate(non-struct) error = %v", err)
	}
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

// validate 会按 source 选择稳定的字段别名和 in 值。
func TestValidate_UsesSourceSpecificFieldAliases(t *testing.T) {
	t.Run("body uses json tag", func(t *testing.T) {
		var dst struct {
			DisplayName string `json:"display_name" validate:"required,nospace"`
		}

		violation := assertSingleViolation(t, validate(&dst, sourceBody))
		if violation.Field != "display_name" || violation.In != ViolationInBody || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("query uses query tag", func(t *testing.T) {
		var dst struct {
			Cursor string `query:"cursor" validate:"required"`
		}

		violation := assertSingleViolation(t, validate(&dst, sourceQuery))
		if violation.Field != "cursor" || violation.In != ViolationInQuery || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("path uses param tag", func(t *testing.T) {
		var dst struct {
			UUID string `param:"uuid" validate:"required,uuid"`
		}

		violation := assertSingleViolation(t, validate(&dst, sourcePath))
		if violation.Field != "uuid" || violation.In != ViolationInPath || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("headers use canonical header tag", func(t *testing.T) {
		var dst struct {
			RequestID string `header:"x-request-id" validate:"required"`
		}

		violation := assertSingleViolation(t, validate(&dst, sourceHeader))
		if violation.Field != "X-Request-Id" || violation.In != ViolationInHeader || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
	})
}

// validate(sourceRequest) 会按请求标签选择字段别名，但统一以 request 作为 in 值。
func TestValidateRequest_UsesRequestFieldAliases(t *testing.T) {
	var dst struct {
		ID      string `param:"id" validate:"required"`
		Cursor  string `query:"cursor" validate:"required"`
		Name    string `json:"name" validate:"required"`
		TraceID string `header:"x-trace-id" validate:"required"`
		Plain   string `validate:"required"`
	}

	violations := assertViolations(t, validate(&dst, sourceRequest))
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

// validate(sourceRequest) 会透传统一的空目标参数错误。
func TestValidateRequest_NilDestinationReturnsError(t *testing.T) {
	err := validate(nil, sourceRequest)
	if err == nil {
		t.Fatal("validate() error = nil")
	}
	if got := err.Error(); got != "reqx: target must not be nil" {
		t.Fatalf("error = %q, want reqx: target must not be nil", got)
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

func TestViolationInputHelpers(t *testing.T) {
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
		"id":         ViolationInRequest,
		"cursor":     ViolationInRequest,
		"name":       ViolationInRequest,
		"X-Trace-Id": ViolationInRequest,
		"Plain":      ViolationInRequest,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request violations = %#v, want %#v", got, want)
	}

	if got := validationInput(sourceRequest, 1, validationErrs[0]); got != ViolationInRequest {
		t.Fatalf("validationInput(fallback) = %q, want request", got)
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
