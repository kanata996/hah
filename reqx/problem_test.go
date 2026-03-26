package reqx

import (
	"net/http"
	"testing"
)

func TestProblemNormalizesDefaults(t *testing.T) {
	problem := NewProblem(200, "", "")

	if got := problem.Status(); got != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", got, http.StatusBadRequest)
	}
	if got := problem.Code(); got != "request_error" {
		t.Fatalf("code = %q, want request_error", got)
	}
	if got := problem.Message(); got != "request error" {
		t.Fatalf("message = %q, want request error", got)
	}
	if got := problem.Error(); got != "request error" {
		t.Fatalf("error = %q, want request error", got)
	}
}

func TestProblemDetailsReturnsCopy(t *testing.T) {
	problem := NewProblem(http.StatusBadRequest, "invalid_request", "invalid request", map[string]any{"field": "name"})

	details := problem.Details()
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}

	details[0] = "mutated"

	again := problem.Details()
	if got, ok := again[0].(map[string]any); !ok || got["field"] != "name" {
		t.Fatalf("details should be copied, got %#v", again[0])
	}
}

func TestNilProblemAccessorsUseRequestDefaults(t *testing.T) {
	var problem *Problem

	if got := problem.Status(); got != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", got, http.StatusBadRequest)
	}
	if got := problem.Code(); got != "request_error" {
		t.Fatalf("code = %q, want request_error", got)
	}
	if got := problem.Message(); got != "request error" {
		t.Fatalf("message = %q, want request error", got)
	}
	if got := problem.Error(); got != "" {
		t.Fatalf("error = %q, want empty", got)
	}
	if got := problem.Details(); got != nil {
		t.Fatalf("details = %#v, want nil", got)
	}
}
