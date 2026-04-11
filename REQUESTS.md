# 请求输入指南

这份文档聚焦 `hah` 的输入侧能力，覆盖两个核心包：

- `bind`：负责把 path / query / header / body 绑定到 Go 目标值
- `reqx`：负责 typed request helper、Normalize、请求级规则和字段校验

`hah` 是 `net/http`-first 的设计，不提供额外的请求上下文抽象，而是围绕标准库 `*http.Request`、显式 binding 和显式 validation 组合能力来组织 API。

## 先看选型

| 目标 | 推荐 API | 说明 |
| --- | --- | --- |
| 只取 1 到 2 个 path / query 参数 | `hah.PathParam` / `hah.QueryParam` | 适合轻量读取，不必定义 DTO |
| 单字段 path / query 需要 `required` / `invalid` violation | `hah.PathValuesBinder` / `hah.QueryParamsBinder` | 适合不定义 DTO，但希望保留 source-aware 结构化错误 |
| 标准 handler 的默认 happy path | `hah.BindAndValidate` | 默认执行 `path -> query(GET/DELETE/HEAD) -> body`，再做 Normalize / RequestValidator / validator |
| 只做 body 绑定，不做校验 | `hah.BindBody` | 适合只需要 JSON 解码的场景 |
| 显式只绑定 query / path / header / body | `bind.BindQueryParams` / `bind.BindPathValues` / `bind.BindHeaders` / `bind.BindBody` | source-specific binding |
| 显式只校验某一类来源 | `reqx.Validate(..., reqx.SourceQuery)` / `reqx.SourcePath` / `reqx.SourceHeader` / `reqx.SourceBody` | 通常和 `bind.Bind*` 配合使用 |
| body 是否必须存在 | `reqx.RequireBody` | 适合在 `RequestValidator` 里声明 body-required 契约 |

## 读取 request 数据

### 原始字符串读取

`hah` 不再包装一个额外的 request reader 类型。原始字符串值直接走标准库：

```go
id := r.PathValue("id")
cursor := r.URL.Query().Get("cursor")
```

这类读取保持 `net/http` 原生形态，不额外包装 request reader 类型。

### typed path / query helper

如果你需要的是“直接拿到目标类型”，优先用 `reqx` 或根包 facade：

```go
import (
	"github.com/google/uuid"
	"github.com/kanata996/hah"
)

func handler(w http.ResponseWriter, r *http.Request) {
	accountID, err := hah.PathParam[uuid.UUID](r, "account_id")
	if err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	limit, err := hah.QueryParam[int](r, "limit")
	if err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}

	_, _ = accountID, limit
}
```

几个公开语义需要注意：

- 缺失值返回零值；例如 `QueryParam[int]` 缺失时返回 `0`
- 如果要区分“缺失”和“有值”，用指针类型，例如 `QueryParam[*uuid.UUID]`
- query 多值支持 slice：`?tag=a&tag=b` 可以用 `QueryParam[[]string](r, "tag")`
- 对标量目标，重复 query 只取第一个值
- 支持 `bind.BindUnmarshaler` 和 `encoding.TextUnmarshaler`

例如：

```go
tags, err := hah.QueryParam[[]string](r, "tag")
cursor, err := hah.QueryParam[*uuid.UUID](r, "cursor")
```

### source-specific value binder

如果你不想定义 DTO，但希望 path/query 单字段错误返回 `Violation` 风格，可以用 `ValueBinder`：

```go
var (
	accountID uuid.UUID
	limit     int
)

if err := hah.PathValuesBinder(r).
	MustBind("account_id", &accountID).
	BindErrors(); err != nil {
	_ = hah.WriteError(w, r, err)
	return
}

if err := hah.QueryParamsBinder(r).
	Bind("limit", &limit).
	BindErrors(); err != nil {
	_ = hah.WriteError(w, r, err)
	return
}
```

几个公开语义需要注意：

- `Bind(name, &dst)`：参数缺失时 no-op
- `MustBind(name, &dst)`：参数缺失时返回 `required` violation
- 常用目标可直接用 typed shorthand，例如 `MustString`、`MustInt`、`MustStrings`、`MustUUID`
- 时间支持显式格式方法：
  - `Time` / `MustTime`：RFC3339
  - `UnixTime` / `MustUnixTime`：10 位秒级 Unix 时间戳
  - `UnixMilliTime` / `MustUnixMilliTime`：13 位毫秒级 Unix 时间戳
- 参数存在但解析失败时，返回 `invalid` violation
- `PathValuesBinder` 的 `in` 固定为 `path`；`QueryParamsBinder` 的 `in` 固定为 `query`
- 默认 `FailFast(true)`；如果需要一次收集多个字段错误，显式调用 `FailFast(false)`
- `BindError()` 返回首个错误；`BindErrors()` 返回聚合后的 `invalid_request`

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

这就是默认的 mixed-source 绑定模型。

### 显式单一来源 binding

当你只想绑定某一类来源时，直接用 `bind` 包：

```go
var query ListAccountsQuery
if err := bind.BindQueryParams(r, &query); err != nil {
	return err
}

var path AccountPath
if err := bind.BindPathValues(r, &path); err != nil {
	return err
}

var headers DeleteHeaders
if err := bind.BindHeaders(r, &headers); err != nil {
	return err
}

var body CreateAccountBody
if err := bind.BindBody(r, &body); err != nil {
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

然后这个类型既可以被 `bind.Bind*` 使用，也可以被 `hah.PathParam` / `hah.QueryParam` 使用。

## 用 `reqx` 做校验和请求级规则

`reqx` 负责 typed request helper、Normalize、请求级规则、字段校验和 violation 包络，把输入后的校验流程集中在同一层。

### 默认 mixed-source happy path

默认推荐入口是：

```go
if err := hah.BindAndValidate(r, &req); err != nil {
	_ = hah.WriteError(w, r, err)
	return
}
```

这个入口内部会执行：

1. `bind.Bind`
2. `Normalize()`
3. `ValidateRequest(*http.Request)`
4. `validator/v10`

### 显式来源校验

如果你显式用了 `bind.Bind*`，校验阶段应明确告诉 `reqx` 当前来源：

```go
var headers DeleteHeaders
if err := bind.BindHeaders(r, &headers); err != nil {
	return err
}
if err := reqx.Validate(r, &headers, reqx.SourceHeader); err != nil {
	return err
}
```

`Source` 会影响两件事：

- validator 字段别名优先读取哪些 tag
- violation 的 `in` 字段写成 `body` / `query` / `path` / `header` / `request`

当前可用值：

- `reqx.SourceBody`
- `reqx.SourceQuery`
- `reqx.SourcePath`
- `reqx.SourceHeader`
- `reqx.SourceRequest`

### Normalize

DTO 如果实现了 `Normalize()`，会在字段校验前执行：

```go
type CreateAccountRequest struct {
	Name string `json:"name" validate:"required,min=3"`
}

func (r *CreateAccountRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
}
```

### RequestValidator

如果 DTO 需要字段之间的组合规则，或要访问原始 request，可以实现 `RequestValidator`：

```go
type SearchRequest struct {
	Query  string `query:"query"`
	Cursor string `query:"cursor"`
}

func (r *SearchRequest) ValidateRequest(*http.Request) error {
	if r.Query == "" && r.Cursor == "" {
		return reqx.InvalidRequest(reqx.Violation{
			Field: "query",
			In:    reqx.ViolationInQuery,
			Code:  reqx.ViolationCodeRequired,
		})
	}
	return nil
}
```

### body 必填

默认情况下，空 body 会沿用 `bind` 的 no-op 语义，不会自动报错。  
如果某个 DTO 需要“必须显式提交 body”，在 `ValidateRequest` 里调用 `reqx.RequireBody(r)`：

```go
type CreateRequest struct {
	Name string `json:"name" validate:"required"`
}

func (*CreateRequest) ValidateRequest(r *http.Request) error {
	return reqx.RequireBody(r)
}
```

### violation 包络

`reqx` 会把字段校验错误统一映射为稳定的 violation 结构：

```json
{
  "field": "name",
  "in": "body",
  "code": "required",
  "detail": "is required"
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
	OrgID string `param:"org_id" validate:"required"`
	Name  string `query:"name"`
}

func (r *ListAccountsRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
}

func handler(w http.ResponseWriter, r *http.Request) {
	var req ListAccountsRequest
	if err := hah.BindAndValidate(r, &req); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}
}
```

### 2. POST：path + body

```go
type CreateAccountRequest struct {
	OrgID string `param:"org_id" validate:"required"`
	Name  string `json:"name" validate:"required,min=3,max=64"`
}

func (r *CreateAccountRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
}
```

这类 handler 仍然优先用 `hah.BindAndValidate`。

### 3. 显式 header 绑定 + header 校验

```go
type DeleteHeaders struct {
	Actor string `header:"x-actor" validate:"required,nospace"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	var headers DeleteHeaders
	if err := bind.BindHeaders(r, &headers); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}
	if err := reqx.Validate(r, &headers, reqx.SourceHeader); err != nil {
		_ = hah.WriteError(w, r, err)
		return
	}
}
```

### 4. 只拿一个 path / query 参数

```go
orgID, err := hah.PathParam[string](r, "org_id")
limit, err := hah.QueryParam[int](r, "limit")
```

这种场景不必为了一个参数单独创建 DTO。

## 注意事项

### 为 binding 定义单独 DTO

建议把“外部输入 DTO”与“内部业务对象”分开，避免把不应该由请求写入的字段暴露出去。

### `reqx.Validate` 目标必须是 `*struct`

`bind.BindBody` 可以绑定到 `map`、`slice` 等 JSON 目标；但 `reqx.Validate` / `hah.BindAndValidate` 最终会执行结构校验，因此要求目标是非 nil 的 `*struct`。

### path 依赖 `net/http` 的 `PathValue` / `Pattern`

`reqx` 和 `bind` 的 path 语义依赖标准库 `PathValue` / `Pattern`。  
如果你接在 `chi` 后面，需要先把 `chi.RouteContext` 回填到 `net/http` 契约；可以参考：

- [_examples/chi/main.go](./_examples/chi/main.go)

## 相关文件

- [README.md](./README.md)
- [_examples/nethttp](./_examples/nethttp)
- [_examples/chi](./_examples/chi)
