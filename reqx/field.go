package reqx

import (
	"net/http"

	"github.com/kanata996/hah/internal/errx"
)

// 本文件负责 reqx 对外公开的 field error 模型、错误码和 invalid_request 错误包络。
//
// 这里承载的能力包括：
//   - 公开的顶层错误码常量
//   - 公开的 field error 类型和输入位置常量
//   - request-side field error 默认 detail 规范化
//   - 422 invalid_request 错误的统一构造

type (
	// FieldErrorCode 描述公开字段错误码。
	FieldErrorCode = errx.FieldErrorCode
	// FieldErrorIn 描述公开字段错误来源。
	FieldErrorIn = errx.FieldErrorIn
	// FieldError 描述单个公开字段错误。
	FieldError = errx.FieldError
)

const (
	CodeInvalid  = errx.CodeInvalid
	CodeRequired = errx.CodeRequired
	CodeUnknown  = errx.CodeUnknown
	CodeType     = errx.CodeType
	CodeMultiple = errx.CodeMultiple
)

const (
	InBody   = errx.InBody
	InQuery  = errx.InQuery
	InPath   = errx.InPath
	InHeader = errx.InHeader
)

const invalidRequestCode = "invalid_request"
const invalidRequestDetail = "request contains invalid fields"

const (
	fieldErrorDetailInvalid       = "is invalid"
	fieldErrorDetailRequired      = "is required"
	fieldErrorDetailUnknownField  = "unknown field"
	fieldErrorDetailInvalidType   = "has invalid type"
	fieldErrorDetailMustNotRepeat = "must appear only once"
)

// InvalidRequest 生成统一的 invalid_request 错误包络。
func InvalidRequest(fieldErrors ...FieldError) error {
	normalized := make([]FieldError, len(fieldErrors))
	for i := range fieldErrors {
		normalized[i] = normalizeFieldError(fieldErrors[i])
	}
	return errx.NewHTTPError(
		http.StatusUnprocessableEntity,
		invalidRequestCode,
		invalidRequestDetail,
	).WithFieldErrors(normalized)
}

func newFieldError(field string, input FieldErrorIn, code FieldErrorCode, detail string) FieldError {
	return FieldError{
		Field:  field,
		In:     input,
		Code:   code,
		Detail: detail,
	}
}

func normalizeFieldError(fieldError FieldError) FieldError {
	if fieldError.Code == "" {
		fieldError.Code = CodeInvalid
	}
	if fieldError.Detail != "" {
		return fieldError
	}
	fieldError.Detail = defaultFieldErrorDetail(fieldError.Code)
	return fieldError
}

func defaultFieldErrorDetail(code FieldErrorCode) string {
	switch code {
	case CodeRequired:
		return fieldErrorDetailRequired
	case CodeUnknown:
		return fieldErrorDetailUnknownField
	case CodeType:
		return fieldErrorDetailInvalidType
	case CodeMultiple:
		return fieldErrorDetailMustNotRepeat
	default:
		return fieldErrorDetailInvalid
	}
}
