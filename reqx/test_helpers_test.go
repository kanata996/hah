package reqx

import (
	"errors"
	"testing"
)

func assertProblem(t *testing.T, err error, status int, code, message string, wantDetails ...Violation) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var problem *Problem
	if !errors.As(err, &problem) || problem == nil {
		t.Fatalf("error = %T(%v), want *Problem", err, err)
	}
	if got := problem.Status(); got != status {
		t.Fatalf("problem.Status() = %d, want %d", got, status)
	}
	if got := problem.Code(); got != code {
		t.Fatalf("problem.Code() = %q, want %q", got, code)
	}
	if got := problem.Message(); got != message {
		t.Fatalf("problem.Message() = %q, want %q", got, message)
	}

	gotDetails := problem.Details()
	if len(gotDetails) != len(wantDetails) {
		t.Fatalf("len(problem.Details()) = %d, want %d", len(gotDetails), len(wantDetails))
	}

	for i, want := range wantDetails {
		got, ok := gotDetails[i].(Violation)
		if !ok {
			t.Fatalf("problem.Details()[%d] = %T(%#v), want Violation", i, gotDetails[i], gotDetails[i])
		}
		if got != want {
			t.Fatalf("problem.Details()[%d] = %#v, want %#v", i, got, want)
		}
	}

}
