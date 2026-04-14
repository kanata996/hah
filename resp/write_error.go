package resp

// 本文件负责“统一错误响应写回”。
//
// 定位：
//   - 这里是 WriteError(...) 的主实现文件。
//   - 它关注的是把任意 error 收敛成稳定的 HTTP 错误响应并写回客户端。
//
// 职责：
//   - 统一错误 JSON 对象的编码与写回。
//   - 处理错误响应写出、响应已开始写出、errors 序列化失败等边界。
//
// 要点：
//   - 对外响应契约稳定优先，不泄露内部原始错误对象。
//   - 普通 4xx / 5xx 只写统一错误响应，不额外输出重复业务错误日志。
//   - 若公开 errors 不可编码，则直接返回错误，由调用方决定如何处理。

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kanata996/hah/errx"
)

const maxResponseWriterUnwrapDepth = 64

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
// 职责分为三步：
//   - 先把任意 error 收敛为可稳定写回的 HTTPError；
//   - 再检查响应是否已经开始写出；
//   - 最后按统一错误响应契约写回客户端。
//
// 约束：
//   - 对 HEAD 等请求沿用 net/http 默认语义：handler 仍正常写回，最终 body 是否发出由底层决定；
//   - 若能明确判断响应已经开始写出，则不再尝试二次改写响应；
//   - 日志策略留给调用方决定；WriteError(...) 本身不输出独立错误日志。
func WriteError(w http.ResponseWriter, err error) error {
	return defaultErrorResponder.Respond(w, err)
}

// responseAlreadyStarted 仅在 writer 显式暴露响应状态时判断是否已经开始写出。
// 对于通用 http.ResponseWriter，标准接口本身无法可靠探测“是否已发出 header/body”。
// 这里采用最小判断：若可读到 status/bytes 且任一非零，则视为已开始。
func responseAlreadyStarted(w http.ResponseWriter) bool {
	type responseStateWriter interface {
		Status() int
		BytesWritten() int
	}
	type responseUnwrapper interface {
		Unwrap() http.ResponseWriter
	}

	for depth := 0; w != nil && depth < maxResponseWriterUnwrapDepth; depth++ {
		if state, ok := w.(responseStateWriter); ok && (state.Status() != 0 || state.BytesWritten() > 0) {
			return true
		}

		unwrapper, ok := w.(responseUnwrapper)
		if !ok {
			break
		}
		w = unwrapper.Unwrap()
	}

	return false
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

	return json.Marshal(problemPayloadFromHTTPError(httpErr, true))
}

func problemPayloadFromHTTPError(httpErr *errx.HTTPError, includeErrors bool) problemPayload {
	payload := problemPayload{
		Title:  httpErr.Title(),
		Status: httpErr.Status(),
		Detail: httpErr.Detail(),
		Code:   httpErr.Code(),
	}
	if includeErrors {
		payload.Errors = httpErr.Errors()
	}

	return payload
}
