# hah Query 设计方案

- 状态：Locked
- 版本：v9
- 锁定日期：2026-04-20
- 适用范围：
  - `hah.Query(...)`
  - `reqx.Query(...)`
- 不覆盖：
  - `Path(...)`
  - `BindQuery(...)`
  - `BindBody(...)`
  - 响应写回
- 关联文档：
  - `docs/testing-standards.md`
  - `docs/path-design.md`
  - `docs/binding-query-design.md`
  - `docs/errx-design.md`
- 变更规则：任何公开行为变化，必须先改本文档并补黑盒测试。

## 1. 设计目标

`Query(...)` 是请求侧单字段 query helper。
它的目标是提供一条直接、类型明确、适合 handler 内就地使用的 query 读取路径：

1. 读取一个命名 query key
2. 按调用方显式选择的类型入口解析
3. 执行必要的值约束
4. 把客户端输入错误收敛为稳定公开错误

它是一个 typed parser，不是 query DSL，也不是通用规则编排器。

## 2. 心智模型

### 2.1 一个 key，一个类型入口

`Query(...)` 只处理一个 query key。
调用方先选 key，再选类型入口：

```go
limit, err := Query(r, "limit").Int().Get()
```

这意味着：

- `Query(...)` 不做 DTO 投影
- `Query(...)` 不做多字段组合校验
- `Query(...)` 不额外扫描整条 raw query 的全局合法性

### 2.2 缺失、单值、重复值

对单值 typed builder，`Query(...)` 只关心三种状态：

- 缺失
- 恰好一个值
- 多个值

缺失时返回零值或默认值。
多个值时返回客户端输入错误。

`Values()` 是唯一公开的多值读取入口。

## 3. 公开契约

### 3.1 builder 创建

`Query(r, name)` 的公开行为固定为：

- `name` 会先做 `strings.TrimSpace`
- `r == nil` 不是构造期 panic，而是在 `Get()` 时返回普通 usage error
- `name` 为空字符串时，在 `Get()` 时返回普通 usage error
- builder 必须通过 `Query(...)` 创建；零值 builder 和 typed-nil builder 不属于公开契约
- 数据只从 `request.URL` 的 query source 读取
- `request.URL == nil` 视为“没有 query 参数”
- query 的解码语义跟随 `net/url`

### 3.2 支持类型表

`QueryParam` 只支持以下类型入口：

| 入口              | Go 类型         | 是否支持 | 缺失参数时    | `?x=` 语义   |
| ----------------- | --------------- | -------- | ------------- | ------------ |
| `String()`        | `string`        | 是       | 返回 `""`     | 返回 `""`    |
| `Int()`           | `int`           | 是       | 返回 `0`      | 解析失败     |
| `Int64()`         | `int64`         | 是       | 返回 `0`      | 解析失败     |
| `Uint()`          | `uint`          | 是       | 返回 `0`      | 解析失败     |
| `Uint64()`        | `uint64`        | 是       | 返回 `0`      | 解析失败     |
| `Bool()`          | `bool`          | 是       | 返回 `false`  | 解析失败     |
| `Float64()`       | `float64`       | 是       | 返回 `0`      | 解析失败     |
| `Duration()`      | `time.Duration` | 是       | 返回 `0`      | 解析失败     |
| `UUID()`          | `uuid.UUID`     | 是       | 返回零值 UUID | 解析失败     |
| `Time()`          | `time.Time`     | 是       | 返回零值时间  | 解析失败     |
| `UnixTime()`      | `time.Time`     | 是       | 返回零值时间  | 解析失败     |
| `Values()`        | `[]string`      | 是       | 返回 `nil`    | 保留空字符串 |

解析规则固定为：

- `Int` / `Int64`：`strconv.ParseInt(..., 10, bits)`
- `Uint` / `Uint64`：`strconv.ParseUint(..., 10, bits)`
- `Bool`：`strconv.ParseBool`
- `Float64`：`strconv.ParseFloat(..., 64)`，且只接受有限值
- `Duration`：`time.ParseDuration`
- `UUID`：`uuid.Parse`
- `Time`：严格 RFC3339，且时区 offset 必须合法；不额外归一化时区
- `UnixTime`：按恰好 10 个十进制数字的秒级 Unix 时间戳解析，不接受符号位，并归一化到 UTC
- `Values()`：返回该 key 的全部解析后值副本，而不是 raw query 子串

### 3.3 重复 key 与空值

单值 typed builder 的规则固定为：

- 目标 key 只接受零个或一个值
- 同名 key 出现多个值时，返回客户端输入错误
- 只有 `String()` 接受空字符串
- 其他类型把空字符串当解析失败处理

`Values()` 的规则固定为：

- 返回该 key 的全部值
- 保留顺序
- 保留重复值
- 保留空字符串

### 3.4 通用规则

通用规则固定为：

- `Required()` 表示参数必须存在
- `Default(v)` 只在参数缺失时生效
- `Required()` 与 `Default(...)` 互斥
- 未声明 `Required()` 且参数缺失时，若未配置 `Default(...)`，直接返回类型零值
- 参数缺失且命中 `Default(v)` 时，以默认值进入后续约束与 `Check(...)`
- 默认值仍然要经过后续全部约束与 `Check(...)`
- `Check(nil)` 返回普通 usage error
- built-in constraint 与 `Check(...)` 都只面向单个已解析值，不承担跨字段或全局 query 校验

链式调用的目标是声明“这个值要满足什么条件”，而不是暴露一套可编排的规则系统。
对重复声明、覆盖顺序或内部错误记录方式，调用方不应建立额外依赖。

### 3.5 可用约束

所有入口都支持：

- `Required()`
- `Default(...)`
- `Check(...)`
- `Get()`

单值入口的类型专属约束为：

- `String()`：`MinLen(n)` / `MaxLen(n)`、`OneOf(values...)`、`Match(re)`
- `Int` / `Int64` / `Uint` / `Uint64` / `Float64` / `Duration`：`Min(v)` / `Max(v)`
- `Time()` / `UnixTime()`：`After(t)` / `Before(t)`

`Values()` 不提供类型专属 built-in constraint；需要额外规则时，通过 `Check(...)` 表达。

## 4. 错误模型

### 4.1 usage error

以下场景返回普通 error，而不是 `*hah.HTTPError`：

- `Query(nil, name)`
- 参数名为空
- 非法约束配置（例如 `Required()` 与 `Default(...)` 同时使用，或 `Check(nil)`）
- 配置的 `Default(...)` 未通过后续约束或 `Check(...)`

### 4.2 客户端输入错误

以下场景返回稳定 `*hah.HTTPError`：

- `Required()` 参数缺失
- 单值 typed builder 命中重复 key
- 来自请求输入的类型解析失败
- 来自请求输入的 built-in constraint 失败
- 来自请求输入的 `Check(...)` 失败

公开收敛固定为：

- `Status() == 422`
- `Code() == "invalid_request"`
- `Detail() == "request contains invalid fields"`
- `Errors()` 只包含一个 field error
- field error `Field` 等于裁剪后的参数名
- field error `In == hah.InQuery`
- 缺失 required 参数时，field error `Code == hah.CodeRequired`
- 单值 typed builder 命中重复 key 时，field error `Code == hah.CodeMultiple`
- 解析失败或校验失败时，field error `Code == hah.CodeInvalid`

## 5. 与其他文档的关系

- `Query(...)` 是单字段 helper；`BindQuery(...)` 是 DTO binder
- 两者共享“空字符串视为已提交参数”“单值入口拒绝重复 key”“默认忽略未知 key”这些方向上的输入模型
- `Query(...)` 不为读取单个 key 额外扫描整条 raw query 的全局合法性
- 顶层错误模型和 field error 词汇由 `hah` 提供；默认响应写回也由 `hah` 决定

## 6. 测试基线

后续实现或重构至少应覆盖以下维度：

- builder 创建与输入来源：参数名裁剪、`nil request`、空参数名、`request.URL == nil`、`net/url` 解码语义
- 单值入口的存在性模型：缺失、恰好一个值、重复 key；其中重复 key 需要稳定收敛为单个 `multiple` field error
- 空字符串语义：只有 `String()` 接受 `?x=`；其他类型把空字符串当作解析失败
- 多值入口语义：`Values()` 缺失时返回 `nil`，存在时返回全部值，并保留顺序、重复值和空字符串
- 缺失参数路径：optional 返回零值，`Required()` 返回单个 `required` field error，`Default(...)` 只在缺失时生效
- 约束路径：代表性覆盖 `String()`、数值类型、时间类型和 `Check(...)` 的成功/失败分支，并验证默认值也会经过后续校验
- 类型解析路径：至少覆盖 `Duration()`、`UUID()`、`Time()`、`UnixTime()` 的代表性成功/失败用例，其中 `Time()` 需要拒绝非法 RFC3339 offset，`UnixTime()` 只接受恰好 10 个十进制数字
- 错误收敛：客户端输入错误稳定返回 `422 invalid_request`，并正确标记 query field error 的 `Field`、`In` 和 `Code`
