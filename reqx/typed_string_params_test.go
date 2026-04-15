package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/kanata996/hah/errx"
)

func TestPathTypedParams_DirectCoverage(t *testing.T) {
	t.Run("string default one-of match and check success", func(t *testing.T) {
		got, err := Path(requestWithPathParams(nil), "slug").String().
			Default("acct_123").
			OneOf("acct_123", "acct_456").
			Match(regexp.MustCompile(`^acct_[0-9]+$`)).
			Check(func(value string) error {
				if value != "acct_123" {
					return errors.New("unexpected slug")
				}
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("Path().String().Default().OneOf().Match().Check().Get() error = %v", err)
		}
		if got != "acct_123" {
			t.Fatalf("slug = %q, want acct_123", got)
		}
	})

	t.Run("int default still honors range", func(t *testing.T) {
		_, err := Path(requestWithPathParams(nil), "page").Int().
			Default(1).
			Min(2).
			Get()
		assertInvalidViolationAt(t, err, "page", errx.ViolationInPath)
	})

	t.Run("required default conflict", func(t *testing.T) {
		_, err := Path(requestWithPathParams(nil), "page").Int().
			Required().
			Default(1).
			Get()
		assertUsageErrorContains(t, err, "required and default are mutually exclusive")
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

	t.Run("default one-of invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "status").String().
			Default("pending").
			OneOf("open", "closed").
			Get()
		assertInvalidViolationAt(t, err, "status", errx.ViolationInQuery)
	})

	t.Run("default match invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "slug").String().
			Default("GO").
			Match(regexp.MustCompile(`^[a-z]+$`)).
			Get()
		assertInvalidViolationAt(t, err, "slug", errx.ViolationInQuery)
	})

	t.Run("default check invalid uses custom detail", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "name").String().
			Default("kanata").
			Check(func(value string) error {
				return errors.New("default must be rejected")
			}).
			Get()
		assertViolation(t, err, errx.Violation{
			Field:  "name",
			In:     errx.ViolationInQuery,
			Code:   errx.ViolationCodeInvalid,
			Detail: "default must be rejected",
		})
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
		assertInvalidViolationAt(t, err, "name", errx.ViolationInQuery)
	})

	t.Run("max len invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=golang", nil), "name").String().MaxLen(3).Get()
		assertInvalidViolationAt(t, err, "name", errx.ViolationInQuery)
	})

	t.Run("negative min len", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=go", nil), "name").String().MinLen(-1).Get()
		assertUsageErrorContains(t, err, "minimum length must be >= 0")
	})

	t.Run("negative max len", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=go", nil), "name").String().MaxLen(-1).Get()
		assertUsageErrorContains(t, err, "maximum length must be >= 0")
	})

	t.Run("min len greater than max len is usage error", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=abcd", nil), "name").String().MinLen(5).MaxLen(3).Get()
		assertUsageErrorContains(t, err, "maximum length must be greater than or equal to minimum length")
	})

	t.Run("max len before larger min len is usage error", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=abcd", nil), "name").String().MaxLen(3).MinLen(5).Get()
		assertUsageErrorContains(t, err, "minimum length must be less than or equal to maximum length")
	})

	t.Run("max len smaller than min len can be corrected by later min len", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=abc", nil), "name").String().
			MaxLen(2).
			MinLen(3).
			MaxLen(3).
			Get()
		if err != nil {
			t.Fatalf("String().MaxLen().MinLen().MaxLen().Get() error = %v", err)
		}
		if got != "abc" {
			t.Fatalf("name = %q, want abc", got)
		}
	})

	t.Run("repeated min len after check preserves custom detail precedence", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?name=bad", nil), "name").String().
			MinLen(1).
			Check(func(value string) error {
				if value == "bad" {
					return errors.New("custom string detail")
				}
				return nil
			}).
			MinLen(5).
			Get()
		assertViolation(t, err, errx.Violation{
			Field:  "name",
			In:     errx.ViolationInQuery,
			Code:   errx.ViolationCodeInvalid,
			Detail: "custom string detail",
		})
	})

	t.Run("one-of invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?status=pending", nil), "status").String().OneOf("open", "closed").Get()
		assertInvalidViolationAt(t, err, "status", errx.ViolationInQuery)
	})

	t.Run("nil match pattern", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?slug=go", nil), "slug").String().Match(nil).Get()
		assertUsageErrorContains(t, err, "match pattern must not be nil")
	})

	t.Run("match invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?slug=GO", nil), "slug").String().Match(regexp.MustCompile(`^[a-z]+$`)).Get()
		assertInvalidViolationAt(t, err, "slug", errx.ViolationInQuery)
	})
}
