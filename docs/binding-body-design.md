# hah BindBody 设计方案

- 状态：Locked
- 版本：v12
- 锁定日期：2026-04-20
- 适用范围：
  - `hah.BindBody(...)`
  - `reqx.BindBody(...)`
- 不覆盖：
  - `BindQuery(...)`
  - 业务校验
  - 响应写回
- 关联文档：
  - `docs/testing-standards.md`
  - `docs/errx-design.md`
  - `docs/resp-design.md`
- 变更规则：任何公开行为变化，必须先改本文档并补黑盒测试。

## 1. 设计目标

`BindBody(...)` 是面向 JSON API 的默认 body binder。
它只解决一件事：把 request body 绑定到 DTO。

本文只锁公开契约，不锁具体实现步骤。

公开保证聚焦于：

- 保持 `net/http`-first
- 复用标准库 `encoding/json` 语义
- 失败不污染 target
- 让 body 输入边界和错误收敛保持稳定、可预测

## 2. 心智模型

### 2.1 body 输入模型

- body 只按 request 当前可见的字节语义解释
- 零字节 body 是 no-op
- 非空 body 按 JSON body 输入处理

### 2.2 target 提交语义

`BindBody(...)` 的 target 只支持非 `nil` 的普通 `*struct`。

公开效果固定为：

- 成功时，target 表示本次 body 的完整解码结果
- 失败时，target 保持调用前状态
- JSON 缺失字段不会继承 target 旧值

## 3. 公开契约

### 3.1 target

公开支持的 target 只有：

- 非 `nil` 的 `*struct`
- 根 DTO 不自定义 `UnmarshalJSON`

以下场景都属于 usage error：

- `nil request`
- `nil target`
- typed-nil target
- 非指针 target
- 指向非 `struct` 的指针
- 根 DTO 自己实现 `UnmarshalJSON`

### 3.2 body 存在性

body 存在性的规则固定为：

- 零字节 body：no-op，target 不变
- 仅空白字符的 body：`400 invalid_json`
- 顶层 `null`：`400 invalid_json`

补充规则：

- 零字节 body 不要求 `Content-Type` 为 JSON

### 3.3 非空 body 的输入模型

一旦 body 非空，规则固定为：

- 必须且只能提供一个 `Content-Type`
- 该 `Content-Type` 的主媒体类型必须是 `application/json`
- `charset=utf-8` 之类的媒体类型参数不影响匹配
- 默认大小限制为 `1 MiB` 原始字节
- 顶层值必须是单个 JSON object
- 文档前后允许空白
- 未知字段默认拒绝

错误分类固定为：

- 非空 body 但媒体类型不是 JSON：`415 unsupported_media_type`
- body 超过 `1 MiB`：`413 request_too_large`
- JSON 文档不合法或不符合本文输入模型：`400 invalid_json`

以下输入都属于 `400 invalid_json`：

- 空白 body
- 顶层 `null`
- 顶层 array、string、number、boolean
- 截断 JSON
- 尾随非空白数据
- 多个 top-level JSON 值
- 带 UTF-8 BOM 的 body
- 未知字段
- 标准 JSON 类型不匹配
- 数值溢出

本文只锁输入边界和错误分类，不额外锁定大小检查、媒体类型校验与 JSON 解码之间的内部先后顺序。

### 3.4 字段解码语义

除本文明确列出的额外约束外，字段发现与解码直接跟随 Go `1.25.9` 的 `encoding/json`。

这包括：

- 命名类型按标准库规则解码
- `json.RawMessage` 按标准库规则解码
- 字段级自定义 `UnmarshalJSON` / `UnmarshalText` 按标准库规则解码
- 嵌入字段提升与遮蔽按标准库规则处理
- 同名 JSON object key 按标准库语义处理，后值覆盖前值

`BindBody(...)` 只额外施加三条 binder 级约束：

- 顶层 target 必须是受支持的 `*struct`
- 顶层值必须是单个 JSON object
- 未知字段默认拒绝

## 4. 错误模型

### 4.1 usage error

以下场景返回普通 error：

- `nil request`
- 非法 target

### 4.2 客户端输入错误

以下场景返回稳定 `*hah.HTTPError`：

- 非空 body 但媒体类型不是 JSON：`415 unsupported_media_type`
- body 超过 `1 MiB`：`413 request_too_large`
- JSON 文档不合法或不符合本文输入模型：`400 invalid_json`

### 4.3 底层读取失败

以下场景返回普通 error：

- `Body.Read` 包装错误
- transport I/O error
- `context` cancellation / deadline

## 5. 业务校验边界

`BindBody(...)` 只负责 JSON body 到 DTO 的绑定。

以下判断不属于 binder 默认职责：

- 某个 endpoint 是否要求客户端必须显式提交 body
- 零字节 body 是否要升级成业务错误
- 绑定后字段之间的业务约束

## 6. 测试基线

后续实现或重构至少应锁住：

- 合法 target 仅限非 `nil`、根 DTO 不自定义 `UnmarshalJSON` 的 `*struct`
- 非法 `request` / `target` 返回 usage error
- 零字节 body 是 no-op，且不修改 target
- 非空 body 的媒体类型、大小上限和错误码收敛保持稳定
- 顶层必须是单个 JSON object；空白 body、顶层 `null`、非 object 顶层值、尾随数据都返回 `400 invalid_json`
- 未知字段、标准字段类型不匹配、数值溢出返回 `400 invalid_json`
- `json.RawMessage`、字段级自定义 decoder、嵌入字段遮蔽、同名 key 后值覆盖等行为跟随标准库
- 失败时 target 保持不变
