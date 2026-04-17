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
	inner := &headLikeResponseWriter{}
	w := &writeCallbackResponseWriter{ResponseWriter: inner}

	if err := JSON(w, http.StatusAccepted, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if inner.status != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", inner.status, http.StatusAccepted)
	}
	if got := inner.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", got, "application/json")
	}
	if got := inner.Header().Get("Content-Length"); got != stringLen([]byte("{\"id\":\"u_1\"}\n")) {
		t.Fatalf("Content-Length = %q, want %s", got, stringLen([]byte("{\"id\":\"u_1\"}\n")))
	}
	if w.writeCalls != 1 {
		t.Fatalf("writeCalls = %d, want 1", w.writeCalls)
	}
}

func TestJSONBodyWritersRespectWrappedWriterContentLength(t *testing.T) {
	inner := &headLikeResponseWriter{}
	w := &transformingResponseWriter{
		ResponseWriter: inner,
		suffix:         []byte("\n"),
	}

	if err := JSON(w, http.StatusAccepted, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if inner.status != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", inner.status, http.StatusAccepted)
	}
	if got := inner.Header().Get("Content-Length"); got != stringLen(w.lastWrite) {
		t.Fatalf("Content-Length = %q, want %s", got, stringLen(w.lastWrite))
	}
	if w.writeCalls != 1 {
		t.Fatalf("writeCalls = %d, want 1", w.writeCalls)
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
	if err := JSON(nil, http.StatusOK, map[string]any{"id": "u_1"}); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestJSONBodyWritersRejectUnsupportedStatusesBeforeCommit(t *testing.T) {
	cases := []struct {
		name  string
		write func(http.ResponseWriter) error
	}{
		{
			name: "JSON unsupported 203",
			write: func(w http.ResponseWriter) error {
				return JSON(w, http.StatusNonAuthoritativeInfo, map[string]any{"id": "u_1"})
			},
		},
		{
			name:  "JSON unsupported 204",
			write: func(w http.ResponseWriter) error { return JSON(w, http.StatusNoContent, map[string]any{"id": "u_1"}) },
		},
		{
			name: "JSON unsupported 205",
			write: func(w http.ResponseWriter) error {
				return JSON(w, http.StatusResetContent, map[string]any{"id": "u_1"})
			},
		},
		{
			name: "JSON unsupported 206",
			write: func(w http.ResponseWriter) error {
				return JSON(w, http.StatusPartialContent, map[string]any{"id": "u_1"})
			},
		},
		{
			name:  "JSON unsupported 207",
			write: func(w http.ResponseWriter) error { return JSON(w, http.StatusMultiStatus, map[string]any{"id": "u_1"}) },
		},
		{
			name: "JSON unsupported 208",
			write: func(w http.ResponseWriter) error {
				return JSON(w, http.StatusAlreadyReported, map[string]any{"id": "u_1"})
			},
		},
		{
			name:  "JSON unsupported 226",
			write: func(w http.ResponseWriter) error { return JSON(w, http.StatusIMUsed, map[string]any{"id": "u_1"}) },
		},
		{
			name:  "JSON invalid status",
			write: func(w http.ResponseWriter) error { return JSON(w, 1000, map[string]any{"id": "u_1"}) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			if err := tc.write(rr); err == nil {
				t.Fatal("expected error, got nil")
			}
			assertRecorderHasNoBodyOrContentType(t, rr)
		})
	}
}

func TestJSONBodyWritersPreserveUnrelatedHeadersAndOwnContentHeaders(t *testing.T) {
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
			rr := httptest.NewRecorder()
			rr.Header().Set("X-Trace-ID", "trace-1")
			rr.Header().Set("Content-Type", "text/plain")
			rr.Header().Set("Content-Length", "999")

			if err := tc.write(rr); err != nil {
				t.Fatalf("write() error = %v", err)
			}
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantStatus)
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
}

func TestJSONRejectsUnsupportedValue(t *testing.T) {
	rr := httptest.NewRecorder()

	_ = assertUnsupportedTypeError(t, JSON(rr, http.StatusOK, make(chan int)))
	assertRecorderHasNoBodyOrContentType(t, rr)
}

func TestJSONBodyWritersReturnWrappedWriteError(t *testing.T) {
	cause := errors.New("socket closed")
	w := &failingWriter{cause: cause}
	err := JSON(w, http.StatusAccepted, map[string]any{"id": "u_1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false, want true")
	}
	if w.status != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.status, http.StatusAccepted)
	}
	if w.writes != 1 {
		t.Fatalf("writes = %d, want 1", w.writes)
	}
}
