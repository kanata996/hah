package resp

// 本文件负责“统一错误响应写回”。
//
// 定位：
//   - 这里承载 error 语义到 HTTP 错误响应的主入口。
//   - 它关注的是把任意 error 收敛成稳定的 HTTP 错误响应并写回客户端。
//
// 职责：
//   - 统一错误 JSON 对象的编码与写回。
//   - 处理错误响应写出等边界。
//
// 要点：
//   - 对外响应契约稳定优先，不泄露内部原始错误对象。
//   - 普通 4xx / 5xx 只写统一错误响应，不额外输出重复业务错误日志。
//   - violation 明细固定为公开结构，不再接受任意 payload。

import (
	"context"
	"errors"
	"net/http"

	"github.com/kanata996/hah/errx"
)

// problemPayload 是最终写入响应体的公共错误字段。
// 这里不包含内部原始 error，避免把服务端细节泄露给客户端。
type problemPayload struct {
	Title  string           `json:"title"`
	Status int              `json:"status"`
	Detail string           `json:"detail,omitempty"`
	Code   string           `json:"code"`
	Errors []errx.Violation `json:"errors,omitempty"`
}

var internalProblemBody = []byte("{\"title\":\"Internal Server Error\",\"status\":500,\"code\":\"internal_error\"}\n")

// problemBodyEncoder 默认走标准 JSON 编码。
// 保持为变量仅用于测试编码失败时的回退契约，不改变公开 API。
var problemBodyEncoder = encodeJSON

// WriteError 是 HTTP 错误写回的统一入口。
//
// 职责分为两步：
//   - 先把任意 error 收敛为可稳定写回的 HTTPError；
//   - 再按统一错误响应契约写回客户端。
//
// 约束：
//   - 对 HEAD 等请求沿用 net/http 默认语义：handler 仍正常写回，最终 body 是否发出由底层决定；
//   - 调用方应在开始写出响应前调用它；resp 不额外探测外部 ResponseWriter 的内部状态；
//   - 日志策略留给调用方决定；WriteError(...) 本身不输出独立错误日志。
func WriteError(w http.ResponseWriter, err error) error {
	if err == nil {
		return nil
	}

	var responseWriteErr *responseWriteError
	if errors.As(err, &responseWriteErr) {
		return err
	}

	if w == nil {
		return errNilResponseWriter
	}

	payload := normalizeProblemPayload(err)
	status, body := encodeProblemBody(payload, problemBodyEncoder)
	return writePreparedJSONBytes(w, status, problemJSONContentType, body)
}

// encodeProblemBody 负责把公开错误 payload 编码成响应体。
// 若主 payload 意外编码失败，则回退到最小 500 problem JSON，避免对外暴露内部细节。
func encodeProblemBody(payload problemPayload, encode func(any) ([]byte, error)) (status int, body []byte) {
	body, err := encode(payload)
	if err == nil {
		return payload.Status, body
	}
	return http.StatusInternalServerError, internalProblemBody
}

func normalizeProblemPayload(err error) problemPayload {
	var httpErr *errx.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		return problemPayload{
			Title:  httpErr.Title(),
			Status: httpErr.Status(),
			Detail: httpErr.Detail(),
			Code:   httpErr.Code(),
			Errors: httpErr.Errors(),
		}
	}

	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, context.Canceled):
		status = 499
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	}

	synthetic := errx.NewHTTPError(status, "", "")
	return problemPayload{
		Title:  synthetic.Title(),
		Status: synthetic.Status(),
		Code:   synthetic.Code(),
	}
}
