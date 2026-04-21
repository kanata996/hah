package reqx

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/kanata996/hah/internal/errx"
)

type bindBodyReadErrorCloser struct{ err error }

func (r bindBodyReadErrorCloser) Read([]byte) (int, error) { return 0, r.err }
func (r bindBodyReadErrorCloser) Close() error             { return nil }

type bindBodyNoProgressThenErrorCloser struct {
	remaining int
	err       error
}

func (r *bindBodyNoProgressThenErrorCloser) Read([]byte) (int, error) {
	if r.remaining > 0 {
		r.remaining--
		return 0, nil
	}
	if r.err == nil {
		return 0, io.EOF
	}
	return 0, r.err
}

func (*bindBodyNoProgressThenErrorCloser) Close() error { return nil }

type bindQueryNamedString string
type bindQueryNamedSlice []string
type bindQueryNamedBool bool
type bindQueryNamedInt int32
type bindQueryNamedUint uint16
type bindQueryNamedFloat float32

type bindQueryTextValue string

func (*bindQueryTextValue) UnmarshalText([]byte) error { return nil }

func newJSONRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func requestWithPathParams(params map[string][]string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for name, values := range params {
		value := ""
		if len(values) > 0 {
			value = values[0]
		}
		req.SetPathValue(name, value)
	}
	req.Pattern = syntheticPatternFromPathParams(params)
	return req
}

func syntheticPatternFromPathParams(params map[string][]string) string {
	if len(params) == 0 {
		return "/"
	}

	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names)+1)
	parts = append(parts, "")
	for _, name := range names {
		parts = append(parts, "{"+name+"}")
	}
	return strings.Join(parts, "/")
}

func assertHTTPError(t *testing.T, err error, wantStatus int, wantCode, wantDetail string) *errx.HTTPError {
	t.Helper()

	httpErr := assertHTTPErrorLike(t, err)
	if httpErr == nil {
		t.Fatalf("error type = %T, want *errx.HTTPError", err)
	}
	if got := httpErr.Status(); got != wantStatus {
		t.Fatalf("status = %d, want %d", got, wantStatus)
	}
	if got := httpErr.Code(); got != wantCode {
		t.Fatalf("code = %q, want %q", got, wantCode)
	}
	if got := httpErr.Detail(); got != wantDetail {
		t.Fatalf("detail = %q, want %q", got, wantDetail)
	}
	return httpErr
}

func assertHTTPStatusCode(t *testing.T, err error, wantStatus int, wantCode string) *errx.HTTPError {
	t.Helper()

	httpErr := assertHTTPErrorLike(t, err)
	if got := httpErr.Status(); got != wantStatus {
		t.Fatalf("status = %d, want %d", got, wantStatus)
	}
	if got := httpErr.Code(); got != wantCode {
		t.Fatalf("code = %q, want %q", got, wantCode)
	}
	return httpErr
}

func assertHTTPErrorLike(t *testing.T, err error) *errx.HTTPError {
	t.Helper()

	var httpErr *errx.HTTPError
	if !errors.As(err, &httpErr) || httpErr == nil {
		t.Fatalf("error type = %T, want *errx.HTTPError", err)
	}
	return httpErr
}

func assertNotHTTPError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want ordinary non-HTTP error")
	}

	var httpErr *errx.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		t.Fatalf("error type = %T, want non-HTTP usage error", err)
	}
}

func assertFieldErrors(t *testing.T, err error) []FieldError {
	t.Helper()

	httpErr := assertHTTPError(
		t,
		err,
		http.StatusUnprocessableEntity,
		invalidRequestCode,
		"request contains invalid fields",
	)

	return httpErr.Errors()
}

func assertSingleFieldError(t *testing.T, err error) FieldError {
	t.Helper()

	fieldErrors := assertFieldErrors(t, err)
	if len(fieldErrors) != 1 {
		t.Fatalf("field errors len = %d, want 1", len(fieldErrors))
	}
	return fieldErrors[0]
}

func assertFieldError(t *testing.T, err error, want FieldError) {
	t.Helper()

	if got := assertSingleFieldError(t, err); got != want {
		t.Fatalf("field error = %#v, want %#v", got, want)
	}
}

func assertInvalidFieldErrorAt(t *testing.T, err error, field string, in FieldErrorIn) {
	t.Helper()

	assertFieldError(t, err, FieldError{
		Field:  field,
		In:     in,
		Code:   CodeInvalid,
		Detail: "is invalid",
	})
}

func assertRequiredFieldErrorAt(t *testing.T, err error, field string, in FieldErrorIn) {
	t.Helper()

	assertFieldError(t, err, FieldError{
		Field:  field,
		In:     in,
		Code:   CodeRequired,
		Detail: "is required",
	})
}
