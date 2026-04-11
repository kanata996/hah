package reqx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

// 测试清单：
// - [✓] ValueBinder 会为 path/query 单字段绑定提供 source-aware required/invalid violation。
// - [✓] ValueBinder 的 typed shorthand 覆盖常见标量、切片、自定义类型、UUID 与时间 helper 的成功路径。
// - [✓] 代表性 shorthand Int / MustString 与底层 Bind / MustBind 保持一致的 invalid、required 与 fail-fast 公开语义。
// - [✓] ValueBinder 默认 fail-fast，可按需聚合多个 violation。
// - [✓] ValueBinder 会把 nil request / 非法目标视为 usage error，而不是 HTTP violation。
// - [✓] MustBind 在值存在但无效时返回 invalid violation，而非 required。
// - [✓] 各类型 shorthand（int64/uint/bool/float64/uuid/slice/自定义 unmarshaler）的 invalid-value 反面路径。
// - [✓] 空字符串参数（?page=）与完全缺失参数的语义区分。
// - [✓] BindError 返回首个 violation，BindErrors 返回完整 violation 列表。
// - [✓] FailFast(true) 显式切换能从聚合模式恢复 fail-fast。
// - [✓] UnixTime/UnixMilliTime 的边界值（epoch 零、负数时间戳）。

func TestPathValuesBinder_BindsSupportedTargets(t *testing.T) {
	want := uuid.New()
	req := requestWithPathParams(map[string][]string{
		"id":   {want.String()},
		"name": {"kanata"},
	})

	var (
		id   uuid.UUID
		name string
	)
	err := PathValuesBinder(req).
		MustBind("id", &id).
		Bind("name", &name).
		BindErrors()
	if err != nil {
		t.Fatalf("PathValuesBinder().BindErrors() error = %v", err)
	}
	if id != want {
		t.Fatalf("id = %v, want %v", id, want)
	}
	if name != "kanata" {
		t.Fatalf("name = %q, want kanata", name)
	}
}

func TestValueBinder_MissingRequiredReturnsViolation(t *testing.T) {
	tests := []struct {
		name string
		bind func() error
		want Violation
	}{
		{
			name: "path",
			bind: func() error {
				req := requestWithPathParams(map[string][]string{
					"other": {"1"},
				})
				var id int
				return PathValuesBinder(req).MustBind("id", &id).BindErrors()
			},
			want: Violation{
				Field:  "id",
				In:     ViolationInPath,
				Code:   ViolationCodeRequired,
				Detail: "is required",
			},
		},
		{
			name: "query",
			bind: func() error {
				req := httptest.NewRequest(http.MethodGet, "/items", nil)
				var page int
				return QueryParamsBinder(req).MustBind("page", &page).BindErrors()
			},
			want: Violation{
				Field:  "page",
				In:     ViolationInQuery,
				Code:   ViolationCodeRequired,
				Detail: "is required",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			violation := assertSingleViolation(t, tc.bind())
			if violation != tc.want {
				t.Fatalf("violation = %#v, want %#v", violation, tc.want)
			}
		})
	}
}

func TestValueBinder_InvalidValueReturnsViolation(t *testing.T) {
	tests := []struct {
		name string
		bind func() error
		want Violation
	}{
		{
			name: "path",
			bind: func() error {
				req := requestWithPathParams(map[string][]string{
					"id": {"oops"},
				})
				var id int
				return PathValuesBinder(req).Bind("id", &id).BindErrors()
			},
			want: Violation{
				Field:  "id",
				In:     ViolationInPath,
				Code:   ViolationCodeInvalid,
				Detail: "is invalid",
			},
		},
		{
			name: "query",
			bind: func() error {
				req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)
				var page int
				return QueryParamsBinder(req).Bind("page", &page).BindErrors()
			},
			want: Violation{
				Field:  "page",
				In:     ViolationInQuery,
				Code:   ViolationCodeInvalid,
				Detail: "is invalid",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			violation := assertSingleViolation(t, tc.bind())
			if violation != tc.want {
				t.Fatalf("violation = %#v, want %#v", violation, tc.want)
			}
		})
	}
}

func TestValueBinder_MustBindInvalidValueReturnsInvalidNotRequired(t *testing.T) {
	tests := []struct {
		name string
		bind func() error
		want Violation
	}{
		{
			name: "path MustBind with invalid int",
			bind: func() error {
				req := requestWithPathParams(map[string][]string{
					"id": {"oops"},
				})
				var id int
				return PathValuesBinder(req).MustBind("id", &id).BindErrors()
			},
			want: Violation{
				Field:  "id",
				In:     ViolationInPath,
				Code:   ViolationCodeInvalid,
				Detail: "is invalid",
			},
		},
		{
			name: "query MustBind with invalid int",
			bind: func() error {
				req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)
				var page int
				return QueryParamsBinder(req).MustBind("page", &page).BindErrors()
			},
			want: Violation{
				Field:  "page",
				In:     ViolationInQuery,
				Code:   ViolationCodeInvalid,
				Detail: "is invalid",
			},
		},
		{
			name: "path MustInt with invalid value",
			bind: func() error {
				req := requestWithPathParams(map[string][]string{
					"id": {"abc"},
				})
				var id int
				return PathValuesBinder(req).MustInt("id", &id).BindErrors()
			},
			want: Violation{
				Field:  "id",
				In:     ViolationInPath,
				Code:   ViolationCodeInvalid,
				Detail: "is invalid",
			},
		},
		{
			name: "query MustUUID with invalid value",
			bind: func() error {
				req := httptest.NewRequest(http.MethodGet, "/items?id=not-a-uuid", nil)
				var id uuid.UUID
				return QueryParamsBinder(req).MustUUID("id", &id).BindErrors()
			},
			want: Violation{
				Field:  "id",
				In:     ViolationInQuery,
				Code:   ViolationCodeInvalid,
				Detail: "is invalid",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			violation := assertSingleViolation(t, tc.bind())
			if violation != tc.want {
				t.Fatalf("violation = %#v, want %#v", violation, tc.want)
			}
		})
	}
}

func TestValueBinder_InvalidValuePerType(t *testing.T) {
	tests := []struct {
		name  string
		query string
		bind  func(req *http.Request) error
		field string
	}{
		{
			name:  "int64 invalid",
			query: "/items?v=abc",
			bind: func(req *http.Request) error {
				var v int64
				return QueryParamsBinder(req).Int64("v", &v).BindErrors()
			},
			field: "v",
		},
		{
			name:  "uint negative",
			query: "/items?v=-1",
			bind: func(req *http.Request) error {
				var v uint
				return QueryParamsBinder(req).Uint("v", &v).BindErrors()
			},
			field: "v",
		},
		{
			name:  "bool invalid",
			query: "/items?v=maybe",
			bind: func(req *http.Request) error {
				var v bool
				return QueryParamsBinder(req).Bool("v", &v).BindErrors()
			},
			field: "v",
		},
		{
			name:  "float64 invalid",
			query: "/items?v=not-a-number",
			bind: func(req *http.Request) error {
				var v float64
				return QueryParamsBinder(req).Float64("v", &v).BindErrors()
			},
			field: "v",
		},
		{
			name:  "uuid invalid",
			query: "/items?v=not-a-uuid",
			bind: func(req *http.Request) error {
				var v uuid.UUID
				return QueryParamsBinder(req).UUID("v", &v).BindErrors()
			},
			field: "v",
		},
		{
			name:  "int slice with bad element",
			query: "/items?v=1&v=oops",
			bind: func(req *http.Request) error {
				var v []int
				return QueryParamsBinder(req).Ints("v", &v).BindErrors()
			},
			field: "v",
		},
		{
			name:  "int64 slice with bad element",
			query: "/items?v=1&v=abc",
			bind: func(req *http.Request) error {
				var v []int64
				return QueryParamsBinder(req).Int64s("v", &v).BindErrors()
			},
			field: "v",
		},
		{
			name:  "uint slice with negative element",
			query: "/items?v=1&v=-2",
			bind: func(req *http.Request) error {
				var v []uint
				return QueryParamsBinder(req).Uints("v", &v).BindErrors()
			},
			field: "v",
		},
		{
			name:  "bool slice with bad element",
			query: "/items?v=true&v=maybe",
			bind: func(req *http.Request) error {
				var v []bool
				return QueryParamsBinder(req).Bools("v", &v).BindErrors()
			},
			field: "v",
		},
		{
			name:  "float64 slice with bad element",
			query: "/items?v=1.5&v=bad",
			bind: func(req *http.Request) error {
				var v []float64
				return QueryParamsBinder(req).Float64s("v", &v).BindErrors()
			},
			field: "v",
		},
		{
			name:  "BindUnmarshaler failure",
			query: "/items?v=anything",
			bind: func(req *http.Request) error {
				var v failingParamValue
				return QueryParamsBinder(req).BindUnmarshaler("v", &v).BindErrors()
			},
			field: "v",
		},
		{
			name:  "TextUnmarshaler failure",
			query: "/items?v=anything",
			bind: func(req *http.Request) error {
				var v failingTextValue
				return QueryParamsBinder(req).TextUnmarshaler("v", &v).BindErrors()
			},
			field: "v",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.query, nil)
			violation := assertSingleViolation(t, tc.bind(req))
			want := Violation{
				Field:  tc.field,
				In:     ViolationInQuery,
				Code:   ViolationCodeInvalid,
				Detail: "is invalid",
			}
			if violation != want {
				t.Fatalf("violation = %#v, want %#v", violation, want)
			}
		})
	}
}

func TestValueBinder_EmptyStringVsMissing(t *testing.T) {
	t.Run("query empty string is present for Bind", func(t *testing.T) {
		// ?name= → 参数存在，值为空字符串，string 应绑定为 ""
		req := httptest.NewRequest(http.MethodGet, "/items?name=", nil)

		name := "original"
		if err := QueryParamsBinder(req).String("name", &name).BindErrors(); err != nil {
			t.Fatalf("BindErrors() error = %v", err)
		}
		if name != "" {
			t.Fatalf("name = %q, want empty string", name)
		}
	})

	t.Run("query missing is no-op for Bind", func(t *testing.T) {
		// 完全不提供 name 参数 → 保持原值
		req := httptest.NewRequest(http.MethodGet, "/items", nil)

		name := "original"
		if err := QueryParamsBinder(req).String("name", &name).BindErrors(); err != nil {
			t.Fatalf("BindErrors() error = %v", err)
		}
		if name != "original" {
			t.Fatalf("name = %q, want preserved original", name)
		}
	})

	t.Run("query empty string is present for MustBind", func(t *testing.T) {
		// ?name= → 参数存在，MustBind 不应报 required
		req := httptest.NewRequest(http.MethodGet, "/items?name=", nil)

		var name string
		if err := QueryParamsBinder(req).MustString("name", &name).BindErrors(); err != nil {
			t.Fatalf("BindErrors() error = %v, want no error for present-but-empty", err)
		}
		if name != "" {
			t.Fatalf("name = %q, want empty string", name)
		}
	})

	t.Run("query empty int is present and binds to zero", func(t *testing.T) {
		// ?page= → 参数存在但值为空字符串，int 应绑定为 0（setIntField 将 "" 视为 "0"）
		req := httptest.NewRequest(http.MethodGet, "/items?page=", nil)

		page := 99
		if err := QueryParamsBinder(req).Int("page", &page).BindErrors(); err != nil {
			t.Fatalf("BindErrors() error = %v", err)
		}
		if page != 0 {
			t.Fatalf("page = %d, want 0", page)
		}
	})
}

func TestValueBinder_TypedShorthands(t *testing.T) {
	t.Run("optional methods", func(t *testing.T) {
		wantUUID := uuid.New()
		req := httptest.NewRequest( // nolint:lll
			http.MethodGet,
			"/items?name=kanata&tags=a&tags=b&id=42&ids=1&ids=2&i64=7&i64s=8&i64s=9&u=5&us=6&us=7&enabled=true&flags=true&flags=false&price=10.5&prices=1.25&prices=2.5&custom=u_1&status=ready&uuid="+wantUUID.String()+"&at=2026-04-12T08:30:00Z&sec=1712910600&ms=1712910600123",
			nil,
		)

		var (
			name   string
			tags   []string
			id     int
			ids    []int
			i64    int64
			i64s   []int64
			u      uint
			us     []uint
			enable bool
			flags  []bool
			price  float64
			prices []float64
			custom customParamValue
			status customTextValue
			gotID  uuid.UUID
			at     time.Time
			sec    time.Time
			ms     time.Time
		)

		err := QueryParamsBinder(req).
			String("name", &name).
			Strings("tags", &tags).
			Int("id", &id).
			Ints("ids", &ids).
			Int64("i64", &i64).
			Int64s("i64s", &i64s).
			Uint("u", &u).
			Uints("us", &us).
			Bool("enabled", &enable).
			Bools("flags", &flags).
			Float64("price", &price).
			Float64s("prices", &prices).
			BindUnmarshaler("custom", &custom).
			TextUnmarshaler("status", &status).
			UUID("uuid", &gotID).
			Time("at", &at).
			UnixTime("sec", &sec).
			UnixMilliTime("ms", &ms).
			BindErrors()
		if err != nil {
			t.Fatalf("QueryParamsBinder().BindErrors() error = %v", err)
		}

		if name != "kanata" || id != 42 || i64 != 7 || u != 5 || !enable || price != 10.5 {
			t.Fatalf("scalar values = (%q, %d, %d, %d, %t, %v)", name, id, i64, u, enable, price)
		}
		if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
			t.Fatalf("tags = %#v, want [a b]", tags)
		}
		if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
			t.Fatalf("ids = %#v, want [1 2]", ids)
		}
		if len(i64s) != 2 || i64s[0] != 8 || i64s[1] != 9 {
			t.Fatalf("i64s = %#v, want [8 9]", i64s)
		}
		if len(us) != 2 || us[0] != 6 || us[1] != 7 {
			t.Fatalf("us = %#v, want [6 7]", us)
		}
		if len(flags) != 2 || !flags[0] || flags[1] {
			t.Fatalf("flags = %#v, want [true false]", flags)
		}
		if len(prices) != 2 || prices[0] != 1.25 || prices[1] != 2.5 {
			t.Fatalf("prices = %#v, want [1.25 2.5]", prices)
		}
		if custom.value != "u_1" || status != "ready" {
			t.Fatalf("custom/status = (%#v, %q)", custom, status)
		}
		if gotID != wantUUID {
			t.Fatalf("uuid = %v, want %v", gotID, wantUUID)
		}
		if got := at.UTC().Format(time.RFC3339); got != "2026-04-12T08:30:00Z" {
			t.Fatalf("at = %q, want 2026-04-12T08:30:00Z", got)
		}
		if got := sec.UTC().Format(time.RFC3339); got != "2024-04-12T08:30:00Z" {
			t.Fatalf("sec = %q, want 2024-04-12T08:30:00Z", got)
		}
		if got := ms.UTC().Format(time.RFC3339Nano); got != "2024-04-12T08:30:00.123Z" {
			t.Fatalf("ms = %q, want 2024-04-12T08:30:00.123Z", got)
		}
	})

	t.Run("path scalar methods", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id":      {"42"},
			"name":    {"kanata"},
			"enabled": {"true"},
			"price":   {"10.5"},
		})

		var (
			id      int
			name    string
			enabled bool
			price   float64
		)
		err := PathValuesBinder(req).
			MustInt("id", &id).
			MustString("name", &name).
			MustBool("enabled", &enabled).
			MustFloat64("price", &price).
			BindErrors()
		if err != nil {
			t.Fatalf("PathValuesBinder().BindErrors() error = %v", err)
		}
		if id != 42 || name != "kanata" || !enabled || price != 10.5 {
			t.Fatalf("values = (%d, %q, %t, %v)", id, name, enabled, price)
		}
	})

	t.Run("query slice methods", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?id=1&id=2&tag=a&tag=b&active=true&active=false", nil)

		var (
			ids    []int
			tags   []string
			active []bool
		)
		err := QueryParamsBinder(req).
			MustInts("id", &ids).
			MustStrings("tag", &tags).
			MustBools("active", &active).
			BindErrors()
		if err != nil {
			t.Fatalf("QueryParamsBinder().BindErrors() error = %v", err)
		}
		if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
			t.Fatalf("ids = %#v, want [1 2]", ids)
		}
		if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
			t.Fatalf("tags = %#v, want [a b]", tags)
		}
		if len(active) != 2 || !active[0] || active[1] {
			t.Fatalf("active = %#v, want [true false]", active)
		}
	})

	t.Run("custom unmarshalers", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id":     {"u_1"},
			"status": {"ready"},
		})

		var (
			id     customParamValue
			status customTextValue
		)
		err := PathValuesBinder(req).
			MustBindUnmarshaler("id", &id).
			MustTextUnmarshaler("status", &status).
			BindErrors()
		if err != nil {
			t.Fatalf("PathValuesBinder().BindErrors() error = %v", err)
		}
		if id.value != "u_1" {
			t.Fatalf("id = %#v, want value u_1", id)
		}
		if status != "ready" {
			t.Fatalf("status = %q, want ready", status)
		}
	})

	t.Run("uuid shorthand", func(t *testing.T) {
		want := uuid.New()
		req := requestWithPathParams(map[string][]string{
			"account_id": {want.String()},
		})

		var accountID uuid.UUID
		err := PathValuesBinder(req).
			MustUUID("account_id", &accountID).
			BindErrors()
		if err != nil {
			t.Fatalf("PathValuesBinder().BindErrors() error = %v", err)
		}
		if accountID != want {
			t.Fatalf("account_id = %v, want %v", accountID, want)
		}
	})

	t.Run("time shorthands", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodGet,
			"/items?at=2026-04-12T08:30:00Z&sec=1712910600&ms=1712910600123",
			nil,
		)

		var (
			at  time.Time
			sec time.Time
			ms  time.Time
		)
		err := QueryParamsBinder(req).
			MustTime("at", &at).
			MustUnixTime("sec", &sec).
			MustUnixMilliTime("ms", &ms).
			BindErrors()
		if err != nil {
			t.Fatalf("QueryParamsBinder().BindErrors() error = %v", err)
		}
		if got := at.UTC().Format(time.RFC3339); got != "2026-04-12T08:30:00Z" {
			t.Fatalf("at = %q, want 2026-04-12T08:30:00Z", got)
		}
		if got := sec.UTC().Format(time.RFC3339); got != "2024-04-12T08:30:00Z" {
			t.Fatalf("sec = %q, want 2024-04-12T08:30:00Z", got)
		}
		if got := ms.UTC().Format(time.RFC3339Nano); got != "2024-04-12T08:30:00.123Z" {
			t.Fatalf("ms = %q, want 2024-04-12T08:30:00.123Z", got)
		}
	})

	t.Run("additional must methods", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodGet,
			"/items?i64=7&i64s=8&i64s=9&u=5&us=6&us=7&prices=1.25&prices=2.5",
			nil,
		)

		var (
			i64    int64
			i64s   []int64
			u      uint
			us     []uint
			prices []float64
		)
		err := QueryParamsBinder(req).
			MustInt64("i64", &i64).
			MustInt64s("i64s", &i64s).
			MustUint("u", &u).
			MustUints("us", &us).
			MustFloat64s("prices", &prices).
			BindErrors()
		if err != nil {
			t.Fatalf("QueryParamsBinder().BindErrors() error = %v", err)
		}
		if i64 != 7 || u != 5 {
			t.Fatalf("scalars = (%d, %d), want (7, 5)", i64, u)
		}
		if len(i64s) != 2 || i64s[0] != 8 || i64s[1] != 9 {
			t.Fatalf("i64s = %#v, want [8 9]", i64s)
		}
		if len(us) != 2 || us[0] != 6 || us[1] != 7 {
			t.Fatalf("us = %#v, want [6 7]", us)
		}
		if len(prices) != 2 || prices[0] != 1.25 || prices[1] != 2.5 {
			t.Fatalf("prices = %#v, want [1.25 2.5]", prices)
		}
	})
}

func TestValueBinder_TypedShorthandsMatchBaseSemantics(t *testing.T) {
	t.Run("optional invalid matches Bind", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)

		var (
			basePage      int
			shorthandPage int
		)
		baseErr := QueryParamsBinder(req).Bind("page", &basePage).BindErrors()
		shorthandErr := QueryParamsBinder(req).Int("page", &shorthandPage).BindErrors()

		_ = assertSameHTTPError(t, shorthandErr, baseErr)
		if basePage != 0 || shorthandPage != 0 {
			t.Fatalf("pages = (%d, %d), want unchanged zero values", basePage, shorthandPage)
		}
	})

	t.Run("required missing matches MustBind", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)

		var (
			baseCursor      string
			shorthandCursor string
		)
		baseErr := QueryParamsBinder(req).MustBind("cursor", &baseCursor).BindErrors()
		shorthandErr := QueryParamsBinder(req).MustString("cursor", &shorthandCursor).BindErrors()

		_ = assertSameHTTPError(t, shorthandErr, baseErr)
		if baseCursor != "" || shorthandCursor != "" {
			t.Fatalf("cursors = (%q, %q), want unchanged zero values", baseCursor, shorthandCursor)
		}
	})

	t.Run("default fail-fast ordering matches base binder", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)

		var (
			basePage        int
			baseCursor      string
			shorthandPage   int
			shorthandCursor string
		)
		baseErr := QueryParamsBinder(req).
			Bind("page", &basePage).
			MustBind("cursor", &baseCursor).
			BindErrors()
		shorthandErr := QueryParamsBinder(req).
			Int("page", &shorthandPage).
			MustString("cursor", &shorthandCursor).
			BindErrors()

		_ = assertSameHTTPError(t, shorthandErr, baseErr)
		if basePage != 0 || shorthandPage != 0 || baseCursor != "" || shorthandCursor != "" {
			t.Fatalf(
				"values = (%d, %d, %q, %q), want unchanged zero values",
				basePage,
				shorthandPage,
				baseCursor,
				shorthandCursor,
			)
		}
	})

	t.Run("aggregate violations match base binder", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)

		var (
			basePage        int
			baseCursor      string
			shorthandPage   int
			shorthandCursor string
		)
		baseErr := QueryParamsBinder(req).
			FailFast(false).
			Bind("page", &basePage).
			MustBind("cursor", &baseCursor).
			BindErrors()
		shorthandErr := QueryParamsBinder(req).
			FailFast(false).
			Int("page", &shorthandPage).
			MustString("cursor", &shorthandCursor).
			BindErrors()

		_ = assertSameHTTPError(t, shorthandErr, baseErr)
		if basePage != 0 || shorthandPage != 0 || baseCursor != "" || shorthandCursor != "" {
			t.Fatalf(
				"values = (%d, %d, %q, %q), want unchanged zero values",
				basePage,
				shorthandPage,
				baseCursor,
				shorthandCursor,
			)
		}
	})
}

func TestValueBinder_DefaultFailFastStopsAtFirstViolation(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)

	var (
		page   int
		cursor string
	)
	err := QueryParamsBinder(req).
		Bind("page", &page).
		MustBind("cursor", &cursor).
		BindErrors()

	violation := assertSingleViolation(t, err)
	want := Violation{
		Field:  "page",
		In:     ViolationInQuery,
		Code:   ViolationCodeInvalid,
		Detail: "is invalid",
	}
	if violation != want {
		t.Fatalf("violation = %#v, want %#v", violation, want)
	}
	if page != 0 || cursor != "" {
		t.Fatalf("values = (%d, %q), want unchanged zero values", page, cursor)
	}
}

func TestValueBinder_FailFastFalseCollectsMultipleViolations(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)

	var (
		page   int
		cursor string
	)
	violations := assertViolations(t, QueryParamsBinder(req).
		FailFast(false).
		Bind("page", &page).
		MustBind("cursor", &cursor).
		BindErrors())
	if len(violations) != 2 {
		t.Fatalf("violations len = %d, want 2", len(violations))
	}

	got := map[string]Violation{}
	for _, violation := range violations {
		got[violation.Field] = violation
	}

	if violation := got["page"]; violation != (Violation{
		Field:  "page",
		In:     ViolationInQuery,
		Code:   ViolationCodeInvalid,
		Detail: "is invalid",
	}) {
		t.Fatalf("page violation = %#v", violation)
	}
	if violation := got["cursor"]; violation != (Violation{
		Field:  "cursor",
		In:     ViolationInQuery,
		Code:   ViolationCodeRequired,
		Detail: "is required",
	}) {
		t.Fatalf("cursor violation = %#v", violation)
	}
}

func TestValueBinder_BindErrorVsBindErrors(t *testing.T) {
	t.Run("BindError returns only first violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)

		var (
			page   int
			cursor string
		)
		err := QueryParamsBinder(req).
			FailFast(false).
			Bind("page", &page).
			MustBind("cursor", &cursor).
			BindError()

		violation := assertSingleViolation(t, err)
		// BindError 应只返回第一个 violation（page invalid），不包含第二个（cursor required）
		if violation.Field != "page" {
			t.Fatalf("first violation field = %q, want page", violation.Field)
		}
		if violation.Code != ViolationCodeInvalid {
			t.Fatalf("first violation code = %q, want %q", violation.Code, ViolationCodeInvalid)
		}
	})

	t.Run("BindErrors returns all violations", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)

		var (
			page   int
			cursor string
		)
		violations := assertViolations(t, QueryParamsBinder(req).
			FailFast(false).
			Bind("page", &page).
			MustBind("cursor", &cursor).
			BindErrors())

		if len(violations) != 2 {
			t.Fatalf("violations len = %d, want 2", len(violations))
		}
	})

	t.Run("BindError on success returns nil", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=1", nil)

		var page int
		if err := QueryParamsBinder(req).Int("page", &page).BindError(); err != nil {
			t.Fatalf("BindError() = %v, want nil", err)
		}
	})

	t.Run("BindErrors on success returns nil", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=1", nil)

		var page int
		if err := QueryParamsBinder(req).Int("page", &page).BindErrors(); err != nil {
			t.Fatalf("BindErrors() = %v, want nil", err)
		}
	})
}

func TestValueBinder_FailFastTrueReenablesStopOnFirst(t *testing.T) {
	// FailFast(false) 聚合 → FailFast(true) 恢复 fail-fast
	req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)

	var (
		page   int
		cursor string
		limit  int
	)
	err := QueryParamsBinder(req).
		FailFast(false).
		FailFast(true). // 显式切回 fail-fast
		Bind("page", &page).
		MustBind("cursor", &cursor).
		MustBind("limit", &limit).
		BindErrors()

	// fail-fast 应在 page invalid 后立即停止
	violation := assertSingleViolation(t, err)
	want := Violation{
		Field:  "page",
		In:     ViolationInQuery,
		Code:   ViolationCodeInvalid,
		Detail: "is invalid",
	}
	if violation != want {
		t.Fatalf("violation = %#v, want %#v", violation, want)
	}
}

func TestValueBinder_TimeHelpers_InvalidInputReturnsViolation(t *testing.T) {
	t.Run("unix seconds wrong digit count", func(t *testing.T) {
		// 13 位时间戳传给 10 位 UnixTime → invalid
		req := httptest.NewRequest(http.MethodGet, "/items?sec=1712910600123", nil)

		var sec time.Time
		violation := assertSingleViolation(t, QueryParamsBinder(req).MustUnixTime("sec", &sec).BindErrors())
		want := Violation{
			Field:  "sec",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "is invalid",
		}
		if violation != want {
			t.Fatalf("violation = %#v, want %#v", violation, want)
		}
	})

	t.Run("unix seconds non-numeric", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?sec=abcdefghij", nil)

		var sec time.Time
		violation := assertSingleViolation(t, QueryParamsBinder(req).UnixTime("sec", &sec).BindErrors())
		want := Violation{
			Field:  "sec",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "is invalid",
		}
		if violation != want {
			t.Fatalf("violation = %#v, want %#v", violation, want)
		}
	})

	t.Run("rfc3339 invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?at=bad", nil)

		var at time.Time
		violation := assertSingleViolation(t, QueryParamsBinder(req).Time("at", &at).BindErrors())
		want := Violation{
			Field:  "at",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "is invalid",
		}
		if violation != want {
			t.Fatalf("violation = %#v, want %#v", violation, want)
		}
	})

	t.Run("unix millis invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?ms=bad", nil)

		var ms time.Time
		violation := assertSingleViolation(t, QueryParamsBinder(req).UnixMilliTime("ms", &ms).BindErrors())
		want := Violation{
			Field:  "ms",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "is invalid",
		}
		if violation != want {
			t.Fatalf("violation = %#v, want %#v", violation, want)
		}
	})

	t.Run("unix millis wrong digit count", func(t *testing.T) {
		// 10 位时间戳传给 13 位 UnixMilliTime → invalid
		req := httptest.NewRequest(http.MethodGet, "/items?ms=1712910600", nil)

		var ms time.Time
		violation := assertSingleViolation(t, QueryParamsBinder(req).UnixMilliTime("ms", &ms).BindErrors())
		want := Violation{
			Field:  "ms",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "is invalid",
		}
		if violation != want {
			t.Fatalf("violation = %#v, want %#v", violation, want)
		}
	})

	t.Run("missing required uses BindError first violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)

		var at time.Time
		violation := assertSingleViolation(t, QueryParamsBinder(req).MustTime("at", &at).BindError())
		want := Violation{
			Field:  "at",
			In:     ViolationInQuery,
			Code:   ViolationCodeRequired,
			Detail: "is required",
		}
		if violation != want {
			t.Fatalf("violation = %#v, want %#v", violation, want)
		}
	})

	t.Run("missing optional is no-op", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)

		at := time.Unix(1, 0).UTC()
		if err := QueryParamsBinder(req).Time("at", &at).BindErrors(); err != nil {
			t.Fatalf("BindErrors() error = %v", err)
		}
		if got := at.Format(time.RFC3339); got != "1970-01-01T00:00:01Z" {
			t.Fatalf("at = %q, want preserved value", got)
		}
	})

	t.Run("nil request and nil destination are usage errors", func(t *testing.T) {
		var at time.Time

		err := QueryParamsBinder(nil).Time("at", &at).BindError()
		if err == nil || err.Error() != "reqx: request must not be nil" {
			t.Fatalf("nil request error = %v", err)
		}
		assertNotHTTPError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/items?at=2026-04-12T08:30:00Z", nil)
		err = QueryParamsBinder(req).Time("at", nil).BindError()
		if err == nil || err.Error() != "reqx: destination must be a non-nil pointer" {
			t.Fatalf("nil destination error = %v", err)
		}
		assertNotHTTPError(t, err)
	})

	t.Run("nil binder and fail-fast skip", func(t *testing.T) {
		var (
			binder *ValueBinder
			at     = time.Unix(1, 0).UTC()
		)
		if got := binder.Time("at", &at); got != nil {
			t.Fatalf("Time(nil) = %#v, want nil", got)
		}

		req := httptest.NewRequest(http.MethodGet, "/items?page=oops&at=2026-04-12T08:30:00Z", nil)
		var page int
		violation := assertSingleViolation(t, QueryParamsBinder(req).
			Bind("page", &page).
			MustTime("at", &at).
			BindErrors())
		if violation.Field != "page" || violation.In != ViolationInQuery || violation.Code != ViolationCodeInvalid {
			t.Fatalf("violation = %#v", violation)
		}
		if got := at.Format(time.RFC3339); got != "1970-01-01T00:00:01Z" {
			t.Fatalf("at = %q, want preserved value when fail-fast skips MustTime", got)
		}
	})
}

func TestValueBinder_UnixTimeBoundaryValues(t *testing.T) {
	t.Run("epoch zero is valid unix seconds", func(t *testing.T) {
		// 0000000000 = 10 位全零 = Unix epoch
		req := httptest.NewRequest(http.MethodGet, "/items?sec=0000000000", nil)

		var sec time.Time
		if err := QueryParamsBinder(req).UnixTime("sec", &sec).BindErrors(); err != nil {
			t.Fatalf("BindErrors() error = %v", err)
		}
		if got := sec.UTC().Format(time.RFC3339); got != "1970-01-01T00:00:00Z" {
			t.Fatalf("sec = %q, want 1970-01-01T00:00:00Z", got)
		}
	})

	t.Run("negative unix seconds is valid", func(t *testing.T) {
		// 负数时间戳：-000000001 = 1 个负号 + 9 位数字，总长度正好 10。
		// parseFixedWidthTimestamp 只校验字符串总长度，因此该值会被当作合法秒级时间戳解析。
		req := httptest.NewRequest(http.MethodGet, "/items?sec=-000000001", nil)

		var sec time.Time
		if err := QueryParamsBinder(req).UnixTime("sec", &sec).BindErrors(); err != nil {
			t.Fatalf("BindErrors() error = %v", err)
		}
		if got := sec.UTC().Format(time.RFC3339); got != "1969-12-31T23:59:59Z" {
			t.Fatalf("sec = %q, want 1969-12-31T23:59:59Z", got)
		}
	})

	t.Run("epoch zero is valid unix millis", func(t *testing.T) {
		// 0000000000000 = 13 位全零 = Unix epoch
		req := httptest.NewRequest(http.MethodGet, "/items?ms=0000000000000", nil)

		var ms time.Time
		if err := QueryParamsBinder(req).UnixMilliTime("ms", &ms).BindErrors(); err != nil {
			t.Fatalf("BindErrors() error = %v", err)
		}
		if got := ms.UTC().Format(time.RFC3339); got != "1970-01-01T00:00:00Z" {
			t.Fatalf("ms = %q, want 1970-01-01T00:00:00Z", got)
		}
	})

	t.Run("negative unix millis is valid", func(t *testing.T) {
		// 负数毫秒时间戳：-000000000001 = 1 个负号 + 12 位数字，总长度正好 13。
		req := httptest.NewRequest(http.MethodGet, "/items?ms=-000000000001", nil)

		var ms time.Time
		if err := QueryParamsBinder(req).UnixMilliTime("ms", &ms).BindErrors(); err != nil {
			t.Fatalf("BindErrors() error = %v", err)
		}
		if got := ms.UTC().Format(time.RFC3339Nano); got != "1969-12-31T23:59:59.999Z" {
			t.Fatalf("ms = %q, want 1969-12-31T23:59:59.999Z", got)
		}
	})

	t.Run("9 digit unix seconds is invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?sec=171291060", nil)

		var sec time.Time
		violation := assertSingleViolation(t, QueryParamsBinder(req).UnixTime("sec", &sec).BindErrors())
		if violation.Code != ViolationCodeInvalid {
			t.Fatalf("violation = %#v, want invalid code", violation)
		}
	})

	t.Run("11 digit unix seconds is invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?sec=17129106001", nil)

		var sec time.Time
		violation := assertSingleViolation(t, QueryParamsBinder(req).UnixTime("sec", &sec).BindErrors())
		if violation.Code != ViolationCodeInvalid {
			t.Fatalf("violation = %#v, want invalid code", violation)
		}
	})
}

func TestValueBinder_UsageErrors(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		var id int
		err := PathValuesBinder(nil).Bind("id", &id).BindError()
		if err == nil {
			t.Fatal("BindError() = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: request must not be nil" {
			t.Fatalf("error = %q, want reqx-prefixed nil request error", got)
		}
	})

	t.Run("destination must be pointer", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"42"},
		})

		var id int
		err := PathValuesBinder(req).Bind("id", id).BindError()
		if err == nil {
			t.Fatal("BindError() = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: destination must be a non-nil pointer" {
			t.Fatalf("error = %q, want destination pointer error", got)
		}
	})

	t.Run("destination must not be nil", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"42"},
		})

		err := PathValuesBinder(req).Bind("id", nil).BindError()
		if err == nil {
			t.Fatal("BindError() = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: destination must not be nil" {
			t.Fatalf("error = %q, want destination nil error", got)
		}
	})

	t.Run("zero-value binder returns usage error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=42", nil)

		var (
			binder ValueBinder
			page   int
		)
		binder.r = req

		err := binder.Bind("page", &page).BindError()
		if err == nil {
			t.Fatal("BindError() = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: binder must be created with PathValuesBinder or QueryParamsBinder" {
			t.Fatalf("error = %q, want zero-value binder error", got)
		}
	})

	t.Run("zero-value binder time helper returns usage error", func(t *testing.T) {
		var (
			binder ValueBinder
			at     time.Time
		)

		err := binder.Time("at", &at).BindError()
		if err == nil {
			t.Fatal("BindError() = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: binder must be created with PathValuesBinder or QueryParamsBinder" {
			t.Fatalf("error = %q, want zero-value binder error", got)
		}
	})

	t.Run("unsupported destination type is usage error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=42", nil)

		var dst struct{}
		err := QueryParamsBinder(req).Bind("page", &dst).BindError()
		if err == nil {
			t.Fatal("BindError() = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: destination type struct {} is not supported" {
			t.Fatalf("error = %q, want unsupported destination error", got)
		}
	})

	t.Run("nil binder helpers return nil", func(t *testing.T) {
		var (
			binder *ValueBinder
			name   string
		)

		if got := binder.FailFast(false); got != nil {
			t.Fatalf("FailFast(nil) = %#v, want nil", got)
		}
		if got := binder.Bind("name", &name); got != nil {
			t.Fatalf("Bind(nil) = %#v, want nil", got)
		}
		if err := binder.BindError(); err != nil {
			t.Fatalf("BindError(nil) = %v, want nil", err)
		}
		if err := binder.BindErrors(); err != nil {
			t.Fatalf("BindErrors(nil) = %v, want nil", err)
		}
	})

	t.Run("missing optional input is no-op", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)

		page := 7
		if err := QueryParamsBinder(req).Int("page", &page).BindErrors(); err != nil {
			t.Fatalf("BindErrors() error = %v", err)
		}
		if page != 7 {
			t.Fatalf("page = %d, want preserved value 7", page)
		}
	})

	t.Run("bind error success and reset", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"42"},
		})

		var id int
		binder := PathValuesBinder(req).MustInt("id", &id)
		if err := binder.BindError(); err != nil {
			t.Fatalf("BindError() error = %v", err)
		}
		if err := binder.BindErrors(); err != nil {
			t.Fatalf("BindErrors() after reset error = %v", err)
		}
		if id != 42 {
			t.Fatalf("id = %d, want 42", id)
		}
	})
}
