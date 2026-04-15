package errx

const (
	ViolationCodeInvalid  = "invalid"
	ViolationCodeRequired = "required"
	ViolationCodeUnknown  = "unknown"
	ViolationCodeType     = "type"
	ViolationCodeMultiple = "multiple"
)

const (
	ViolationInBody   = "body"
	ViolationInQuery  = "query"
	ViolationInPath   = "path"
	ViolationInHeader = "header"
)

// Violation 描述单个公开请求违规。
type Violation struct {
	Field  string `json:"field,omitempty"`
	In     string `json:"in,omitempty"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}
