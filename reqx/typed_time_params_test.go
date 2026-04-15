package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kanata996/hah/errx"
)

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

	t.Run("unix time required equal boundaries and check success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?sec=1712910600", nil)
		want := time.Date(2024, 4, 12, 8, 30, 0, 0, time.UTC)

		got, err := Query(req, "sec").UnixTime().
			Required().
			After(want).
			Before(want).
			Check(func(value time.Time) error {
				if !value.Equal(want) {
					return errors.New("unexpected unix time")
				}
				return nil
			}).
			Get()
		if err != nil {
			t.Fatalf("UnixTime().Required().After().Before().Check().Get() error = %v", err)
		}
		if !got.Equal(want) {
			t.Fatalf("unix sec = %v, want %v", got, want)
		}
	})

	t.Run("unix milli default when missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)
		want := time.Date(2024, 4, 12, 8, 30, 0, 123*int(time.Millisecond), time.UTC)

		got, err := Query(req, "ms").UnixMilliTime().Default(want).Get()
		if err != nil {
			t.Fatalf("UnixMilliTime().Default().Get() error = %v", err)
		}
		if !got.Equal(want) {
			t.Fatalf("unix ms = %v, want %v", got, want)
		}
	})

	t.Run("unix milli default still honors before", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)
		def := time.Date(2024, 4, 12, 8, 30, 0, 123*int(time.Millisecond), time.UTC)

		_, err := Query(req, "ms").UnixMilliTime().
			Default(def).
			Before(def.Add(-time.Millisecond)).
			Get()
		assertInvalidViolationAt(t, err, "ms", errx.ViolationInQuery)
	})

	t.Run("unix time default check invalid uses custom detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)
		def := time.Date(2024, 4, 12, 8, 30, 0, 0, time.UTC)

		_, err := Query(req, "sec").UnixTime().
			Default(def).
			Check(func(value time.Time) error {
				return errors.New("default unix time must be rejected")
			}).
			Get()
		assertViolation(t, err, errx.Violation{
			Field:  "sec",
			In:     errx.ViolationInQuery,
			Code:   errx.ViolationCodeInvalid,
			Detail: "default unix time must be rejected",
		})
	})

	t.Run("unix time required missing returns query violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)

		_, err := Query(req, "sec").UnixTime().Required().Get()
		assertRequiredViolationAt(t, err, "sec", errx.ViolationInQuery)
	})

	t.Run("unix milli before invalid returns query violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?ms=1712910600123", nil)

		_, err := Query(req, "ms").UnixMilliTime().
			Before(time.Date(2024, 4, 12, 8, 30, 0, 122*int(time.Millisecond), time.UTC)).
			Get()
		assertInvalidViolationAt(t, err, "ms", errx.ViolationInQuery)
	})

	t.Run("unix time invalid width returns query violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?sec=123", nil)

		_, err := Query(req, "sec").UnixTime().Get()
		assertInvalidViolationAt(t, err, "sec", errx.ViolationInQuery)
	})

	t.Run("unix milli non numeric returns query violation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?ms=abcdefghijklm", nil)

		_, err := Query(req, "ms").UnixMilliTime().Get()
		assertInvalidViolationAt(t, err, "ms", errx.ViolationInQuery)
	})
}

func TestDurationParam_ValidationAndRangeErrors(t *testing.T) {
	t.Run("duration boundary success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?timeout=5s", nil), "timeout").Duration().
			Required().
			Min(5 * time.Second).
			Max(5 * time.Second).
			Check(func(value time.Duration) error { return nil }).
			Get()
		if err != nil {
			t.Fatalf("Duration().Required().Min().Max().Check().Get() error = %v", err)
		}
		if got != 5*time.Second {
			t.Fatalf("duration = %v, want 5s", got)
		}
	})

	t.Run("duration default success", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "timeout").Duration().Default(1500 * time.Millisecond).Get()
		if err != nil || got != 1500*time.Millisecond {
			t.Fatalf("Duration().Default().Get() = (%v, %v), want (1.5s, nil)", got, err)
		}
	})

	t.Run("duration empty string parses zero when required", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?timeout=", nil), "timeout").Duration().Required().Get()
		if err != nil || got != 0 {
			t.Fatalf("Duration().Required().Get() = (%v, %v), want (0, nil)", got, err)
		}
	})

	t.Run("duration default check invalid uses custom detail", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "timeout").Duration().
			Default(1500 * time.Millisecond).
			Check(func(value time.Duration) error {
				return errors.New("default duration must be rejected")
			}).
			Get()
		assertViolation(t, err, errx.Violation{
			Field:  "timeout",
			In:     errx.ViolationInQuery,
			Code:   errx.ViolationCodeInvalid,
			Detail: "default duration must be rejected",
		})
	})

	t.Run("duration min invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?timeout=1s", nil), "timeout").Duration().Min(2 * time.Second).Get()
		assertInvalidViolationAt(t, err, "timeout", errx.ViolationInQuery)
	})

	t.Run("duration max invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?timeout=3s", nil), "timeout").Duration().Max(2 * time.Second).Get()
		assertInvalidViolationAt(t, err, "timeout", errx.ViolationInQuery)
	})

	t.Run("duration max then min conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?timeout=3s", nil), "timeout").Duration().Max(1 * time.Second).Min(2 * time.Second).Get()
		assertUsageErrorContains(t, err, "minimum must be less than or equal to maximum")
	})

	t.Run("duration min then max conflict", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?timeout=3s", nil), "timeout").Duration().Min(2 * time.Second).Max(1 * time.Second).Get()
		assertUsageErrorContains(t, err, "maximum must be greater than or equal to minimum")
	})

	t.Run("duration parse invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?timeout=soon", nil), "timeout").Duration().Get()
		assertInvalidViolationAt(t, err, "timeout", errx.ViolationInQuery)
	})
}

func TestTimeParam_ValidationAndRangeErrors(t *testing.T) {
	t.Run("time required range and check success", func(t *testing.T) {
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
	})

	t.Run("time default success", func(t *testing.T) {
		at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "at").Time().Default(at).Get()
		if err != nil || !got.Equal(at) {
			t.Fatalf("Time().Default().Get() = (%v, %v), want (%v, nil)", got, err, at)
		}
	})

	t.Run("time required default conflict short-circuits usage error", func(t *testing.T) {
		at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "at").Time().
			Required().
			Default(at).
			Get()
		assertUsageErrorContains(t, err, "required and default are mutually exclusive")
	})

	t.Run("time default still honors after", func(t *testing.T) {
		at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items", nil), "at").Time().
			Default(at).
			After(at.Add(time.Minute)).
			Get()
		assertInvalidViolationAt(t, err, "at", errx.ViolationInQuery)
	})

	t.Run("time after invalid", func(t *testing.T) {
		from := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)
		at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+from.Format(time.RFC3339), nil), "at").Time().After(at).Get()
		assertInvalidViolationAt(t, err, "at", errx.ViolationInQuery)
	})

	t.Run("time before invalid", func(t *testing.T) {
		at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
		to := time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+to.Format(time.RFC3339), nil), "at").Time().Before(at).Get()
		assertInvalidViolationAt(t, err, "at", errx.ViolationInQuery)
	})

	t.Run("time before then after conflict", func(t *testing.T) {
		from := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)
		to := time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+to.Format(time.RFC3339), nil), "at").Time().Before(from).After(to).Get()
		assertUsageErrorContains(t, err, "after time must be less than or equal to before time")
	})

	t.Run("time after then before conflict", func(t *testing.T) {
		from := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)
		to := time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+to.Format(time.RFC3339), nil), "at").Time().After(to).Before(from).Get()
		assertUsageErrorContains(t, err, "before time must be greater than or equal to after time")
	})

	t.Run("time parse invalid", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at=not-a-time", nil), "at").Time().Get()
		assertInvalidViolationAt(t, err, "at", errx.ViolationInQuery)
	})

	t.Run("latest after overrides earlier after check", func(t *testing.T) {
		at := time.Date(2026, 4, 13, 10, 30, 0, 0, time.UTC)

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+at.Format(time.RFC3339), nil), "at").Time().
			After(time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)).
			After(time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)).
			Before(time.Date(2026, 4, 13, 10, 45, 0, 0, time.UTC)).
			Get()
		if err != nil {
			t.Fatalf("Time().After().After().Before().Get() error = %v", err)
		}
		if !got.Equal(at) {
			t.Fatalf("time = %v, want %v", got, at)
		}
	})

	t.Run("later time bounds can recover from earlier conflict", func(t *testing.T) {
		at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+at.Format(time.RFC3339), nil), "at").Time().
			After(time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)).
			Before(at).
			After(time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)).
			Get()
		if err != nil {
			t.Fatalf("Time().After().Before().After().Get() error = %v", err)
		}
		if !got.Equal(at) {
			t.Fatalf("time = %v, want %v", got, at)
		}
	})

	t.Run("repeated after after check preserves custom detail precedence", func(t *testing.T) {
		at := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?at="+at.Format(time.RFC3339), nil), "at").Time().
			After(time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)).
			Check(func(value time.Time) error {
				if value.Equal(at) {
					return errors.New("custom time detail")
				}
				return nil
			}).
			After(time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)).
			Get()
		assertViolation(t, err, errx.Violation{
			Field:  "at",
			In:     errx.ViolationInQuery,
			Code:   errx.ViolationCodeInvalid,
			Detail: "custom time detail",
		})
	})
}
