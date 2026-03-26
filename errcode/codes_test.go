package errcode

import (
	"testing"

	"github.com/kanata996/hah/reqx"
)

func TestReqxDelegatedCodes(t *testing.T) {
	if RequestError != reqx.CodeRequestError {
		t.Fatalf("RequestError = %q, want %q", RequestError, reqx.CodeRequestError)
	}
	if InvalidJSON != reqx.CodeInvalidJSON {
		t.Fatalf("InvalidJSON = %q, want %q", InvalidJSON, reqx.CodeInvalidJSON)
	}
	if UnsupportedMediaType != reqx.CodeUnsupportedMediaType {
		t.Fatalf("UnsupportedMediaType = %q, want %q", UnsupportedMediaType, reqx.CodeUnsupportedMediaType)
	}
	if RequestTooLarge != reqx.CodeRequestTooLarge {
		t.Fatalf("RequestTooLarge = %q, want %q", RequestTooLarge, reqx.CodeRequestTooLarge)
	}
	if InvalidRequest != reqx.CodeInvalidRequest {
		t.Fatalf("InvalidRequest = %q, want %q", InvalidRequest, reqx.CodeInvalidRequest)
	}
}

func TestReqxDelegatedViolationCodes(t *testing.T) {
	if ViolationInvalid != reqx.ViolationCodeInvalid {
		t.Fatalf("ViolationInvalid = %q, want %q", ViolationInvalid, reqx.ViolationCodeInvalid)
	}
	if ViolationRequired != reqx.ViolationCodeRequired {
		t.Fatalf("ViolationRequired = %q, want %q", ViolationRequired, reqx.ViolationCodeRequired)
	}
	if ViolationUnknown != reqx.ViolationCodeUnknown {
		t.Fatalf("ViolationUnknown = %q, want %q", ViolationUnknown, reqx.ViolationCodeUnknown)
	}
	if ViolationType != reqx.ViolationCodeType {
		t.Fatalf("ViolationType = %q, want %q", ViolationType, reqx.ViolationCodeType)
	}
	if ViolationMultiple != reqx.ViolationCodeMultiple {
		t.Fatalf("ViolationMultiple = %q, want %q", ViolationMultiple, reqx.ViolationCodeMultiple)
	}
}

func TestGenericBusinessCodes(t *testing.T) {
	tests := map[string]string{
		"ResourceNotFound":    ResourceNotFound,
		"AlreadyExists":       AlreadyExists,
		"OperationNotAllowed": OperationNotAllowed,
		"StateConflict":       StateConflict,
	}

	want := map[string]string{
		"ResourceNotFound":    "resource_not_found",
		"AlreadyExists":       "already_exists",
		"OperationNotAllowed": "operation_not_allowed",
		"StateConflict":       "state_conflict",
	}

	for name, got := range tests {
		if got != want[name] {
			t.Fatalf("%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestExtendedViolationCodes(t *testing.T) {
	tests := map[string]string{
		"ViolationOneOf":     ViolationOneOf,
		"ViolationMin":       ViolationMin,
		"ViolationRange":     ViolationRange,
		"ViolationMinLength": ViolationMinLength,
		"ViolationMaxLength": ViolationMaxLength,
	}

	want := map[string]string{
		"ViolationOneOf":     "one_of",
		"ViolationMin":       "min",
		"ViolationRange":     "range",
		"ViolationMinLength": "min_length",
		"ViolationMaxLength": "max_length",
	}

	for name, got := range tests {
		if got != want[name] {
			t.Fatalf("%s = %q, want %q", name, got, want[name])
		}
	}
}
