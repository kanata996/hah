package reqx

import (
	"net/http"

	"github.com/kanata996/hah/errx"
)

// 本文件负责 reqx 对外公开的违规模型、错误码和 invalid_request 错误包络。
//
// 这里承载的能力包括：
//   - 公开的顶层错误码常量
//   - 公开的 violation code / violation in 常量
//   - Violation 结构及默认 detail 规范化
//   - 422 invalid_request 错误的统一构造

const CodeInvalidRequest = "invalid_request"

const (
	ViolationCodeInvalid  = "invalid"
	ViolationCodeRequired = "required"
	ViolationCodeUnknown  = "unknown"
	ViolationCodeType     = "type"
	ViolationCodeMultiple = "multiple"
)

const (
	violationDetailInvalid       = "is invalid"
	violationDetailRequired      = "is required"
	violationDetailUnknownField  = "unknown field"
	violationDetailInvalidType   = "has invalid type"
	violationDetailMustNotRepeat = "must not be repeated"
)

const (
	ViolationInBody   = "body"
	ViolationInQuery  = "query"
	ViolationInPath   = "path"
	ViolationInHeader = "header"
)

// Violation 描述单个请求字段违规。
type Violation struct {
	Field  string `json:"field,omitempty"`
	In     string `json:"in,omitempty"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

func invalidFieldsError(violations []Violation) error {
	details := make([]any, 0, len(violations))
	for _, violation := range violations {
		details = append(details, normalizeViolation(violation))
	}
	return errx.NewHTTPError(
		http.StatusUnprocessableEntity,
		CodeInvalidRequest,
		"request contains invalid fields",
		details...,
	)
}

func newViolation(field, input, code, detail string) Violation {
	return Violation{
		Field:  field,
		In:     input,
		Code:   code,
		Detail: detail,
	}
}

func normalizeViolation(violation Violation) Violation {
	if violation.Code == "" {
		violation.Code = ViolationCodeInvalid
	}
	if violation.Detail == "" {
		violation.Detail = violationDetailForCode(violation.Code)
	}
	return violation
}

func violationDetailForCode(code string) string {
	switch code {
	case ViolationCodeRequired:
		return violationDetailRequired
	case ViolationCodeUnknown:
		return violationDetailUnknownField
	case ViolationCodeType:
		return violationDetailInvalidType
	case ViolationCodeMultiple:
		return violationDetailMustNotRepeat
	default:
		return violationDetailInvalid
	}
}
