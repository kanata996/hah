package hah

import (
	"context"
	"net/http"

	internalrender "github.com/kanata996/hah/internal/render"
	"github.com/kanata996/hah/internal/reqid"
)

type scopeState struct {
	config scopeConfig
}

type scopeStateKey struct{}

type scopeConfig struct {
	writeError writeErrorConfig
	success    successConfig
}

type successConfig struct {
	status int
}

// WithResponses applies hah's route-scoped response behavior to a chi/net/http
// subtree. It provides response configuration and shared request state for hah
// render helpers.
func WithResponses(opts ...ScopeOption) func(http.Handler) http.Handler {
	cfg := buildScopeConfig(opts...)

	return func(next http.Handler) http.Handler {
		if next == nil {
			panic("hah: WithResponses requires a non-nil next handler")
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, withScopeConfig(r, cfg))
		})
	}
}

func buildScopeConfig(opts ...ScopeOption) scopeConfig {
	cfg := scopeConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.applyScope(&cfg)
		}
	}
	if !cfg.writeError.reporterSet {
		cfg.writeError.reporter = defaultErrorReporter
	}
	return cfg
}

func withScopeConfig(r *http.Request, cfg scopeConfig) *http.Request {
	if state := scopeStateFrom(r); state != nil {
		cfg = mergeScopeConfig(state.config, cfg)
	}

	r = reqid.EnsureState(r)
	r = internalrender.EnsureState(r)
	if cfg.success.status != 0 {
		internalrender.Status(r, cfg.success.status)
	}

	return r.WithContext(context.WithValue(
		r.Context(),
		scopeStateKey{},
		&scopeState{config: cfg},
	))
}

func scopeStateFrom(r *http.Request) *scopeState {
	state, _ := r.Context().Value(scopeStateKey{}).(*scopeState)
	return state
}

func mergeScopeConfig(base, override scopeConfig) scopeConfig {
	return scopeConfig{
		writeError: mergeWriteErrorConfig(base.writeError, override.writeError),
		success:    mergeSuccessConfig(base.success, override.success),
	}
}

func mergeSuccessConfig(base, override successConfig) successConfig {
	if override.status != 0 {
		return override
	}
	return base
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
