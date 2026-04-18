# hah BindBody 设计方案

- 状态：Locked
- 版本：v6
- 锁定日期：2026-04-18
- 适用范围：
  - `hah.BindBody(...)`
  - `reqx.BindBody(...)`
  - `hah.RequireBody(...)`
  - `reqx.RequireBody(...)`
- 不覆盖：
  - `BindQuery(...)`
  - 业务校验
  - 响应写回
- 关联文档：
  - `docs/testing-standards.md`
  - `docs/errx-design.md`
  - `docs/resp-design.md`
- 变更规则：任何公开行为变化，必须先改本文档并补黑盒测试。

## 1. 设计定位

`BindBody(...)` 是一个刻意收窄的 JSON body binder。
它只做四件事：

- 读取并缓存同一个 request 上的 body 字节
- 校验媒体类型与大小限制
- 用标准库 `encoding/json` 解码到临时 DTO
- 成功后一次性提交到 target

它不再维护额外的字段家族白名单，也不再试图在 decode 前复制一套 `encoding/json` 的字段发现规则。
字段解码语义直接跟随标准库。

## 2. 稳定公开契约

### 2.1 target 与 body 读取

公开支持的 target 只有：

| target 形状           | 是否支持 | 说明               |
| --------------------- | -------- | ------------------ |
| 非 `nil` 的 `*struct` | 是       | 默认 DTO bind target |

其他 target 一律是 usage error，包括：

- `nil request`
- `nil target`
- typed-nil target
- 非指针 target
- 指向非 `struct` 的指针

body 读取语义固定为：

- `BindBody(...)` / `RequireBody(...)` 在同一个 request 上共享已读取的 body 字节
- 可以在同一个 request 上按任意顺序组合调用 `BindBody(...)` 与 `RequireBody(...)`
- 本文档不承诺 clone / sibling request 共享同一份 body 缓存
- 默认大小限制固定为 `1 MiB` 原始字节
- 超过限制时返回 `413 request_too_large`
- body read failure 返回普通 error

### 2.2 零字节 body 与 `RequireBody(...)`

组合语义固定为：

| 输入              | `BindBody(...)`    | `RequireBody(...)` |
| ----------------- | ------------------ | ------------------ |
| 零字节 body       | no-op，target 不变 | 视为缺失 body      |
| 仅空白字符的 body | `400 invalid_json` | 视为存在 body      |
| 顶层 `null`       | `400 invalid_json` | 视为存在 body      |

补充规则：

- 零字节 body 的 no-op 发生在 `Content-Type` 检查之前
- 零字节 body 不要求 `Content-Type` 为 JSON

### 2.3 非空 body 的输入模型

一旦 body 非空，规则固定为：

- 只接受 `application/json`
- 媒体类型比较基于解析后的主媒体类型；参数如 `charset=utf-8` 不影响匹配
- 当前不支持 `application/*+json`
- 非空 body 必须恰好构成一个以 object 为顶层值的 JSON 文档
- 顶层 `null`、array、string、number、boolean 都返回 `400 invalid_json`
- 尾随非空白数据或多个 top-level JSON 值返回 `400 invalid_json`
- UTF-8 BOM 不做剥离，带 BOM 的 body 返回 `400 invalid_json`
- 默认拒绝未知字段

### 2.4 字段解码语义

`BindBody(...)` 不再维护独立字段白名单。
对 `*struct` target 的字段发现与解码规则直接跟随 Go `1.25.9` 的 `encoding/json`。

这意味着：

- 嵌入字段提升与遮蔽跟随标准库
- `json.RawMessage`、命名类型、自定义 `UnmarshalJSON` / `UnmarshalText` 类型按标准库语义工作
- 同名 JSON object key 跟随标准库语义，后值覆盖前值
- 字段类型与当前 JSON 输入不匹配时，返回 `400 invalid_json`

### 2.5 提交语义

对于公开支持的 `*struct` target：

- 解码必须先进入与 target 同构的零值临时对象
- 只有全部成功后，才允许一次性提交到 target
- 失败时，target 必须保持调用前状态
- JSON 缺失字段不会继承 target 旧值

## 3. 错误分类

错误分类固定为：

- usage error：
  - `nil request`
  - 非法 target
  - 返回普通 error
- 稳定客户端输入错误：
  - 不支持的媒体类型：`415 unsupported_media_type`
  - body 超限：`413 request_too_large`
  - 非法 JSON、顶层非 object、尾随数据、多个 top-level 值、未知字段、标准 JSON 类型不匹配：`400 invalid_json`
  - 返回稳定 `*errx.HTTPError`
- 底层读取失败：
  - `Body.Read` 包装错误
  - transport I/O error
  - `context` cancellation / deadline
  - 返回普通 error

冲突条件下的优先级固定为：

1. usage error
2. body read failure / request too large
3. 零字节 body no-op 或缺失 body
4. 非空 body 的媒体类型检查
5. JSON 解码与文档边界错误

## 4. 测试基线

后续实现或重构至少应锁住：

- 合法 target 仅限非 `nil` 的 `*struct`
- 非法 `request` / `target` 返回 usage error
- 零字节 body 是 no-op，且不修改 target
- 零字节 body 对 `RequireBody(...)` 是缺失 body
- 同一个 request 上 `BindBody(...)` / `RequireBody(...)` 可以按任意顺序组合
- `application/json`
- `application/json; charset=utf-8`
- 非空非 JSON `Content-Type` 返回 `415 unsupported_media_type`
- 大于 `1 MiB` 的 body 返回 `413 request_too_large`
- 恰好 `1 MiB` 的 body 仍允许进入 JSON 解码
- 空白 body、顶层 `null`、array、string、number、boolean 返回 `400 invalid_json`
- 截断 JSON、尾随数据、多个 top-level 值返回 `400 invalid_json`
- 未知字段、标准字段类型不匹配、数值溢出返回 `400 invalid_json`
- `json.RawMessage`、自定义 decoder、嵌入字段遮蔽等行为跟随标准库
- 同名 JSON object key 跟随标准库后值覆盖前值
- 失败时 target 保持不变
