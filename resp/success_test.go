package resp

import (
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
			wantContentType: jsonContentType,
			wantBody:        "{\"id\":\"u_1\"}\n",
		},
		{
			name:            "OK writes compact JSON",
			write:           func(w http.ResponseWriter) error { return OK(w, map[string]any{"id": "u_1"}) },
			wantStatus:      http.StatusOK,
			wantContentType: jsonContentType,
			wantBody:        "{\"id\":\"u_1\"}\n",
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

func TestSuccessWritersCooperateWithHeadLikeWriter(t *testing.T) {
	cases := []struct {
		name       string
		wantStatus int
		wantBody   []byte
		write      func(http.ResponseWriter) error
	}{
		{
			name:       "OK",
			wantStatus: http.StatusOK,
			wantBody:   mustEncodeJSON(t, map[string]any{"id": "u_1"}),
			write:      func(w http.ResponseWriter) error { return OK(w, map[string]any{"id": "u_1"}) },
		},
		{
			name:       "Created",
			wantStatus: http.StatusCreated,
			wantBody:   mustEncodeJSON(t, map[string]any{"id": "u_1"}),
			write:      func(w http.ResponseWriter) error { return Created(w, map[string]any{"id": "u_1"}) },
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
			if got := inner.Header().Get("Content-Type"); got != jsonContentType {
				t.Fatalf("Content-Type = %q, want %q", got, jsonContentType)
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

func TestSuccessWritersRejectNilWriter(t *testing.T) {
	cases := []struct {
		name    string
		write   func() error
		wantErr string
	}{
		{name: "Created", write: func() error { return Created(nil, map[string]any{"id": "u_1"}) }, wantErr: "resp: response writer is nil"},
		{name: "OK", write: func() error { return OK(nil, map[string]any{"id": "u_1"}) }, wantErr: "resp: response writer is nil"},
		{name: "NoContent", write: func() error { return NoContent(nil) }, wantErr: "resp: response writer is nil"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.write(); err == nil || err.Error() != tc.wantErr {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestSuccessWritersRejectNullPayload(t *testing.T) {
	type user struct {
		ID string `json:"id"`
	}
	var nilUser *user

	cases := []struct {
		name    string
		write   func(http.ResponseWriter) error
		wantErr string
	}{
		{name: "Created nil", write: func(w http.ResponseWriter) error { return Created(w, nil) }, wantErr: "resp: data must exist and must not encode to null"},
		{name: "Created typed nil", write: func(w http.ResponseWriter) error { return Created(w, nilUser) }, wantErr: "resp: data must exist and must not encode to null"},
		{name: "OK nil", write: func(w http.ResponseWriter) error { return OK(w, nil) }, wantErr: "resp: data must exist and must not encode to null"},
		{name: "OK typed nil", write: func(w http.ResponseWriter) error { return OK(w, nilUser) }, wantErr: "resp: data must exist and must not encode to null"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			if err := tc.write(rr); err == nil || err.Error() != tc.wantErr {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			assertRecorderHasNoBodyOrContentType(t, rr)
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
			w := &failingWriter{}
			_ = assertWrappedResponseWriteError(t, tc.write(w))
			if w.status != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.status, tc.wantStatus)
			}
			if w.writes != 1 {
				t.Fatalf("writes = %d, want 1", w.writes)
			}
		})
	}
}
