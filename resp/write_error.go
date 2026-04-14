package resp

// 本文件负责“统一错误响应写回”。
//
// 定位：
//   - 这里是 WriteError(...) 的主实现文件。
//   - 它关注的是把任意 error 收敛成稳定的 HTTP 错误响应并写回客户端。
//   - 与之配套的请求日志注解逻辑位于 error_log.go，避免响应写回和日志字段提取混在一起。
//
// 职责：
//   - 统一 error -> HTTPError 的收敛规则。
//   - 统一错误 JSON 对象的编码与写回。
//   - 处理错误响应写出、响应已开始写出、errors 序列化失败等边界。
//
// 要点：
//   - 对外响应契约稳定优先，不泄露内部原始错误对象。
//   - 普通 4xx / 5xx 只写统一错误响应，不额外输出重复业务错误日志。
//   - 若 errors 无法编码，则降级保留 title/status/detail/code，尽量保证客户端仍收到有效错误响应。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/kanata996/hah/errx"
)

// problemPayload 是最终写入响应体的公共错误字段。
// 这里不包含内部原始 error，避免把服务端细节泄露给客户端。
type problemPayload struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Code   string `json:"code"`
	Errors []any  `json:"errors,omitempty"`
}

// ErrorWriteDegraded 表示错误响应在写出时发生了“可恢复降级”。
// 典型场景是 errors 无法序列化，此时仍尽量保留 title/status/detail/code，
// 但会丢弃不可编码的 errors，并把降级信息返回给调用方。
type ErrorWriteDegraded struct {
	Cause                   error
	PreservedPublicResponse bool
}

// WriteError 是 HTTP 错误写回的统一入口。
//
// 职责分为三步：
//   - 先把任意 error 收敛为可稳定写回的 HTTPError；
//   - 再给当前 request log 补充低噪音 `error.*` 诊断字段；
//   - 最后按统一错误响应契约写回客户端。
//
// 约束：
//   - 对 HEAD 等请求沿用 net/http 默认语义：handler 仍正常写回，最终 body 是否发出由底层决定；
//   - 若能明确判断响应已经开始写出，则不再尝试二次改写响应；
//   - 普通 4xx / 5xx 不在这里额外输出一条重复业务错误日志。
func WriteError(w http.ResponseWriter, r *http.Request, err error) error {
	return defaultErrorResponder.Respond(w, r, err)
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

	for depth := 0; w != nil && depth < 8; depth++ {
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

// Error 返回降级错误的文本描述，便于上层记录或断言。
func (e *ErrorWriteDegraded) Error() string {
	if e == nil || e.Cause == nil {
		return "resp: error response errors were dropped"
	}
	if cause := safeErrorString(e.Cause); cause != "" {
		return "resp: error response errors were dropped: " + cause
	}
	return "resp: error response errors were dropped"
}

// Unwrap 返回降级的底层原因，便于 errors.Is / errors.As 继续判断。
func (e *ErrorWriteDegraded) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// writeErrorPayload 负责真正把错误对象编码并写到响应里。
// 如果 errors 序列化失败，会降级为只保留 title/status/detail/code 的响应，
// 尽量避免整次错误响应完全失败。
func writeErrorPayload(w http.ResponseWriter, httpErr *errx.HTTPError) error {
	body, err := marshalProblemPayload(httpErr)
	if err != nil {
		fallbackBody, _ := json.Marshal(problemPayloadFromHTTPError(httpErr, false))
		if writeErr := writeJSONBytesWithContentType(w, httpErr.Status(), problemJSONContentType, fallbackBody); writeErr != nil {
			return errors.Join(&ErrorWriteDegraded{Cause: err}, writeErr)
		}
		return &ErrorWriteDegraded{
			Cause:                   err,
			PreservedPublicResponse: true,
		}
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
