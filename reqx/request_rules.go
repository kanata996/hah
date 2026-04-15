package reqx

import (
	"net/http"
)

// 本文件负责请求输入辅助错误与 body-required helper。
//
// 这里承载的能力包括：
//   - InvalidRequest helper，用于生成统一的 invalid_request 错误
//   - RequireBody helper，用于显式声明 body-required 契约

// InvalidRequest 生成统一的 invalid_request 错误包络。
func InvalidRequest(violations ...Violation) error {
	return invalidFieldsError(violations)
}

// RequireBody 按 body 绑定契约要求请求必须显式提交 body。
//
// 在当前实现里，实际读取到零字节 body 会被视为“没有 body”，与 BindBody 的
// empty-body no-op 语义保持一致。它和 BindBody 共享同一个非破坏性 body
// 探测，因此可按调用方需要在绑定前后调用。
func RequireBody(r *http.Request) error {
	if r == nil {
		return usageErrorf("request must not be nil")
	}

	hasBody, err := hasRequestBody(r)
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
