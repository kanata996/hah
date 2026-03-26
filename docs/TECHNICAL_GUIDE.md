# 技术指导文档

本文面向 `hah` 的维护者与贡献者，说明当前实现的边界、设计约束与演进方向。

## 1. 项目定位

`hah` 是一个保持 `net/http` 原生兼容的业务边界 JSON API 契约层。

当前目标：

- 统一 JSON 成功响应和错误响应
- 保持成功路径与失败路径都显式
- 让 mapper / reporter 在业务边界集中挂载
- 保持 `chi` / `net/http` 的原生 handler 和 middleware 组织方式

明确的非目标：

- 不接管整个 HTTP 生命周期
- 不接管 panic recover、auth、rate limit、CORS、redirect
- 不发明新的 handler runtime
- 不为少见 transport 场景提前承诺一组复杂 API

## 2. 当前运行时模型

推荐模型如下：

1. 外层 middleware 处理 request id、日志、auth、rate limit、recover 等接入层职责
2. 在业务边界 route group 上可选地挂 `hah.Contract(...)`
3. handler 内用 `hah.Decode*` / `hah.Validate` 处理输入
4. 成功路径显式调用 `hah.Render(...)` / `hah.RenderWithMeta(...)` / `hah.RenderEmpty(...)`
5. 失败路径在发生点显式调用 `hah.RenderError(...)`

典型代码：

```go
func(w http.ResponseWriter, r *http.Request) {
	if err := hah.DecodeJSON(r, &req); err != nil {
		_ = hah.RenderError(w, r, err)
		return
	}

	user, err := svc.GetUser(r.Context(), userID)
	if err != nil {
		_ = hah.RenderError(w, r, err)
		return
	}

	if err := hah.Render(w, r, user); err != nil {
		_ = hah.RenderError(w, r, err)
		return
	}
}
```

维护时应坚持这几个原则：

- 成功路径显式
- 失败路径显式
- 策略集中
- 控制流透明

## 3. `Contract(...)` 的职责

`Contract(...)` 现在是一个很薄的边界 middleware。

它负责：

- 注入 route-scoped mapper / reporter 配置
- 复用 request-scoped state
- 和 request id state 协同工作

它不负责：

- 包装 `ResponseWriter`
- 观测任意 raw `w.Write(...)`
- 接管 panic
- 在请求结束时统一回收错误
- 自动决定成功响应

如果未来某个改动开始让 `Contract(...)` 看起来像隐藏 runtime，应优先拒绝。

## 4. 为什么删除 tracking writer

旧实现试图同时满足三件事：

- 包装 `ResponseWriter`
- 跟踪“响应是否已经开始”
- 继续透明暴露 `Flusher` / `Hijacker` / `Pusher` 等可选接口

这会迫使实现落回大量 concrete type 组合和方法集样板。

当前版本明确不再追求这件事。原因很简单：

- `hah` 的主问题域是普通 JSON API，而不是通用 transport runtime
- 对常规 JSON API 来说，显式 `Render*` / `RenderError` 比透明包装 writer 更简单、更稳定
- 少见场景后续如果真的有明确需求，再单独设计；不要提前公开承诺

## 5. render-first 设计

当前实现参考 `go-chi/render` 的思路：

- request context 里只保存很薄的 render state
- `Status(r, status)` 只是写一个 status hint
- `Render(...)` / `RenderWithMeta(...)` / `RenderEmpty(...)` 才是真正的写回入口
- `RenderError(...)` 负责 mapper、reporter 和统一错误响应

实现分层：

- 根包 `render.go` 只做公开转发
- `internal/render` 承载底层 render runtime
- 错误映射和错误观测仍然留在根包

这层 request state 当前只承担两件事：

- 给后续 `Render(...)` 提供 status hint
- 标记 `hah` 管理的响应是否已经开始

这里的“响应开始”语义是显式 render 语义，不是对任意 raw writer 操作的全局观测。

## 6. 错误处理原则

`RenderError(...)` 的处理顺序应保持为：

1. 合并 route-scoped 和 call-site mapper / reporter 配置
2. 必要时确保 request id
3. 把内部错误映射成公开 `*HTTPError`
4. 发出 `ErrorReport`
5. 如果响应已经开始，只做观测，不再尝试改写
6. 否则写统一错误 envelope
7. 如果错误写回失败，再发第二条内部错误观测

维护约束：

- 不要把错误缓存到 request 结束再统一处理
- 不要为了“更方便”重新引入隐式 error state
- `RenderError(...)` 的行为必须继续保持同步、显式、可预测

## 7. 公开 API 收缩原则

当前公开 API 故意收得很窄，保持接近 `chi/render` 的主叙事：

- `Contract(...)`
- `Render(...)`
- `RenderWithMeta(...)`
- `RenderEmpty(...)`
- `RenderError(...)`
- `Status(...)`

不要轻易新增这些类别的 API：

- streaming runtime
- SSE runtime
- websocket / upgrade runtime
- 复杂 content-type 管理器

新增公开 API 的前提应是：

- 已经出现真实使用场景
- 可以清楚描述公开契约
- 可以提供稳定测试口径
- 不会把 `hah` 重新推回“通用 HTTP runtime”

## 8. 测试口径

当前维护要求：

- 根包公开 API 行为必须有测试
- `internal/render` 的关键分支必须有测试
- request id 共享和 reporter 行为必须有测试
- 公开 HTTP 契约的变更，必须先更新 `README.md`

如果某个实现分支只能靠非常牵强的假测试去覆盖，优先检查它是否其实是不必要的防御分支。

## 9. 文档约束

- `README.md` 是公开契约文档，优先描述用户该怎么用
- 本文档只讨论实现原则和维护边界
- `docs/LOGGING_STRATEGY.md` 解释默认错误观测
- `docs/REQUEST_ID_STRATEGY.md` 解释 request id 桥接与生成策略

一旦代码和文档不一致，优先修文档，再决定是否修实现。
