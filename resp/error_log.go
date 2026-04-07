package resp

// 本文件负责“独立错误日志输出”。
//
// 定位：
//   - 这里服务于 WriteError(...) 的内部流程。
//   - 它不依赖任何 router、request logger 或 tracing 中间件。
//   - 它只在需要时通过 slog.Default() 输出一条独立错误日志。
//
// 职责：
//   - 从 error / HTTPError 提取更适合排障的结构化字段。
//   - 在不泄露不可控内部对象的前提下，尽量保留原始错误文本、类型以及首层/根因摘要。
//   - 在错误响应自身写出失败时，额外输出基础设施级异常日志。
//
// 要点：
//   - 普通 4xx 不额外输出独立错误日志。
//   - 5xx 只输出一条独立错误日志，不再尝试补 access log 字段。
//   - 诊断链优先从原始 cause 开始，避免 *HTTPError 包装层淹没真正根因。

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/kanata996/hah/errx"
)

const (
	maxLoggedErrorChainDepth  = 8
	maxLoggedErrorStringBytes = 1024
)

type errorChainInfo struct {
	message     string
	errorType   string
	rootMessage string
	rootType    string
}

func diagnosticErrorLogAttrs(err error, httpErr *errx.HTTPError) []slog.Attr {
	if err == nil || httpErr == nil {
		return nil
	}

	return diagnosticErrorLogAttrsForError(errorForDiagnostics(err, httpErr), err, httpErr.Code())
}

func diagnosticErrorLogAttrsForError(diagnosticErr, flagErr error, code string) []slog.Attr {
	if diagnosticErr == nil {
		return nil
	}

	chain := buildErrorChainInfo(diagnosticErr)
	attrs := make([]slog.Attr, 0, 7)
	if strings.TrimSpace(code) != "" {
		attrs = append(attrs, slog.String("error.code", code))
	}
	if chain.message != "" {
		attrs = append(attrs, slog.String("error.message", chain.message))
	}
	if chain.errorType != "" {
		attrs = append(attrs, slog.String("error.type", chain.errorType))
	}

	if chain.rootMessage != "" {
		attrs = append(attrs, slog.String("error.root_message", chain.rootMessage))
	}
	if chain.rootType != "" {
		attrs = append(attrs, slog.String("error.root_type", chain.rootType))
	}
	if errors.Is(flagErr, context.Canceled) {
		attrs = append(attrs, slog.Bool("error.canceled", true))
	}
	if errors.Is(flagErr, context.DeadlineExceeded) {
		attrs = append(attrs, slog.Bool("error.timeout", true))
	}

	return attrs
}

// errorForDiagnostics 返回用于日志诊断的起始 error。
// 如果 HTTPError 已包住原始 cause，则优先从 cause 开始展开错误链，
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
func logErrorResponseWriteFailure(r *http.Request, httpErr *errx.HTTPError, err error) {
	if err == nil || httpErr == nil {
		return
	}

	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}

	attrs := []slog.Attr{
		slog.Int("http.response.status_code", httpErr.Status()),
	}
	attrs = append(attrs, diagnosticErrorLogAttrsForError(err, err, httpErr.Code())...)
	attrs = append(attrs, requestMetadataAttrs(r)...)

	var degraded *ErrorWriteDegraded
	if errors.As(err, &degraded) && degraded != nil {
		attrs = append(attrs,
			slog.Bool("resp.error_degraded", true),
			slog.Bool("resp.public_response_preserved", degraded.PreservedPublicResponse),
		)
	}

	slog.Default().LogAttrs(ctx, slog.LevelError, "resp: failed to write error response", attrs...)
}

// logServerError 记录一次独立的 5xx 错误日志，便于在 access log 之外排查问题。
func logServerError(r *http.Request, httpErr *errx.HTTPError, err error) {
	if err == nil || httpErr == nil || httpErr.Status() < http.StatusInternalServerError {
		return
	}

	logServerErrorAttrs(r, httpErr, diagnosticErrorLogAttrs(err, httpErr))
}

func logServerErrorAttrs(r *http.Request, httpErr *errx.HTTPError, diagnosticAttrs []slog.Attr) {
	if httpErr == nil || httpErr.Status() < http.StatusInternalServerError {
		return
	}

	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}

	attrs := []slog.Attr{
		slog.Int("http.response.status_code", httpErr.Status()),
	}
	attrs = append(attrs, diagnosticAttrs...)
	attrs = append(attrs, requestMetadataAttrs(r)...)

	slog.Default().LogAttrs(ctx, slog.LevelError, "resp: request failed with server error", attrs...)
}

// buildErrorChainInfo 把错误链整理成适合日志输出的摘要结构。
// 它只保留请求日志会消费的首层/根因 message 与 type 摘要。
func buildErrorChainInfo(err error) errorChainInfo {
	chain := flattenErrorChain(err, maxLoggedErrorChainDepth)
	if len(chain) == 0 {
		return errorChainInfo{}
	}

	var info errorChainInfo
	for _, item := range chain {
		// 某些异常 error 实现（例如 typed-nil 或不安全的 Error()）可能在这里 panic。
		// 日志诊断路径不能反向把主错误处理打崩，因此统一做恢复并降级为说明性文本。
		message := safeErrorString(item)
		errType := errorTypeName(item)
		if message != "" {
			if info.message == "" {
				info.message = message
			}
			info.rootMessage = message
		}
		if errType != "" {
			if info.errorType == "" {
				info.errorType = errType
			}
			info.rootType = errType
		}
	}

	return info
}

// flattenErrorChain 按 Unwrap 语义展开错误链，并限制深度、防止循环。
// 这样可以兼容标准单链 unwrap，也兼容 errors.Join 形成的多分支链路。
func flattenErrorChain(err error, limit int) []error {
	if err == nil || limit <= 0 {
		return nil
	}

	initialCap := limit
	if initialCap > 4 {
		initialCap = 4
	}
	seen := make(map[error]struct{}, initialCap)
	chain := make([]error, 0, initialCap)
	stack := make([]error, 1, initialCap)
	stack[0] = err

	for len(stack) > 0 && len(chain) < limit {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if current == nil {
			continue
		}
		// error 接口底层可能承载“不可比较类型”（例如包含 slice/map 的 struct）。
		// 这类值一旦作为 map key 会直接 panic，所以只能在可比较时参与去重；
		// 对不可比较 error 则退化为仅依赖深度上限来防止无限展开。
		if isComparableError(current) {
			if _, ok := seen[current]; ok {
				continue
			}
			seen[current] = struct{}{}
		}
		chain = append(chain, current)

		unwrapped := unwrapErrors(current)
		remaining := limit - len(chain) - len(stack)
		if remaining <= 0 {
			continue
		}

		// 预算不足时优先保留更靠前的分支，避免 errors.Join(...) 在截断后
		// 偏向后面的子错误；同时也确保 stack 不会因宽 join 无界增长。
		if len(unwrapped) > remaining {
			unwrapped = unwrapped[:remaining]
		}
		for i := len(unwrapped) - 1; i >= 0; i-- {
			stack = append(stack, unwrapped[i])
		}
	}

	return chain
}

// isComparableError 判断当前 error 是否可安全作为 map key 使用。
func isComparableError(err error) bool {
	if err == nil {
		return false
	}
	errType := reflect.TypeOf(err)
	return errType != nil && errType.Comparable()
}

// unwrapErrors 统一兼容单个 Unwrap() error 和多个 Unwrap() []error 的错误类型。
func unwrapErrors(err error) (errs []error) {
	type multiUnwrapper interface {
		Unwrap() []error
	}
	defer func() {
		if recover() != nil {
			// 某些第三方 error 的 Unwrap() 可能在 nil receiver 或坏状态下 panic。
			// 日志注解只做诊断，不应该因为展开失败而影响主流程，因此这里直接降级停止下钻。
			errs = nil
		}
	}()

	switch current := err.(type) {
	case multiUnwrapper:
		return current.Unwrap()
	default:
		if next := errors.Unwrap(err); next != nil {
			return []error{next}
		}
		return nil
	}
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
func limitErrorLogString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= maxLoggedErrorStringBytes {
		return trimmed
	}
	return trimmed[:maxLoggedErrorStringBytes] + "...(truncated)"
}

func requestMetadataAttrs(r *http.Request) []slog.Attr {
	if r == nil {
		return nil
	}

	attrs := make([]slog.Attr, 0, 2)
	if method := strings.TrimSpace(r.Method); method != "" {
		attrs = append(attrs, slog.String("http.request.method", method))
	}
	if r.URL != nil {
		if path := strings.TrimSpace(r.URL.Path); path != "" {
			attrs = append(attrs, slog.String("url.path", path))
		}
	}

	return attrs
}
