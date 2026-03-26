package hah

import (
	"net/http"

	"github.com/kanata996/hah/internal/resp"
)

// Respond writes a success envelope without meta.
// The status must permit a response body.
func Respond(w http.ResponseWriter, status int, data any) error {
	return resp.WriteSuccess(w, status, data, nil, false)
}

// RespondWithMeta writes a success envelope with explicit meta.
// The status must permit a response body.
func RespondWithMeta(w http.ResponseWriter, status int, data any, meta any) error {
	return resp.WriteSuccess(w, status, data, meta, true)
}

// RespondEmpty writes a body-less successful response.
func RespondEmpty(w http.ResponseWriter, status int) error {
	return resp.WriteEmpty(w, status)
}
