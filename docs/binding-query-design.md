# hah BindQuery 设计方案

- 状态：Locked
- 版本：v3
- 锁定日期：2026-04-17
- 适用范围：
  - `hah.BindQuery(...)`
  - `reqx.BindQuery(...)`
- 不覆盖：
  - `Path(...)`
  - `BindBody(...)`
  - header / body source
  - 响应写回
- 关联文档：
  - `docs/testing-standards.md`
  - `docs/query-design.md`
  - `docs/errx-design.md`
- 变更规则：任何公开行为变化，必须先改本文档并补黑盒测试。

## 1. 设计定位

`BindQuery(...)` 是 query source 到 DTO 的 binder。
它的目标是“显式、单值、可预测”，而不是做万能解码器。

`BindQuery(...)` 负责：

- 从 query source 读取字符串值
- 按显式 tag 把值写到 DTO
- 区分 DTO 使用错误与客户端输入错误

`BindQuery(...)` 不负责：

- 多值 query 模型
- 业务规则校验
- 自定义 decoder 扩展
- 时间、duration 等复杂字段语义
- 默认严格模式

## 2. 稳定公开契约

### 2.1 调用前提与执行阶段

调用前提固定为：

- `request` 必须非 `nil`
- `target` 必须是非 `nil` 指针
- typed-nil target 视为 usage error
- `request.URL == nil` 视为空 query source

公开支持的 target 只有两类：

| target 形状          | 是否支持 | 说明                      |
| -------------------- | -------- | ------------------------- |
| `*struct`            | 是       | 默认 DTO 绑定目标         |
| `*map[string]string` | 是       | 当前请求 query 的单值快照 |

其余 target 一律是普通 usage error，包括：

- 非指针 target
- 指向非 `struct` / 非 `map[string]string` 的指针
- 多级指针 target

除上表外的 target 形状一律不支持。

执行分三阶段：

1. 规划阶段：校验 request / target / DTO/tag 形状并构建字段计划。
2. source 阶段：解析 raw query 并得到当前请求的 query source 快照。
3. 写入阶段：按固定顺序把 query source 写入与 target 同构的零值临时对象；全部成功后一次性提交到 target。

稳定边界：

- 规划阶段失败时，target 必须保持零修改
- raw query 解析失败时，target 必须保持零修改
- 写入阶段的客户端输入错误必须保持 target 零修改

### 2.2 `struct` target 的 tag 规则

`struct` 目标只认三种 `query` tag：

- `query:"name"`
- `query:"-"`
- `query:",inline"`

其他 tag 形式一律是普通 usage error。

规则固定为：

- 未标注字段一律忽略
- `query:"-"` 一律显式忽略；不参与绑定、冲突检测或字段类型校验
- `query:"name"` 中的 `name` 必须非空，且不得包含前后空白
- `query:"name"` 的 key 按 tag 字面值原样参与匹配；不做 trim、大小写归一化或额外解码
- `query:"name"` 字段必须导出、可设置、且字段形状受支持
- `query:",inline"` 只能用于导出且可设置的 `struct` / `*struct`
- `inline` 展开后若没有任何可绑定子字段，属于普通 usage error
- 多个可绑定字段映射到同一个 query key 时，属于普通 usage error
- 冲突检测必须发生在任何字段写入之前

### 2.3 支持的字段形状

`query:"name"` 只支持以下字段形状：

| 字段形状                                          | 是否支持 | 缺失参数         | 命中参数时                         |
| ------------------------------------------------- | -------- | ---------------- | ---------------------------------- |
| `string`                                          | 是       | 保持零值         | 直接写入首值                       |
| `bool`                                            | 是       | 保持零值         | 按 `strconv.ParseBool` 解析        |
| `int` / `int8` / `int16` / `int32` / `int64`      | 是       | 保持零值         | 按十进制 `strconv.ParseInt` 解析   |
| `uint` / `uint8` / `uint16` / `uint32` / `uint64` | 是       | 保持零值         | 按十进制 `strconv.ParseUint` 解析  |
| `float32` / `float64`                             | 是       | 保持零值         | 按 `strconv.ParseFloat` 解析       |
| 指向上述内建标量的一级指针 `*T`                   | 是       | 保持 `nil`       | 先解码到临时值，成功后再分配或覆盖 |
| `query:",inline"` 的 `struct` / `*struct`         | 是       | 保持零值语义     | 递归按子字段计划写入               |

除上表外，其他字段形状一律不支持。

以下形状一律不支持，且在规划阶段返回普通 usage error：

- 命名类型，即使底层类型是受支持标量
- `time.Time` / `time.Duration`
- 实现 `BindUnmarshaler` / `encoding.TextUnmarshaler` 的类型
- 未标记 `inline` 的 `struct` / `*struct`
- slice / array
- map / interface
- 多级指针

### 2.4 query source 语义

公开语义固定为：

- binder 必须先解析 raw query，再进入字段写入
- raw query 解析失败属于客户端输入错误，不是 usage error
- raw query 解析失败时，返回稳定 `400 bad_request`，且 target 零修改
- query source 采用默认单值模型：同名 key 出现多个值时，视为客户端输入错误
- 对 `struct` 目标，未知 key 默认忽略；但未知 key 也受单值模型约束
- 缺失 key 不会继承 target 旧值；对应字段保持零值临时对象中的默认状态
- 参数存在但首值为空字符串时，仍视为“已提交参数”
- 只有 `string` 与 `*string` 接受空字符串；其他受支持类型把空字符串当解析失败处理

### 2.5 `map[string]string` target

对 `*map[string]string` 的规则固定为：

- 成功绑定后，target 必须替换为当前请求的“单值字符串快照”
- 旧项必须被移除，不允许保留历史 key
- 空 query 绑定后得到可用空 map
- 任一 key 命中多个值时，返回 `400 bad_request` 且 target 零修改
- 不做类型解码
- 不会因为单个值内容本身返回 `400 bad_request`

### 2.6 指针与 inline 写入规则

普通指针字段规则：

- 缺失参数时，对应字段保持零值临时对象中的默认状态
- 命中参数时，先解码到临时 `T`
- 解码成功后再分配或覆盖 pointee
- 单字段解码失败不应污染临时对象，也不得影响 target 当前值

`query:",inline"` 的 `*struct` 规则：

- 冲突检测基于元素类型，而不是运行时是否为 `nil`
- 若零值临时对象中的该字段本次没有命中任何子字段，`nil` 保持 `nil`
- 只有首个命中的子字段即将成功写入时，才允许为 `nil` 指针分配对象
- 若 `nil` 指针的首个命中子字段在写入前失败，临时对象中的字段必须保持 `nil`

### 2.7 原子提交

对于 `*struct` target：

- 写入必须先进入与 target 同构的零值临时对象
- 只有全部字段写入成功后，才允许一次性提交到 target
- 写入阶段任意客户端输入错误都必须保持 target 调用前状态
- 缺失字段不会继承 target 旧值

### 2.8 字段执行顺序

写入阶段顺序固定为：

- 先按当前 `struct` 的字段声明顺序遍历
- 遇到 `inline` 字段时，立刻按其子字段声明顺序深度优先展开
- 遇到第一个客户端输入错误后立即停止

“第一个错误”必须由上述顺序决定，而不是由 map 遍历顺序或反射内部顺序决定。

## 3. 错误边界

### 3.1 usage error

以下场景返回普通 error：

- `nil request`
- `nil target`
- 非指针 target
- typed-nil target
- 不支持的 target 形状
- 非法 `query` tag
- 不支持的字段类型
- 冲突字段计划

### 3.2 客户端输入错误

以下场景返回稳定 `*errx.HTTPError`：

- raw query 解析失败
- query source 中存在重复 key
- 内建标量解析失败
- 空字符串落到非 `string` 字段

稳定契约只锁以下结果：

- `errors.As(err, *errx.HTTPError)` 必须成功
- `Status() == 400`
- `Code() == "bad_request"`

本文档不为 `BindQuery(...)` 额外承诺统一 violation 列表或自定义 detail 文案。

## 4. 与其他文档的关系

- `BindQuery(...)` 与 `Query(...)` 共享“空字符串视为已提交参数”“单值入口拒绝重复 key”“默认忽略未知 key（在各自适用范围内）”的输入模型。
- `BindQuery(...)` 比 `Query(...)` 更严格，因为它还要处理 DTO 规划、整条 raw query 解析、字段白名单和原子提交。
- 顶层错误模型来自 `errx`；对外如何写成 Problem JSON 由 `resp` 决定。
- 若未来需要严格模式，只能作为独立 opt-in 契约引入，不能直接改变本文档的默认行为。

## 5. 测试基线

后续实现或重构至少应锁住：

- `nil request`
- `nil target`
- 非指针 target
- typed-nil target
- 指向不支持目标类型的指针
- `request.URL == nil` 视为空 query source
- raw query 解析失败返回 `400 bad_request`
- raw query 解析失败时 target 零修改
- `map[string]string` 成功绑定后替换为当前请求单值快照
- `map[string]string` 在空 query 下得到空 map
- `map[string]string` 成功绑定时清除旧项
- query source 中任一重复 key 返回 `400 bad_request`
- 重复 key 时 target 零修改
- `query:"name"`
- `query:"-"`
- `query:",inline"`
- `query:"-"` 与未标注字段一样始终忽略
- `query:"-"` 不参与冲突检测或字段类型校验
- inline 子字段计划为空时返回 usage error
- 非法 `query` tag 形式
- `query:""` 返回 usage error
- 带前后空白的 `query:"name"` key 返回 usage error
- `query:"name"` 的 key 按 tag 字面值原样匹配，不做 trim、大小写归一化或额外解码
- tagged 但不可设置字段返回 usage error
- 不支持字段类型在规划阶段返回 usage error
- 命名类型、多级指针、`time.Time`、`time.Duration`、自定义 decoder 类型返回 usage error
- inline 非 `struct` / `*struct` 返回 usage error
- 冲突 query key 在写入前返回 usage error
- 规划阶段 usage error 时 target 零修改
- 未知 query key 默认忽略
- 缺失参数不会继承 target 旧值
- 空字符串参数视为存在
- 只有 `string` / `*string` 接受空字符串
- 普通 pointer 字段命中时按成功结果分配或覆盖
- 普通 pointer 字段单字段解码失败不污染字段当前值
- inline `*struct` 冲突检测不依赖运行时 `nil` 状态
- inline `*struct` 仅在首个子字段即将成功写入时按需分配
- inline `*struct` 在 `nil` 状态下首个命中子字段失败时保持 `nil`
- `*struct` target 写入先进入零值临时对象，成功后一次性提交
- 字段执行顺序按声明顺序加 inline 深度优先
- 第一个客户端输入错误具有确定性
- `string` / `bool` / `int` / `uint` / `float` 的代表性成功 / 失败路径
- 内建标量解码失败收敛为 `400 bad_request`
- 客户端输入错误下 target 零修改
