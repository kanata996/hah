package hah

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/kanata996/hah/internal/reqid"
)

func TestContractAllowsNilNextHandler(t *testing.T) {
	handler := Contract()(nil)
	if handler == nil {
		t.Fatal("Contract()(nil) = nil")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestWithContractConfigGuardsAndStoresState(t *testing.T) {
	if got := withContractConfig(nil, contractConfig{}); got != nil {
		t.Fatalf("withContractConfig(nil, cfg) = %#v, want nil", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	cfg := contractConfig{
		writeError: writeErrorConfig{
			mappers: []ErrorMapper{
				func(err error) *HTTPError { return nil },
			},
		},
	}

	got := withContractConfig(req, cfg)
	if got == nil {
		t.Fatal("withContractConfig(req, cfg) = nil")
	}
	if got == req {
		t.Fatal("withContractConfig(req, cfg) returned original request")
	}

	if state := contractStateFrom(got); state == nil {
		t.Fatal("contractStateFrom(got) = nil")
	} else if len(state.config.writeError.mappers) != 1 {
		t.Fatalf("len(state.config.writeError.mappers) = %d, want 1", len(state.config.writeError.mappers))
	}

	if state := contractStateFrom(got); state == nil || state.storedRequestID() != nil {
		t.Fatalf("contractStateFrom(got).storedRequestID() = %#v, want nil before first use", state)
	}

	_, id := ensureRequestID(got)
	_, idAgain := ensureRequestID(got)
	if id == "" {
		t.Fatal("ensureRequestID(got) id = empty, want generated request id")
	}
	if idAgain != id {
		t.Fatalf("ensureRequestID(got) ids = (%q, %q), want same generated value", id, idAgain)
	}
}

func TestContractStateFromNilAndMissingState(t *testing.T) {
	if got := contractStateFrom(nil); got != nil {
		t.Fatalf("contractStateFrom(nil) = %#v, want nil", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := contractStateFrom(req); got != nil {
		t.Fatalf("contractStateFrom(req) = %#v, want nil", got)
	}

	var nilState *contractState
	if got := nilState.storedRequestID(); got != nil {
		t.Fatalf("nilState.storedRequestID() = %#v, want nil", got)
	}
	if got := nilState.ensureRequestID(req); got != nil {
		t.Fatalf("nilState.ensureRequestID(req) = %#v, want nil", got)
	}
}

func TestContractEnsureRequestIDAdoptsExistingRequestState(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = reqid.Set(req, "req_upstream")
	state := &contractState{}

	current := state.ensureRequestID(req)
	if current == nil {
		t.Fatal("state.ensureRequestID(req) = nil")
	}
	if current != reqid.StateFrom(req) {
		t.Fatal("state.ensureRequestID(req) did not reuse existing request state")
	}
	if id := reqid.EnsureID(current); id != "req_upstream" {
		t.Fatalf("reqid.EnsureID(current) = %q, want req_upstream", id)
	}
}

func TestMergeWriteErrorConfigCoversAllBranches(t *testing.T) {
	mapOuter := func(err error) *HTTPError {
		if errors.Is(err, errSentinelOuter) {
			return NewHTTPError(http.StatusBadRequest, "outer", "outer")
		}
		return nil
	}
	mapInner := func(err error) *HTTPError {
		if errors.Is(err, errSentinelInner) {
			return NewHTTPError(http.StatusConflict, "inner", "inner")
		}
		return nil
	}
	reporterOuter := func(ErrorReport) {}
	reporterInner := func(ErrorReport) {}

	tests := []struct {
		name            string
		base            writeErrorConfig
		override        writeErrorConfig
		wantMapperCount int
		wantReporter    ErrorReporter
		wantReporterSet bool
	}{
		{
			name:            "both empty gets default reporter",
			base:            writeErrorConfig{},
			override:        writeErrorConfig{},
			wantMapperCount: 0,
			wantReporter:    defaultErrorReporter,
			wantReporterSet: false,
		},
		{
			name: "override mappers only",
			override: writeErrorConfig{
				mappers: []ErrorMapper{mapInner},
			},
			wantMapperCount: 1,
			wantReporter:    defaultErrorReporter,
			wantReporterSet: false,
		},
		{
			name: "base mappers only",
			base: writeErrorConfig{
				mappers: []ErrorMapper{mapOuter},
			},
			wantMapperCount: 1,
			wantReporter:    defaultErrorReporter,
			wantReporterSet: false,
		},
		{
			name: "override prepends mappers and reporter",
			base: writeErrorConfig{
				mappers:     []ErrorMapper{mapOuter},
				reporter:    reporterOuter,
				reporterSet: true,
			},
			override: writeErrorConfig{
				mappers:     []ErrorMapper{mapInner},
				reporter:    reporterInner,
				reporterSet: true,
			},
			wantMapperCount: 2,
			wantReporter:    reporterInner,
			wantReporterSet: true,
		},
		{
			name: "base reporter preserved when override unset",
			base: writeErrorConfig{
				reporter:    reporterOuter,
				reporterSet: true,
			},
			override:        writeErrorConfig{},
			wantReporter:    reporterOuter,
			wantReporterSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeWriteErrorConfig(tt.base, tt.override)

			if len(got.mappers) != tt.wantMapperCount {
				t.Fatalf("len(mergeWriteErrorConfig(...).mappers) = %d, want %d", len(got.mappers), tt.wantMapperCount)
			}
			if !sameReporter(got.reporter, tt.wantReporter) {
				t.Fatalf("mergeWriteErrorConfig(...).reporter mismatch")
			}
			if got.reporterSet != tt.wantReporterSet {
				t.Fatalf("mergeWriteErrorConfig(...).reporterSet = %t, want %t", got.reporterSet, tt.wantReporterSet)
			}
			if tt.wantMapperCount == 2 {
				if mapped := got.mappers[0](errSentinelInner); mapped == nil || mapped.Code() != "inner" {
					t.Fatalf("got.mappers[0](inner) = %#v, want inner mapper", mapped)
				}
				if mapped := got.mappers[1](errSentinelOuter); mapped == nil || mapped.Code() != "outer" {
					t.Fatalf("got.mappers[1](outer) = %#v, want outer mapper", mapped)
				}
			}
		})
	}
}

func sameReporter(a, b ErrorReporter) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
	}
}

var (
	errSentinelOuter = errors.New("outer")
	errSentinelInner = errors.New("inner")
)
