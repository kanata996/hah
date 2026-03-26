package hah_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kanata996/hah"
	reqxpkg "github.com/kanata996/hah/reqx"
)

func TestDecodeAndValidateFacadeReturnsDecodeErrorWithoutValidation(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		type createUserRequest struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":`))
		req.Header.Set("Content-Type", "application/json")

		var body createUserRequest
		validationCalled := false
		err := hah.DecodeAndValidateJSON(req, &body, func(value *createUserRequest) []hah.Violation {
			validationCalled = true
			return nil
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if validationCalled {
			t.Fatal("validation function was called on decode failure")
		}

		var boundaryErr *hah.HTTPError
		if !errors.As(err, &boundaryErr) || boundaryErr == nil {
			t.Fatalf("error = %T, want *hah.HTTPError", err)
		}
		if got := boundaryErr.Code(); got != "invalid_json" {
			t.Fatalf("code = %q, want invalid_json", got)
		}
	})

	t.Run("query", func(t *testing.T) {
		type queryInput struct {
			Page int `query:"page"`
		}

		req := httptest.NewRequest(http.MethodGet, "/users?page=bad", nil)

		var query queryInput
		validationCalled := false
		err := hah.DecodeAndValidateQuery(req, &query, func(value *queryInput) []hah.Violation {
			validationCalled = true
			return nil
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if validationCalled {
			t.Fatal("validation function was called on decode failure")
		}

		var boundaryErr *hah.HTTPError
		if !errors.As(err, &boundaryErr) || boundaryErr == nil {
			t.Fatalf("error = %T, want *hah.HTTPError", err)
		}
		if got := boundaryErr.Code(); got != "invalid_request" {
			t.Fatalf("code = %q, want invalid_request", got)
		}
	})
}

func TestRenderErrorAdaptsReqxProblem(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)

	if err := hah.RenderError(rr, req, reqxpkg.NewProblem(http.StatusBadRequest, "invalid_request", "invalid request")); err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}

	assertErrorResponse(t, rr, http.StatusBadRequest, "invalid_request", "invalid request")
}
