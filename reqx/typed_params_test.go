package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kanata996/hah/internal/errx"
)

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

	t.Run("values nil default stays nil", func(t *testing.T) {
		var defaults []string

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "tag").Values().
			Default(defaults).
			Check(func(values []string) error {
				if values != nil {
					return errors.New("want nil values")
				}
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("Values().Get() error = %v", err)
		}
		if got != nil {
			t.Fatalf("values = %#v, want nil", got)
		}
	})

	t.Run("default check failure is usage error", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().
			Default(2).
			Check(func(v int) error {
				if v%2 == 0 {
					return errors.New("want odd default")
				}
				return nil
			}).
			Get()
		assertNotHTTPError(t, err)
	})
}

func TestQueryBuilder_AdditionalBaselineContracts(t *testing.T) {
	t.Run("required and default are mutually exclusive", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().
			Required().
			Default(1).
			Get()
		assertNotHTTPError(t, err)
	})

	t.Run("default then required is also a usage error", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "enabled").Bool().
			Default(true).
			Required().
			Get()
		assertNotHTTPError(t, err)
	})

	t.Run("string and numeric constraints cover failure and conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?mode=go", nil), "mode").String().
			MinLen(3).
			Get()
		assertInvalidViolationAt(t, err, "mode", errx.InQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "mode").String().
			MinLen(3).
			MaxLen(2).
			Get()
		assertNotHTTPError(t, err)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?page=2", nil), "page").Int().
			Min(3).
			Get()
		assertInvalidViolationAt(t, err, "page", errx.InQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?page=4", nil), "page").Int().
			Max(3).
			Get()
		assertInvalidViolationAt(t, err, "page", errx.InQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().
			Min(4).
			Max(3).
			Get()
		assertNotHTTPError(t, err)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().
			Max(3).
			Min(4).
			Get()
		assertNotHTTPError(t, err)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "mode").String().
			MaxLen(2).
			MinLen(3).
			Get()
		assertNotHTTPError(t, err)
	})

	t.Run("time invalid builder configuration is usage error", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "at").Time().
			Check(nil).
			Get()
		assertNotHTTPError(t, err)
	})

	t.Run("duplicate keys short circuit parsing and checks", func(t *testing.T) {
		checkCalled := false

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=bad&page=2", nil), "page").Int().
			Check(func(int) error {
				checkCalled = true
				return nil
			}).
			Get()
		assertViolation(t, err, errx.Violation{
			Field:  "page",
			In:     errx.InQuery,
			Code:   errx.CodeMultiple,
			Detail: "must appear only once",
		})
		if checkCalled {
			t.Fatal("Check() ran after duplicate-key rejection")
		}
	})

	t.Run("string validators short circuit before custom checks", func(t *testing.T) {
		checkCalled := false

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?mode=rust", nil), "mode").String().
			OneOf("go").
			Check(func(string) error {
				checkCalled = true
				return nil
			}).
			Get()
		assertInvalidViolationAt(t, err, "mode", errx.InQuery)
		if checkCalled {
			t.Fatal("Check() ran after OneOf() failure")
		}

		checkCalled = false
		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?mode=rust", nil), "mode").String().
			Match(regexp.MustCompile("^g")).
			Check(func(string) error {
				checkCalled = true
				return nil
			}).
			Get()
		assertInvalidViolationAt(t, err, "mode", errx.InQuery)
		if checkCalled {
			t.Fatal("Check() ran after Match() failure")
		}
	})

	t.Run("uuid duration and empty-string contracts", func(t *testing.T) {
		want := uuid.New()

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?id="+want.String(), nil), "id").UUID().
			Required().
			Get()
		if err != nil {
			t.Fatalf("UUID().Get() error = %v", err)
		}
		if got != want {
			t.Fatalf("uuid = %v, want %v", got, want)
		}

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?id=not-a-uuid", nil), "id").UUID().Get()
		assertInvalidViolationAt(t, err, "id", errx.InQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?wait=5", nil), "wait").Duration().Get()
		assertInvalidViolationAt(t, err, "wait", errx.InQuery)

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items?enabled=", nil), "enabled").Bool().Get()
		assertInvalidViolationAt(t, err, "enabled", errx.InQuery)
	})

	t.Run("time before accepts earlier values and default then required stays usage error", func(t *testing.T) {
		boundary := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?at=2026-04-13T09:30:00Z", nil), "at").Time().
			Before(boundary).
			Get()
		if err != nil {
			t.Fatalf("Time().Before().Get() error = %v", err)
		}
		if got.UTC().Format(time.RFC3339) != "2026-04-13T09:30:00Z" {
			t.Fatalf("time = %q, want 2026-04-13T09:30:00Z", got.UTC().Format(time.RFC3339))
		}

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "at").Time().
			Default(boundary).
			Required().
			Get()
		assertNotHTTPError(t, err)
	})

	t.Run("missing optional skips built in constraints and match uses regexp semantics", func(t *testing.T) {
		called := false

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "page").Int().
			Min(100).
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
			t.Fatal("Check() ran for missing optional query parameter")
		}

		gotString, err := Query(httptest.NewRequest(http.MethodGet, "/items?mode=xxgoyy", nil), "mode").String().
			Match(regexp.MustCompile("go")).
			Get()
		if err != nil {
			t.Fatalf("String().Get() error = %v", err)
		}
		if gotString != "xxgoyy" {
			t.Fatalf("mode = %q, want xxgoyy", gotString)
		}
	})

	t.Run("typed and values builders keep error priority stable", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=2&page=3", nil), "page").Int().
			Min(4).
			Max(3).
			Get()
		assertNotHTTPError(t, err)

		_, err = Query(nil, "tag").Values().
			Required().
			Get()
		assertNotHTTPError(t, err)

		checkCalled := false
		_, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "tag").Values().
			Required().
			Check(func([]string) error {
				checkCalled = true
				return nil
			}).
			Get()
		assertRequiredViolationAt(t, err, "tag", errx.InQuery)
		if checkCalled {
			t.Fatal("Values().Check() ran after required violation")
		}
	})
}

func TestQueryBuilder_BuiltInConstraintsRunBeforeCustomChecks(t *testing.T) {
	t.Run("later scalar constraint replaces earlier one before custom checks", func(t *testing.T) {
		checkCalled := false

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=1", nil), "page").Int().
			Min(0).
			Check(func(int) error {
				checkCalled = true
				return nil
			}).
			Min(2).
			Get()
		assertInvalidViolationAt(t, err, "page", errx.InQuery)
		if checkCalled {
			t.Fatal("Check() ran after built-in min rejection")
		}
	})

	t.Run("later string constraint replaces earlier one before custom checks", func(t *testing.T) {
		checkCalled := false

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?mode=go", nil), "mode").String().
			MaxLen(2).
			Check(func(string) error {
				checkCalled = true
				return nil
			}).
			MaxLen(1).
			Get()
		assertInvalidViolationAt(t, err, "mode", errx.InQuery)
		if checkCalled {
			t.Fatal("Check() ran after built-in max length rejection")
		}
	})

	t.Run("later time constraint replaces earlier one before custom checks", func(t *testing.T) {
		boundary := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
		checkCalled := false

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at=2026-04-13T10:30:00Z", nil), "at").Time().
			After(boundary.Add(-time.Hour)).
			Check(func(time.Time) error {
				checkCalled = true
				return nil
			}).
			After(boundary.Add(time.Hour)).
			Get()
		assertInvalidViolationAt(t, err, "at", errx.InQuery)
		if checkCalled {
			t.Fatal("Check() ran after built-in time rejection")
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

	testCases := []struct {
		name  string
		build func(*TimeParam) *TimeParam
	}{
		{
			name: "after configured last",
			build: func(p *TimeParam) *TimeParam {
				return p.Before(boundary).After(boundary)
			},
		},
		{
			name: "before configured last",
			build: func(p *TimeParam) *TimeParam {
				return p.After(boundary).Before(boundary)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.build(Query(httptest.NewRequest(http.MethodGet, "/items", nil), "at").Time()).Get()
			assertNotHTTPError(t, err)
		})
	}
}
