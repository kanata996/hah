package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
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
}
