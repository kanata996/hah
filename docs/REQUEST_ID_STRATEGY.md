# Request ID 策略

本文面向 `hah` 维护者与内部 review，说明当前 `request id` 的职责边界、公开契约与接入建议。

## 1. 设计目标

当前策略要同时满足以下目标：

- `hah` 根包不依赖某个具体 HTTP 框架的 request id 实现
- `chi`、`net/http`、自定义 middleware 都能接入
- 进入 `hah` 错误处理链后，`ErrorReport.RequestID` 必须稳定可用
- 同一请求中的多次错误观测应复用同一个 request id
- 不把“不可信外部 header”误当成“框架已确认的内部 request id”

## 2. 非目标

当前策略明确不负责：

- 规定整个应用必须使用哪一种 request id 生成算法
- 自动识别所有框架的 context key
- 自动把 request id 写回请求头或响应头
- 把 `request id` 和 `trace id` 合并为同一个字段
- 为所有成功请求都提前生成 request id

## 3. 术语

### 3.1 外部 request id

由网关、客户端或外层 middleware 提供的 request id。

这个值适合用于跨服务关联，但不应天然视为可信输入。

### 3.2 有效 request id

`hah` 当前错误处理链实际采用的 request id。

`ErrorReport.RequestID` 表达的就是这个语义。

来源优先级：

1. 调用方通过 `SetRequestID(...)` 显式注入
2. 如果没有显式注入，`hah` 在第一次发送错误观测时惰性生成

## 4. 公开契约

当前公开 API：

```go
func SetRequestID(r *http.Request, id string) *http.Request
```

约束：

- 调用方应始终使用返回后的 `*http.Request` 继续向下传递
- `id` 为空或只包含空白字符时，`hah` 会忽略本次设置
- `SetRequestID(...)` 只负责把 request id 交给 `hah`
- `SetRequestID(...)` 不负责生成 request id，不负责写响应头

`ErrorReport.RequestID` 的公开语义：

- 表示当前错误观测实际使用的 request id
- 不承诺它来自请求头
- 不承诺它来自某个具体框架
- 只承诺在当前错误处理链中稳定

## 5. 为什么不再读取请求头

旧方案通过配置 header 名称，从请求头直接拷贝 request id。

这个方案的问题是：

- 根包只能看到“原始输入”，看不到“应用最终采用的 request id”
- 会鼓励把 header 输入误当成强可信内部标识
- 对 `chi`、自定义 middleware、trace 系统等 context 驱动方案不友好
- 一旦为某个框架加特判，会污染核心库设计

因此当前策略改成：

- `hah` 不再自己猜测 request id 来源
- 调用方显式注入
- 缺失时由 `hah` 在错误路径内部兜底生成

## 6. 默认生成策略

如果当前请求没有显式设置 request id，`hah` 会在第一次发送错误观测时自动生成一个。

当前行为：

- 只在错误路径生成
- 成功请求不会因为 `hah` 额外生成 request id
- 同一错误处理链中的后续观测会复用第一次生成的值
- 生成格式当前是 `req_` 前缀加随机串

注意：

- 生成格式是内部实现细节，不应作为对外兼容契约
- 这个兜底 id 主要用于 `hah` 自身错误日志关联，不替代全局 access log / tracing 策略

## 7. `chi` 接入建议

推荐接法：

1. 外层使用 `chi/middleware.RequestID`
2. 在应用层通过一个 bridge middleware 调 `hah.SetRequestID(...)`
3. 在业务 handler 边界显式调用 `hah.WriteError(...)`

示例：

```go
func bindRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqID := middleware.GetReqID(r.Context()); reqID != "" {
			r = hah.SetRequestID(r, reqID)
		}
		next.ServeHTTP(w, r)
	})
}
```

这样做的好处：

- `chi` 负责生成或传播 request id
- `hah` 只消费应用显式确认后的值
- `hah` 根包不会依赖 `chi` 的 context 协议

## 8. `net/http` 接入建议

对于标准库项目，推荐在自定义 middleware 中自行生成 request id，然后传给 `hah`。

示例：

```go
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = hah.SetRequestID(r, newRequestID())
		next.ServeHTTP(w, r)
	})
}
```

这里的 `newRequestID()` 由应用自己决定，可以来自：

- UUID
- ULID
- 网关下发的相关性标识
- 自定义内部生成器

## 9. 安全边界

`request id` 不是秘密，但也不应被当成可信业务输入。

维护时应坚持：

- 不要用 `request id` 做鉴权或权限判断
- 不要把未清洗的 header 值直接当可信内部 id
- 如果接入方要复用外部 header，建议在应用层先完成校验和清洗
- `request id` 与 `trace id` 应分开建模

## 10. 对日志排障的意义

当前策略能保证：

- `processing`、`write_response` 等错误日志都有 request id
- 同一请求内多条 `ErrorReport` 可以稳定串起来
- 即使调用方忘了设置 request id，`hah` 也不会在错误日志里留下空洞

当前策略不能保证：

- `hah` 自动生成的兜底 request id 能和 access log、网关日志天然对齐
- 所有成功请求都具备可全链路检索的 request id
- request id 天然等同于 tracing 系统里的 span / trace 语义

## 11. review 关注点

以后改动 request id 策略时，review 至少应回答：

- 是否引入了对具体框架的根包依赖
- 是否改变了 `ErrorReport.RequestID` 的公开语义
- 是否仍能保证错误链路内 request id 稳定
- 是否误把 trace id 和 request id 混成一个字段
- 是否需要同步更新 `README.md`、`docs/TECHNICAL_GUIDE.md`、`docs/LOGGING_STRATEGY.md`
