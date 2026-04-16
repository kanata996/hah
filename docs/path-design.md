# hah Path 设计方案

- 状态：Locked
- 版本：v3
- 锁定日期：2026-04-17
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

## 1. 设计定位

`Path(...)` 是请求侧单字段 path helper。
它只处理“一个 path 参数名 + 一个显式类型入口 + 零个或多个链式约束”。

`Path(...)` 负责：

- 读取一个 path 参数
- 按调用方显式选择的类型入口解析该值
- 执行简单值约束
- 把客户端输入错误收敛为稳定公开错误

`Path(...)` 不负责：

- DTO 投影
- 多字段组合校验
- router-specific pattern 兼容
- 多值 path 参数模型
- 业务规则解释

## 2. 稳定公开契约

### 2.1 Source 与 builder 创建

`Path(r, name)` 的公开行为固定为：

- `name` 会先做 `strings.TrimSpace`
- `r == nil` 不是构造期 panic，而是在 `Get()` 时返回普通 usage error
- `name` 为空字符串时，在 `Get()` 时返回普通 usage error
- 零值 builder 不是合法公开入口，`Get()` 时返回普通 usage error
- 数据只从 `request.PathValue(name)` 读取

### 2.2 存在性判定

`Path(...)` 对“参数是否存在”的规则固定为：

- 当 `request.PathValue(name) != ""` 时，参数视为存在
- 当 `request.PathValue(name) == ""` 时，只有 `request.Pattern` 是标准库 `net/http` `ServeMux` 合法 pattern，且其中声明了同名命名 wildcard，该参数才视为存在
- 只认标准库语义中的 `{name}` 与 `{name...}`
- `{$}`、malformed pattern、adapter-specific pattern（例如 `{id:[0-9]+}`）都不在默认契约内
- bridge 若手工写入 `PathValue`，必须先自行完成解码、归一化和 pattern 对齐；`Path(...)` 不做二次 unescape 或额外归一化

### 2.3 支持类型表

`PathParam` 只支持下表中的类型入口。
除表内类型入口外，不支持其他公开类型。

| 入口       | Go 类型     | 是否支持 | 缺失参数时    | 空字符串存在时 |
| ---------- | ----------- | -------- | ------------- | -------------- |
| `String()` | `string`    | 是       | 返回 `""`     | 返回 `""`      |
| `Int()`    | `int`       | 是       | 返回 `0`      | 解析失败       |
| `Int64()`  | `int64`     | 是       | 返回 `0`      | 解析失败       |
| `Uint()`   | `uint`      | 是       | 返回 `0`      | 解析失败       |
| `Uint64()` | `uint64`    | 是       | 返回 `0`      | 解析失败       |
| `UUID()`   | `uuid.UUID` | 是       | 返回零值 UUID | 解析失败       |

解析规则固定为：

- `Int` / `Int64`：`strconv.ParseInt(..., 10, bits)`
- `Uint` / `Uint64`：`strconv.ParseUint(..., 10, bits)`
- `UUID`：`uuid.Parse`

`Path(...)` 只支持单值模型，不支持 `Values()`。
表外类型入口一律不支持。

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

### 2.5 约束执行顺序

- 所有值约束按最终声明顺序执行
- 遇到第一个失败约束即短路返回
- `OneOf(...)`、`Match(...)`、`Check(...)` 每调用一次都会追加一条独立约束
- `Min` / `Max`、`MinLen` / `MaxLen` 属于 named 约束；重复声明时以后一次为准，并按最后一次声明的位置参与执行

### 2.6 `Get()` 执行顺序

`Path(...)` 的 `Get()` 执行顺序固定为：

1. 若 builder 已记录 usage error，立即返回首次记录的 usage error
2. 先按本文档 2.2 的规则判定该 path 参数是“缺失”还是“存在”
3. 若为缺失：
   - 声明了 `Required()`：返回 `required` violation
   - 未声明 `Required()` 且声明了 `Default(v)`：以 `v` 作为候选值继续后续约束与 `Check(...)`
   - 未声明 `Required()` 且未声明 `Default(v)`：直接返回类型零值，不进入类型解析、built-in constraint 或 `Check(...)`
4. 若为存在：先按类型入口完成解析；解析失败立即返回客户端输入错误
5. 对进入约束阶段的候选值，按本文档 2.5 的顺序执行 built-in constraint 与 `Check(...)`
6. 若候选值来自请求输入，则约束失败返回客户端输入错误；若候选值来自 `Default(v)`，则约束失败返回 usage error
7. 全部成功后返回最终值

### 2.7 类型专属约束

`String()` 额外支持：

- `MinLen(n)` / `MaxLen(n)`：按 UTF-8 rune 数比较
- `OneOf(values...)`
- `Match(re)`：直接按 `regexp.Regexp.MatchString` 判断；若要求整串匹配，由调用方自行加锚点

`Int` / `Int64` / `Uint` / `Uint64` 额外支持：

- `Min(v)` / `Max(v)`，且边界包含

`UUID()` 支持：

- `Check(...)`

以下配置属于普通 usage error：

- `OneOf()` 为空
- `Match(nil)`
- `Check(nil)`
- `MinLen` / `MaxLen` 为负数
- `Min` / `Max`、`MinLen` / `MaxLen` 的最终配置自相矛盾

## 3. 错误边界

### 3.1 usage error

以下场景返回普通 error，而不是 `*errx.HTTPError`：

- `Path(nil, name)`
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
- 来自请求输入的类型解析失败
- 来自请求输入的 built-in constraint 失败
- 来自请求输入的 `Check(...)` 失败

公开收敛固定为：

- `Status() == 422`
- `Code() == "invalid_request"`
- `Detail() == "request contains invalid fields"`
- `Errors()` 只包含一个 violation
- violation `Field` 等于裁剪后的参数名
- violation `In == errx.InPath`
- 缺失 required 参数时，violation `Code == errx.CodeRequired`，`Detail == "is required"`
- 解析失败或校验失败时，violation `Code == errx.CodeInvalid`，`Detail == "is invalid"`

## 4. 与其他文档的关系

- `Path(...)` 与 `Query(...)` 共享显式类型入口、链式约束和 request-side violation 模型。
- `Path(...)` 故意比 `Query(...)` 更窄：只支持 path 常见的单值类型，不支持多值读取。
- `errx` 提供统一错误模型；`resp` 决定这些错误如何写回。
- router-specific bridge 不是默认契约的一部分，必须在进入 `Path(...)` 前完成归一化。

## 5. 测试基线

后续实现或重构至少应锁住：

- `Path(r, name)` 会裁剪参数名空白
- `nil request`
- 空参数名
- 零值 builder 与零值 typed builder 直接使用
- usage error 只要求 `err != nil` 且不是 `*errx.HTTPError`
- 缺失 optional 返回各类型零值
- 未声明 `Required()` 且参数缺失时，不执行 built-in constraint 或 `Check(...)`
- `Required()` 缺失时返回单个 `required` violation
- 重复 `Required()` 幂等
- `Default(...)` 与 `Required()` 互斥
- 重复 `Default(...)` 以后一次为准
- default 值仍然经过后续校验
- 非法 default 值会返回 usage error
- `Check(nil)` usage error
- `OneOf(...)`、`Match(...)`、`Check(...)` 的顺序与短路语义
- usage error 的 sticky 语义
- `Get()` 的执行顺序与错误优先级固定
- `OneOf()` 空参数 usage error
- `Match(nil)` usage error
- `Match(...)` 的匹配语义等同于 `Regexp.MatchString`
- `String().MinLen()` / `MaxLen()` 的成功、失败、冲突路径
- 数值类型 `Min()` / `Max()` 的成功、失败、冲突路径
- `UUID()` 的代表性成功 / 失败路径
- 声明过的空 wildcard 会被视为存在
- 标准库合法 `Pattern` 中的 `{name}` / `{name...}` 会参与空字符串存在性判定
- blank pattern、无 wildcard、不同 wildcard 名、`{id:[0-9]+}`、`{$}`、malformed pattern 都不会把空字符串视为存在
- 标准库 `ServeMux` 命中的请求里，typed builder 看到的值与 `request.PathValue(name)` 一致
- bridge 手工填充的 `PathValue` 会被原样消费，不会再做二次 unescape 或额外归一化
- 空字符串只有 `String()` 接受，其他类型按解析失败处理
- 客户端输入错误返回稳定 `422 invalid_request`
- path violation 会稳定标记 `Field`、`In=InPath`、`Code=required/invalid` 与默认 `Detail`
- `Check(...)` 失败时公开 detail 仍保持稳定 `is invalid`
