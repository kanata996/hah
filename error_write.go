package hah

import "net/http"

// ErrorReport is the centralized observation emitted when hah handles an
// error. RequestID is the effective request identifier used by the current
// error handling chain.
type ErrorReport struct {
	Request         *http.Request
	Error           error
	PublicError     *HTTPError
	RequestID       string
	ResponseStarted bool
}

// ErrorReporter receives centralized observations for errors handled by hah.
type ErrorReporter func(ErrorReport)

// ErrorMapper maps an application error into a standardized boundary error.
type ErrorMapper func(err error) *HTTPError

type writeErrorConfig struct {
	mappers     []ErrorMapper
	reporter    ErrorReporter
	reporterSet bool
}

// ErrorOption customizes how hah maps, reports, and writes boundary errors.
type ErrorOption func(*writeErrorConfig)

func buildWriteErrorConfig(opts ...ErrorOption) writeErrorConfig {
	cfg := writeErrorConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if !cfg.reporterSet {
		cfg.reporter = defaultErrorReporter
	}
	return cfg
}

// WriteError immediately maps, reports, and writes err at the business
// boundary. When the current request entered via Contract, route-scoped
// configuration is applied before any call-site options.
func WriteError(w http.ResponseWriter, r *http.Request, err error, opts ...ErrorOption) bool {
	if err == nil {
		return false
	}

	cfg := buildWriteErrorConfig(opts...)
	if state := contractStateFrom(r); state != nil {
		cfg = mergeWriteErrorConfig(state.config.writeError, cfg)
	}

	handleErrorWithConfig(w, r, err, cfg)
	return true
}

// WithErrorReporter overrides error reporting for errors written by hah.
// Passing nil disables reporting for hah error handling.
func WithErrorReporter(reporter ErrorReporter) ErrorOption {
	return func(cfg *writeErrorConfig) {
		cfg.reporter = reporter
		cfg.reporterSet = true
	}
}

// WithErrorMappers appends mappers to a WriteError call or reusable
// ErrorOption fragment.
func WithErrorMappers(mappers ...ErrorMapper) ErrorOption {
	filtered := filterErrorMappers(mappers...)

	return func(cfg *writeErrorConfig) {
		cfg.mappers = append(cfg.mappers, filtered...)
	}
}
