package render

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

func decodePayload(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	return payload
}

func TestRenderUsesStatusHintAndMarksStarted(t *testing.T) {
	rr := newResponseRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	Status(req, http.StatusCreated)

	err := Render(rr, req, map[string]any{"id": "u_1"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	if !ResponseStarted(req) {
		t.Fatal("ResponseStarted(req) = false, want true")
	}
}

func TestRenderWithMetaWritesEnvelope(t *testing.T) {
	rr := newResponseRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	err := RenderWithMeta(rr, req, []string{"a"}, map[string]any{"request_id": "req_1"})
	if err != nil {
		t.Fatalf("RenderWithMeta() error = %v", err)
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

func TestEnsureStateNilAndReuse(t *testing.T) {
	if got := EnsureState(nil); got != nil {
		t.Fatalf("EnsureState(nil) = %#v, want nil", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	withState := EnsureState(req)
	if withState == nil {
		t.Fatal("EnsureState(req) = nil")
	}
	if StateFrom(withState) == nil {
		t.Fatal("StateFrom(withState) = nil")
	}
	if got := EnsureState(withState); got != withState {
		t.Fatal("EnsureState(reqWithState) did not reuse request")
	}
}

func TestStatusIgnoresZeroAndAttachesState(t *testing.T) {
	Status(nil, http.StatusAccepted)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	Status(req, 0)
	if state := StateFrom(req); state != nil {
		t.Fatalf("StateFrom(req) = %#v, want nil after zero status", state)
	}

	Status(req, http.StatusAccepted)
	state := StateFrom(req)
	if state == nil {
		t.Fatal("StateFrom(req) = nil after Status")
	}
	if state.Status != http.StatusAccepted {
		t.Fatalf("state.Status = %d, want %d", state.Status, http.StatusAccepted)
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
				return WriteSuccess(newResponseRecorder(), nil, http.StatusBadRequest, map[string]any{"ok": true}, nil, false)
			},
		},
		{
			name: "informational status",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), nil, http.StatusContinue, map[string]any{"ok": true}, nil, false)
			},
		},
		{
			name: "nil data",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), nil, http.StatusOK, nil, nil, false)
			},
		},
		{
			name: "invalid meta",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), nil, http.StatusOK, map[string]any{"ok": true}, "bad-meta", true)
			},
		},
		{
			name: "marshal failure",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), nil, http.StatusOK, map[string]any{"bad": func() {}}, nil, false)
			},
		},
		{
			name: "custom marshaler encodes data as null",
			fn: func() error {
				return WriteSuccess(newResponseRecorder(), nil, http.StatusOK, nullJSONValue{}, nil, false)
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

func TestRenderEmptyWritesStatusWithoutBody(t *testing.T) {
	rr := newResponseRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	err := RenderEmpty(rr, req, http.StatusNoContent)
	if err != nil {
		t.Fatalf("RenderEmpty() error = %v", err)
	}

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if !ResponseStarted(req) {
		t.Fatal("ResponseStarted(req) = false, want true")
	}
}

func TestRenderEmptyDefaultsToNoContent(t *testing.T) {
	rr := newResponseRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	err := RenderEmpty(rr, req, 0)
	if err != nil {
		t.Fatalf("RenderEmpty() error = %v", err)
	}

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
}

func TestWriteEmptyUsesRequestContentTypeHint(t *testing.T) {
	rr := newResponseRecorder()
	req := EnsureState(httptest.NewRequest(http.MethodGet, "/", nil))
	StateFrom(req).ContentType = "application/problem+json"

	err := WriteEmpty(rr, req, http.StatusNoContent)
	if err != nil {
		t.Fatalf("WriteEmpty() error = %v", err)
	}

	if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
}

func TestWriteEmptyRejectsInvalidStatus(t *testing.T) {
	if err := WriteEmpty(newResponseRecorder(), nil, http.StatusBadRequest); err == nil {
		t.Fatalf("expected error for 4xx status")
	}
}

func TestRenderErrorPayloadFallsBackWhenDetailsCannotBeMarshaled(t *testing.T) {
	rr := newResponseRecorder()

	err := RenderErrorPayload(rr, nil, ErrorPayload{
		Status:  http.StatusBadRequest,
		Code:    "bad_request",
		Message: "bad request",
		Details: []any{func() {}},
	})
	if err != nil {
		t.Fatalf("RenderErrorPayload() error = %v", err)
	}

	payload := decodePayload(t, rr)
	rawError, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope missing or wrong type: %#v", payload["error"])
	}
	if details, ok := rawError["details"].([]any); !ok || len(details) != 0 {
		t.Fatalf("error.details = %#v, want empty array", rawError["details"])
	}
}

func TestRenderErrorPayloadReturnsWriteError(t *testing.T) {
	rw := newFailingResponseWriter(errors.New("write failed"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	err := RenderErrorPayload(rw, req, ErrorPayload{
		Status:  http.StatusConflict,
		Code:    "conflict",
		Message: "conflict",
	})
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !ResponseStarted(req) {
		t.Fatal("ResponseStarted(req) = false, want true")
	}
}

func TestWriteSuccessAcceptsMetaThatMarshalsToJSONObject(t *testing.T) {
	rr := newResponseRecorder()

	err := WriteSuccess(rr, nil, http.StatusOK, []string{"a"}, objectJSONValue{}, true)
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

	err := WriteSuccess(rr, nil, http.StatusOK, map[string]any{"ok": true}, nullJSONValue{}, true)
	if err != nil {
		t.Fatalf("WriteSuccess() error = %v", err)
	}

	payload := decodePayload(t, rr)
	if _, ok := payload["meta"]; ok {
		t.Fatalf("meta should be omitted, payload = %#v", payload)
	}
}

func TestWriteSuccessRejectsMetaThatMarshalsToNonObject(t *testing.T) {
	err := WriteSuccess(newResponseRecorder(), nil, http.StatusOK, []string{"a"}, stringJSONValue{}, true)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestWriteSuccessRejectsMetaMarshalFailure(t *testing.T) {
	err := WriteSuccess(newResponseRecorder(), nil, http.StatusOK, []string{"a"}, errorJSONValue{}, true)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestIsJSONObjectBytesRejectsEmptyInput(t *testing.T) {
	if isJSONObjectBytes([]byte(" \n\t ")) {
		t.Fatalf("empty input should not be treated as JSON object")
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

func TestIsJSONObjectBytesRejectsMalformedInput(t *testing.T) {
	if isJSONObjectBytes([]byte(`"bad-meta"`)) {
		t.Fatalf("string input should not be treated as JSON object")
	}
	if isJSONObjectBytes([]byte(`{"ok":true`)) {
		t.Fatalf("unterminated object should not be treated as JSON object")
	}
}

func TestEnsureAttachedStateHandlesNil(t *testing.T) {
	if got := ensureAttachedState(nil); got != nil {
		t.Fatalf("ensureAttachedState(nil) = %#v, want nil", got)
	}
}
