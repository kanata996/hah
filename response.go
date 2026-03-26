package hah

import (
	"net/http"

	"github.com/kanata996/hah/internal/core"
	"github.com/kanata996/hah/internal/errx"
)

// Respond writes a success envelope without meta.
// The status must permit a response body.
func Respond(w http.ResponseWriter, status int, data any) error {
	return errx.WithStage(core.WriteSuccess(w, status, data, nil, false), errx.StageWriteResponse)
}

// RespondWithMeta writes a success envelope with explicit meta.
// The status must permit a response body.
func RespondWithMeta(w http.ResponseWriter, status int, data any, meta any) error {
	return errx.WithStage(core.WriteSuccess(w, status, data, meta, true), errx.StageWriteResponse)
}

// RespondEmpty writes a body-less successful response.
func RespondEmpty(w http.ResponseWriter, status int) error {
	return errx.WithStage(core.WriteEmpty(w, status), errx.StageWriteResponse)
}
