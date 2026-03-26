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

func TestWriteErrorAllowsNilRequest(t *testing.T) {
	var report hah.ErrorReport

	rr := newResponseRecorder()

	if ok := hah.WriteError(
		rr,
		nil,
		hah.NewHTTPError(http.StatusConflict, "conflict", "conflict"),
		hah.WithErrorReporter(func(r hah.ErrorReport) {
			report = r
		}),
	); !ok {
		t.Fatal("WriteError() = false, want true")
	}

	assertErrorResponse(t, rr, http.StatusConflict, "conflict", "conflict")
	if report.Request != nil {
		t.Fatalf("report.Request = %#v, want nil", report.Request)
	}
	if report.RequestID == "" {
		t.Fatal("report.RequestID = empty, want generated request id")
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
}

func TestWriteErrorHEADUsesStandardServerSemantics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hah.WriteError(w, r, hah.NewHTTPError(http.StatusNotFound, "route_not_found", "route not found"))
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
}

func TestWriteErrorDropsInvalidDetailsWithoutExtraObservation(t *testing.T) {
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

func TestWriteErrorReportsWriteFailureAsSecondObservation(t *testing.T) {
	var reports []hah.ErrorReport

	rw := newFailingResponseWriter(errors.New("write failed"))
	req := newRequest()

	hah.WriteError(
		rw,
		req,
		hah.NewHTTPError(http.StatusBadRequest, "invalid_request", "request is invalid"),
		hah.WithErrorReporter(func(r hah.ErrorReport) {
			reports = append(reports, r)
		}),
	)

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
	if reports[1].ResponseStarted {
		t.Fatal("reports[1].ResponseStarted = true, want false")
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
	if len(reports) != 1 {
		t.Fatalf("reports len = %d, want 1", len(reports))
	}
	if reports[0].RequestID == "" {
		t.Fatal("reports[0].RequestID = empty, want generated request id")
	}
	if !strings.HasPrefix(reports[0].RequestID, "req_") {
		t.Fatalf("reports[0].RequestID = %q, want req_ prefix", reports[0].RequestID)
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
