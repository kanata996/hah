package resp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type payloadMap map[string]any

type realHTTPRoundTrip struct {
	response   *http.Response
	body       []byte
	readErr    error
	handlerErr error
}

type failingWriter struct {
	header http.Header
	status int
	writes int
	cause  error
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

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want recorder default %d", rr.Code, http.StatusOK)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
	if len(rr.Header()) != 0 {
		t.Fatalf("headers = %#v, want empty", rr.Header())
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

func roundTripOverHTTP(t *testing.T, handler func(http.ResponseWriter, *http.Request) error) realHTTPRoundTrip {
	t.Helper()

	return roundTripOverHTTPMethod(t, http.MethodGet, handler)
}

func roundTripOverHTTPMethod(t *testing.T, method string, handler func(http.ResponseWriter, *http.Request) error) realHTTPRoundTrip {
	t.Helper()

	errCh := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errCh <- handler(w, r)
	}))
	defer srv.Close()

	req, err := http.NewRequest(method, srv.URL, nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	body, readErr := io.ReadAll(res.Body)
	if err := res.Body.Close(); err != nil {
		t.Fatalf("Body.Close() error = %v", err)
	}

	return realHTTPRoundTrip{
		response:   res,
		body:       body,
		readErr:    readErr,
		handlerErr: <-errCh,
	}
}
