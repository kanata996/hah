# 请求输入指南

这份文档聚焦 `hah` 的输入侧能力。

`hah` 是 `net/http`-first 的设计，不提供额外的请求上下文抽象，也不内建 validation engine。它围绕标准库 `*http.Request`、显式读取和显式 post-bind validation 组织 API。

当前设计里：

- `hah.Path(...)` / `hah.Query(...)` 是默认请求侧 API
- `hah.BindQuery(...)` / `hah.BindBody(...)` 是默认 DTO 绑定入口
- `reqx` 暴露同一套较低层 request-side 原生入口，并承载 `FieldError` / `Code*` / `In*` 这组输入错误公开契约；常规 handler 路径仍优先使用 `hah`

## 先看选型

| 目标 | 推荐 API | 说明 |
| --- | --- | --- |
| 单字段 path / query 读取并顺手做常见校验 | `hah.Path` / `hah.Query` | 主路径，直接返回 source-aware `required` / `invalid` 错误 |
| 批量 query DTO 绑定 | `hah.BindQuery` | 适合筛选条件、分页参数、显式 DTO 投影 |
| 只做 JSON body 绑定 | `hah.BindBody` | 适合 body DTO 解码 |
| 手写字段级 FieldError | `hah.InvalidRequest` | 适合把业务前的输入错误收敛成统一 `422 invalid_request` |
| 读取 header | `r.Header.Get(...)` / `r.Header.Values(...)` | header 默认直接走标准库 |

## 读取 request 数据

### 原始字符串读取

`hah` 不再包装一个额外的 request reader 类型。原始字符串值直接走标准库：

```go
id := r.PathValue("id")
cursor := r.URL.Query().Get("cursor")
actor := r.Header.Get("X-Actor")
```

### 单参数读取与常见校验

如果你不想定义 DTO，但希望 path/query 单字段读取时直接得到 `required` / `invalid` 风格错误，优先用根包 `hah`：

```go
import "github.com/kanata996/hah"

func handler(w http.ResponseWriter, r *http.Request) {
	accountID, err := hah.Path(r, "account_id").
		String().
		Required().
		Get()
	if err != nil {
		_ = hah.WriteError(w, err)
		return
	}

	limit, err := hah.Query(r, "limit").
		Int().
		Default(20).
		Min(1).
		Max(100).
		Get()
	if err != nil {
		_ = hah.WriteError(w, err)
		return
	}

	_, _ = accountID, limit
}
```

如果 query 需要保留重复 key 的全部解析后值，可以直接走 `Values()`：

```go
tags, err := hah.Query(r, "tag").Values().Get()
```

公开语义：

- `Path(r, name)` / `Query(r, name)` 先指定来源，再选择类型
- `Path` 只暴露 path 适用的单值能力
- `Query` 额外支持 `Values()` 读取重复 query 的解析后值
- `Required()`：参数缺失时返回 `required` field error
- `Default(v)`：参数缺失时使用默认值；与 `Required()` 互斥
- 常见快捷校验直接链式表达，例如 `Min`、`Max`、`MinLen`、`MaxLen`、`OneOf`、`Match`、`Before`、`After`
- 同类 built-in constraint 重复声明时以后一次为准；built-in constraint 总在 `Check(...)` 之前执行
- `Check(...)` 作为通用兜底校验；返回的非 nil error 会映射成 `invalid` field error
- `Get()` 返回最终值；参数存在但解析失败或校验失败时，返回 `invalid_request`
- `?name=` 这类空串算“存在”；如果要限制空串，配合 `MinLen(1)`、`Match(...)` 或 `Check(...)`
- `Query(...)` 只对当前显式读取的 key 负责；如果其他 query 参数未被本次调用读取，它们默认不会触发额外报错

这套 `Path / Query` 分工是当前请求侧核心设计：

- `Path` 代表资源标识型输入，保持窄而清晰
- `Query` 代表更宽的参数语义，允许 richer scalar helpers 与重复值读取
- 调整它们的类型面、链式方法或错误语义时，应按核心 public API 变更对待
- 后续原则上不再轻易增加新的 source root、builder family 或 tag 驱动规则

`Path(...)` 更完整的公开 API、source 语义和错误边界，见 [docs/path-design.md](./docs/path-design.md)。

`Query(...)` 更完整的公开 API、类型面和错误边界，见 [docs/query-design.md](./docs/query-design.md)。

## 绑定 DTO

默认直接用根包 `hah.BindQuery(...)` / `hah.BindBody(...)`。DTO binder 只负责 source-to-DTO 映射，不做 Normalize、请求级规则或字段校验。

`BindQuery(...)` 更完整的公开契约、顶层字段模型和演进边界，见 [docs/binding-query-design.md](./docs/binding-query-design.md)。
`BindBody(...)` 更完整的公开契约和字段支持边界，见 [docs/binding-body-design.md](./docs/binding-body-design.md)。

### query DTO 绑定

`hah.BindQuery(...)` 只从 query 参数绑定数据。

```go
type ListAccountsQuery struct {
	Name  string `query:"name"`
	Limit int    `query:"limit"`
}

var query ListAccountsQuery
if err := hah.BindQuery(r, &query); err != nil {
	return err
}
```

当前 query binder 的公开语义：

- 目标必须是 `*struct` 或 `*map[string]string`
- 对于 struct，只绑定显式声明了 `query` tag 的字段，不改写未参与绑定的其他字段
- 只支持顶层平铺字段；不展开嵌套 DTO
- 普通 `query:"name"` 字段支持常见内建标量、命名标量、`time.Time`、`time.Duration`、`uuid.UUID` 及其一级指针
- query 名字按精确值匹配
- malformed raw query 返回稳定 `400 bad_request`，并保证 target 零修改
- 对于 struct target，未知 query key 默认忽略
- 任一 query key 只要出现多个值就返回稳定 `400 bad_request`
- 缺失参数不会继承已绑定字段的旧值，而是回到这些字段的零值状态
- DTO/tag 形状本身非法时，先返回普通错误，并保证 target 零修改
- 对 `struct` target，绑定先在临时对象里重建参与绑定的字段；客户端输入错误下不会部分污染 DTO

这里的严格度故意高于 `hah.Query(...)`。
`hah.Query(...)` 是单 key 局部读取 helper；`hah.BindQuery(...)` 则表示当前 handler 正在对这次请求的 query source 做一次批量绑定。
因此 malformed raw query、重复 key 和参与绑定字段的解析失败，都属于 `BindQuery(...)` 要直接拒绝的客户端输入。

它适合“批量投影”，不适合表达请求级规则。像 `Required`、`Default`、`OneOf`、`Min/Max` 这类规则，仍然优先放在 `hah.Query(...)` 或绑定后的显式校验里。
它也不打算演进成通用 form/query decoder：不以支持嵌套 DTO、slice/map、多值自动投影、`TextUnmarshaler` 泛化或 tag 驱动校验为目标。

### body DTO 绑定

`hah.BindBody(...)` 只从请求 body 绑定数据。

```go
type CreateAccountBody struct {
	Name string `json:"name"`
}

var body CreateAccountBody
if err := hah.BindBody(r, &body); err != nil {
	return err
}
```

`hah.BindBody(...)` 当前的公开契约是：

- 公开只支持非 `nil`、且根 DTO 不自定义 `UnmarshalJSON` 的 `*struct` target
- 非空 body 只接受且只接受一个主媒体类型为 `application/json` 的 `Content-Type`
- 零字节 body 不要求 `Content-Type` 为 JSON
- 零字节 body 视为 no-op；仅空白字符 body 和顶层 `null` 返回 `invalid_json`
- 非空 body 必须恰好构成一个以 object 为顶层值的 JSON 文档，只允许前后空白
- body 超过 `1 MiB` 返回 `request_too_large`
- 默认拒绝未知字段
- 顶层 array、string、number、boolean 返回 `invalid_json`
- struct 字段解码默认跟随标准库 `encoding/json`；像 `json.RawMessage`、命名类型、字段级自定义 `UnmarshalJSON` / `UnmarshalText` 类型默认允许
- 同名 JSON object key 跟随标准库 `encoding/json` 语义，后值覆盖前值
- 截断 JSON、尾随数据、多个 top-level JSON 值都返回 `invalid_json`
- 非 JSON 语义的 body read failure 返回普通 error，不收敛成 `HTTPError`
- 绑定成功后才会提交到 target，因此 JSON 里缺失的字段不会继承 DTO 旧值；如果返回错误，DTO 保持调用前状态，不应出现部分更新

`hah` 不内建独立的 body-required helper。是否把零字节 body 视为业务错误，由调用方在 `BindBody(...)` 之后自行决定。

如果 body 非法，会返回稳定的公开错误，例如：

- `invalid_json`
- `unsupported_media_type`
- `request_too_large`

如果失败来自非 JSON 语义的 body read 过程，例如 wrapped `Body.Read` error、transport I/O 或 `context` cancellation，则返回普通 error，而不是 `*hah.HTTPError`。

### 字段解码范围

除顶层 target 必须是受支持的 `*struct`、顶层值必须是单个 JSON object、未知字段默认拒绝外，`BindBody(...)` 的字段解码公开语义直接跟随标准库 `encoding/json`：

- 根 DTO 不允许自定义 `UnmarshalJSON`
- 字段发现、嵌入字段遮蔽、命名类型、字段级自定义 decoder、`json.RawMessage`、slice / map / 指针 等行为按标准库处理
- 如果某个字段类型对当前 JSON 输入不可解码，返回 `invalid_json`

如果你需要更克制、更稳定的 DTO 面，建议由调用方自己收窄 DTO 形状，而不是依赖 binder 内建白名单。

## 绑定后的显式校验

`hah` 不预设 DTO 的校验方式。绑定完成后，调用方自己决定下一步是手写校验、接入第三方库，还是映射到应用层命令再校验。
多数 handler 直接用 `hah.InvalidRequest(...)`、`hah.FieldError{...}` 就够了；如果你需要更完整的错误构造器族，或某个更深层已经明确要返回稳定公共 HTTP 错误，继续直接使用根包提供的 `hah.NotFound(...)`、`hah.Conflict(...)`、`hah.UnprocessableEntity(...)`、`hah.InternalServer(...)` 等入口即可。
`hah` 不计划新增 `BindAndValidate`、`validate` tag 解释或 body/query/header 混合绑定入口。

### 1. 手写校验

```go
type CreateAccountBody struct {
	Name string `json:"name"`
}

func validateCreateAccountBody(body *CreateAccountBody) error {
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		return hah.InvalidRequest(hah.FieldError{
			Field: "name",
			In:    hah.InBody,
			Code:  hah.CodeRequired,
		})
	}
	return nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	orgID, err := hah.Path(r, "org_id").String().Required().Get()
	if err != nil {
		_ = hah.WriteError(w, err)
		return
	}

	var body CreateAccountBody
	if err := hah.BindBody(r, &body); err != nil {
		_ = hah.WriteError(w, err)
		return
	}
	if err := validateCreateAccountBody(&body); err != nil {
		_ = hah.WriteError(w, err)
		return
	}

	_ = orgID
}
```

### 2. 继续用你自己的验证库

`hah` 不解析 `validate:"..."` 这类 tag，但也不会阻止你继续用它们。struct tag 可以并存：

```go
type CreateAccountBody struct {
	Name string `json:"name" validate:"required,min=3,max=64"`
}
```

绑定后直接调用你自己的验证库即可。

## 典型组合

### path 用 helper，query 用 DTO

```go
orgID, err := hah.Path(r, "org_id").String().Required().Get()
if err != nil {
	return err
}

var query struct {
	Name  string `query:"name"`
	Limit int    `query:"limit"`
}
if err := hah.BindQuery(r, &query); err != nil {
	return err
}

_ = orgID
```

### path 用 helper，body 用 DTO

```go
orgID, err := hah.Path(r, "org_id").String().Required().Get()
if err != nil {
	return err
}

var body struct {
	Name string `json:"name"`
}
if err := hah.BindBody(r, &body); err != nil {
	return err
}

_ = orgID
```

### header 直接读取

```go
actor := strings.TrimSpace(r.Header.Get("X-Actor"))
if actor == "" {
	return hah.InvalidRequest(hah.FieldError{
		Field: "X-Actor",
		In:    hah.InHeader,
		Code:  hah.CodeRequired,
	})
}
```

补充：本文默认直接使用 `hah.xx`。只有当根包 facade 不满足包边界或导入约束时，才退到同契约的 `reqx.xx`。
