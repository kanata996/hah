package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `BindQueryParams` 能绑定支持的标量、指针、切片和 `encoding.TextUnmarshaler` 类型。
// - [✓] `BindQueryParams` 的未知字段、空输入、nil URL 与非法绑定定义契约。
// - [✓] `BindQueryParams` 会把重复值、类型错误和无效文本解码稳定映射为 violation。
// - [✓] 多字段同时失败时会聚合 violation 且保持目标对象不变。
// - [✓] `BindAndValidateQuery` 会在校验前执行 `Normalize()`，并使用 `query` tag 字段名。
// - [✓] `BindAndValidateQuery` 在绑定失败时优先返回绑定错误，并透传公开空输入参数错误。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type queryState string

func (s *queryState) UnmarshalText(text []byte) error {
	switch string(text) {
	case "active", "disabled":
		*s = queryState(text)
		return nil
	default:
		return errors.New("invalid state")
	}
}

// 查询参数可以绑定到支持的标量、指针、切片和 TextUnmarshaler 类型。
func TestBindQueryParams_BindsSupportedTypes(t *testing.T) {
	type request struct {
		Name     string       `query:"name"`
		Enabled  bool         `query:"enabled"`
		Age      *int         `query:"age"`
		Score    float64      `query:"score"`
		Tags     []string     `query:"tag"`
		State    queryState   `query:"state"`
		StatePtr *queryState  `query:"state_ptr"`
		States   []queryState `query:"states"`
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/?name=kanata&enabled=true&age=17&score=9.5&tag=a&tag=b&state=active&state_ptr=disabled&states=active&states=disabled",
		nil,
	)

	var dst request
	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Name != "kanata" {
		t.Fatalf("name = %q, want kanata", dst.Name)
	}
	if !dst.Enabled {
		t.Fatal("enabled = false, want true")
	}
	if dst.Age == nil || *dst.Age != 17 {
		t.Fatalf("age = %#v, want 17", dst.Age)
	}
	if dst.Score != 9.5 {
		t.Fatalf("score = %v, want 9.5", dst.Score)
	}
	if strings.Join(dst.Tags, ",") != "a,b" {
		t.Fatalf("tags = %#v, want [a b]", dst.Tags)
	}
	if dst.State != "active" {
		t.Fatalf("state = %q, want active", dst.State)
	}
	if dst.StatePtr == nil || *dst.StatePtr != "disabled" {
		t.Fatalf("state_ptr = %#v, want disabled", dst.StatePtr)
	}
	if len(dst.States) != 2 || dst.States[0] != "active" || dst.States[1] != "disabled" {
		t.Fatalf("states = %#v, want [active disabled]", dst.States)
	}
}

// 默认配置下，未知查询参数会被忽略。
func TestBindQueryParams_IgnoresUnknownFieldsByDefault(t *testing.T) {
	type request struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?page=2&extra=1", nil)

	var dst request
	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Page != 2 {
		t.Fatalf("page = %d, want 2", dst.Page)
	}
}

// 单字段 query 解析错误会稳定映射为对应 violation。
func TestBindQueryParams_MapsSingleFieldViolations(t *testing.T) {
	t.Run("repeated scalar", func(t *testing.T) {
		type request struct {
			ID string `query:"id"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?id=1&id=2", nil)

		var dst request
		violation := assertSingleViolation(t, BindQueryParams(req, &dst))
		if violation.Field != "id" || violation.Code != ViolationCodeMultiple || violation.Detail != "must not be repeated" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("scalar type mismatch", func(t *testing.T) {
		type request struct {
			Page int `query:"page"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?page=nope", nil)

		var dst request
		violation := assertSingleViolation(t, BindQueryParams(req, &dst))
		if violation.Field != "page" || violation.Code != ViolationCodeType || violation.Detail != "must be number" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("invalid text unmarshaler", func(t *testing.T) {
		type request struct {
			State queryState `query:"state"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?state=unknown", nil)

		var dst request
		violation := assertSingleViolation(t, BindQueryParams(req, &dst))
		if violation.Field != "state" || violation.Code != ViolationCodeInvalid || violation.Detail != "is invalid" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("repeated text unmarshaler", func(t *testing.T) {
		type request struct {
			State queryState `query:"state"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?state=active&state=disabled", nil)

		var dst request
		violation := assertSingleViolation(t, BindQueryParams(req, &dst))
		if violation.Field != "state" || violation.Code != ViolationCodeMultiple || violation.Detail != "must not be repeated" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("pointer type mismatch", func(t *testing.T) {
		type request struct {
			Page *int `query:"page"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?page=oops", nil)

		var dst request
		violation := assertSingleViolation(t, BindQueryParams(req, &dst))
		if violation.Field != "page" || violation.Code != ViolationCodeType || violation.Detail != "must be number" {
			t.Fatalf("violation = %#v", violation)
		}
		if dst.Page != nil {
			t.Fatalf("page = %#v, want nil", dst.Page)
		}
	})
}

// 非法的 query 绑定定义会在构建解码计划时被拒绝。
func TestBindQueryParams_RejectsInvalidBindingSchema(t *testing.T) {
	t.Run("unsupported field type", func(t *testing.T) {
		type nested struct {
			Name string
		}
		type request struct {
			Nested nested `query:"nested"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?nested=x", nil)

		var dst request
		err := BindQueryParams(req, &dst)
		if err == nil {
			t.Fatal("BindQueryParams() error = nil")
		}
		if got := err.Error(); !strings.Contains(got, `unsupported query type`) {
			t.Fatalf("error = %q, want unsupported query type", got)
		}
	})

	t.Run("duplicate tagged fields", func(t *testing.T) {
		type request struct {
			Page  int `query:"page"`
			Limit int `query:"page"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?page=1", nil)

		var dst request
		err := BindQueryParams(req, &dst)
		if err == nil {
			t.Fatal("BindQueryParams() error = nil")
		}
		if got := err.Error(); got != `reqx: duplicate query field "page" on Page and Limit` {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("unexported tagged field", func(t *testing.T) {
		type request struct {
			name string `query:"name"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=kanata", nil)

		var dst request
		_ = dst.name
		err := BindQueryParams(req, &dst)
		if err == nil {
			t.Fatal("BindQueryParams() error = nil")
		}
		if got := err.Error(); !strings.Contains(got, `must be exported`) {
			t.Fatalf("error = %q, want must be exported", got)
		}
	})
}

// 空输入会在进入绑定前被直接拒绝。
func TestBindQueryParams_RejectsNilInputs(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		var dst struct{}
		err := BindQueryParams[struct{}](nil, &dst)
		if err == nil {
			t.Fatal("BindQueryParams() error = nil")
		}
		if got := err.Error(); got != "reqx: request must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("nil destination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?page=1", nil)

		err := BindQueryParams[struct{}](req, nil)
		if err == nil {
			t.Fatal("BindQueryParams() error = nil")
		}
		if got := err.Error(); got != "reqx: destination must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})
}

// nil URL 会按空查询串处理，并保留已有字段值。
func TestBindQueryParams_NilURLPreservesExistingValues(t *testing.T) {
	type request struct {
		Page int    `query:"page"`
		Name string `query:"name"`
	}

	req := &http.Request{Header: make(http.Header)}
	dst := request{
		Page: 7,
		Name: "kanata",
	}

	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Page != 7 || dst.Name != "kanata" {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}
}

// 多个 query 字段同时出错时会聚合 violation，并保持目标对象不变。
func TestBindQueryParams_AggregatesViolationsAndPreservesDestination(t *testing.T) {
	type request struct {
		Page   int          `query:"page"`
		IDs    []int        `query:"id"`
		States []queryState `query:"state"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?page=oops&id=1&id=nope&state=active&state=unknown", nil)
	dst := request{
		Page:   7,
		IDs:    []int{9, 8},
		States: []queryState{"disabled"},
	}

	violations := assertViolations(t, BindQueryParams(req, &dst))
	if len(violations) != 3 {
		t.Fatalf("violations len = %d, want 3", len(violations))
	}

	want := []Violation{
		{Field: "page", Code: ViolationCodeType, Detail: "must be number"},
		{Field: "id", Code: ViolationCodeType, Detail: "must be number"},
		{Field: "state", Code: ViolationCodeInvalid, Detail: "is invalid"},
	}
	for i, wantViolation := range want {
		got := violations[i]
		if got.Field != wantViolation.Field || got.Code != wantViolation.Code || got.Detail != wantViolation.Detail {
			t.Fatalf("violations[%d] = %#v, want %#v", i, got, wantViolation)
		}
	}

	if dst.Page != 7 {
		t.Fatalf("page = %d, want 7", dst.Page)
	}
	if !reflect.DeepEqual(dst.IDs, []int{9, 8}) {
		t.Fatalf("ids = %#v, want [9 8]", dst.IDs)
	}
	if !reflect.DeepEqual(dst.States, []queryState{"disabled"}) {
		t.Fatalf("states = %#v, want [disabled]", dst.States)
	}
}

// 查询校验错误字段名使用 query tag 名称。
func TestBindAndValidateQuery_UsesQueryTagName(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	var dst struct {
		Cursor string `query:"cursor" validate:"required"`
	}
	err := BindAndValidateQuery(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "cursor" || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
		t.Fatalf("violation = %#v", violation)
	}
}

// 绑定后会先标准化再校验。
func TestBindAndValidateQuery_NormalizesBeforeValidation(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?name=%20kanata%20", nil)

	var dst normalizedQueryRequest
	if err := BindAndValidateQuery(req, &dst); err != nil {
		t.Fatalf("BindAndValidateQuery() error = %v", err)
	}
	if dst.Name != "kanata" {
		t.Fatalf("name = %q, want kanata", dst.Name)
	}
}

// query 包装器会在绑定前直接拒绝空输入参数。
func TestBindAndValidateQuery_RejectsNilInputs(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		var dst struct{}
		err := BindAndValidateQuery[struct{}](nil, &dst)
		if err == nil {
			t.Fatal("BindAndValidateQuery() error = nil")
		}
		if got := err.Error(); got != "reqx: request must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("nil destination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?cursor=abc", nil)
		err := BindAndValidateQuery[struct{}](req, nil)
		if err == nil {
			t.Fatal("BindAndValidateQuery() error = nil")
		}
		if got := err.Error(); got != "reqx: destination must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})
}

// query 包装器在绑定失败时不会继续进入校验阶段，也不会污染目标对象。
func TestBindAndValidateQuery_ReturnsBindingErrorBeforeValidation(t *testing.T) {
	type request struct {
		Name string `query:"name" validate:"required,nospace"`
		Page int    `query:"page" validate:"min=1"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?name=bad%20value&page=oops", nil)
	dst := request{
		Name: "existing name",
		Page: 3,
	}

	err := BindAndValidateQuery(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "page" || violation.In != ViolationInQuery || violation.Code != ViolationCodeType || violation.Detail != "must be number" {
		t.Fatalf("violation = %#v", violation)
	}
	if dst.Name != "existing name" || dst.Page != 3 {
		t.Fatalf("dst = %#v, want destination preserved when bind fails", dst)
	}
}

type normalizedQueryRequest struct {
	Name string `query:"name" validate:"required,nospace"`
}

func (r *normalizedQueryRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
}
