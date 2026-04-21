# net/http example

这个独立子模块演示 `hah` 在标准库 `net/http` / `ServeMux` 中的最新公开 API，用最少依赖把请求绑定、显式校验和默认 JSON envelope 响应串起来。

核心关注点：

- 只使用当前根包公开 API：`Path`、`BindQuery`、`BindBody`、`InvalidRequest`、`OK`、`Accepted`、`Created`、`NoContent`、`WriteError`
- path 走 `hah.Path(...)`，query/body 走独立的 DTO binding API
- handler 在 `BindQuery(...)` / `BindBody(...)` 之后显式做最小输入校验
- 当更深层已经明确要暴露稳定公共 HTTP 错误时，可以直接返回 `hah.NotFound(...)`、`hah.Conflict(...)`
- 成功和失败都走默认 JSON envelope；只有 `hah.JSON(...)` 才会绕过默认协议

主要路由：

- `GET /healthz`
- `GET /orgs/{org_id}/accounts`
- `POST /orgs/{org_id}/accounts`
- `GET /orgs/{org_id}/accounts/{account_id}`
- `DELETE /orgs/{org_id}/accounts/{account_id}`

请求主流程：

1. handler 用 `hah.Path(...)` 读取 path，用 `hah.BindQuery(...)` / `hah.BindBody(...)` 处理 DTO 输入。
2. handler 在 `BindQuery(...)` / `BindBody(...)` 之后显式执行最小请求级校验，并用 `hah.InvalidRequest(...)` 收敛输入错误。
3. handler 调用内存 store；store 在已明确公开错误语义时直接返回 `hah` 的公共错误。
4. 失败路径统一调用 `hah.WriteError(w, err)` 写回错误 envelope。
5. 成功路径通常走 `hah.OK(...)` / `hah.Accepted(...)` / `hah.Created(...)`；若调用方明确需要无响应体成功，也可以显式用 `hah.NoContent(...)`。当前示例仍统一返回 envelope。

入口文件：

- `main.go`
- `smoke_test.go`

常用命令：

- `go test ./...`
- `go run .`
