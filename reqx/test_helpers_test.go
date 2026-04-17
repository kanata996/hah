package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

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

func assertViolations(t *testing.T, err error) []errx.Violation {
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

func assertSingleViolation(t *testing.T, err error) errx.Violation {
	t.Helper()

	violations := assertViolations(t, err)
	if len(violations) != 1 {
		t.Fatalf("violations len = %d, want 1", len(violations))
	}
	return violations[0]
}

func assertViolation(t *testing.T, err error, want errx.Violation) {
	t.Helper()

	if got := assertSingleViolation(t, err); got != want {
		t.Fatalf("violation = %#v, want %#v", got, want)
	}
}

func assertInvalidViolationAt(t *testing.T, err error, field string, in errx.ViolationIn) {
	t.Helper()

	assertViolation(t, err, errx.Violation{
		Field:  field,
		In:     in,
		Code:   errx.CodeInvalid,
		Detail: "is invalid",
	})
}

func assertRequiredViolationAt(t *testing.T, err error, field string, in errx.ViolationIn) {
	t.Helper()

	assertViolation(t, err, errx.Violation{
		Field:  field,
		In:     in,
		Code:   errx.CodeRequired,
		Detail: "is required",
	})
}
