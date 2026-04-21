package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateAndGetAccountFlow(t *testing.T) {
	handler := newServer(newAccountStore())

	createReq := httptest.NewRequest(http.MethodPost, "/orgs/org_123/accounts", strings.NewReader(`{"name":"  Platform Team  "}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	handler.ServeHTTP(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRR.Code, http.StatusCreated)
	}
	if got := createRR.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("create Content-Type = %q, want application/json", got)
	}

	var created account
	decodeJSON(t, mustResponseData(t, createRR.Body.Bytes()), &created)
	if created.OrgID != "org_123" {
		t.Fatalf("created org_id = %q, want org_123", created.OrgID)
	}
	if created.Name != "Platform Team" {
		t.Fatalf("created name = %q, want Platform Team", created.Name)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/orgs/org_123/accounts/"+created.ID, nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRR.Code, http.StatusOK)
	}

	var fetched account
	decodeJSON(t, mustResponseData(t, getRR.Body.Bytes()), &fetched)
	if fetched != created {
		t.Fatalf("fetched account = %#v, want %#v", fetched, created)
	}
}

func TestListAndDeleteAccountFlow(t *testing.T) {
	handler := newServer(newAccountStore())

	listReq := httptest.NewRequest(http.MethodGet, "/orgs/org_123/accounts?name=prim", nil)
	listRR := httptest.NewRecorder()
	handler.ServeHTTP(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRR.Code, http.StatusOK)
	}

	var listed map[string]any
	decodeJSON(t, mustResponseData(t, listRR.Body.Bytes()), &listed)
	if got := int(listed["count"].(float64)); got != 1 {
		t.Fatalf("list count = %d, want 1", got)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/orgs/org_123/accounts/acct_001", nil)
	deleteRR := httptest.NewRecorder()
	handler.ServeHTTP(deleteRR, deleteReq)

	if deleteRR.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRR.Code, http.StatusOK)
	}

	deletePayload := decodeEnvelope(t, deleteRR.Body.Bytes())
	if got := deletePayload["code"]; got != float64(0) {
		t.Fatalf("delete code = %#v, want 0", got)
	}
	if _, exists := deletePayload["data"]; exists {
		t.Fatalf("delete data unexpectedly present: %#v", deletePayload["data"])
	}

	getReq := httptest.NewRequest(http.MethodGet, "/orgs/org_123/accounts/acct_001", nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusNotFound {
		t.Fatalf("post-delete get status = %d, want %d", getRR.Code, http.StatusNotFound)
	}

	problem := decodeEnvelope(t, getRR.Body.Bytes())
	if got := problem["code"]; got != float64(40400) {
		t.Fatalf("top code = %#v, want 40400", got)
	}
	errorValue := mustErrorObject(t, problem)
	if got := errorValue["reason"]; got != "account_not_found" {
		t.Fatalf("error.reason = %#v, want account_not_found", got)
	}
}

func TestValidationFailureReturnsErrorEnvelope(t *testing.T) {
	handler := newServer(newAccountStore())

	req := httptest.NewRequest(http.MethodPost, "/orgs/org_123/accounts", strings.NewReader(`{"name":"  "}`))
	req.Header.Set("Content-Type", "application/json")
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
	if got := problem["message"]; got != "request contains invalid fields" {
		t.Fatalf("message = %#v, want request contains invalid fields", got)
	}

	errorValue := mustErrorObject(t, problem)
	if got := errorValue["reason"]; got != "invalid_request" {
		t.Fatalf("error.reason = %#v, want invalid_request", got)
	}

	fields, ok := errorValue["fields"].([]any)
	if !ok || len(fields) == 0 {
		t.Fatalf("fields = %#v, want at least one field error", errorValue["fields"])
	}

	first, ok := fields[0].(map[string]any)
	if !ok {
		t.Fatalf("first field = %#v, want object", fields[0])
	}
	if got := first["field"]; got != "name" {
		t.Fatalf("field error field = %#v, want name", got)
	}
	if got := first["detail"]; got != "is required" {
		t.Fatalf("field error detail = %#v, want is required", got)
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
