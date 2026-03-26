package reqx

// Public problem codes exposed by reqx. These constants are optional helpers
// for common built-in codes; callers may still pass their own stable
// machine-readable strings to NewProblem.
const (
	CodeRequestError         = "request_error"
	CodeInvalidJSON          = "invalid_json"
	CodeUnsupportedMediaType = "unsupported_media_type"
	CodeRequestTooLarge      = "request_too_large"
	CodeInvalidRequest       = "invalid_request"
)

// Public violation codes used by reqx validation details. These constants are
// optional helpers; callers may still use their own stable detail codes.
const (
	ViolationCodeInvalid  = "invalid"
	ViolationCodeRequired = "required"
	ViolationCodeUnknown  = "unknown"
	ViolationCodeType     = "type"
	ViolationCodeMultiple = "multiple"
)
