# hah BindBody 设计方案

- 状态：Locked
- 版本：v7
- 锁定日期：2026-04-19
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

## 1. 设计目标

`BindBody(...)` 是面向 JSON API 的默认 body binder。
它提供一条直接、稳定、可组合的 body 处理路径：

1. 读取当前 request 的 body 字节
2. 在同一个 request 上缓存读取结果
3. 校验媒体类型与大小限制
4. 用标准库 `encoding/json` 解码到临时 DTO
5. 成功后一次性提交到 target

这套设计的重点是：

- 保持 `net/http`-first
- 复用标准库 JSON 语义
- 让 `BindBody(...)` 与 `RequireBody(...)` 可在同一个 request 上自然组合
- 用临时值提交保证 DTO 不会被失败路径污染

## 2. 心智模型

### 2.1 request 级 body 缓存

body 是 request 级输入。
`BindBody(...)` 与 `RequireBody(...)` 在同一个 request 上共享已经读取到的 body 字节。

公开语义固定为：

- 同一个 request 上，`BindBody(...)` / `RequireBody(...)` 可以按任意顺序组合调用
- body 只按 request 当前看到的字节语义解释
- 零字节 body 代表“没有提交 body”
- 非零字节 body 代表“客户端显式提交了 body”

### 2.2 DTO 绑定模型

`BindBody(...)` 的 target 固定为非 `nil` 的 `*struct`。

绑定过程固定为：

1. 先创建与 target 同构的零值临时对象
2. 把 body 解码到临时对象
3. 全部成功后一次性提交到 target

因此：

- 失败时 target 保持调用前状态
- JSON 缺失字段不会继承 target 旧值
- 成功时 target 表现为“本次 body 的完整投影结果”

## 3. 公开契约

### 3.1 target

公开支持的 target 只有：

| target 形状           | 是否支持 | 说明                |
| --------------------- | -------- | ------------------- |
| 非 `nil` 的 `*struct` | 是       | 默认 DTO bind target |

其他 target 都属于 usage error，包括：

- `nil request`
- `nil target`
- typed-nil target
- 非指针 target
- 指向非 `struct` 的指针

### 3.2 body 存在性

body 存在性的规则固定为：

| 输入              | `BindBody(...)`    | `RequireBody(...)` |
| ----------------- | ------------------ | ------------------ |
| 零字节 body       | no-op，target 不变 | 视为缺失 body      |
| 仅空白字符的 body | `400 invalid_json` | 视为存在 body      |
| 顶层 `null`       | `400 invalid_json` | 视为存在 body      |

补充规则：

- 零字节 body 的 no-op 发生在 `Content-Type` 检查之前
- 零字节 body 不要求 `Content-Type` 为 JSON

### 3.3 非空 body 的输入模型

一旦 body 非空，规则固定为：

- 主媒体类型必须是 `application/json`
- `charset=utf-8` 之类的媒体类型参数不影响匹配
- 默认大小限制为 `1 MiB` 原始字节
- 顶层值必须是单个 JSON object
- 文档前后允许空白
- 未知字段默认拒绝

这意味着以下输入都返回 `400 invalid_json`：

- 顶层 `null`
- 顶层 array、string、number、boolean
- 截断 JSON
- 尾随非空白数据
- 多个 top-level JSON 值
- 带 UTF-8 BOM 的 body
- 未知字段
- 标准 JSON 类型不匹配

### 3.4 字段解码语义

字段发现与解码直接跟随 Go `1.25.9` 的 `encoding/json`。

公开语义固定为：

- 命名类型按标准库规则解码
- `json.RawMessage` 按标准库规则解码
- 自定义 `UnmarshalJSON` / `UnmarshalText` 按标准库规则解码
- 嵌入字段提升与遮蔽按标准库规则处理
- 同名 JSON object key 按标准库语义处理，后值覆盖前值

`BindBody(...)` 以标准库字段发现与解码语义作为唯一字段语义来源。

## 4. 错误模型

### 4.1 usage error

以下场景返回普通 error：

- `nil request`
- 非法 target

### 4.2 客户端输入错误

以下场景返回稳定 `*errx.HTTPError`：

- 非空 body 但媒体类型不是 JSON：`415 unsupported_media_type`
- body 超过 `1 MiB`：`413 request_too_large`
- JSON 文档不合法或不符合本文输入模型：`400 invalid_json`

### 4.3 底层读取失败

以下场景返回普通 error：

- `Body.Read` 包装错误
- transport I/O error
- `context` cancellation / deadline

### 4.4 优先级

冲突条件下的优先级固定为：

1. usage error
2. body read failure / request too large
3. 零字节 body no-op 或缺失 body
4. 非空 body 的媒体类型检查
5. JSON 解码与文档边界错误

## 5. 与 `RequireBody(...)` 的关系

`RequireBody(...)` 负责表达“调用方显式要求 body 必填”。
`BindBody(...)` 负责表达“把当前 request body 绑定到 DTO”。

两者的组合方式固定为：

- 可以先 `RequireBody(...)` 再 `BindBody(...)`
- 也可以先 `BindBody(...)` 再 `RequireBody(...)`
- 组合范围限定在同一个 request
- 调用方可以把“body 是否必填”和“body 如何解码”分开表达

## 6. 测试基线

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
