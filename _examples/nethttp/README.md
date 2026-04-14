# net/http example

这个独立子模块演示 `hah` 在标准库 `net/http` / `ServeMux` 中的最新公开 API，用最少依赖把请求绑定、显式校验、成功响应和 `problem+json` 错误响应串起来。

核心关注点：

- 只使用当前根包公开 API：`Path`、`BindQuery`、`BindBody`、`RequireBody`、`AsHTTPError`、`OK`、`Created`、`NoContent`、`WriteError`
- path 走 `hah.Path(...)`，query/body 走独立的 DTO binder
- handler 在 `BindQuery(...)` / `BindBody(...)` 之后显式做最小输入校验
- 领域层直接返回 `errx.NotFound(...)`、`errx.Conflict(...)` 这类稳定公共错误
- 成功路径直接写 JSON，失败路径显式记录 `5xx` 后再写 `application/problem+json`

主要路由：

- `GET /healthz`
- `GET /orgs/{org_id}/accounts`
- `POST /orgs/{org_id}/accounts`
- `GET /orgs/{org_id}/accounts/{account_id}`
- `DELETE /orgs/{org_id}/accounts/{account_id}`

请求主流程：

1. handler 用 `hah.Path(...)` 读取 path，用 `hah.BindQuery(...)` / `hah.BindBody(...)` 处理 DTO 输入。
2. handler 显式执行请求级校验，例如 `hah.RequireBody(...)` 与 `reqx.InvalidRequest(...)`。
3. handler 调用内存 store；store 直接返回 `errx` 公共错误。
4. 失败路径先用 `hah.AsHTTPError(err)` 判断是否需要记录 `5xx`，再调用 `hah.WriteError(w, err)`。
5. 成功路径统一走 `hah.OK(...)`、`hah.Created(...)` 或 `hah.NoContent(...)`。

入口文件：

- `main.go`
- `smoke_test.go`

常用命令：

- `go test ./...`
- `go run .`
