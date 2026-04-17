package reqx

import (
	"errors"
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

	t.Run("query values do not satisfy path lookup", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?id=42", nil)

		_, err := Path(req, "id").String().Required().Get()
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

	t.Run("request check failure keeps stable invalid detail", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"id": {"kanata"}})

		_, err := Path(req, "id").String().
			Check(func(string) error { return errors.New("must be numeric") }).
			Get()
		assertInvalidViolationAt(t, err, "id", errx.InPath)
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
			name:      "malformed wildcard token with inner spaces stays missing",
			pattern:   "/accounts/{ id }",
			paramName: "id",
		},
		{
			name:      "end anchor wildcard does not count as named wildcard",
			pattern:   "/accounts/{$}",
			paramName: "id",
		},
		{
			name:      "end anchor segment inside pattern makes it malformed",
			pattern:   "/{$}/{id}",
			paramName: "id",
		},
		{
			name:      "duplicate wildcard after matching segment makes pattern malformed",
			pattern:   "/{id}/{id}",
			paramName: "id",
		},
		{
			name:        "method qualified pattern counts as present",
			pattern:     "GET /{id}",
			paramName:   "id",
			assertValue: true,
		},
		{
			name:        "tab separated method qualified pattern counts as present",
			pattern:     "GET\t/{id}",
			paramName:   "id",
			assertValue: true,
		},
		{
			name:      "invalid method qualified pattern stays missing",
			pattern:   "A=B /{id}",
			paramName: "id",
		},
		{
			name:      "host qualified pattern stays missing",
			pattern:   "example.com/{id}",
			paramName: "id",
		},
		{
			name:      "blank pattern stays missing",
			pattern:   " ",
			paramName: "id",
		},
		{
			name:      "pattern without wildcard stays missing",
			pattern:   "/accounts/id",
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

func TestPathBuilder_ServeMuxPathValueContracts(t *testing.T) {
	t.Run("non empty wildcard matches request path value", func(t *testing.T) {
		mux := http.NewServeMux()

		var got string
		var pathValue string
		var handlerErr error

		mux.HandleFunc("GET /accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
			pathValue = r.PathValue("id")
			got, handlerErr = Path(r, "id").String().Required().Get()
			w.WriteHeader(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodGet, "/accounts/kanata", nil)
		mux.ServeHTTP(httptest.NewRecorder(), req)

		if handlerErr != nil {
			t.Fatalf("Get() error = %v", handlerErr)
		}
		if pathValue != "kanata" {
			t.Fatalf("request.PathValue(id) = %q, want kanata", pathValue)
		}
		if got != pathValue {
			t.Fatalf("Path().String().Get() = %q, want %q", got, pathValue)
		}
	})

	t.Run("method qualified empty catch all still counts as present", func(t *testing.T) {
		mux := http.NewServeMux()

		var got string
		var pathValue string
		var pattern string
		var handlerErr error

		mux.HandleFunc("GET /files/{rest...}", func(w http.ResponseWriter, r *http.Request) {
			pattern = r.Pattern
			pathValue = r.PathValue("rest")
			got, handlerErr = Path(r, "rest").String().Required().Get()
			w.WriteHeader(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodGet, "/files/", nil)
		mux.ServeHTTP(httptest.NewRecorder(), req)

		if handlerErr != nil {
			t.Fatalf("Get() error = %v", handlerErr)
		}
		if pattern != "GET /files/{rest...}" {
			t.Fatalf("request.Pattern = %q, want GET /files/{rest...}", pattern)
		}
		if pathValue != "" {
			t.Fatalf("request.PathValue(rest) = %q, want empty string", pathValue)
		}
		if got != pathValue {
			t.Fatalf("Path().String().Get() = %q, want empty string", got)
		}
	})
}

func TestPathBuilder_InternalPathLookupContracts(t *testing.T) {
	t.Run("nil request returns missing path lookup", func(t *testing.T) {
		got, ok := pathParamValues(nil, "id")
		if ok {
			t.Fatal("pathParamValues(nil, id) reported present")
		}
		if got != nil {
			t.Fatalf("pathParamValues(nil, id) = %v, want nil", got)
		}
	})

	t.Run("catch all wildcard must be terminal", func(t *testing.T) {
		if pathHasWildcard("/files/{rest...}/meta", "rest") {
			t.Fatal("pathHasWildcard accepted non-terminal catch-all wildcard")
		}
	})
}

func TestPathBuilder_InternalPathPatternPartContracts(t *testing.T) {
	testCases := []struct {
		name        string
		pattern     string
		wantPattern string
		wantOK      bool
	}{
		{
			name:    "empty pattern rejected",
			pattern: "",
		},
		{
			name:        "plain path preserved",
			pattern:     "/accounts/{id}",
			wantPattern: "/accounts/{id}",
			wantOK:      true,
		},
		{
			name:    "method qualified path requires path part",
			pattern: "GET \t",
		},
		{
			name:    "method qualified path requires leading slash",
			pattern: "GET accounts/{id}",
		},
		{
			name:    "method qualified path rejects whitespace in path",
			pattern: "GET /accounts /{id}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotPattern, gotOK := pathPatternPart(tc.pattern)
			if gotOK != tc.wantOK {
				t.Fatalf("pathPatternPart(%q) ok = %v, want %v", tc.pattern, gotOK, tc.wantOK)
			}
			if gotPattern != tc.wantPattern {
				t.Fatalf("pathPatternPart(%q) pattern = %q, want %q", tc.pattern, gotPattern, tc.wantPattern)
			}
		})
	}
}

func TestPathBuilder_InternalWildcardNameContracts(t *testing.T) {
	t.Run("wildcard name requires letter or underscore prefix", func(t *testing.T) {
		if isValidPathWildcardName("9id") {
			t.Fatal("isValidPathWildcardName accepted a digit-prefixed name")
		}
	})
}

func TestPathBuilder_InternalPatternMethodContracts(t *testing.T) {
	t.Run("empty method rejected", func(t *testing.T) {
		if isValidPathPatternMethod("") {
			t.Fatal("isValidPathPatternMethod accepted an empty method")
		}
	})

	testCases := []struct {
		name string
		b    byte
		want bool
	}{
		{
			name: "digit is allowed",
			b:    '9',
			want: true,
		},
		{
			name: "lowercase letter is allowed",
			b:    'g',
			want: true,
		},
		{
			name: "tchar punctuation is allowed",
			b:    '-',
			want: true,
		},
		{
			name: "separator is rejected",
			b:    '=',
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPathPatternMethodByte(tc.b); got != tc.want {
				t.Fatalf("isPathPatternMethodByte(%q) = %v, want %v", tc.b, got, tc.want)
			}
		})
	}
}
