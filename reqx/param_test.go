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
// - [✓] Path / Query builder 会为单参数 path/query 读取提供 source-aware required/invalid violation。
// - [✓] Param builder typed getter 会覆盖默认值、范围/枚举/正则/自定义 Check 与 usage error 边界。
// - [✓] Time / UnixTime / UnixMilliTime builder 会覆盖公开成功路径、失败路径与区间约束。
// - [✓] 内部参数 helper 会维持 path/query 值提取与 path wildcard 解析的稳定契约。

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

	t.Run("present empty string remains empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?name=", nil)

		got, err := Query(req, "name").String().Get()
		if err != nil {
			t.Fatalf("Query().String().Get() error = %v", err)
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

func TestTimeParam_UnixBuilders(t *testing.T) {
	t.Run("unix time success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?sec=1712910600", nil)

		sec, err := Query(req, "sec").UnixTime().Get()
		if err != nil {
			t.Fatalf("UnixTime().Get() error = %v", err)
		}
		if sec.UTC().Format(time.RFC3339) != "2024-04-12T08:30:00Z" {
			t.Fatalf("unix sec = %q, want 2024-04-12T08:30:00Z", sec.UTC().Format(time.RFC3339))
		}
	})

	t.Run("unix milli success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?ms=1712910600123", nil)

		ms, err := Query(req, "ms").UnixMilliTime().Get()
		if err != nil {
			t.Fatalf("UnixMilliTime().Get() error = %v", err)
		}
		if ms.UTC().Format(time.RFC3339Nano) != "2024-04-12T08:30:00.123Z" {
			t.Fatalf("unix ms = %q, want 2024-04-12T08:30:00.123Z", ms.UTC().Format(time.RFC3339Nano))
		}
	})

	t.Run("unix time required equal boundaries and check success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?sec=1712910600", nil)
		want := time.Date(2024, 4, 12, 8, 30, 0, 0, time.UTC)

		got, err := Query(req, "sec").UnixTime().
			Required().
			After(want).
			Before(want).
			Check(func(value time.Time) error {
				if !value.Equal(want) {
					return errors.New("unexpected unix time")
				}
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("UnixTime().Required().After().Before().Check().Get() error = %v", err)
		}
		if !got.Equal(want) {
			t.Fatalf("unix sec = %v, want %v", got, want)
		}
	})

	t.Run("unix milli default when missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)
		want := time.Date(2024, 4, 12, 8, 30, 0, 123*int(time.Millisecond), time.UTC)

		got, err := Query(req, "ms").UnixMilliTime().Default(want).Get()
		if err != nil {
			t.Fatalf("UnixMilliTime().Default().Get() error = %v", err)
		}
		if !got.Equal(want) {
			t.Fatalf("unix ms = %v, want %v", got, want)
		}
	})

	t.Run("unix time required missing returns query violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)

		_, err := Query(req, "sec").UnixTime().Required().Get()
		assertRequiredViolationAt(t, err, "sec", ViolationInQuery)
	})

	t.Run("unix milli before invalid returns query violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?ms=1712910600123", nil)

		_, err := Query(req, "ms").UnixMilliTime().
			Before(time.Date(2024, 4, 12, 8, 30, 0, 122*int(time.Millisecond), time.UTC)).
			Get()
		assertInvalidViolationAt(t, err, "ms", ViolationInQuery)
	})

	t.Run("unix time invalid width returns query violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?sec=123", nil)

		_, err := Query(req, "sec").UnixTime().Get()
		assertInvalidViolationAt(t, err, "sec", ViolationInQuery)
	})

	t.Run("unix milli non numeric returns query violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?ms=abcdefghijklm", nil)

		_, err := Query(req, "ms").UnixMilliTime().Get()
		assertInvalidViolationAt(t, err, "ms", ViolationInQuery)
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

func TestStringParam_ValidationAndUsageErrors(t *testing.T) {
	t.Run("default and check success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "name").String().
			Default("kanata").
			Check(func(value string) error {
				if value != "kanata" {
					return errors.New("unexpected name")
				}
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("String().Default().Check().Get() error = %v", err)
		}
		if got != "kanata" {
			t.Fatalf("name = %q, want kanata", got)
		}
	})

	t.Run("min and max len success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=go", nil), "name").String().MinLen(2).MaxLen(2).Get()
		if err != nil {
			t.Fatalf("String().MinLen().MaxLen().Get() error = %v", err)
		}
		if got != "go" {
			t.Fatalf("name = %q, want go", got)
		}
	})

	t.Run("min len invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=g", nil), "name").String().MinLen(2).Get()
		assertInvalidViolationAt(t, err, "name", ViolationInQuery)
	})

	t.Run("max len invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=golang", nil), "name").String().MaxLen(3).Get()
		assertInvalidViolationAt(t, err, "name", ViolationInQuery)
	})

	t.Run("negative min len", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=go", nil), "name").String().MinLen(-1).Get()
		assertUsageErrorContains(t, err, "minimum length must be >= 0")
	})

	t.Run("negative max len", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=go", nil), "name").String().MaxLen(-1).Get()
		assertUsageErrorContains(t, err, "maximum length must be >= 0")
	})

	t.Run("one-of invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?status=pending", nil), "status").String().OneOf("open", "closed").Get()
		assertInvalidViolationAt(t, err, "status", ViolationInQuery)
	})

	t.Run("nil match pattern", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?slug=go", nil), "slug").String().Match(nil).Get()
		assertUsageErrorContains(t, err, "match pattern must not be nil")
	})

	t.Run("match invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?slug=GO", nil), "slug").String().Match(regexp.MustCompile(`^[a-z]+$`)).Get()
		assertInvalidViolationAt(t, err, "slug", ViolationInQuery)
	})
}

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

func TestTypedParams_ValidationAndRangeErrors(t *testing.T) {
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

	t.Run("bool required and check success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?active=true", nil), "active").Bool().
			Required().
			Check(func(value bool) error { return nil }).
			Get()
		if err != nil {
			t.Fatalf("Bool().Required().Check().Get() error = %v", err)
		}
		if !got {
			t.Fatal("bool = false, want true")
		}
	})

	t.Run("bool default success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "active").Bool().Default(true).Get()
		if err != nil || !got {
			t.Fatalf("Bool().Default().Get() = (%v, %v), want (true, nil)", got, err)
		}
	})

	t.Run("bool parse invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?active=maybe", nil), "active").Bool().Get()
		assertInvalidViolationAt(t, err, "active", ViolationInQuery)
	})

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

	t.Run("uuid default success", func(t *testing.T) {
		want := uuid.New()

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "id").UUID().Default(want).Get()
		if err != nil || got != want {
			t.Fatalf("UUID().Default().Get() = (%v, %v), want (%v, nil)", got, err, want)
		}
	})

	t.Run("uuid check success", func(t *testing.T) {
		want := uuid.New()

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?id="+want.String(), nil), "id").UUID().
			Check(func(value uuid.UUID) error { return nil }).
			Get()
		if err != nil || got != want {
			t.Fatalf("UUID().Check().Get() = (%v, %v), want (%v, nil)", got, err, want)
		}
	})

	t.Run("uuid parse invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?id=oops", nil), "id").UUID().Get()
		assertInvalidViolationAt(t, err, "id", ViolationInQuery)
	})

	t.Run("time required range and check success", func(t *testing.T) {
		from := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)
		at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
		to := time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+at.Format(time.RFC3339), nil), "at").Time().
			Required().
			After(from).
			Before(to).
			Check(func(value time.Time) error { return nil }).
			Get()
		if err != nil || !got.Equal(at) {
			t.Fatalf("Time().Required().After().Before().Check().Get() = (%v, %v), want (%v, nil)", got, err, at)
		}
	})

	t.Run("time default success", func(t *testing.T) {
		at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "at").Time().Default(at).Get()
		if err != nil || !got.Equal(at) {
			t.Fatalf("Time().Default().Get() = (%v, %v), want (%v, nil)", got, err, at)
		}
	})

	t.Run("time after invalid", func(t *testing.T) {
		from := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)
		at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+from.Format(time.RFC3339), nil), "at").Time().After(at).Get()
		assertInvalidViolationAt(t, err, "at", ViolationInQuery)
	})

	t.Run("time before invalid", func(t *testing.T) {
		at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
		to := time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+to.Format(time.RFC3339), nil), "at").Time().Before(at).Get()
		assertInvalidViolationAt(t, err, "at", ViolationInQuery)
	})

	t.Run("time before then after conflict", func(t *testing.T) {
		from := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)
		to := time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+to.Format(time.RFC3339), nil), "at").Time().Before(from).After(to).Get()
		assertUsageErrorContains(t, err, "after time must be less than or equal to before time")
	})

	t.Run("time after then before conflict", func(t *testing.T) {
		from := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)
		to := time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+to.Format(time.RFC3339), nil), "at").Time().After(to).Before(from).Get()
		assertUsageErrorContains(t, err, "before time must be greater than or equal to after time")
	})

	t.Run("time parse invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at=not-a-time", nil), "at").Time().Get()
		assertInvalidViolationAt(t, err, "at", ViolationInQuery)
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
