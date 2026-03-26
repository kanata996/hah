package reqx

import "testing"

func TestValidateReturnsNilWhenValidatorIsNil(t *testing.T) {
	req := createUserRequest{Name: "alice"}

	if err := Validate(&req, nil); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidateReturnsNilWhenNoViolations(t *testing.T) {
	req := createUserRequest{Name: "alice"}

	err := Validate(&req, func(value *createUserRequest) []Violation {
		if value.Name != "alice" {
			return []Violation{{Field: "name", Code: "invalid", Message: "is invalid"}}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidateRejectsNilDestination(t *testing.T) {
	err := Validate[createUserRequest](nil, func(value *createUserRequest) []Violation {
		return nil
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "hah/reqx: destination must not be nil" {
		t.Fatalf("error = %q, want destination must not be nil", got)
	}
}

func TestValidateNormalizesDefaultViolationFields(t *testing.T) {
	req := createUserRequest{}

	err := Validate(&req, func(value *createUserRequest) []Violation {
		return []Violation{{Field: "name"}}
	})

	assertProblem(
		t,
		err,
		422,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "name", Code: "invalid", Message: "is invalid"},
	)
}

func TestInvalidRequestNormalizesViolations(t *testing.T) {
	err := InvalidRequest(
		Violation{Field: "name"},
		Violation{Field: "age", Code: "required"},
	)

	assertProblem(
		t,
		err,
		422,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "name", Code: "invalid", Message: "is invalid"},
		Violation{Field: "age", Code: "required", Message: "is required"},
	)
}

func TestInvalidRequestAllowsEmptyDetails(t *testing.T) {
	err := InvalidRequest()

	assertProblem(
		t,
		err,
		422,
		"invalid_request",
		"request contains invalid fields",
	)
}
