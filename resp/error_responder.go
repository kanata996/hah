package resp

import (
	"errors"
	"net/http"

	"github.com/kanata996/hah/errx"
)

// ErrorResponder 协调 HTTP 错误标准化与错误响应写回。
//
// 零值可用，且保持 resp 在纯 net/http 基线上：
//   - AsHTTPError 回退到 resp 的默认标准化
type ErrorResponder struct {
	AsHTTPError func(error) *errx.HTTPError
}

var defaultErrorResponder ErrorResponder

// NewErrorResponder 返回一个具有默认纯 net/http 行为的响应器。
// 调用方可修改返回值的字段来自定义策略。
func NewErrorResponder() *ErrorResponder {
	return &ErrorResponder{}
}

// AsHTTPError 把任意 error 适配为公开稳定的 HTTPError。
func AsHTTPError(err error) *errx.HTTPError {
	return asHTTPError(err)
}

// Respond 把任意 error 收敛为稳定的 HTTP 错误响应并写回客户端。
//
// 流程：先标准化 error → 检查响应是否已开始 → 写回错误响应。
// 若传入 nil error 则为 no-op。
func (r *ErrorResponder) Respond(w http.ResponseWriter, err error) error {
	if err == nil {
		return nil
	}

	httpErr := r.httpError(err)

	var responseStartedErr *responseWriteError
	if errors.As(err, &responseStartedErr) && responseStartedErr != nil && responseStartedErr.responseStarted {
		return err
	}
	if responseAlreadyStarted(w) {
		return err
	}

	return writeErrorPayload(w, httpErr)
}

func (r *ErrorResponder) httpError(err error) *errx.HTTPError {
	if r != nil && r.AsHTTPError != nil {
		if httpErr := r.AsHTTPError(err); httpErr != nil {
			return httpErr
		}
	}
	return asHTTPError(err)
}
