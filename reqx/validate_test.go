package reqx

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/kanata996/hah/bind"
)

func bindAndValidateBody(r *http.Request, target any) error {
	if err := bind.BindBody(r, target); err != nil {
		return err
	}
	return Validate(r, target, SourceBody)
}

func bindAndValidateQuery(r *http.Request, target any) error {
	if err := bind.BindQueryParams(r, target); err != nil {
		return err
	}
	return Validate(r, target, SourceQuery)
}

func bindAndValidatePath(r *http.Request, target any) error {
	if err := bind.BindPathValues(r, target); err != nil {
		return err
	}
	return Validate(r, target, SourcePath)
}

func bindAndValidateHeaders(r *http.Request, target any) error {
	if err := bind.BindHeaders(r, target); err != nil {
		return err
	}
	return Validate(r, target, SourceHeader)
}

func mustValidationErrors(t *testing.T, source Source, target any) validator.ValidationErrors {
	t.Helper()

	err := validatorFor(source).Struct(target)
	if err == nil {
		t.Fatalf("validatorFor(%q).Struct(%T) error = nil, want validation errors", source, target)
	}

	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		t.Fatalf("validatorFor(%q).Struct(%T) error = %T, want validator.ValidationErrors", source, target, err)
	}

	return validationErrs
}

func TestBindAndValidateRejectsInvalidInputs(t *testing.T) {
	var dst struct{}

	if err := BindAndValidate(nil, &dst); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("BindAndValidate() error = %v", err)
	}
	req := newJSONRequest(http.MethodPost, "/", `{}`)
	if err := BindAndValidate(req, nil); err == nil || err.Error() != "reqx: destination must not be nil" {
		t.Fatalf("BindAndValidate(nil target) error = %v", err)
	}

	t.Run("struct value target", func(t *testing.T) {
		err := BindAndValidate(httptest.NewRequest(http.MethodGet, "/", nil), struct{}{})
		if err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
			t.Fatalf("BindAndValidate(struct value) error = %v", err)
		}
	})

	t.Run("typed nil struct pointer", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		var dst *request
		err := BindAndValidate(newJSONRequest(http.MethodPost, "/", `{}`), dst)
		if err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
			t.Fatalf("BindAndValidate(typed nil struct pointer) error = %v", err)
		}
	})

	t.Run("pointer to non-struct", func(t *testing.T) {
		dst := &[]string{}
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		err := BindAndValidate(req, dst)
		if err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
			t.Fatalf("BindAndValidate(pointer to non-struct) error = %v", err)
		}
	})
}

func TestBindAndValidate_RejectsNonStructTargetBeforeBinding(t *testing.T) {
	dst := []string{"existing"}
	req := newJSONRequest(http.MethodPost, "/", `["changed"]`)

	err := BindAndValidate(req, &dst)
	if err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
		t.Fatalf("BindAndValidate(pointer to slice) error = %v", err)
	}
	if !reflect.DeepEqual(dst, []string{"existing"}) {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}

	body, readErr := io.ReadAll(req.Body)
	if readErr != nil {
		t.Fatalf("ReadAll(req.Body) error = %v", readErr)
	}
	if string(body) != `["changed"]` {
		t.Fatalf("remaining body = %q, want original unread body", body)
	}
}

func TestValidateRejectsInvalidInputs(t *testing.T) {
	var dst struct{}

	if err := Validate(nil, &dst, SourceBody); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("Validate(nil request) error = %v", err)
	}

	req := newJSONRequest(http.MethodPost, "/", `{}`)
	if err := Validate(req, nil, SourceBody); err == nil || err.Error() != "reqx: target must not be nil" {
		t.Fatalf("Validate(nil target) error = %v", err)
	}
	if err := Validate(req, &dst, Source("unsupported")); err == nil || err.Error() != `reqx: unsupported validation source "unsupported"` {
		t.Fatalf("Validate(unsupported source) error = %v", err)
	}

	t.Run("struct value target", func(t *testing.T) {
		err := Validate(req, struct{}{}, SourceBody)
		if err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
			t.Fatalf("Validate(struct value) error = %v", err)
		}
	})

	t.Run("typed nil struct pointer", func(t *testing.T) {
		var dst *struct{}
		err := Validate(req, dst, SourceBody)
		if err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
			t.Fatalf("Validate(typed nil struct pointer) error = %v", err)
		}
	})

	t.Run("pointer to non-struct", func(t *testing.T) {
		dst := &[]string{}
		err := Validate(req, dst, SourceBody)
		if err == nil || err.Error() != "reqx: target must be a non-nil pointer to struct" {
			t.Fatalf("Validate(pointer to non-struct) error = %v", err)
		}
	})
}

func TestValidationResultFromError(t *testing.T) {
	t.Run("validation errors are sorted and deduplicated by field", func(t *testing.T) {
		type zRequiredRequest struct {
			Z string `query:"z" validate:"required"`
		}
		type aRequiredRequest struct {
			A string `query:"a" validate:"required"`
		}
		type aMinRequest struct {
			A string `query:"a" validate:"min=2"`
		}

		errs := append(validator.ValidationErrors{}, mustValidationErrors(t, SourceQuery, zRequiredRequest{})...)
		errs = append(errs, mustValidationErrors(t, SourceQuery, aRequiredRequest{})...)
		errs = append(errs, mustValidationErrors(t, SourceQuery, aMinRequest{A: "x"})...)

		got, err := validationResultFromError(SourceQuery, errs)
		if err != nil {
			t.Fatalf("validationResultFromError() error = %v", err)
		}

		want := []Violation{
			{Field: "a", In: ViolationInQuery, Code: ViolationCodeRequired, Detail: "is required"},
			{Field: "z", In: ViolationInQuery, Code: ViolationCodeRequired, Detail: "is required"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("violations = %#v, want %#v", got, want)
		}
	})

	t.Run("invalid validation error is returned as is", func(t *testing.T) {
		wantErr := &validator.InvalidValidationError{Type: reflect.TypeOf(0)}

		got, err := validationResultFromError(SourceBody, wantErr)
		if err != wantErr {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if got != nil {
			t.Fatalf("violations = %#v, want nil", got)
		}
	})

	t.Run("unexpected validator error is returned as is", func(t *testing.T) {
		wantErr := errors.New("validator boom")

		got, err := validationResultFromError(SourceBody, wantErr)
		if err != wantErr {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if got != nil {
			t.Fatalf("violations = %#v, want nil", got)
		}
	})
}

func TestFieldAlias_UsesSourceTagPriority(t *testing.T) {
	type request struct {
		Value string `json:"json_name,omitempty" query:"query_name" param:"param_name" header:"x-trace-id"`
	}

	field := reflect.TypeOf(request{}).Field(0)
	testCases := []struct {
		name   string
		source Source
		want   string
	}{
		{name: "body", source: SourceBody, want: "json_name"},
		{name: "query", source: SourceQuery, want: "query_name"},
		{name: "path", source: SourcePath, want: "param_name"},
		{name: "header", source: SourceHeader, want: "X-Trace-Id"},
		{name: "request conflicting tags fall back to struct field", source: SourceRequest, want: "Value"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fieldAlias(field, tc.source); got != tc.want {
				t.Fatalf("fieldAlias(%q) = %q, want %q", tc.source, got, tc.want)
			}
		})
	}
}

func TestFieldAlias_SourceRequestUsesSharedInputNameWhenTagsAgree(t *testing.T) {
	type request struct {
		AccountID string `json:"account_id" query:"account_id" param:"account_id" validate:"required"`
	}

	field := reflect.TypeOf(request{}).Field(0)
	if got := fieldAlias(field, SourceRequest); got != "account_id" {
		t.Fatalf("fieldAlias(SourceRequest) = %q, want account_id", got)
	}
}

func TestBindAndValidateCompositionsReturnBindingErrorsFromBindPackage(t *testing.T) {
	t.Run("body invalid json", func(t *testing.T) {
		type bodyRequest struct {
			Name int `json:"name"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"oops"}`)
		gotErr := bindAndValidateBody(req, &bodyRequest{})

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
		gotErr := bindAndValidateBody(req, &bodyRequest{})

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
		gotErr := bindAndValidateQuery(req, &requestQuery{})

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
		gotErr := bindAndValidatePath(req, &requestPath{})

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
		gotErr := bindAndValidateHeaders(req, &requestHeader{})

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

// `BindAndValidate` 与 `bind + Validate` 组合在正常输入下都能顺利通过。
func TestBindAndValidateAndValidateSuccessPaths(t *testing.T) {
	type bodyRequest struct {
		Name string `json:"name" validate:"required"`
	}
	bodyReq := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)
	var bodyDst bodyRequest
	if err := bindAndValidateBody(bodyReq, &bodyDst); err != nil {
		t.Fatalf("bindAndValidateBody() error = %v", err)
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
	if err := bindAndValidateQuery(queryReq, &queryDst); err != nil {
		t.Fatalf("bindAndValidateQuery() error = %v", err)
	}
	if queryDst.Cursor != "abc" {
		t.Fatalf("queryDst = %#v, want bound query field", queryDst)
	}

	type pathRequest struct {
		UUID string `param:"uuid" validate:"required"`
	}
	pathReq := requestWithPathParams(map[string][]string{"uuid": {"u_1"}})
	var pathDst pathRequest
	if err := bindAndValidatePath(pathReq, &pathDst); err != nil {
		t.Fatalf("bindAndValidatePath() error = %v", err)
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
	if err := bindAndValidateHeaders(headerReq, &headerDst); err != nil {
		t.Fatalf("bindAndValidateHeaders() error = %v", err)
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

// `Validate(Source*)` 会把各来源的字段别名和 violation 包络直接暴露给调用方。
func TestValidateReturnPublicValidationViolations(t *testing.T) {
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
		violation := assertSingleViolation(t, bindAndValidateBody(newJSONRequest(http.MethodPost, "/", `{"display_name":"bad value"}`), &dst))
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
		violation := assertSingleViolation(t, bindAndValidateQuery(req, &dst))
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
		violation := assertSingleViolation(t, bindAndValidatePath(req, &dst))
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
		violation := assertSingleViolation(t, bindAndValidateHeaders(req, &dst))
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

func TestBindAndValidate_RequestAliasFallsBackToStructFieldOnConflictingSourceNames(t *testing.T) {
	var dst struct {
		AccountID string `param:"account_id" json:"id" validate:"required,nospace"`
	}

	req := requestWithPathParams(map[string][]string{
		"account_id": {"acct_1"},
	})
	req.Method = http.MethodPost
	req.Body = io.NopCloser(strings.NewReader(`{"id":"bad value"}`))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(`{"id":"bad value"}`))

	violation := assertSingleViolation(t, BindAndValidate(req, &dst))
	want := Violation{
		Field:  "AccountID",
		In:     ViolationInRequest,
		Code:   ViolationCodeInvalid,
		Detail: "is invalid",
	}
	if violation != want {
		t.Fatalf("violation = %#v, want %#v", violation, want)
	}
	if dst.AccountID != "bad value" {
		t.Fatalf("dst = %#v, want bound body value preserved", dst)
	}
}

// body 来源的嵌套 struct 校验失败时，violation 字段路径应包含完整嵌套层级。
func TestValidateBody_NestedStructViolationFieldPath(t *testing.T) {
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
	violation := assertSingleViolation(t, bindAndValidateBody(req, &dst))
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
func TestValidateBody_NormalizerOnlyWithoutRequestValidator(t *testing.T) {
	var events []string
	dst := normalizerOnlyRequest{events: &events, Name: ""}
	req := newJSONRequest(http.MethodPost, "/", `{"name":"  kanata  "}`)

	err := bindAndValidateBody(req, &dst)
	if err != nil {
		t.Fatalf("bindAndValidateBody() error = %v", err)
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
