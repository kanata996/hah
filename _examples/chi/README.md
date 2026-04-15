# chi + hah example

这个独立子模块演示把 `hah` 放在 `chi` 接入层后面使用：router 和入口层 middleware 继续由 `chi` 管，`hah` 只负责绑定、显式输入 helper、JSON 成功响应和 `application/problem+json` 错误响应。

核心关注点：

- 使用当前 `hah` / `errx` 公开 API，而不是旧的 render/runtime 模型
- 保留 `chi` 常用中间件：`RequestID`、`RealIP`、`Timeout`、`Heartbeat`
- 用 `traceid.Middleware` 生成/透传 `TraceId`，并把它带到 `httplog` 和 `slog` 上下文
- 用 `httplog/v3` 输出结构化 access log，并补 `request.id`
- 在 handler 入口把 `chi.RouteContext` 显式桥接到 `net/http` `PathValue` / `Pattern` 契约
- `DELETE` 路由额外演示直接读取 header 后手写 header 校验

主要路由：

- `GET /healthz`
- `GET /orgs/{org_id}/accounts`
- `POST /orgs/{org_id}/accounts`
- `GET /orgs/{org_id}/accounts/{account_id}`
- `DELETE /orgs/{org_id}/accounts/{account_id}`

请求主流程：

1. `middleware.RequestID` 生成 request ID，`traceid.Middleware` 生成或透传 `TraceId`。
2. `httplog/v3` 输出结构化 request log，并记录 `request.id` / `trace.id`。
3. handler 入口先把 `chi.RouteContext` 回填到 `net/http` 的 `PathValue` / `Pattern` 契约。
4. handler 用 `hah.Path(...)` 读取 path，用 `hah.BindQuery(...)` / `hah.BindBody(...)` 处理 DTO 输入，再显式做最小请求校验。
5. `DELETE` 路由额外演示直接读取 header 后的手写 header 校验。
6. 领域层直接返回 `errx` 公共错误；失败路径统一走 `hah.WriteError(...)`。
7. 成功路径统一走 `hah.OK(...)`、`hah.Created(...)` 和 `hah.NoContent(...)`。

响应层观察点：

- 响应头包含 `X-Request-Id`
- 响应头包含 `TraceId`
- `GET /orgs/{org_id}/accounts` 的 JSON 响应会回显 `request_id` / `trace_id`
- `main.go` 里把 `slog.Default()` 设为带 `traceid.LogHandler(...)` 的 logger，方便响应写回失败日志复用同一个 `trace.id`

入口文件：

- `main.go`
- `smoke_test.go`

常用命令：

- `go test ./...`
- `go run .`
