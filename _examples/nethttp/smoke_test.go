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
	decodeJSON(t, createRR.Body.Bytes(), &created)
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
	decodeJSON(t, getRR.Body.Bytes(), &fetched)
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
	decodeJSON(t, listRR.Body.Bytes(), &listed)
	if got := int(listed["count"].(float64)); got != 1 {
		t.Fatalf("list count = %d, want 1", got)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/orgs/org_123/accounts/acct_001", nil)
	deleteRR := httptest.NewRecorder()
	handler.ServeHTTP(deleteRR, deleteReq)

	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteRR.Code, http.StatusNoContent)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/orgs/org_123/accounts/acct_001", nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusNotFound {
		t.Fatalf("post-delete get status = %d, want %d", getRR.Code, http.StatusNotFound)
	}

	var problem map[string]any
	decodeJSON(t, getRR.Body.Bytes(), &problem)
	if got := problem["code"]; got != "account_not_found" {
		t.Fatalf("problem code = %#v, want account_not_found", got)
	}
}

func TestValidationFailureReturnsProblemJSON(t *testing.T) {
	handler := newServer(newAccountStore())

	req := httptest.NewRequest(http.MethodPost, "/orgs/org_123/accounts", strings.NewReader(`{"name":"  "}`))
	req.Header.Set("Content-Type", "application/json")
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
	if got := first["field"]; got != "name" {
		t.Fatalf("violation field = %#v, want name", got)
	}
}

func decodeJSON(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", string(body), err)
	}
}
