package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChiMappedModeSmoke(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		assertions  func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			name:   "list users success with meta",
			method: http.MethodGet,
			path:   "/users?page=2&limit=1&role=admin",
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()

				if rr.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
				}

				payload := decodePayload(t, rr)
				data, ok := payload["data"].([]any)
				if !ok || len(data) != 1 {
					t.Fatalf("data = %#v, want single-item array", payload["data"])
				}

				first, ok := data[0].(map[string]any)
				if !ok {
					t.Fatalf("data[0] = %#v, want object", data[0])
				}
				if got := first["id"]; got != "u_3" {
					t.Fatalf("data[0].id = %#v, want u_3", got)
				}

				meta, ok := payload["meta"].(map[string]any)
				if !ok {
					t.Fatalf("meta = %#v, want object", payload["meta"])
				}
				if got := meta["page"]; got != float64(2) {
					t.Fatalf("meta.page = %#v, want 2", got)
				}
				if got := meta["limit"]; got != float64(1) {
					t.Fatalf("meta.limit = %#v, want 1", got)
				}
				if got := meta["role"]; got != "admin" {
					t.Fatalf("meta.role = %#v, want admin", got)
				}
			},
		},
		{
			name:   "list users validation error",
			method: http.MethodGet,
			path:   "/users?page=0",
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertErrorResponse(t, rr, http.StatusUnprocessableEntity, "invalid_request", "request contains invalid fields")
				assertFirstDetail(t, rr, "page", "min", "must be at least 1")
			},
		},
		{
			name:   "get user success",
			method: http.MethodGet,
			path:   "/users/u_1",
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()

				if rr.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
				}

				payload := decodePayload(t, rr)
				data, ok := payload["data"].(map[string]any)
				if !ok {
					t.Fatalf("data = %#v, want object", payload["data"])
				}
				if got := data["name"]; got != "Alice" {
					t.Fatalf("data.name = %#v, want Alice", got)
				}
			},
		},
		{
			name:   "get user mapped not found",
			method: http.MethodGet,
			path:   "/users/missing",
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertErrorResponse(t, rr, http.StatusNotFound, "user_not_found", "user not found")
			},
		},
		{
			name:        "create user success",
			method:      http.MethodPost,
			path:        "/users",
			contentType: "application/json",
			body:        `{"name":"Dave"}`,
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()

				if rr.Code != http.StatusCreated {
					t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
				}

				payload := decodePayload(t, rr)
				data, ok := payload["data"].(map[string]any)
				if !ok {
					t.Fatalf("data = %#v, want object", payload["data"])
				}
				if got := data["name"]; got != "Dave" {
					t.Fatalf("data.name = %#v, want Dave", got)
				}
				if got := data["role"]; got != "member" {
					t.Fatalf("data.role = %#v, want member", got)
				}
			},
		},
		{
			name:        "create user validation error",
			method:      http.MethodPost,
			path:        "/users",
			contentType: "application/json",
			body:        `{"name":"Alice","role":"owner"}`,
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertErrorResponse(t, rr, http.StatusUnprocessableEntity, "invalid_request", "request contains invalid fields")
				assertFirstDetail(t, rr, "role", "one_of", "must be member or admin")
			},
		},
		{
			name:        "create user invalid json",
			method:      http.MethodPost,
			path:        "/users",
			contentType: "application/json",
			body:        `{"name":`,
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertErrorResponse(t, rr, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
			},
		},
		{
			name:        "create user mapped conflict",
			method:      http.MethodPost,
			path:        "/users",
			contentType: "application/json",
			body:        `{"name":"alice"}`,
			assertions: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertErrorResponse(t, rr, http.StatusConflict, "user_conflict", "user already exists")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newRouter()
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			tt.assertions(t, rr)
		})
	}
}

func decodePayload(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return payload
}

func assertErrorResponse(t *testing.T, rr *httptest.ResponseRecorder, wantStatus int, wantCode, wantMessage string) {
	t.Helper()

	if rr.Code != wantStatus {
		t.Fatalf("status = %d, want %d", rr.Code, wantStatus)
	}

	payload := decodePayload(t, rr)
	rawError, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", payload["error"])
	}
	if got := rawError["code"]; got != wantCode {
		t.Fatalf("error.code = %#v, want %q", got, wantCode)
	}
	if got := rawError["message"]; got != wantMessage {
		t.Fatalf("error.message = %#v, want %q", got, wantMessage)
	}
}

func assertFirstDetail(t *testing.T, rr *httptest.ResponseRecorder, wantField, wantCode, wantMessage string) {
	t.Helper()

	payload := decodePayload(t, rr)
	rawError, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", payload["error"])
	}

	details, ok := rawError["details"].([]any)
	if !ok || len(details) == 0 {
		t.Fatalf("error.details = %#v, want non-empty array", rawError["details"])
	}

	first, ok := details[0].(map[string]any)
	if !ok {
		t.Fatalf("error.details[0] = %#v, want object", details[0])
	}
	if got := first["field"]; got != wantField {
		t.Fatalf("error.details[0].field = %#v, want %q", got, wantField)
	}
	if got := first["code"]; got != wantCode {
		t.Fatalf("error.details[0].code = %#v, want %q", got, wantCode)
	}
	if got := first["message"]; got != wantMessage {
		t.Fatalf("error.details[0].message = %#v, want %q", got, wantMessage)
	}
}
