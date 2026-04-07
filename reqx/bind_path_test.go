package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `BindPathValues` 的空输入、未知字段、无匹配 pattern 与类型错误契约。
// - [✓] 非法 path 绑定定义会在解码计划阶段被拒绝。
// - [✓] `ParamString`、`ParamInt`、`ParamUUID` 的裁剪、解析与错误映射行为。
// - [✓] `BindAndValidatePath` 会在校验前执行 `Normalize()`，并使用 `param` tag 字段名。
// - [✓] `BindAndValidatePath` 在绑定失败时优先返回绑定错误，并透传公开空输入参数错误。

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// 绑定后会先裁剪 path 参数再做校验。
func TestBindAndValidatePath_BindsTrimmedParams(t *testing.T) {
	type pathRequest struct {
		UUID string `param:"uuid" validate:"required,uuid"`
	}

	req := requestWithPathParams(map[string][]string{
		"uuid": {" 550e8400-e29b-41d4-a716-446655440000 "},
	})

	var path pathRequest
	if err := BindAndValidatePath(req, &path); err != nil {
		t.Fatalf("BindAndValidatePath() error = %v", err)
	}
	if path.UUID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("BindAndValidatePath() uuid = %q", path.UUID)
	}
}

// BindAndValidatePath 会使用 path 校验器，错误字段名应来自 param tag。
func TestBindAndValidatePath_UsesParamTagName(t *testing.T) {
	req := requestWithPathParams(nil)

	var dst struct {
		UUID string `param:"uuid" validate:"required,uuid"`
	}
	err := BindAndValidatePath(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "uuid" || violation.In != ViolationInPath || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
		t.Fatalf("violation = %#v", violation)
	}
}

// 绑定后会先做标准化，再进入 path 校验。
func TestBindAndValidatePath_NormalizesBeforeValidation(t *testing.T) {
	req := requestWithPathParams(map[string][]string{
		"state": {" ACTIVE "},
	})

	var dst normalizedPathRequest
	if err := BindAndValidatePath(req, &dst); err != nil {
		t.Fatalf("BindAndValidatePath() error = %v", err)
	}
	if dst.State != "active" {
		t.Fatalf("state = %q, want active", dst.State)
	}
}

// 空输入会在进入 path 绑定前被直接拒绝。
func TestBindPathValues_RejectsNilInputs(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		var dst struct{}

		err := BindPathValues[struct{}](nil, &dst)
		if err == nil {
			t.Fatal("BindPathValues() error = nil")
		}
		if got := err.Error(); got != "reqx: request must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("nil destination", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"1"},
		})

		err := BindPathValues[struct{}](req, nil)
		if err == nil {
			t.Fatal("BindPathValues() error = nil")
		}
		if got := err.Error(); got != "reqx: destination must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})
}

// path 包装器会在绑定前直接拒绝空输入参数。
func TestBindAndValidatePath_RejectsNilInputs(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		var dst struct{}
		err := BindAndValidatePath[struct{}](nil, &dst)
		if err == nil {
			t.Fatal("BindAndValidatePath() error = nil")
		}
		if got := err.Error(); got != "reqx: request must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("nil destination", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"1"},
		})

		err := BindAndValidatePath[struct{}](req, nil)
		if err == nil {
			t.Fatal("BindAndValidatePath() error = nil")
		}
		if got := err.Error(); got != "reqx: destination must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})
}

// 默认配置下，未知路径参数会被忽略。
func TestBindPathValues_IgnoresUnknownFieldsByDefault(t *testing.T) {
	type pathRequest struct {
		ID string `param:"id"`
	}

	req := requestWithPathParams(map[string][]string{
		"id":    {"42"},
		"extra": {"ignored"},
	})

	var path pathRequest
	if err := BindPathValues(req, &path); err != nil {
		t.Fatalf("BindPathValues() error = %v", err)
	}
	if path.ID != "42" {
		t.Fatalf("BindPathValues() id = %q", path.ID)
	}
}

// 没有匹配 pattern 时，path 绑定应退化为 no-op，而不是报错或清空已有值。
func TestBindPathValues_NoPatternPreservesExistingValues(t *testing.T) {
	type pathRequest struct {
		ID string `param:"id"`
	}

	req := httptest.NewRequest(http.MethodGet, "/items/42", nil)
	dst := pathRequest{ID: "existing-id"}

	if err := BindPathValues(req, &dst); err != nil {
		t.Fatalf("BindPathValues() error = %v", err)
	}
	if dst.ID != "existing-id" {
		t.Fatalf("BindPathValues() id = %q, want existing-id", dst.ID)
	}
}

// path 参数类型不匹配时返回类型错误。
func TestBindPathValues_TypeMismatchReturnsViolation(t *testing.T) {
	type pathRequest struct {
		ID int `param:"id"`
	}

	req := requestWithPathParams(map[string][]string{
		"id": {"abc"},
	})

	var path pathRequest
	err := BindPathValues(req, &path)
	violation := assertSingleViolation(t, err)
	if violation.Field != "id" || violation.Code != ViolationCodeType || violation.Detail != "must be number" {
		t.Fatalf("BindPathValues() violation = %#v", violation)
	}
}

// 空 path 值会在已知 wildcard 存在时绑定为空串，而不是被当成缺失字段。
func TestBindPathValues_EmptyPathValueBindsBlankString(t *testing.T) {
	type pathRequest struct {
		ID string `param:"id"`
	}

	req := requestWithPathParams(map[string][]string{
		"id": {""},
	})

	var path pathRequest
	if err := BindPathValues(req, &path); err != nil {
		t.Fatalf("BindPathValues() error = %v", err)
	}
	if path.ID != "" {
		t.Fatalf("BindPathValues() id = %q, want empty string", path.ID)
	}
}

// 非法的 path 绑定定义会在构建解码计划时被拒绝。
func TestBindPathValues_RejectsInvalidBindingSchema(t *testing.T) {
	t.Run("unsupported field type", func(t *testing.T) {
		type nested struct {
			Name string
		}
		type pathRequest struct {
			Nested nested `param:"nested"`
		}

		req := requestWithPathParams(map[string][]string{
			"nested": {"x"},
		})

		var path pathRequest
		err := BindPathValues(req, &path)
		if err == nil {
			t.Fatal("BindPathValues() error = nil")
		}
		if got := err.Error(); got != `reqx: field "Nested" has unsupported path type reqx.nested` {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("unexported tagged field", func(t *testing.T) {
		type pathRequest struct {
			id string `param:"id"`
		}

		req := requestWithPathParams(map[string][]string{
			"id": {"1"},
		})

		var path pathRequest
		_ = path.id
		err := BindPathValues(req, &path)
		if err == nil {
			t.Fatal("BindPathValues() error = nil")
		}
		if got := err.Error(); got != `reqx: path field "id" must be exported` {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("duplicate tagged fields", func(t *testing.T) {
		type pathRequest struct {
			ID   string `param:"id"`
			UUID string `param:"id"`
		}

		req := requestWithPathParams(map[string][]string{
			"id": {"1"},
		})

		var path pathRequest
		err := BindPathValues(req, &path)
		if err == nil {
			t.Fatal("BindPathValues() error = nil")
		}
		if got := err.Error(); got != `reqx: duplicate path field "id" on ID and UUID` {
			t.Fatalf("error = %q", got)
		}
	})
}

// path 包装器在绑定失败时不会进入校验阶段，也不会污染目标对象。
func TestBindAndValidatePath_ReturnsBindingErrorBeforeValidation(t *testing.T) {
	type pathRequest struct {
		State string `param:"state" validate:"required,oneof=active disabled"`
		ID    int    `param:"id" validate:"min=1"`
	}

	req := requestWithPathParams(map[string][]string{
		"state": {"bad state"},
		"id":    {"oops"},
	})
	dst := pathRequest{
		State: "existing state",
		ID:    3,
	}

	err := BindAndValidatePath(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "id" || violation.In != ViolationInPath || violation.Code != ViolationCodeType || violation.Detail != "must be number" {
		t.Fatalf("violation = %#v", violation)
	}
	if dst.State != "existing state" || dst.ID != 3 {
		t.Fatalf("dst = %#v, want destination preserved when bind fails", dst)
	}
}

// path helper 会裁剪输入，并在需要时做解析或规范化。
func TestPathHelpers_ParseAndNormalizeValues(t *testing.T) {
	t.Run("ParamString trims value", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"name": {"  kanata  "},
		})

		value, err := ParamString(req, "name")
		if err != nil {
			t.Fatalf("ParamString() error = %v", err)
		}
		if value != "kanata" {
			t.Fatalf("ParamString() value = %q", value)
		}
	})

	t.Run("ParamInt parses trimmed value", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {" 42 "},
		})

		value, err := ParamInt(req, "id")
		if err != nil {
			t.Fatalf("ParamInt() error = %v", err)
		}
		if value != 42 {
			t.Fatalf("ParamInt() value = %d", value)
		}
	})

	t.Run("ParamUUID normalizes value", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"uuid": {" 550E8400-E29B-41D4-A716-446655440000 "},
		})

		value, err := ParamUUID(req, "uuid")
		if err != nil {
			t.Fatalf("ParamUUID() error = %v", err)
		}
		if value != "550e8400-e29b-41d4-a716-446655440000" {
			t.Fatalf("ParamUUID() value = %q", value)
		}
	})
}

// ParamInt 会把类型错误和缺失参数都映射为稳定的 violation。
func TestParamInt_MapsLookupAndDecodeFailures(t *testing.T) {
	t.Run("invalid value", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"oops"},
		})

		_, err := ParamInt(req, "id")
		violation := assertSingleViolation(t, err)
		if violation.Field != "id" || violation.Code != ViolationCodeType || violation.Detail != "must be number" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("missing value", func(t *testing.T) {
		req := requestWithPathParams(nil)

		_, err := ParamInt(req, "id")
		violation := assertSingleViolation(t, err)
		if violation.Field != "id" || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
	})
}

// ParamString 会把 path 查找阶段的各种失败统一映射为稳定的错误契约。
func TestParamString_MapsLookupFailures(t *testing.T) {
	t.Run("whitespace only value", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"   "},
		})

		_, err := ParamString(req, "id")
		violation := assertSingleViolation(t, err)
		if violation.Field != "id" || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("missing value", func(t *testing.T) {
		req := requestWithPathParams(nil)

		_, err := ParamString(req, "uuid")
		violation := assertSingleViolation(t, err)
		if violation.Field != "uuid" || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
			t.Fatalf("ParamString() violation = %#v", violation)
		}
	})

	t.Run("no matched pattern", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items/42", nil)

		_, err := ParamString(req, "id")
		violation := assertSingleViolation(t, err)
		if violation.Field != "id" || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
	})
}

// 空白参数名会直接返回参数名错误。
func TestParamString_EmptyNameReturnsError(t *testing.T) {
	req := requestWithPathParams(map[string][]string{
		"id": {"1"},
	})

	_, err := ParamString(req, "   ")
	if err == nil {
		t.Fatal("ParamString() error = nil")
	}
	if got := err.Error(); got != "reqx: path param name must not be empty" {
		t.Fatalf("error = %q", got)
	}
}

// ParamUUID 会把字符串查找失败和 UUID 语义校验失败都映射为稳定的错误契约。
func TestParamUUID_MapsLookupAndValidationFailures(t *testing.T) {
	t.Run("invalid value", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"uuid": {"not-a-uuid"},
		})

		_, err := ParamUUID(req, "uuid")
		violation := assertSingleViolation(t, err)
		if violation.Field != "uuid" || violation.Code != ViolationCodeInvalid || violation.Detail != "is invalid" {
			t.Fatalf("ParamUUID() violation = %#v", violation)
		}
	})

	t.Run("missing value", func(t *testing.T) {
		req := requestWithPathParams(nil)

		_, err := ParamUUID(req, "uuid")
		violation := assertSingleViolation(t, err)
		if violation.Field != "uuid" || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
	})
}

func requestWithPathParams(params map[string][]string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Pattern = syntheticPatternFromPathParams(params)

	for key, values := range params {
		if len(values) == 0 {
			continue
		}
		req.SetPathValue(key, values[0])
	}

	return req
}

func syntheticPatternFromPathParams(params map[string][]string) string {
	if len(params) == 0 {
		return ""
	}

	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString("/{")
		builder.WriteString(key)
		builder.WriteString("}")
	}

	return builder.String()
}

type normalizedPathRequest struct {
	State string `param:"state" validate:"required,oneof=active disabled"`
}

func (r *normalizedPathRequest) Normalize() {
	r.State = strings.ToLower(strings.TrimSpace(r.State))
}
