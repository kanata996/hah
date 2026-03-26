package hah

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kanata996/hah/reqx"
)

func TestMapBoundaryErrorSkipsNilMapper(t *testing.T) {
	mapped := mapBoundaryError(errors.New("boom"), writeErrorConfig{
		mappers: []ErrorMapper{nil},
	})

	if got := mapped.Status(); got != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", got, http.StatusInternalServerError)
	}
	if got := mapped.Code(); got != "internal_error" {
		t.Fatalf("code = %q, want internal_error", got)
	}
}

func TestMapBoundaryErrorDefaultsNilError(t *testing.T) {
	mapped := mapBoundaryError(nil, writeErrorConfig{})
	if got := mapped.Status(); got != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", got, http.StatusInternalServerError)
	}
}

func TestMapBoundaryErrorRecognizesBoundaryError(t *testing.T) {
	boundary := NewHTTPError(http.StatusConflict, "conflict", "conflict")

	mapped := mapBoundaryError(boundary, writeErrorConfig{})
	if mapped != boundary {
		t.Fatalf("public = %#v, want original boundary error %#v", mapped, boundary)
	}
}

func TestMapBoundaryErrorUsesMapper(t *testing.T) {
	target := errors.New("boom")
	mapped := NewHTTPError(http.StatusGone, "gone", "gone")

	got := mapBoundaryError(target, writeErrorConfig{
		mappers: []ErrorMapper{
			func(err error) *HTTPError {
				if errors.Is(err, target) {
					return mapped
				}
				return nil
			},
		},
	})

	if got != mapped {
		t.Fatalf("public = %#v, want mapped error %#v", got, mapped)
	}
}

func TestMapBoundaryErrorRecognizesReqxProblem(t *testing.T) {
	// Keep direct reqx coverage here so the root package explicitly exercises the
	// reqx -> hah bridge independent of the root facade helpers.
	mapped := mapBoundaryError(reqx.NewProblem(
		http.StatusUnprocessableEntity,
		"invalid_request",
		"request contains invalid fields",
		map[string]any{"field": "name"},
	), writeErrorConfig{})
	if got := mapped.Status(); got != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", got, http.StatusUnprocessableEntity)
	}
	if got := mapped.Code(); got != "invalid_request" {
		t.Fatalf("code = %q, want invalid_request", got)
	}
	if got := mapped.Message(); got != "request contains invalid fields" {
		t.Fatalf("message = %q, want request contains invalid fields", got)
	}
	if details := mapped.Details(); len(details) != 1 {
		t.Fatalf("details = %#v, want one detail", details)
	}
}

func TestRenderErrorWithNilReporterDisablesDefaultLogging(t *testing.T) {
	var logs bytes.Buffer
	previousWriter := errorLogger.Writer()
	errorLogger.SetOutput(&logs)
	defer errorLogger.SetOutput(previousWriter)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := RenderError(rr, req, errors.New("boom"), WithErrorReporter(nil)); err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if logs.Len() != 0 {
		t.Fatalf("logs = %q, want empty", logs.String())
	}
}

func TestFilterErrorMappersDropsNilValues(t *testing.T) {
	filtered := filterErrorMappers(
		nil,
		func(err error) *HTTPError { return nil },
		nil,
	)

	if len(filtered) != 1 {
		t.Fatalf("len(filterErrorMappers(...)) = %d, want 1", len(filtered))
	}
	if filtered[0] == nil {
		t.Fatal("filtered[0] = nil, want non-nil mapper")
	}
}
