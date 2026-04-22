# hah BindQuery 设计方案

- 状态：Locked
- 版本：v10
- 锁定日期：2026-04-20
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

## 1. 设计目标

`BindQuery(...)` 是默认的 query DTO binder。
它提供一条刻意收窄、易于理解的绑定路径：

1. 解析当前 request 的 raw query
2. 把 query source 投影到 DTO 的显式顶层字段
3. 成功后一次性提交到 target

这套设计的重点是：

- 只绑定显式声明的字段
- 只支持顶层平铺字段
- 只支持单值 query 模型
- 把 DTO 形状错误与客户端输入错误区分开
- 作为整条 query source 的批量绑定入口，对当前请求 query 输入的整体可接受性负责

## 2. 心智模型

### 2.1 平铺字段 DTO

`BindQuery(...)` 面向“顶层平铺字段”的 DTO。
每个参与绑定的字段都显式声明一个 `query:"name"`。

例子：

```go
type ListAccountsQuery struct {
	Name  string `query:"name"`
	Limit int    `query:"limit"`
}
```

这意味着：

- binder 只看 DTO 顶层字段
- binder 不展开嵌套 DTO
- binder 不引入 `inline`、命名空间或分组语义

### 2.2 一次性提交语义

对 `*struct` target，`BindQuery(...)` 的对外表现固定为：

1. 先根据当前 query source 计算参与绑定字段的新值
2. 全部成功后再一次性提交到 target

因此：

- 失败时 target 保持调用前状态
- 缺失的已绑定字段不会继承 target 旧值
- 未参与绑定的字段保持 target 原值
- 成功时，只有显式参与绑定的字段表现为当前 query source 的投影结果

## 3. 公开契约

### 3.1 target

公开支持的 target 只有两类：

| target 形状          | 是否支持 | 说明                      |
| -------------------- | -------- | ------------------------- |
| `*struct`            | 是       | 默认 DTO 绑定目标         |
| `*map[string]string` | 是       | 当前请求 query 的单值快照 |

其他 target 都属于 usage error，包括：

- `nil request`
- `nil target`
- typed-nil target
- 非指针 target
- 指向非 `struct` / 非 `map[string]string` 的指针
- 多级指针 target

### 3.2 `struct` target 的 tag 规则

对顶层导出字段，只认两种 `query` tag：

- `query:"name"`
- `query:"-"`

规则固定为：

- 未标注字段一律忽略，并在成功绑定后保持原值
- 未导出字段一律忽略；即使带 `query` tag，也不参与 tag 校验、绑定或重复 key 声明检测
- `query:"-"` 一律显式忽略；不参与绑定、重复 key 声明检测或字段类型校验，且在成功绑定后保持原值
- `query:"name"` 中的 `name` 必须非空，且不得包含前后空白
- `query:"name"` 的 key 按 tag 字面值原样参与匹配；不做 trim、大小写归一化或额外解码
- 两个导出字段不得声明同一个 `query:"name"` key；否则属于 usage error
- 导出字段上的其他 tag 形式都属于 usage error

### 3.3 支持的字段形状

`query:"name"` 只支持以下顶层字段形状：

| 字段形状                              | 是否支持 | 缺失参数时   | 命中参数时                         |
| ------------------------------------- | -------- | ------------ | ---------------------------------- |
| `string`                              | 是       | 保持零值     | 直接写入首值                       |
| `bool`                                | 是       | 保持零值     | 按 `strconv.ParseBool` 解析        |
| `int` / `int8` / `int16` / `int32` / `int64` | 是 | 保持零值     | 按十进制 `strconv.ParseInt` 解析   |
| `uint` / `uint8` / `uint16` / `uint32` / `uint64` | 是 | 保持零值 | 按十进制 `strconv.ParseUint` 解析  |
| `float32` / `float64`                 | 是       | 保持零值     | 按 `strconv.ParseFloat` 解析，且只接受有限值 |
| 命名标量类型                          | 是       | 保持零值     | 按其底层标量家族规则解析后写入     |
| `time.Duration`                       | 是       | 保持零值     | 按 `time.ParseDuration` 解析       |
| `time.Time`                           | 是       | 保持零值     | 按严格 RFC3339 解析               |
| `uuid.UUID`                           | 是       | 保持零值     | 按 `uuid.Parse` 解析               |
| 指向上述受支持叶子类型的一级指针 `*T` | 是       | 保持 `nil`   | 先解码到临时值，成功后再分配或覆盖 |

除上表外，其他字段形状一律不支持。

这意味着以下形状都属于 usage error：

- `query:",inline"`
- 未标记展开的 `struct` / `*struct`
- slice / array
- map / interface
- 多级指针
- 自定义 `encoding.TextUnmarshaler` 类型

### 3.4 query source 语义

公开语义固定为：

- `request.URL == nil` 视为空 query source
- 当 `request.URL != nil` 时，query 解析语义跟随 Go `1.25.9` `net/url`
- raw query 解析失败属于客户端输入错误，不是 usage error
- raw query 解析失败时，返回稳定 `400 bad_request`，且 target 零修改
- `query:"name"` 的 tag key 必须与解析后的 query key 做精确字符串匹配
- query source 采用默认单值模型：同名 key 出现多个值时，视为客户端输入错误
- 对 `struct` 目标，未知 key 默认忽略
- 缺失 key 不会继承已绑定字段的旧值；对应已绑定字段保持零值状态
- 参数存在但首值为空字符串时，仍视为“已提交参数”
- 只有 `string`、底层为 `string` 的命名标量及其一级指针接受空字符串；其他受支持类型把空字符串当解析失败处理

与单字段 helper `Query(...)` 不同，`BindQuery(...)` 不是“只取当前关心字段”的局部读取工具。
一旦调用它，就表示当前 handler 选择对这次请求的 query source 做一次批量投影；
因此 raw query 非法、重复 key 或参与绑定字段不可解码，都属于当前入口要拒绝的客户端输入。

### 3.5 `map[string]string` target

对 `*map[string]string` 的规则固定为：

- 成功绑定后，target 替换为当前请求的“解析后单值字符串快照”
- 旧项必须被移除，不允许保留历史 key
- 空 query 绑定后得到可用空 map
- 任一 key 命中多个值时，返回 `400 bad_request` 且 target 零修改
- 不做类型解码

## 4. 错误模型

### 4.1 usage error

以下场景返回普通 error：

- `nil request`
- `nil target`
- 非指针 target
- typed-nil target
- 不支持的 target 形状
- 非法 `query` tag
- 两个导出字段声明同一个 query key
- 不支持的字段类型

### 4.2 客户端输入错误

以下场景返回稳定 `*hah.HTTPError`：

- raw query 解析失败
- query source 中存在重复 key
- 受支持字段类型解析失败
- 空字符串落到非字符串家族字段

稳定契约只锁以下结果：

- `errors.As(err, *hah.HTTPError)` 必须成功
- `Status() == 400`
- `Code() == "bad_request"`

## 5. 与其他文档的关系

- `BindQuery(...)` 与 `Query(...)` 共享“空字符串视为已提交参数”“单值入口拒绝重复值”的输入方向
- `BindQuery(...)` 是 DTO binder；`Query(...)` 是单字段 helper
- `BindQuery(...)` 比 `Query(...)` 更严格，因为它还要处理 DTO 规则、整条 query source 和一次性提交语义
- `Query(...)` 允许“只读取显式关心的 key，并忽略其他未消费 query 输入”；`BindQuery(...)` 则承担当前 query source 的整体绑定责任
- 顶层错误模型来自 `hah.HTTPError`；对外如何写成默认错误 envelope 也由 `hah.WriteError(...)` 决定

## 6. 测试基线

后续实现或重构至少应锁住：

- target 前置条件：`nil request`、`nil target`、typed-nil、非指针以及不支持的 target 形状都返回 usage error
- `request.URL == nil` 视为空 query source
- raw query 解析遵循 Go `1.25.9` `net/url` 的代表性语义；解析失败返回 `400 bad_request` 且 target 零修改
- `*map[string]string` 成功绑定后替换为当前请求的解析后单值快照；空 query 得到空 map，且旧项会被清除
- `struct` 目标只绑定顶层导出且显式标注 `query:"name"` 的字段；未标注字段、未导出字段和 `query:"-"` 字段始终忽略并保持原值
- 非法 `query` tag、不支持字段类型以及两个导出字段声明同一个 query key 都返回 usage error
- query key 按解析后字符串精确匹配；未知 key 默认忽略
- query source 中任一重复 key 返回 `400 bad_request` 且 target 零修改
- 缺失已绑定参数不会继承 target 旧值；成功绑定时只更新参与绑定的字段
- 空字符串参数视为存在；只有字符串家族接受空字符串
- 代表性覆盖基础标量、命名标量、`time.Duration`、`time.Time`、`uuid.UUID` 及其一级指针的成功 / 失败路径
- `time.Time` 会拒绝非法 RFC3339 offset；`float` 只接受有限值
- 客户端输入错误统一收敛为 `400 bad_request`，且 target 零修改
