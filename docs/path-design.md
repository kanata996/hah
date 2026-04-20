# hah Path 设计方案

- 状态：Locked
- 版本：v8
- 锁定日期：2026-04-20
- 适用范围：
  - `hah.Path(...)`
  - `reqx.Path(...)`
- 不覆盖：
  - `Query(...)`
  - `BindQuery(...)`
  - `BindBody(...)`
  - 响应写回
- 关联文档：
  - `docs/testing-standards.md`
  - `docs/query-design.md`
  - `docs/errx-design.md`
- 变更规则：任何公开行为变化，必须先改本文档并补黑盒测试。

## 1. 设计目标

`Path(...)` 是请求侧单字段 path helper。
它的目标是提供一条直接、克制的 path 参数读取路径：

1. 读取一个命名 path value
2. 按调用方显式选择的类型入口解析
3. 执行简单值约束
4. 把客户端输入错误收敛为稳定公开错误

它只消费已经路由完成后的 `request.PathValue(name)`。
它不解析 router pattern，也不承担 router 兼容层职责。

## 2. 心智模型

### 2.1 数据来源

`Path(...)` 只从 `request.PathValue(name)` 读取值。

这意味着：

- path 参数是否存在，由上游 router / bridge 决定
- `Path(...)` 不解析 `request.Pattern`
- `Path(...)` 不推导空 wildcard 是否算“存在”
- 如果上游希望空字符串也算已绑定值，应在进入 `Path(...)` 前先做自己的 bridge 处理

### 2.2 存在性

`Path(...)` 对存在性的规则固定为：

- `request.PathValue(name) != ""`：参数存在
- `request.PathValue(name) == ""`：参数缺失

因此，空字符串不再是特殊 path 值语义；它统一落在“缺失”分支上。

## 3. 公开契约

### 3.1 builder 创建

`Path(r, name)` 的公开行为固定为：

- `name` 会先做 `strings.TrimSpace`
- `r == nil` 不是构造期 panic，而是在 `Get()` 时返回普通 usage error
- `name` 为空字符串时，在 `Get()` 时返回普通 usage error
- builder 必须通过 `Path(...)` 创建；零值 builder 和 typed-nil builder 不属于公开契约

### 3.2 支持类型表

`PathParam` 只支持以下类型入口：

| 入口       | Go 类型     | 是否支持 | 缺失参数时    |
| ---------- | ----------- | -------- | ------------- |
| `String()` | `string`    | 是       | 返回 `""`     |
| `Int()`    | `int`       | 是       | 返回 `0`      |
| `Int64()`  | `int64`     | 是       | 返回 `0`      |
| `Uint()`   | `uint`      | 是       | 返回 `0`      |
| `Uint64()` | `uint64`    | 是       | 返回 `0`      |
| `UUID()`   | `uuid.UUID` | 是       | 返回零值 UUID |

解析规则固定为：

- `Int` / `Int64`：`strconv.ParseInt(..., 10, bits)`
- `Uint` / `Uint64`：`strconv.ParseUint(..., 10, bits)`
- `UUID`：`uuid.Parse`

`Path(...)` 只支持单值模型，不支持多值读取。

### 3.3 通用 builder 规则

通用规则固定为：

- `Required()` 表示参数必须存在
- `Default(v)` 只在参数缺失时生效
- `Required()` 与 `Default(...)` 互斥
- 未声明 `Required()` 且参数缺失时，若未配置 `Default(...)`，直接返回类型零值
- 参数缺失且命中 `Default(v)` 时，以默认值进入后续约束与 `Check(...)`
- 默认值仍然要经过后续全部约束与 `Check(...)`
- built-in constraint 总在 `Check(...)` 之前执行
- `Check(nil)` 返回普通 usage error

除本文显式列出的语义外，链式调用的内部状态管理不属于公开契约。

### 3.4 类型专属约束

`String()` 额外支持：

- `MinLen(n)` / `MaxLen(n)`
- `OneOf(values...)`
- `Match(re)`

`Int` / `Int64` / `Uint` / `Uint64` 额外支持：

- `Min(v)` / `Max(v)`

`UUID()` 支持：

- `Check(...)`

## 4. 错误模型

### 4.1 usage error

以下场景返回普通 error，而不是 `*hah.HTTPError`：

- `Path(nil, name)`
- 参数名为空
- 非法约束配置
- 配置的 `Default(...)` 未通过后续约束或 `Check(...)`

### 4.2 客户端输入错误

以下场景返回稳定 `*hah.HTTPError`：

- `Required()` 参数缺失
- 来自请求输入的类型解析失败
- 来自请求输入的 built-in constraint 失败
- 来自请求输入的 `Check(...)` 失败

错误包络沿用共享 request-side 模型；`Path(...)` 额外固定为：

- `Errors()` 只包含一个 violation
- violation `Field` 等于裁剪后的参数名
- violation `In == hah.InPath`
- 缺失 required 参数时，violation `Code == hah.CodeRequired`
- 解析失败或校验失败时，violation `Code == hah.CodeInvalid`

其余顶层 `422 invalid_request` 包络语义，沿用关联文档中的共享 request-side 错误模型。

## 5. 与其他文档的关系

- `Path(...)` 与 `Query(...)` 共享显式类型入口、约束风格和 request-side 错误模型
- `Path(...)` 故意比 `Query(...)` 更窄：只支持 path 常见的单值类型
- router-specific bridge 不是默认契约的一部分，必须在进入 `Path(...)` 前完成

## 6. 测试基线

除关联文档中共享的 request-side 错误与通用约束基线外，`Path(...)` 额外至少应锁住：

- `Path(r, name)` 会裁剪参数名空白
- `nil request`
- 空参数名
- 缺失 optional 返回各类型零值
- `UUID()` 的代表性成功 / 失败路径
- `request.PathValue(name)` 非空时被原样消费
- `request.PathValue(name)` 为空字符串时按缺失处理
- bridge 手工填充的 `PathValue` 会被原样消费，不会再做二次 unescape 或额外归一化
- path violation 会稳定标记 `Field`、`In=InPath`、`Code=required/invalid`
