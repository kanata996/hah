# net/http example

这个独立子模块只演示 `hah` 在标准库 `net/http` / `ServeMux` 里的 `Direct HTTPError Mode` 主流程，不混入 auth、rate limit、panic recover、request id 等入口层噪音。

核心关注点：

- 不使用 `WithResponses(...)`，也不使用 mapper
- handler 只负责 `DecodeAndValidate*`、调用 service、再用 `hah.RenderError(...)` / `Render*`
- service / repository 直接返回公开 HTTP 错误，例如 `hah.NotFound(...)`、`hah.Conflict(...)`
- 成功响应统一走 `Render(...)` / `RenderWithMeta(...)`

主要路由：

- `GET /users`：query decode + validate + `RenderWithMeta`
- `GET /users/{userID}`：repository 直接返回 `404`
- `POST /users`：JSON decode + validate + `Create`，冲突时直接返回 `409`

请求主流程：

1. handler 用 `DecodeAndValidateQuery(...)` 或 `DecodeAndValidateJSON(...)` 处理输入。
2. handler 调用 service，service 再调用 repository。
3. repository / service 直接返回 `hah.NotFound(...)`、`hah.Conflict(...)` 这类公开 HTTP 错误。
4. handler 在失败点统一调用 `hah.RenderError(w, r, err)`。
5. `RenderError(...)` 直接把这些公开错误写成统一 JSON HTTP 错误响应。
6. 成功路径统一走 `Render(...)` 或 `RenderWithMeta(...)`。

说明：

- `Mapped Internal Error Mode` 在 `net/http` 里也能做，但不是这个示例的重点。
- 如果要强调 feature 边界挂载 mapper，`chi` 示例更直观。

入口文件：

- `main.go`
- `smoke_test.go`

常用命令：

- `go test ./...`
- `go run .`
