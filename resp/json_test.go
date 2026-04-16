package resp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestJSONWritersWriteExpectedResponses(t *testing.T) {
	cases := []struct {
		name            string
		write           func(http.ResponseWriter) error
		wantStatus      int
		wantContentType string
		wantBody        string
	}{
		{
			name:            "JSON writes compact JSON",
			write:           func(w http.ResponseWriter) error { return JSON(w, http.StatusAccepted, map[string]any{"id": "u_1"}) },
			wantStatus:      http.StatusAccepted,
			wantContentType: "application/json",
			wantBody:        "{\"id\":\"u_1\"}\n",
		},
		{
			name:            "JSON allows nil data",
			write:           func(w http.ResponseWriter) error { return JSON(w, http.StatusOK, nil) },
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        "null\n",
		},
		{
			name:            "JSONBlob writes raw bytes",
			write:           func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusAccepted, []byte(`{"id":"u_1"}`)) },
			wantStatus:      http.StatusAccepted,
			wantContentType: "application/json",
			wantBody:        `{"id":"u_1"}`,
		},
		{
			name:            "JSONBlob passes through invalid JSON",
			write:           func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusOK, []byte(`{"id":`)) },
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"id":`,
		},
		{
			name:            "JSONBlob allows nil body",
			write:           func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusOK, nil) },
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        "",
		},
		{
			name:            "JSONBlob allows empty body",
			write:           func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusOK, []byte{}) },
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()

			if err := tc.write(rr); err != nil {
				t.Fatalf("write() error = %v", err)
			}
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantStatus)
			}
			if got := rr.Header().Get("Content-Type"); got != tc.wantContentType {
				t.Fatalf("Content-Type = %q, want %q", got, tc.wantContentType)
			}
			if got := rr.Body.String(); got != tc.wantBody {
				t.Fatalf("body = %q, want %q", got, tc.wantBody)
			}
		})
	}
}

func TestJSONBodyWritersCooperateWithHeadLikeWriter(t *testing.T) {
	cases := []struct {
		name       string
		wantStatus int
		wantBody   []byte
		write      func(http.ResponseWriter) error
	}{
		{
			name:       "JSON",
			wantStatus: http.StatusAccepted,
			wantBody:   []byte("{\"id\":\"u_1\"}\n"),
			write:      func(w http.ResponseWriter) error { return JSON(w, http.StatusAccepted, map[string]any{"id": "u_1"}) },
		},
		{
			name:       "JSONBlob",
			wantStatus: http.StatusAccepted,
			wantBody:   []byte(`{"id":"u_1"}`),
			write:      func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusAccepted, []byte(`{"id":"u_1"}`)) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := &headLikeResponseWriter{}
			w := &writeCallbackResponseWriter{ResponseWriter: inner}

			if err := tc.write(w); err != nil {
				t.Fatalf("write() error = %v", err)
			}
			if inner.status != tc.wantStatus {
				t.Fatalf("status = %d, want %d", inner.status, tc.wantStatus)
			}
			if got := inner.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want %q", got, "application/json")
			}
			if got := inner.Header().Get("Content-Length"); got != stringLen(tc.wantBody) {
				t.Fatalf("Content-Length = %q, want %s", got, stringLen(tc.wantBody))
			}
			if w.writeCalls != 1 {
				t.Fatalf("writeCalls = %d, want 1", w.writeCalls)
			}
		})
	}
}

func TestJSONBodyWritersRespectWrappedWriterContentLength(t *testing.T) {
	cases := []struct {
		name       string
		write      func(http.ResponseWriter) error
		wantStatus int
	}{
		{
			name:       "JSON",
			write:      func(w http.ResponseWriter) error { return JSON(w, http.StatusAccepted, map[string]any{"id": "u_1"}) },
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "JSONBlob",
			write:      func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusAccepted, []byte(`{"id":"u_1"}`)) },
			wantStatus: http.StatusAccepted,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := &headLikeResponseWriter{}
			w := &transformingResponseWriter{
				ResponseWriter: inner,
				suffix:         []byte("\n"),
			}

			if err := tc.write(w); err != nil {
				t.Fatalf("write() error = %v", err)
			}
			if inner.status != tc.wantStatus {
				t.Fatalf("status = %d, want %d", inner.status, tc.wantStatus)
			}
			if got := inner.Header().Get("Content-Length"); got != stringLen(w.lastWrite) {
				t.Fatalf("Content-Length = %q, want %s", got, stringLen(w.lastWrite))
			}
			if w.writeCalls != 1 {
				t.Fatalf("writeCalls = %d, want 1", w.writeCalls)
			}
		})
	}
}

func TestJSONClearsStaleContentLengthOnRealHTTPServer(t *testing.T) {
	result := roundTripOverHTTP(t, func(w http.ResponseWriter, _ *http.Request) error {
		w.Header().Set("Content-Length", "100")
		return JSON(w, http.StatusOK, map[string]any{"id": "u_1"})
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
	if result.readErr != nil {
		t.Fatalf("ReadAll() error = %v", result.readErr)
	}
	if got := string(result.body); got != "{\"id\":\"u_1\"}\n" {
		t.Fatalf("body = %q, want %q", got, "{\"id\":\"u_1\"}\n")
	}
	if got := result.response.Header.Get("Content-Length"); got != "" && got != strconv.Itoa(len(result.body)) {
		t.Fatalf("Content-Length = %q, want empty or %d", got, len(result.body))
	}
}

func TestJSONBodyWritersRejectNilWriter(t *testing.T) {
	cases := []struct {
		name    string
		write   func() error
		wantErr string
	}{
		{name: "JSON", write: func() error { return JSON(nil, http.StatusOK, map[string]any{"id": "u_1"}) }, wantErr: "resp: response writer is nil"},
		{name: "JSONBlob", write: func() error { return JSONBlob(nil, http.StatusOK, []byte(`{"id":"u_1"}`)) }, wantErr: "resp: response writer is nil"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.write(); err == nil || err.Error() != tc.wantErr {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestJSONBodyWritersRejectInvalidStatusOrBodylessStatus(t *testing.T) {
	cases := []struct {
		name      string
		write     func(http.ResponseWriter) error
		wantErr   string
		checkBody bool
	}{
		{
			name:      "JSON informational status",
			write:     func(w http.ResponseWriter) error { return JSON(w, http.StatusContinue, map[string]any{"id": "u_1"}) },
			wantErr:   "resp: JSON body writers cannot use informational status 100",
			checkBody: true,
		},
		{
			name:      "JSON invalid status",
			write:     func(w http.ResponseWriter) error { return JSON(w, 1000, map[string]any{"id": "u_1"}) },
			wantErr:   "resp: invalid HTTP status 1000",
			checkBody: true,
		},
		{
			name: "JSON bodyless status",
			write: func(w http.ResponseWriter) error {
				return JSON(w, http.StatusResetContent, map[string]any{"id": "u_1"})
			},
			wantErr:   "resp: JSON body writers cannot use bodyless status 205",
			checkBody: true,
		},
		{
			name:      "JSONBlob informational status",
			write:     func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusProcessing, []byte(`{"id":"u_1"}`)) },
			wantErr:   "resp: JSON body writers cannot use informational status 102",
			checkBody: true,
		},
		{
			name:      "JSONBlob invalid status",
			write:     func(w http.ResponseWriter) error { return JSONBlob(w, 1000, []byte(`{"id":"u_1"}`)) },
			wantErr:   "resp: invalid HTTP status 1000",
			checkBody: true,
		},
		{
			name:      "JSONBlob bodyless status",
			write:     func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusNoContent, []byte(`{"id":"u_1"}`)) },
			wantErr:   "resp: JSON body writers cannot use bodyless status 204",
			checkBody: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			if err := tc.write(rr); err == nil || err.Error() != tc.wantErr {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if tc.checkBody {
				assertRecorderHasNoBodyOrContentType(t, rr)
			}
		})
	}
}

func TestJSONRejectsUnsupportedValue(t *testing.T) {
	rr := httptest.NewRecorder()

	_ = assertUnsupportedTypeError(t, JSON(rr, http.StatusOK, make(chan int)))
	assertRecorderHasNoBodyOrContentType(t, rr)
}

func TestJSONBodyWritersReturnWrappedWriteError(t *testing.T) {
	cases := []struct {
		name       string
		wantStatus int
		write      func(http.ResponseWriter) error
	}{
		{
			name:       "JSON",
			wantStatus: http.StatusAccepted,
			write:      func(w http.ResponseWriter) error { return JSON(w, http.StatusAccepted, map[string]any{"id": "u_1"}) },
		},
		{
			name:       "JSONBlob",
			wantStatus: http.StatusAccepted,
			write:      func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusAccepted, []byte(`{"id":"u_1"}`)) },
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
			if got := err.Error(); got != "resp: write response failed: socket closed" {
				t.Fatalf("error = %q, want %q", got, "resp: write response failed: socket closed")
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
