# 错误日志当前行为

本文只说明 `resp.WriteError(...)` 的当前独立日志行为。

相关源码：

- `resp/write_error.go`
- `resp/error_log.go`

## 总体原则

- `WriteError(...)` 不再依赖任何 router、request logger 或 tracing 中间件
- 不再补 request log 字段
- 只在需要时通过 `slog.Default()` 输出独立错误日志

## `WriteError(...)` 的日志分支

### `err == nil`

- 直接返回 `nil`
- 不写日志

### 响应已经开始写出

如果满足以下任一条件：

- 传入错误能解出 `responseWriteError`，且 `responseStarted=true`
- `ResponseWriter` 显式暴露可读的状态信息，且已检测到状态码或已写字节数

则：

- 不再改写响应
- 若错误最终收敛为 `5xx`，仍会通过 `slog.Default()` 输出一条独立错误日志
- 返回原错误

### 常规错误响应路径

未命中提前返回时，执行顺序为：

1. `asHTTPError(err)`
2. `logServerError(r, httpErr, err)`
3. `writeHTTPError(w, httpErr)`
4. `logErrorResponseWriteFailure(r, httpErr, writeErr)`
5. 返回 `writeErr`

其中：

- 第 2 步只在 `httpErr.Status() >= 500` 时输出独立错误日志
- 第 4 步只在 `writeErr != nil` 时输出独立错误日志

## 独立错误日志行为

### 普通 5xx

当 `WriteError(...)` 最终收敛为 `5xx` 时，会额外输出一条独立 `error` 日志：

- 消息：`resp: request failed with server error`

字段包括：

- `http.response.status_code`
- `http.request.method`
- `url.path`
- `error.code`
- `error.message`
- `error.type`
- `error.root_message`
- `error.root_type`
- `error.timeout`
- `error.canceled`

其中：

- `http.request.method` / `url.path` 仅在 `*http.Request` 非空时写入
- `error.timeout` 仅在 `errors.Is(err, context.DeadlineExceeded)` 时写入
- `error.canceled` 仅在 `errors.Is(err, context.Canceled)` 时写入
- `error.message` / `error.type` 来自诊断起点本身
- `error.root_message` / `error.root_type` 来自默认单链 `Unwrap() error` 向下追踪后的尾部摘要

### 错误响应写出失败

`logErrorResponseWriteFailure(...)` 只在错误响应写出路径返回非空错误时输出独立
`error` 日志：

- 消息：`resp: failed to write error response`

字段包括：

- `http.response.status_code`
- `http.request.method`
- `url.path`
- `error.code`
- `error.message`
- `error.type`
- `error.root_message`
- `error.root_type`

如果错误可解出 `ErrorWriteDegraded`，还会追加：

- `resp.error_degraded`
- `resp.public_response_preserved`

## 诊断起点与错误链

5xx 诊断起点规则为：

- 如果 `HTTPError` 持有 `cause`，优先从 `httpErr.cause` 开始
- 否则从原始 `err` 开始

错误响应写出失败的诊断起点规则为：

- 始终从实际的 `writeErr` 开始
- 不再回跳到原始 `HTTPError.cause`

默认诊断摘要只沿单链 `Unwrap() error` 下钻，不展开多分支错误图。

这意味着：

- 常见 `fmt.Errorf("...: %w", err)` / 包装 error 场景会保留首层和尾部摘要
- `errors.Join(...)` 或自定义 `Unwrap() []error` 不会被继续展开
- 若 `Error()` / `Unwrap()` 实现本身 panic，会安全降级为停止下钻或说明性文本

限制为：

- 最大下钻深度 `8`

`error.root_message` 和 `error.root_type` 表示默认单链诊断路径的尾部摘要，
用于帮助排查常见包装错误，不承诺覆盖所有复杂错误组合形式。
