package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func assertSameHTTPError(t *testing.T, gotErr, wantErr error) *errx.HTTPError {
	t.Helper()

	got := assertHTTPErrorLike(t, gotErr)
	want := assertHTTPErrorLike(t, wantErr)

	if got.Status() != want.Status() || got.Code() != want.Code() || got.Detail() != want.Detail() {
		t.Fatalf(
			"got error = (%d, %q, %q), want (%d, %q, %q)",
			got.Status(),
			got.Code(),
			got.Detail(),
			want.Status(),
			want.Code(),
			want.Detail(),
		)
	}
	if !reflect.DeepEqual(got.Errors(), want.Errors()) {
		t.Fatalf("got error details = %#v, want %#v", got.Errors(), want.Errors())
	}

	return got
}
