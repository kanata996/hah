package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
)

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] Path / Query builder 会为单参数 path/query 读取提供 source-aware required/invalid violation。
// - [✓] Param builder typed getter 会覆盖默认值、范围/枚举/正则/自定义 Check 与 usage error 边界。
// - [✓] Time / UnixTime / UnixMilliTime builder 会覆盖公开成功路径、失败路径与区间约束。
// - [✓] 内部 parse helper 会维持标量空值默认与固定宽度时间戳解析契约；仅作为辅助覆盖，不替代公开 builder 验收。

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

	t.Run("present but empty still satisfies required", func(t *testing.T) {
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
		if err == nil {
			t.Fatal("Query(nil).Int().Get() = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: request must not be nil" {
			t.Fatalf("error = %q, want request must not be nil", got)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), " ").Int().Get()
		if err == nil {
			t.Fatal("Query(empty name).Int().Get() = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: parameter name must not be empty" {
			t.Fatalf("error = %q, want parameter name must not be empty", got)
		}
	})

	t.Run("required default conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().Required().Default(1).Get()
		if err == nil {
			t.Fatal("Required+Default = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: required and default are mutually exclusive" {
			t.Fatalf("error = %q, want required/default conflict", got)
		}
	})

	t.Run("min greater than max", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=3", nil), "page").Int().Min(10).Max(2).Get()
		if err == nil {
			t.Fatal("Min>Max = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: maximum must be greater than or equal to minimum" {
			t.Fatalf("error = %q, want min/max conflict", got)
		}
	})

	t.Run("empty one-of", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?status=open", nil), "status").String().OneOf().Get()
		if err == nil {
			t.Fatal("OneOf(empty) = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: one-of values must not be empty" {
			t.Fatalf("error = %q, want one-of usage error", got)
		}
	})

	t.Run("nil check", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=3", nil), "page").Int().Check(nil).Get()
		if err == nil {
			t.Fatal("Check(nil) = nil, want usage error")
		}
		assertNotHTTPError(t, err)
		if got := err.Error(); got != "reqx: check must not be nil" {
			t.Fatalf("error = %q, want check must not be nil", got)
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
		assertUsageErrorMessage(t, err, "reqx: param builder must not be nil")
	})

	t.Run("zero builder", func(t *testing.T) {
		_, err := (&Param{}).String().Get()
		assertUsageErrorMessage(t, err, "reqx: param builder must be created with Path or Query")
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
		assertUsageErrorMessage(t, err, "reqx: required and default are mutually exclusive")
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
		assertUsageErrorMessage(t, err, "reqx: minimum length must be >= 0")
	})

	t.Run("negative max len", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=go", nil), "name").String().MaxLen(-1).Get()
		assertUsageErrorMessage(t, err, "reqx: maximum length must be >= 0")
	})

	t.Run("one-of invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?status=pending", nil), "status").String().OneOf("open", "closed").Get()
		assertInvalidViolationAt(t, err, "status", ViolationInQuery)
	})

	t.Run("nil match pattern", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?slug=go", nil), "slug").String().Match(nil).Get()
		assertUsageErrorMessage(t, err, "reqx: match pattern must not be nil")
	})

	t.Run("match invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?slug=GO", nil), "slug").String().Match(regexp.MustCompile(`^[a-z]+$`)).Get()
		assertInvalidViolationAt(t, err, "slug", ViolationInQuery)
	})
}

func TestIntParam_ValidationAndRangeErrors(t *testing.T) {
	t.Run("present but empty parses to zero", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=", nil), "page").Int().Get()
		if err != nil {
			t.Fatalf("Int().Get(empty) error = %v", err)
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
		assertUsageErrorMessage(t, err, "reqx: minimum must be less than or equal to maximum")
	})
}

func TestTypedParams_ValidationAndRangeErrors(t *testing.T) {
	t.Run("int64 methods and branches", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=", nil), "v").Int64().
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

		got, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "v").Int64().Default(9).Get()
		if err != nil || got != 9 {
			t.Fatalf("Int64().Default().Get() = (%d, %v), want (9, nil)", got, err)
		}

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?v=1", nil), "v").Int64().Min(2).Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Int64().Max(2).Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Int64().Max(1).Min(2).Get()
		assertUsageErrorMessage(t, err, "reqx: minimum must be less than or equal to maximum")

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Int64().Min(2).Max(1).Get()
		assertUsageErrorMessage(t, err, "reqx: maximum must be greater than or equal to minimum")
	})

	t.Run("uint methods and branches", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?v=", nil), "v").Uint().
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

		got, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "v").Uint().Default(7).Get()
		if err != nil || got != 7 {
			t.Fatalf("Uint().Default().Get() = (%d, %v), want (7, nil)", got, err)
		}

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?v=1", nil), "v").Uint().Min(2).Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Uint().Max(2).Get()
		assertInvalidViolationAt(t, err, "v", ViolationInQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Uint().Max(1).Min(2).Get()
		assertUsageErrorMessage(t, err, "reqx: minimum must be less than or equal to maximum")

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?v=3", nil), "v").Uint().Min(2).Max(1).Get()
		assertUsageErrorMessage(t, err, "reqx: maximum must be greater than or equal to minimum")
	})

	t.Run("bool methods and parse errors", func(t *testing.T) {
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

		got, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "active").Bool().Default(true).Get()
		if err != nil || !got {
			t.Fatalf("Bool().Default().Get() = (%v, %v), want (true, nil)", got, err)
		}

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?active=maybe", nil), "active").Bool().Get()
		assertInvalidViolationAt(t, err, "active", ViolationInQuery)
	})

	t.Run("float64 methods and branches", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?price=", nil), "price").Float64().
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

		got, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "price").Float64().Default(1.25).Get()
		if err != nil || got != 1.25 {
			t.Fatalf("Float64().Default().Get() = (%v, %v), want (1.25, nil)", got, err)
		}

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?price=1", nil), "price").Float64().Min(2).Get()
		assertInvalidViolationAt(t, err, "price", ViolationInQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?price=3", nil), "price").Float64().Max(2).Get()
		assertInvalidViolationAt(t, err, "price", ViolationInQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?price=3", nil), "price").Float64().Max(1).Min(2).Get()
		assertUsageErrorMessage(t, err, "reqx: minimum must be less than or equal to maximum")

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?price=3", nil), "price").Float64().Min(2).Max(1).Get()
		assertUsageErrorMessage(t, err, "reqx: maximum must be greater than or equal to minimum")
	})

	t.Run("uuid methods", func(t *testing.T) {
		want := uuid.New()

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "id").UUID().Default(want).Get()
		if err != nil || got != want {
			t.Fatalf("UUID().Default().Get() = (%v, %v), want (%v, nil)", got, err, want)
		}

		got, err = Query(httptest.NewRequest(http.MethodGet, "/items?id="+want.String(), nil), "id").UUID().
			Check(func(value uuid.UUID) error { return nil }).
			Get()
		if err != nil || got != want {
			t.Fatalf("UUID().Check().Get() = (%v, %v), want (%v, nil)", got, err, want)
		}
	})

	t.Run("time methods and branches", func(t *testing.T) {
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

		got, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "at").Time().Default(at).Get()
		if err != nil || !got.Equal(at) {
			t.Fatalf("Time().Default().Get() = (%v, %v), want (%v, nil)", got, err, at)
		}

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?at="+from.Format(time.RFC3339), nil), "at").Time().After(at).Get()
		assertInvalidViolationAt(t, err, "at", ViolationInQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?at="+to.Format(time.RFC3339), nil), "at").Time().Before(at).Get()
		assertInvalidViolationAt(t, err, "at", ViolationInQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?at="+to.Format(time.RFC3339), nil), "at").Time().Before(from).After(to).Get()
		assertUsageErrorMessage(t, err, "reqx: after time must be less than or equal to before time")

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?at="+to.Format(time.RFC3339), nil), "at").Time().After(to).Before(from).Get()
		assertUsageErrorMessage(t, err, "reqx: before time must be greater than or equal to after time")

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?at=not-a-time", nil), "at").Time().Get()
		assertInvalidViolationAt(t, err, "at", ViolationInQuery)
	})
}

func TestParamParseHelpers_ScalarAndTimestampContracts(t *testing.T) {
	if got, err := parseStringValue("kanata"); err != nil || got != "kanata" {
		t.Fatalf("parseStringValue() = (%q, %v), want (kanata, nil)", got, err)
	}

	if got, err := parseIntValue(""); err != nil || got != 0 {
		t.Fatalf("parseIntValue(empty) = (%d, %v), want (0, nil)", got, err)
	}

	if got, err := parseInt64Value(""); err != nil || got != 0 {
		t.Fatalf("parseInt64Value(empty) = (%d, %v), want (0, nil)", got, err)
	}
	if _, err := parseInt64Value("oops"); err == nil {
		t.Fatal("parseInt64Value(invalid) = nil, want error")
	}

	if got, err := parseUintValue(""); err != nil || got != 0 {
		t.Fatalf("parseUintValue(empty) = (%d, %v), want (0, nil)", got, err)
	}
	if _, err := parseUintValue("oops"); err == nil {
		t.Fatal("parseUintValue(invalid) = nil, want error")
	}

	if got, err := parseBoolValue(""); err != nil || got {
		t.Fatalf("parseBoolValue(empty) = (%v, %v), want (false, nil)", got, err)
	}
	if _, err := parseBoolValue("oops"); err == nil {
		t.Fatal("parseBoolValue(invalid) = nil, want error")
	}

	if got, err := parseFloat64Value(""); err != nil || got != 0 {
		t.Fatalf("parseFloat64Value(empty) = (%v, %v), want (0, nil)", got, err)
	}
	if _, err := parseFloat64Value("oops"); err == nil {
		t.Fatal("parseFloat64Value(invalid) = nil, want error")
	}

	if _, err := parseRFC3339Time("not-a-time"); err == nil {
		t.Fatal("parseRFC3339Time(invalid) = nil, want error")
	}

	if _, err := parseUnixTime("123"); err == nil {
		t.Fatal("parseUnixTime(short) = nil, want error")
	}
	if _, err := parseUnixTime("abcdefghij"); err == nil {
		t.Fatal("parseUnixTime(non-numeric) = nil, want error")
	}

	if _, err := parseUnixMilliTime("123"); err == nil {
		t.Fatal("parseUnixMilliTime(short) = nil, want error")
	}
	if _, err := parseUnixMilliTime("abcdefghijklm"); err == nil {
		t.Fatal("parseUnixMilliTime(non-numeric) = nil, want error")
	}
}

func assertUsageErrorMessage(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want usage error")
	}
	assertNotHTTPError(t, err)
	if got := err.Error(); got != want {
		t.Fatalf("error = %q, want %q", got, want)
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
