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

// ErrorReportHandler receives centralized observations for errors handled by
// hah.
type ErrorReportHandler func(ErrorReport)

// ErrorMapper maps an application error into a standardized boundary error.
type ErrorMapper func(err error) *HTTPError

// ScopeOption customizes route-scoped hah response behavior.
type ScopeOption interface {
	applyScope(*scopeConfig)
}

type writeErrorConfig struct {
	mappers     []ErrorMapper
	reporter    ErrorReportHandler
	reporterSet bool
}

// ErrorOption customizes how hah maps, reports, and writes boundary errors.
// Error options also satisfy ScopeOption, so they can be reused inside
// WithResponses.
type ErrorOption interface {
	ScopeOption
	applyError(*writeErrorConfig)
}

func buildWriteErrorConfig(opts ...ErrorOption) writeErrorConfig {
	cfg := writeErrorConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.applyError(&cfg)
		}
	}
	if !cfg.reporterSet {
		cfg.reporter = defaultErrorReporter
	}
	return cfg
}

type errorReporterOption struct {
	reporter ErrorReportHandler
}

func (o errorReporterOption) applyScope(cfg *scopeConfig) {
	o.applyError(&cfg.writeError)
}

func (o errorReporterOption) applyError(cfg *writeErrorConfig) {
	cfg.reporter = o.reporter
	cfg.reporterSet = true
}

// ErrorReporter overrides error reporting for a WithResponses subtree or a
// RenderError call. Passing nil disables reporting for that scope.
func ErrorReporter(reporter ErrorReportHandler) ErrorOption {
	return errorReporterOption{reporter: reporter}
}

type errorMappersOption struct {
	mappers []ErrorMapper
}

func (o errorMappersOption) applyScope(cfg *scopeConfig) {
	o.applyError(&cfg.writeError)
}

func (o errorMappersOption) applyError(cfg *writeErrorConfig) {
	cfg.mappers = append(cfg.mappers, o.mappers...)
}

// ErrorMappers appends mappers to a WithResponses subtree, a RenderError call,
// or a reusable ErrorOption fragment.
func ErrorMappers(mappers ...ErrorMapper) ErrorOption {
	filtered := filterErrorMappers(mappers...)

	return errorMappersOption{mappers: filtered}
}
