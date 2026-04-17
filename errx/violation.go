package errx

type ViolationCode string
type ViolationIn string

const (
	CodeInvalid  ViolationCode = "invalid"
	CodeRequired ViolationCode = "required"
	CodeUnknown  ViolationCode = "unknown"
	CodeType     ViolationCode = "type"
	CodeMultiple ViolationCode = "multiple"
)

const (
	InBody   ViolationIn = "body"
	InQuery  ViolationIn = "query"
	InPath   ViolationIn = "path"
	InHeader ViolationIn = "header"
)

// Violation 描述单个公开请求违规。
type Violation struct {
	Field  string        `json:"field,omitempty"`
	In     ViolationIn   `json:"in,omitempty"`
	Code   ViolationCode `json:"code"`
	Detail string        `json:"detail"`
}
