package resp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type nullJSONValue struct{}

func (nullJSONValue) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

type objectJSONValue struct{}

func (objectJSONValue) MarshalJSON() ([]byte, error) {
	return []byte(`{"request_id":"req_1"}`), nil
}

type stringJSONValue struct{}

func (stringJSONValue) MarshalJSON() ([]byte, error) {
	return []byte(`"bad-meta"`), nil
}

type errorJSONValue struct{}

func (errorJSONValue) MarshalJSON() ([]byte, error) {
	return nil, errors.New("boom")
}

type failingResponseWriter struct {
	header   http.Header
	status   int
	writeErr error
	writes   int
}

func newFailingResponseWriter(err error) *failingResponseWriter {
	return &failingResponseWriter{
		header:   make(http.Header),
		writeErr: err,
	}
}

func (w *failingResponseWriter) Header() http.Header {
	return w.header
}

func (w *failingResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *failingResponseWriter) Write(_ []byte) (int, error) {
	w.writes++
	return 0, w.writeErr
}

func newResponseRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}

func assertErrorResponse(t *testing.T, rr *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()

	if rr.Code != status {
		t.Fatalf("status = %d, want %d", rr.Code, status)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	rawError, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope missing or wrong type: %#v", payload["error"])
	}
	if got := rawError["code"]; got != code {
		t.Fatalf("error.code = %#v, want %q", got, code)
	}
	if got := rawError["message"]; got != message {
		t.Fatalf("error.message = %#v, want %q", got, message)
	}
	if details, ok := rawError["details"].([]any); !ok || len(details) != 0 {
		t.Fatalf("error.details = %#v, want empty array", rawError["details"])
	}
}

func decodePayload(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	return payload
}

func TestWriteSuccessWritesEnvelope(t *testing.T) {
	rr := newResponseRecorder()

	err := WriteSuccess(rr, http.StatusCreated, map[string]any{"id": "u_1"}, nil, false)
	if err != nil {
		t.Fatalf("WriteSuccess() error = %v", err)
	}

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	payload := decodePayload(t, rr)

	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("data envelope missing or wrong type: %#v", payload["data"])
	}
	if got := data["id"]; got != "u_1" {
		t.Fatalf("data.id = %#v, want u_1", got)
	}
	if _, ok := payload["meta"]; ok {
		t.Fatalf("meta should be omitted, payload = %#v", payload)
	}
}

func TestWriteSuccessRejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name string
		fn   func() error
	}{
		{
			name: "error status",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), http.StatusBadRequest, map[string]any{"ok": true}, nil, false)
			},
		},
		{
			name: "informational status",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), http.StatusContinue, map[string]any{"ok": true}, nil, false)
			},
		},
		{
			name: "no content status",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), http.StatusNoContent, map[string]any{"ok": true}, nil, false)
			},
		},
		{
			name: "reset content status",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), http.StatusResetContent, map[string]any{"ok": true}, nil, false)
			},
		},
		{
			name: "not modified status",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), http.StatusNotModified, map[string]any{"ok": true}, nil, false)
			},
		},
		{
			name: "nil data",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), http.StatusOK, nil, nil, false)
			},
		},
		{
			name: "invalid meta",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), http.StatusOK, map[string]any{"ok": true}, "bad-meta", true)
			},
		},
		{
			name: "marshal failure",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), http.StatusOK, map[string]any{"bad": func() {}}, nil, false)
			},
		},
		{
			name: "custom marshaler encodes data as null",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), http.StatusOK, nullJSONValue{}, nil, false)
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestWriteEmptyWritesStatusWithoutBody(t *testing.T) {
	rr := newResponseRecorder()

	err := WriteEmpty(rr, http.StatusNoContent)
	if err != nil {
		t.Fatalf("WriteEmpty() error = %v", err)
	}

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
}

func TestWriteEmptyRejectsInvalidStatus(t *testing.T) {
	if err := WriteEmpty(newResponseRecorder(), http.StatusBadRequest); err == nil {
		t.Fatalf("expected error for 4xx status")
	}
}

func TestWriteErrorWritesEnvelope(t *testing.T) {
	t.Run("without details", func(t *testing.T) {
		rr := newResponseRecorder()

		err := WriteErrorPayload(rr, ErrorPayload{
			Status:  http.StatusConflict,
			Code:    "conflict",
			Message: "conflict",
		})
		if err != nil {
			t.Fatalf("WriteErrorPayload() error = %v", err)
		}

		assertErrorResponse(t, rr, http.StatusConflict, "conflict", "conflict")
	})

	t.Run("with details", func(t *testing.T) {
		rr := newResponseRecorder()

		err := WriteErrorPayload(rr, ErrorPayload{
			Status:  http.StatusUnprocessableEntity,
			Code:    "invalid",
			Message: "invalid",
			Details: []any{map[string]any{"field": "email"}},
		})
		if err != nil {
			t.Fatalf("WriteErrorPayload() error = %v", err)
		}

		payload := decodePayload(t, rr)
		rawError, ok := payload["error"].(map[string]any)
		if !ok {
			t.Fatalf("error envelope missing or wrong type: %#v", payload["error"])
		}

		details, ok := rawError["details"].([]any)
		if !ok || len(details) != 1 {
			t.Fatalf("error.details = %#v, want one item", rawError["details"])
		}

		detail, ok := details[0].(map[string]any)
		if !ok || detail["field"] != "email" {
			t.Fatalf("details[0] = %#v, want field=email", details[0])
		}
	})
}

func TestWriteErrorFallsBackWhenPayloadCannotBeMarshaled(t *testing.T) {
	rr := newResponseRecorder()

	err := WriteErrorPayload(rr, ErrorPayload{
		Status:  http.StatusBadRequest,
		Code:    "bad_request",
		Message: "bad request",
		Details: []any{func() {}},
	})
	if err == nil {
		t.Fatal("expected degraded write error, got nil")
	}
	var degraded *ErrorWriteDegraded
	if !errors.As(err, &degraded) || degraded == nil {
		t.Fatalf("WriteErrorPayload() error = %T, want *ErrorWriteDegraded", err)
	}
	if !degraded.PreservedPublicResponse {
		t.Fatal("degraded.PreservedPublicResponse = false, want true")
	}

	assertErrorResponse(t, rr, http.StatusBadRequest, "bad_request", "bad request")
}

func TestErrorWriteDegradedMethods(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var degraded *ErrorWriteDegraded
		if got := degraded.Error(); got != "hah: error response details were dropped" {
			t.Fatalf("degraded.Error() = %q, want default message", got)
		}
		if got := degraded.Unwrap(); got != nil {
			t.Fatalf("degraded.Unwrap() = %#v, want nil", got)
		}
	})

	t.Run("nil cause", func(t *testing.T) {
		degraded := &ErrorWriteDegraded{}
		if got := degraded.Error(); got != "hah: error response details were dropped" {
			t.Fatalf("degraded.Error() = %q, want default message", got)
		}
		if got := degraded.Unwrap(); got != nil {
			t.Fatalf("degraded.Unwrap() = %#v, want nil", got)
		}
	})

	t.Run("with cause", func(t *testing.T) {
		degraded := &ErrorWriteDegraded{Cause: errors.New("json: unsupported type: func()")}
		if got := degraded.Error(); got != "hah: error response details were dropped: json: unsupported type: func()" {
			t.Fatalf("degraded.Error() = %q, want message with cause", got)
		}
		if got := degraded.Unwrap(); got == nil || got.Error() != "json: unsupported type: func()" {
			t.Fatalf("degraded.Unwrap() = %#v, want original cause", got)
		}
	})
}

func TestWriteErrorReturnsWhenFallbackWriteFails(t *testing.T) {
	rw := newFailingResponseWriter(errors.New("write failed"))

	err := WriteErrorPayload(rw, ErrorPayload{
		Status:  http.StatusBadRequest,
		Code:    "bad_request",
		Message: "bad request",
		Details: []any{func() {}},
	})
	if err == nil {
		t.Fatal("expected joined write error, got nil")
	}

	var degraded *ErrorWriteDegraded
	if !errors.As(err, &degraded) || degraded == nil {
		t.Fatalf("WriteErrorPayload() error = %T, want joined error containing *ErrorWriteDegraded", err)
	}
	if degraded.PreservedPublicResponse {
		t.Fatal("degraded.PreservedPublicResponse = true, want false")
	}

	if rw.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rw.status, http.StatusBadRequest)
	}
	if got := rw.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if rw.writes != 1 {
		t.Fatalf("writes = %d, want 1", rw.writes)
	}
}

func TestWriteErrorReturnsWhenPrimaryWriteFails(t *testing.T) {
	rw := newFailingResponseWriter(errors.New("write failed"))

	err := WriteErrorPayload(rw, ErrorPayload{
		Status:  http.StatusConflict,
		Code:    "conflict",
		Message: "conflict",
	})
	if err == nil {
		t.Fatal("expected write error, got nil")
	}

	if rw.status != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rw.status, http.StatusConflict)
	}
	if got := rw.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if rw.writes != 1 {
		t.Fatalf("writes = %d, want 1", rw.writes)
	}
}

func TestWriteSuccessAcceptsMetaThatMarshalsToJSONObject(t *testing.T) {
	rr := newResponseRecorder()

	err := WriteSuccess(rr, http.StatusOK, []string{"a"}, objectJSONValue{}, true)
	if err != nil {
		t.Fatalf("WriteSuccess() error = %v", err)
	}

	payload := decodePayload(t, rr)
	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta missing or wrong type: %#v", payload["meta"])
	}
	if got := meta["request_id"]; got != "req_1" {
		t.Fatalf("meta.request_id = %#v, want req_1", got)
	}
}

func TestWriteSuccessOmitsNullMeta(t *testing.T) {
	rr := newResponseRecorder()

	err := WriteSuccess(rr, http.StatusOK, map[string]any{"ok": true}, nil, true)
	if err != nil {
		t.Fatalf("WriteSuccess() error = %v", err)
	}

	payload := decodePayload(t, rr)
	if _, ok := payload["meta"]; ok {
		t.Fatalf("meta should be omitted, payload = %#v", payload)
	}
}

func TestWriteSuccessRejectsMetaThatMarshalsToNonObject(t *testing.T) {
	err := WriteSuccess(newResponseRecorder(), http.StatusOK, []string{"a"}, stringJSONValue{}, true)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestWriteSuccessRejectsMetaMarshalFailure(t *testing.T) {
	err := WriteSuccess(newResponseRecorder(), http.StatusOK, []string{"a"}, errorJSONValue{}, true)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestValidateSuccessBodyStatus(t *testing.T) {
	if err := ValidateSuccessBodyStatus(http.StatusOK); err != nil {
		t.Fatalf("ValidateSuccessBodyStatus() unexpected error = %v", err)
	}
	for _, status := range []int{
		http.StatusContinue,
		http.StatusNoContent,
		http.StatusResetContent,
		http.StatusNotModified,
		http.StatusBadRequest,
		99,
	} {
		if err := ValidateSuccessBodyStatus(status); err == nil {
			t.Fatalf("expected error for status %d", status)
		}
	}
}

func TestValidateSuccessStatus(t *testing.T) {
	if err := ValidateSuccessStatus(http.StatusNoContent); err != nil {
		t.Fatalf("ValidateSuccessStatus() unexpected error = %v", err)
	}
	if err := ValidateSuccessStatus(http.StatusNotModified); err != nil {
		t.Fatalf("ValidateSuccessStatus() unexpected error = %v", err)
	}
	if err := ValidateSuccessStatus(http.StatusBadRequest); err == nil {
		t.Fatalf("expected error for 4xx status")
	}
	if err := ValidateSuccessStatus(99); err == nil {
		t.Fatalf("expected error for invalid status")
	}
}

func TestIsJSONObjectBytesRejectsEmptyInput(t *testing.T) {
	if isJSONObjectBytes([]byte(" \n\t ")) {
		t.Fatalf("empty input should not be treated as JSON object")
	}
}

func TestIsJSONObjectBytesRejectsMalformedInput(t *testing.T) {
	if isJSONObjectBytes([]byte(`"bad-meta"`)) {
		t.Fatalf("string input should not be treated as JSON object")
	}
	if isJSONObjectBytes([]byte(`{"ok":true`)) {
		t.Fatalf("unterminated object should not be treated as JSON object")
	}
}
