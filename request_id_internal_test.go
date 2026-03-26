package hah

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kanata996/hah/internal/reqid"
)

func TestSetRequestIDUsesSharedRequestStateInsideWithResponses(t *testing.T) {
	req := withScopeConfig(httptest.NewRequest(http.MethodGet, "/", nil), scopeConfig{})
	if req == nil {
		t.Fatal("withScopeConfig(req, cfg) = nil")
	}

	got := SetRequestID(req, "req_wrap")
	if got != req {
		t.Fatal("SetRequestID(reqWithResponses, id) returned different request")
	}

	if current := reqid.StateFrom(req); current == nil {
		t.Fatal("reqid.StateFrom(req) = nil")
	} else if id := reqid.EnsureID(current); id != "req_wrap" {
		t.Fatalf("reqid.EnsureID(current) = %q, want req_wrap", id)
	}
	if state := scopeStateFrom(req); state == nil {
		t.Fatal("scopeStateFrom(req) = nil")
	}
}
