package hah

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kanata996/hah/internal/reqid"
)

func TestSetRequestIDUsesContractSharedState(t *testing.T) {
	req := withContractConfig(httptest.NewRequest(http.MethodGet, "/", nil), contractConfig{})
	if req == nil {
		t.Fatal("withContractConfig(req, cfg) = nil")
	}

	got := SetRequestID(req, "req_contract")
	if got != req {
		t.Fatal("SetRequestID(reqWithContract, id) returned different request")
	}

	state := contractStateFrom(req)
	if state == nil {
		t.Fatal("contractStateFrom(req) = nil")
	}
	if current := state.storedRequestID(); current == nil {
		t.Fatal("state.storedRequestID() = nil")
	} else if id := reqid.EnsureID(current); id != "req_contract" {
		t.Fatalf("reqid.EnsureID(current) = %q, want req_contract", id)
	}
	if current := reqid.StateFrom(req); current != nil {
		t.Fatalf("reqid.StateFrom(req) = %#v, want nil because contract stores request id state internally", current)
	}
}
