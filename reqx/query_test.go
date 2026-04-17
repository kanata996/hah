package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/kanata996/hah/errx"
)

func TestQueryTypedBuilder_Contracts(t *testing.T) {
	t.Run("duplicate key returns multiple violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=5&page=9", nil)

		_, err := Query(req, "page").Int().Get()
		assertViolation(t, err, errx.Violation{
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
		assertInvalidViolationAt(t, err, "page", errx.InQuery)
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

	t.Run("unix time width is fixed to 10 digits", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?sec=123", nil)

		_, err := Query(req, "sec").UnixTime().Get()
		assertInvalidViolationAt(t, err, "sec", errx.InQuery)
	})

	t.Run("unix milli parses and normalizes to utc", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?ms=1712910600123", nil)

		got, err := Query(req, "ms").UnixMilliTime().Get()
		if err != nil {
			t.Fatalf("UnixMilliTime().Get() error = %v", err)
		}
		if got.UTC().Format(time.RFC3339Nano) != "2024-04-12T08:30:00.123Z" {
			t.Fatalf("time = %q, want 2024-04-12T08:30:00.123Z", got.UTC().Format(time.RFC3339Nano))
		}
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
	t.Run("zero value builders are usage errors", func(t *testing.T) {
		var builder QueryParam
		_, err := builder.String().Get()
		assertNotHTTPError(t, err)

		var typed StringParam
		_, err = typed.Get()
		assertNotHTTPError(t, err)
	})

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
		assertRequiredViolationAt(t, err, "page", errx.InQuery)

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

func TestQueryTypedBuilder_ScalarParsersAndConstraints(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/items?i64=-9&u=7&u64=12&enabled=true&score=1.25&wait=5s&when=2026-04-13T18:00:00%2B08:00", nil)

	gotInt64, err := Query(req, "i64").Int64().Max(-1).Get()
	if err != nil {
		t.Fatalf("Int64().Get() error = %v", err)
	}
	if gotInt64 != -9 {
		t.Fatalf("i64 = %d, want -9", gotInt64)
	}

	gotUint, err := Query(req, "u").Uint().Min(1).Max(7).Get()
	if err != nil {
		t.Fatalf("Uint().Get() error = %v", err)
	}
	if gotUint != 7 {
		t.Fatalf("u = %d, want 7", gotUint)
	}

	gotUint64, err := Query(req, "u64").Uint64().Get()
	if err != nil {
		t.Fatalf("Uint64().Get() error = %v", err)
	}
	if gotUint64 != 12 {
		t.Fatalf("u64 = %d, want 12", gotUint64)
	}

	gotBool, err := Query(req, "enabled").Bool().
		Check(func(v bool) error {
			if !v {
				return errors.New("want true")
			}
			return nil
		}).
		Get()
	if err != nil {
		t.Fatalf("Bool().Get() error = %v", err)
	}
	if !gotBool {
		t.Fatal("enabled = false, want true")
	}

	gotFloat, err := Query(req, "score").Float64().Max(2).Get()
	if err != nil {
		t.Fatalf("Float64().Get() error = %v", err)
	}
	if gotFloat != 1.25 {
		t.Fatalf("score = %v, want 1.25", gotFloat)
	}

	gotDuration, err := Query(req, "wait").Duration().Get()
	if err != nil {
		t.Fatalf("Duration().Get() error = %v", err)
	}
	if gotDuration != 5*time.Second {
		t.Fatalf("wait = %v, want 5s", gotDuration)
	}

	gotTime, err := Query(req, "when").Time().
		Required().
		Check(func(v time.Time) error {
			if v.Format(time.RFC3339) != "2026-04-13T18:00:00+08:00" {
				return errors.New("time mismatch")
			}
			return nil
		}).
		Get()
	if err != nil {
		t.Fatalf("Time().Get() error = %v", err)
	}
	if got := gotTime.Format(time.RFC3339); got != "2026-04-13T18:00:00+08:00" {
		t.Fatalf("when = %q, want 2026-04-13T18:00:00+08:00", got)
	}
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

	t.Run("string usage errors stay ordinary errors", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "mode").String().
			OneOf().
			Match(nil).
			MaxLen(-1).
			Check(nil).
			Get()
		assertNotHTTPError(t, err)
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
		assertRequiredViolationAt(t, err, "tag", errx.InQuery)
	})
}

func TestQueryDefaultContracts(t *testing.T) {
	t.Run("bool default runs checks", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "enabled").Bool().
			Default(true).
			Check(func(v bool) error {
				if !v {
					return errors.New("want true")
				}
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("Bool().Get() error = %v", err)
		}
		if !got {
			t.Fatal("enabled = false, want true")
		}
	})

	t.Run("time default returns configured value", func(t *testing.T) {
		want := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "when").Time().
			Default(want).
			Check(func(v time.Time) error {
				if !v.Equal(want) {
					return errors.New("want default time")
				}
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("Time().Get() error = %v", err)
		}
		if !got.Equal(want) {
			t.Fatalf("when = %v, want %v", got, want)
		}
	})
}

func TestQueryTimeParam_EqualBoundariesAreRejected(t *testing.T) {
	boundary := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

	testCases := []struct {
		name  string
		raw   string
		build func(*QueryParam) *TimeParam
	}{
		{
			name: "rfc3339 time",
			raw:  boundary.Format(time.RFC3339),
			build: func(p *QueryParam) *TimeParam {
				return p.Time()
			},
		},
		{
			name: "unix time",
			raw:  strconv.FormatInt(boundary.Unix(), 10),
			build: func(p *QueryParam) *TimeParam {
				return p.UnixTime()
			},
		},
		{
			name: "unix milli time",
			raw:  strconv.FormatInt(boundary.UnixMilli(), 10),
			build: func(p *QueryParam) *TimeParam {
				return p.UnixMilliTime()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name+"/after", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/items?at="+tc.raw, nil)

			_, err := tc.build(Query(req, "at")).After(boundary).Get()
			assertInvalidViolationAt(t, err, "at", errx.InQuery)
		})

		t.Run(tc.name+"/before", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/items?at="+tc.raw, nil)

			_, err := tc.build(Query(req, "at")).Before(boundary).Get()
			assertInvalidViolationAt(t, err, "at", errx.InQuery)
		})
	}
}

func TestQueryTimeParam_EqualAfterBeforeIsUsageError(t *testing.T) {
	boundary := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

	_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "at").Time().
		After(boundary).
		Before(boundary).
		Get()
	assertNotHTTPError(t, err)
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
