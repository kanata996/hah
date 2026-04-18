package reqx

import (
	"errors"
	"net/http"

	"github.com/kanata996/hah/errx"
)

// 本文件负责请求输入辅助错误与 body-required helper。
//
// 这里承载的能力包括：
//   - InvalidRequest helper，用于生成统一的 invalid_request 错误
//   - RequireBody helper，用于显式声明 body-required 契约

// InvalidRequest 生成统一的 invalid_request 错误包络。
func InvalidRequest(violations ...errx.Violation) error {
	return invalidFieldsError(violations)
}

// RequireBody 按 body 绑定契约要求请求必须显式提交 body。
//
// 在当前实现里，它与 BindBody 共享同一个 request 上已经读取过的 body 字节，
// 因此可按调用方需要在同一个 request 上前后组合使用。
func RequireBody(r *http.Request) error {
	if r == nil {
		return usageErrorf("request must not be nil")
	}

	body, err := requestBodyBytes(r)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			return requestTooLargeError()
		}
		return err
	}
	if len(body) > 0 {
		return nil
	}

	return InvalidRequest(errx.Violation{
		Field: "body",
		In:    errx.InBody,
		Code:  errx.CodeRequired,
	})
}
