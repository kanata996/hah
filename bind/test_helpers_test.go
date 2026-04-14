package bind

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

const (
	wantNilRequestErr     = "bind: request must not be nil"
	wantNilDestinationErr = "bind: destination must not be nil"
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

func assertHTTPError(t *testing.T, err error, wantStatus int, wantCode, wantDetail string) *errx.HTTPError {
	t.Helper()

	httpErr := assertHTTPErrorLike(t, err)
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
