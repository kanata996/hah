package hah_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func assertErrorResponse(t *testing.T, rr *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()

	if rr.Code != status {
		t.Fatalf("status = %d, want %d", rr.Code, status)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	rawError, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope missing or wrong type: %#v", payload["error"])
	}
	if got := rawError["code"]; got != code {
		t.Fatalf("error.code = %#v, want %q", got, code)
	}
	if got := rawError["message"]; got != message {
		t.Fatalf("error.message = %#v, want %q", got, message)
	}
	if details, ok := rawError["details"].([]any); !ok || len(details) != 0 {
		t.Fatalf("error.details = %#v, want empty array", rawError["details"])
	}
}

func newRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/", nil)
}

func newResponseRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}
