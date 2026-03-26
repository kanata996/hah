package hah

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/kanata996/hah/internal/reqid"
)

func TestContractRejectsNilNextHandler(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Contract()(nil) did not panic")
		}
	}()

	_ = Contract()(nil)
}

func TestWithContractConfigStoresConfigAndSharedRequestState(t *testing.T) {
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

	if current := reqid.StateFrom(got); current == nil {
		t.Fatal("reqid.StateFrom(got) = nil")
	} else if id := current.Get(); id != "" {
		t.Fatalf("reqid.StateFrom(got).Get() = %q, want empty before first use", id)
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

func TestContractStateFromMissingState(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := contractStateFrom(req); got != nil {
		t.Fatalf("contractStateFrom(req) = %#v, want nil", got)
	}
}

func TestWithContractConfigReusesExistingRequestState(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = reqid.Set(req, "req_upstream")

	got := withContractConfig(req, contractConfig{})
	if got == nil {
		t.Fatal("withContractConfig(req, cfg) = nil")
	}

	current := reqid.StateFrom(got)
	if current != reqid.StateFrom(req) {
		t.Fatal("withContractConfig(req, cfg) did not reuse existing request state")
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
