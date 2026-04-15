package reqx

import (
	"net/http"

	"github.com/kanata996/hah/errx"
)

// 本文件负责 reqx 对外公开的违规模型、错误码和 invalid_request 错误包络。
//
// 这里承载的能力包括：
//   - 公开的顶层错误码常量
//   - request-side violation 默认 detail 规范化
//   - 422 invalid_request 错误的统一构造

const invalidRequestCode = "invalid_request"

const (
	violationDetailInvalid       = "is invalid"
	violationDetailRequired      = "is required"
	violationDetailUnknownField  = "unknown field"
	violationDetailInvalidType   = "has invalid type"
	violationDetailMustNotRepeat = "must not be repeated"
)

func invalidFieldsError(violations []errx.Violation) error {
	details := make([]errx.Violation, 0, len(violations))
	for _, violation := range violations {
		details = append(details, normalizeViolation(violation))
	}
	return errx.NewHTTPError(
		http.StatusUnprocessableEntity,
		invalidRequestCode,
		"request contains invalid fields",
	).WithViolations(details)
}

func newViolation(field, input, code, detail string) errx.Violation {
	return errx.Violation{
		Field:  field,
		In:     input,
		Code:   code,
		Detail: detail,
	}
}

func normalizeViolation(violation errx.Violation) errx.Violation {
	if violation.Code == "" {
		violation.Code = errx.ViolationCodeInvalid
	}
	if violation.Detail == "" {
		violation.Detail = violationDetailForCode(violation.Code)
	}
	return violation
}

func violationDetailForCode(code string) string {
	switch code {
	case errx.ViolationCodeRequired:
		return violationDetailRequired
	case errx.ViolationCodeUnknown:
		return violationDetailUnknownField
	case errx.ViolationCodeType:
		return violationDetailInvalidType
	case errx.ViolationCodeMultiple:
		return violationDetailMustNotRepeat
	default:
		return violationDetailInvalid
	}
}
