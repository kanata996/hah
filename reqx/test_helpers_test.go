package reqx

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

const (
	wantNilRequestErr     = "reqx: request must not be nil"
	wantNilDestinationErr = "reqx: destination must not be nil"
)

func newJSONRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func setRequestBody(req *http.Request, contentType, body string) {
	req.Header.Set("Content-Type", contentType)
	req.Body = io.NopCloser(strings.NewReader(body))
	req.ContentLength = int64(len(body))
}

func mustBindQuery(t *testing.T, req *http.Request, target any) {
	t.Helper()

	if err := BindQuery(req, target); err != nil {
		t.Fatalf("BindQuery() error = %v", err)
	}
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

	var httpErr *errx.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		t.Fatalf("error type = %T, want non-HTTP usage error", err)
	}
}

func assertErrorString(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want %q", want)
	}
	if got := err.Error(); got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func assertUsageError(t *testing.T, err error, want string) {
	t.Helper()

	assertErrorString(t, err, want)
}

func assertBadRequest(t *testing.T, err error) *errx.HTTPError {
	t.Helper()

	return assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
}

func assertViolations(t *testing.T, err error) []Violation {
	t.Helper()

	httpErr := assertHTTPError(
		t,
		err,
		http.StatusUnprocessableEntity,
		CodeInvalidRequest,
		"request contains invalid fields",
	)

	details := httpErr.Errors()
	violations := make([]Violation, 0, len(details))
	for i, detail := range details {
		violation, ok := detail.(Violation)
		if !ok {
			t.Fatalf("detail[%d] type = %T, want reqx.Violation", i, detail)
		}
		violations = append(violations, violation)
	}
	return violations
}

func assertSingleViolation(t *testing.T, err error) Violation {
	t.Helper()

	violations := assertViolations(t, err)
	if len(violations) != 1 {
		t.Fatalf("details len = %d, want 1", len(violations))
	}
	return violations[0]
}
