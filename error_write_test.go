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

type statusTrackingRecorder struct {
	*httptest.ResponseRecorder
	status       int
	bytesWritten int
}

func (w *statusTrackingRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseRecorder.WriteHeader(status)
}

func (w *statusTrackingRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseRecorder.Write(p)
	w.bytesWritten += n
	return n, err
}

func (w *statusTrackingRecorder) Status() int {
	return w.status
}

func (w *statusTrackingRecorder) BytesWritten() int {
	return w.bytesWritten
}

func TestWriteErrorWritesImmediatelyWithoutMiddleware(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()

	if ok := hah.WriteError(rr, req, hah.NewHTTPError(http.StatusConflict, "conflict", "conflict")); !ok {
		t.Fatal("WriteError() = false, want true")
	}

	assertErrorResponse(t, rr, http.StatusConflict, "conflict", "conflict")
}

func TestWriteErrorAllowsNilWriter(t *testing.T) {
	req := newRequest()

	if ok := hah.WriteError(nil, req, hah.NewHTTPError(http.StatusConflict, "conflict", "conflict")); !ok {
		t.Fatal("WriteError() = false, want true")
	}
}

func TestWriteErrorAppliesMappersImmediately(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()
	target := errors.New("target")

	if ok := hah.WriteError(
		rr,
		req,
		target,
		hah.WithErrorMappers(func(err error) *hah.HTTPError {
			if errors.Is(err, target) {
				return hah.NewHTTPError(http.StatusNotFound, "route_not_found", "route not found")
			}
			return nil
		}),
	); !ok {
		t.Fatal("WriteError() = false, want true")
	}

	assertErrorResponse(t, rr, http.StatusNotFound, "route_not_found", "route not found")
}

func TestWriteErrorReturnsFalseForNilError(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()

	if ok := hah.WriteError(rr, req, nil); ok {
		t.Fatal("WriteError() = true, want false")
	}
	if rr.Code != 200 && rr.Code != 0 {
		t.Fatalf("status = %d, want zero-value recorder status", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
}

func TestWriteErrorDoesNotRewriteStartedStatusTrackingWriter(t *testing.T) {
	rr := &statusTrackingRecorder{ResponseRecorder: newResponseRecorder()}
	req := newRequest()

	if _, err := rr.Write([]byte("partial")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	hah.WriteError(rr, req, hah.NewHTTPError(http.StatusUnauthorized, "unauthorized", "unauthorized"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Body.String(); got != "partial" {
		t.Fatalf("body = %q, want partial", got)
	}
}

func TestWriteErrorReportsStartedResponseWithoutRewrite(t *testing.T) {
	var report hah.ErrorReport

	rr := &statusTrackingRecorder{ResponseRecorder: newResponseRecorder()}
	req := newRequest()

	if _, err := rr.Write([]byte("partial")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	hah.WriteError(
		rr,
		req,
		hah.NewHTTPError(http.StatusUnauthorized, "unauthorized", "unauthorized"),
		hah.WithErrorReporter(func(r hah.ErrorReport) {
			report = r
		}),
	)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Body.String(); got != "partial" {
		t.Fatalf("body = %q, want partial", got)
	}
	if !report.ResponseStarted {
		t.Fatal("report.ResponseStarted = false, want true")
	}
	if report.PublicError == nil || report.PublicError.Status() != http.StatusUnauthorized {
		t.Fatalf("public error = %#v, want unauthorized mapping", report.PublicError)
	}
	if report.Stage != "processing" {
		t.Fatalf("stage = %q, want processing", report.Stage)
	}
}

func TestWriteErrorWritesNoBodyForHEADError(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()
	req.Method = http.MethodHead

	hah.WriteError(rr, req, hah.NewHTTPError(http.StatusNotFound, "route_not_found", "route not found"))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
}

func TestWriteErrorReportsReqxDecodeObservation(t *testing.T) {
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
	hah.WriteError(rr, req, err, hah.WithErrorReporter(func(r hah.ErrorReport) {
		report = r
	}))

	assertErrorResponse(t, rr, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
	if report.PublicError == nil || report.PublicError.Code() != "invalid_json" {
		t.Fatalf("public error = %#v, want invalid_json", report.PublicError)
	}
	if report.Stage != "decode" {
		t.Fatalf("stage = %q, want decode", report.Stage)
	}
}

func TestWriteErrorReportsWriteResponseObservation(t *testing.T) {
	var report hah.ErrorReport

	rr := newResponseRecorder()
	req := newRequest()

	err := hah.Respond(rr, http.StatusOK, map[string]any{"bad": func() {}})
	hah.WriteError(rr, req, err, hah.WithErrorReporter(func(r hah.ErrorReport) {
		report = r
	}))

	assertErrorResponse(t, rr, http.StatusInternalServerError, "internal_error", "internal server error")
	if report.PublicError == nil || report.PublicError.Code() != "internal_error" {
		t.Fatalf("public error = %#v, want internal_error", report.PublicError)
	}
	if report.Stage != "write_response" {
		t.Fatalf("stage = %q, want write_response", report.Stage)
	}
}

func TestWriteErrorReportsWriteErrorFallbackAsSecondObservation(t *testing.T) {
	var reports []hah.ErrorReport

	rr := newResponseRecorder()
	req := newRequest()

	hah.WriteError(
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

	assertErrorResponse(t, rr, http.StatusBadRequest, "invalid_request", "request is invalid")
	if len(reports) != 2 {
		t.Fatalf("reports len = %d, want 2", len(reports))
	}

	if reports[0].PublicError == nil || reports[0].PublicError.Status() != http.StatusBadRequest {
		t.Fatalf("reports[0].public = %#v, want 400 boundary error", reports[0].PublicError)
	}
	if reports[0].Stage != "processing" {
		t.Fatalf("reports[0].stage = %q, want processing", reports[0].Stage)
	}
	if reports[0].ResponseStarted {
		t.Fatal("reports[0].ResponseStarted = true, want false")
	}

	if reports[1].PublicError == nil || reports[1].PublicError.Status() != http.StatusBadRequest {
		t.Fatalf("reports[1].public = %#v, want preserved 400 boundary error", reports[1].PublicError)
	}
	if reports[1].Stage != "write_response" {
		t.Fatalf("reports[1].stage = %q, want write_response", reports[1].Stage)
	}
	if !reports[1].ResponseStarted {
		t.Fatal("reports[1].ResponseStarted = false, want true")
	}
}

func TestWriteErrorUsesExplicitRequestIDWithoutMiddleware(t *testing.T) {
	var report hah.ErrorReport

	rr := newResponseRecorder()
	req := hah.SetRequestID(newRequest(), "req_direct")

	if ok := hah.WriteError(
		rr,
		req,
		hah.NewHTTPError(http.StatusConflict, "conflict", "conflict"),
		hah.WithErrorReporter(func(r hah.ErrorReport) {
			report = r
		}),
	); !ok {
		t.Fatal("WriteError() = false, want true")
	}

	assertErrorResponse(t, rr, http.StatusConflict, "conflict", "conflict")
	if got := report.RequestID; got != "req_direct" {
		t.Fatalf("request id = %q, want req_direct", got)
	}
}

func TestWriteErrorGeneratesStableRequestIDWhenMissing(t *testing.T) {
	var reports []hah.ErrorReport

	rr := newResponseRecorder()
	req := newRequest()

	hah.WriteError(
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

	assertErrorResponse(t, rr, http.StatusBadRequest, "invalid_request", "request is invalid")
	if len(reports) != 2 {
		t.Fatalf("reports len = %d, want 2", len(reports))
	}
	if reports[0].RequestID == "" {
		t.Fatal("reports[0].RequestID = empty, want generated request id")
	}
	if !strings.HasPrefix(reports[0].RequestID, "req_") {
		t.Fatalf("reports[0].RequestID = %q, want req_ prefix", reports[0].RequestID)
	}
	if reports[1].RequestID != reports[0].RequestID {
		t.Fatalf("reports request ids = (%q, %q), want same generated value", reports[0].RequestID, reports[1].RequestID)
	}
}

func TestWithErrorMappersUsesFirstMatchInOrder(t *testing.T) {
	target := errors.New("target")
	rr := newResponseRecorder()
	req := newRequest()

	hah.WriteError(
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

	assertErrorResponse(t, rr, http.StatusConflict, "conflict", "conflict")
}

func TestWithErrorMappersIgnoresNil(t *testing.T) {
	target := errors.New("target")
	rr := newResponseRecorder()
	req := newRequest()

	hah.WriteError(
		rr,
		req,
		target,
		hah.WithErrorMappers(nil),
		hah.WithErrorReporter(nil),
	)

	assertErrorResponse(t, rr, http.StatusInternalServerError, "internal_error", "internal server error")
}

func TestWithErrorMappersReturnsInternalErrorWhenNoMapperMatches(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()

	hah.WriteError(
		rr,
		req,
		errors.New("miss"),
		hah.WithErrorMappers(
			func(err error) *hah.HTTPError {
				return nil
			},
		),
		hah.WithErrorReporter(nil),
	)

	assertErrorResponse(t, rr, http.StatusInternalServerError, "internal_error", "internal server error")
}
