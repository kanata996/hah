package hah

import (
	"context"
	"net/http"

	"github.com/kanata996/hah/internal/reqid"
)

type contractState struct {
	config contractConfig
}

type contractStateKey struct{}

type contractConfig struct {
	writeError writeErrorConfig
}

// ContractOption customizes route-scoped hah contract behavior.
type ContractOption func(*contractConfig)

// Contract marks a chi/net/http subtree as entering the hah API contract
// layer. It provides route-scoped error handling configuration and shared
// request state for hah render helpers.
func Contract(opts ...ContractOption) func(http.Handler) http.Handler {
	cfg := buildContractConfig(opts...)

	return func(next http.Handler) http.Handler {
		if next == nil {
			panic("hah: Contract requires a non-nil next handler")
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, withContractConfig(r, cfg))
		})
	}
}

func buildContractConfig(opts ...ContractOption) contractConfig {
	cfg := contractConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// WithContractErrorReporter overrides error reporting for RenderError calls
// made inside the current Contract subtree. Passing nil disables hah reporting
// for that subtree unless a RenderError call overrides it explicitly.
func WithContractErrorReporter(reporter ErrorReporter) ContractOption {
	return func(cfg *contractConfig) {
		cfg.writeError.reporter = reporter
		cfg.writeError.reporterSet = true
	}
}

// WithContractErrorMappers appends route-scoped mappers to the current
// Contract subtree. Inner Contract scopes are evaluated before outer scopes.
func WithContractErrorMappers(mappers ...ErrorMapper) ContractOption {
	filtered := filterErrorMappers(mappers...)

	return func(cfg *contractConfig) {
		cfg.writeError.mappers = append(cfg.writeError.mappers, filtered...)
	}
}

func withContractConfig(r *http.Request, cfg contractConfig) *http.Request {
	if state := contractStateFrom(r); state != nil {
		cfg = mergeContractConfig(state.config, cfg)
	}

	r = reqid.EnsureState(r)

	return r.WithContext(context.WithValue(
		r.Context(),
		contractStateKey{},
		&contractState{config: cfg},
	))
}

func contractStateFrom(r *http.Request) *contractState {
	state, _ := r.Context().Value(contractStateKey{}).(*contractState)
	return state
}

func mergeContractConfig(base, override contractConfig) contractConfig {
	return contractConfig{
		writeError: mergeWriteErrorConfig(base.writeError, override.writeError),
	}
}

func mergeWriteErrorConfig(base, override writeErrorConfig) writeErrorConfig {
	merged := base

	switch {
	case len(base.mappers) == 0 && len(override.mappers) == 0:
		merged.mappers = nil
	case len(base.mappers) == 0:
		merged.mappers = append([]ErrorMapper(nil), override.mappers...)
	case len(override.mappers) == 0:
		merged.mappers = append([]ErrorMapper(nil), base.mappers...)
	default:
		merged.mappers = make([]ErrorMapper, 0, len(override.mappers)+len(base.mappers))
		merged.mappers = append(merged.mappers, override.mappers...)
		merged.mappers = append(merged.mappers, base.mappers...)
	}

	if override.reporterSet {
		merged.reporter = override.reporter
		merged.reporterSet = true
	}

	if merged.reporter == nil && !merged.reporterSet {
		merged.reporter = defaultErrorReporter
	}

	return merged
}
