package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/kanata996/hah/errx"
)

func TestBoolParam_ValidationAndErrors(t *testing.T) {
	t.Run("bool required and check success", func(t *testing.T) {
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
	})

	t.Run("bool default success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "active").Bool().Default(true).Get()
		if err != nil || !got {
			t.Fatalf("Bool().Default().Get() = (%v, %v), want (true, nil)", got, err)
		}
	})

	t.Run("bool empty string parses false when required", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?active=", nil), "active").Bool().Required().Get()
		if err != nil || got {
			t.Fatalf("Bool().Required().Get() = (%v, %v), want (false, nil)", got, err)
		}
	})

	t.Run("bool default check invalid uses custom detail", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "active").Bool().
			Default(true).
			Check(func(value bool) error {
				return errors.New("default bool must be rejected")
			}).
			Get()
		assertViolation(t, err, errx.Violation{
			Field:  "active",
			In:     errx.ViolationInQuery,
			Code:   errx.ViolationCodeInvalid,
			Detail: "default bool must be rejected",
		})
	})

	t.Run("bool parse invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?active=maybe", nil), "active").Bool().Get()
		assertInvalidViolationAt(t, err, "active", errx.ViolationInQuery)
	})
}

func TestUUIDParam_ValidationAndErrors(t *testing.T) {
	t.Run("uuid default success", func(t *testing.T) {
		want := uuid.New()

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "id").UUID().Default(want).Get()
		if err != nil || got != want {
			t.Fatalf("UUID().Default().Get() = (%v, %v), want (%v, nil)", got, err, want)
		}
	})

	t.Run("uuid default check invalid uses custom detail", func(t *testing.T) {
		want := uuid.New()

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "id").UUID().
			Default(want).
			Check(func(value uuid.UUID) error {
				return errors.New("default uuid must be rejected")
			}).
			Get()
		assertViolation(t, err, errx.Violation{
			Field:  "id",
			In:     errx.ViolationInQuery,
			Code:   errx.ViolationCodeInvalid,
			Detail: "default uuid must be rejected",
		})
	})

	t.Run("uuid check success", func(t *testing.T) {
		want := uuid.New()

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?id="+want.String(), nil), "id").UUID().
			Check(func(value uuid.UUID) error { return nil }).
			Get()
		if err != nil || got != want {
			t.Fatalf("UUID().Check().Get() = (%v, %v), want (%v, nil)", got, err, want)
		}
	})

	t.Run("uuid parse invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?id=oops", nil), "id").UUID().Get()
		assertInvalidViolationAt(t, err, "id", errx.ViolationInQuery)
	})
}
