# net/http example

这个独立子模块演示 `hah` 在标准库 `net/http` / `ServeMux` 中的最新公开 API，用最少依赖把请求绑定、显式校验、成功响应和 `problem+json` 错误响应串起来。

核心关注点：

- 只使用当前根包公开 API：`Path`、`BindQuery`、`BindBody`、`RequireBody`、`InvalidRequest`、`OK`、`Created`、`NoContent`、`WriteError`
- path 走 `hah.Path(...)`，query/body 走独立的 DTO binding API
- handler 在 `BindQuery(...)` / `BindBody(...)` 之后显式做最小输入校验
- 当更深层已经明确要暴露稳定公共 HTTP 错误时，可以直接返回 `hah.NotFound(...)`、`hah.Conflict(...)`
- 成功路径直接写 JSON，失败路径统一走 `hah.WriteError(...)`

主要路由：

- `GET /healthz`
- `GET /orgs/{org_id}/accounts`
- `POST /orgs/{org_id}/accounts`
- `GET /orgs/{org_id}/accounts/{account_id}`
- `DELETE /orgs/{org_id}/accounts/{account_id}`

请求主流程：

1. handler 用 `hah.Path(...)` 读取 path，用 `hah.BindQuery(...)` / `hah.BindBody(...)` 处理 DTO 输入。
2. handler 显式执行请求级校验，例如 `hah.RequireBody(...)` 与 `hah.InvalidRequest(...)`。
3. handler 调用内存 store；store 在已明确公开错误语义时直接返回 `hah` 的公共错误。
4. 失败路径统一调用 `hah.WriteError(w, err)` 写回 `application/problem+json`。
5. 成功路径统一走 `hah.OK(...)`、`hah.Created(...)` 或 `hah.NoContent(...)`。

入口文件：

- `main.go`
- `smoke_test.go`

常用命令：

- `go test ./...`
- `go run .`
