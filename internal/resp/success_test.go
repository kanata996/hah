package resp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

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
		{name: "Created", write: func() error { return Created(nil, map[string]any{"id": "u_1"}) }},
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
