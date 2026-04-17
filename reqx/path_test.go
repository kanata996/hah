package reqx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/kanata996/hah/errx"
)

func TestPathBuilder_Contracts(t *testing.T) {
	t.Run("required missing path returns required violation", func(t *testing.T) {
		_, err := Path(requestWithPathParams(nil), "id").String().Required().Get()
		assertRequiredViolationAt(t, err, "id", errx.InPath)
	})

	t.Run("declared empty wildcard counts as present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Pattern = "/accounts/{id}"
		req.SetPathValue("id", "")

		got, err := Path(req, "id").String().Required().Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != "" {
			t.Fatalf("id = %q, want empty string", got)
		}
	})

	t.Run("malformed pattern does not make empty value present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Pattern = "/accounts/{id"
		req.SetPathValue("id", "")

		_, err := Path(req, "id").String().Required().Get()
		assertRequiredViolationAt(t, err, "id", errx.InPath)
	})

	t.Run("empty string only string accepts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Pattern = "/accounts/{id}"
		req.SetPathValue("id", "")

		_, err := Path(req, "id").Int().Required().Get()
		assertInvalidViolationAt(t, err, "id", errx.InPath)
	})

	t.Run("default validation failure is usage error", func(t *testing.T) {
		_, err := Path(requestWithPathParams(nil), "since").String().
			Default("2026-04-13T10:00:00Z").
			MinLen(100).
			Get()
		assertNotHTTPError(t, err)
	})

	t.Run("uuid parse failure is invalid violation", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"id": {"not-a-uuid"}})

		_, err := Path(req, "id").UUID().Get()
		assertInvalidViolationAt(t, err, "id", errx.InPath)
	})

	t.Run("uuid parse success", func(t *testing.T) {
		want := uuid.New()
		req := requestWithPathParams(map[string][]string{"id": {want.String()}})

		got, err := Path(req, "id").UUID().Required().Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != want {
			t.Fatalf("uuid = %v, want %v", got, want)
		}
	})
}

func TestPathBuilder_BaselineContracts(t *testing.T) {
	t.Run("nil request empty name and zero value builders are usage errors", func(t *testing.T) {
		_, err := Path(nil, "id").String().Get()
		assertNotHTTPError(t, err)

		_, err = Path(httptest.NewRequest(http.MethodGet, "/", nil), " ").String().Get()
		assertNotHTTPError(t, err)

		var builder PathParam
		_, err = builder.String().Get()
		assertNotHTTPError(t, err)

		var typed StringParam
		_, err = typed.Get()
		assertNotHTTPError(t, err)
	})

	t.Run("optional missing returns zero and required is idempotent", func(t *testing.T) {
		got, err := Path(requestWithPathParams(nil), "count").Int().Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != 0 {
			t.Fatalf("count = %d, want 0", got)
		}

		_, err = Path(requestWithPathParams(nil), "count").Int().
			Required().
			Required().
			Get()
		assertRequiredViolationAt(t, err, "count", errx.InPath)
	})

	t.Run("required default and nil check are usage errors", func(t *testing.T) {
		_, err := Path(requestWithPathParams(nil), "id").String().
			Required().
			Default("x").
			Get()
		assertNotHTTPError(t, err)

		_, err = Path(requestWithPathParams(nil), "id").String().
			Check(nil).
			Get()
		assertNotHTTPError(t, err)
	})

	t.Run("string constraints cover success failure and conflict", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"slug": {"go"}})

		got, err := Path(req, "slug").String().
			MinLen(2).
			MaxLen(2).
			Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != "go" {
			t.Fatalf("slug = %q, want go", got)
		}

		_, err = Path(requestWithPathParams(map[string][]string{"slug": {"g"}}), "slug").String().
			MinLen(2).
			Get()
		assertInvalidViolationAt(t, err, "slug", errx.InPath)

		_, err = Path(requestWithPathParams(nil), "slug").String().
			MinLen(3).
			MaxLen(2).
			Get()
		assertNotHTTPError(t, err)
	})

	t.Run("numeric constraints cover success failure and conflict", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"id": {"7"}})

		got, err := Path(req, "id").Int().
			Min(1).
			Max(7).
			Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != 7 {
			t.Fatalf("id = %d, want 7", got)
		}

		_, err = Path(req, "id").Int().
			Max(6).
			Get()
		assertInvalidViolationAt(t, err, "id", errx.InPath)

		_, err = Path(requestWithPathParams(nil), "id").Int().
			Min(8).
			Max(7).
			Get()
		assertNotHTTPError(t, err)
	})

	t.Run("bridge path values are consumed as set without unescape", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetPathValue("id", "a%2Fb")

		got, err := Path(req, " id ").String().Required().Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != "a%2Fb" {
			t.Fatalf("id = %q, want a%%2Fb", got)
		}
	})
}

func TestPathBuilder_WildcardPresenceRules(t *testing.T) {
	testCases := []struct {
		name        string
		pattern     string
		paramName   string
		assertValue bool
	}{
		{
			name:        "catch all wildcard counts as present",
			pattern:     "/accounts/{id...}",
			paramName:   "id",
			assertValue: true,
		},
		{
			name:      "different wildcard name stays missing",
			pattern:   "/accounts/{other}",
			paramName: "id",
		},
		{
			name:      "malformed adapter specific wildcard stays missing",
			pattern:   "/accounts/{id:[0-9]+}",
			paramName: "id",
		},
		{
			name:      "end anchor wildcard does not count as named wildcard",
			pattern:   "/accounts/{$}",
			paramName: "id",
		},
		{
			name:      "blank pattern stays missing",
			pattern:   " ",
			paramName: "id",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Pattern = tc.pattern
			req.SetPathValue("id", "")

			got, err := Path(req, " "+tc.paramName+" ").String().Required().Get()
			if tc.assertValue {
				if err != nil {
					t.Fatalf("Get() error = %v", err)
				}
				if got != "" {
					t.Fatalf("id = %q, want empty string", got)
				}
				return
			}
			assertRequiredViolationAt(t, err, "id", errx.InPath)
		})
	}
}
