package reqx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type createUserRequest struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestDecodeJSONSuccess(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"alice","age":20}`))
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	if err := DecodeJSON(req, &got); err != nil {
		t.Fatalf("DecodeJSON() error = %v, want nil", err)
	}
	if got.Name != "alice" || got.Age != 20 {
		t.Fatalf("decoded request = %#v, want name=alice age=20", got)
	}
}

func TestDecodeJSONAcceptsPlusJSONContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"alice"}`))
	req.Header.Set("Content-Type", "application/merge-patch+json")

	var got createUserRequest
	if err := DecodeJSON(req, &got); err != nil {
		t.Fatalf("DecodeJSON() error = %v, want nil", err)
	}
	if got.Name != "alice" {
		t.Fatalf("decoded request = %#v, want name=alice", got)
	}
}

func TestDecodeJSONAcceptsMissingContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"alice"}`))

	var got createUserRequest
	if err := DecodeJSON(req, &got); err != nil {
		t.Fatalf("DecodeJSON() error = %v, want nil", err)
	}
	if got.Name != "alice" {
		t.Fatalf("decoded request = %#v, want name=alice", got)
	}
}

func TestDecodeAndValidateJSONReturnsDecodeError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":`))
	req.Header.Set("Content-Type", "application/json")

	called := false
	var got createUserRequest
	err := DecodeAndValidateJSON(req, &got, func(value *createUserRequest) []Violation {
		called = true
		return nil
	})

	assertProblem(t, err, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
	if called {
		t.Fatal("validator should not be called when JSON decode fails")
	}
}

func TestDecodeJSONRejectsUnsupportedContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"alice"}`))
	req.Header.Set("Content-Type", "text/plain")

	var got createUserRequest
	err := DecodeJSON(req, &got)

	assertProblem(t, err, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
}

func TestDecodeJSONRejectsEmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	err := DecodeJSON(req, &got)

	assertProblem(t, err, http.StatusBadRequest, "invalid_json", "request body must not be empty")
}

func TestDecodeJSONRejectsMalformedJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":`))
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	err := DecodeJSON(req, &got)

	assertProblem(t, err, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
}

func TestDecodeJSONRejectsTrailingJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"alice"} {"name":"bob"}`))
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	err := DecodeJSON(req, &got)

	assertProblem(t, err, http.StatusBadRequest, "invalid_json", "request body must contain a single JSON value")
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"alice","extra":true}`))
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	err := DecodeJSON(req, &got)

	assertProblem(
		t,
		err,
		http.StatusUnprocessableEntity,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "extra", Code: "unknown", Message: "unknown field"},
	)
}

func TestDecodeJSONRejectsFieldTypeMismatch(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":123}`))
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	err := DecodeJSON(req, &got)

	assertProblem(
		t,
		err,
		http.StatusUnprocessableEntity,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "name", Code: "type", Message: "must be string"},
	)
}

func TestDecodeJSONRejectsLargeBodies(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"alice"}`))
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	err := DecodeJSON(req, &got, WithMaxBodyBytes(8))

	assertProblem(t, err, http.StatusRequestEntityTooLarge, "request_too_large", "request body is too large")
}

func TestDecodeJSONRejectsNilRequest(t *testing.T) {
	var got createUserRequest

	err := DecodeJSON(nil, &got)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "hah/reqx: request must not be nil" {
		t.Fatalf("error = %q, want request must not be nil", got)
	}
}

func TestDecodeJSONRejectsNilDestination(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"alice"}`))

	err := DecodeJSON[createUserRequest](req, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "hah/reqx: destination must not be nil" {
		t.Fatalf("error = %q, want destination must not be nil", got)
	}
}

func TestDecodeJSONReturnsReadError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", errReaderCloser{err: simpleError("read failed")})
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	err := DecodeJSON(req, &got)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "read failed" {
		t.Fatalf("error = %q, want read failed", got)
	}
}

func TestDecodeJSONCanAllowUnknownFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"alice","extra":true}`))
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	if err := DecodeJSON(req, &got, AllowUnknownFields()); err != nil {
		t.Fatalf("DecodeJSON() error = %v, want nil", err)
	}
	if got.Name != "alice" {
		t.Fatalf("decoded request = %#v, want name=alice", got)
	}
}

func TestDecodeJSONCanAllowEmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	if err := DecodeJSON(req, &got, AllowEmptyBody()); err != nil {
		t.Fatalf("DecodeJSON() error = %v, want nil", err)
	}
	if got != (createUserRequest{}) {
		t.Fatalf("decoded request = %#v, want zero value", got)
	}
}

func TestDecodeAndValidateJSONRejectsViolations(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"   "}`))
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	err := DecodeAndValidateJSON(req, &got, func(value *createUserRequest) []Violation {
		if strings.TrimSpace(value.Name) == "" {
			return []Violation{{Field: "name", Code: "required", Message: "is required"}}
		}
		return nil
	})

	assertProblem(
		t,
		err,
		http.StatusUnprocessableEntity,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "name", Code: "required", Message: "is required"},
	)
}

func TestDecodeAndValidateJSONSuccess(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":" alice "}`))
	req.Header.Set("Content-Type", "application/json")

	var got createUserRequest
	if err := DecodeAndValidateJSON(req, &got, func(value *createUserRequest) []Violation {
		if strings.TrimSpace(value.Name) == "" {
			return []Violation{{Field: "name", Code: "required", Message: "is required"}}
		}
		return nil
	}); err != nil {
		t.Fatalf("DecodeAndValidateJSON() error = %v, want nil", err)
	}
	if strings.TrimSpace(got.Name) != "alice" {
		t.Fatalf("decoded request = %#v, want trimmed name alice", got)
	}
}
