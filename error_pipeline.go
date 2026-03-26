package hah

import (
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
		reportError(
			cfg,
			r,
			writeErr,
			defaultInternalError(),
			requestID,
			resp.ResponseStarted(w),
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
