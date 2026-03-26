# chi example

这个独立子模块只演示 `hah` 在 `chi` 里的 `Mapped Internal Error Mode` 主流程，不混入 auth、rate limit、panic recover 等入口层噪音。

核心关注点：

- `Contract(hah.WithContractErrorMappers(...))` 挂在 `/users` feature 边界
- handler 只负责 `DecodeAndValidate*`、调用 service、再用 `hah.RenderError(...)` / `Render*`
- service / repository 返回内部错误语义，mapper 统一转成公开 HTTP 错误
- 成功响应统一走 `Render(...)` / `RenderWithMeta(...)`

主要路由：

- `GET /users`：query decode + validate + `RenderWithMeta`
- `GET /users/{userID}`：repository/service 返回内部 `not found`，由 mapper 转成 `404`
- `POST /users`：JSON decode + validate + `Create`，冲突错误由 mapper 转成 `409`

请求主流程：

1. handler 用 `DecodeAndValidateQuery(...)` 或 `DecodeAndValidateJSON(...)` 处理输入。
2. handler 调用 service，service 再调用 repository。
3. repository / service 返回内部错误语义，例如 `errUserNotFound`、`errUserConflict`。
4. handler 在失败点统一调用 `hah.RenderError(w, r, err)`。
5. `/users` 上挂载的 `Contract(...)` 通过 mapper 把内部错误转成统一 JSON HTTP 错误响应。
6. 成功路径统一走 `Render(...)` 或 `RenderWithMeta(...)`。

入口文件：

- `main.go`
- `smoke_test.go`

常用命令：

- `go test ./...`
- `go run .`
