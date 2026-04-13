package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] Path / Query 入口会为单参数 path/query 读取提供 source-aware required/invalid violation，并对重复 query key 维持首值语义。
// - [✓] Param builder 的通用 usage error 与 optional 行为维持稳定契约。
// - [✓] 内部参数 helper 会维持 path/query 值提取与 path wildcard 解析的稳定契约。
// - [✓] Fuzz 评估：本文件当前只补公开入口与 lookup 契约回归，不新增 fuzz；原因是未引入新的 query/path 解析逻辑或状态空间。

func TestPathAndQuery_SuccessPaths(t *testing.T) {
	t.Run("path uuid required", func(t *testing.T) {
		want := uuid.New()
		req := requestWithPathParams(map[string][]string{"id": {want.String()}})

		got, err := Path(req, "id").UUID().Required().Get()
		if err != nil {
			t.Fatalf("Path().UUID().Required().Get() error = %v", err)
		}
		if got != want {
			t.Fatalf("uuid = %v, want %v", got, want)
		}
	})

	t.Run("query int default min max", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)

		got, err := Query(req, "page").Int().Default(20).Min(1).Max(100).Get()
		if err != nil {
			t.Fatalf("Query().Int().Default().Min().Max().Get() error = %v", err)
		}
		if got != 20 {
			t.Fatalf("page = %d, want 20", got)
		}
	})

	t.Run("query duplicate key uses first value", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=5&page=9", nil)

		got, err := Query(req, "page").Int().Get()
		if err != nil {
			t.Fatalf("Query().Int().Get() error = %v", err)
		}
		if got != 5 {
			t.Fatalf("page = %d, want 5", got)
		}
	})

	t.Run("query string one-of match", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?status=open", nil)

		got, err := Query(req, "status").String().
			Required().
			OneOf("open", "closed").
			Match(regexp.MustCompile(`^[a-z]+$`)).
			Get()
		if err != nil {
			t.Fatalf("Query().String().OneOf().Match().Get() error = %v", err)
		}
		if got != "open" {
			t.Fatalf("status = %q, want open", got)
		}
	})

	t.Run("query time before after", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?at=2026-04-13T10:00:00Z", nil)

		got, err := Query(req, "at").Time().
			After(time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)).
			Before(time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)).
			Get()
		if err != nil {
			t.Fatalf("Query().Time().After().Before().Get() error = %v", err)
		}
		if got.UTC().Format(time.RFC3339) != "2026-04-13T10:00:00Z" {
			t.Fatalf("time = %q, want 2026-04-13T10:00:00Z", got.UTC().Format(time.RFC3339))
		}
	})
}

func TestPathAndQuery_RequiredAndInvalidViolations(t *testing.T) {
	t.Run("required missing path", func(t *testing.T) {
		req := requestWithPathParams(nil)

		_, err := Path(req, "id").String().Required().Get()
		assertRequiredViolationAt(t, err, "id", ViolationInPath)
	})

	t.Run("required missing query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)

		_, err := Query(req, "page").Int().Required().Get()
		assertRequiredViolationAt(t, err, "page", ViolationInQuery)
	})

	t.Run("invalid path uuid", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"id": {"not-a-uuid"}})

		_, err := Path(req, "id").UUID().Get()
		assertInvalidViolationAt(t, err, "id", ViolationInPath)
	})

	t.Run("invalid query int", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)

		_, err := Query(req, "page").Int().Get()
		assertInvalidViolationAt(t, err, "page", ViolationInQuery)
	})

	t.Run("duplicate query validates first value", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=oops&page=3", nil)

		_, err := Query(req, "page").Int().Get()
		assertInvalidViolationAt(t, err, "page", ViolationInQuery)
	})

	t.Run("check failure uses custom detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=3", nil)

		_, err := Query(req, "page").Int().Check(func(value int) error {
			if value%2 != 0 {
				return errors.New("must be even")
			}
			return nil
		}).Get()
		assertViolation(t, err, Violation{
			Field:  "page",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "must be even",
		})
	})

	t.Run("present empty string remains empty when required", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?name=", nil)

		got, err := Query(req, "name").String().Required().Get()
		if err != nil {
			t.Fatalf("Query().String().Required().Get() error = %v", err)
		}
		if got != "" {
			t.Fatalf("name = %q, want empty string", got)
		}
	})
}

func TestPathAndQuery_UsageErrors(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		_, err := Query(nil, "page").Int().Get()
		assertUsageErrorContains(t, err, "request must not be nil")
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), " ").Int().Get()
		assertUsageErrorContains(t, err, "parameter name must not be empty")
	})

	t.Run("required default conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().Required().Default(1).Get()
		assertUsageErrorContains(t, err, "required and default are mutually exclusive")
	})

	t.Run("min greater than max", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=3", nil), "page").Int().Min(10).Max(2).Get()
		assertUsageErrorContains(t, err, "greater than or equal to minimum")
	})

	t.Run("empty one-of", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?status=open", nil), "status").String().OneOf().Get()
		assertUsageErrorContains(t, err, "one-of values must not be empty")
	})

	t.Run("nil check", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=3", nil), "page").Int().Check(nil).Get()
		assertUsageErrorContains(t, err, "check must not be nil")
	})
}

func TestParamLookupHelpers_Branches(t *testing.T) {
	t.Run("path helper", func(t *testing.T) {
		if values, ok := pathParamValues(nil, "id"); ok || values != nil {
			t.Fatalf("pathParamValues(nil) = (%v, %v), want (nil, false)", values, ok)
		}

		reqWithValue := requestWithPathParams(map[string][]string{
			"id": {"u_1"},
		})
		if values, ok := pathParamValues(reqWithValue, "id"); !ok || len(values) != 1 || values[0] != "u_1" {
			t.Fatalf("pathParamValues(value) = (%v, %v), want ([u_1], true)", values, ok)
		}

		reqWithEmpty := requestWithPathParams(map[string][]string{
			"id": {""},
		})
		if values, ok := pathParamValues(reqWithEmpty, "id"); !ok || len(values) != 1 || values[0] != "" {
			t.Fatalf("pathParamValues(empty) = (%v, %v), want ([\"\"], true)", values, ok)
		}

		reqMissing := httptest.NewRequest(http.MethodGet, "/accounts", nil)
		reqMissing.Pattern = "/accounts"
		if values, ok := pathParamValues(reqMissing, "id"); ok || values != nil {
			t.Fatalf("pathParamValues(missing) = (%v, %v), want (nil, false)", values, ok)
		}
	})

	t.Run("query helper", func(t *testing.T) {
		if values, ok := queryParamValues(nil, "page"); ok || values != nil {
			t.Fatalf("queryParamValues(nil) = (%v, %v), want (nil, false)", values, ok)
		}

		req := httptest.NewRequest(http.MethodGet, "/items?page=5&tag=a&tag=b", nil)

		t.Run("single value", func(t *testing.T) {
			values, ok := queryParamValues(req, "page")
			if !ok || len(values) != 1 || values[0] != "5" {
				t.Fatalf("queryParamValues(page) = (%v, %v), want ([5], true)", values, ok)
			}
		})

		t.Run("multiple values", func(t *testing.T) {
			values, ok := queryParamValues(req, "tag")
			if !ok || len(values) != 2 || values[0] != "a" || values[1] != "b" {
				t.Fatalf("queryParamValues(tag) = (%v, %v), want ([a b], true)", values, ok)
			}
		})

		t.Run("missing key", func(t *testing.T) {
			values, ok := queryParamValues(req, "missing")
			if ok || values != nil {
				t.Fatalf("queryParamValues(missing) = (%v, %v), want (nil, false)", values, ok)
			}
		})

		t.Run("empty value", func(t *testing.T) {
			emptyReq := httptest.NewRequest(http.MethodGet, "/items?page=", nil)
			values, ok := queryParamValues(emptyReq, "page")
			if !ok || len(values) != 1 || values[0] != "" {
				t.Fatalf("queryParamValues(empty) = (%v, %v), want ([\"\"], true)", values, ok)
			}
		})
	})

	t.Run("path wildcard names", func(t *testing.T) {
		tests := []struct {
			name    string
			pattern string
			want    []string
		}{
			{name: "blank", pattern: "   ", want: nil},
			{name: "no wildcard", pattern: "/accounts", want: []string{}},
			{name: "basic", pattern: "/accounts/{id}", want: []string{"id"}},
			{name: "with method prefix", pattern: "GET /accounts/{id}", want: []string{"id"}},
			{name: "catch all", pattern: "/files/{path...}", want: []string{"path"}},
			{name: "typed wildcard", pattern: "/accounts/{id:[0-9]+}", want: []string{"id"}},
			{name: "skip dollar", pattern: "/{$}", want: []string{}},
			{name: "malformed", pattern: "/accounts/{id", want: []string{}},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := pathWildcardNames(tc.pattern); !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("pathWildcardNames(%q) = %#v, want %#v", tc.pattern, got, tc.want)
				}
			})
		}
	})
}

func TestParamBuilder_UsageAndOptionalBehavior(t *testing.T) {
	t.Run("nil builder", func(t *testing.T) {
		var p *Param

		_, err := p.String().Get()
		assertUsageErrorContains(t, err, "param builder must not be nil")
	})

	t.Run("zero builder", func(t *testing.T) {
		_, err := (&Param{}).String().Get()
		assertUsageErrorContains(t, err, "param builder must be created with Path or Query")
	})

	t.Run("missing optional returns zero", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "name").String().Get()
		if err != nil {
			t.Fatalf("Query().String().Get() error = %v", err)
		}
		if got != "" {
			t.Fatalf("name = %q, want empty string", got)
		}
	})

	t.Run("default then required conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "name").String().Default("kanata").Required().Get()
		assertUsageErrorContains(t, err, "required and default are mutually exclusive")
	})
}

func assertUsageErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want usage error")
	}
	assertNotHTTPError(t, err)
	if got := err.Error(); !strings.Contains(got, want) {
		t.Fatalf("error = %q, want to contain %q", got, want)
	}
}

func assertViolation(t *testing.T, err error, want Violation) {
	t.Helper()

	if got := assertSingleViolation(t, err); got != want {
		t.Fatalf("violation = %#v, want %#v", got, want)
	}
}

func assertInvalidViolationAt(t *testing.T, err error, field, in string) {
	t.Helper()

	assertViolation(t, err, Violation{
		Field:  field,
		In:     in,
		Code:   ViolationCodeInvalid,
		Detail: "is invalid",
	})
}

func assertRequiredViolationAt(t *testing.T, err error, field, in string) {
	t.Helper()

	assertViolation(t, err, Violation{
		Field:  field,
		In:     in,
		Code:   ViolationCodeRequired,
		Detail: "is required",
	})
}
