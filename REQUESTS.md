# 请求输入指南

这份文档聚焦 `hah` 的输入侧能力。默认入口是根包 `hah`，底层输入核心包是：

- `reqx`：负责 request helper、query/body binding、`RequireBody`、`InvalidRequest` 和公开 violations

`hah` 是 `net/http`-first 的设计，不提供额外的请求上下文抽象，也不内建 validation engine。它围绕标准库 `*http.Request`、显式读取和显式 post-bind validation 组织 API。

当前设计里：

- `reqx.Path(...)` / `reqx.Query(...)` 是请求侧核心 API
- `reqx.BindQuery(...)` / `reqx.BindBody(...)` 是 DTO 场景下的补充能力

## 先看选型

| 目标 | 推荐 API | 说明 |
| --- | --- | --- |
| 单字段 path / query 读取并顺手做常见校验 | `hah.Path` / `hah.Query` | 主路径，直接返回 source-aware `required` / `invalid` 错误 |
| 批量 query DTO 绑定 | `hah.BindQuery` / `reqx.BindQuery` | 适合筛选条件、分页参数、显式 DTO 投影 |
| 只做 JSON body 绑定 | `hah.BindBody` / `reqx.BindBody` | 适合 body DTO 解码 |
| body 是否必须存在 | `hah.RequireBody` / `reqx.RequireBody` | 适合在 body 绑定后显式声明 body-required 契约 |
| 手写字段级请求违规 | `hah.InvalidRequest` / `reqx.InvalidRequest` | 适合把业务前的输入错误收敛成统一 `422 invalid_request` |
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
- `Required()`：参数缺失时返回 `required` violation
- `Default(v)`：参数缺失时使用默认值；与 `Required()` 互斥
- 常见快捷校验直接链式表达，例如 `Min`、`Max`、`MinLen`、`MaxLen`、`OneOf`、`Match`、`Before`、`After`
- `Check(...)` 作为通用兜底校验；返回的非 nil error 会映射成 `invalid` violation
- `Get()` 返回最终值；参数存在但解析失败或校验失败时，返回 `invalid_request`
- `?name=` 这类空串算“存在”；如果要限制空串，配合 `MinLen(1)`、`Match(...)` 或 `Check(...)`

这套 `Path / Query` 分工是当前请求侧核心设计：

- `Path` 代表资源标识型输入，保持窄而清晰
- `Query` 代表更宽的参数语义，允许 richer scalar helpers 与重复值读取
- 调整它们的类型面、链式方法或错误语义时，应按核心 public API 变更对待

`Path(...)` 更完整的公开 API、source 语义和错误边界，见 [docs/path-design.md](./docs/path-design.md)。

`Query(...)` 更完整的公开 API、类型面和错误边界，见 [docs/query-design.md](./docs/query-design.md)。

## 用 `reqx` 绑定 DTO

`reqx` 里的 DTO binder 只负责 source-to-DTO 映射，不做 Normalize、请求级规则或字段校验。

`BindQuery(...)` 更完整的公开契约、字段白名单和演进边界，见 [docs/binding-query-design.md](./docs/binding-query-design.md)。

### query DTO 绑定

`reqx.BindQuery` / `hah.BindQuery` 只从 query 参数绑定数据。

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

- 目标必须是 struct 或 `map[string]string`
- 对于 struct，只绑定显式声明了 `query` tag 的字段
- `query:",inline"` 是唯一的嵌套 DTO 展开方式
- 普通 `query:"name"` 字段只支持常见内建标量字段及其一级指针
- query 名字按精确值匹配
- malformed raw query 返回稳定 `400 bad_request`，并保证 target 零修改
- 未知 query key 默认忽略
- 重复 query key 默认只消费第一个值
- 缺失参数不会覆盖 DTO 现有值
- DTO/tag 形状本身非法时，先返回普通错误，并保证 target 零修改
- 绑定在遇到第一个客户端输入错误时停止；后续字段不会继续处理
- 客户端输入错误下，绑定不是事务性的；如果返回错误，DTO 可能已经被部分更新

它适合“批量投影”，不适合表达请求级规则。像 `Required`、`Default`、`OneOf`、`Min/Max` 这类规则，仍然优先放在 `reqx.Query(...)` 或绑定后的显式校验里。

### body DTO 绑定

`reqx.BindBody` / `hah.BindBody` 只从请求 body 绑定数据。

```go
type CreateAccountBody struct {
	Name string `json:"name"`
}

var body CreateAccountBody
if err := hah.BindBody(r, &body); err != nil {
	return err
}
```

`reqx.BindBody` 当前的公开契约是：

- 实际读取到零字节 body 时视为 no-op
- 公开只支持非 `nil` 的 `*struct` target
- 这个 no-op 发生在 `Content-Type` 检查之前
- 非空 body 只接受 `application/json`
- 非空 body 必须恰好构成一个以 object 为顶层值的 JSON 文档，只允许前后空白
- 默认使用标准库 `encoding/json`
- 不接受 `application/*+json`
- struct 字段支持范围按公开表闭集处理；超出表格的字段类型直接返回 usage error
- 默认拒绝未知字段
- 顶层 `null`、array、string、number、boolean 返回 `invalid_json`；重复 object key 的生效结果遵循 `encoding/json`
- 截断 JSON / `unexpected EOF` 返回 `invalid_json`
- 非 JSON 语义的 body read failure 返回普通 error，不收敛成 `HTTPError`
- 绑定先解码到临时值；成功后才一次性提交，因此 JSON 里缺失的字段不会继承 DTO 旧值
- 如果返回错误，DTO 保持调用前状态，不应出现部分更新

`reqx.RequireBody` / `hah.RequireBody` 与 `BindBody` 共享同一个非破坏性 body-presence probe：

- 可以先 `RequireBody(...)` 再 `BindBody(...)`
- 也可以先 `BindBody(...)` 再决定是否显式要求 body 必填
- 这两条路径都不会因为额外探测而把 body 提前消费掉
- 零字节 body 对 `BindBody(...)` 是 no-op，对 `RequireBody(...)` 视为缺失
- 仅空白字符 body 对 `RequireBody(...)` 视为存在，但对 `BindBody(...)` 返回 `invalid_json`
- 顶层 `null` 对 `RequireBody(...)` 视为存在，但对 `BindBody(...)` 返回 `invalid_json`

如果 body 非法，会返回稳定的公开错误，例如：

- `invalid_json`
- `unsupported_media_type`
- `request_too_large`

如果失败来自非 JSON 语义的 body read 过程，例如 wrapped `Body.Read` error、transport I/O 或 `context` cancellation，则返回普通 error，而不是 `*errx.HTTPError`。

### 字段类型范围

当前默认 query binder 只支持封闭白名单中的常见字段类型：

- `string`
- `bool`
- `int`、`int8`、`int16`、`int32`、`int64`
- `uint`、`uint8`、`uint16`、`uint32`、`uint64`
- `float32`、`float64`
- 上述字段类型的一级指针 `*T`
- `query:",inline"` 的 `struct` / `*struct`

当前默认 query binder 不支持：

- 命名类型
- `time.Time`
- `time.Duration`
- `BindUnmarshaler`
- `encoding.TextUnmarshaler`
- slice、array、map、interface
- 多级指针

如果你需要时间、duration、枚举或其他自定义语义，优先先绑定为 `string`，再在绑定后显式解析。

## 绑定后的显式校验

`hah` 不预设 DTO 的校验方式。绑定完成后，调用方自己决定下一步是手写校验、接入第三方库，还是映射到应用层命令再校验。
多数 handler 直接用 `hah.InvalidRequest(...)`、`hah.Violation{...}` 就够了；如果你需要更完整的错误构造器族，再导入 `errx`。

### 1. 手写校验

```go
type CreateAccountBody struct {
	Name string `json:"name"`
}

func validateCreateAccountBody(r *http.Request, body *CreateAccountBody) error {
	if err := hah.RequireBody(r); err != nil {
		return err
	}

	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		return hah.InvalidRequest(hah.Violation{
			Field: "name",
			In:    hah.ViolationInBody,
			Code:  hah.ViolationCodeRequired,
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
	if err := validateCreateAccountBody(r, &body); err != nil {
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
	return hah.InvalidRequest(hah.Violation{
		Field: "X-Actor",
		In:    hah.ViolationInHeader,
		Code:  hah.ViolationCodeRequired,
	})
}
```
