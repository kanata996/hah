package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] Int / Int64 / Uint / Uint64 / Float64 builder 会在 present-empty query 值下维持公开零值语义，而不是把空值当成缺失。
// - [✓] Numeric builder 会覆盖代表性的 default 成功、range 失败、parse 失败与 usage error 边界。
// - [✓] Numeric builder 的代表性 Default(...) / Check(...) 失败路径会在自身 API 上直接校验，保证默认值不会绕过后续约束。
// - [✓] Fuzz 评估：本文件当前只补 numeric builder 规格回归，不新增 fuzz；原因是未引入新的 query/path 解析逻辑或输入状态空间。

func TestIntParam_ValidationAndRangeErrors(t *testing.T) {
	t.Run("boundary success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=0", nil), "page").Int().
			Required().
			Min(0).
			Max(0).
			Get()
		if err != nil {
			t.Fatalf("Int().Required().Min().Max().Get() error = %v", err)
		}
		if got != 0 {
			t.Fatalf("page = %d, want 0", got)
		}
	})

	t.Run("empty string parses zero when required", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=", nil), "page").Int().Required().Get()
		if err != nil || got != 0 {
			t.Fatalf("Int().Required().Get() = (%d, %v), want (0, nil)", got, err)
		}
	})

	t.Run("default min invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().
			Default(1).
			Min(2).
			Get()
		assertInvalidViolationAt(t, err, "page", ViolationInQuery)
	})

	t.Run("min invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=1", nil), "page").Int().Min(2).Get()
		assertInvalidViolationAt(t, err, "page", ViolationInQuery)
	})

	t.Run("max invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=3", nil), "page").Int().Max(2).Get()
		assertInvalidViolationAt(t, err, "page", ViolationInQuery)
	})

	t.Run("max then min conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=3", nil), "page").Int().Max(1).Min(2).Get()
		assertUsageErrorContains(t, err, "minimum must be less than or equal to maximum")
	})
}

func TestInt64Param_ValidationAndRangeErrors(t *testing.T) {
	t.Run("int64 boundary success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=0", nil), "v").Int64().
			Required().
			Min(0).
			Max(0).
			Check(func(value int64) error { return nil }).
			Get()
		if err != nil {
			t.Fatalf("Int64().Required().Min().Max().Check().Get() error = %v", err)
		}
		if got != 0 {
			t.Fatalf("int64 = %d, want 0", got)
		}
	})

	t.Run("int64 default success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "v").Int64().Default(9).Get()
		if err != nil || got != 9 {
			t.Fatalf("Int64().Default().Get() = (%d, %v), want (9, nil)", got, err)
		}
	})

	t.Run("int64 empty string parses zero when required", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=", nil), "v").Int64().Required().Get()
		if err != nil || got != 0 {
			t.Fatalf("Int64().Required().Get() = (%d, %v), want (0, nil)", got, err)
		}
	})

	t.Run("int64 default check invalid uses custom detail", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "v").Int64().
			Default(9).
			Check(func(value int64) error {
				return errors.New("default int64 must be rejected")
			}).
			Get()
		assertViolation(t, err, Violation{
			Field:  "v",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "default int64 must be rejected",
		})
	})

	t.Run("int64 min invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=1", nil), "v").Int64().Min(2).Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)
	})

	t.Run("int64 max invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Int64().Max(2).Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)
	})

	t.Run("int64 max then min conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Int64().Max(1).Min(2).Get()
		assertUsageErrorContains(t, err, "minimum must be less than or equal to maximum")
	})

	t.Run("int64 min then max conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Int64().Min(2).Max(1).Get()
		assertUsageErrorContains(t, err, "maximum must be greater than or equal to minimum")
	})

	t.Run("int64 parse invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=oops", nil), "v").Int64().Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)
	})
}

func TestUintParam_ValidationAndRangeErrors(t *testing.T) {
	t.Run("uint boundary success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=0", nil), "v").Uint().
			Required().
			Min(0).
			Max(0).
			Check(func(value uint) error { return nil }).
			Get()
		if err != nil {
			t.Fatalf("Uint().Required().Min().Max().Check().Get() error = %v", err)
		}
		if got != 0 {
			t.Fatalf("uint = %d, want 0", got)
		}
	})

	t.Run("uint default success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "v").Uint().Default(7).Get()
		if err != nil || got != 7 {
			t.Fatalf("Uint().Default().Get() = (%d, %v), want (7, nil)", got, err)
		}
	})

	t.Run("uint empty string parses zero when required", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=", nil), "v").Uint().Required().Get()
		if err != nil || got != 0 {
			t.Fatalf("Uint().Required().Get() = (%d, %v), want (0, nil)", got, err)
		}
	})

	t.Run("uint default check invalid uses custom detail", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "v").Uint().
			Default(7).
			Check(func(value uint) error {
				return errors.New("default uint must be rejected")
			}).
			Get()
		assertViolation(t, err, Violation{
			Field:  "v",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "default uint must be rejected",
		})
	})

	t.Run("uint min invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=1", nil), "v").Uint().Min(2).Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)
	})

	t.Run("uint max invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Uint().Max(2).Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)
	})

	t.Run("uint max then min conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Uint().Max(1).Min(2).Get()
		assertUsageErrorContains(t, err, "minimum must be less than or equal to maximum")
	})

	t.Run("uint min then max conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Uint().Min(2).Max(1).Get()
		assertUsageErrorContains(t, err, "maximum must be greater than or equal to minimum")
	})

	t.Run("uint parse invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=-1", nil), "v").Uint().Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)
	})
}

func TestUint64Param_ValidationAndRangeErrors(t *testing.T) {
	t.Run("uint64 boundary success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=0", nil), "v").Uint64().
			Required().
			Min(0).
			Max(0).
			Check(func(value uint64) error { return nil }).
			Get()
		if err != nil {
			t.Fatalf("Uint64().Required().Min().Max().Check().Get() error = %v", err)
		}
		if got != 0 {
			t.Fatalf("uint64 = %d, want 0", got)
		}
	})

	t.Run("uint64 default success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "v").Uint64().Default(11).Get()
		if err != nil || got != 11 {
			t.Fatalf("Uint64().Default().Get() = (%d, %v), want (11, nil)", got, err)
		}
	})

	t.Run("uint64 empty string parses zero when required", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=", nil), "v").Uint64().Required().Get()
		if err != nil || got != 0 {
			t.Fatalf("Uint64().Required().Get() = (%d, %v), want (0, nil)", got, err)
		}
	})

	t.Run("uint64 default check invalid uses custom detail", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "v").Uint64().
			Default(11).
			Check(func(value uint64) error {
				return errors.New("default uint64 must be rejected")
			}).
			Get()
		assertViolation(t, err, Violation{
			Field:  "v",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "default uint64 must be rejected",
		})
	})

	t.Run("uint64 min invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=1", nil), "v").Uint64().Min(2).Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)
	})

	t.Run("uint64 max invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Uint64().Max(2).Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)
	})

	t.Run("uint64 max then min conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Uint64().Max(1).Min(2).Get()
		assertUsageErrorContains(t, err, "minimum must be less than or equal to maximum")
	})

	t.Run("uint64 min then max conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Uint64().Min(2).Max(1).Get()
		assertUsageErrorContains(t, err, "maximum must be greater than or equal to minimum")
	})

	t.Run("uint64 parse invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=-1", nil), "v").Uint64().Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)
	})
}

func TestFloat64Param_ValidationAndRangeErrors(t *testing.T) {
	t.Run("float64 boundary success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?price=0", nil), "price").Float64().
			Required().
			Min(0).
			Max(0).
			Check(func(value float64) error { return nil }).
			Get()
		if err != nil {
			t.Fatalf("Float64().Required().Min().Max().Check().Get() error = %v", err)
		}
		if got != 0 {
			t.Fatalf("float64 = %v, want 0", got)
		}
	})

	t.Run("float64 default success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "price").Float64().Default(1.25).Get()
		if err != nil || got != 1.25 {
			t.Fatalf("Float64().Default().Get() = (%v, %v), want (1.25, nil)", got, err)
		}
	})

	t.Run("float64 empty string parses zero when required", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?price=", nil), "price").Float64().Required().Get()
		if err != nil || got != 0 {
			t.Fatalf("Float64().Required().Get() = (%v, %v), want (0, nil)", got, err)
		}
	})

	t.Run("float64 default check invalid uses custom detail", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "price").Float64().
			Default(1.25).
			Check(func(value float64) error {
				return errors.New("default float64 must be rejected")
			}).
			Get()
		assertViolation(t, err, Violation{
			Field:  "price",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "default float64 must be rejected",
		})
	})

	t.Run("float64 min invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?price=1", nil), "price").Float64().Min(2).Get()
		assertInvalidViolationAt(t, err, "price", ViolationInQuery)
	})

	t.Run("float64 max invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?price=3", nil), "price").Float64().Max(2).Get()
		assertInvalidViolationAt(t, err, "price", ViolationInQuery)
	})

	t.Run("float64 max then min conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?price=3", nil), "price").Float64().Max(1).Min(2).Get()
		assertUsageErrorContains(t, err, "minimum must be less than or equal to maximum")
	})

	t.Run("float64 min then max conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?price=3", nil), "price").Float64().Min(2).Max(1).Get()
		assertUsageErrorContains(t, err, "maximum must be greater than or equal to minimum")
	})

	t.Run("float64 parse invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?price=oops", nil), "price").Float64().Get()
		assertInvalidViolationAt(t, err, "price", ViolationInQuery)
	})
}
