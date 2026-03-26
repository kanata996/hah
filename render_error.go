package hah

import (
	"net/http"

	internalrender "github.com/kanata996/hah/internal/render"
	"github.com/kanata996/hah/internal/reqid"
)

// RenderError immediately maps, reports, and writes err at the business
// boundary. When the current request entered via WithResponses, route-scoped
// configuration is applied before any call-site options.
func RenderError(w http.ResponseWriter, r *http.Request, err error, opts ...ErrorOption) error {
	if err == nil {
		return nil
	}

	cfg := buildWriteErrorConfig(opts...)
	if r != nil {
		if state := scopeStateFrom(r); state != nil {
			cfg = mergeWriteErrorConfig(state.config.writeError, cfg)
		}
	}

	return handleRenderError(w, r, err, cfg)
}

func handleRenderError(w http.ResponseWriter, r *http.Request, err error, cfg writeErrorConfig) error {
	requestID := ""
	if cfg.reporter != nil {
		r, requestID = ensureRequestID(r)
	}

	mapped := mapBoundaryError(err, cfg)
	started := internalrender.ResponseStarted(r)

	reportError(cfg, r, err, mapped, requestID, started)

	if started || w == nil {
		return nil
	}

	writeErr := internalrender.RenderErrorPayload(w, r, internalrender.ErrorPayload{
		Status:  mapped.Status(),
		Code:    mapped.Code(),
		Message: mapped.Message(),
		Details: mapped.Details(),
	})
	if writeErr == nil {
		return nil
	}

	reportError(
		cfg,
		r,
		writeErr,
		defaultInternalError(),
		requestID,
		internalrender.ResponseStarted(r),
	)
	return writeErr
}

func reportError(cfg writeErrorConfig, r *http.Request, err error, public *HTTPError, requestID string, started bool) {
	if cfg.reporter == nil {
		return
	}

	cfg.reporter(ErrorReport{
		Request:         r,
		Error:           err,
		PublicError:     public,
		RequestID:       requestID,
		ResponseStarted: started,
	})
}

func ensureRequestID(r *http.Request) (*http.Request, string) {
	return reqid.Ensure(r)
}
