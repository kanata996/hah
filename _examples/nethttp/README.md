# net/http example

这个独立子模块演示 `hah` 在标准库 `net/http` / `ServeMux` 中的最新公开 API，用最少依赖把请求绑定、校验、成功响应和 `problem+json` 错误响应串起来。

核心关注点：

- 只使用当前根包公开 API：`BindAndValidate`、`OK`、`Created`、`NoContent`、`WriteError`
- path/query/body 都走 `hah` 的 `net/http` 绑定契约，不依赖 router 私有上下文
- 领域层直接返回 `errx.NotFound(...)`、`errx.Conflict(...)` 这类稳定公共错误
- 成功路径直接写 JSON，失败路径统一写 `application/problem+json`

主要路由：

- `GET /healthz`
- `GET /orgs/{org_id}/accounts`
- `POST /orgs/{org_id}/accounts`
- `GET /orgs/{org_id}/accounts/{account_id}`
- `DELETE /orgs/{org_id}/accounts/{account_id}`

请求主流程：

1. handler 用 `hah.BindAndValidate(...)` 处理 path/query/body 输入。
2. handler 调用内存 store；store 直接返回 `errx` 公共错误。
3. 失败路径统一调用 `hah.WriteError(w, r, err)`。
4. 成功路径统一走 `hah.OK(...)`、`hah.Created(...)` 或 `hah.NoContent(...)`。

入口文件：

- `main.go`
- `smoke_test.go`

常用命令：

- `go test ./...`
- `go run .`
