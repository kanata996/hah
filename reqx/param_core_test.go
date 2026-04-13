package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] multiParamValue 的内部边角分支会覆盖 clone/default/required/parse error 的核心契约。

func TestMultiParamValue_InternalBranches(t *testing.T) {
	t.Run("cloneValue falls back to identity when clone is nil", func(t *testing.T) {
		value := newMultiParamValue(paramSpec{}, false, func(values []string) (int, error) {
			return len(values), nil
		}, nil)

		if got := value.cloneValue(7); got != 7 {
			t.Fatalf("cloneValue(7) = %d, want 7", got)
		}
	})

	t.Run("optional missing returns zero nil slice", func(t *testing.T) {
		got, err := Query(newTestQueryRequest("/items"), "tag").Values().Get()
		if err != nil {
			t.Fatalf("Query().Values().Get() error = %v", err)
		}
		if got != nil {
			t.Fatalf("Query().Values().Get() = %#v, want nil", got)
		}
	})

	t.Run("default nil slice is preserved", func(t *testing.T) {
		got, err := Query(newTestQueryRequest("/items"), "tag").Values().Default(nil).Get()
		if err != nil {
			t.Fatalf("Query().Values().Default(nil).Get() error = %v", err)
		}
		if got != nil {
			t.Fatalf("Query().Values().Default(nil).Get() = %#v, want nil", got)
		}
	})

	t.Run("default then required conflict", func(t *testing.T) {
		_, err := Query(newTestQueryRequest("/items"), "tag").Values().Default([]string{"a"}).Required().Get()
		assertUsageErrorContains(t, err, "required and default are mutually exclusive")
	})

	t.Run("required then default conflict", func(t *testing.T) {
		_, err := Query(newTestQueryRequest("/items"), "tag").Values().Required().Default([]string{"a"}).Get()
		assertUsageErrorContains(t, err, "required and default are mutually exclusive")
	})

	t.Run("parse error maps to invalid violation", func(t *testing.T) {
		value := newMultiParamValue(
			paramSpec{
				r:      newTestQueryRequest("/items?tag=a"),
				name:   "tag",
				input:  ViolationInQuery,
				lookup: queryParamValues,
			},
			false,
			func(values []string) ([]string, error) {
				return nil, errors.New("boom")
			},
			nil,
		)

		_, err := value.resolve()
		assertInvalidViolationAt(t, err, "tag", ViolationInQuery)
	})
}

func newTestQueryRequest(target string) *http.Request {
	req := httptest.NewRequest("GET", target, nil)
	return req
}
