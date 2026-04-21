# hah 共享 HTTP 错误模型设计方案

- 状态：Locked
- 版本：v4
- 锁定日期：2026-04-20
- 适用范围：
  - `hah.HTTPError`
  - `hah.FieldError`
  - `hah` 暴露的错误构造器、快捷构造器与共享常量
  - `reqx` / `resp` 对 `internal/errx` 的仓库内协作依赖
- 不覆盖：
  - 请求输入规则
  - 业务错误分类
  - 默认错误 envelope 写回
- 关联文档：
  - `docs/testing-standards.md`
  - `docs/binding-query-design.md`
  - `docs/binding-body-design.md`
  - `docs/query-design.md`
  - `docs/path-design.md`
  - `docs/resp-design.md`
- 变更规则：任何根包公开行为变化，必须先改本文档并补黑盒测试；若调整 `internal/errx` 的仓库内协作依赖，也必须先更新本文档并补相关包测试。

## 1. 包定位

当前仓库通过根包 `hah` 暴露共享的公共 HTTP 错误模型，`internal/errx` 只是该模型的内部实现与仓库内协作载体。
本文档优先锁定根包 `hah` 的公开契约；只有当 `reqx` / `resp` 明确依赖某些 `internal/errx` 行为时，才额外记录为仓库内协作约束。
除 `hah.go` 明确暴露的符号外，`internal/errx` 的导出项不构成对外公开 API。

这里提到的 `HTTPError` / `FieldError`，如无特殊说明，均指根包 `hah` 暴露给调用方的公开类型。
它不限定错误必须在 handler 层构造；只要某一层已经明确决定该错误可以直接公开给客户端，就可以返回 `hah.HTTPError`。
如果错误仍属于内部业务语义，则应继续保留普通 error 或内部错误类型，并在 HTTP 边界再映射。

该错误模型对外负责：

- 定义 `HTTPError`
- 定义 `FieldError`
- 标准化 `status`、`code`、`title`、`detail`
- 提供可保留或不保留 `cause` 的基础构造器
- 提供一组常用状态的快捷构造器
- 提供项目级共享 field error 常量

该错误模型对外不负责：

- request-side 默认文案
- `invalid_request` 包络
- 业务错误枚举
- 内部业务错误到公共 HTTP 错误的映射时机
- 默认错误 envelope 写回

仓库内另外约束：

- `reqx` 会依赖 `internal/errx` 的共享错误模型生成 request-side 错误
- `resp` 会依赖共享错误模型消费 `status` / `code` / `title` / `detail` / `errors`
- 这些依赖属于仓库内协作约束，不等同于对外公开 API

## 2. 根包 `hah` 的稳定公开契约

### 2.1 公开类型、构造器与常量

根包 `hah` 必须公开：

- `HTTPError`
- `FieldError`
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
- `CodeInvalid`
- `CodeRequired`
- `CodeUnknown`
- `CodeType`
- `CodeMultiple`
- `InBody`
- `InQuery`
- `InPath`
- `InHeader`

两种构造器的公共字段标准化必须完全一致，差别只在是否保留 `cause`。

`hah` 不公开：

- `FieldErrorCode`
- `FieldErrorIn`

### 2.2 `hah.HTTPError` 公开方法

`hah.HTTPError` 必须公开：

- `Error() string`
- `Unwrap() error`
- `Status() int`
- `Code() string`
- `Title() string`
- `Detail() string`
- `Errors() []FieldError`
- `WithFieldErrors([]FieldError) *HTTPError`

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

- `Detail()` 等于 `Code()` 的确定性可读化结果

规则：

- `detail` 只做 trim
- 默认 `detail` 只允许基于公开、稳定、可对外暴露的 `code` 生成
- 将 `snake_case` / `kebab-case` 中的 `_` / `-` 统一替换为空格
- 合并连续分隔符并输出为小写短语
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
- `Detail() == "internal error"`
- `Errors() == nil`
- `Unwrap() == nil`
- `Error() == "internal error"`

`nil` 的 `*HTTPError` receiver 不属于公开契约。
调用方不得依赖其任何方法行为；实现可以直接 panic。

## 4. `FieldError` 与 `WithFieldErrors(...)`

### 4.1 `FieldError` 字段语义

`FieldError` 公开字段只包含：

- `Field`
- `In`
- `Code`
- `Detail`

共享错误模型只定义 `FieldError` 的公开字段和值承载语义；
若这些字段被写入默认错误 envelope，由 `resp` 契约定义输出字段与包络。

共享错误模型对 `FieldError` 只做承载，不负责：

- 自动补默认 `Code`
- 自动补默认 `Detail`
- 自动生成 request-side 包络

### 4.2 `WithFieldErrors(...)`

`WithFieldErrors(...)` 的固定公开结果为：

- receiver 必须是非 `nil` 的 `*HTTPError`
- 返回新的 `*HTTPError`，且不修改 receiver
- 除 `field errors` 外，必须保留 receiver 的 `Status()` / `Code()` / `Title()` / `Detail()` / `Unwrap()` 公开语义
- 返回值中的 field errors 必须完全替换 receiver 当前的 field errors；不得 merge、append 或保留 receiver 的旧 field errors
- `nil` 或空切片输入表示“无 field errors”，即 `Errors() == nil`
- `Errors()` 按提供顺序暴露 field error 列表，不排序、不去重、不重写单个 `FieldError`
- 后续对入参切片或 `Errors()` 返回结果的修改，都不得影响错误对象内部保存的 field errors

## 5. 仓库内协作约束

- `reqx` 负责决定 request-side 的默认 detail、包络和 field error 内容。
- `reqx` 当前会直接使用 `internal/errx.FieldErrorCode` 与 `internal/errx.FieldErrorIn` 组装内部错误；这是仓库内协作细节，不是对外公开 API。
- `resp` 只能通过 `Status()` / `Code()` / `Title()` / `Detail()` / `Errors()` 消费共享错误模型；其中 `Status()` / `Code()` / `Detail()` 会被直接映射到默认错误 envelope 的 `status` / `reason` / 顶层 `message`。
- `resp` 若需要参与错误链，只能用 `errors.Is` / `errors.As`，不能读内部字段。

## 6. 测试基线

黑盒测试应直接覆盖第 2 到第 4 节定义的根包公开契约，不在本节重复写一份完整规格。
第 5 节的仓库内协作约束由相关包测试和集成测试覆盖，不作为对外黑盒契约。

至少额外锁住以下容易回归的边界：

- 根包公开面存在：`HTTPError`、`FieldError`、两个基础构造器、八个快捷构造器、`HTTPError` 的公开方法、全部 `Code*` / `In*` 常量
- 状态码与默认文案收敛规则：非错误状态码统一回落到 `500`，`499` 原样保留，默认 `title` / `code` / `detail` 与表格一致
- `cause` 语义：`NewHTTPError(...)` 不保留 `cause`，`NewHTTPErrorWithCause(...)` 保留 `cause`，typed-nil `cause` 归一化为 `nil`，`Error()` 不泄漏 `cause` 文本，`errors.Is` / `errors.As` 能通过 `Unwrap()` 正常工作
- `WithFieldErrors(...)` 语义：不修改 receiver、替换而不是追加旧 field errors、且不受入参切片和 `Errors()` 返回结果后续修改影响
- 零值 `HTTPError` 的全部 getter 与 `Error()` 语义
