package resp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/kanata996/hah/errx"
)

type httpErrorAsCarrier struct {
	httpErr *errx.HTTPError
}

func (e httpErrorAsCarrier) Error() string {
	return "http error carrier"
}

func (e httpErrorAsCarrier) As(target any) bool {
	httpErr, ok := target.(**errx.HTTPError)
	if !ok {
		return false
	}
	*httpErr = e.httpErr
	return true
}

func TestWriteErrorWritesEnvelope(t *testing.T) {
	rr := httptest.NewRecorder()

	err := WriteError(rr, errx.NewHTTPError(
		http.StatusUnprocessableEntity,
		"",
		"",
	).WithViolations([]errx.Violation{
		{Field: "name", In: errx.InBody, Code: "required", Detail: "is required"},
	}))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnprocessableEntity)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "unprocessable_entity" {
		t.Fatalf("code = %#v, want unprocessable_entity", got)
	}
	if got := body["title"]; got != "Unprocessable Entity" {
		t.Fatalf("title = %#v, want Unprocessable Entity", got)
	}
	if got := body["status"]; got != float64(http.StatusUnprocessableEntity) {
		t.Fatalf("status = %#v, want %d", got, http.StatusUnprocessableEntity)
	}
	if got := body["detail"]; got != "Unprocessable Entity" {
		t.Fatalf("detail = %#v, want Unprocessable Entity", got)
	}
	errorsValue, ok := body["errors"].([]any)
	if !ok || len(errorsValue) != 1 {
		t.Fatalf("errors = %#v, want 1 item", body["errors"])
	}
	assertPublicErrorObject(t, errorsValue[0], map[string]any{
		"field":  "name",
		"in":     "body",
		"code":   "required",
		"detail": "is required",
	})
}

func TestWriteErrorWritesNotFoundProblem(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := WriteError(rr, errx.NewHTTPError(http.StatusNotFound, "", "")); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["title"]; got != http.StatusText(http.StatusNotFound) {
		t.Fatalf("title = %#v, want %q", got, http.StatusText(http.StatusNotFound))
	}
	if got := body["status"]; got != float64(http.StatusNotFound) {
		t.Fatalf("status = %#v, want %d", got, http.StatusNotFound)
	}
	if got := body["code"]; got != "not_found" {
		t.Fatalf("code = %#v, want not_found", got)
	}
}

func TestWriteErrorPreservesWrappedHTTPErrorFields(t *testing.T) {
	rr := httptest.NewRecorder()

	input := errors.Join(
		errors.New("handler failed"),
		errx.NewHTTPError(http.StatusBadRequest, "invalid_json", "payload invalid").WithViolations([]errx.Violation{
			{Field: "name", Code: "required", Detail: "is required"},
		}),
	)
	if err := WriteError(rr, input); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "invalid_json" {
		t.Fatalf("code = %#v, want invalid_json", got)
	}
	if got := body["title"]; got != http.StatusText(http.StatusBadRequest) {
		t.Fatalf("title = %#v, want %q", got, http.StatusText(http.StatusBadRequest))
	}
	if got := body["detail"]; got != "payload invalid" {
		t.Fatalf("detail = %#v, want payload invalid", got)
	}
	errorsValue, ok := body["errors"].([]any)
	if !ok || len(errorsValue) != 1 {
		t.Fatalf("errors = %#v, want 1 item", body["errors"])
	}
	assertPublicErrorObject(t, errorsValue[0], map[string]any{
		"field":  "name",
		"code":   "required",
		"detail": "is required",
	})
}

func TestWriteErrorPreservesViolationOrderAndContent(t *testing.T) {
	rr := httptest.NewRecorder()

	err := WriteError(rr, errx.NewHTTPError(http.StatusUnprocessableEntity, "", "").WithViolations([]errx.Violation{
		{Field: "name", In: errx.InBody, Code: errx.CodeRequired, Detail: "is required"},
		{Field: "email", In: errx.InQuery, Code: errx.CodeInvalid, Detail: "is invalid"},
		{Field: "name", In: errx.InBody, Code: errx.CodeInvalid, Detail: "must be unique"},
	}))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	body := decodePayload(t, rr.Body.Bytes())
	errorsValue, ok := body["errors"].([]any)
	if !ok || len(errorsValue) != 3 {
		t.Fatalf("errors = %#v, want 3 items", body["errors"])
	}

	assertPublicErrorObject(t, errorsValue[0], map[string]any{
		"field":  "name",
		"in":     "body",
		"code":   "required",
		"detail": "is required",
	})
	assertPublicErrorObject(t, errorsValue[1], map[string]any{
		"field":  "email",
		"in":     "query",
		"code":   "invalid",
		"detail": "is invalid",
	})
	assertPublicErrorObject(t, errorsValue[2], map[string]any{
		"field":  "name",
		"in":     "body",
		"code":   "invalid",
		"detail": "must be unique",
	})
}

func TestWriteErrorUsesCustomAsHTTPError(t *testing.T) {
	rr := httptest.NewRecorder()

	input := httpErrorAsCarrier{
		httpErr: errx.NewHTTPError(http.StatusUnauthorized, "unauthorized", "token missing"),
	}
	if err := WriteError(rr, input); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	if got := body["code"]; got != "unauthorized" {
		t.Fatalf("code = %#v, want unauthorized", got)
	}
	if got := body["title"]; got != http.StatusText(http.StatusUnauthorized) {
		t.Fatalf("title = %#v, want %q", got, http.StatusText(http.StatusUnauthorized))
	}
	if got := body["detail"]; got != "token missing" {
		t.Fatalf("detail = %#v, want token missing", got)
	}
}

func TestWriteErrorMapsContextAndUnknownErrors(t *testing.T) {
	t.Run("context canceled", func(t *testing.T) {
		rr := httptest.NewRecorder()

		if err := WriteError(rr, context.Canceled); err != nil {
			t.Fatalf("WriteError() error = %v", err)
		}

		body := decodePayload(t, rr.Body.Bytes())
		if rr.Code != 499 {
			t.Fatalf("status = %d, want 499", rr.Code)
		}
		if got := body["code"]; got != "client_closed_request" {
			t.Fatalf("code = %#v, want client_closed_request", got)
		}
		if _, exists := body["detail"]; exists {
			t.Fatalf("detail unexpectedly present: %#v", body["detail"])
		}
		if _, exists := body["errors"]; exists {
			t.Fatalf("errors unexpectedly present: %#v", body["errors"])
		}
	})

	t.Run("context deadline exceeded", func(t *testing.T) {
		rr := httptest.NewRecorder()

		if err := WriteError(rr, context.DeadlineExceeded); err != nil {
			t.Fatalf("WriteError() error = %v", err)
		}

		body := decodePayload(t, rr.Body.Bytes())
		if rr.Code != http.StatusGatewayTimeout {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusGatewayTimeout)
		}
		if got := body["code"]; got != "timeout" {
			t.Fatalf("code = %#v, want timeout", got)
		}
		if _, exists := body["detail"]; exists {
			t.Fatalf("detail unexpectedly present: %#v", body["detail"])
		}
		if _, exists := body["errors"]; exists {
			t.Fatalf("errors unexpectedly present: %#v", body["errors"])
		}
	})

	t.Run("http error wins over context error", func(t *testing.T) {
		rr := httptest.NewRecorder()

		input := errors.Join(
			context.Canceled,
			errx.NewHTTPError(http.StatusForbidden, "forbidden", "access denied"),
		)
		if err := WriteError(rr, input); err != nil {
			t.Fatalf("WriteError() error = %v", err)
		}

		body := decodePayload(t, rr.Body.Bytes())
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
		}
		if got := body["code"]; got != "forbidden" {
			t.Fatalf("code = %#v, want forbidden", got)
		}
		if got := body["detail"]; got != "access denied" {
			t.Fatalf("detail = %#v, want access denied", got)
		}
	})

	t.Run("unknown error becomes internal error", func(t *testing.T) {
		rr := httptest.NewRecorder()

		if err := WriteError(rr, errors.New("db timeout")); err != nil {
			t.Fatalf("WriteError() error = %v", err)
		}

		body := decodePayload(t, rr.Body.Bytes())
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
		}
		if got := body["code"]; got != "internal_error" {
			t.Fatalf("code = %#v, want internal_error", got)
		}
		if _, exists := body["detail"]; exists {
			t.Fatalf("detail unexpectedly present: %#v", body["detail"])
		}
		if _, exists := body["errors"]; exists {
			t.Fatalf("errors unexpectedly present: %#v", body["errors"])
		}
		if bytes.Contains(rr.Body.Bytes(), []byte("db timeout")) {
			t.Fatalf("body leaked internal cause: %q", rr.Body.String())
		}
	})
}

func TestWriteErrorResponseBoundaries(t *testing.T) {
	t.Run("nil error is noop", func(t *testing.T) {
		rr := httptest.NewRecorder()

		if err := WriteError(rr, nil); err != nil {
			t.Fatalf("WriteError() error = %v", err)
		}
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want recorder default %d", rr.Code, http.StatusOK)
		}
		if rr.Body.Len() != 0 {
			t.Fatalf("body = %q, want empty", rr.Body.String())
		}
	})

	t.Run("nil writer rejects non nil error", func(t *testing.T) {
		if err := WriteError(nil, errors.New("db timeout")); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("nil writer and nil error is noop", func(t *testing.T) {
		if err := WriteError(nil, nil); err != nil {
			t.Fatalf("WriteError() error = %v, want nil", err)
		}
	})

	t.Run("head uses net http head semantics", func(t *testing.T) {
		httpErr := errx.NewHTTPError(http.StatusBadRequest, "", "").WithViolations([]errx.Violation{
			{Field: "name", Code: "required", Detail: "is required"},
		})

		result := roundTripOverHTTPMethod(t, http.MethodHead, func(w http.ResponseWriter, _ *http.Request) error {
			return WriteError(w, httpErr)
		})

		if result.handlerErr != nil {
			t.Fatalf("handler error = %v", result.handlerErr)
		}
		if result.response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", result.response.StatusCode, http.StatusBadRequest)
		}
		if got := result.response.Header.Get("Content-Type"); got != "application/problem+json" {
			t.Fatalf("Content-Type = %q, want %q", got, "application/problem+json")
		}
		if len(result.body) != 0 {
			t.Fatalf("body = %q, want empty for HEAD", string(result.body))
		}
	})

	t.Run("preserves unrelated headers and owns content headers", func(t *testing.T) {
		rr := httptest.NewRecorder()
		rr.Header().Set("X-Trace-ID", "trace-1")
		rr.Header().Set("Content-Type", "text/plain")
		rr.Header().Set("Content-Length", "999")

		err := WriteError(rr, errx.NewHTTPError(http.StatusBadRequest, "invalid_json", "payload invalid"))
		if err != nil {
			t.Fatalf("WriteError() error = %v", err)
		}

		if got := rr.Header().Get("X-Trace-ID"); got != "trace-1" {
			t.Fatalf("X-Trace-ID = %q, want %q", got, "trace-1")
		}
		if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
			t.Fatalf("Content-Type = %q, want %q", got, "application/problem+json")
		}
		if got := rr.Header().Get("Content-Length"); got != "" {
			t.Fatalf("Content-Length = %q, want empty before net/http recalculates it", got)
		}
	})
}

func TestWriteErrorWriteFailureAndFallback(t *testing.T) {
	t.Run("returns write failure after first commit", func(t *testing.T) {
		cause := errors.New("socket closed")
		w := &failingWriter{cause: cause}
		err := WriteError(w, errx.NewHTTPError(http.StatusInternalServerError, "internal_error", "Internal Server Error"))
		if err == nil {
			t.Fatal("expected write error, got nil")
		}
		if !errors.Is(err, cause) {
			t.Fatalf("errors.Is(err, cause) = false, want true")
		}
		if w.status != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.status, http.StatusInternalServerError)
		}
		if w.writes != 1 {
			t.Fatalf("writes = %d, want 1", w.writes)
		}
	})

	t.Run("falls back to internal error when problem encoding fails", func(t *testing.T) {
		t.Cleanup(func() {
			problemBodyEncoder = encodeJSON
		})
		problemBodyEncoder = func(any) ([]byte, error) {
			return nil, errors.New("encode payload failed")
		}

		rr := httptest.NewRecorder()
		err := WriteError(rr, errx.NewHTTPError(
			http.StatusBadRequest,
			"invalid_json",
			"payload invalid",
		).WithViolations([]errx.Violation{
			{Field: "name", Code: "required", Detail: "is required"},
		}))
		if err != nil {
			t.Fatalf("WriteError() error = %v, want nil after fallback write", err)
		}

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
		}
		if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
			t.Fatalf("Content-Type = %q, want application/problem+json", got)
		}

		payload := decodePayload(t, rr.Body.Bytes())
		if got := payload["code"]; got != "internal_error" {
			t.Fatalf("code = %#v, want internal_error", got)
		}
		if _, exists := payload["detail"]; exists {
			t.Fatalf("detail unexpectedly present: %#v", payload["detail"])
		}
		if _, exists := payload["errors"]; exists {
			t.Fatalf("errors unexpectedly present: %#v", payload["errors"])
		}
	})

	t.Run("clears stale content length on real http server", func(t *testing.T) {
		httpErr := errx.NewHTTPError(http.StatusBadRequest, "", "").WithViolations([]errx.Violation{
			{Field: "name", Code: "required", Detail: "is required"},
		})

		expected := httptest.NewRecorder()
		if err := WriteError(expected, httpErr); err != nil {
			t.Fatalf("WriteError() expected recorder error = %v", err)
		}

		result := roundTripOverHTTP(t, func(w http.ResponseWriter, _ *http.Request) error {
			w.Header().Set("Content-Length", "100")
			return WriteError(w, httpErr)
		})

		if result.handlerErr != nil {
			t.Fatalf("handler error = %v", result.handlerErr)
		}
		if result.response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", result.response.StatusCode, http.StatusBadRequest)
		}
		if got := string(result.body); got != expected.Body.String() {
			t.Fatalf("body = %q, want %q", got, expected.Body.String())
		}
		if got := result.response.Header.Get("Content-Length"); got != "" && got != strconv.Itoa(len(result.body)) {
			t.Fatalf("Content-Length = %q, want empty or %d", got, len(result.body))
		}
	})
}
