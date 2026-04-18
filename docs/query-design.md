# hah Query 设计方案

- 状态：Locked
- 版本：v7
- 锁定日期：2026-04-19
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
3. 执行少量通用约束
4. 把客户端输入错误收敛为稳定公开错误

它是一个 typed parser，不是 query DSL。

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
- 零值 builder 不是合法公开入口，`Get()` 时返回普通 usage error
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
- `Float64`：`strconv.ParseFloat(..., 64)`
- `Duration`：`time.ParseDuration`
- `UUID`：`uuid.Parse`
- `Time`：RFC3339，不额外归一化时区
- `UnixTime`：按 10 位秒级 Unix 时间戳解析，并归一化到 UTC
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

### 3.4 通用 builder 规则

通用规则固定为：

- `Required()` 表示参数必须存在
- `Default(v)` 只在参数缺失时生效
- `Required()` 与 `Default(...)` 互斥
- 重复 `Required()` 幂等
- 未声明 `Required()` 时，重复 `Default(...)` 以后一次为准
- 未声明 `Required()` 且参数缺失时，若未配置 `Default(...)`，直接返回类型零值
- 参数缺失且命中 `Default(v)` 时，以默认值进入后续约束与 `Check(...)`
- 默认值仍然要经过后续全部约束与 `Check(...)`
- `Check(nil)` 返回普通 usage error
- builder 一旦记录 usage error，后续链式调用不会清除该状态
- `Get()` 返回首次记录的 usage error

### 3.5 约束能力

所有 builder 都支持：

- `Required()`
- `Default(...)`
- `Check(...)`
- `Get()`

`String()` 额外支持：

- `MinLen(n)` / `MaxLen(n)`
- `OneOf(values...)`
- `Match(re)`

`Int` / `Int64` / `Uint` / `Uint64` / `Float64` / `Duration` 额外支持：

- `Min(v)` / `Max(v)`

`Time()` / `UnixTime()` 额外支持：

- `After(t)` / `Before(t)`

`Values()` 支持：

- `Required()`
- `Default(...)`
- `Check(...)`
- `Get()`

## 4. 错误模型

### 4.1 usage error

以下场景返回普通 error，而不是 `*errx.HTTPError`：

- `Query(nil, name)`
- 参数名为空
- 零值 builder 直接使用
- 非法约束配置
- 配置的 `Default(...)` 未通过后续约束或 `Check(...)`

### 4.2 客户端输入错误

以下场景返回稳定 `*errx.HTTPError`：

- `Required()` 参数缺失
- 单值 typed builder 命中重复 key
- 来自请求输入的类型解析失败
- 来自请求输入的 built-in constraint 失败
- 来自请求输入的 `Check(...)` 失败

公开收敛固定为：

- `Status() == 422`
- `Code() == "invalid_request"`
- `Detail() == "request contains invalid fields"`
- `Errors()` 只包含一个 violation
- violation `Field` 等于裁剪后的参数名
- violation `In == errx.InQuery`
- 缺失 required 参数时，violation `Code == errx.CodeRequired`
- 单值 typed builder 命中重复 key 时，violation `Code == errx.CodeMultiple`
- 解析失败或校验失败时，violation `Code == errx.CodeInvalid`

## 5. 与其他文档的关系

- `Query(...)` 是单字段 helper；`BindQuery(...)` 是 DTO binder
- 两者共享“空字符串视为已提交参数”“单值入口拒绝重复 key”“默认忽略未知 key”这些方向上的输入模型
- `Query(...)` 不为读取单个 key 额外扫描整条 raw query 的全局合法性
- 顶层错误模型和 violation 词汇由 `errx` 提供；响应写回由 `resp` 决定

## 6. 测试基线

后续实现或重构至少应锁住：

- `Query(r, name)` 会裁剪参数名空白
- `nil request`
- 空参数名
- 零值 builder 与零值 typed builder 直接使用
- `request.URL == nil` 视为空输入
- typed builder 命中重复 key 时返回单个 `multiple` violation
- `Values()` 返回全部解析后值并保留顺序
- `Values()` 保留重复值
- `Values()` 跟随 `net/url` 的解码语义
- `Values()` 保留空字符串；缺失时返回 `nil`
- `Values()` 支持 `Required()`、`Default(...)`、`Check(...)`
- typed builder 缺失时返回零值
- `Required()` 缺失时返回单个 `required` violation
- 重复 `Required()` 幂等
- `Default(...)` 与 `Required()` 互斥
- 重复 `Default(...)` 以后一次为准
- default 值仍然经过后续校验
- `Check(nil)` usage error
- `String().MinLen()` / `MaxLen()` 的成功、失败路径
- 数值类型 `Min()` / `Max()` 的成功、失败路径
- `Duration()`、`UUID()`、`Time()`、`UnixTime()` 的代表性成功 / 失败路径
- 空字符串只有 `String()` 接受，其他类型按解析失败处理
- `Check(...)` 失败时公开 detail 仍保持稳定 `is invalid`
