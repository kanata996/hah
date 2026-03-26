package reqx

import (
	"fmt"
	"net/http"
)

// ValidateFunc validates a decoded request value.
type ValidateFunc[T any] func(*T) []Violation

// Violation describes a single request field validation problem.
type Violation struct {
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Validate applies a validation function and returns a standardized 422 request
// error when violations are present.
//
// Validate is a normalization helper, not a validation engine: the caller owns
// the actual validation logic via fn, and reqx only turns returned violations
// into a stable 422 invalid_request problem.
func Validate[T any](dst *T, fn ValidateFunc[T]) error {
	if fn == nil {
		return nil
	}
	if dst == nil {
		return fmt.Errorf("hah/reqx: destination must not be nil")
	}

	violations := fn(dst)
	if len(violations) == 0 {
		return nil
	}

	return InvalidRequest(violations...)
}

// InvalidRequest constructs a standardized 422 invalid_request problem from one
// or more violations. Each violation is normalized before being included in the
// returned problem details.
func InvalidRequest(violations ...Violation) error {
	return invalidFieldsError(violations)
}

func invalidFieldError(violation Violation) error {
	return InvalidRequest(violation)
}

func invalidFieldsError(violations []Violation) error {
	details := make([]any, 0, len(violations))
	for _, violation := range violations {
		details = append(details, normalizeViolation(violation))
	}

	return NewProblem(
		http.StatusUnprocessableEntity,
		CodeInvalidRequest,
		"request contains invalid fields",
		details...,
	)
}

func normalizeViolation(violation Violation) Violation {
	if violation.Code == "" {
		violation.Code = ViolationCodeInvalid
	}
	if violation.Message == "" {
		switch violation.Code {
		case ViolationCodeRequired:
			violation.Message = "is required"
		case ViolationCodeUnknown:
			violation.Message = "unknown field"
		case ViolationCodeType:
			violation.Message = "has invalid type"
		default:
			violation.Message = "is invalid"
		}
	}

	return violation
}
