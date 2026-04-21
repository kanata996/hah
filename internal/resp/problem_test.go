package resp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/kanata996/hah/internal/errx"
)

type httpErrorAsCarrier struct {
	httpErr *errx.HTTPError
	child   error
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

func (e httpErrorAsCarrier) Unwrap() error {
	return e.child
}

func TestWriteErrorWritesDefaultEnvelope(t *testing.T) {
	rr := httptest.NewRecorder()

	err := WriteError(rr, errx.NewHTTPError(
		http.StatusUnprocessableEntity,
		"",
		"",
	).WithFieldErrors([]errx.FieldError{
		{Field: "name", In: errx.InBody, Code: "required", Detail: "is required"},
	}))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	assertErrorEnvelopeBasics(t, rr, http.StatusUnprocessableEntity, 42200, "unprocessable entity")

	body := decodePayload(t, rr.Body.Bytes())
	if _, exists := body["data"]; exists {
		t.Fatalf("data unexpectedly present: %#v", body["data"])
	}

	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	assertPublicErrorObject(t, errorValue, map[string]any{
		"reason": "unprocessable_entity",
		"details": []any{
			map[string]any{
				"field":  "name",
				"in":     "body",
				"code":   "required",
				"detail": "is required",
			},
		},
	})
}

func TestWriteErrorUsesExplicitTopCodeAndDetailDerivedMessage(t *testing.T) {
	rr := httptest.NewRecorder()

	input := errors.Join(
		errors.New("handler failed"),
		errx.NewHTTPError(http.StatusBadRequest, "invalid_json", "payload invalid").WithFieldErrors([]errx.FieldError{
			{Field: "name", Code: "required", Detail: "is required"},
		}),
	)
	if err := WriteError(rr, input, 40001); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	assertErrorEnvelopeBasics(t, rr, http.StatusBadRequest, 40001, "payload invalid")

	body := decodePayload(t, rr.Body.Bytes())
	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	if _, exists := errorValue["detail"]; exists {
		t.Fatalf("error.detail unexpectedly present: %#v", errorValue["detail"])
	}
	if _, exists := errorValue["title"]; exists {
		t.Fatalf("error.title unexpectedly present: %#v", errorValue["title"])
	}
	if got := errorValue["reason"]; got != "invalid_json" {
		t.Fatalf("error.reason = %#v, want invalid_json", got)
	}
	details, ok := errorValue["details"].([]any)
	if !ok || len(details) != 1 {
		t.Fatalf("details = %#v, want 1 item", errorValue["details"])
	}
	assertPublicErrorObject(t, details[0], map[string]any{
		"field":  "name",
		"code":   "required",
		"detail": "is required",
	})
}

func TestWriteErrorUsesNormalizedDetailAsMessageWhenDetailIsAbsent(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := WriteError(rr, errx.NewHTTPError(http.StatusUnauthorized, "token-missing__forbidden", "")); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	assertErrorEnvelopeBasics(t, rr, http.StatusUnauthorized, 40100, "token missing forbidden")

	body := decodePayload(t, rr.Body.Bytes())
	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	assertPublicErrorObject(t, errorValue, map[string]any{
		"reason": "token-missing__forbidden",
	})
}

func TestWriteErrorUsesExplicitDetailEvenWhenItMatchesTitle(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := WriteError(rr, errx.NewHTTPError(http.StatusBadRequest, "invalid_json", "Bad Request")); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	assertErrorEnvelopeBasics(t, rr, http.StatusBadRequest, 40000, "Bad Request")

	body := decodePayload(t, rr.Body.Bytes())
	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	assertPublicErrorObject(t, errorValue, map[string]any{
		"reason": "invalid_json",
	})
}

func TestWriteErrorPreservesFieldErrorOrderAndContent(t *testing.T) {
	rr := httptest.NewRecorder()

	err := WriteError(rr, errx.NewHTTPError(http.StatusUnprocessableEntity, "", "").WithFieldErrors([]errx.FieldError{
		{Field: "name", In: errx.InBody, Code: errx.CodeRequired, Detail: "is required"},
		{Field: "email", In: errx.InQuery, Code: errx.CodeInvalid, Detail: "is invalid"},
		{Field: "name", In: errx.InBody, Code: errx.CodeInvalid, Detail: "must be unique"},
	}))
	if err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	body := decodePayload(t, rr.Body.Bytes())
	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	details, ok := errorValue["details"].([]any)
	if !ok || len(details) != 3 {
		t.Fatalf("details = %#v, want 3 items", errorValue["details"])
	}

	assertPublicErrorObject(t, details[0], map[string]any{
		"field":  "name",
		"in":     "body",
		"code":   "required",
		"detail": "is required",
	})
	assertPublicErrorObject(t, details[1], map[string]any{
		"field":  "email",
		"in":     "query",
		"code":   "invalid",
		"detail": "is invalid",
	})
	assertPublicErrorObject(t, details[2], map[string]any{
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

	assertErrorEnvelopeBasics(t, rr, http.StatusUnauthorized, 40100, "token missing")

	body := decodePayload(t, rr.Body.Bytes())
	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	assertPublicErrorObject(t, errorValue, map[string]any{
		"reason": "unauthorized",
	})
}

func TestWriteErrorSkipsTypedNilHTTPErrorAndKeepsScanningChain(t *testing.T) {
	var typedNil *errx.HTTPError

	input := fmt.Errorf("wrapped: %w", errors.Join(
		typedNil,
		errx.NewHTTPError(http.StatusBadRequest, "invalid_json", "payload invalid"),
	))
	rr := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("WriteError() panicked: %v", recovered)
		}
	}()

	if err := WriteError(rr, input); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	assertErrorEnvelopeBasics(t, rr, http.StatusBadRequest, 40000, "payload invalid")

	body := decodePayload(t, rr.Body.Bytes())
	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	assertPublicErrorObject(t, errorValue, map[string]any{
		"reason": "invalid_json",
	})
}

func TestWriteErrorSkipsTypedNilCustomAsHTTPErrorAndKeepsScanningChain(t *testing.T) {
	var typedNil *errx.HTTPError

	rr := httptest.NewRecorder()
	input := httpErrorAsCarrier{
		httpErr: typedNil,
		child:   errx.NewHTTPError(http.StatusForbidden, "forbidden", "access denied"),
	}

	if err := WriteError(rr, input); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	assertErrorEnvelopeBasics(t, rr, http.StatusForbidden, 40300, "access denied")

	body := decodePayload(t, rr.Body.Bytes())
	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	assertPublicErrorObject(t, errorValue, map[string]any{
		"reason": "forbidden",
	})
}

func TestWriteErrorMapsContextAndUnknownErrors(t *testing.T) {
	t.Run("context canceled", func(t *testing.T) {
		rr := httptest.NewRecorder()

		if err := WriteError(rr, context.Canceled); err != nil {
			t.Fatalf("WriteError() error = %v", err)
		}

		assertErrorEnvelopeBasics(t, rr, 499, 49900, "client closed request")

		body := decodePayload(t, rr.Body.Bytes())
		errorValue := body["error"].(map[string]any)
		assertPublicErrorObject(t, errorValue, map[string]any{
			"reason": "client_closed_request",
		})
	})

	t.Run("context deadline exceeded", func(t *testing.T) {
		rr := httptest.NewRecorder()

		if err := WriteError(rr, context.DeadlineExceeded); err != nil {
			t.Fatalf("WriteError() error = %v", err)
		}

		assertErrorEnvelopeBasics(t, rr, http.StatusGatewayTimeout, 50400, "timeout")

		body := decodePayload(t, rr.Body.Bytes())
		errorValue := body["error"].(map[string]any)
		assertPublicErrorObject(t, errorValue, map[string]any{
			"reason": "timeout",
		})
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

		assertErrorEnvelopeBasics(t, rr, http.StatusForbidden, 40300, "access denied")

		body := decodePayload(t, rr.Body.Bytes())
		errorValue := body["error"].(map[string]any)
		assertPublicErrorObject(t, errorValue, map[string]any{
			"reason": "forbidden",
		})
	})

	t.Run("unknown error becomes internal error without leak", func(t *testing.T) {
		rr := httptest.NewRecorder()

		if err := WriteError(rr, errors.New("db timeout")); err != nil {
			t.Fatalf("WriteError() error = %v", err)
		}

		assertErrorEnvelopeBasics(t, rr, http.StatusInternalServerError, 50000, "internal error")

		body := decodePayload(t, rr.Body.Bytes())
		errorValue := body["error"].(map[string]any)
		assertPublicErrorObject(t, errorValue, map[string]any{
			"reason": "internal_error",
		})
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

	t.Run("nil error ignores explicit code and stays noop", func(t *testing.T) {
		rr := httptest.NewRecorder()

		if err := WriteError(rr, nil, 40001); err != nil {
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
		if err := WriteError(nil, nil, 40001); err != nil {
			t.Fatalf("WriteError() error = %v, want nil", err)
		}
	})

	t.Run("rejects non positive top code before commit", func(t *testing.T) {
		rr := httptest.NewRecorder()

		if err := WriteError(rr, errx.BadRequest("", ""), 0); err == nil {
			t.Fatal("expected error, got nil")
		}
		assertRecorderHasNoBodyOrContentType(t, rr)
	})

	t.Run("rejects non five digit top code before commit", func(t *testing.T) {
		cases := []int{9999, 100000}

		for _, code := range cases {
			t.Run(strconv.Itoa(code), func(t *testing.T) {
				rr := httptest.NewRecorder()

				if err := WriteError(rr, errx.BadRequest("", ""), code); err == nil {
					t.Fatal("expected error, got nil")
				}
				assertRecorderHasNoBodyOrContentType(t, rr)
			})
		}
	})

	t.Run("rejects multiple top codes before commit", func(t *testing.T) {
		rr := httptest.NewRecorder()

		if err := WriteError(rr, errx.BadRequest("", ""), 40001, 40002); err == nil {
			t.Fatal("expected error, got nil")
		}
		assertRecorderHasNoBodyOrContentType(t, rr)
	})

	t.Run("head uses net http head semantics", func(t *testing.T) {
		httpErr := errx.NewHTTPError(http.StatusBadRequest, "", "").WithFieldErrors([]errx.FieldError{
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
		if got := result.response.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", got, "application/json")
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
		if got := rr.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", got, "application/json")
		}
		if got := rr.Header().Get("Content-Length"); got != "" {
			t.Fatalf("Content-Length = %q, want empty before net/http recalculates it", got)
		}
	})
}

func TestWriteErrorWriteFailure(t *testing.T) {
	t.Run("returns encode failure before first commit", func(t *testing.T) {
		rr := httptest.NewRecorder()
		cause := errors.New("encode failed")

		original := encodeErrorEnvelope
		encodeErrorEnvelope = func(responseEnvelope) ([]byte, error) {
			return nil, cause
		}
		t.Cleanup(func() {
			encodeErrorEnvelope = original
		})

		err := WriteError(rr, errx.NewHTTPError(http.StatusBadRequest, "invalid_json", "payload invalid"))
		if err == nil {
			t.Fatal("expected encode error, got nil")
		}
		if !errors.Is(err, cause) {
			t.Fatalf("errors.Is(err, cause) = false, want true")
		}
		assertRecorderHasNoBodyOrContentType(t, rr)
	})

	t.Run("returns write failure after first commit", func(t *testing.T) {
		cause := errors.New("socket closed")
		w := &failingWriter{cause: cause}
		err := WriteError(w, errx.NewHTTPError(http.StatusInternalServerError, "internal_error", "internal error"))
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

	t.Run("clears stale content length on real http server", func(t *testing.T) {
		httpErr := errx.NewHTTPError(http.StatusBadRequest, "", "").WithFieldErrors([]errx.FieldError{
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

func assertErrorEnvelopeBasics(t *testing.T, rr *httptest.ResponseRecorder, wantStatus, wantCode int, wantMessage string) {
	t.Helper()

	if rr.Code != wantStatus {
		t.Fatalf("status = %d, want %d", rr.Code, wantStatus)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != float64(wantCode) {
		t.Fatalf("code = %#v, want %d", got, wantCode)
	}
	if got := body["message"]; got != wantMessage {
		t.Fatalf("message = %#v, want %q", got, wantMessage)
	}
	if _, exists := body["data"]; exists {
		t.Fatalf("data unexpectedly present: %#v", body["data"])
	}
	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	if _, exists := errorValue["status"]; exists {
		t.Fatalf("error.status unexpectedly present: %#v", errorValue["status"])
	}
}
