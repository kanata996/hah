package resp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/kanata996/hah/errx"
)

// ErrorResponder coordinates HTTP error normalization, request-log enrichment,
// error response writing, and independent 5xx error logging.
//
// The zero value is usable and keeps resp on a pure net/http baseline:
//   - Logger falls back to slog.Default()
//   - AsHTTPError falls back to resp's default normalization
//   - ContextAttrs / AnnotateRequestLog are no-ops
//   - RequestLogAttrs falls back to resp's default low-noise 5xx attrs
type ErrorResponder struct {
	Logger             *slog.Logger
	AsHTTPError        func(error) *errx.HTTPError
	ContextAttrs       func(context.Context) []slog.Attr
	AnnotateRequestLog func(*http.Request, []slog.Attr)
	RequestLogAttrs    func(error, *errx.HTTPError) []slog.Attr
}

var defaultErrorResponder ErrorResponder

// NewErrorResponder returns a responder with the default pure net/http
// behavior. Callers may mutate the returned value to customize strategy.
func NewErrorResponder() *ErrorResponder {
	return &ErrorResponder{}
}

func (r *ErrorResponder) Respond(w http.ResponseWriter, req *http.Request, err error) error {
	if err == nil {
		return nil
	}

	httpErr := r.httpError(err)

	var responseStartedErr *responseWriteError
	if errors.As(err, &responseStartedErr) && responseStartedErr != nil && responseStartedErr.responseStarted {
		r.logServerError(req, httpErr, err)
		return err
	}
	if responseAlreadyStarted(w) {
		r.logServerError(req, httpErr, err)
		return err
	}

	r.annotateRequestLog(req, r.requestLogAttrs(err, httpErr))
	r.logServerError(req, httpErr, err)
	writeErr := writeHTTPError(w, req, httpErr)
	r.logErrorResponseWriteFailure(req, httpErr, writeErr)
	return writeErr
}

func (r *ErrorResponder) logger() *slog.Logger {
	if r != nil && r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

func (r *ErrorResponder) httpError(err error) *errx.HTTPError {
	if r != nil && r.AsHTTPError != nil {
		return r.AsHTTPError(err)
	}
	return asHTTPError(err)
}

func (r *ErrorResponder) contextAttrs(ctx context.Context) []slog.Attr {
	if r != nil && r.ContextAttrs != nil {
		return r.ContextAttrs(ctx)
	}
	return nil
}

func (r *ErrorResponder) annotateRequestLog(req *http.Request, attrs []slog.Attr) {
	if r == nil || r.AnnotateRequestLog == nil || req == nil || len(attrs) == 0 {
		return
	}
	r.AnnotateRequestLog(req, attrs)
}

func (r *ErrorResponder) requestLogAttrs(err error, httpErr *errx.HTTPError) []slog.Attr {
	if r != nil && r.RequestLogAttrs != nil {
		return r.RequestLogAttrs(err, httpErr)
	}
	return requestErrorLogAttrs(err, httpErr)
}
