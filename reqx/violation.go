package reqx

import (
	"net/http"

	"github.com/kanata996/hah/internal/errx"
)

// 本文件负责 reqx 对外公开的违规模型、错误码和 invalid_request 错误包络。
//
// 这里承载的能力包括：
//   - 公开的顶层错误码常量
//   - request-side violation 默认 detail 规范化
//   - 422 invalid_request 错误的统一构造

const invalidRequestCode = "invalid_request"
const invalidRequestDetail = "request contains invalid fields"

const (
	violationDetailInvalid       = "is invalid"
	violationDetailRequired      = "is required"
	violationDetailUnknownField  = "unknown field"
	violationDetailInvalidType   = "has invalid type"
	violationDetailMustNotRepeat = "must appear only once"
)

// InvalidRequest 生成统一的 invalid_request 错误包络。
func InvalidRequest(violations ...errx.Violation) error {
	normalized := make([]errx.Violation, len(violations))
	for i := range violations {
		normalized[i] = normalizeViolation(violations[i])
	}
	return errx.NewHTTPError(
		http.StatusUnprocessableEntity,
		invalidRequestCode,
		invalidRequestDetail,
	).WithViolations(normalized)
}

func newViolation(field string, input errx.ViolationIn, code errx.ViolationCode, detail string) errx.Violation {
	return errx.Violation{
		Field:  field,
		In:     input,
		Code:   code,
		Detail: detail,
	}
}

func normalizeViolation(violation errx.Violation) errx.Violation {
	if violation.Code == "" {
		violation.Code = errx.CodeInvalid
	}
	if violation.Detail != "" {
		return violation
	}
	violation.Detail = defaultViolationDetail(violation.Code)
	return violation
}

func defaultViolationDetail(code errx.ViolationCode) string {
	switch code {
	case errx.CodeRequired:
		return violationDetailRequired
	case errx.CodeUnknown:
		return violationDetailUnknownField
	case errx.CodeType:
		return violationDetailInvalidType
	case errx.CodeMultiple:
		return violationDetailMustNotRepeat
	default:
		return violationDetailInvalid
	}
}
