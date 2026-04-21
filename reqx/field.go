package reqx

import (
	"net/http"

	"github.com/kanata996/hah/internal/errx"
)

// 本文件负责 reqx 对外公开的违规模型、错误码和 invalid_request 错误包络。
//
// 这里承载的能力包括：
//   - 公开的顶层错误码常量
//   - request-side field error 默认 detail 规范化
//   - 422 invalid_request 错误的统一构造

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
func InvalidRequest(fieldErrors ...errx.FieldError) error {
	normalized := make([]errx.FieldError, len(fieldErrors))
	for i := range fieldErrors {
		normalized[i] = normalizeFieldError(fieldErrors[i])
	}
	return errx.NewHTTPError(
		http.StatusUnprocessableEntity,
		invalidRequestCode,
		invalidRequestDetail,
	).WithFieldErrors(normalized)
}

func newFieldError(field string, input errx.FieldErrorIn, code errx.FieldErrorCode, detail string) errx.FieldError {
	return errx.FieldError{
		Field:  field,
		In:     input,
		Code:   code,
		Detail: detail,
	}
}

func normalizeFieldError(fieldError errx.FieldError) errx.FieldError {
	if fieldError.Code == "" {
		fieldError.Code = errx.CodeInvalid
	}
	if fieldError.Detail != "" {
		return fieldError
	}
	fieldError.Detail = defaultFieldErrorDetail(fieldError.Code)
	return fieldError
}

func defaultFieldErrorDetail(code errx.FieldErrorCode) string {
	switch code {
	case errx.CodeRequired:
		return fieldErrorDetailRequired
	case errx.CodeUnknown:
		return fieldErrorDetailUnknownField
	case errx.CodeType:
		return fieldErrorDetailInvalidType
	case errx.CodeMultiple:
		return fieldErrorDetailMustNotRepeat
	default:
		return fieldErrorDetailInvalid
	}
}
