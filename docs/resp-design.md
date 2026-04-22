# hah 响应协议设计方案

- 状态：Locked
- 版本：v19
- 起草日期：2026-04-20
- 适用范围：
  - 根包 `hah` 面向业务 JSON API 的默认响应协议
  - `hah.OK`
  - `hah.Created`
  - `hah.WriteError`
- 不覆盖：
  - `hah.JSON` 的底层 raw JSON escape hatch
  - router / handler 生命周期
  - 内容协商
  - panic recover
- 关联文档：
  - `docs/testing-standards.md`
  - `docs/errx-design.md`
- 变更规则：任何公开行为变化，必须先改本文档并补黑盒测试。

## 1. 设计目标

根包 `hah` 对外的默认响应协议，目标从“只保证 HTTP-first 写回”调整为“HTTP 状态码 + 统一 JSON envelope”：

- 前端与 SDK 不再依赖成功响应的 body 形状判断结果
- 所有带 body 的默认响应都收敛为顶层 JSON 对象
- 顶层固定暴露 `code` 与 `message`
- 成功与失败分别通过 `data` / `error` 承载具体内容
- HTTP 状态码仍保持正常语义，不采用“永远 200”
- 默认带 body 的成功协议使用统一 JSON envelope；若调用方显式选择 `NoContent`，可写 `204 No Content`

本文档锁定的是默认业务协议，而不是最底层 JSON 写回能力。
若最终继续保留 `hah.JSON`，它只应作为不带 envelope 的底层逃生口，不反向约束本文定义的默认协议。
在 `v1` 之前，响应协议不承诺兼容旧设计；如有必要，可以直接删除与本文冲突的旧入口和旧行为。
本文当前版本自本次起视为 `Locked` 契约；后续任何公开行为变化都必须先更新本文并补黑盒测试。

## 2. 公开入口

默认业务响应协议对外固定提供以下入口：

- `OK(w http.ResponseWriter, data any) error`
- `Accepted(w http.ResponseWriter, data any) error`
- `Created(w http.ResponseWriter, data any) error`
- `NoContent(w http.ResponseWriter) error`
- `WriteError(w http.ResponseWriter, err error, code ...int) error`

稳定规则：

- `OK` 固定写 `200 OK`
- `Accepted` 固定写 `202 Accepted`
- `Created` 固定写 `201 Created`
- `NoContent` 固定写 `204 No Content`
- `WriteError` 根据最终公开错误模型写出对应 `4xx` / `5xx`
- `WriteError` 在未传第三个参数时使用默认顶层 `code`
- `WriteError` 在传入单个第三参数时使用显式顶层 `code`
- `WriteError` 传入多个第三参数属于调用错误

`hah.JSON` 如继续保留，只作为 raw JSON escape hatch，不属于本文定义的默认业务协议。

## 3. 通用写回边界

默认业务响应协议的公开入口都必须遵守以下通用边界：

- `w == nil` 时必须返回 error，且不得写出任何内容
- `HEAD` 请求沿用 `net/http` 默认语义
- 默认协议拥有 `Content-Type` 与 `Content-Length` 的所有权
- 无关自定义头部必须保留
- 冲突的 `Content-Type` 必须覆盖
- 预设的 `Content-Length` 不得原样穿透
- 首次提交前必须完成全部校验与编码；预提交失败不得写出半成品响应
- 首次提交后若底层写失败，只返回 error，不尝试回滚或改写已开始的响应

`WriteError` 额外固定以下规则：

- `WriteError(w, nil)` 是 no-op
- `WriteError(w, nil, code)` 也是 no-op；可选 `code` 在 `err == nil` 时必须被忽略
- `WriteError(w, err)` 使用默认顶层 `code` 规则
- `WriteError(w, err, code)` 使用显式顶层 `code`
- `WriteError(w, err)` 在失败响应中必须最终收敛出非空 `error.reason`
- `WriteError(w, err, code1, code2, ...)` 必须在首次提交前返回 error
- `WriteError(w, err, code)` 中非五位或 `code <= 0` 的值必须在首次提交前返回 error
- `WriteError(w, err)` 会优先选择错误链中第一个可见的公共 `HTTPError`
- 若错误链中不存在公共 `HTTPError`，则依次按 `context canceled`、`context deadline exceeded`、通用 `internal error` 兜底

## 4. 总体模型

带 body 的默认响应协议固定为顶层 JSON 对象：

```go
type Response[T any] struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    *T         `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
}

type ErrorBody struct {
	Reason  string       `json:"reason"`
	Details []FieldError `json:"details,omitempty"`
}

type FieldError struct {
	Field  string `json:"field,omitempty"`
	In     string `json:"in,omitempty"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}
```

上面的 Go 类型只表达逻辑形状，不锁定具体零值编码技巧。
最终实现必须保证：

- 成功时 `data` 是可选字段
- 无 payload 的成功响应允许省略 `data`
- 成功时 `data` 是否省略或写出 `null` 不作为稳定契约
- 失败时不得写出 `data`
- 成功时不得写出 `error`

稳定规则：

- 所有带 body 的默认响应顶层都必须是 JSON object
- 顶层固定字段为 `code` 与 `message`
- 成功响应可写 `data`，且不得写 `error`
- 失败响应写 `error`，不得写 `data`
- 顶层 `code` 是整数业务码；失败时固定为五位业务错误码
- 嵌套 `error.reason` 是必填、非空的稳定字符串错误类型
- `message` 是给人看的短摘要，不用于程序分支判断
- 失败时顶层 `message` 直接来自共享错误模型的 `detail`，不作为独立输入
- `title` 不再属于公开 JSON 协议；人类可读摘要统一收敛到顶层 `message`
- 内部允许保留 `detail` 作为最具体的错误摘要字段
- 若底层错误模型仍有 `title`，它只属于内部兼容信息，不参与默认响应协议，也不参与 `message` 计算
- `FieldError.Code` 是字段级规则码，例如 `required` / `invalid` / `type`
- `FieldError.Detail` 是字段级错误提示

## 5. 成功响应

成功响应的稳定协议为：

- `OK` 固定使用 `200`
- `Accepted` 固定使用 `202`
- `Created` 固定使用 `201`
- `Content-Type` 为 `application/json`
- 顶层 `code` 固定为 `0`
- 顶层 `message` 固定为 `"success"`
- 有业务数据时写入 `data`
- 不写 `error`
- 无数据成功也可以继续写 envelope；只有显式调用 `NoContent` 时才写 `204`

示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "u_1"
  }
}
```

无数据成功也合法：

```json
{
  "code": 0,
  "message": "success"
}
```

规则：

- 成功时 `data` 是可选字段
- `data` 若存在，可以是对象、数组、标量或 `null`
- 无 payload 的成功响应可以省略 `data`
- 显式 `nil` payload 是否编码为 JSON `null` 不作为稳定契约
- 具体 JSON 序列化语义仍跟随 `encoding/json`
- `OK` 的无数据成功允许写 `200` + 不含 `data` 的 envelope
- `Accepted` 的无数据成功允许写 `202` + 不含 `data` 的 envelope
- `Created` 的无数据成功允许写 `201` + 不含 `data` 的 envelope
- 成功 envelope 不承担业务分页、trace、meta 等扩展字段；是否追加额外顶层字段，留待单独设计

显式无响应体成功的稳定协议为：

- `NoContent` 固定使用 `204`
- `NoContent` 不写响应体
- `NoContent` 不写 success envelope
- `NoContent` 在提交前必须清理冲突的 `Content-Type` 与 `Content-Length`
- `NoContent` 必须保留无关自定义头部

## 6. 失败响应

失败响应的稳定协议为：

- HTTP 状态码使用 `4xx` 或 `5xx`
- `Content-Type` 为 `application/json`
- 顶层 `code` 为非 `0` 的五位整数
- 顶层 `message` 为错误摘要，且固定等于共享错误模型的 `detail`
- 不写 `data`
- 具体错误内容写入 `error`

示例：

```json
{
  "code": 42200,
  "message": "request validation failed",
  "error": {
    "reason": "unprocessable_entity",
    "details": [
      {
        "field": "email",
        "in": "body",
        "code": "invalid",
        "detail": "must be a valid email"
      }
    ]
  }
}
```

`error` 对象的字段职责固定为：

- `reason`：必填、非空的稳定字符串错误类型，例如 `unprocessable_entity`、`timeout`、`not_found`
- `details`：字段级错误列表；仅在有 field errors 时写出

`detail` 仍允许保留在内部错误承载结构中，但不作为公开 `error` JSON 字段输出；它的公开投影固定为顶层 `message`。
若现有底层错误模型仍有 `title` 概念，它只属于内部兼容信息，不参与默认响应协议，也不参与 `message` 计算。

`details` 的稳定 JSON 字段固定为：

- `field`
- `in`
- `code`
- `detail`

规则：

- `error.reason` 与顶层 `code` 语义不同，不得混用
- 顶层 `code` 负责业务分支
- `error.reason` 负责稳定错误类型
- `details` 不排序、不去重、不改写顺序

## 7. 顶层 `code` / `message` 规则

### 7.1 成功默认值

成功时固定为：

- `code = 0`
- `message = "success"`

该规则不允许调用方覆盖。

### 7.2 失败 `code` 传入方式

失败时，调用方若要显式指定顶层 `code`，必须使用：

- `WriteError(w, err, code)`

约束：

- 显式失败 `code` 必须是五位正整数
- 显式失败 `code` 不得为 `0`
- 显式失败 `code` 必须满足 `10000 <= code <= 99999`
- 顶层 `message` 不允许作为独立输入
- `WriteError` 的第三参数只改变顶层 `code` 选择规则，不改变 HTTP 状态码、顶层 `message` 映射规则和 `error` payload 结构
- `WriteError(w, err)` 表示“不显式传入顶层 `code`，使用默认规则”
- `WriteError(w, err, code)` 表示“显式指定顶层 `code`”
- `WriteError(w, err, code1, code2, ...)` 必须在首次提交前返回 error

该 variadic 入口只用于承载单个可选顶层 `code`，不允许借此扩展其他可选参数。

### 7.3 失败 `message` 映射规则

失败时，顶层 `message` 固定等于共享错误模型的 `detail`。

调用方若要影响失败 `message`，应通过公开错误模型提供 `detail`；若未提供 `detail`，则由共享错误模型先基于公开、稳定、可对外暴露的 `reason` 标准化出默认 `detail`，`resp` 不再二次 humanize。

内部 `detail` 允许由调用方通过公开错误模型定义，但不作为公开 JSON 字段输出。

### 7.4 失败默认 `code`

若调用方使用 `WriteError(w, err)`，即未显式提供顶层 `code`，默认规则固定为：

- `code = status * 100`

示例：

| HTTP status | 默认顶层 `code` | 顶层 `message` 来源 |
| ----------- | --------------- | ------------------- |
| `400`       | `40000`         | `detail`            |
| `401`       | `40100`         | `detail`            |
| `404`       | `40400`         | `detail`            |
| `422`       | `42200`         | `detail`            |
| `500`       | `50000`         | `detail`            |

该规则只定义默认值，不限制业务方在此基础上自行细分，例如：

- `40101` 表示 token missing
- `40102` 表示 token invalid
- `42201` 表示 invalid json

### 7.5 顶层 `code` 保留约定

顶层 `code` 的保留规则固定为：

- `0` 保留给成功响应
- `40000` 到 `59999` 保留给默认 HTTP 错误映射
- 其他五位正整数可由业务方通过 `WriteError(w, err, code)` 显式传入使用

业务方若显式传入 `40000` 到 `59999` 区间内的值，应自行保证语义一致，不得制造与默认 HTTP 错误映射相冲突的歧义。

## 8. 与现有 `hah` 错误模型的映射

默认失败 envelope 复用 `hah` 已有公开错误模型，不重新发明第二套底层错误对象。

映射规则固定为：

- `error.reason` 对应 `hah.HTTPError.Code()`
- 顶层 `message` 直接对应 `hah.HTTPError.Detail()`
- `hah.HTTPError.Code()` 必须能提供非空 `reason`
- `hah.HTTPError.Title()` 不再映射到公开 JSON 字段，也不参与 `message` 计算；若当前模型仍保留 `Title()`，它只属于内部兼容信息

`WriteError(...)` 选择错误对象时，不做“最严重优先”“最深层优先”或“最后一个优先”的额外排序。
当前稳定规则是：沿错误链按可见顺序找到第一个公共 `HTTPError` 就使用它；只有当整条链都没有公共 `HTTPError` 时，才进入 `context` / `internal error` 兜底路径。
- `error.details` 对应 `hah.HTTPError.Errors()`

字段级错误直接复用 `hah.FieldError` 的 JSON 形状：

- `hah.FieldError.Field -> error.details[].field`
- `hah.FieldError.In -> error.details[].in`
- `hah.FieldError.Code -> error.details[].code`
- `hah.FieldError.Detail -> error.details[].detail`

对 `context.Canceled`、`context.DeadlineExceeded` 与普通 `error` 这类框架合成错误：

- 仍先按 `hah` 公开错误模型收敛出 `status` / `reason` / `detail`
- 顶层 `message` 固定直接使用该 `detail`
- 不得泄漏原始内部错误文本

## 9. HTTP 语义

默认协议仍遵循正常 HTTP 语义：

- `OK` 使用 `200`
- `Accepted` 使用 `202`
- `Created` 使用 `201`
- `NoContent` 在显式调用时使用 `204`
- 失败使用 `4xx` 或 `5xx`
- 不采用“所有业务错误都返回 `200`”
- 默认带 body 的成功协议不使用 `204 No Content`
- 无数据成功仍可返回 JSON envelope；`OK` 默认使用 `200`，`Accepted` 默认使用 `202`，`Created` 默认使用 `201`
- 失败时的顶层 `message` 只是 `error` 的摘要投影，不是额外独立错误源
- 内部 `detail` 可以保留，但对外 `error` JSON 不输出 `detail` / `title`
- 失败时顶层 `code` 要么显式来自 `WriteError` 的单个第三参数，要么默认来自 `WriteError` 的 `status * 100` 规则

## 10. 前端判断规则

前端与 SDK 的默认判断规则固定为：

1. 先看 HTTP 状态码
2. `2xx` 视为成功；若存在 `data`，则读取 `data`
3. 非 `2xx` 视为失败，读取 `error`
4. 业务分支优先看顶层 `code`
5. 细分错误类型看 `error.reason`
6. 字段级提示看 `error.details`

禁止：

- 依赖 `message` 做稳定程序分支
- 依赖成功 `data` 的 JSON 形状判断“请求是否成功”

## 11. 测试基线

后续实现或重构至少覆盖以下代表性黑盒基线：

- 成功响应固定写出 `code = 0` 与 `message = "success"`
- `OK` 固定写 `200`，`Accepted` 固定写 `202`，`Created` 固定写 `201`
- 成功有 payload 时写 `data`；无 payload 时允许省略 `data`
- 成功 `data` 若存在，支持对象、数组、标量与 `null`
- `NoContent` 固定写 `204`、清理冲突的 `Content-Type` / `Content-Length`，且响应体为空
- 任一公开入口在 `w == nil` 时都返回 error，且不得写内容
- `HEAD` 请求沿用 `net/http` 默认语义
- 默认协议拥有 `Content-Type` / `Content-Length` 所有权；无关头部保留，冲突 `Content-Type` 覆盖，预设 `Content-Length` 不原样穿透
- 首次提交前必须完成校验与编码；预提交失败不得写半成品响应
- 首次提交后底层写失败只返回 error，不回滚响应
- 失败响应固定写出非 `0` 顶层 `code`、顶层 `message` 与 `error`
- 失败 `message` 固定直接映射共享错误模型的 `detail`，不接受独立传入
- 未显式提供失败 `code` 时，按 `status * 100` 生成默认值
- `WriteError` 显式提供单个失败 `code` 时，优先使用调用方传入值
- `WriteError` 对非五位或 `code <= 0` 的失败 `code` 必须在首次提交前返回 error
- `WriteError` 传入多个 `code` 参数时，必须在首次提交前返回 error
- `WriteError(w, nil)` 与 `WriteError(w, nil, code)` 都是 no-op
- `error` 固定包含非空 `reason`，并按需包含 `details`
- `error.details` 的稳定 JSON 字段固定为 `field` / `in` / `code` / `detail`
- 调用方显式提供公开 `detail` 时，顶层 `message` 必须与之保持一致
- 共享错误模型未显式提供 `detail` 时，顶层 `message` 必须等于该模型标准化后的 `Detail()`
- 对外 `error` JSON 不输出 `detail` / `title`
- `context.Canceled`、`context.DeadlineExceeded` 与普通 `error` 不得泄漏内部错误文本
- 无数据成功固定返回 envelope，而不是 `204`
- 成功时 `data` 为可选字段；是否省略或写出 `null` 不作为稳定契约
