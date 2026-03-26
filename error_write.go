package hah

import (
	"errors"
	"net/http"

	"github.com/kanata996/hah/errcode"
	"github.com/kanata996/hah/internal/core"
	"github.com/kanata996/hah/internal/errx"
	"github.com/kanata996/hah/internal/reqid"
	"github.com/kanata996/hah/reqx"
)

// ErrorReport is the centralized observation emitted when hah handles an
// error. Stage identifies the internal observation point such as decode,
// validate, processing, or write_response. RequestID is the effective request
// identifier used by the current error handling chain.
type ErrorReport struct {
	Request         *http.Request
	Error           error
	PublicError     *HTTPError
	Stage           string
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

func handleErrorWithConfig(w http.ResponseWriter, r *http.Request, err error, cfg writeErrorConfig) {
	requestID := ""
	if cfg.reporter != nil {
		r, requestID = reqid.Ensure(r)
	}

	mapped := mapBoundaryError(err, cfg)
	observation := errx.Derive(err, errx.StageProcessing)
	started := core.ResponseStarted(w)

	reportError(cfg, r, err, mapped, observation.Stage, requestID, started)

	if started {
		return
	}

	if writeErr := writeMappedError(w, r, mapped); writeErr != nil {
		public := defaultInternalError()
		writeStarted := core.ResponseStarted(w)
		var degraded *core.ErrorWriteDegraded
		if errors.As(writeErr, &degraded) && degraded.PreservedPublicResponse {
			public = mapped
			writeStarted = true
		}
		reportError(
			cfg,
			r,
			writeErr,
			public,
			errx.StageWriteResponse,
			requestID,
			writeStarted,
		)
	}
}

func reportError(cfg writeErrorConfig, r *http.Request, err error, public *HTTPError, stage errx.Stage, requestID string, started bool) {
	if cfg.reporter == nil {
		return
	}

	cfg.reporter(ErrorReport{
		Request:         r,
		Error:           err,
		PublicError:     public,
		Stage:           stage.String(),
		RequestID:       requestID,
		ResponseStarted: started,
	})
}

func writeMappedError(w http.ResponseWriter, r *http.Request, mapped *HTTPError) error {
	if w == nil {
		return nil
	}
	if r != nil && r.Method == http.MethodHead {
		w.WriteHeader(mapped.Status())
		return nil
	}

	return core.WriteError(w, core.ErrorPayload{
		Status:  mapped.Status(),
		Code:    mapped.Code(),
		Message: mapped.Message(),
		Details: mapped.Details(),
	})
}

func mapBoundaryError(err error, cfg writeErrorConfig) *HTTPError {
	if err == nil {
		return defaultInternalError()
	}

	var boundaryErr *HTTPError
	if errors.As(err, &boundaryErr) && boundaryErr != nil {
		return boundaryErr
	}

	var problem *reqx.Problem
	if errors.As(err, &problem) && problem != nil {
		// Accept direct reqx usage as a first-class bridge: callers can bypass the
		// hah facade and WriteError should still normalize reqx problems into the
		// public hah error contract.
		return NewHTTPError(problem.Status(), problem.Code(), problem.Message(), problem.Details()...)
	}

	for _, mapper := range cfg.mappers {
		if mapper == nil {
			continue
		}
		if mapped := mapper(err); mapped != nil {
			return mapped
		}
	}

	return defaultInternalError()
}

func defaultInternalError() *HTTPError {
	return NewHTTPError(
		http.StatusInternalServerError,
		errcode.InternalError,
		"internal server error",
	)
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

func filterErrorMappers(mappers ...ErrorMapper) []ErrorMapper {
	filtered := make([]ErrorMapper, 0, len(mappers))
	for _, mapper := range mappers {
		if mapper != nil {
			filtered = append(filtered, mapper)
		}
	}
	return filtered
}
