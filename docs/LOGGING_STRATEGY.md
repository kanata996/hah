# 日志记录策略

本文说明 `hah` 默认错误观测与日志输出的当前策略。

## 1. 范围

`hah` 只负责业务边界内、由 `RenderError(...)` 触发的错误观测。

它不负责：

- access log
- panic recover log
- router / middleware 自己写出的响应日志
- tracing / metrics 平台

## 2. `ErrorReport`

`hah` 的统一错误观测结构是：

```go
type ErrorReport struct {
	Request         *http.Request
	Error           error
	PublicError     *HTTPError
	RequestID       string
	ResponseStarted bool
}
```

字段语义：

- `Request`：当前请求
- `Error`：当前这次观测对应的原始错误
- `PublicError`：对外公开的边界错误
- `RequestID`：这次观测实际使用的 request id
- `ResponseStarted`：观测发生时，`hah` 管理的响应是否已经开始

当前没有 `Stage` 字段。不要在实现或文档里假设存在内部阶段标签。

## 3. 默认 reporter

默认 reporter 当前只记录：

- `401` / `403` 安全事件
- `5xx` 内部错误

默认 reporter 不记录：

- 普通业务 `4xx`
- 成功响应

原因：

- 业务 `4xx` 通常不等于系统异常
- access log、metrics、trace 应由外层系统负责
- `hah` 只补业务边界内部最需要的错误观测

## 4. `ResponseStarted`

`ResponseStarted` 的当前语义是：

- 只反映 `hah` 自己管理的 render 路径是否已经开始
- 不再是对任意 `ResponseWriter` 操作的透明观测

因此：

- 如果先调用了 `Render(...)`，再调用 `RenderError(...)`，`ResponseStarted` 会是 `true`
- 如果错误在首次写回前发生，`ResponseStarted` 会是 `false`

这已经足够支撑当前 render-first 模型。

## 5. 二次观测

如果 `RenderError(...)` 在写统一错误响应时再次失败，`hah` 会：

1. 先发出原始错误对应的 `ErrorReport`
2. 再发出一次针对“写错误响应失败”的内部错误观测

要求保持不变：

- 两次观测共享同一个 request id
- 第二次观测的 `PublicError` 是内部错误
- 第二次观测的 `Error` 是底层写回失败

## 6. request id

如果调用方没有显式设置 request id，`hah` 会在第一次进入错误观测路径时惰性生成一个。

更多细节见 [REQUEST_ID_STRATEGY.md](./REQUEST_ID_STRATEGY.md)。

## 7. 维护原则

- 不要把 `hah` 默认 logger 扩张成通用日志平台
- 不要为了“更精细”伪造不可靠的阶段标签
- 不要让默认 reporter 接管 access log 或 tracing 语义
- 任何可观察日志行为变更都应同步更新 `README.md` 或本文件
