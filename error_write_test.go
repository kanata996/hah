package hah_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kanata996/hah"
)

type failingResponseWriter struct {
	header   http.Header
	status   int
	writeErr error
	writes   int
}

func newFailingResponseWriter(err error) *failingResponseWriter {
	return &failingResponseWriter{
		header:   make(http.Header),
		writeErr: err,
	}
}

func (w *failingResponseWriter) Header() http.Header {
	return w.header
}

func (w *failingResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *failingResponseWriter) Write(_ []byte) (int, error) {
	w.writes++
	return 0, w.writeErr
}

func TestRenderErrorWritesImmediatelyWithoutMiddleware(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()

	if err := hah.RenderError(rr, req, hah.NewHTTPError(http.StatusConflict, "conflict", "conflict")); err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}

	assertErrorResponse(t, rr, http.StatusConflict, "conflict", "conflict")
}

func TestRenderErrorAllowsNilWriter(t *testing.T) {
	req := newRequest()

	if err := hah.RenderError(nil, req, hah.NewHTTPError(http.StatusConflict, "conflict", "conflict")); err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}
}

func TestRenderErrorAllowsNilRequest(t *testing.T) {
	var report hah.ErrorReport

	rr := newResponseRecorder()

	err := hah.RenderError(
		rr,
		nil,
		hah.NewHTTPError(http.StatusConflict, "conflict", "conflict"),
		hah.WithErrorReporter(func(r hah.ErrorReport) {
			report = r
		}),
	)
	if err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}

	assertErrorResponse(t, rr, http.StatusConflict, "conflict", "conflict")
	if report.Request != nil {
		t.Fatalf("report.Request = %#v, want nil", report.Request)
	}
	if report.RequestID == "" {
		t.Fatal("report.RequestID = empty, want generated request id")
	}
}

func TestRenderErrorAppliesMappersImmediately(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()
	target := errors.New("target")

	err := hah.RenderError(
		rr,
		req,
		target,
		hah.WithErrorMappers(func(err error) *hah.HTTPError {
			if errors.Is(err, target) {
				return hah.NewHTTPError(http.StatusNotFound, "route_not_found", "route not found")
			}
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}

	assertErrorResponse(t, rr, http.StatusNotFound, "route_not_found", "route not found")
}

func TestRenderErrorReturnsNilForNilError(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()

	if err := hah.RenderError(rr, req, nil); err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}
	if rr.Code != 200 && rr.Code != 0 {
		t.Fatalf("status = %d, want zero-value recorder status", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
}

func TestRenderErrorDoesNotRewriteAfterRender(t *testing.T) {
	var report hah.ErrorReport

	rr := newResponseRecorder()
	req := newRequest()

	if err := hah.Render(rr, req, map[string]any{"partial": true}); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	err := hah.RenderError(
		rr,
		req,
		hah.NewHTTPError(http.StatusUnauthorized, "unauthorized", "unauthorized"),
		hah.WithErrorReporter(func(r hah.ErrorReport) {
			report = r
		}),
	)
	if err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !report.ResponseStarted {
		t.Fatal("report.ResponseStarted = false, want true")
	}
	if report.PublicError == nil || report.PublicError.Status() != http.StatusUnauthorized {
		t.Fatalf("public error = %#v, want unauthorized mapping", report.PublicError)
	}
}

func TestRenderErrorHEADUsesStandardServerSemantics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := hah.RenderError(w, r, hah.NewHTTPError(http.StatusNotFound, "route_not_found", "route not found")); err != nil {
			t.Fatalf("RenderError() error = %v", err)
		}
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodHead, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("resp.Body.Close() error = %v", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	if len(body) != 0 {
		t.Fatalf("body length = %d, want 0", len(body))
	}
}

func TestRenderErrorReportsReqxDecodeObservation(t *testing.T) {
	var report hah.ErrorReport

	rr := newResponseRecorder()
	req := newRequest()
	req.Method = http.MethodPost
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(strings.NewReader("{"))

	var input struct {
		Name string `json:"name"`
	}
	err := hah.DecodeJSON(req, &input)
	if renderErr := hah.RenderError(rr, req, err, hah.WithErrorReporter(func(r hah.ErrorReport) {
		report = r
	})); renderErr != nil {
		t.Fatalf("RenderError() error = %v", renderErr)
	}

	assertErrorResponse(t, rr, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
	if report.PublicError == nil || report.PublicError.Code() != "invalid_json" {
		t.Fatalf("public error = %#v, want invalid_json", report.PublicError)
	}
}

func TestRenderErrorReportsRenderFailureObservation(t *testing.T) {
	var report hah.ErrorReport

	rr := newResponseRecorder()
	req := newRequest()

	err := hah.Render(rr, req, map[string]any{"bad": func() {}})
	if renderErr := hah.RenderError(rr, req, err, hah.WithErrorReporter(func(r hah.ErrorReport) {
		report = r
	})); renderErr != nil {
		t.Fatalf("RenderError() error = %v", renderErr)
	}

	assertErrorResponse(t, rr, http.StatusInternalServerError, "internal_error", "internal server error")
	if report.PublicError == nil || report.PublicError.Code() != "internal_error" {
		t.Fatalf("public error = %#v, want internal_error", report.PublicError)
	}
}

func TestRenderErrorDropsInvalidDetailsWithoutExtraObservation(t *testing.T) {
	var reports []hah.ErrorReport

	rr := newResponseRecorder()
	req := newRequest()

	err := hah.RenderError(
		rr,
		req,
		hah.NewHTTPError(
			http.StatusBadRequest,
			"invalid_request",
			"request is invalid",
			func() {},
		),
		hah.WithErrorReporter(func(r hah.ErrorReport) {
			reports = append(reports, r)
		}),
	)
	if err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}

	assertErrorResponse(t, rr, http.StatusBadRequest, "invalid_request", "request is invalid")
	if len(reports) != 1 {
		t.Fatalf("reports len = %d, want 1", len(reports))
	}
	if reports[0].PublicError == nil || reports[0].PublicError.Status() != http.StatusBadRequest {
		t.Fatalf("reports[0].public = %#v, want 400 boundary error", reports[0].PublicError)
	}
	if reports[0].ResponseStarted {
		t.Fatal("reports[0].ResponseStarted = true, want false")
	}
}

func TestRenderErrorReportsWriteFailureAsSecondObservation(t *testing.T) {
	var reports []hah.ErrorReport

	rw := newFailingResponseWriter(errors.New("write failed"))
	req := newRequest()

	err := hah.RenderError(
		rw,
		req,
		hah.NewHTTPError(http.StatusBadRequest, "invalid_request", "request is invalid"),
		hah.WithErrorReporter(func(r hah.ErrorReport) {
			reports = append(reports, r)
		}),
	)
	if err == nil {
		t.Fatal("expected write error, got nil")
	}

	if len(reports) != 2 {
		t.Fatalf("reports len = %d, want 2", len(reports))
	}
	if reports[0].PublicError == nil || reports[0].PublicError.Status() != http.StatusBadRequest {
		t.Fatalf("reports[0].public = %#v, want 400 boundary error", reports[0].PublicError)
	}
	if reports[0].ResponseStarted {
		t.Fatal("reports[0].ResponseStarted = true, want false")
	}
	if reports[1].Error == nil || reports[1].Error.Error() != "write failed" {
		t.Fatalf("reports[1].error = %#v, want write failed", reports[1].Error)
	}
	if reports[1].PublicError == nil || reports[1].PublicError.Status() != http.StatusInternalServerError {
		t.Fatalf("reports[1].public = %#v, want 500 internal error", reports[1].PublicError)
	}
	if !reports[1].ResponseStarted {
		t.Fatal("reports[1].ResponseStarted = false, want true")
	}
	if reports[1].RequestID != reports[0].RequestID {
		t.Fatalf("reports request ids = (%q, %q), want same generated value", reports[0].RequestID, reports[1].RequestID)
	}
	if rw.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rw.status, http.StatusBadRequest)
	}
	if got := rw.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if rw.writes != 1 {
		t.Fatalf("writes = %d, want 1", rw.writes)
	}
}

func TestRenderErrorUsesExplicitRequestIDWithoutMiddleware(t *testing.T) {
	var report hah.ErrorReport

	rr := newResponseRecorder()
	req := hah.SetRequestID(newRequest(), "req_direct")

	err := hah.RenderError(
		rr,
		req,
		hah.NewHTTPError(http.StatusConflict, "conflict", "conflict"),
		hah.WithErrorReporter(func(r hah.ErrorReport) {
			report = r
		}),
	)
	if err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}

	assertErrorResponse(t, rr, http.StatusConflict, "conflict", "conflict")
	if got := report.RequestID; got != "req_direct" {
		t.Fatalf("request id = %q, want req_direct", got)
	}
}

func TestWithErrorMappersUsesFirstMatchInOrder(t *testing.T) {
	target := errors.New("target")
	rr := newResponseRecorder()
	req := newRequest()

	err := hah.RenderError(
		rr,
		req,
		target,
		hah.WithErrorMappers(
			func(err error) *hah.HTTPError { return nil },
			func(err error) *hah.HTTPError {
				if errors.Is(err, target) {
					return hah.NewHTTPError(http.StatusConflict, "conflict", "conflict")
				}
				return nil
			},
			func(err error) *hah.HTTPError {
				if errors.Is(err, target) {
					return hah.NewHTTPError(http.StatusBadRequest, "wrong", "wrong")
				}
				return nil
			},
		),
	)
	if err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}

	assertErrorResponse(t, rr, http.StatusConflict, "conflict", "conflict")
}

func TestWithErrorMappersIgnoresNil(t *testing.T) {
	target := errors.New("target")
	rr := newResponseRecorder()
	req := newRequest()

	err := hah.RenderError(
		rr,
		req,
		target,
		hah.WithErrorMappers(nil),
		hah.WithErrorReporter(nil),
	)
	if err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}

	assertErrorResponse(t, rr, http.StatusInternalServerError, "internal_error", "internal server error")
}
