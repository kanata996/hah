package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `BindHeaders` 会绑定常见标量 header，并对空输入参数与非法绑定定义做明确校验。
// - [✓] `BindAndValidateHeaders` 会在校验前执行 `Normalize()`，并使用规范化后的 header tag 名称。
// - [✓] `BindAndValidateHeaders` 在绑定失败时优先返回绑定错误，并透传公开空输入参数错误。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Header 绑定应支持常见的标量字段类型。
func TestBindHeaders_BindsSupportedScalarTypes(t *testing.T) {
	type request struct {
		RequestID string `header:"x-request-id"`
		Retry     int    `header:"x-retry"`
		Enabled   bool   `header:"x-enabled"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "req-123")
	req.Header.Set("X-Retry", "2")
	req.Header.Set("X-Enabled", "true")

	var dst request
	if err := BindHeaders(req, &dst); err != nil {
		t.Fatalf("BindHeaders() error = %v", err)
	}
	if dst.RequestID != "req-123" || dst.Retry != 2 || !dst.Enabled {
		t.Fatalf("dst = %#v, want bound scalar header values", dst)
	}
}

// 空输入会在进入 header 绑定前被直接拒绝。
func TestBindHeaders_RejectsNilInputs(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		var dst struct{}
		err := BindHeaders[struct{}](nil, &dst)
		if err == nil {
			t.Fatal("BindHeaders() error = nil")
		}
		if got := err.Error(); got != "reqx: request must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("nil destination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		err := BindHeaders[struct{}](req, nil)
		if err == nil {
			t.Fatal("BindHeaders() error = nil")
		}
		if got := err.Error(); got != "reqx: destination must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})
}

// header 包装器会在绑定前直接拒绝空输入参数。
func TestBindAndValidateHeaders_RejectsNilInputs(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		var dst struct{}
		err := BindAndValidateHeaders[struct{}](nil, &dst)
		if err == nil {
			t.Fatal("BindAndValidateHeaders() error = nil")
		}
		if got := err.Error(); got != "reqx: request must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("nil destination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		err := BindAndValidateHeaders[struct{}](req, nil)
		if err == nil {
			t.Fatal("BindAndValidateHeaders() error = nil")
		}
		if got := err.Error(); got != "reqx: destination must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})
}

// 非法的 header 绑定定义会在构建解码计划时被拒绝。
func TestBindHeaders_RejectsInvalidBindingSchema(t *testing.T) {
	t.Run("unsupported field type", func(t *testing.T) {
		type nested struct {
			Name string
		}
		type request struct {
			Nested nested `header:"x-nested"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Nested", "x")

		var dst request
		err := BindHeaders(req, &dst)
		if err == nil {
			t.Fatal("BindHeaders() error = nil")
		}
		if got := err.Error(); got != `reqx: field "Nested" has unsupported header type reqx.nested` {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("duplicate tagged fields", func(t *testing.T) {
		type request struct {
			RequestID string `header:"x-request-id"`
			TraceID   string `header:"X-Request-Id"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-Id", "req-1")

		var dst request
		err := BindHeaders(req, &dst)
		if err == nil {
			t.Fatal("BindHeaders() error = nil")
		}
		if got := err.Error(); got != `reqx: duplicate header field "X-Request-Id" on RequestID and TraceID` {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("unexported tagged field", func(t *testing.T) {
		type request struct {
			requestID string `header:"x-request-id"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-Id", "req-1")

		var dst request
		_ = dst.requestID
		err := BindHeaders(req, &dst)
		if err == nil {
			t.Fatal("BindHeaders() error = nil")
		}
		if got := err.Error(); got != `reqx: header field "requestID" must be exported` {
			t.Fatalf("error = %q", got)
		}
	})
}

// 绑定后会先做标准化再进入 header 校验。
func TestBindAndValidateHeaders_NormalizesBeforeValidation(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", " req-123 ")

	var dst normalizedHeaderRequest
	if err := BindAndValidateHeaders(req, &dst); err != nil {
		t.Fatalf("BindAndValidateHeaders() error = %v", err)
	}
	if dst.RequestID != "req-123" {
		t.Fatalf("request id = %q, want req-123", dst.RequestID)
	}
}

// Header 校验错误字段名应使用规范化后的 header tag 名称。
func TestBindAndValidateHeaders_UsesCanonicalHeaderTagName(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	var dst struct {
		RequestID string `header:"x-request-id" validate:"required"`
	}
	err := BindAndValidateHeaders(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "X-Request-Id" || violation.In != ViolationInHeader || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
		t.Fatalf("violation = %#v", violation)
	}
}

// header 包装器在绑定失败时不会继续进入校验阶段，也不会污染目标对象。
func TestBindAndValidateHeaders_ReturnsBindingErrorBeforeValidation(t *testing.T) {
	type request struct {
		RequestID string `header:"x-request-id" validate:"required,nospace"`
		Retry     int    `header:"x-retry" validate:"min=1"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "bad value")
	req.Header.Set("X-Retry", "oops")

	dst := request{
		RequestID: "existing value",
		Retry:     2,
	}

	err := BindAndValidateHeaders(req, &dst)
	violation := assertSingleViolation(t, err)
	if violation.Field != "X-Retry" || violation.In != ViolationInHeader || violation.Code != ViolationCodeType || violation.Detail != "must be number" {
		t.Fatalf("violation = %#v", violation)
	}
	if dst.RequestID != "existing value" || dst.Retry != 2 {
		t.Fatalf("dst = %#v, want destination preserved when bind fails", dst)
	}
}

type normalizedHeaderRequest struct {
	RequestID string `header:"x-request-id" validate:"required,nospace"`
}

func (r *normalizedHeaderRequest) Normalize() {
	r.RequestID = strings.TrimSpace(r.RequestID)
}
