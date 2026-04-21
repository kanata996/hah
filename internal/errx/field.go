package errx

type FieldErrorCode string
type FieldErrorIn string

const (
	CodeInvalid  FieldErrorCode = "invalid"
	CodeRequired FieldErrorCode = "required"
	CodeUnknown  FieldErrorCode = "unknown"
	CodeType     FieldErrorCode = "type"
	CodeMultiple FieldErrorCode = "multiple"
)

const (
	InBody   FieldErrorIn = "body"
	InQuery  FieldErrorIn = "query"
	InPath   FieldErrorIn = "path"
	InHeader FieldErrorIn = "header"
)

// FieldError 描述单个公开字段错误。
type FieldError struct {
	Field  string         `json:"field,omitempty"`
	In     FieldErrorIn   `json:"in,omitempty"`
	Code   FieldErrorCode `json:"code"`
	Detail string         `json:"detail"`
}
