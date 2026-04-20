# hah resp 设计方案

- 状态：Locked
- 版本：v7
- 锁定日期：2026-04-20
- 适用范围：
  - `hah.JSON`
  - `hah.OK`
  - `hah.Created`
  - `hah.NoContent`
  - `hah.WriteError`
- 不覆盖：
  - router / handler 生命周期
  - 业务 envelope
  - 内容协商
  - panic recover
- 关联文档：
  - `docs/testing-standards.md`
  - `docs/errx-design.md`
- 变更规则：任何公开行为变化，必须先改本文档并补黑盒测试。

## 1. 设计目标

根包 `hah` 的响应入口只负责 HTTP 响应写回。
它默认只提供两类输出语义：

- 成功写回：`application/json` 或 `204 No Content`
- 错误响应：`application/problem+json`

目标是保持响应边界简单、稳定、可预测：

- `JSON` 只按调用方给定状态写 JSON 响应
- `OK` / `Created` / `NoContent` 只是成功写回快捷入口
- `WriteError` 只把错误收敛成公开 Problem JSON
- 不做内容协商
- 不发明 envelope
- 不承担业务错误分类

## 2. 公开入口

公开入口固定为五个：

- `JSON(w http.ResponseWriter, status int, data any) error`
- `OK(w http.ResponseWriter, data any) error`
- `Created(w http.ResponseWriter, data any) error`
- `NoContent(w http.ResponseWriter) error`
- `WriteError(w http.ResponseWriter, err error) error`

## 3. 通用契约

稳定规则固定为：

- 除 `WriteError(nil, nil)` 外，`w == nil` 时返回 error，且不得写出内容
- `WriteError(nil, nil)` 是纯 no-op：返回 `nil` 且不得写出任何内容
- `hah` 的响应写回入口只拥有 `Content-Type` 与 `Content-Length` 的所有权
- 无关自定义头部必须保留
- 冲突的 `Content-Type` 必须覆盖
- 预设的 `Content-Length` 不得原样穿透
- `HEAD` 请求沿用 `net/http` 默认语义
- 不负责恢复调用方自定义 `MarshalJSON`、`Error()` 或包装 `ResponseWriter` 实现中的 panic
- 首次提交前必须完成所有可能失败的校验与编码；预提交失败不得写出半成品响应
- 首次提交后若底层写失败，返回写失败 error，且不得尝试改写已开始的响应

## 4. 成功写回

### 4.1 `JSON` / `OK` / `Created`

成功 JSON 写回契约固定为：

- `JSON` 接受常规 HTTP 语义下允许携带响应体的状态码
- `OK` 固定写 `200`
- `Created` 固定写 `201`
- 主媒体类型必须是 `application/json`
- 响应体是调用方提供值的直接 JSON 表达
- `nil` payload 编码为 JSON `null`
- JSON 序列化语义跟随 `encoding/json`
- 不支持的状态码必须在首次提交前返回 error
- payload 不可 JSON 编码时必须在首次提交前返回 error
- `JSON` / 成功快捷入口不得隐式回退成 `500` Problem 响应

这里的“不支持”固定包括：

- `1xx` 状态码
- `204 No Content`
- `205 Reset Content`
- `304 Not Modified`

### 4.2 `NoContent`

`NoContent()` 的契约固定为：

- 写出 `204 No Content`
- 不写响应体
- 清除 `Content-Type`
- 清除 `Content-Length`
- 保留无关自定义头部

## 5. 错误写回

### 5.1 `WriteError` 收敛规则

`WriteError` 只做一层简单收敛，优先级固定为：

1. `err == nil`：返回 `nil`，不写任何内容
2. `w == nil`：返回 error，且不得写任何内容
3. 若错误链里存在 `*hah.HTTPError`：直接按该公开错误写回
4. `errors.Is(err, context.Canceled)`：写回 `499`
5. `errors.Is(err, context.DeadlineExceeded)`：写回 `504`
6. 其他情况：写回最小 `500` Problem

这里只锁最终优先级，不锁内部解释步骤。

### 5.2 Problem JSON

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

- `code` 必须写出
- `detail` 仅在公开错误对象本身提供非空 detail 时写出
- `errors` 仅在公开错误对象本身提供非空 violations 时写出
- `errors` 的稳定 JSON 字段固定为 `field` / `in` / `code` / `detail`
- 响应写回入口不排序、不去重、不改写 `errors`

### 5.3 框架合成错误

对 `context.Canceled`、`context.DeadlineExceeded` 与普通 `error` 这类框架合成错误：

- `title` 由最终状态码决定
- `code` 使用 `hah` 公开错误模型的默认 `code`
- 不写 `detail`
- 不写 `errors`
- 不得泄漏原始 `err.Error()` 文本

## 6. 测试基线

后续实现或重构至少覆盖以下代表性黑盒基线；其余细节直接跟随上文稳定契约，不再重复列举：

### 6.1 成功写回

- `JSON(..., 200, obj)`、`OK(...)`、`Created(...)` 的基本成功路径
- `JSON(..., 203, obj)` 与 `JSON(..., 302, obj)` 这类允许携带 body 的非常规状态
- 数组、布尔值、数字、字符串和 `nil` 的直接 JSON 表达
- `NoContent()` 写出 `204`、空响应体、空 `Content-Type`、空 `Content-Length`

### 6.2 失败与边界

- `JSON` 拒绝 `1xx`、`204`、`205`、`304` 与非法状态码
- payload 不可序列化时，`JSON` / 成功快捷入口返回 error 且不得发生首次提交
- 除 `WriteError(nil, nil)` 外，任一公开入口在 `w == nil` 时都返回 error 且不写内容
- 无关自定义头部会保留，冲突的 `Content-Type` 会被覆盖，预设的 `Content-Length` 不会原样穿透

### 6.3 错误收敛

- `WriteError(hahHTTPError404)` 按公开错误模型写回，并保留 `detail`、`code` 与 violations 顺序
- `errors[]` 的稳定 JSON 字段固定为 `field` / `in` / `code` / `detail`
- `WriteError(context.Canceled)` 写出 `499`，且不写 `detail` / `errors`
- `WriteError(context.DeadlineExceeded)` 写出 `504`，且不写 `detail` / `errors`
- `WriteError` 对普通错误写出最小 `500` Problem，且不写 `detail` / `errors`
- 错误链同时匹配 `*hah.HTTPError` 与 `context` 错误时，优先按 `*hah.HTTPError` 收敛

### 6.4 写回失败

- `WriteError(nil)` 是 no-op
- Problem payload 形状固定且必须保持可 JSON 编码；`WriteError` 不暴露单独的 problem 编码失败分支
- 任一失败场景都不得产生“前半段成功 JSON、后半段错误 JSON”的混杂响应
- 首次提交后的底层写失败只要求返回错误，不要求响应可回退
