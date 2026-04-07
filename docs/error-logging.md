# 错误日志当前行为

本文只说明 `resp.WriteError(...)` 相关日志记录的当前行为。

相关源码：

- `resp/write_error.go`
- `resp/error_log.go`

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
- 不再补 request log 字段
- 若错误最终收敛为 `5xx`，仍会通过 `slog.Default()` 输出一条独立错误日志
- 返回原错误

### 常规错误响应路径

未命中提前返回时，执行顺序为：

1. `asHTTPError(err)`
2. `annotateRequestErrorLog(r, err, httpErr)`
3. `logServerError(r, httpErr, err)`
4. `writeHTTPError(w, r, httpErr)`
5. `logErrorResponseWriteFailure(r, httpErr, writeErr)`
6. 返回 `writeErr`

其中：

- 第 2 步只影响当前 request log
- 第 3 步只在 `httpErr.Status() >= 500` 时输出独立错误日志
- 第 5 步只在 `writeErr != nil` 时输出独立错误日志

## Request Log 行为

`annotateRequestErrorLog(...)` 只通过 `httplog.SetAttrs(...)` 给当前 request log
补字段，本身不直接输出日志。

如果请求没有经过 `httplog.RequestLogger(...)`，这一步会自然退化为 no-op。

### 4xx

当 `httpErr.Status() < 500` 时：

- 不补任何 `error.*` 字段
- 只保留外层 request log 原有字段

### 5xx

当 `httpErr.Status() >= 500` 时，会补以下字段：

- `error.code`
- `error.timeout`
- `error.canceled`
- `traceId`
- `request.id`

其中：

- `error.code` 始终来自 `HTTPError.Code()`
- `error.timeout` 仅在 `errors.Is(err, context.DeadlineExceeded)` 时写入
- `error.canceled` 仅在 `errors.Is(err, context.Canceled)` 时写入
- `traceId` / `request.id` 仅在当前上下文里已有对应值时写入

如果你希望所有 access log 都带上 `traceId`、`request.id`，需要在服务
自己的 `chi + httplog` 链路里额外挂 `middleware.RequestLogAttrs()`；`WriteError(...)`
自己的 5xx request log 注解不依赖这个中间件。

默认示例不会挂这个中间件，因为 `WriteError(...)` 已经会在 5xx request log 上补
`traceId` / `request.id`。如果两者同时使用，5xx access log 会再次追加同名字段。

### 不再写入 Request Log 的字段

当前不会写入 request log 的字段有：

- `error.message`
- `error.type`
- `error.root_message`
- `error.root_type`
- `error.details`
- `error.details_count`
- `error.details_dropped`
- `error.chain`
- `error.chain_types`
- `error.wrapped`
- `error.public_message`
- `error.category`
- `error.expected`

## 独立错误日志行为

### 普通 5xx

当 `WriteError(...)` 最终收敛为 `5xx` 时，会通过 `slog.Default()` 输出一条独立
`error` 日志：

- 消息：`resp: request failed with server error`

字段包括：

- `http.response.status_code`
- `error.code`
- `error.message`
- `error.type`
- `error.root_message`
- `error.root_type`
- `error.timeout`
- `error.canceled`
- `traceId`
- `request.id`

### 错误响应写出失败

`logErrorResponseWriteFailure(...)` 只在错误响应写出路径返回非空错误时输出独立
`error` 日志：

- 消息：`resp: failed to write error response`

字段包括：

- `http.response.status_code`
- `error.code`
- `error.message`
- `error.type`
- `error.root_message`
- `error.root_type`
- `traceId`
- `request.id`

如果错误可解出 `ErrorWriteDegraded`，还会追加：

- `resp.error_degraded`
- `resp.public_response_preserved`

## 诊断起点与错误链

5xx 诊断起点规则为：

- 如果 `HTTPError` 持有 `cause`，优先从 `httpErr.cause` 开始
- 否则从原始 `err` 开始

错误链展开同时兼容：

- `Unwrap() error`
- `Unwrap() []error`

限制为：

- 最大深度 `8`
- 使用 `seen` 集合避免循环引用

`error.root_message` 和 `error.root_type` 在普通单链包装场景里通常对应最底层
错误；在 `errors.Join(...)` 场景里，它们表示本次遍历尾部摘要，不保证是唯一根因。

## `errors` 的当前行为

`errors` 当前只属于公共错误响应，不写入 request log，也不会原样塞进独立错误日志。
