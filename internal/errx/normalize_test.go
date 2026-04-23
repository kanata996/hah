package errx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

type httpErrorCarrier struct {
	httpErr *HTTPError
	child   error
}

func (e httpErrorCarrier) Error() string {
	return "http error carrier"
}

func (e httpErrorCarrier) As(target any) bool {
	httpErr, ok := target.(**HTTPError)
	if !ok {
		return false
	}
	*httpErr = e.httpErr
	return true
}

func (e httpErrorCarrier) Unwrap() error {
	return e.child
}

func assertNormalizedHTTPError(t *testing.T, err *HTTPError, wantStatus int, wantCode, wantDetail string) {
	t.Helper()

	if got := err.Status(); got != wantStatus {
		t.Fatalf("Status() = %d, want %d", got, wantStatus)
	}
	if got := err.Code(); got != wantCode {
		t.Fatalf("Code() = %q, want %q", got, wantCode)
	}
	if got := err.Detail(); got != wantDetail {
		t.Fatalf("Detail() = %q, want %q", got, wantDetail)
	}
}

func TestNormalizeHTTPError(t *testing.T) {
	t.Run("returns direct http error", func(t *testing.T) {
		input := NewHTTPError(http.StatusBadRequest, "invalid_json", "payload invalid")

		got := NormalizeHTTPError(input)

		if got != input {
			t.Fatal("NormalizeHTTPError() should reuse the visible HTTPError")
		}
		assertNormalizedHTTPError(t, got, http.StatusBadRequest, "invalid_json", "payload invalid")
	})

	t.Run("uses custom as http error", func(t *testing.T) {
		got := NormalizeHTTPError(httpErrorCarrier{
			httpErr: NewHTTPError(http.StatusUnauthorized, "unauthorized", "token missing"),
		})

		assertNormalizedHTTPError(t, got, http.StatusUnauthorized, "unauthorized", "token missing")
	})

	t.Run("prefers first http error in joined chain", func(t *testing.T) {
		got := NormalizeHTTPError(errors.Join(
			NewHTTPError(http.StatusBadRequest, "invalid_json", "payload invalid"),
			NewHTTPError(http.StatusForbidden, "forbidden", "access denied"),
		))

		assertNormalizedHTTPError(t, got, http.StatusBadRequest, "invalid_json", "payload invalid")
	})

	t.Run("skips typed nil http error and keeps scanning chain", func(t *testing.T) {
		var typedNil *HTTPError

		got := NormalizeHTTPError(fmt.Errorf("wrapped: %w", errors.Join(
			typedNil,
			NewHTTPError(http.StatusBadRequest, "invalid_json", "payload invalid"),
		)))

		assertNormalizedHTTPError(t, got, http.StatusBadRequest, "invalid_json", "payload invalid")
	})

	t.Run("skips typed nil custom as http error and keeps scanning chain", func(t *testing.T) {
		var typedNil *HTTPError

		got := NormalizeHTTPError(httpErrorCarrier{
			httpErr: typedNil,
			child:   NewHTTPError(http.StatusForbidden, "forbidden", "access denied"),
		})

		assertNormalizedHTTPError(t, got, http.StatusForbidden, "forbidden", "access denied")
	})

	t.Run("context canceled becomes client closed request", func(t *testing.T) {
		got := NormalizeHTTPError(context.Canceled)
		assertNormalizedHTTPError(t, got, 499, "client_closed_request", "client closed request")
	})

	t.Run("context deadline exceeded becomes timeout", func(t *testing.T) {
		got := NormalizeHTTPError(context.DeadlineExceeded)
		assertNormalizedHTTPError(t, got, http.StatusGatewayTimeout, "timeout", "timeout")
	})

	t.Run("http error wins over context error", func(t *testing.T) {
		got := NormalizeHTTPError(errors.Join(
			context.Canceled,
			NewHTTPError(http.StatusForbidden, "forbidden", "access denied"),
		))

		assertNormalizedHTTPError(t, got, http.StatusForbidden, "forbidden", "access denied")
	})

	t.Run("typed nil http error becomes internal error without panic", func(t *testing.T) {
		var typedNil *HTTPError

		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("NormalizeHTTPError() panicked: %v", recovered)
			}
		}()

		got := NormalizeHTTPError(typedNil)
		assertNormalizedHTTPError(t, got, http.StatusInternalServerError, "internal_error", "internal error")
	})

	t.Run("unknown error becomes internal error without leak", func(t *testing.T) {
		got := NormalizeHTTPError(errors.New("db timeout"))
		assertNormalizedHTTPError(t, got, http.StatusInternalServerError, "internal_error", "internal error")
	})
}
