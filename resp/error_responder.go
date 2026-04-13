package resp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/kanata996/hah/errx"
)

// ErrorResponder 协调 HTTP 错误标准化、请求日志注解、错误响应写回和独立 5xx 错误日志。
//
// 零值可用，且保持 resp 在纯 net/http 基线上：
//   - Logger 回退到 slog.Default()
//   - AsHTTPError 回退到 resp 的默认标准化
//   - ContextAttrs / AnnotateRequestLog 为 no-op
//   - RequestLogAttrs 回退到 resp 内置的低噪音 5xx 属性
type ErrorResponder struct {
	Logger             *slog.Logger
	AsHTTPError        func(error) *errx.HTTPError
	ContextAttrs       func(context.Context) []slog.Attr
	AnnotateRequestLog func(*http.Request, []slog.Attr)
	RequestLogAttrs    func(error, *errx.HTTPError) []slog.Attr
}

var defaultErrorResponder ErrorResponder

// NewErrorResponder 返回一个具有默认纯 net/http 行为的响应器。
// 调用方可修改返回值的字段来自定义策略。
func NewErrorResponder() *ErrorResponder {
	return &ErrorResponder{}
}

// Respond 把任意 error 收敛为稳定的 HTTP 错误响应并写回客户端。
//
// 流程：先标准化 error → 检查响应是否已开始 → 注解请求日志 → 记录 5xx 独立日志 → 写回错误响应。
// 若传入 nil error 则为 no-op。
func (r *ErrorResponder) Respond(w http.ResponseWriter, req *http.Request, err error) error {
	if err == nil {
		return nil
	}

	httpErr := r.httpError(err)

	var responseStartedErr *responseWriteError
	if errors.As(err, &responseStartedErr) && responseStartedErr != nil && responseStartedErr.responseStarted {
		r.logServerError(req, httpErr, err)
		return err
	}
	if responseAlreadyStarted(w) {
		r.logServerError(req, httpErr, err)
		return err
	}

	r.annotateRequestLog(req, r.requestLogAttrs(err, httpErr))
	r.logServerError(req, httpErr, err)
	writeErr := writeHTTPError(w, httpErr)
	r.logErrorResponseWriteFailure(req, httpErr, writeErr)
	return writeErr
}

func (r *ErrorResponder) logger() *slog.Logger {
	if r != nil && r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

func (r *ErrorResponder) httpError(err error) *errx.HTTPError {
	if r != nil && r.AsHTTPError != nil {
		if httpErr := r.AsHTTPError(err); httpErr != nil {
			return httpErr
		}
	}
	return asHTTPError(err)
}

func (r *ErrorResponder) contextAttrs(ctx context.Context) []slog.Attr {
	if r != nil && r.ContextAttrs != nil {
		return r.ContextAttrs(ctx)
	}
	return nil
}

func (r *ErrorResponder) annotateRequestLog(req *http.Request, attrs []slog.Attr) {
	if r == nil || r.AnnotateRequestLog == nil || req == nil || len(attrs) == 0 {
		return
	}
	r.AnnotateRequestLog(req, attrs)
}

func (r *ErrorResponder) requestLogAttrs(err error, httpErr *errx.HTTPError) []slog.Attr {
	if r != nil && r.RequestLogAttrs != nil {
		return r.RequestLogAttrs(err, httpErr)
	}
	return requestErrorLogAttrs(err, httpErr)
}
