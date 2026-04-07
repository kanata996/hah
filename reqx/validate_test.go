package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] 内部 `validate(..., sourceX)` 会使用来源专属字段别名和 `in` 值。
// - [✓] 内部 `invalidFieldsError` 会规范化 violation 默认值并写成稳定 HTTP 错误。
// - [✓] 内部 `validate(sourceRequest)` 会按 param/query/json/header/plain 选择字段名，但统一以 `request` 作为 `in`。
// - [✓] 内部 `validate(sourceRequest)` 的空目标错误契约会透传当前包装器的参数校验错误。

import "testing"

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

// invalidFieldsError 会把 violation 默认值补齐后写成统一错误。
func TestInvalidFieldsError_NormalizesViolation(t *testing.T) {
	err := invalidFieldsError([]Violation{{Field: "name"}})
	violation := assertSingleViolation(t, err)
	if violation.Field != "name" || violation.Code != ViolationCodeInvalid || violation.Detail != "is invalid" {
		t.Fatalf("violation = %#v", violation)
	}
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
	err := validate[struct{}](nil, sourceRequest)
	if err == nil {
		t.Fatal("validate() error = nil")
	}
	if got := err.Error(); got != "reqx: target must be a non-nil pointer to struct" {
		t.Fatalf("error = %q, want reqx: target must be a non-nil pointer to struct", got)
	}
}
