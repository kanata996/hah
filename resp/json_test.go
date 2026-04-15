package resp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
			wantContentType: jsonContentType,
			wantBody:        "{\"id\":\"u_1\"}\n",
		},
		{
			name:            "JSON allows nil data",
			write:           func(w http.ResponseWriter) error { return JSON(w, http.StatusOK, nil) },
			wantStatus:      http.StatusOK,
			wantContentType: jsonContentType,
			wantBody:        "null\n",
		},
		{
			name:            "JSONBlob writes raw bytes",
			write:           func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusAccepted, []byte(`{"id":"u_1"}`)) },
			wantStatus:      http.StatusAccepted,
			wantContentType: jsonContentType,
			wantBody:        `{"id":"u_1"}`,
		},
		{
			name:            "JSONBlob passes through invalid JSON",
			write:           func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusOK, []byte(`{"id":`)) },
			wantStatus:      http.StatusOK,
			wantContentType: jsonContentType,
			wantBody:        `{"id":`,
		},
		{
			name:            "JSONBlob allows nil body",
			write:           func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusOK, nil) },
			wantStatus:      http.StatusOK,
			wantContentType: jsonContentType,
			wantBody:        "",
		},
		{
			name:            "JSONBlob allows empty body",
			write:           func(w http.ResponseWriter) error { return JSONBlob(w, http.StatusOK, []byte{}) },
			wantStatus:      http.StatusOK,
			wantContentType: jsonContentType,
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

func TestJSONRecoversFromMarshalPanic(t *testing.T) {
	rr := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("JSON() panicked: %v", recovered)
		}
	}()

	err := JSON(rr, http.StatusOK, panicSuccessJSONValue{})
	if err == nil || err.Error() != "resp: encode JSON panicked: panic during MarshalJSON" {
		t.Fatalf("JSON() error = %v, want panic recovery error", err)
	}
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

func TestJSONBodyWritersRecoverWriteErrorStringFromCausePanic(t *testing.T) {
	w := &failingWriter{cause: panicWriteCause{}}

	err := JSONBlob(w, http.StatusAccepted, []byte(`{"id":"u_1"}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("err.Error() panicked: %v", recovered)
		}
	}()

	if got := err.Error(); !strings.Contains(got, "resp: write response failed: panic calling Error()") {
		t.Fatalf("error = %q, want panic fallback text", got)
	}
}

func TestJSONBodyWritersFallbackWriteErrorStringOnBlankCause(t *testing.T) {
	w := &failingWriter{cause: blankWriteCause{}}

	err := JSONBlob(w, http.StatusAccepted, []byte(`{"id":"u_1"}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "resp: write response failed" {
		t.Fatalf("error = %q, want fallback text", got)
	}
}

func TestResponseWriteErrorFallbackMessageWithoutCause(t *testing.T) {
	cases := []struct {
		name string
		err  *responseWriteError
	}{
		{name: "nil receiver", err: nil},
		{name: "nil cause", err: &responseWriteError{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != "resp: write response failed" {
				t.Fatalf("Error() = %q, want fallback text", got)
			}
		})
	}
}

func TestResponseWriteErrorUnwrapNilReceiver(t *testing.T) {
	var err *responseWriteError

	if got := err.Unwrap(); got != nil {
		t.Fatalf("Unwrap() = %#v, want nil", got)
	}
}

func TestSafeErrorStringNil(t *testing.T) {
	if got := safeErrorString(nil); got != "" {
		t.Fatalf("safeErrorString(nil) = %q, want empty", got)
	}
}
