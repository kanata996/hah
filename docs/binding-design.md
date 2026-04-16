# hah 绑定设计方案（BindBody / BindQuery）

本文档定义 `hah` 请求绑定能力的设计目标、公开契约与后续演进边界。
它面向 `reqx.BindBody(...)` / `reqx.BindQuery(...)` 及根包 `hah` facade。

适用范围：

- `hah.BindBody(...)`
- `hah.BindQuery(...)`
- `reqx.BindBody(...)`
- `reqx.BindQuery(...)`

本文档只覆盖“绑定”本身，不覆盖 `Path(...)` / `Query(...)` request helper、响应写回或业务校验。

## 1. 设计定位

`hah` 是 `net/http`-first 的 JSON API 边界层。
绑定能力的职责是把 HTTP 输入投影到调用方 DTO，并把绑定阶段的错误收敛为稳定的公开错误。

绑定层的设计取向参考 Echo `BindBody` 的思路：

- binder 负责 source / content-type 分发
- binder 行为尽量贴近底层解码器
- binder 提供稳定错误边界
- binder 不顺手承担请求级校验、DTO 纯度治理或业务规则执行

在 `hah` 中，这个取向落到两条能力线上：

- `BindBody(...)`：只处理 JSON body -> DTO
- `BindQuery(...)`：只处理 query -> DTO

## 2. 核心原则

### 2.1 单一职责

`BindBody(...)` / `BindQuery(...)` 只负责：

- 从指定来源读取输入
- 按既定规则绑定到目标 DTO
- 把绑定阶段错误收敛为稳定的公开错误

它们不负责：

- 业务规则校验
- 请求级必填 / 枚举 / 范围规则
- DTO 安全分层策略
- 失败后的事务性回滚

### 2.2 行为尽量贴近底层解码器

`BindBody(...)` 的字段语义尽量与 `encoding/json` 保持一致。

这意味着：

- 缺失字段不会清空目标已有值
- 未知字段默认忽略
- 指针、嵌套对象、零值覆盖等行为由标准库 JSON 解码器决定
- binder 不额外发明一套只在 body 路径生效的字段限制

`BindQuery(...)` 没有直接可复用的标准库结构体解码器，但也应遵循同一原则：

- 规则保持简单、可预期
- 不做局部特判式 DTO 审查
- 调用方能从 tag 和字段形状直接推断绑定结果

### 2.3 错误边界稳定优先

绑定层需要明确区分两类错误：

- 使用错误：调用方传入非法 request / target / DTO 形状
- 客户端输入错误：请求内容不合法、类型不匹配、媒体类型不支持等

前者返回普通错误，后者收敛为稳定 HTTP 错误。

### 2.4 非事务性

`BindBody(...)` 与 `BindQuery(...)` 都不是事务性的。

公开契约明确规定：

- 绑定直接作用在调用方传入的 target 上
- 如果返回错误，target 可能已经被部分更新
- 调用方不应依赖失败后的 target 精确状态

该规则对 body / query 保持一致，不对部分字段做选择性回滚。

## 3. BindBody 设计

### 3.1 目标

`BindBody(...)` 只解决以下问题：

- 请求是否有 body
- body 是否是支持的媒体类型
- body 是否在大小限制内
- 如何把 JSON body 解码到 target
- 如何把 decode 错误映射为稳定公开错误

### 3.2 非目标

`BindBody(...)` 不负责：

- 校验 body 是否“业务上必须存在”
- 限制 DTO 必须只包含某些字段
- 检查是否存在未使用 tag
- 校验 JSON 里是否出现未知字段
- 对 decode 失败做字段级回滚

### 3.3 支持范围

当前只支持：

- 空 body
- `application/json`

当前不支持：

- XML
- `application/*+json`
- 多媒体类型协商

如果未来要支持 `application/*+json`，应视为公开契约变化，单独设计与发布说明。

### 3.4 公开契约

`BindBody(...)` 的公开行为如下：

1. 若 request 或 target 非法，返回普通 usage error。
2. 若实际读取到零字节 body，直接 no-op。
3. 空 body 的 no-op 发生在 `Content-Type` 检查之前。
4. 非空 body 只接受 `application/json`。
5. JSON 解码使用标准库 `encoding/json`。
6. 解码直接作用在传入 target 上。
7. JSON 缺失字段不会清空 target 已有值。
8. 若解码失败，target 可能已经被部分更新。
9. 若 body 超过默认大小限制，返回稳定 `413 request_too_large`。
10. 若 `Content-Type` 不支持，返回稳定 `415 unsupported_media_type`。
11. 若 JSON 非法或类型不匹配，返回稳定 `400 invalid_json`。

### 3.5 错误边界

`BindBody(...)` 推荐维持以下错误收敛边界：

- usage / config 错误：
  - `nil request`
  - `nil target`
  - 非指针 target
  - typed-nil target
  - 这类错误返回普通 error
- 输入错误：
  - 非法 JSON
  - JSON 类型不匹配
  - body 超限
  - 不支持的 `Content-Type`
  - 这类错误收敛为稳定 `*errx.HTTPError`

### 3.6 与 RequireBody 的关系

`RequireBody(...)` 与 `BindBody(...)` 保持正交：

- `BindBody(...)` 不负责“body 是否必填”
- `RequireBody(...)` 显式表达 body-required 规则
- 二者共享同一个非破坏性 body-presence probe
- 调用顺序可自由组合

## 4. BindQuery 设计

### 4.1 目标

`BindQuery(...)` 只解决以下问题：

- 如何从 query 读取字符串值
- 如何把 query 值映射到目标 DTO
- 如何支持切片、多值和自定义解码
- 如何把客户端输入错误与 DTO 使用错误区分开

### 4.2 非目标

`BindQuery(...)` 不负责：

- 请求级规则校验
- 默认严格模式 DTO 审查
- 判断“哪些字段本不该出现在 DTO 里”
- 对失败字段做回滚
- 自动推导“业务上哪些字段敏感”

DTO 的安全性和边界治理，优先通过“专用输入 DTO + 显式映射”解决，而不是不断在 binder 内增加字段形状限制。

### 4.3 支持的目标类型

`BindQuery(...)` 公开支持以下目标：

- `struct`
- `map[string]string`
- `map[string][]string`
- `map[string]any`

其余目标类型一律返回普通 usage error。

### 4.4 字段规则

对于 `struct` 目标，字段规则定义如下。

#### 4.4.1 显式绑定字段

`query:"name"` 表示该字段绑定 query key `name`。

语义：

- query 名按精确值匹配
- 标量字段取第一个值
- 切片字段接收全部同名值
- 缺失参数不覆盖字段已有值

#### 4.4.2 显式跳过字段

`query:"-"` 表示该字段完全不参与 query binding。

语义：

- 不绑定
- 不递归
- 不校验该字段形状

该规则用于显式排除 helper 字段、缓存字段、映射中间字段等“存在于 DTO 结构但不属于 query 输入面”的字段。

#### 4.4.3 未标注字段

未标注字段的默认规则如下：

- 未导出字段：忽略
- 导出非 struct 字段：忽略
- 导出 `struct` 字段：按 inline nested DTO 递归绑定
- 导出 `*struct` 字段：
  - 若字段为非 `nil`，按 inline nested DTO 递归绑定
  - 若字段为 `nil`，直接忽略，不自动分配

该规则保持兼容、简单和可预期，不把“未标注字段存在”本身视为 DTO 非法。

### 4.5 嵌套 DTO 规则

未标注的嵌套 `struct` / `*struct` 代表 inline nested DTO，而不是严格 tag-only 模式。

设计约束：

- inline nested DTO 只对导出且可绑定字段生效
- 私有字段一律忽略
- 不因为私有字段、helper 字段或不可设置字段而报错
- 不对未标注的命名 `*struct` 做单独禁用

如果调用方不希望某个嵌套字段参与绑定，应显式写 `query:"-"`。

### 4.6 自定义解码

`BindQuery(...)` 继续支持以下解码扩展：

- `BindUnmarshaler`
- `BindMultipleUnmarshaler`
- `encoding.TextUnmarshaler`
- `time.Time` + `format:"..."`
- `time.Duration`

设计要求：

- 标量字段默认消费第一个值
- 多值字段可通过切片或 `BindMultipleUnmarshaler` 接收全部值
- 自定义 decoder 返回 `*errx.HTTPError` 时原样保留
- 其余普通错误收敛为 `400 bad_request`

### 4.7 公开契约

`BindQuery(...)` 的公开行为如下：

1. 若 request 或 target 非法，返回普通 usage error。
2. 只从 query source 读取数据，不读取 path、header、body。
3. 缺失参数不覆盖 DTO 已有值。
4. 绑定按字段声明顺序执行。
5. 遇到第一个字段错误立即停止，后续字段不再处理。
6. 绑定不是事务性的；若返回错误，DTO 可能已经被部分更新。
7. DTO/tag 形状本身非法时，返回普通 usage error，而不是 `400 bad_request`。
8. 普通 query 解码失败收敛为稳定 `400 bad_request`。
9. 若自定义 decoder 返回 `*errx.HTTPError`，原样保留其公开语义。

### 4.8 默认不采用严格模式

默认 `BindQuery(...)` 不把“所有导出字段都必须显式写 tag”作为约束。

原因：

- 它会把 binder 从“source-to-DTO 映射器”变成“DTO 形状审查器”
- 容易把无关 helper 字段和私有字段误纳入失败条件
- 与 `BindBody(...)` 的设计取向不一致
- 会带来不必要的兼容性破坏

## 5. query:"-" 设计

### 5.1 目的

`query:"-"` 是显式跳过机制，用于回答：

- 这个字段存在于 struct 中，但不属于 query 输入面
- 即便它是导出字段或嵌套 struct，也不应参与绑定

### 5.2 公开语义

`query:"-"` 应具有最高优先级：

- 不绑定
- 不递归
- 不校验

这使调用方可以稳定地在 DTO 中保留：

- helper 字段
- 组合字段
- 缓存字段
- 业务内部映射字段

### 5.3 示例

```go
type ListAccountsQuery struct {
	Page    int      `query:"page"`
	Tags    []string `query:"tag"`
	Loaded  bool     `query:"-"`
	private string
}
```

公开结果：

- `Page` / `Tags` 参与绑定
- `Loaded` 被显式跳过
- `private` 被忽略

## 6. 严格模式的演进路径

如果未来确实需要“所有参与 query binding 的字段都必须显式声明”的能力，不应直接改变默认 `BindQuery(...)`。

建议单独提供 opt-in 严格模式，例如：

- `BindQueryStrict(...)`
- 或 `BindQueryWithOptions(..., StrictTags())`

严格模式的推荐语义：

- 导出字段必须显式写 `query:"name"` 或 `query:"-"`
- 未导出字段仍然忽略
- 不因私有字段报错
- 保持同样的错误边界和非事务性语义

这样可以：

- 满足偏严格团队的需求
- 不破坏默认 binder 的兼容性与简单性
- 让“严格模式”成为可选择契约，而不是隐式破坏性变化

## 7. 错误边界总表

| 场景 | 返回类型 | 说明 |
| --- | --- | --- |
| request / target 非法 | 普通 error | 使用错误 |
| DTO/tag 形状非法 | 普通 error | 使用错误 |
| 非法 JSON | `400 invalid_json` | 客户端输入错误 |
| body 超限 | `413 request_too_large` | 客户端输入错误 |
| 不支持的 `Content-Type` | `415 unsupported_media_type` | 客户端输入错误 |
| query 值解析失败 | `400 bad_request` | 客户端输入错误 |
| 自定义 decoder 返回 `*errx.HTTPError` | 原样保留 | 调用方显式公共语义 |

## 8. 安全建议

绑定层不负责“安全字段白名单”。
推荐调用方遵循以下实践：

1. 为 HTTP 输入定义专用 DTO。
2. 不把业务实体直接作为 bind target。
3. 绑定完成后显式映射到应用层对象。
4. 敏感字段不要出现在输入 DTO 上。

这比在 binder 内部不断增加字段审查规则更稳定，也更容易被调用方理解。

## 9. 测试基线

后续实现或重构时，至少应锁住以下公开行为。

### 9.1 BindBody

- 空 body no-op
- 非 JSON `Content-Type`
- body 超限
- 非法 JSON
- 类型不匹配
- 缺失字段保留已有值
- 失败时可能部分更新
- `RequireBody(...)` 组合行为

### 9.2 BindQuery

- `query:"name"`
- `query:"-"`
- 未标注导出 `struct` 递归绑定
- 未标注导出非 `nil` `*struct` 递归绑定
- 未标注导出 `nil` `*struct` 忽略
- 未导出字段忽略
- 标量 / 切片多值语义
- 自定义 decoder 成功 / 失败
- `*errx.HTTPError` 透传
- 失败时可能部分更新
- usage error 与 `400 bad_request` 的边界

## 10. 决策摘要

本设计的最终决策如下：

- `hah` 只提供 `BindBody(...)` 和 `BindQuery(...)` 两类绑定能力。
- `BindBody(...)` 保持 JSON-only，字段语义尽量贴近 `encoding/json`。
- `BindQuery(...)` 保持简单、兼容、可预期的 source-to-DTO 映射，不默认启用严格模式。
- `BindBody(...)` / `BindQuery(...)` 都是非事务性的。
- 新增 `query:"-"` 作为显式跳过机制。
- 如果未来需要“全显式 tag”模式，单独提供 opt-in 严格模式，不改变默认 `BindQuery(...)` 契约。

