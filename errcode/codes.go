// Package errcode provides common stable public error codes for hah APIs.
//
// Reqx-owned request/validation codes are re-exported from reqx so reqx can
// remain self-contained and independently reusable.
package errcode

import "github.com/kanata996/hah/reqx"

// Common top-level public error codes.
const (
	ClientError      = "client_error"
	InternalError    = "internal_error"
	BadRequest       = "bad_request"
	Unauthorized     = "unauthorized"
	Forbidden        = "forbidden"
	NotFound         = "not_found"
	Conflict         = "conflict"
	Gone             = "gone"
	RateLimited      = "rate_limited"
	RouteNotFound    = "route_not_found"
	MethodNotAllowed = "method_not_allowed"
)

// Common cross-domain business error codes.
const (
	ResourceNotFound    = "resource_not_found"
	AlreadyExists       = "already_exists"
	OperationNotAllowed = "operation_not_allowed"
	StateConflict       = "state_conflict"
)

// Reqx-owned request/validation codes.
const (
	RequestError         = reqx.CodeRequestError
	InvalidJSON          = reqx.CodeInvalidJSON
	UnsupportedMediaType = reqx.CodeUnsupportedMediaType
	RequestTooLarge      = reqx.CodeRequestTooLarge
	InvalidRequest       = reqx.CodeInvalidRequest
)

// Common validation/detail codes.
const (
	ViolationInvalid   = reqx.ViolationCodeInvalid
	ViolationRequired  = reqx.ViolationCodeRequired
	ViolationUnknown   = reqx.ViolationCodeUnknown
	ViolationType      = reqx.ViolationCodeType
	ViolationMultiple  = reqx.ViolationCodeMultiple
	ViolationOneOf     = "one_of"
	ViolationMin       = "min"
	ViolationRange     = "range"
	ViolationMinLength = "min_length"
	ViolationMaxLength = "max_length"
)
