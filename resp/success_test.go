package resp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSuccessWritersWriteExpectedResponses(t *testing.T) {
	cases := []struct {
		name            string
		write           func(http.ResponseWriter) error
		wantStatus      int
		wantContentType string
		wantBody        string
	}{
		{
			name:            "Created writes compact JSON",
			write:           func(w http.ResponseWriter) error { return Created(w, map[string]any{"id": "u_1"}) },
			wantStatus:      http.StatusCreated,
			wantContentType: "application/json",
			wantBody:        "{\"id\":\"u_1\"}\n",
		},
		{
			name:            "OK writes compact JSON",
			write:           func(w http.ResponseWriter) error { return OK(w, map[string]any{"id": "u_1"}) },
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        "{\"id\":\"u_1\"}\n",
		},
		{
			name:            "Created nil payload encodes to JSON null",
			write:           func(w http.ResponseWriter) error { return Created(w, nil) },
			wantStatus:      http.StatusCreated,
			wantContentType: "application/json",
			wantBody:        "null\n",
		},
		{
			name:            "OK nil payload encodes to JSON null",
			write:           func(w http.ResponseWriter) error { return OK(w, nil) },
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

func TestSuccessWritersRejectNilWriter(t *testing.T) {
	cases := []struct {
		name  string
		write func() error
	}{
		{name: "Created", write: func() error { return Created(nil, map[string]any{"id": "u_1"}) }},
		{name: "OK", write: func() error { return OK(nil, map[string]any{"id": "u_1"}) }},
		{name: "NoContent", write: func() error { return NoContent(nil) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.write(); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestSuccessWritersAllowTypedNilPayload(t *testing.T) {
	type user struct {
		ID string `json:"id"`
	}
	var nilUser *user

	cases := []struct {
		name       string
		write      func(http.ResponseWriter) error
		wantStatus int
	}{
		{name: "Created typed nil", write: func(w http.ResponseWriter) error { return Created(w, nilUser) }, wantStatus: http.StatusCreated},
		{name: "OK typed nil", write: func(w http.ResponseWriter) error { return OK(w, nilUser) }, wantStatus: http.StatusOK},
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
			if got := rr.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want %q", got, "application/json")
			}
			if got := rr.Body.String(); got != "null\n" {
				t.Fatalf("body = %q, want %q", got, "null\n")
			}
		})
	}
}

func TestSuccessWritersRejectUnsupportedValue(t *testing.T) {
	cases := []struct {
		name  string
		write func(http.ResponseWriter) error
	}{
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

func TestNoContentWritesStatusOnly(t *testing.T) {
	rr := httptest.NewRecorder()
	rr.Header().Set("X-Trace-ID", "trace-1")
	rr.Header().Set("Content-Type", "application/json")
	rr.Header().Set("Content-Length", "10")

	if err := NoContent(rr); err != nil {
		t.Fatalf("NoContent() error = %v", err)
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
	if got := rr.Header().Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length = %q, want empty", got)
	}
	if got := rr.Header().Get("X-Trace-ID"); got != "trace-1" {
		t.Fatalf("X-Trace-ID = %q, want %q", got, "trace-1")
	}
}

func TestNoContentPreservesUnrelatedHeadersOnRealHTTPServer(t *testing.T) {
	result := roundTripOverHTTP(t, func(w http.ResponseWriter, _ *http.Request) error {
		w.Header().Set("X-Trace-ID", "trace-1")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "10")
		return NoContent(w)
	})

	if result.handlerErr != nil {
		t.Fatalf("handler error = %v", result.handlerErr)
	}
	if result.response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", result.response.StatusCode, http.StatusNoContent)
	}
	if got := result.response.Header.Get("X-Trace-ID"); got != "trace-1" {
		t.Fatalf("X-Trace-ID = %q, want %q", got, "trace-1")
	}
	if got := result.response.Header.Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
	if got := result.response.Header.Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length = %q, want empty", got)
	}
	if result.readErr != nil {
		t.Fatalf("ReadAll() error = %v", result.readErr)
	}
	if len(result.body) != 0 {
		t.Fatalf("body = %q, want empty", string(result.body))
	}
}

func TestSuccessWritersReturnWrappedWriteError(t *testing.T) {
	cases := []struct {
		name       string
		wantStatus int
		write      func(http.ResponseWriter) error
	}{
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
