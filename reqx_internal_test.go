package hah

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONAdaptsReqxProblemToHTTPError(t *testing.T) {
	var body struct {
		Name string `json:"name"`
	}

	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":`))
	req.Header.Set("Content-Type", "application/json")

	err := DecodeJSON(req, &body)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var boundaryErr *HTTPError
	if !errors.As(err, &boundaryErr) || boundaryErr == nil {
		t.Fatalf("error = %T, want *HTTPError", err)
	}
	if got := boundaryErr.Code(); got != "invalid_json" {
		t.Fatalf("boundaryErr.Code() = %q, want invalid_json", got)
	}
}

func TestValidateAdaptsReqxProblemToHTTPError(t *testing.T) {
	type input struct {
		Name string `json:"name"`
	}

	err := Validate(&input{}, func(value *input) []Violation {
		return []Violation{{Field: "name"}}
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var boundaryErr *HTTPError
	if !errors.As(err, &boundaryErr) || boundaryErr == nil {
		t.Fatalf("error = %T, want *HTTPError", err)
	}
	if got := boundaryErr.Code(); got != "invalid_request" {
		t.Fatalf("boundaryErr.Code() = %q, want invalid_request", got)
	}
}

func TestInvalidRequestAdaptsReqxProblemToHTTPError(t *testing.T) {
	err := InvalidRequest(
		Violation{Field: "name"},
		Violation{Field: "age", Code: "required"},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var boundaryErr *HTTPError
	if !errors.As(err, &boundaryErr) || boundaryErr == nil {
		t.Fatalf("error = %T, want *HTTPError", err)
	}
	if got := boundaryErr.Status(); got != http.StatusUnprocessableEntity {
		t.Fatalf("boundaryErr.Status() = %d, want %d", got, http.StatusUnprocessableEntity)
	}
	if got := boundaryErr.Code(); got != "invalid_request" {
		t.Fatalf("boundaryErr.Code() = %q, want invalid_request", got)
	}

	gotDetails := boundaryErr.Details()
	if len(gotDetails) != 2 {
		t.Fatalf("len(boundaryErr.Details()) = %d, want 2", len(gotDetails))
	}

	first, ok := gotDetails[0].(Violation)
	if !ok {
		t.Fatalf("boundaryErr.Details()[0] = %T, want Violation", gotDetails[0])
	}
	if first != (Violation{Field: "name", Code: "invalid", Message: "is invalid"}) {
		t.Fatalf("boundaryErr.Details()[0] = %#v, want normalized violation", first)
	}

	second, ok := gotDetails[1].(Violation)
	if !ok {
		t.Fatalf("boundaryErr.Details()[1] = %T, want Violation", gotDetails[1])
	}
	if second != (Violation{Field: "age", Code: "required", Message: "is required"}) {
		t.Fatalf("boundaryErr.Details()[1] = %#v, want normalized violation", second)
	}
}

func TestDecodeJSONPassesThroughNonProblemErrors(t *testing.T) {
	var body struct {
		Name string `json:"name"`
	}

	err := DecodeJSON((*http.Request)(nil), &body)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var boundaryErr *HTTPError
	if errors.As(err, &boundaryErr) {
		t.Fatalf("error = %T, want non-boundary error", err)
	}
}

func TestDecodeJSONOptionFacades(t *testing.T) {
	t.Run("allows unknown fields with body limit", func(t *testing.T) {
		var body struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(
			http.MethodPost,
			"/users",
			strings.NewReader(`{"name":"alice","extra":true}`),
		)
		req.Header.Set("Content-Type", "application/json")

		err := DecodeJSON(req, &body, WithMaxBodyBytes(1024), AllowUnknownFields())
		if err != nil {
			t.Fatalf("DecodeJSON() error = %v", err)
		}
		if body.Name != "alice" {
			t.Fatalf("body.Name = %q, want alice", body.Name)
		}
	})

	t.Run("allows empty body", func(t *testing.T) {
		var body struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/users", http.NoBody)
		req.Header.Set("Content-Type", "application/json")

		err := DecodeJSON(req, &body, AllowEmptyBody())
		if err != nil {
			t.Fatalf("DecodeJSON() error = %v", err)
		}
		if body.Name != "" {
			t.Fatalf("body.Name = %q, want empty", body.Name)
		}
	})
}

func TestDecodeAndValidateQueryAllowsUnknownFieldsOnSuccess(t *testing.T) {
	var query struct {
		ID string `query:"id"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users?id=u_1&extra=yes", nil)

	err := DecodeAndValidateQuery(req, &query, func(value *struct {
		ID string `query:"id"`
	}) []Violation {
		if strings.TrimSpace(value.ID) == "" {
			return []Violation{{Field: "id", Code: "required", Message: "is required"}}
		}
		return nil
	}, AllowUnknownQueryFields())
	if err != nil {
		t.Fatalf("DecodeAndValidateQuery() error = %v", err)
	}
	if query.ID != "u_1" {
		t.Fatalf("query.ID = %q, want u_1", query.ID)
	}
}
