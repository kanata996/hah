package reqx

import (
	"net/http"

	"github.com/kanata996/hah/internal/req"
)

// 本文件负责请求级规则扩展点和通用 helper。
//
// 这里承载的能力包括：
//   - RequestValidator 扩展点及其执行适配
//   - InvalidRequest helper，用于生成统一的 invalid_request 错误
//   - RequireBody helper，用于在组合流程中声明 body-required 契约

// RequestValidator 允许 DTO 在 binding 之后、字段校验之前声明请求级规则。
type RequestValidator interface {
	ValidateRequest(r *http.Request) error
}

func applyRequestValidation(r *http.Request, target any) error {
	validator, ok := target.(RequestValidator)
	if !ok {
		return nil
	}
	return validator.ValidateRequest(r)
}

// InvalidRequest 生成统一的 invalid_request 错误包络。
func InvalidRequest(violations ...Violation) error {
	return invalidFieldsError(violations)
}

// RequireBody 按默认 binder 的 body 契约要求请求必须显式提交 body。
//
// 在当前实现里，实际读取到零字节 body 会被视为“没有 body”，与 Bind/BindBody
// 的 empty-body no-op 语义保持一致。
func RequireBody(r *http.Request) error {
	if r == nil {
		return errorsf("request must not be nil")
	}

	hasBody, err := req.HasBody(r)
	if err != nil {
		return err
	}
	if hasBody {
		return nil
	}

	return InvalidRequest(Violation{
		Field: "body",
		In:    ViolationInBody,
		Code:  ViolationCodeRequired,
	})
}
