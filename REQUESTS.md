# 请求输入指南

这份文档聚焦 `hah` 的输入侧能力，覆盖两个核心包：

- `bind`：负责把 path / query / header / body 绑定到 Go 目标值
- `reqx`：负责 request helper、`RequireBody`、`InvalidRequest` 和公开 violations

`hah` 是 `net/http`-first 的设计，不提供额外的请求上下文抽象，也不内建 validation engine。它围绕标准库 `*http.Request`、显式 binding 和显式 post-bind validation 组织 API。

## 先看选型

| 目标 | 推荐 API | 说明 |
| --- | --- | --- |
| 单字段 path / query 读取并顺手做常见校验 | `hah.Path` / `hah.Query` | 适合不定义 DTO，但希望直接返回 source-aware `required` / `invalid` 错误 |
| 自定义类型或结构化输入 | `bind.Bind*` / `hah.Bind*` | 复杂类型、多值 query、自定义解码统一走 `bind` |
| 默认 mixed-source 绑定 | `hah.Bind` | 默认执行 `path -> query(GET/DELETE/HEAD) -> body` |
| 只做 body 绑定 | `hah.BindBody` | 适合只需要 JSON 解码的场景 |
| 显式只绑定 query / path / header / body | `hah.BindQueryParams` / `hah.BindPathValues` / `hah.BindHeaders` / `hah.BindBody` | 常用 source-specific binding；底层实现仍在 `bind` 包 |
| body 是否必须存在 | `hah.RequireBody` / `reqx.RequireBody` | 适合在绑定完成后显式声明 body-required 契约 |
| 手写字段级请求违规 | `reqx.InvalidRequest` | 适合把业务前的输入错误收敛成统一 `422 invalid_request` |

## 读取 request 数据

### 原始字符串读取

`hah` 不再包装一个额外的 request reader 类型。原始字符串值直接走标准库：

```go
id := r.PathValue("id")
cursor := r.URL.Query().Get("cursor")
```

### 单参数读取与常见校验

如果你不想定义 DTO，但希望 path/query 单字段读取时直接得到 `required` / `invalid` 风格错误，优先用 `reqx` 或根包 facade：

```go
import "github.com/kanata996/hah"

func handler(w http.ResponseWriter, r *http.Request) {
	accountID, err := hah.Path(r, "account_id").
		String().
		Required().
		Get()
	if err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	limit, err := hah.Query(r, "limit").
		Int().
		Default(20).
		Min(1).
		Max(100).
		Get()
	if err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	_, _ = accountID, limit
}
```

如果 query 需要保留重复 key 的全部原始值，可以直接走 `Values()` / `Strings()`：

```go
tags, err := hah.Query(r, "tag").Values().Get()
```

公开语义：

- `Path(r, name)` / `Query(r, name)` 先指定来源，再选择类型；`Path` 只暴露 path 适用的单值能力，`Query` 额外支持 `Values()` / `Strings()` 读取重复 query 的原始值
- `Required()`：参数缺失时返回 `required` violation
- `Default(v)`：参数缺失时使用默认值；与 `Required()` 互斥
- 常见快捷校验直接链式表达，例如 `Min`、`Max`、`MinLen`、`MaxLen`、`OneOf`、`Match`、`Before`、`After`
- `Check(...)` 作为通用兜底校验；返回的非 nil error 会映射成 `invalid` violation
- `Get()` 返回最终值；参数存在但解析失败或校验失败时，返回 `invalid_request`
- `?name=` 这类空串算“存在”；如果要限制空串，配合 `MinLen(1)`、`Match(...)` 或 `Check(...)`

## 用 `bind` 绑定 DTO

`bind` 只负责 source-to-DTO 映射，不做 Normalize、请求级规则或字段校验。

### struct tag 绑定

```go
type ListAccountsRequest struct {
	OrgID string `param:"org_id"`
	Name  string `query:"name"`
}
```

标签含义：

- `param`：path 参数
- `query`：query 参数
- `header`：header
- `json`：JSON body

### 默认 mixed-source binding

`bind.Bind` 和 `hah.Bind` 会按固定顺序绑定：

1. path
2. query，仅对 `GET` / `DELETE` / `HEAD`
3. body

后一个来源会覆盖前一个来源的同名字段。

```go
type Request struct {
	ID   string `param:"id" query:"id" json:"id"`
	Name string `json:"name"`
}

var req Request
if err := hah.Bind(r, &req); err != nil {
	return err
}
```

### 显式单一来源 binding

当你只想绑定某一类来源时，优先用根包 facade；需要 `bind` 的错误码常量或更底层类型时，再直接导入 `bind` 包：

```go
var query ListAccountsQuery
if err := hah.BindQueryParams(r, &query); err != nil {
	return err
}

var path AccountPath
if err := hah.BindPathValues(r, &path); err != nil {
	return err
}

var headers DeleteHeaders
if err := hah.BindHeaders(r, &headers); err != nil {
	return err
}

var body CreateAccountBody
if err := hah.BindBody(r, &body); err != nil {
	return err
}
```

注意：

- header 不参与默认 `Bind`
- 如果你需要 header，请显式调用 `BindHeaders`

### body 契约

`bind.BindBody` 当前的公开契约是：

- 实际读取到零字节 body 时视为 no-op
- 这个 no-op 发生在 `Content-Type` 检查之前
- 非空 body 只接受 `application/json`
- 默认使用标准库 `encoding/json`
- 不接受 `application/*+json`

如果 body 非法，会返回稳定的公开错误，例如：

- `invalid_json`
- `unsupported_media_type`
- `request_too_large`

### 自定义解码

对于 path / query / header 这类字符串输入，`bind` 支持自定义解码：

```go
type Timestamp time.Time

func (t *Timestamp) UnmarshalParam(src string) error {
	parsed, err := time.Parse(time.RFC3339, src)
	*t = Timestamp(parsed)
	return err
}
```

除 `UnmarshalParam(string) error` 外，当前默认 binder 还支持：

- 字段实现 `encoding.TextUnmarshaler`
- `time.Time` 字段配合 `format:"..."` tag 做按格式解析
- 重复 query/header 值绑定到切片字段；如果目标字段需要自行消费全部重复值，可以实现 `UnmarshalParams([]string) error`

示例：

```go
type SearchQuery struct {
	At   time.Time `query:"at" format:"2006-01-02"`
	Tags []string  `query:"tag"`
}
```

## 绑定后的显式校验

`hah` 不预设 DTO 的校验方式。绑定完成后，调用方自己决定下一步是手写校验、接入第三方库，还是映射到应用层命令再校验。

### 1. 手写校验

```go
type CreateAccountRequest struct {
	OrgID string `param:"org_id"`
	Name  string `json:"name"`
}

func validateCreateAccountRequest(r *http.Request, req *CreateAccountRequest) error {
	if err := hah.RequireBody(r); err != nil {
		return err
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return reqx.InvalidRequest(reqx.Violation{
			Field: "name",
			In:    reqx.ViolationInBody,
			Code:  reqx.ViolationCodeRequired,
		})
	}
	return nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := hah.Bind(r, &req); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}
	if err := validateCreateAccountRequest(r, &req); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}
}
```

### 2. 继续用你自己的验证库

`hah` 不解析 `validate:"..."` 这类 tag，但也不会阻止你继续用它们。struct tag 可以并存：

```go
type CreateAccountRequest struct {
	OrgID string `param:"org_id"`
	Name  string `json:"name" validate:"required,min=3,max=64"`
}
```

绑定后直接调用你自己的验证库即可：

```go
var req CreateAccountRequest
if err := hah.Bind(r, &req); err != nil {
	return err
}
if err := myValidator.Struct(&req); err != nil {
	return translateValidationError(err)
}
```

也就是说，`hah` 不绑定 `validator/v10`，但 DTO 依然可以自带你自己的 tag。

### 3. 映射到应用层对象再校验

如果你不想让 DTO 带任何验证元数据，绑定完成后再映射到应用层命令：

```go
type CreateAccountRequest struct {
	OrgID string `param:"org_id"`
	Name  string `json:"name"`
}

type CreateAccountCommand struct {
	OrgID string
	Name  string
}

func handler(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := hah.Bind(r, &req); err != nil {
		return
	}

	cmd := CreateAccountCommand{
		OrgID: req.OrgID,
		Name:  strings.TrimSpace(req.Name),
	}
	if err := service.CreateAccount(r.Context(), cmd); err != nil {
		return
	}
}
```

## `reqx` 的输入辅助 helper

### `RequireBody`

默认情况下，空 body 会沿用 `bind` 的 no-op 语义，不会自动报错。  
如果某个 handler 需要“必须显式提交 body”，在绑定完成后显式调用：

```go
if err := hah.RequireBody(r); err != nil {
	return err
}
```

### `InvalidRequest`

`reqx.InvalidRequest(...)` 会把调用方自己构造的 violations 收敛成统一的 `422 invalid_request`：

```go
return reqx.InvalidRequest(
	reqx.Violation{
		Field:  "limit",
		In:     reqx.ViolationInQuery,
		Code:   reqx.ViolationCodeInvalid,
		Detail: "must be between 1 and 100",
	},
)
```

默认错误结构：

```json
{
  "title": "Unprocessable Entity",
  "status": 422,
  "detail": "request contains invalid fields",
  "code": "invalid_request",
  "errors": [
    {
      "field": "limit",
      "in": "query",
      "code": "invalid",
      "detail": "must be between 1 and 100"
    }
  ]
}
```

常用公开常量：

- `reqx.CodeInvalidRequest`
- `reqx.ViolationCodeInvalid`
- `reqx.ViolationCodeRequired`
- `reqx.ViolationCodeUnknown`
- `reqx.ViolationCodeType`
- `reqx.ViolationCodeMultiple`

## 常见模式

### 1. GET：path + query

```go
type ListAccountsRequest struct {
	OrgID string `param:"org_id"`
	Name  string `query:"name"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	var req ListAccountsRequest
	if err := hah.Bind(r, &req); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
}
```

### 2. POST：path + body + 显式校验

```go
type CreateAccountRequest struct {
	OrgID string `param:"org_id"`
	Name  string `json:"name"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := hah.Bind(r, &req); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}
	if err := validateCreateAccountRequest(r, &req); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}
}
```

### 3. 显式 header 绑定 + 手写 header 校验

```go
type DeleteHeaders struct {
	Actor string `header:"x-actor"`
}

func validateDeleteHeaders(headers *DeleteHeaders) error {
	headers.Actor = strings.TrimSpace(headers.Actor)
	if headers.Actor == "" {
		return reqx.InvalidRequest(reqx.Violation{
			Field: "X-Actor",
			In:    reqx.ViolationInHeader,
			Code:  reqx.ViolationCodeRequired,
		})
	}
	return nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	var headers DeleteHeaders
	if err := hah.BindHeaders(r, &headers); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}
	if err := validateDeleteHeaders(&headers); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}
}
```

### 4. 只拿一个 path / query 参数

```go
orgID, err := hah.Path(r, "org_id").String().Required().Get()
limit, err := hah.Query(r, "limit").Int().Default(20).Min(1).Max(100).Get()
```

## 注意事项

### 为 binding 定义单独 DTO

建议把“外部输入 DTO”与“内部业务对象”分开，避免把不应该由请求写入的字段暴露出去。

### `BindBody` 仍可绑定到非 struct

`bind.BindBody` 可以绑定到 `map`、`slice` 等 JSON 目标。是否接受这类输入、如何进一步校验，由调用方自行决定。

### path 依赖 `net/http` 的 `PathValue` / `Pattern`

`reqx` 和 `bind` 的 path 语义依赖标准库 `PathValue` / `Pattern`。  
如果你接在 `chi` 后面，需要先把 `chi.RouteContext` 回填到 `net/http` 契约；可以参考：

- [_examples/chi/main.go](./_examples/chi/main.go)

## 相关文件

- [README.md](./README.md)
- [_examples/nethttp](./_examples/nethttp)
- [_examples/chi](./_examples/chi)
