package resp

// 本文件负责“错误请求日志注解”，而不是“统一错误日志输出”。
//
// 定位：
//   - 这里服务于 ErrorResponder 的内部流程。
//   - 它只负责生成低噪音 request-log attrs 和独立 error log 的诊断 attrs。
//   - 具体 request log 集成由 ErrorResponder.AnnotateRequestLog hook 决定。
//
// 职责：
//   - 从 error / HTTPError 提取更适合排障的结构化字段。
//   - request log 只补低噪音诊断字段；独立 error log 保留完整诊断摘要。
//   - 在不泄露不可控内部对象的前提下，尽量保留原始错误文本、类型以及首层/尾部摘要。
//   - 仅在“错误响应自身写出失败”这类基础设施异常时，额外输出一条独立 error 日志。
//
// 要点：
//   - 普通 4xx / 5xx 不在这里额外打一条重复业务错误日志。
//   - 诊断字段优先围绕排障，而不是简单镜像对外响应 JSON。
//   - 诊断链优先从原始 cause 开始，避免 *HTTPError 包装层淹没真正根因。

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/kanata996/hah/errx"
)

const (
	maxLoggedErrorChainDepth  = 8
	maxLoggedErrorStringBytes = 1024
)

type diagnosticErrorSummary struct {
	message     string
	errorType   string
	rootMessage string
	rootType    string
}

// requestErrorLogAttrs 生成请求级错误日志字段。
// 4xx 仅保留外层请求日志；5xx 只补充低噪音、可聚合的诊断字段。
func requestErrorLogAttrs(err error, httpErr *errx.HTTPError) []slog.Attr {
	if err == nil || httpErr == nil {
		return nil
	}

	status := httpErr.Status()
	if status < http.StatusInternalServerError {
		return nil
	}

	attrs := make([]slog.Attr, 0, 6)
	attrs = append(attrs, slog.String("error.code", httpErr.Code()))
	if errors.Is(err, context.Canceled) {
		attrs = append(attrs, slog.Bool("error.canceled", true))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		attrs = append(attrs, slog.Bool("error.timeout", true))
	}

	return attrs
}

func diagnosticErrorLogAttrs(err error, httpErr *errx.HTTPError) []slog.Attr {
	return diagnosticErrorLogAttrsWithSource(err, httpErr, true)
}

func diagnosticErrorLogAttrsWithSource(err error, httpErr *errx.HTTPError, preferHTTPErrorCause bool) []slog.Attr {
	if err == nil || httpErr == nil {
		return nil
	}

	diagnosticSource := err
	if preferHTTPErrorCause {
		diagnosticSource = errorForDiagnostics(err, httpErr)
	}
	summary := summarizeDiagnosticError(diagnosticSource)
	attrs := make([]slog.Attr, 0, 7)
	attrs = append(attrs, slog.String("error.code", httpErr.Code()))
	if summary.message != "" {
		attrs = append(attrs, slog.String("error.message", summary.message))
	}
	if summary.errorType != "" {
		attrs = append(attrs, slog.String("error.type", summary.errorType))
	}

	if summary.rootMessage != "" {
		attrs = append(attrs, slog.String("error.root_message", summary.rootMessage))
	}
	if summary.rootType != "" {
		attrs = append(attrs, slog.String("error.root_type", summary.rootType))
	}
	if errors.Is(err, context.Canceled) {
		attrs = append(attrs, slog.Bool("error.canceled", true))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		attrs = append(attrs, slog.Bool("error.timeout", true))
	}

	return attrs
}

// errorForDiagnostics 返回用于日志诊断的起始 error。
// 如果 HTTPError 已包住原始 cause，则优先从 cause 开始构建单链诊断摘要，
// 避免把 *HTTPError 本身误当成主要错误类型。
func errorForDiagnostics(err error, httpErr *errx.HTTPError) error {
	if httpErr != nil {
		if cause := httpErr.Unwrap(); cause != nil {
			return cause
		}
	}
	return err
}

// logErrorResponseWriteFailure 只记录“错误响应自身写出失败”的异常。
// 这是基础设施级问题，不属于普通业务失败，因此需要单独打一条 error 日志。
func (r *ErrorResponder) logErrorResponseWriteFailure(req *http.Request, httpErr *errx.HTTPError, err error) {
	if err == nil || httpErr == nil {
		return
	}

	ctx := context.Background()
	if req != nil {
		ctx = req.Context()
	}

	attrs := []slog.Attr{
		slog.Int("http.response.status_code", httpErr.Status()),
	}
	// 错误响应写出失败时，诊断起点必须是 writeErr 本身，而不是业务 cause。
	attrs = append(attrs, diagnosticErrorLogAttrsWithSource(err, httpErr, false)...)
	attrs = append(attrs, requestMetadataAttrs(req)...)
	attrs = append(attrs, r.contextAttrs(ctx)...)

	var degraded *ErrorWriteDegraded
	if errors.As(err, &degraded) && degraded != nil {
		attrs = append(attrs,
			slog.Bool("resp.error_degraded", true),
			slog.Bool("resp.public_response_preserved", degraded.PreservedPublicResponse),
		)
	}

	r.logger().LogAttrs(ctx, slog.LevelError, "resp: failed to write error response", attrs...)
}

// logServerError 记录一次独立的 5xx 错误日志，便于在 access log 之外排查问题。
func (r *ErrorResponder) logServerError(req *http.Request, httpErr *errx.HTTPError, err error) {
	if err == nil || httpErr == nil || httpErr.Status() < http.StatusInternalServerError {
		return
	}

	r.logServerErrorAttrs(req, httpErr, diagnosticErrorLogAttrs(err, httpErr))
}

func (r *ErrorResponder) logServerErrorAttrs(req *http.Request, httpErr *errx.HTTPError, diagnosticAttrs []slog.Attr) {
	ctx := context.Background()
	if req != nil {
		ctx = req.Context()
	}

	attrs := []slog.Attr{
		slog.Int("http.response.status_code", httpErr.Status()),
	}
	attrs = append(attrs, diagnosticAttrs...)
	attrs = append(attrs, requestMetadataAttrs(req)...)
	attrs = append(attrs, r.contextAttrs(ctx)...)

	r.logger().LogAttrs(ctx, slog.LevelError, "resp: request failed with server error", attrs...)
}

// summarizeDiagnosticError 只沿单链 Unwrap 语义提取错误摘要。
// 这足以覆盖 Go 常见包装错误场景，同时避免在 resp 内部维护整套错误图分析逻辑。
func summarizeDiagnosticError(err error) diagnosticErrorSummary {
	var summary diagnosticErrorSummary
	current := err
	for depth := 0; current != nil && depth < maxLoggedErrorChainDepth; depth++ {
		message := safeErrorString(current)
		errType := errorTypeName(current)
		if message != "" {
			if summary.message == "" {
				summary.message = message
			}
			summary.rootMessage = message
		}
		if errType != "" {
			if summary.errorType == "" {
				summary.errorType = errType
			}
			summary.rootType = errType
		}

		current = unwrapError(current)
	}

	return summary
}

// unwrapError 只兼容单个 Unwrap() error。
// 如果 Unwrap() 本身不安全，则降级为停止下钻，不反向影响错误响应主流程。
func unwrapError(err error) (next error) {
	if err == nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			next = nil
		}
	}()
	return errors.Unwrap(err)
}

// errorTypeName 返回 error 的 Go 运行时类型名，便于按类型聚合和检索。
func errorTypeName(err error) string {
	if err == nil {
		return ""
	}
	return reflect.TypeOf(err).String()
}

// safeErrorString 读取错误文本，并对异常 Error() 实现做恢复。
func safeErrorString(err error) (message string) {
	if err == nil {
		return ""
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			message = "panic calling Error()"
			if errType := errorTypeName(err); errType != "" {
				message += " on " + errType
			}
			message += ": " + fmt.Sprint(recovered)
			message = limitErrorLogString(message)
		}
	}()

	return limitErrorLogString(err.Error())
}

// limitErrorLogString 对错误文本做长度限制，避免单条日志过大。
// 截断位置会对齐到 UTF-8 rune 边界，避免产生非法 UTF-8 序列。
func limitErrorLogString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= maxLoggedErrorStringBytes {
		return trimmed
	}
	// 从限制位置向前回退到最近的完整 rune 边界。
	cut := maxLoggedErrorStringBytes
	for cut > 0 && !utf8.RuneStart(trimmed[cut]) {
		cut--
	}
	return trimmed[:cut] + "...(truncated)"
}

func requestMetadataAttrs(req *http.Request) []slog.Attr {
	if req == nil {
		return nil
	}

	attrs := make([]slog.Attr, 0, 2)
	if method := strings.TrimSpace(req.Method); method != "" {
		attrs = append(attrs, slog.String("http.request.method", method))
	}
	if req.URL != nil {
		if path := strings.TrimSpace(req.URL.Path); path != "" {
			attrs = append(attrs, slog.String("url.path", path))
		}
	}

	return attrs
}
