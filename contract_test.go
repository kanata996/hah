package hah_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/kanata996/hah"
)

func TestContractAppliesRouteScopedMappers(t *testing.T) {
	target := errors.New("target")

	handler := hah.Contract(
		hah.WithContractErrorMappers(func(err error) *hah.HTTPError {
			if errors.Is(err, target) {
				return hah.NotFound("user_not_found", "user not found")
			}
			return nil
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := hah.RenderError(w, r, target); err != nil {
			t.Fatalf("RenderError() error = %v", err)
		}
	}))

	rr := newResponseRecorder()
	req := newRequest()
	handler.ServeHTTP(rr, req)

	assertErrorResponse(t, rr, http.StatusNotFound, "user_not_found", "user not found")
}

func TestContractPanicsOnNilNext(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("Contract(...)(nil) did not panic")
		}
		if got := recovered.(string); got != "hah: Contract requires a non-nil next handler" {
			t.Fatalf("panic = %q, want %q", got, "hah: Contract requires a non-nil next handler")
		}
	}()

	hah.Contract()(nil)
}

func TestContractPrefersInnerMapperOverOuterAndCallSiteOverContract(t *testing.T) {
	target := errors.New("target")

	handler := hah.Contract(
		hah.WithContractErrorMappers(func(err error) *hah.HTTPError {
			if errors.Is(err, target) {
				return hah.NewHTTPError(http.StatusBadRequest, "outer", "outer")
			}
			return nil
		}),
	)(
		hah.Contract(
			hah.WithContractErrorMappers(func(err error) *hah.HTTPError {
				if errors.Is(err, target) {
					return hah.NewHTTPError(http.StatusConflict, "inner", "inner")
				}
				return nil
			}),
		)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := hah.RenderError(
				w,
				r,
				target,
				hah.WithErrorMappers(func(err error) *hah.HTTPError {
					if errors.Is(err, target) {
						return hah.NewHTTPError(http.StatusGone, "call", "call")
					}
					return nil
				}),
			)
			if err != nil {
				t.Fatalf("RenderError() error = %v", err)
			}
		})),
	)

	rr := newResponseRecorder()
	req := newRequest()
	handler.ServeHTTP(rr, req)

	assertErrorResponse(t, rr, http.StatusGone, "call", "call")
}

func TestContractAppliesRouteScopedReporter(t *testing.T) {
	var report hah.ErrorReport

	handler := hah.Contract(
		hah.WithContractErrorReporter(func(r hah.ErrorReport) {
			report = r
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := hah.RenderError(w, r, hah.NewHTTPError(http.StatusUnauthorized, "unauthorized", "unauthorized")); err != nil {
			t.Fatalf("RenderError() error = %v", err)
		}
	}))

	rr := newResponseRecorder()
	req := newRequest()
	handler.ServeHTTP(rr, req)

	assertErrorResponse(t, rr, http.StatusUnauthorized, "unauthorized", "unauthorized")
	if report.PublicError == nil || report.PublicError.Code() != "unauthorized" {
		t.Fatalf("public error = %#v, want unauthorized", report.PublicError)
	}
}

func TestRenderErrorReporterOverridesContractReporter(t *testing.T) {
	var contractReport hah.ErrorReport
	var writeReport hah.ErrorReport

	handler := hah.Contract(
		hah.WithContractErrorReporter(func(r hah.ErrorReport) {
			contractReport = r
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := hah.RenderError(
			w,
			r,
			hah.NewHTTPError(http.StatusUnauthorized, "unauthorized", "unauthorized"),
			hah.WithErrorReporter(func(r hah.ErrorReport) {
				writeReport = r
			}),
		)
		if err != nil {
			t.Fatalf("RenderError() error = %v", err)
		}
	}))

	rr := newResponseRecorder()
	req := newRequest()
	handler.ServeHTTP(rr, req)

	assertErrorResponse(t, rr, http.StatusUnauthorized, "unauthorized", "unauthorized")
	if contractReport.PublicError != nil {
		t.Fatalf("contract report = %#v, want zero value because RenderError reporter overrides it", contractReport)
	}
	if writeReport.PublicError == nil || writeReport.PublicError.Code() != "unauthorized" {
		t.Fatalf("write report = %#v, want unauthorized", writeReport)
	}
}

func TestContractReportsStartedAfterRender(t *testing.T) {
	var report hah.ErrorReport

	handler := hah.Contract(
		hah.WithContractErrorReporter(func(r hah.ErrorReport) {
			report = r
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := hah.Render(w, r, map[string]any{"partial": true}); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		if err := hah.RenderError(w, r, hah.NewHTTPError(http.StatusUnauthorized, "unauthorized", "unauthorized")); err != nil {
			t.Fatalf("RenderError() error = %v", err)
		}
	}))

	rr := newResponseRecorder()
	req := newRequest()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !report.ResponseStarted {
		t.Fatal("report.ResponseStarted = false, want true")
	}
}

func TestContractReusesGeneratedRequestIDAcrossMultipleRenderErrors(t *testing.T) {
	var reports []hah.ErrorReport

	handler := hah.Contract(
		hah.WithContractErrorReporter(func(r hah.ErrorReport) {
			reports = append(reports, r)
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := hah.RenderError(w, r, hah.BadRequest("first", "first")); err != nil {
			t.Fatalf("RenderError() error = %v", err)
		}
		if err := hah.RenderError(w, r, hah.Conflict("second", "second")); err != nil {
			t.Fatalf("RenderError() error = %v", err)
		}
	}))

	rr := newResponseRecorder()
	req := newRequest()
	handler.ServeHTTP(rr, req)

	assertErrorResponse(t, rr, http.StatusBadRequest, "first", "first")
	if len(reports) != 2 {
		t.Fatalf("reports len = %d, want 2", len(reports))
	}
	if reports[0].RequestID == "" {
		t.Fatal("reports[0].RequestID = empty, want generated request id")
	}
	if reports[1].RequestID != reports[0].RequestID {
		t.Fatalf("reports request ids = (%q, %q), want same generated value", reports[0].RequestID, reports[1].RequestID)
	}
	if !reports[1].ResponseStarted {
		t.Fatal("reports[1].ResponseStarted = false, want true")
	}
}

func TestContractReusesExplicitRequestIDAcrossMultipleRenderErrors(t *testing.T) {
	var reports []hah.ErrorReport

	handler := hah.Contract(
		hah.WithContractErrorReporter(func(r hah.ErrorReport) {
			reports = append(reports, r)
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := hah.RenderError(w, r, hah.BadRequest("first", "first")); err != nil {
			t.Fatalf("RenderError() error = %v", err)
		}
		if err := hah.RenderError(w, r, hah.Conflict("second", "second")); err != nil {
			t.Fatalf("RenderError() error = %v", err)
		}
	}))

	rr := newResponseRecorder()
	req := hah.SetRequestID(newRequest(), "req_explicit")
	handler.ServeHTTP(rr, req)

	assertErrorResponse(t, rr, http.StatusBadRequest, "first", "first")
	if len(reports) != 2 {
		t.Fatalf("reports len = %d, want 2", len(reports))
	}
	if reports[0].RequestID != "req_explicit" {
		t.Fatalf("reports[0].RequestID = %q, want req_explicit", reports[0].RequestID)
	}
	if reports[1].RequestID != "req_explicit" {
		t.Fatalf("reports[1].RequestID = %q, want req_explicit", reports[1].RequestID)
	}
}
