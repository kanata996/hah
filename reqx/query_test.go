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
)

func TestQuery_SuccessPaths(t *testing.T) {
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

	t.Run("query duplicate key uses first value", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=5&page=9", nil)

		got, err := Query(req, "page").Int().Get()
		if err != nil {
			t.Fatalf("Query().Int().Get() error = %v", err)
		}
		if got != 5 {
			t.Fatalf("page = %d, want 5", got)
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

func TestQuery_RequiredAndInvalidViolations(t *testing.T) {
	t.Run("required missing query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)

		_, err := Query(req, "page").Int().Required().Get()
		assertRequiredViolationAt(t, err, "page", ViolationInQuery)
	})

	t.Run("invalid query int", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)

		_, err := Query(req, "page").Int().Get()
		assertInvalidViolationAt(t, err, "page", ViolationInQuery)
	})

	t.Run("duplicate query validates first value", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=oops&page=3", nil)

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

	t.Run("present empty string remains empty when required", func(t *testing.T) {
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

func TestQuery_UsageErrors(t *testing.T) {
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

func TestQueryValuesParam_SuccessAndErrors(t *testing.T) {
	t.Run("values preserves repeated query order", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?tag=a&tag=b", nil)

		got, err := Query(req, "tag").Values().Required().Get()
		if err != nil {
			t.Fatalf("Query().Values().Required().Get() error = %v", err)
		}
		if !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Fatalf("tag = %#v, want [a b]", got)
		}
	})

	t.Run("values returns raw values including empty string", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?tag=&tag=b", nil)

		got, err := Query(req, "tag").Values().Get()
		if err != nil {
			t.Fatalf("Query().Values().Get() error = %v", err)
		}
		if !reflect.DeepEqual(got, []string{"", "b"}) {
			t.Fatalf("tag = %#v, want [\"\" b]", got)
		}
	})

	t.Run("missing optional returns nil slice", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "tag").Values().Get()
		if err != nil {
			t.Fatalf("Query().Values().Get() error = %v", err)
		}
		if got != nil {
			t.Fatalf("tag = %#v, want nil", got)
		}
	})

	t.Run("default clones slice", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)
		def := []string{"a", "b"}
		builder := Query(req, "tag").Values().Default(def)
		def[0] = "mutated"

		got, err := builder.Get()
		if err != nil {
			t.Fatalf("Query().Values().Default().Get() error = %v", err)
		}
		if !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Fatalf("tag = %#v, want [a b]", got)
		}

		got[0] = "changed"
		again, err := builder.Get()
		if err != nil {
			t.Fatalf("builder.Get() second call error = %v", err)
		}
		if !reflect.DeepEqual(again, []string{"a", "b"}) {
			t.Fatalf("tag second call = %#v, want [a b]", again)
		}
	})

	t.Run("default nil stays nil across calls", func(t *testing.T) {
		builder := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "tag").Values().Default(nil)

		got, err := builder.Get()
		if err != nil {
			t.Fatalf("Query().Values().Default(nil).Get() error = %v", err)
		}
		if got != nil {
			t.Fatalf("tag = %#v, want nil", got)
		}

		again, err := builder.Get()
		if err != nil {
			t.Fatalf("builder.Get() second call error = %v", err)
		}
		if again != nil {
			t.Fatalf("tag second call = %#v, want nil", again)
		}
	})

	t.Run("required missing returns query violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)

		_, err := Query(req, "tag").Values().Required().Get()
		assertRequiredViolationAt(t, err, "tag", ViolationInQuery)
	})

	t.Run("check failure uses custom detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?tag=a&tag=b", nil)

		_, err := Query(req, "tag").Values().Check(func(values []string) error {
			if len(values) == 2 {
				return errors.New("multi value tag is not allowed")
			}
			return nil
		}).Get()
		assertViolation(t, err, Violation{
			Field:  "tag",
			In:     ViolationInQuery,
			Code:   ViolationCodeInvalid,
			Detail: "multi value tag is not allowed",
		})
	})

	t.Run("nil check", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?tag=a", nil), "tag").Values().Check(nil).Get()
		assertUsageErrorContains(t, err, "check must not be nil")
	})

	t.Run("required and default conflict in both orders", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "tag").Values().Default([]string{"a"}).Required().Get()
		assertUsageErrorContains(t, err, "required and default are mutually exclusive")

		_, err = Query(httptest.NewRequest(http.MethodGet, "/items", nil), "tag").Values().Required().Default([]string{"a"}).Get()
		assertUsageErrorContains(t, err, "required and default are mutually exclusive")
	})
}

func TestQueryBuilder_UsageAndOptionalBehavior(t *testing.T) {
	t.Run("zero query builder", func(t *testing.T) {
		_, err := (&QueryParam{}).Values().Get()
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

	t.Run("nil url behaves as missing query input", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)
		req.URL = nil

		got, err := Query(req, "name").String().Get()
		if err != nil {
			t.Fatalf("Query(req with nil URL).String().Get() error = %v", err)
		}
		if got != "" {
			t.Fatalf("name = %q, want empty string", got)
		}

		_, err = Query(req, "name").String().Required().Get()
		assertRequiredViolationAt(t, err, "name", ViolationInQuery)
	})

	t.Run("default then required conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "name").String().Default("kanata").Required().Get()
		assertUsageErrorContains(t, err, "required and default are mutually exclusive")
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
