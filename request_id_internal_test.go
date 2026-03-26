package hah

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kanata996/hah/internal/reqid"
)

func TestSetRequestIDUsesSharedRequestStateInsideContract(t *testing.T) {
	req := withContractConfig(httptest.NewRequest(http.MethodGet, "/", nil), contractConfig{})
	if req == nil {
		t.Fatal("withContractConfig(req, cfg) = nil")
	}

	got := SetRequestID(req, "req_contract")
	if got != req {
		t.Fatal("SetRequestID(reqWithContract, id) returned different request")
	}

	if current := reqid.StateFrom(req); current == nil {
		t.Fatal("reqid.StateFrom(req) = nil")
	} else if id := reqid.EnsureID(current); id != "req_contract" {
		t.Fatalf("reqid.EnsureID(current) = %q, want req_contract", id)
	}
	if state := contractStateFrom(req); state == nil {
		t.Fatal("contractStateFrom(req) = nil")
	}
}
