package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/kanata996/hah/internal/errx"
)

func TestQueryTypedBuilder_Contracts(t *testing.T) {
	t.Run("duplicate key returns multiple field error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=5&page=9", nil)

		_, err := Query(req, "page").Int().Get()
		assertFieldError(t, err, errx.FieldError{
			Field:  "page",
			In:     errx.InQuery,
			Code:   errx.CodeMultiple,
			Detail: "must appear only once",
		})
	})

	t.Run("missing optional returns zero without running check", func(t *testing.T) {
		called := false

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().
			Check(func(int) error {
				called = true
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != 0 {
			t.Fatalf("page = %d, want 0", got)
		}
		if called {
			t.Fatal("Check() ran for missing optional parameter")
		}
	})

	t.Run("request check failure keeps stable invalid detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=3", nil)

		_, err := Query(req, "page").Int().
			Check(func(int) error { return errors.New("must be even") }).
			Get()
		assertInvalidFieldErrorAt(t, err, "page", errx.InQuery)
	})

	t.Run("default validation failure is usage error", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().
			Default(1).
			Min(2).
			Get()
		assertNotHTTPError(t, err)
	})

	t.Run("values preserves order duplicates and empty strings", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?tag=&tag=b&tag=b", nil)

		got, err := Query(req, "tag").Values().Get()
		if err != nil {
			t.Fatalf("Values().Get() error = %v", err)
		}
		if !reflect.DeepEqual(got, []string{"", "b", "b"}) {
			t.Fatalf("tag = %#v, want [\"\" \"b\" \"b\"]", got)
		}
	})

	t.Run("unix time parses fixed width seconds into utc", func(t *testing.T) {
		want := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
		req := httptest.NewRequest(http.MethodGet, "/items?sec="+strconv.FormatInt(want.Unix(), 10), nil)

		got, err := Query(req, "sec").UnixTime().Get()
		if err != nil {
			t.Fatalf("UnixTime().Get() error = %v", err)
		}
		if !got.Equal(want) {
			t.Fatalf("sec = %v, want %v", got, want)
		}
	})

	t.Run("unix time width is fixed to 10 digits", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?sec=123", nil)

		_, err := Query(req, "sec").UnixTime().Get()
		assertInvalidFieldErrorAt(t, err, "sec", errx.InQuery)
	})

	t.Run("unix time requires exactly 10 decimal digits", func(t *testing.T) {
		for _, raw := range []string{"+123456789", "-123456789", "-1234567890"} {
			t.Run(raw, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/items?sec="+url.QueryEscape(raw), nil)

				_, err := Query(req, "sec").UnixTime().Get()
				assertInvalidFieldErrorAt(t, err, "sec", errx.InQuery)
			})
		}
	})

	t.Run("time rejects non strict rfc3339 syntax", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?at=2026-04-13T10:00:00,123Z", nil)

		_, err := Query(req, "at").Time().Get()
		assertInvalidFieldErrorAt(t, err, "at", errx.InQuery)
	})

	t.Run("time rejects invalid rfc3339 offsets", func(t *testing.T) {
		for _, raw := range []string{
			"2026-04-13T10:00:00+08:60",
			"2026-04-13T10:00:00+24:00",
		} {
			t.Run(raw, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/items?at="+url.QueryEscape(raw), nil)

				_, err := Query(req, "at").Time().Get()
				assertInvalidFieldErrorAt(t, err, "at", errx.InQuery)
			})
		}
	})

	t.Run("time rejects strict rfc3339 values with invalid calendar fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?at=2026-13-13T10:00:00Z", nil)

		_, err := Query(req, "at").Time().Get()
		assertInvalidFieldErrorAt(t, err, "at", errx.InQuery)
	})
}

func TestQueryBuilder_UsageContracts(t *testing.T) {
	t.Run("nil request is usage error", func(t *testing.T) {
		_, err := Query(nil, "page").Int().Get()
		assertNotHTTPError(t, err)
	})

	t.Run("empty trimmed name is usage error", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), " ").Int().Get()
		assertNotHTTPError(t, err)
	})

	t.Run("nil url counts as empty query source", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)
		req.URL = nil

		got, err := Query(req, "page").Int().Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != 0 {
			t.Fatalf("page = %d, want 0", got)
		}
	})
}

func TestQueryBuilder_BaselineContracts(t *testing.T) {
	t.Run("query fail open keeps reading requested key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?bad=%zz&tag=a", nil)

		got, err := Query(req, " tag ").String().Required().Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != "a" {
			t.Fatalf("tag = %q, want a", got)
		}
	})

	t.Run("values use net url decoding and missing returns nil", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?tag=a+b&tag=%2B", nil)

		got, err := Query(req, "tag").Values().Get()
		if err != nil {
			t.Fatalf("Values().Get() error = %v", err)
		}
		if !reflect.DeepEqual(got, []string{"a b", "+"}) {
			t.Fatalf("tag = %#v, want []string{\"a b\", \"+\"}", got)
		}

		missing, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "tag").Values().Get()
		if err != nil {
			t.Fatalf("Values().Get() missing error = %v", err)
		}
		if missing != nil {
			t.Fatalf("missing values = %#v, want nil", missing)
		}
	})

	t.Run("required is idempotent and later default wins", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().
			Required().
			Required().
			Get()
		assertRequiredFieldErrorAt(t, err, "page", errx.InQuery)

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().
			Default(1).
			Default(2).
			Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != 2 {
			t.Fatalf("page = %d, want 2", got)
		}
	})

	t.Run("usage error remains sticky after later valid calls", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "mode").String().
			OneOf().
			Default("go").
			Match(regexp.MustCompile("^g$")).
			Get()
		assertNotHTTPError(t, err)
	})
}

func TestQueryStringAndMultiValueContracts(t *testing.T) {
	t.Run("string constraints compose on request input", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?mode=go", nil)

		got, err := Query(req, "mode").String().
			OneOf("go", "rust").
			Match(regexp.MustCompile("^g")).
			MaxLen(2).
			Check(func(v string) error {
				if v != "go" {
					return errors.New("want go")
				}
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("String().Get() error = %v", err)
		}
		if got != "go" {
			t.Fatalf("mode = %q, want go", got)
		}
	})

	t.Run("one of snapshots configured candidates", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?mode=go", nil)
		allowed := []string{"go", "rust"}

		builder := Query(req, "mode").String().OneOf(allowed...)
		allowed[0] = "python"

		got, err := builder.Get()
		if err != nil {
			t.Fatalf("String().OneOf().Get() error = %v", err)
		}
		if got != "go" {
			t.Fatalf("mode = %q, want go", got)
		}
	})

	t.Run("string accepts explicit empty value and required treats it as present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?mode=", nil)

		got, err := Query(req, "mode").String().Required().Get()
		if err != nil {
			t.Fatalf("String().Required().Get() error = %v", err)
		}
		if got != "" {
			t.Fatalf("mode = %q, want empty string", got)
		}
	})

	t.Run("string invalid configurations are usage errors", func(t *testing.T) {
		testCases := []struct {
			name  string
			build func(*StringParam) *StringParam
		}{
			{
				name: "one of requires candidates",
				build: func(p *StringParam) *StringParam {
					return p.OneOf()
				},
			},
			{
				name: "match rejects nil regexp",
				build: func(p *StringParam) *StringParam {
					return p.Match(nil)
				},
			},
			{
				name: "check rejects nil function",
				build: func(p *StringParam) *StringParam {
					return p.Check(nil)
				},
			},
			{
				name: "min len rejects negative numbers",
				build: func(p *StringParam) *StringParam {
					return p.MinLen(-1)
				},
			},
			{
				name: "max len rejects negative numbers",
				build: func(p *StringParam) *StringParam {
					return p.MaxLen(-1)
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := tc.build(Query(httptest.NewRequest(http.MethodGet, "/items", nil), "mode").String()).Get()
				assertNotHTTPError(t, err)
			})
		}
	})

	t.Run("min len and max len count utf8 runes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?mode=%E4%BD%A0", nil)

		got, err := Query(req, "mode").String().MaxLen(1).Get()
		if err != nil {
			t.Fatalf("String().MaxLen(1).Get() error = %v", err)
		}
		if got != "你" {
			t.Fatalf("mode = %q, want 你", got)
		}

		_, err = Query(req, "mode").String().MinLen(2).Get()
		assertInvalidFieldErrorAt(t, err, "mode", errx.InQuery)
	})

	t.Run("values required and check are enforced", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?tag=a&tag=b", nil)

		got, err := Query(req, "tag").Values().
			Required().
			Check(func(values []string) error {
				if len(values) != 2 {
					return errors.New("want two values")
				}
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("Values().Get() error = %v", err)
		}
		if !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Fatalf("tag = %#v, want []string{\"a\", \"b\"}", got)
		}

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "tag").Values().Required().Get()
		assertRequiredFieldErrorAt(t, err, "tag", errx.InQuery)
	})
}

func TestQueryValues_DefaultSnapshotsAndReturnsCopies(t *testing.T) {
	defaults := []string{"a", "b"}
	builder := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "tag").Values().Default(defaults)
	defaults[0] = "mutated"

	got, err := builder.Get()
	if err != nil {
		t.Fatalf("Values().Get() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("values = %#v, want []string{\"a\", \"b\"}", got)
	}

	got[0] = "changed"

	again, err := builder.Get()
	if err != nil {
		t.Fatalf("Values().Get() second error = %v", err)
	}
	if !reflect.DeepEqual(again, []string{"a", "b"}) {
		t.Fatalf("values second = %#v, want []string{\"a\", \"b\"}", again)
	}
}

func TestQueryValues_RequestResultIsDefensiveCopy(t *testing.T) {
	builder := Query(httptest.NewRequest(http.MethodGet, "/items?tag=a&tag=b", nil), "tag").Values()

	got, err := builder.Get()
	if err != nil {
		t.Fatalf("Values().Get() error = %v", err)
	}
	got[0] = "changed"

	again, err := builder.Get()
	if err != nil {
		t.Fatalf("Values().Get() second error = %v", err)
	}
	if !reflect.DeepEqual(again, []string{"a", "b"}) {
		t.Fatalf("values second = %#v, want []string{\"a\", \"b\"}", again)
	}
}

func TestQueryValues_CheckCannotMutateReturnedValue(t *testing.T) {
	t.Run("request values", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?tag=a&tag=b", nil), "tag").Values().
			Check(func(values []string) error {
				values[0] = "mutated"
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("Values().Get() error = %v", err)
		}
		if !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Fatalf("values = %#v, want []string{\"a\", \"b\"}", got)
		}
	})

	t.Run("default values", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "tag").Values().
			Default([]string{"a", "b"}).
			Check(func(values []string) error {
				values[0] = "mutated"
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("Values().Get() error = %v", err)
		}
		if !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Fatalf("values = %#v, want []string{\"a\", \"b\"}", got)
		}
	})
}

func TestQueryValues_ReReadsCurrentRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/items?tag=a&tag=b", nil)
	builder := Query(req, "tag").Values()

	got, err := builder.Get()
	if err != nil {
		t.Fatalf("Values().Get() error = %v", err)
	}
	got[0] = "changed"

	req.URL.RawQuery = "tag=c&tag=d"

	again, err := builder.Get()
	if err != nil {
		t.Fatalf("Values().Get() second error = %v", err)
	}
	if !reflect.DeepEqual(again, []string{"c", "d"}) {
		t.Fatalf("values second = %#v, want []string{\"c\", \"d\"}", again)
	}
}

func TestQueryTypedBuilder_ReReadsCurrentRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/items?page=1", nil)
	builder := Query(req, "page").Int()

	got, err := builder.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != 1 {
		t.Fatalf("page = %d, want 1", got)
	}

	req.URL.RawQuery = "page=2"

	again, err := builder.Get()
	if err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if again != 2 {
		t.Fatalf("page second = %d, want 2", again)
	}
}
