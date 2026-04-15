package resp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

type payloadMap map[string]any
type failingWriter struct {
	header http.Header
	status int
	writes int
	cause  error
}
type headLikeResponseWriter struct {
	header           http.Header
	writeHeaderCalls int
	writeCalls       int
	status           int
}
type writeCallbackResponseWriter struct {
	http.ResponseWriter
	writeCalls int
}

func (w *failingWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingWriter) WriteHeader(status int) {
	w.status = status
}

func (w *failingWriter) Write(_ []byte) (int, error) {
	w.writes++
	if w.cause != nil {
		return 0, w.cause
	}
	return 0, errors.New("socket closed")
}

func (w *headLikeResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *headLikeResponseWriter) WriteHeader(status int) {
	w.writeHeaderCalls++
	w.status = status
}

func (w *headLikeResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	w.writeCalls++
	if w.Header().Get("Content-Length") == "" {
		w.Header().Set("Content-Length", strconv.Itoa(len(p)))
	}
	return len(p), nil
}

func (w *writeCallbackResponseWriter) Write(p []byte) (int, error) {
	w.writeCalls++
	return w.ResponseWriter.Write(p)
}

func decodePayload(t *testing.T, body []byte) payloadMap {
	t.Helper()

	var payload payloadMap
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return payload
}

func assertRecorderHasNoBodyOrContentType(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()

	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
}

func assertUnsupportedTypeError(t *testing.T, err error) *json.UnsupportedTypeError {
	t.Helper()

	if err == nil {
		t.Fatal("expected unsupported type error, got nil")
	}

	var unsupportedErr *json.UnsupportedTypeError
	if !errors.As(err, &unsupportedErr) {
		t.Fatalf("error = %T, want *json.UnsupportedTypeError", err)
	}
	return unsupportedErr
}

func assertPublicErrorObject(t *testing.T, got any, want map[string]any) {
	t.Helper()

	gotMap, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("error item = %#v, want object", got)
	}
	for key, wantValue := range want {
		if gotValue := gotMap[key]; gotValue != wantValue {
			t.Fatalf("error item %q = %#v, want %#v", key, gotValue, wantValue)
		}
	}
	if len(gotMap) != len(want) {
		t.Fatalf("error item = %#v, want only %#v", gotMap, want)
	}
}

func stringLen(body []byte) string {
	return strconv.Itoa(len(body))
}
