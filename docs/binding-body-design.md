# hah BindBody 设计方案

- 状态：Locked
- 版本：v1
- 锁定日期：2026-04-17
- 适用范围：
  - `hah.BindBody(...)`
  - `reqx.BindBody(...)`
- 不覆盖：
  - `BindQuery(...)`
  - 除 `RequireBody(...)` 组合语义之外的请求规则
  - 响应写回
- 关联文档：
  - `docs/testing-standards.md`
  - `docs/errx-design.md`
  - `docs/resp-design.md`
- 变更规则：任何公开行为变化，必须先改本文档并补黑盒测试。

## 1. 设计定位

`BindBody(...)` 是 JSON body 到 DTO 的 binder。
它只处理 HTTP body 边界，不做业务校验，也不做 merge。

`BindBody(...)` 负责：

- 判断是否存在 body
- 校验媒体类型
- 执行默认大小限制
- 校验 JSON 文档边界
- 把 JSON 解码到临时值并原子提交
- 区分稳定客户端输入错误、自定义 decoder 错误和底层读取失败

`BindBody(...)` 不负责：

- body-required 规则
- 业务规则校验
- 基于旧 target 做默认值推导
- 非 JSON 媒体类型

## 2. 稳定公开契约

### 2.1 target 与零字节 body

公开支持的 target 只有下表：

| target 形状             | 是否支持 | 说明                       |
| ----------------------- | -------- | -------------------------- |
| 非 `nil` 的 `*struct`   | 是       | 唯一支持的顶层 bind target |
| `nil target`            | 否       | usage error                |
| typed-nil target        | 否       | usage error                |
| 非指针 target           | 否       | usage error                |
| 非 `struct` 指针 target | 否       | usage error                |
| 多级指针 target         | 否       | usage error                |

除上表外的 target 形状一律不支持。

对“零字节 body”的判定只锁定可观察结果：

- `request.Body == nil` 时，视为零字节 body
- 首次用于判定 body 是否存在的探测读取若返回 `n == 0` 且 `err == io.EOF`，视为零字节 body
- 零字节 body 是 no-op，target 保持不变
- 零字节 body 不要求 `Content-Type` 为 JSON
- 判定依据是“实际可读取字节”，不是 `Content-Length`

### 2.2 非空 body 的输入模型

一旦 body 被判定为非空，公开规则固定为：

- 只接受 `application/json`
- 媒体类型比较基于解析后的主媒体类型；参数如 `charset=utf-8` 不影响匹配
- 当前不支持 `application/*+json`
- 默认大小限制固定为 `1 MiB` 原始字节
- 只有严格大于 `1,048,576` bytes 时才返回超限错误

JSON 文档边界固定为：

- 非空 body 必须恰好构成一个 JSON 文档
- 只允许文档前后有可忽略空白
- 仅空白字符的 body 不是空 body，而是非法 JSON
- 尾随非空白数据是非法 JSON
- 多个 top-level JSON 值是非法 JSON
- UTF-8 BOM 不做剥离，带 BOM 的 body 视为非法 JSON
- 对公开支持的 `*struct` target，顶层 JSON 必须是 object
- 顶层 `null`、array、string、number、boolean 都是非法 JSON
- 未知字段默认拒绝

### 2.3 支持字段类型表

`BindBody(...)` 对 `*struct` target 的字段支持只锁下表中的“参与 JSON binding 的字段类型家族”。
除表内字段类型家族外，其他参与 JSON binding 的字段类型一律不支持，并应返回 usage error。

这张表锁的是“字段类型家族”，不展开 `encoding/json` 的全部内部派发细节。
表内类型的字段发现、赋值、自定义 decoder 触发和重复 object key 覆盖，仍跟随 Go `1.25.9` 的 `encoding/json`。

补充规则：

- 这里的“参与 JSON binding 的字段”指按 Go `1.25.9` `encoding/json` 规则会参与字段发现与解码的字段
- 被 `encoding/json` 忽略的字段不在本表约束内，也不因字段类型本身触发 usage error，例如未导出字段、`json:"-"` 字段

| 字段类型家族                                                                                     | 是否支持 | 公开语义                                      | 备注                      |
| ------------------------------------------------------------------------------------------------ | -------- | --------------------------------------------- | ------------------------- |
| `string`                                                                                         | 是       | 按标准库 JSON string 规则解码                 |                           |
| `bool`                                                                                           | 是       | 按标准库 JSON boolean 规则解码                |                           |
| 有符号整数类型：`int` / `int8` / `int16` / `int32` / `int64`                                     | 是       | 按标准库数值规则解码                          |                           |
| 无符号整数类型：`uint` / `uint8` / `uint16` / `uint32` / `uint64` / `uintptr`                    | 是       | 按标准库数值规则解码                          |                           |
| 浮点类型：`float32` / `float64`                                                                  | 是       | 按标准库数值规则解码                          |                           |
| `struct`                                                                                         | 是       | 按标准库对象规则递归解码                      | 顶层仍要求 JSON object    |
| `*T`，其中 `T` 属于表内支持类型                                                                  | 是       | 按标准库指针语义解码                          |                           |
| `[]T` / `[N]T`，其中元素类型 `T` 属于表内支持类型                                                | 是       | 按标准库 array / slice 语义解码               |                           |
| `map[K]V`，其中该 map 类型由 Go `1.25.9` `encoding/json` 默认支持，且值类型 `V` 属于表内支持类型 | 是       | 按标准库 object 语义解码                      |                           |
| `interface{}` / `any`                                                                            | 是       | 按标准库默认表示解码                          |                           |
| `json.RawMessage`                                                                                | 是       | 按标准库原始 JSON 片段语义解码                |                           |
| 实现 `json.Unmarshaler` 的类型                                                                   | 是       | 按标准库自定义 decoder 语义解码；若 decoder 返回 error，收敛为 `400 invalid_json` | |
| 实现 `encoding.TextUnmarshaler` 的类型                                                           | 是       | 按标准库文本 decoder 语义解码；若 decoder 返回 error，收敛为 `400 invalid_json` | |
| 命名类型                                                                                         | 是       | 前提是其底层类型或自定义 decoder 命中表内规则 | 例如命名标量、命名 struct |

代表性不支持类型包括但不限于：

- `func`
- `chan`
- `complex64` / `complex128`
- `unsafe.Pointer`
- 不满足表内条件的 map
- 不属于表内类型家族的其他字段类型

### 2.4 字段值语义与原子提交

对于公开支持的 `*struct` target：

- 字段发现、字段赋值、自定义 decoder 触发和重复 object key 覆盖语义，在表内类型范围内跟随 Go `1.25.9` 的 `encoding/json`
- 解码必须先进入与 target 同构的零值临时对象
- 只有全部成功后，才允许一次性提交到 target
- 失败时，target 必须保持调用前状态
- JSON 缺失字段不会继承 target 旧值

### 2.5 错误分类

错误分类固定为：

- usage error：
  - `nil request`
  - 非法 target
  - DTO 含有参与 JSON binding 的表外字段类型
  - 返回普通 error
- 稳定客户端输入错误：
  - 不支持的媒体类型：`415 unsupported_media_type`
  - body 超限：`413 request_too_large`
  - 非法 JSON、空白 body、截断 JSON、多个 top-level 值、尾随非空白数据、UTF-8 BOM、顶层非 object、未知字段、标准 JSON 类型不匹配、数值溢出、字段级自定义 decoder 返回 error：`400 invalid_json`
  - 返回稳定 `*errx.HTTPError`
- 底层读取失败：
  - `Body.Read` 包装错误
  - transport I/O error
  - `context` cancellation / deadline
  - 返回普通 error

稳定契约只锁以下结果：

- 对客户端输入错误，`errors.As(err, *errx.HTTPError)` 必须成功
- `Status()` / `Code()` 必须分别匹配 `415 unsupported_media_type`、`413 request_too_large`、`400 invalid_json`

### 2.6 错误优先级

冲突条件下的优先级固定为：

1. usage error
2. 零字节 body no-op
3. body 存在性探测阶段的底层读取失败
4. 非空 body 的媒体类型检查
5. body 大小限制
6. JSON 文档边界与顶层 JSON 形状错误
7. 标准 JSON 解码中的客户端输入错误，包括字段级自定义 decoder 返回 error

本文只锁最终分类，不锁内部实现步骤。

## 3. 与 `RequireBody(...)` 的关系

`BindBody(...)` 与 `RequireBody(...)` 保持正交。

组合语义固定为：

| 输入              | `BindBody(...)`    | `RequireBody(...)` |
| ----------------- | ------------------ | ------------------ |
| 零字节 body       | no-op，target 不变 | 视为缺失 body      |
| 仅空白字符的 body | `400 invalid_json` | 视为存在 body      |
| 顶层 `null`       | `400 invalid_json` | 视为存在 body      |

本文档不承诺二者共享特定探测实现，也不承诺同一个请求 body 可以被任意顺序重复消费。

## 4. 测试基线

后续实现或重构至少应锁住：

- 合法 target 仅限非 `nil` 的 `*struct`
- 非法 `request` / `target` 返回 usage error
- 可稳定判定为零字节 body 时 no-op，且不修改 target
- 零字节 body 不要求 `Content-Type` 为 JSON
- 零字节 body 按实际可读字节判定，而不是只按 `Content-Length`
- wrapped `Body.Read` error 返回普通 error
- transport I/O error 返回普通 error
- `context` cancellation / deadline 返回普通 error
- `application/json`
- `application/json; charset=utf-8`
- 非空非 JSON `Content-Type` 返回 `415 unsupported_media_type`
- 大于 `1 MiB` 的 body 返回 `413 request_too_large`
- 恰好 `1 MiB` 的 body 仍允许进入 JSON 校验与解码
- 空白 body 返回 `400 invalid_json`
- 非法 JSON
- 截断 JSON / `unexpected EOF`
- 尾随非空白数据
- 多个 top-level JSON 值
- UTF-8 BOM
- 顶层 `null`
- 顶层 array
- 顶层 string / number / boolean
- 未知字段返回 `400 invalid_json`
- 标准字段类型不匹配或数值溢出返回 `400 invalid_json`
- 参与 JSON binding 的表外字段类型返回 usage error
- 未导出字段、`json:"-"` 字段等被 `encoding/json` 忽略的字段不会因表外类型触发 usage error
- 重复 object key 的覆盖语义跟随 `encoding/json`
- 缺失字段不会继承 target 旧值
- 失败时 target 保持不变
- 自定义 decoder 的成功路径遵循 `encoding/json`
- 自定义 decoder 返回 error 时，返回 `400 invalid_json` 且 target 保持不变
- `RequireBody(...)` 在零字节、空白 body、`null` 下的组合行为
