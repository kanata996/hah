package reqx

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] Path 入口会为资源标识型 path 参数提供 source-aware required/invalid violation。
// - [✓] PathParam 只保留 path 允许的窄类型面：String、UUID、Int、Int64、Uint、Uint64。
// - [✓] path lookup helper 会维持 PathValue / Pattern wildcard 的公开契约。

func TestPath_SuccessPaths(t *testing.T) {
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

	t.Run("path supported identifier types", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"slug": {"acct_123"},
			"n":    {"7"},
			"n64":  {"9"},
			"u":    {"11"},
			"u64":  {"13"},
		})

		slug, err := Path(req, "slug").String().Required().Get()
		if err != nil || slug != "acct_123" {
			t.Fatalf("Path().String().Get() = (%q, %v), want (acct_123, nil)", slug, err)
		}

		n, err := Path(req, "n").Int().Required().Get()
		if err != nil || n != 7 {
			t.Fatalf("Path().Int().Get() = (%d, %v), want (7, nil)", n, err)
		}

		n64, err := Path(req, "n64").Int64().Required().Get()
		if err != nil || n64 != 9 {
			t.Fatalf("Path().Int64().Get() = (%d, %v), want (9, nil)", n64, err)
		}

		u, err := Path(req, "u").Uint().Required().Get()
		if err != nil || u != 11 {
			t.Fatalf("Path().Uint().Get() = (%d, %v), want (11, nil)", u, err)
		}

		u64, err := Path(req, "u64").Uint64().Required().Get()
		if err != nil || u64 != 13 {
			t.Fatalf("Path().Uint64().Get() = (%d, %v), want (13, nil)", u64, err)
		}
	})
}

func TestPath_RequiredAndInvalidViolations(t *testing.T) {
	t.Run("required missing path", func(t *testing.T) {
		req := requestWithPathParams(nil)

		_, err := Path(req, "id").String().Required().Get()
		assertRequiredViolationAt(t, err, "id", ViolationInPath)
	})

	t.Run("invalid path uuid", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"id": {"not-a-uuid"}})

		_, err := Path(req, "id").UUID().Get()
		assertInvalidViolationAt(t, err, "id", ViolationInPath)
	})

	t.Run("invalid path int", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"id": {"oops"}})

		_, err := Path(req, "id").Int().Get()
		assertInvalidViolationAt(t, err, "id", ViolationInPath)
	})
}

func TestPathBuilder_UsageAndOptionalBehavior(t *testing.T) {
	t.Run("nil path builder", func(t *testing.T) {
		var p *PathParam

		_, err := p.String().Get()
		assertUsageErrorContains(t, err, "param builder must not be nil")
	})

	t.Run("zero path builder", func(t *testing.T) {
		_, err := (&PathParam{}).String().Get()
		assertUsageErrorContains(t, err, "param builder must be created with Path or Query")
	})
}

func TestPathLookupHelpers_Branches(t *testing.T) {
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
