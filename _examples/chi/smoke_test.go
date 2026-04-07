package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/traceid"
	"github.com/google/uuid"
)

func TestChiCreateGetListAndDeleteFlow(t *testing.T) {
	handler := newRouter(newAccountStore())

	createReq := httptest.NewRequest(http.MethodPost, "/orgs/org_123/accounts", strings.NewReader(`{"name":"  Platform Team  "}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	handler.ServeHTTP(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRR.Code, http.StatusCreated)
	}

	var created account
	decodeJSON(t, createRR.Body.Bytes(), &created)
	if created.Name != "Platform Team" {
		t.Fatalf("created name = %q, want Platform Team", created.Name)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/orgs/org_123/accounts?limit=10", nil)
	listReq.Header.Set("X-Forwarded-For", "203.0.113.7")
	listRR := httptest.NewRecorder()
	handler.ServeHTTP(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRR.Code, http.StatusOK)
	}

	var listed map[string]any
	decodeJSON(t, listRR.Body.Bytes(), &listed)
	if got := int(listed["count"].(float64)); got != 3 {
		t.Fatalf("list count = %d, want 3", got)
	}
	if got := listed["request_id"]; got == "" || got == nil {
		t.Fatalf("request_id = %#v, want non-empty value", got)
	}
	if got := listed["trace_id"]; got == "" || got == nil {
		t.Fatalf("trace_id = %#v, want non-empty value", got)
	}
	if got := listRR.Header().Get(middleware.RequestIDHeader); got == "" {
		t.Fatal("missing X-Request-Id response header")
	}
	if got := listRR.Header().Get(traceid.Header); got == "" {
		t.Fatal("missing TraceId response header")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/orgs/org_123/accounts/"+created.ID, nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRR.Code, http.StatusOK)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/orgs/org_123/accounts/"+created.ID, nil)
	deleteReq.Header.Set("X-Actor", "ops@example.com")
	deleteRR := httptest.NewRecorder()
	handler.ServeHTTP(deleteRR, deleteReq)

	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteRR.Code, http.StatusNoContent)
	}
	if got := deleteRR.Header().Get("X-Deleted-By"); got != "ops@example.com" {
		t.Fatalf("X-Deleted-By = %q, want ops@example.com", got)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/orgs/org_123/accounts/"+created.ID, nil)
	missingRR := httptest.NewRecorder()
	handler.ServeHTTP(missingRR, missingReq)

	if missingRR.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want %d", missingRR.Code, http.StatusNotFound)
	}
}

func TestDeleteRequiresActorHeader(t *testing.T) {
	handler := newRouter(newAccountStore())

	req := httptest.NewRequest(http.MethodDelete, "/orgs/org_123/accounts/acct_001", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnprocessableEntity)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}

	var problem map[string]any
	decodeJSON(t, rr.Body.Bytes(), &problem)
	if got := problem["code"]; got != "invalid_request" {
		t.Fatalf("problem code = %#v, want invalid_request", got)
	}

	errorsList, ok := problem["errors"].([]any)
	if !ok || len(errorsList) == 0 {
		t.Fatalf("errors = %#v, want at least one violation", problem["errors"])
	}

	first, ok := errorsList[0].(map[string]any)
	if !ok {
		t.Fatalf("first error = %#v, want object", errorsList[0])
	}
	if got := first["in"]; got != "header" {
		t.Fatalf("violation in = %#v, want header", got)
	}
}

func TestHeartbeatAndPathValueBridge(t *testing.T) {
	handler := newRouter(newAccountStore())

	heartbeatReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	heartbeatRR := httptest.NewRecorder()
	handler.ServeHTTP(heartbeatRR, heartbeatReq)

	if heartbeatRR.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, want %d", heartbeatRR.Code, http.StatusOK)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/orgs/org_123/accounts/acct_001", nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("path bridge get status = %d, want %d", getRR.Code, http.StatusOK)
	}

	var fetched account
	decodeJSON(t, getRR.Body.Bytes(), &fetched)
	if fetched.ID != "acct_001" {
		t.Fatalf("fetched id = %q, want acct_001", fetched.ID)
	}
}

func TestRequestIDTraceIDAndHTTPLogIntegration(t *testing.T) {
	var logBuf bytes.Buffer
	logger := newExampleLogger(&logBuf)
	handler := newRouterWithLogger(newAccountStore(), logger)

	req := httptest.NewRequest(http.MethodGet, "/orgs/org_123/accounts?limit=1", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	requestID := rr.Header().Get(middleware.RequestIDHeader)
	if requestID == "" {
		t.Fatal("missing X-Request-Id response header")
	}

	traceID := rr.Header().Get(traceid.Header)
	if traceID == "" {
		t.Fatal("missing TraceId response header")
	}
	if _, err := uuid.Parse(traceID); err != nil {
		t.Fatalf("TraceId header = %q, want valid UUID: %v", traceID, err)
	}

	lines := bytes.Split(bytes.TrimSpace(logBuf.Bytes()), []byte{'\n'})
	if len(lines) == 0 || len(bytes.TrimSpace(lines[0])) == 0 {
		t.Fatal("expected request log output")
	}

	var entry map[string]any
	decodeJSON(t, lines[len(lines)-1], &entry)
	if got := entry["request.id"]; got != requestID {
		t.Fatalf("request.id = %#v, want %q", got, requestID)
	}
	if got := entry["trace.id"]; got != traceID {
		t.Fatalf("trace.id = %#v, want %q", got, traceID)
	}
	if got := entry["http.response.status_code"]; got != float64(http.StatusOK) {
		t.Fatalf("http.response.status_code = %#v, want %d", got, http.StatusOK)
	}
}

func decodeJSON(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", string(body), err)
	}
}
