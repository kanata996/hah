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
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kanata996/hah/errx"
)

// asHTTPError 把任意 error 适配为 HTTPError。
// 这是错误响应语义的收敛点，负责得到最终 status/code/detail/errors。
//
// 适配顺序：
//   - 已经是 HTTPError，直接返回；
//   - context.Canceled / context.DeadlineExceeded 走固定 HTTP 语义；
//   - 其余错误统一视为内部错误。
func asHTTPError(err error) *errx.HTTPError {
	if err == nil {
		return nil
	}

	var httpErr *errx.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		return httpErr
	}

	switch {
	case errors.Is(err, context.Canceled):
		return errx.NewHTTPErrorWithCause(499, "", "", err)
	case errors.Is(err, context.DeadlineExceeded):
		return errx.NewHTTPErrorWithCause(http.StatusGatewayTimeout, "", "", err)
	}

	return errx.NewHTTPErrorWithCause(http.StatusInternalServerError, "", "", err)
}

// problemPayload 是最终写入响应体的公共错误字段。
// 这里不包含内部原始 error，避免把服务端细节泄露给客户端。
type problemPayload struct {
	Title  string           `json:"title"`
	Status int              `json:"status"`
	Detail string           `json:"detail"`
	Code   string           `json:"code"`
	Errors []errx.Violation `json:"errors,omitempty"`
}

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

	httpErr := asHTTPError(err)
	body, err := json.Marshal(problemPayload{
		Title:  httpErr.Title(),
		Status: httpErr.Status(),
		Detail: httpErr.Detail(),
		Code:   httpErr.Code(),
		Errors: httpErr.Errors(),
	})
	if err != nil {
		return err
	}

	return writePreparedJSONBytes(w, httpErr.Status(), problemJSONContentType, body)
}
