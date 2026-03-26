package hah

import (
	"errors"
	"net/http"

	"github.com/kanata996/hah/internal/reqid"
	"github.com/kanata996/hah/internal/resp"
)

func handleErrorWithConfig(w http.ResponseWriter, r *http.Request, err error, cfg writeErrorConfig) {
	requestID := ""
	if cfg.reporter != nil {
		r, requestID = ensureRequestID(r)
	}

	mapped := mapBoundaryError(err, cfg)
	started := resp.ResponseStarted(w)

	reportError(cfg, r, err, mapped, requestID, started)

	if started {
		return
	}

	if writeErr := writeMappedError(w, mapped); writeErr != nil {
		public := defaultInternalError()
		writeStarted := resp.ResponseStarted(w)
		var degraded *resp.ErrorWriteDegraded
		if errors.As(writeErr, &degraded) && degraded.PreservedPublicResponse {
			public = mapped
			writeStarted = true
		}
		reportError(
			cfg,
			r,
			writeErr,
			public,
			requestID,
			writeStarted,
		)
	}
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
	if state := contractStateFrom(r); state != nil {
		return r, reqid.EnsureID(state.ensureRequestID(r))
	}

	return reqid.Ensure(r)
}

func writeMappedError(w http.ResponseWriter, mapped *HTTPError) error {
	if w == nil {
		return nil
	}

	return resp.WriteErrorPayload(w, resp.ErrorPayload{
		Status:  mapped.Status(),
		Code:    mapped.Code(),
		Message: mapped.Message(),
		Details: mapped.Details(),
	})
}
