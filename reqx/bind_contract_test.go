package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] 缺失的 body/query/path/header 输入不会清空目标对象已有值。
// - [✓] `Bind` 在 path/query/body 任一阶段失败时不会留下部分更新。
// - [✓] `BindBody` 失败时不会污染引用类型字段。
// - [✓] `BindAndValidate` 绑定失败时不会修改目标对象，也不会继续执行校验。

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// 空 body 的 no-op 语义应保留已有字段值，而不是把目标对象重置为零值。
func TestBindBody_EmptyBodyPreservesExistingValues(t *testing.T) {
	type request struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	req := newJSONRequest(http.MethodPost, "/", "")
	dst := request{
		Name: "kanata",
		Age:  17,
	}

	if err := BindBody(req, &dst); err != nil {
		t.Fatalf("BindBody() error = %v", err)
	}
	if dst.Name != "kanata" || dst.Age != 17 {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}
}

// 纯空白 body 也应视为 no-op，并保留已有字段值。
func TestBindBody_WhitespaceOnlyBodyPreservesExistingValues(t *testing.T) {
	type request struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	req := newJSONRequest(http.MethodPost, "/", " \n\t ")
	dst := request{
		Name: "kanata",
		Age:  17,
	}

	if err := BindBody(req, &dst); err != nil {
		t.Fatalf("BindBody() error = %v", err)
	}
	if dst.Name != "kanata" || dst.Age != 17 {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}
}

// 缺失 query 参数时应保持目标对象中的现有值，避免把更早阶段的数据清空。
func TestBindQueryParams_MissingParamsPreserveExistingValues(t *testing.T) {
	type request struct {
		Page int    `query:"page"`
		Name string `query:"name"`
		Age  *int   `query:"age"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?other=1", nil)
	age := 17
	dst := request{
		Page: 3,
		Name: "kanata",
		Age:  &age,
	}

	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Page != 3 || dst.Name != "kanata" {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}
	if dst.Age == nil || *dst.Age != 17 {
		t.Fatalf("age = %#v, want 17", dst.Age)
	}
}

// 缺失 path 参数时应保持已有字段值。
func TestBindPathValues_MissingParamsPreserveExistingValues(t *testing.T) {
	type request struct {
		ID   int    `param:"id"`
		Name string `param:"name"`
	}

	req := requestWithPathParams(map[string][]string{
		"other": {"1"},
	})
	dst := request{
		ID:   42,
		Name: "kanata",
	}

	if err := BindPathValues(req, &dst); err != nil {
		t.Fatalf("BindPathValues() error = %v", err)
	}
	if dst.ID != 42 || dst.Name != "kanata" {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}
}

// Header 绑定默认应忽略未声明的请求头。
func TestBindHeaders_IgnoresUnknownFieldsByDefault(t *testing.T) {
	type request struct {
		RequestID string `header:"x-request-id"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "req-123")
	req.Header.Set("X-Extra", "ignored")

	var dst request
	if err := BindHeaders(req, &dst); err != nil {
		t.Fatalf("BindHeaders() error = %v", err)
	}
	if dst.RequestID != "req-123" {
		t.Fatalf("dst = %#v, want request id bound", dst)
	}
}

// 缺失 header 时也应保持已有字段值。
func TestBindHeaders_MissingHeaderPreservesExistingValues(t *testing.T) {
	type request struct {
		RequestID string `header:"x-request-id"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	dst := request{RequestID: "req-existing"}

	if err := BindHeaders(req, &dst); err != nil {
		t.Fatalf("BindHeaders() error = %v", err)
	}
	if dst.RequestID != "req-existing" {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}
}

// 顶层 Bind 在 path 阶段失败时，应直接返回错误且不保留 query 阶段的潜在更新。
func TestBind_PathTypeMismatchDoesNotPartiallyApply(t *testing.T) {
	type request struct {
		ID   int    `param:"id"`
		Page int    `query:"page"`
		Name string `json:"name"`
	}

	req := requestWithPathParams(map[string][]string{
		"id": {"oops"},
	})
	req.URL.RawQuery = "page=7"

	dst := request{
		ID:   42,
		Page: 3,
		Name: "kanata",
	}

	err := Bind(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "id" || violation.Code != ViolationCodeType || violation.Detail != "must be number" {
		t.Fatalf("violation = %#v", violation)
	}
	if dst.ID != 42 || dst.Page != 3 || dst.Name != "kanata" {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}
}

// POST 请求会跳过 query，空 body 也不会覆盖已有 JSON 字段。
func TestBind_PostSkipsQueryAndEmptyBodyPreservesExistingValues(t *testing.T) {
	type request struct {
		ID    string `param:"id" json:"id"`
		Scope string `query:"scope"`
		Name  string `json:"name"`
	}

	req := requestWithPathParams(map[string][]string{
		"id": {"route-id"},
	})
	req.Method = http.MethodPost
	req.URL.RawQuery = "scope=query-scope"

	dst := request{
		Scope: "existing-scope",
		Name:  "existing-name",
	}

	if err := Bind(req, &dst); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if dst.ID != "route-id" {
		t.Fatalf("Bind() id = %q, want route-id", dst.ID)
	}
	if dst.Scope != "existing-scope" || dst.Name != "existing-name" {
		t.Fatalf("dst = %#v, want skipped/no-op stages to preserve existing values", dst)
	}
}

// 绑定失败时不应留下部分更新，避免调用方拿到混合了新旧状态的脏对象。
func TestBindBody_TypeMismatchDoesNotPartiallyApply(t *testing.T) {
	type request struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	req := newJSONRequest(http.MethodPost, "/", `{"name":"new-name","age":"oops"}`)
	dst := request{
		Name: "existing-name",
		Age:  17,
	}

	err := BindBody(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "age" || violation.Code != ViolationCodeType {
		t.Fatalf("violation = %#v", violation)
	}
	if dst.Name != "existing-name" || dst.Age != 17 {
		t.Fatalf("dst = %#v, want bind failure to leave destination untouched", dst)
	}
}

// body 绑定失败时，引用类型字段也不应因浅拷贝而被偷偷修改。
func TestBindBody_TypeMismatchDoesNotMutateReferenceFields(t *testing.T) {
	type request struct {
		Meta map[string]string `json:"meta"`
		Age  int               `json:"age"`
	}

	req := newJSONRequest(http.MethodPost, "/", `{"meta":{"status":"new"},"age":"oops"}`)
	dst := request{
		Meta: map[string]string{"status": "old"},
		Age:  17,
	}

	err := BindBody(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "age" || violation.Code != ViolationCodeType {
		t.Fatalf("violation = %#v", violation)
	}
	if got := dst.Meta["status"]; got != "old" {
		t.Fatalf("meta.status = %q, want old", got)
	}
	if dst.Age != 17 {
		t.Fatalf("age = %d, want 17", dst.Age)
	}
}

// query 绑定失败时也不应留下部分更新。
func TestBindQueryParams_TypeMismatchDoesNotPartiallyApply(t *testing.T) {
	type request struct {
		Name string `query:"name"`
		Page int    `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?name=new-name&page=oops", nil)
	dst := request{
		Name: "existing-name",
		Page: 3,
	}

	err := BindQueryParams(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "page" || violation.Code != ViolationCodeType {
		t.Fatalf("violation = %#v", violation)
	}
	if dst.Name != "existing-name" || dst.Page != 3 {
		t.Fatalf("dst = %#v, want bind failure to leave destination untouched", dst)
	}
}

// header 绑定失败时也不应留下部分更新。
func TestBindHeaders_TypeMismatchDoesNotPartiallyApply(t *testing.T) {
	type request struct {
		RequestID string `header:"x-request-id"`
		Retry     int    `header:"x-retry"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "req-new")
	req.Header.Set("X-Retry", "oops")

	dst := request{
		RequestID: "req-existing",
		Retry:     2,
	}

	err := BindHeaders(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "X-Retry" || violation.Code != ViolationCodeType {
		t.Fatalf("violation = %#v", violation)
	}
	if dst.RequestID != "req-existing" || dst.Retry != 2 {
		t.Fatalf("dst = %#v, want bind failure to leave destination untouched", dst)
	}
}

// path 绑定失败时也不应留下部分更新。
func TestBindPathValues_TypeMismatchDoesNotPartiallyApply(t *testing.T) {
	type request struct {
		Name string `param:"name"`
		ID   int    `param:"id"`
	}

	req := requestWithPathParams(map[string][]string{
		"name": {"new-name"},
		"id":   {"oops"},
	})
	dst := request{
		Name: "existing-name",
		ID:   17,
	}

	err := BindPathValues(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "id" || violation.Code != ViolationCodeType {
		t.Fatalf("violation = %#v", violation)
	}
	if dst.Name != "existing-name" || dst.ID != 17 {
		t.Fatalf("dst = %#v, want bind failure to leave destination untouched", dst)
	}
}

// 顶层 Bind 在 query 阶段失败时，不应保留 path 阶段或 query 阶段的部分更新。
func TestBind_QueryTypeMismatchDoesNotPartiallyApply(t *testing.T) {
	type request struct {
		ID   string `param:"id"`
		Name string `query:"name"`
		Page int    `query:"page"`
	}

	req := requestWithPathParams(map[string][]string{
		"id": {"route-id"},
	})
	req.Method = http.MethodGet
	req.URL.RawQuery = "name=new-name&page=oops"

	dst := request{
		ID:   "existing-id",
		Name: "existing-name",
		Page: 3,
	}

	err := Bind(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "page" || violation.Code != ViolationCodeType {
		t.Fatalf("violation = %#v", violation)
	}
	if dst.ID != "existing-id" || dst.Name != "existing-name" || dst.Page != 3 {
		t.Fatalf("dst = %#v, want bind failure to leave destination untouched", dst)
	}
}

// 顶层 Bind 在 body 阶段失败时，不应保留 path 阶段或 body 阶段的部分更新。
func TestBind_BodyTypeMismatchDoesNotPartiallyApply(t *testing.T) {
	type request struct {
		ID   string `param:"id" json:"id"`
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	req := requestWithPathParams(map[string][]string{
		"id": {"route-id"},
	})
	req.Method = http.MethodPost
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.NoBody
	req.Body = newJSONRequest(http.MethodPost, "/", `{"name":"new-name","age":"oops"}`).Body

	dst := request{
		ID:   "existing-id",
		Name: "existing-name",
		Age:  17,
	}

	err := Bind(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "age" || violation.Code != ViolationCodeType {
		t.Fatalf("violation = %#v", violation)
	}
	if dst.ID != "existing-id" || dst.Name != "existing-name" || dst.Age != 17 {
		t.Fatalf("dst = %#v, want bind failure to leave destination untouched", dst)
	}
}

// BindAndValidate 在绑定失败时应直接返回绑定错误，不污染目标对象，也不进入校验阶段。
func TestBindAndValidate_BindFailureDoesNotMutateDestinationOrRunValidation(t *testing.T) {
	type request struct {
		ID   string `param:"id"`
		Name string `query:"name" validate:"nospace"`
		Page int    `query:"page" validate:"min=1"`
	}

	req := requestWithPathParams(map[string][]string{
		"id": {"route-id"},
	})
	req.Method = http.MethodGet
	req.URL.RawQuery = "name=new-name&page=oops"

	dst := request{
		ID:   "existing-id",
		Name: "bad value",
		Page: 3,
	}

	err := BindAndValidate(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "page" || violation.Code != ViolationCodeType {
		t.Fatalf("violation = %#v", violation)
	}
	if dst.ID != "existing-id" || dst.Name != "bad value" || dst.Page != 3 {
		t.Fatalf("dst = %#v, want bind failure to leave destination untouched", dst)
	}
}
