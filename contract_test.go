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
		hah.WriteError(w, r, target)
	}))

	rr := newResponseRecorder()
	req := newRequest()
	handler.ServeHTTP(rr, req)

	assertErrorResponse(t, rr, http.StatusNotFound, "user_not_found", "user not found")
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
			hah.WriteError(
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
		hah.WriteError(w, r, hah.NewHTTPError(http.StatusUnauthorized, "unauthorized", "unauthorized"))
	}))

	rr := newResponseRecorder()
	req := newRequest()
	handler.ServeHTTP(rr, req)

	assertErrorResponse(t, rr, http.StatusUnauthorized, "unauthorized", "unauthorized")
	if report.PublicError == nil || report.PublicError.Code() != "unauthorized" {
		t.Fatalf("public error = %#v, want unauthorized", report.PublicError)
	}
}

func TestWriteErrorReporterOverridesContractReporter(t *testing.T) {
	var contractReport hah.ErrorReport
	var writeReport hah.ErrorReport

	handler := hah.Contract(
		hah.WithContractErrorReporter(func(r hah.ErrorReport) {
			contractReport = r
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hah.WriteError(
			w,
			r,
			hah.NewHTTPError(http.StatusUnauthorized, "unauthorized", "unauthorized"),
			hah.WithErrorReporter(func(r hah.ErrorReport) {
				writeReport = r
			}),
		)
	}))

	rr := newResponseRecorder()
	req := newRequest()
	handler.ServeHTTP(rr, req)

	assertErrorResponse(t, rr, http.StatusUnauthorized, "unauthorized", "unauthorized")
	if contractReport.PublicError != nil {
		t.Fatalf("contract report = %#v, want zero value because WriteError reporter overrides it", contractReport)
	}
	if writeReport.PublicError == nil || writeReport.PublicError.Code() != "unauthorized" {
		t.Fatalf("write report = %#v, want unauthorized", writeReport)
	}
}

func TestContractTracksStartedResponseWithPlainRecorder(t *testing.T) {
	var report hah.ErrorReport

	handler := hah.Contract(
		hah.WithContractErrorReporter(func(r hah.ErrorReport) {
			report = r
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("partial")); err != nil {
			t.Fatalf("write error = %v", err)
		}
		hah.WriteError(w, r, hah.NewHTTPError(http.StatusUnauthorized, "unauthorized", "unauthorized"))
	}))

	rr := newResponseRecorder()
	req := newRequest()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Body.String(); got != "partial" {
		t.Fatalf("body = %q, want partial", got)
	}
	if !report.ResponseStarted {
		t.Fatal("report.ResponseStarted = false, want true")
	}
}

func TestContractReusesGeneratedRequestIDAcrossMultipleWriteErrors(t *testing.T) {
	var reports []hah.ErrorReport

	handler := hah.Contract(
		hah.WithContractErrorReporter(func(r hah.ErrorReport) {
			reports = append(reports, r)
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hah.WriteError(w, r, hah.BadRequest("first", "first"))
		hah.WriteError(w, r, hah.Conflict("second", "second"))
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

func TestContractReusesExplicitRequestIDAcrossMultipleWriteErrors(t *testing.T) {
	var reports []hah.ErrorReport

	handler := hah.Contract(
		hah.WithContractErrorReporter(func(r hah.ErrorReport) {
			reports = append(reports, r)
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hah.WriteError(w, r, hah.BadRequest("first", "first"))
		hah.WriteError(w, r, hah.Conflict("second", "second"))
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
