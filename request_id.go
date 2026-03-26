package hah

import (
	"net/http"

	"github.com/kanata996/hah/internal/reqid"
)

// SetRequestID stores the request identifier used by hah error observations.
//
// Callers should always use the returned request for downstream handlers.
func SetRequestID(r *http.Request, id string) *http.Request {
	if state := contractStateFrom(r); state != nil {
		state.ensureRequestID(r).Set(id)
		return r
	}

	return reqid.Set(r, id)
}
