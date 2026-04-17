# hah errx 设计方案

- 状态：Locked
- 版本：v3
- 锁定日期：2026-04-18
- 适用范围：
  - `errx`
  - `reqx` 对 `errx` 的错误生产依赖
  - `resp` 对 `errx` 的错误消费依赖
- 不覆盖：
  - 请求输入规则
  - 业务错误分类
  - Problem JSON 写回
- 关联文档：
  - `docs/testing-standards.md`
  - `docs/binding-query-design.md`
  - `docs/binding-body-design.md`
  - `docs/query-design.md`
  - `docs/path-design.md`
  - `docs/resp-design.md`
- 变更规则：任何公开行为变化，必须先改本文档并补黑盒测试。

## 1. 包定位

`errx` 是仓库共享的公共 HTTP 错误模型。
它只负责表达稳定、可公开返回、可组合的 HTTP 错误语义。

`errx` 负责：

- 定义 `HTTPError`
- 定义 `Violation`
- 标准化 `status`、`code`、`title`、`detail`
- 提供可保留或不保留 `cause` 的基础构造器
- 提供项目级共享 violation 常量

`errx` 不负责：

- request-side 默认文案
- `invalid_request` 包络
- 业务错误枚举
- Problem JSON 写回

## 2. 稳定公开 API

### 2.1 公开类型与构造器

必须导出：

- `HTTPError`
- `Violation`
- `ViolationCode`
- `ViolationIn`
- `NewHTTPError(status int, code, detail string) *HTTPError`
- `NewHTTPErrorWithCause(status int, code, detail string, cause error) *HTTPError`
- `BadRequest(code, detail string) *HTTPError`
- `Unauthorized(code, detail string) *HTTPError`
- `Forbidden(code, detail string) *HTTPError`
- `NotFound(code, detail string) *HTTPError`
- `MethodNotAllowed(code, detail string) *HTTPError`
- `Conflict(code, detail string) *HTTPError`
- `UnprocessableEntity(code, detail string) *HTTPError`
- `TooManyRequests(code, detail string) *HTTPError`

两种构造器的公共字段标准化必须完全一致，差别只在是否保留 `cause`。

### 2.2 `HTTPError` 公开方法

`HTTPError` 必须公开：

- `Error() string`
- `Unwrap() error`
- `Status() int`
- `Code() string`
- `Title() string`
- `Detail() string`
- `Errors() []Violation`
- `WithViolations([]Violation) *HTTPError`

### 2.3 共享常量

必须导出以下 `ViolationCode`：

- `CodeInvalid`
- `CodeRequired`
- `CodeUnknown`
- `CodeType`
- `CodeMultiple`

必须导出以下 `ViolationIn`：

- `InBody`
- `InQuery`
- `InPath`
- `InHeader`

## 3. `HTTPError` 公开语义

### 3.1 公开结果

`HTTPError` 的公开契约只锁定：

- `Status()`
- `Code()`
- `Title()`
- `Detail()`
- `Error()`
- `Unwrap()`
- `Errors()`

对外不锁定内部标准化步骤、计算顺序或字段存储方式。

### 3.2 状态码

最终公开状态码规则固定为：

- `400` 到 `599` 原样保留
- `499` 显式允许并原样保留
- 其他值统一收敛为 `500`

### 3.3 `Title()`

`Title()` 只由最终状态码决定：

| 最终状态码                          | `Title()`                        |
| ----------------------------------- | -------------------------------- |
| `499`                               | `Client Closed Request`          |
| 其他存在 `http.StatusText` 的状态码 | 对应的 `http.StatusText(status)` |
| 其他 4xx                            | `Client Error`                   |
| 其他 5xx                            | `Internal Server Error`          |

### 3.4 `Code()`

若调用方提供的 `code` 经 trim 后非空，则 `Code()` 返回该值。
否则按下表补默认值：

| 最终状态码 | 默认 `code`             |
| ---------- | ----------------------- |
| `400`      | `bad_request`           |
| `401`      | `unauthorized`          |
| `403`      | `forbidden`             |
| `404`      | `not_found`             |
| `405`      | `method_not_allowed`    |
| `409`      | `conflict`              |
| `422`      | `unprocessable_entity`  |
| `429`      | `too_many_requests`     |
| `499`      | `client_closed_request` |
| `503`      | `service_unavailable`   |
| `504`      | `timeout`               |
| 其他 4xx   | `client_error`          |
| 其他 5xx   | `internal_error`        |

规则：

- `code` 只做 trim
- 不做大小写转换或额外格式修正

### 3.5 `Detail()`

若调用方提供的 `detail` 经 trim 后非空，则 `Detail()` 返回该值。
否则：

- `Detail() == Title()`

规则：

- `detail` 只做 trim
- `detail` 不得从 `cause` 派生

### 3.6 `Error()` 与 `Unwrap()`

`Error()` / `Unwrap()` 的规则固定为：

- `Error()` 必须始终返回 `Detail()`
- `Error()` 不得泄漏 `cause` 文本
- 有 `cause` 时，`Unwrap()` 必须原样返回原始 `cause`
- typed-nil `cause` 必须归一化为 `nil`
- 无 `cause` 时，`Unwrap()` 返回 `nil`

### 3.7 零值语义

零值 `HTTPError` 的公开语义等价于：

- `Status() == 500`
- `Code() == "internal_error"`
- `Title() == "Internal Server Error"`
- `Detail() == "Internal Server Error"`
- `Errors() == nil`
- `Unwrap() == nil`
- `Error() == "Internal Server Error"`

`nil` 的 `*HTTPError` receiver 不属于公开契约。

## 4. `Violation` 与 `WithViolations(...)`

### 4.1 `Violation` 字段语义

`Violation` 公开字段只包含：

- `Field`
- `In`
- `Code`
- `Detail`

`errx` 只定义 `Violation` 的公开字段和值承载语义；
若这些字段被写入 Problem JSON，由 `resp` 契约定义输出字段与包络。

`errx` 对 `Violation` 只做承载，不负责：

- 自动补默认 `Code`
- 自动补默认 `Detail`
- 自动生成 request-side 包络

### 4.2 `WithViolations(...)`

`WithViolations(...)` 的规则固定为：

- 返回新的 `*HTTPError`
- 不修改 receiver
- 除 `violations` 外，必须保留 receiver 的 `Status()` / `Code()` / `Title()` / `Detail()` / `Unwrap()` 公开语义
- 返回值中的 violations 必须完全替换 receiver 当前的 violations；不得 merge、append 或保留 receiver 的旧 violations
- 立即拷贝入参切片
- `Errors()` 必须返回 defensive copy
- `nil` 或空切片输入都统一表现为 `nil`
- 保留输入顺序
- 保留重复项
- 不排序、不去重、不过滤、不重写单个 `Violation`

## 5. 与其他包的关系

- `reqx` 负责决定 request-side 的默认 detail、包络和 violation 内容。
- `resp` 只能通过 `Status()` / `Code()` / `Title()` / `Detail()` / `Errors()` 消费 `errx`，并可把 `Code()` 暴露为顶层 Problem JSON 的 `code` 字段。
- `resp` 若需要参与错误链，只能用 `errors.Is` / `errors.As`，不能读内部字段。

## 6. 测试基线

后续实现或重构至少应锁住：

- `HTTPError`、`Violation`、`ViolationCode`、`ViolationIn` 类型存在
- 两个构造器存在
- 八个快捷构造器存在
- `HTTPError` 的八个公开方法存在
- 全部 `Code*` / `In*` 常量存在
- 非错误状态码统一回落到 `500`
- `499` 原样保留
- 默认 `title` 规则与表格一致
- 默认 `code` 规则与表格一致
- 空白 `detail` 回落到 `Title()`
- 显式 `code` / `detail` 只做 trim
- `NewHTTPError(...)` 不保留 `cause`
- `NewHTTPErrorWithCause(...)` 保留 `cause`
- `Unwrap()` 除 typed-nil 归一化外原样返回原始 `cause`
- `Error()` 始终等于 `Detail()`
- `Error()` 不泄漏 `cause` 文本
- `errors.Is` / `errors.As` 能通过 `Unwrap()` 正常工作
- typed-nil `cause` 被归一化为 `nil`
- `WithViolations(...)` 不修改 receiver
- `WithViolations(...)` 返回值会保留 receiver 的 `Status()` / `Code()` / `Title()` / `Detail()` / `Unwrap()` 公开语义
- `WithViolations(...)` 会替换而不是追加 receiver 的旧 violations
- 传入切片被修改时，错误对象不受影响
- `Errors()` 返回结果被修改时，错误对象不受影响
- `nil` / 空切片统一表现为 `nil`
- 输入顺序保持不变
- 重复项被原样保留
- `WithViolations(...)` 不会重写单个 violation 的字段
- 零值 `HTTPError` 的全部 getter 与 `Error()` 语义
