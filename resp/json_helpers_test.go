package resp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

type nilableResponseWriter struct {
	header http.Header
	status int
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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

func (w *nilableResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *nilableResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *nilableResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
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
	doneCh := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(doneCh)
		defer func() {
			if recovered := recover(); recovered != nil {
				errCh <- fmt.Errorf("resp test handler panicked: %v", recovered)
			}
		}()
		errCh <- handler(w, r)
	}))
	defer srv.Close()

	req, err := http.NewRequest(method, srv.URL, nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	body, readErr := io.ReadAll(res.Body)
	if err := res.Body.Close(); err != nil {
		t.Fatalf("Body.Close() error = %v", err)
	}

	<-doneCh
	handlerErr := errors.New("resp test handler did not report result")
	select {
	case handlerErr = <-errCh:
	default:
	}

	return realHTTPRoundTrip{
		response:   res,
		body:       body,
		readErr:    readErr,
		handlerErr: handlerErr,
	}
}

func TestRoundTripOverHTTPMethodConvertsHandlerPanicToError(t *testing.T) {
	result := roundTripOverHTTPMethod(t, http.MethodGet, func(http.ResponseWriter, *http.Request) error {
		panic("boom")
	})

	if result.handlerErr == nil {
		t.Fatal("expected handler error, got nil")
	}
	if got := result.handlerErr.Error(); !strings.Contains(got, "resp test handler panicked: boom") {
		t.Fatalf("handler error = %q, want to contain panic detail", got)
	}
}

func TestRoundTripOverHTTPMethodUsesServerClient(t *testing.T) {
	original := http.DefaultClient
	t.Cleanup(func() {
		http.DefaultClient = original
	})

	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("poisoned default client")
		}),
	}

	result := roundTripOverHTTPMethod(t, http.MethodGet, func(w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusNoContent)
		return nil
	})

	if result.handlerErr != nil {
		t.Fatalf("handler error = %v", result.handlerErr)
	}
	if result.response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", result.response.StatusCode, http.StatusNoContent)
	}
	if result.readErr != nil {
		t.Fatalf("ReadAll() error = %v", result.readErr)
	}
	if len(result.body) != 0 {
		t.Fatalf("body = %q, want empty", string(result.body))
	}
}
