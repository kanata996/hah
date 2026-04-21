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
	decodeJSON(t, mustResponseData(t, createRR.Body.Bytes()), &created)
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
	decodeJSON(t, mustResponseData(t, listRR.Body.Bytes()), &listed)
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

	if deleteRR.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRR.Code, http.StatusOK)
	}
	if got := deleteRR.Header().Get("X-Deleted-By"); got != "ops@example.com" {
		t.Fatalf("X-Deleted-By = %q, want ops@example.com", got)
	}

	deletePayload := decodeEnvelope(t, deleteRR.Body.Bytes())
	if got := deletePayload["code"]; got != float64(0) {
		t.Fatalf("delete code = %#v, want 0", got)
	}
	if _, exists := deletePayload["data"]; exists {
		t.Fatalf("delete data unexpectedly present: %#v", deletePayload["data"])
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
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	problem := decodeEnvelope(t, rr.Body.Bytes())
	if got := problem["code"]; got != float64(42200) {
		t.Fatalf("top code = %#v, want 42200", got)
	}

	errorValue := mustErrorObject(t, problem)
	if got := errorValue["reason"]; got != "invalid_request" {
		t.Fatalf("error.reason = %#v, want invalid_request", got)
	}

	details, ok := errorValue["details"].([]any)
	if !ok || len(details) == 0 {
		t.Fatalf("details = %#v, want at least one field error", errorValue["details"])
	}

	first, ok := details[0].(map[string]any)
	if !ok {
		t.Fatalf("first detail = %#v, want object", details[0])
	}
	if got := first["in"]; got != "header" {
		t.Fatalf("field error in = %#v, want header", got)
	}
	if got := first["detail"]; got != "is required" {
		t.Fatalf("field error detail = %#v, want is required", got)
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
	decodeJSON(t, mustResponseData(t, getRR.Body.Bytes()), &fetched)
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

func decodeEnvelope(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", string(body), err)
	}
	return payload
}

func mustResponseData(t *testing.T, body []byte) []byte {
	t.Helper()

	payload := decodeEnvelope(t, body)
	data, ok := payload["data"]
	if !ok {
		t.Fatalf("data missing in payload: %#v", payload)
	}

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal(data) error = %v", err)
	}
	return raw
}

func mustErrorObject(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	errorValue, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", payload["error"])
	}
	return errorValue
}

func decodeJSON(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", string(body), err)
	}
}
