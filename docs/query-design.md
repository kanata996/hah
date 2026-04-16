# hah Query 设计方案

- 状态：Locked
- 版本：v4
- 锁定日期：2026-04-17
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

## 1. 设计定位

`Query(...)` 是请求侧单字段 query helper。
它只处理“一个 query key + 一个显式类型入口 + 零个或多个链式约束”。

`Query(...)` 负责：

- 从 query source 读取一个 key
- 按调用方显式选择的类型入口解析该值
- 执行简单值约束
- 把客户端输入错误收敛为稳定公开错误

`Query(...)` 不负责：

- DTO 投影
- 多字段组合校验
- 未知 query key 拒绝
- 业务规则解释
- 通用 query DSL

## 2. 稳定公开契约

### 2.1 Source 与 builder 创建

`Query(r, name)` 的公开行为固定为：

- `name` 会先做 `strings.TrimSpace`
- `r == nil` 不是构造期 panic，而是在 `Get()` 时返回普通 usage error
- `name` 为空字符串时，在 `Get()` 时返回普通 usage error
- 零值 builder 不是合法公开入口，`Get()` 时返回普通 usage error
- 数据只从 `request.URL` 的 query source 读取
- `request.URL == nil` 视为“没有 query 参数”
- query 的解码语义跟随 `net/url`
- `Query(...)` 只负责读取一个命名 key，不负责为此额外校验整条 raw query
- 默认 `Query(...)` 不把 `net/url` 的全局 raw query 解析错误额外提升为公开客户端错误；若需要整条 query fail-closed，应使用 `BindQuery(...)`

### 2.2 支持类型表

`QueryParam` 只支持下表中的类型入口。
除表内类型入口外，不支持其他公开类型。

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
| `UnixMilliTime()` | `time.Time`     | 是       | 返回零值时间  | 解析失败     |
| `Values()`        | `[]string`      | 是       | 返回 `nil`    | 保留空字符串 |

解析规则固定为：

- `Int` / `Int64`：`strconv.ParseInt(..., 10, bits)`
- `Uint` / `Uint64`：`strconv.ParseUint(..., 10, bits)`
- `Bool`：`strconv.ParseBool`
- `Float64`：`strconv.ParseFloat(..., 64)`
- `Duration`：`time.ParseDuration`
- `UUID`：`uuid.Parse`
- `Time`：RFC3339，保留标准库解析结果，不额外归一化时区
- `UnixTime`：先校验宽度恰好为 10，再按秒级 Unix 时间戳解析，并归一化到 UTC
- `UnixMilliTime`：先校验宽度恰好为 13，再按毫秒级 Unix 时间戳解析，并归一化到 UTC
- `Values()`：返回该 key 的全部解析后值副本，而不是 raw query 子串

表外类型入口一律不支持。

### 2.3 重复 key 与空值语义

- typed builder 对目标 key 只接受零个或一个值
- 当同名 key 出现多个值时，typed builder 返回客户端输入错误
- `Values()` 返回全部值并保留顺序
- `Values()` 保留重复值和空字符串
- 参数缺失与参数存在但首值为空字符串是两种不同状态
- 对 typed builder，只有 `String()` 接受空字符串；其他类型把空字符串当普通解析失败处理

### 2.4 通用 builder 规则

通用规则固定为：

- `Required()` 表示参数必须存在
- `Default(v)` 只在参数缺失时生效
- `Required()` 与 `Default(...)` 互斥
- 重复 `Required()` 幂等
- 未声明 `Required()` 时，重复 `Default(...)` 以后一次为准
- 未声明 `Required()` 且参数缺失时，若未配置 `Default(...)`，直接返回类型零值，不执行类型解析、built-in constraint 或 `Check(...)`
- 参数缺失且命中 `Default(v)` 时，以默认值进入后续约束与 `Check(...)`
- 默认值仍然要经过后续全部约束与 `Check(...)`
- 若 `Default(v)` 未通过后续约束或 `Check(...)`，`Get()` 返回普通 usage error
- `Check(nil)` 返回普通 usage error
- 自定义 `Check(...)` 返回非 `nil` error 时，整体视为校验失败
- `Check(...)` 的 error 文本不是默认公开 detail 契约
- builder 一旦记录 usage error，后续链式调用不会清除该状态
- `Get()` 返回首次记录的 usage error
- builder 不缓存 query 快照；每次 `Get()` 都重新读取当前 request

### 2.5 约束执行顺序

- 所有值约束按最终声明顺序执行
- 遇到第一个失败约束即短路返回
- `OneOf(...)`、`Match(...)`、`Check(...)` 每调用一次都会追加一条独立约束
- `Min` / `Max`、`MinLen` / `MaxLen`、`After` / `Before` 属于 named 约束；重复声明时以后一次为准，并按最后一次声明的位置参与执行

### 2.6 typed builder 的 `Get()` 执行顺序

对除 `Values()` 之外的 typed builder，`Get()` 执行顺序固定为：

1. 若 builder 已记录 usage error，立即返回首次记录的 usage error
2. 读取当前 request 中该 key 的当前值状态，只区分三种状态：缺失、恰好一个值、多个值
3. 若为多个值，立即返回客户端输入错误；不进入缺失处理、类型解析或后续约束
4. 若为缺失：
   - 声明了 `Required()`：返回 `required` violation
   - 未声明 `Required()` 且声明了 `Default(v)`：以 `v` 作为候选值继续后续约束与 `Check(...)`
   - 未声明 `Required()` 且未声明 `Default(v)`：直接返回类型零值，不进入类型解析、built-in constraint 或 `Check(...)`
5. 若为恰好一个请求值：先按类型入口完成解析；解析失败立即返回客户端输入错误
6. 对进入约束阶段的候选值，按本文档 2.5 的顺序执行 built-in constraint 与 `Check(...)`
7. 若候选值来自请求输入，则约束失败返回客户端输入错误；若候选值来自 `Default(v)`，则约束失败返回 usage error
8. 全部成功后返回最终值

该顺序只锁 typed builder；`Values()` 不参与“单值重复 key”判定。

### 2.7 `Values()` builder 语义

`Values()` 是唯一公开的多值读取入口。

`Values()` 的公开语义固定为：

- 支持 `Required()`、`Default(...)`、`Check(...)`、`Get()`
- 不支持单值 typed builder 的类型解析、重复 key 拒绝或 built-in constraint
- `Get()` 必须读取当前 request 中该 key 的全部值，并返回独立副本
- 返回值必须保留顺序、重复值和空字符串
- 修改一次 `Get()` 返回的切片，不得影响 builder 内部状态、后续 `Get()` 结果或默认值本身

`Values().Get()` 执行顺序固定为：

1. 若 builder 已记录 usage error，立即返回首次记录的 usage error
2. 读取当前 request 中该 key 的全部当前值
3. 若该 key 缺失：
   - 声明了 `Required()`：返回 `required` violation
   - 未声明 `Required()` 且声明了 `Default(v)`：以默认切片副本作为候选值进入 `Check(...)`
   - 未声明 `Required()` 且未声明 `Default(v)`：直接返回 `nil`
4. 若该 key 存在：以当前全部值的副本作为候选值进入 `Check(...)`
5. 若 `Check(...)` 失败：
   - 候选值来自请求输入：返回客户端输入错误
   - 候选值来自 `Default(v)`：返回 usage error
6. 全部成功后返回候选切片

### 2.8 类型专属约束

`String()` 额外支持：

- `MinLen(n)` / `MaxLen(n)`：按 UTF-8 rune 数比较
- `OneOf(values...)`
- `Match(re)`：直接按 `regexp.Regexp.MatchString` 判断；若要求整串匹配，由调用方自行加锚点

`Int` / `Int64` / `Uint` / `Uint64` / `Float64` / `Duration` 额外支持：

- `Min(v)` / `Max(v)`，且边界包含

`Time()` / `UnixTime()` / `UnixMilliTime()` 额外支持：

- `After(t)` / `Before(t)`，且边界严格排除相等值

所有 builder 都支持：

- `Check(...)`
- `Get()`

以下配置属于普通 usage error：

- `OneOf()` 为空
- `Match(nil)`
- `Check(nil)`
- `MinLen` / `MaxLen` 为负数
- `Min` / `Max`、`MinLen` / `MaxLen`、`After` / `Before` 的最终配置自相矛盾

## 3. 错误边界

### 3.1 usage error

以下场景返回普通 error，而不是 `*errx.HTTPError`：

- `Query(nil, name)`
- 参数名为空
- 零值 builder 直接使用
- 非法约束配置
- 配置的 `Default(...)` 未通过后续约束或 `Check(...)`

稳定契约只有：

- `err != nil`
- `err` 不是 `*errx.HTTPError`
- `errors.As(err, *errx.HTTPError)` 必须失败

usage error 的具体 type、wrapping 和文本不属于公开契约。

### 3.2 客户端输入错误

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
- 缺失 required 参数时，violation `Code == errx.CodeRequired`，`Detail == "is required"`
- 单值 typed builder 命中重复 key 时，violation `Code == errx.CodeMultiple`，`Detail == "must appear only once"`
- 解析失败或校验失败时，violation `Code == errx.CodeInvalid`，`Detail == "is invalid"`

## 4. 与其他文档的关系

- `Query(...)` 是单字段 helper；`BindQuery(...)` 是 DTO binder。
- 两者共享“空字符串视为已提交参数”“单值入口拒绝重复 key”“默认忽略未知 key（在各自适用范围内）”这些方向上的输入模型。
- `Query(...)` 不为读取单个 key 额外扫描整条 raw query 的全局合法性。
- `BindQuery(...)` 比 `Query(...)` 更严格：它要处理 DTO 规划、整条 raw query 解析、字段白名单和原子提交。
- 顶层错误模型和 violation 词汇由 `errx` 提供；响应写回由 `resp` 决定。

## 5. 测试基线

后续实现或重构至少应锁住：

- `Query(r, name)` 会裁剪参数名空白
- `nil request`
- 空参数名
- 零值 builder 与零值 typed builder 直接使用
- `request.URL == nil` 视为空输入
- typed builder 命中重复 key 时返回单个 `multiple` violation
- `Values()` 返回全部解析后值并保留顺序
- `Values()` 保留重复值
- `Values()` 跟随 `net/url` 的解码语义，而不是 raw query 子串
- `Values()` 保留空字符串；缺失时返回 `nil`
- `Values()` 支持 `Required()`、`Default(...)`、`Check(...)`
- `Values()` 不参与单值重复 key 拒绝，也不做 built-in constraint
- `Values()` 缺失且声明 `Required()` 时返回单个 `required` violation
- `Values()` 缺失且声明 `Default(...)` 时返回默认切片副本
- `Values()` 的返回值是 defensive copy；修改返回切片不会影响后续 `Get()`
- `Query(...)` 不会为了读取单个 key 额外扫描整条 raw query 并引入新的全局错误路径
- typed builder 缺失时返回零值
- 未声明 `Required()` 且参数缺失时，不执行 built-in constraint 或 `Check(...)`
- `Required()` 缺失时返回单个 `required` violation
- 重复 `Required()` 幂等
- `Default(...)` 与 `Required()` 互斥
- 重复 `Default(...)` 以后一次为准
- default 值仍然经过后续校验
- 非法 default 值会返回 usage error
- `Check(nil)` usage error
- `OneOf(...)`、`Match(...)`、`Check(...)` 的顺序与短路语义
- `OneOf()` 空参数 usage error
- `Match(nil)` usage error
- `Match(...)` 的匹配语义等同于 `Regexp.MatchString`
- `String().MinLen()` / `MaxLen()` 的成功、失败、冲突路径
- 数值类型 `Min()` / `Max()` 的成功、失败、冲突路径
- `Duration()`、`UUID()`、`Time()`、`UnixTime()`、`UnixMilliTime()` 的代表性成功 / 失败路径
- `Time()` 不额外做时区归一化
- `UnixTime()` 的 10 位宽度规则
- `UnixMilliTime()` 的 13 位宽度规则
- `After()` / `Before()` 在相等边界时失败
- 空字符串只有 `String()` 接受，其他类型按解析失败处理
- `Check(...)` 失败时公开 detail 仍保持稳定 `is invalid`
- usage error 的 sticky 语义
- typed builder 的 `Get()` 执行顺序与错误优先级固定
- typed builder 命中重复 key 时，不进入类型解析或后续约束
- `Values()` 的 `Get()` 执行顺序与错误优先级固定
- 同一 builder 多次 `Get()` 会重新读取当前 request query
