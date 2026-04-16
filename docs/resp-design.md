# hah resp 设计方案

- 状态：Locked
- 版本：v2
- 锁定日期：2026-04-17
- 适用范围：
  - `resp.JSON`
  - `resp.OK`
  - `resp.Created`
  - `resp.NoContent`
  - `resp.WriteError`
- 不覆盖：
  - router / handler 生命周期
  - 业务 envelope
  - 内容协商
  - panic recover
- 关联文档：
  - `docs/testing-standards.md`
  - `docs/errx-design.md`
- 变更规则：任何公开行为变化，必须先改本文档并补黑盒测试。

## 1. 设计定位

`resp` 只负责 HTTP 响应写回。
它只提供两类默认输出：

- 成功响应：`application/json`
- 错误响应：`application/problem+json`

`resp` 不做内容协商，不发明 envelope，也不负责业务错误分类。

## 2. 稳定公开 API 与通用规则

### 2.1 公开入口

公开入口固定为五个：

- `JSON(w http.ResponseWriter, status int, data any) error`
- `OK(w http.ResponseWriter, data any) error`
- `Created(w http.ResponseWriter, data any) error`
- `NoContent(w http.ResponseWriter) error`
- `WriteError(w http.ResponseWriter, err error) error`

当前设计不包含 `JSONBlob` 之类的原始字节透传入口。

### 2.2 通用规则

所有入口都只在“首次提交前”提供保证。

稳定规则固定为：

- 除 `WriteError(nil, nil)` 外，`w == nil` 时必须返回 error，且不得写出任何内容
- `WriteError(nil, nil)` 必须是纯 no-op：返回 `nil` 且不得写出任何内容
- `resp` 只拥有 `Content-Type` 与 `Content-Length` 的所有权
- 无关自定义头部必须保留
- 冲突的 `Content-Type` 必须覆盖
- 预设的 `Content-Length` 不得原样穿透
- `HEAD` 请求沿用 `net/http` 默认语义；`resp` 按正常路径写回，由底层决定最终是否发送 body
- 不负责恢复调用方自定义 `MarshalJSON`、`Error()` 或包装 `ResponseWriter` 实现中的 panic

## 3. 成功写回契约

### 3.1 `JSON` / `OK` / `Created`

三者共享同一套成功写回契约：

- `JSON` 只接受 `200 OK`、`201 Created`、`202 Accepted`
- `OK` 固定写 `200`
- `Created` 固定写 `201`
- 主媒体类型必须是 `application/json`
- 响应体必须是调用方提供值的直接 JSON 表达
- 不得隐式包裹成 envelope
- `nil` payload 必须编码为 JSON `null`
- JSON 序列化语义跟随 `encoding/json`

失败规则：

- 非法成功状态码必须在首次提交前返回 error
- payload 不可 JSON 编码时必须在首次提交前返回 error
- 上述失败都不得隐式回退成 `500` Problem 响应
- `206`、`207`、`208`、`226` 以及其他未列出的 `2xx` 都不属于 `JSON(...)` 的支持范围

### 3.2 `NoContent`

`NoContent()` 的契约固定为：

- 写出 `204 No Content`
- 不写响应体
- 清除 `Content-Type`
- 清除 `Content-Length`
- 保留无关自定义头部

## 4. 错误写回契约

### 4.1 Problem JSON 形状

错误响应固定为：

- HTTP 状态码属于 `4xx` 或 `5xx`
- 主媒体类型为 `application/problem+json`
- 响应体是最小稳定 Problem JSON

稳定字段只有：

- `title`
- `status`
- `detail`
- `code`
- `errors`

规则：

- `detail` 仅在归一化后非空时写出
- `code` 必须写出，且值来自归一化后公开错误的 `Code()`
- `errors` 仅在归一化后非空时写出
- `title` 是人类可读标题，不等同于 `code`
- `errors` 的稳定 JSON 字段固定为 `field` / `in` / `code` / `detail`
- `resp` 不排序、不去重、不改写 `errors`

### 4.2 `WriteError` 收敛顺序

`WriteError` 必须先把任意 `error` 收敛为稳定公开错误语义，再写回。

收敛顺序固定为：

1. `err == nil`：返回 `nil`，不写任何内容
2. 若输入 error 已表示一次“已开始的响应写回失败”：返回传入的同一个 error，且不得再次写回
3. `w == nil`：返回 error，且不得写任何内容
4. 错误链中存在非 `nil` 的 `*errx.HTTPError`：最终状态码取其 `Status()`
5. `errors.Is(err, context.Canceled)`：最终状态码为 `499`
6. `errors.Is(err, context.DeadlineExceeded)`：最终状态码为 `504`
7. 其他情况：最终状态码为 `500`

补充规则：

- typed-nil `*errx.HTTPError` 不视为匹配到公共 HTTP 错误
- 若错误链里只有 typed-nil `*errx.HTTPError`，则按“未匹配到 `*errx.HTTPError`”继续后续收敛
- `WriteError` 不得依赖 nil receiver 的 `Status()` / `Detail()` / `Errors()` 行为
- 若匹配到非 `nil` 的 `*errx.HTTPError`，后续 `code` / `detail` / `errors` 从其归一化公开语义提取
- 若未匹配到非 `nil` 的 `*errx.HTTPError`，则视为框架合成错误，只允许写最小 Problem：`title` / `status` / `code`

然后再执行一次状态码归一化：

- `499` 原样保留
- `400..599` 的其他状态码原样保留
- 其他值统一收敛为 `500`

### 4.3 `WriteError` 字段归一化与安全规则

字段归一化固定为：

- `title` 只由最终状态码决定：
  - 标准状态码取 `http.StatusText(status)`
  - `499` 固定为 `Client Closed Request`
  - 其他无标准文本的 `4xx` 回退为 `Client Error`
  - 其他无标准文本的 `5xx` 回退为 `Internal Server Error`
- `code`：
  - 若匹配到非 `nil` 的 `*errx.HTTPError`，直接取其归一化后的 `Code()`
  - 否则按 `errx` 对该最终状态码的默认 `code` 规则补 `code`
- `detail`：
  - 只有匹配到非 `nil` 的 `*errx.HTTPError` 时，才允许写出其归一化后的 `Detail()`
  - 对普通 `error`、`context.Canceled`、`context.DeadlineExceeded` 等框架合成错误，不写 `detail`
- `errors`：
  - 只有匹配到非 `nil` 的 `*errx.HTTPError` 时，才允许写出其归一化后的 `Errors()`
  - 对普通 `error`、`context.Canceled`、`context.DeadlineExceeded` 等框架合成错误，不写 `errors`

安全规则固定为：

- 不得从普通 `error` 泄漏 `err.Error()` 文本
- 不得泄漏栈、文件路径、私有类型名或内部实现细节
- 除 `code`、`detail` 与 `errors` 外，不得从 `*errx.HTTPError` 复制其他公开字段
- 对未匹配到 `*errx.HTTPError` 的错误，不得额外派生 `detail` 或 `errors`

### 4.4 首次提交、回退与返回值

返回与回退规则固定为：

- 若完整、合法的目标响应已成功写出，则返回 `nil`
- 必须先在内存里构造好目标 payload，再执行首次提交
- 成功入口不得在内部隐式调用 `WriteError`
- 对普通 `error`、`context.Canceled`、`context.DeadlineExceeded` 等框架合成错误，目标响应本身就是最小 Problem，不包含 `detail` 或 `errors`
- 最小内部错误回退只允许由“Problem payload 在首次提交前无法被 JSON 编码”触发
- 该回退固定写出：
  - `status = 500`
  - `title = "Internal Server Error"`
  - `code = "internal_error"`
  - 不包含 `detail`
  - 不包含 `errors`
- 若该最小内部错误 Problem 成功写出，`WriteError` 仍返回 `nil`
- 若首次提交后发生底层写失败，返回写失败 error，且不得尝试改写已开始的响应
- 若把“已开始的响应写回失败”再次传给 `WriteError`，必须返回同一个 error，且不得写任何内容

## 5. 测试基线

后续实现或重构至少应锁住：

- `JSON(..., 200, obj)` 写出 `200`、`application/json`，且响应体语义与输入一致
- `OK(...)` 写出 `200`
- `Created(...)` 写出 `201`
- 数组、布尔值、数字、字符串和 `nil` 都按其 JSON 本体写出，而不是被包进 envelope
- 无关自定义头部会保留
- 冲突的 `Content-Type` 会被覆盖
- 预设的 `Content-Length` 不会原样穿透
- payload 不可序列化时，成功入口返回 error 且不得发生首次提交
- `JSON` 只接受 `200`、`201`、`202`
- `JSON` 收到 `203`、`204`、`205`、`206`、`207`、`208`、`226` 或其他未支持状态码时，必须在首次提交前返回 error
- `NoContent()` 写出 `204`、空响应体、空 `Content-Type`、空 `Content-Length`
- 除 `WriteError(nil, nil)` 外，任一公开入口在 `w == nil` 时都返回 error 且不写内容
- `WriteError(errxHTTPError404)` 写出 `404`、`application/problem+json`、`title=Not Found`、`status=404`、`code=not_found`
- `WriteError` 会保留来自 `*errx.HTTPError` 的非空 `detail`
- `WriteError` 会保留来自 `*errx.HTTPError` 的 `code`
- `WriteError` 会保留来自 `*errx.HTTPError` 的 violations 顺序与内容
- `errors[]` 的稳定 JSON 字段固定为 `field` / `in` / `code` / `detail`
- `WriteError(context.Canceled)` 写出 `499`、`Client Closed Request`、`code=client_closed_request`，且不写 `detail` / `errors`
- `WriteError(context.DeadlineExceeded)` 写出 `504`、`Gateway Timeout`、`code=timeout`，且不写 `detail` / `errors`
- `WriteError` 对普通错误写出最小 `500` Problem，且不写 `detail` / `errors`，也不泄漏内部错误文本
- `WriteError(nil)` 是 no-op
- Problem payload 无法编码时，`WriteError` 回退为最小内部错误 Problem
- 若输入本身表示一次已开始的响应写回失败，`WriteError` 返回同一个 error 且不再写回
- 错误链同时匹配 `*errx.HTTPError` 与 `context` 错误时，优先按 `*errx.HTTPError` 收敛
- typed-nil `*errx.HTTPError` 不算匹配到公共 HTTP 错误；此时继续按 `context` 错误或默认 `500` 收敛
- 若候选状态码不属于 `499` 或 `400..599`，`WriteError` 必须先把状态码收敛为 `500`
- 任一失败场景都不得产生“前半段成功 JSON、后半段错误 JSON”的混杂响应
- 首次提交后的底层写失败只要求返回错误，不要求响应可回退
