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

func TestWriteResponseUsesSharedResponseEncoder(t *testing.T) {
	rr := httptest.NewRecorder()

	err := writeResponse(rr, &Response{
		Status:  http.StatusOK,
		Code:    successTopCode,
		Message: successMessage,
		Data:    func() {},
	})
	if err == nil {
		t.Fatal("expected encode error, got nil")
	}
	if got := err.Error(); got != "json: unsupported type: func()" {
		t.Fatalf("error = %q, want %q", got, "json: unsupported type: func()")
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

func TestWriteResponseBoundaries(t *testing.T) {
	t.Run("nil writer rejects non nil response", func(t *testing.T) {
		err := writeResponse(nil, &Response{
			Status:  http.StatusOK,
			Code:    0,
			Message: "success",
		})
		if !errors.Is(err, errNilResponseWriter) {
			t.Fatalf("errors.Is(err, errNilResponseWriter) = false, err = %v", err)
		}
	})

	t.Run("encode failure happens before first commit", func(t *testing.T) {
		rr := httptest.NewRecorder()

		err := writeResponse(rr, &Response{
			Status:  http.StatusOK,
			Code:    0,
			Message: "success",
			Data:    make(chan int),
		})
		_ = assertUnsupportedTypeError(t, err)
		assertRecorderHasNoBodyOrContentType(t, rr)
	})
}

func TestSuccessWritersWriteEnvelopeResponses(t *testing.T) {
	type user struct {
		ID string `json:"id"`
	}
	var nilUser *user

	cases := []struct {
		name       string
		write      func(http.ResponseWriter) error
		wantStatus int
		assertData func(*testing.T, payloadMap)
	}{
		{
			name:       "Accepted writes object payload under data",
			write:      func(w http.ResponseWriter) error { return Accepted(w, map[string]any{"id": "u_1"}) },
			wantStatus: http.StatusAccepted,
			assertData: func(t *testing.T, body payloadMap) {
				t.Helper()
				data, ok := body["data"].(map[string]any)
				if !ok {
					t.Fatalf("data = %#v, want object", body["data"])
				}
				if got := data["id"]; got != "u_1" {
					t.Fatalf("data.id = %#v, want u_1", got)
				}
			},
		},
		{
			name:       "Created writes object payload under data",
			write:      func(w http.ResponseWriter) error { return Created(w, map[string]any{"id": "u_1"}) },
			wantStatus: http.StatusCreated,
			assertData: func(t *testing.T, body payloadMap) {
				t.Helper()
				data, ok := body["data"].(map[string]any)
				if !ok {
					t.Fatalf("data = %#v, want object", body["data"])
				}
				if got := data["id"]; got != "u_1" {
					t.Fatalf("data.id = %#v, want u_1", got)
				}
			},
		},
		{
			name:       "OK writes array payload under data",
			write:      func(w http.ResponseWriter) error { return OK(w, []string{"a", "b"}) },
			wantStatus: http.StatusOK,
			assertData: func(t *testing.T, body payloadMap) {
				t.Helper()
				data, ok := body["data"].([]any)
				if !ok || len(data) != 2 {
					t.Fatalf("data = %#v, want 2 item array", body["data"])
				}
				if data[0] != "a" || data[1] != "b" {
					t.Fatalf("data = %#v, want [a b]", data)
				}
			},
		},
		{
			name:       "OK writes scalar payload under data",
			write:      func(w http.ResponseWriter) error { return OK(w, "hello") },
			wantStatus: http.StatusOK,
			assertData: func(t *testing.T, body payloadMap) {
				t.Helper()
				if got := body["data"]; got != "hello" {
					t.Fatalf("data = %#v, want hello", got)
				}
			},
		},
		{
			name:       "OK omits data for nil interface payload",
			write:      func(w http.ResponseWriter) error { return OK(w, nil) },
			wantStatus: http.StatusOK,
			assertData: func(t *testing.T, body payloadMap) {
				t.Helper()
				if _, exists := body["data"]; exists {
					t.Fatalf("data unexpectedly present: %#v", body["data"])
				}
			},
		},
		{
			name:       "Created keeps typed nil payload as json null",
			write:      func(w http.ResponseWriter) error { return Created(w, nilUser) },
			wantStatus: http.StatusCreated,
			assertData: func(t *testing.T, body payloadMap) {
				t.Helper()
				data, exists := body["data"]
				if !exists {
					t.Fatal("data missing, want null")
				}
				if data != nil {
					t.Fatalf("data = %#v, want nil", data)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()

			if err := tc.write(rr); err != nil {
				t.Fatalf("write() error = %v", err)
			}
			assertSuccessEnvelope(t, rr, tc.wantStatus)

			body := decodePayload(t, rr.Body.Bytes())
			tc.assertData(t, body)
		})
	}
}

func TestSuccessWritersRejectNilWriter(t *testing.T) {
	cases := []struct {
		name  string
		write func() error
	}{
		{name: "Accepted", write: func() error { return Accepted(nil, map[string]any{"id": "u_1"}) }},
		{name: "Created", write: func() error { return Created(nil, map[string]any{"id": "u_1"}) }},
		{name: "NoContent", write: func() error { return NoContent(nil) }},
		{name: "OK", write: func() error { return OK(nil, map[string]any{"id": "u_1"}) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.write(); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestSuccessWritersRejectUnsupportedValue(t *testing.T) {
	cases := []struct {
		name  string
		write func(http.ResponseWriter) error
	}{
		{name: "Accepted", write: func(w http.ResponseWriter) error { return Accepted(w, make(chan int)) }},
		{name: "Created", write: func(w http.ResponseWriter) error { return Created(w, make(chan int)) }},
		{name: "OK", write: func(w http.ResponseWriter) error { return OK(w, make(chan int)) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			_ = assertUnsupportedTypeError(t, tc.write(rr))
			assertRecorderHasNoBodyOrContentType(t, rr)
		})
	}
}

func TestWriteSuccessRejectsUnsupportedStatusBeforeCommit(t *testing.T) {
	rr := httptest.NewRecorder()

	err := writeSuccess(rr, http.StatusNoContent, map[string]any{"id": "u_1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertRecorderHasNoBodyOrContentType(t, rr)
}

func TestSuccessWritersResponseBoundaries(t *testing.T) {
	t.Run("head uses net http head semantics", func(t *testing.T) {
		result := roundTripOverHTTPMethod(t, http.MethodHead, func(w http.ResponseWriter, _ *http.Request) error {
			return OK(w, map[string]any{"id": "u_1"})
		})

		if result.handlerErr != nil {
			t.Fatalf("handler error = %v", result.handlerErr)
		}
		if result.response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", result.response.StatusCode, http.StatusOK)
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

		if err := OK(rr, map[string]any{"id": "u_1"}); err != nil {
			t.Fatalf("OK() error = %v", err)
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

	t.Run("clears stale content length on real http server", func(t *testing.T) {
		expected := httptest.NewRecorder()
		if err := Created(expected, map[string]any{"id": "u_1"}); err != nil {
			t.Fatalf("Created() expected recorder error = %v", err)
		}

		result := roundTripOverHTTP(t, func(w http.ResponseWriter, _ *http.Request) error {
			w.Header().Set("Content-Length", "100")
			return Created(w, map[string]any{"id": "u_1"})
		})

		if result.handlerErr != nil {
			t.Fatalf("handler error = %v", result.handlerErr)
		}
		if result.response.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want %d", result.response.StatusCode, http.StatusCreated)
		}
		if got := string(result.body); got != expected.Body.String() {
			t.Fatalf("body = %q, want %q", got, expected.Body.String())
		}
		if got := result.response.Header.Get("Content-Length"); got != "" && got != strconv.Itoa(len(result.body)) {
			t.Fatalf("Content-Length = %q, want empty or %d", got, len(result.body))
		}
	})

	t.Run("no content clears stale content headers and writes no body", func(t *testing.T) {
		rr := httptest.NewRecorder()
		rr.Header().Set("X-Trace-ID", "trace-1")
		rr.Header().Set("Content-Type", "application/json")
		rr.Header().Set("Content-Length", "999")

		if err := NoContent(rr); err != nil {
			t.Fatalf("NoContent() error = %v", err)
		}

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
		}
		if rr.Body.Len() != 0 {
			t.Fatalf("body = %q, want empty", rr.Body.String())
		}
		if got := rr.Header().Get("X-Trace-ID"); got != "trace-1" {
			t.Fatalf("X-Trace-ID = %q, want %q", got, "trace-1")
		}
		if got := rr.Header().Get("Content-Type"); got != "" {
			t.Fatalf("Content-Type = %q, want empty", got)
		}
		if got := rr.Header().Get("Content-Length"); got != "" {
			t.Fatalf("Content-Length = %q, want empty", got)
		}
	})

	t.Run("no content keeps real http response body empty", func(t *testing.T) {
		result := roundTripOverHTTP(t, func(w http.ResponseWriter, _ *http.Request) error {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "999")
			return NoContent(w)
		})

		if result.handlerErr != nil {
			t.Fatalf("handler error = %v", result.handlerErr)
		}
		if result.response.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", result.response.StatusCode, http.StatusNoContent)
		}
		if len(result.body) != 0 {
			t.Fatalf("body = %q, want empty", string(result.body))
		}
		if got := result.response.Header.Get("Content-Type"); got != "" {
			t.Fatalf("Content-Type = %q, want empty", got)
		}
		if got := result.response.Header.Get("Content-Length"); got != "" {
			t.Fatalf("Content-Length = %q, want empty", got)
		}
	})
}

func TestSuccessWritersReturnWrappedWriteError(t *testing.T) {
	cases := []struct {
		name       string
		wantStatus int
		write      func(http.ResponseWriter) error
	}{
		{
			name:       "Accepted",
			wantStatus: http.StatusAccepted,
			write:      func(w http.ResponseWriter) error { return Accepted(w, map[string]any{"id": "u_1"}) },
		},
		{
			name:       "OK",
			wantStatus: http.StatusOK,
			write:      func(w http.ResponseWriter) error { return OK(w, map[string]any{"id": "u_1"}) },
		},
		{
			name:       "Created",
			wantStatus: http.StatusCreated,
			write:      func(w http.ResponseWriter) error { return Created(w, map[string]any{"id": "u_1"}) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cause := errors.New("socket closed")
			w := &failingWriter{cause: cause}
			err := tc.write(w)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, cause) {
				t.Fatalf("errors.Is(err, cause) = false, want true")
			}
			if w.status != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.status, tc.wantStatus)
			}
			if w.writes != 1 {
				t.Fatalf("writes = %d, want 1", w.writes)
			}
		})
	}
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

func TestWriteErrorOmitsEmptyDetails(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := WriteError(rr, errx.NewHTTPError(http.StatusBadRequest, "invalid_json", "payload invalid").WithFieldErrors([]errx.FieldError{})); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	assertErrorEnvelopeBasics(t, rr, http.StatusBadRequest, 40000, "payload invalid")

	body := decodePayload(t, rr.Body.Bytes())
	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	if _, exists := errorValue["details"]; exists {
		t.Fatalf("error.details unexpectedly present: %#v", errorValue["details"])
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

func TestWriteErrorPrefersFirstHTTPErrorInJoinedChain(t *testing.T) {
	rr := httptest.NewRecorder()

	input := errors.Join(
		errx.NewHTTPError(http.StatusBadRequest, "invalid_json", "payload invalid"),
		errx.NewHTTPError(http.StatusForbidden, "forbidden", "access denied"),
	)
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

	t.Run("typed nil http error becomes internal error without panic", func(t *testing.T) {
		var typedNil *errx.HTTPError
		rr := httptest.NewRecorder()

		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("WriteError() panicked: %v", recovered)
			}
		}()

		if err := WriteError(rr, typedNil); err != nil {
			t.Fatalf("WriteError() error = %v", err)
		}

		assertErrorEnvelopeBasics(t, rr, http.StatusInternalServerError, 50000, "internal error")

		body := decodePayload(t, rr.Body.Bytes())
		errorValue := body["error"].(map[string]any)
		assertPublicErrorObject(t, errorValue, map[string]any{
			"reason": "internal_error",
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

	t.Run("rejects invalid explicit top code before commit", func(t *testing.T) {
		rr := httptest.NewRecorder()

		if err := WriteError(rr, errx.BadRequest("", ""), 0); err == nil {
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

func assertSuccessEnvelope(t *testing.T, rr *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()

	if rr.Code != wantStatus {
		t.Fatalf("status = %d, want %d", rr.Code, wantStatus)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != float64(0) {
		t.Fatalf("code = %#v, want 0", got)
	}
	if got := body["message"]; got != "success" {
		t.Fatalf("message = %#v, want success", got)
	}
	if _, exists := body["error"]; exists {
		t.Fatalf("error unexpectedly present: %#v", body["error"])
	}
}
