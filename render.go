package hah

import (
	"net/http"

	"github.com/kanata996/hah/internal/render"
)

// Status sets the HTTP status hint used by subsequent Render calls.
func Status(r *http.Request, status int) {
	render.Status(r, status)
}

// Render writes a JSON success envelope using the current request render hints.
func Render(w http.ResponseWriter, r *http.Request, data any) error {
	return render.Render(w, r, data)
}

// RenderWithMeta writes a JSON success envelope with explicit meta.
func RenderWithMeta(w http.ResponseWriter, r *http.Request, data any, meta any) error {
	return render.RenderWithMeta(w, r, data, meta)
}

// RenderEmpty writes a body-less successful response.
func RenderEmpty(w http.ResponseWriter, r *http.Request, status int) error {
	return render.RenderEmpty(w, r, status)
}
