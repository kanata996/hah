package reqid

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetGuardsAndTrim(t *testing.T) {
	if got := Set(nil, "req_123"); got != nil {
		t.Fatalf("Set(nil, id) = %#v, want nil", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := Set(req, "   "); got != req {
		t.Fatalf("Set(req, empty) = %#v, want original request", got)
	}

	got := Set(req, " req_123 ")
	if got == nil {
		t.Fatal("Set(req, id) = nil")
	}
	if got == req {
		t.Fatal("Set(req, id) returned original request without state")
	}
	if current := StateFrom(got); current == nil || current.Get() != "req_123" {
		t.Fatalf("stored request id = %#v, want req_123", current)
	}
}

func TestSetUpdatesExistingSharedState(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withState(req, NewState())
	if req == nil {
		t.Fatal("withState(req, NewState()) = nil")
	}

	got := Set(req, "req_updated")
	if got != req {
		t.Fatal("Set(reqWithState, id) should reuse existing request")
	}

	if current := StateFrom(req); current == nil || current.Get() != "req_updated" {
		t.Fatalf("stored request id = %#v, want req_updated", current)
	}
}

func TestWithStateGuards(t *testing.T) {
	current := NewState()
	if got := withState(nil, current); got != nil {
		t.Fatalf("withState(nil, state) = %#v, want nil", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := withState(req, nil); got != req {
		t.Fatalf("withState(req, nil) = %#v, want original request", got)
	}
}

func TestStateGuards(t *testing.T) {
	var nilState *State
	nilState.Set("req_ignored")
	if got := nilState.Get(); got != "" {
		t.Fatalf("nilState.get() = %q, want empty", got)
	}

	if got := StateFrom(nil); got != nil {
		t.Fatalf("StateFrom(nil) = %#v, want nil", got)
	}

	if got := EnsureID(nilState); got == "" {
		t.Fatal("EnsureID(nilState) = empty, want generated id")
	}
}

func TestStateSetIgnoresEmptyNormalizedValue(t *testing.T) {
	current := NewState()
	current.Set("req_keep")
	current.Set("   ")

	if got := current.Get(); got != "req_keep" {
		t.Fatalf("current.Get() = %q, want req_keep", got)
	}
}

func TestEnsureAndEnsureID(t *testing.T) {
	previousGenerator := requestIDGenerator
	requestIDGenerator = func() string { return "req_generated" }
	defer func() {
		requestIDGenerator = previousGenerator
	}()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ensured, id := Ensure(req)
	if ensured == req {
		t.Fatal("Ensure(req) returned original request, want request with state")
	}
	if id != "req_generated" {
		t.Fatalf("Ensure(req) id = %q, want req_generated", id)
	}
	if got := StateFrom(ensured).Get(); got != "req_generated" {
		t.Fatalf("stored request id = %q, want req_generated", got)
	}

	if stateID := EnsureID(StateFrom(ensured)); stateID != "req_generated" {
		t.Fatalf("EnsureID(state) = %q, want req_generated", stateID)
	}

	ensuredAgain, idAgain := Ensure(ensured)
	if ensuredAgain != ensured {
		t.Fatal("Ensure(ensured) returned different request")
	}
	if idAgain != "req_generated" {
		t.Fatalf("Ensure(ensured) id = %q, want req_generated", idAgain)
	}

	nilReq, nilID := Ensure(nil)
	if nilReq != nil {
		t.Fatalf("Ensure(nil) request = %#v, want nil", nilReq)
	}
	if nilID != "req_generated" {
		t.Fatalf("Ensure(nil) id = %q, want req_generated", nilID)
	}
}

func TestDefaultRequestIDGenerator(t *testing.T) {
	previousEntropyRead := requestIDEntropyRead
	previousCounter := fallbackRequestIDCounter.Load()
	defer func() {
		requestIDEntropyRead = previousEntropyRead
		fallbackRequestIDCounter.Store(previousCounter)
	}()

	requestIDEntropyRead = func(p []byte) (int, error) {
		for i := range p {
			p[i] = byte(i)
		}
		return len(p), nil
	}

	if got := defaultRequestIDGenerator(); got != "req_000102030405060708090a0b0c0d0e0f" {
		t.Fatalf("defaultRequestIDGenerator() = %q, want deterministic hex id", got)
	}

	requestIDEntropyRead = func(p []byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}
	fallbackRequestIDCounter.Store(previousCounter)

	got := defaultRequestIDGenerator()
	if got == "" {
		t.Fatal("defaultRequestIDGenerator() fallback = empty, want generated id")
	}
	if !strings.HasPrefix(got, generatedRequestIDPrefix) {
		t.Fatalf("defaultRequestIDGenerator() fallback = %q, want req_ prefix", got)
	}
}
