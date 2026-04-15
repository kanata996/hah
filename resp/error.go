package resp

// 本文件负责“统一错误响应写回”。
//
// 定位：
//   - 这里承载 error 语义到 HTTP 错误响应的主入口。
//   - 它关注的是把任意 error 收敛成稳定的 HTTP 错误响应并写回客户端。
//
// 职责：
//   - 统一错误 JSON 对象的编码与写回。
//   - 处理错误响应写出与 errors 序列化失败等边界。
//
// 要点：
//   - 对外响应契约稳定优先，不泄露内部原始错误对象。
//   - 普通 4xx / 5xx 只写统一错误响应，不额外输出重复业务错误日志。
//   - 若公开 errors 不可编码，则直接返回错误，由调用方决定如何处理。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		return errx.NewHTTPErrorWithCause(499, "client_closed_request", "Client Closed Request", err)
	case errors.Is(err, context.DeadlineExceeded):
		return errx.NewHTTPErrorWithCause(http.StatusGatewayTimeout, "timeout", "", err)
	}

	return errx.NewHTTPErrorWithCause(http.StatusInternalServerError, "", "", err)
}

// problemPayload 是最终写入响应体的公共错误字段。
// 这里不包含内部原始 error，避免把服务端细节泄露给客户端。
type problemPayload struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Code   string `json:"code"`
	Errors []any  `json:"errors,omitempty"`
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

	return writeErrorPayload(w, asHTTPError(err))
}

// writeErrorPayload 负责真正把错误对象编码并写到响应里。
// 如果公开 errors 序列化失败，则直接返回编码错误。
func writeErrorPayload(w http.ResponseWriter, httpErr *errx.HTTPError) error {
	body, err := marshalProblemPayload(httpErr)
	if err != nil {
		return err
	}

	return writeJSONBytesWithContentType(w, httpErr.Status(), problemJSONContentType, body)
}

// marshalProblemPayload 把公共错误字段编码为最终的 JSON 响应体。
// 该步骤只关心响应体结构，不处理日志、副作用或写出行为。
func marshalProblemPayload(httpErr *errx.HTTPError) (body []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			body = nil
			err = fmt.Errorf("resp: marshal problem payload panicked: %v", recovered)
		}
	}()

	return json.Marshal(problemPayload{
		Title:  httpErr.Title(),
		Status: httpErr.Status(),
		Detail: httpErr.Detail(),
		Code:   httpErr.Code(),
		Errors: httpErr.Errors(),
	})
}
