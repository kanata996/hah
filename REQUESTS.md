# 请求输入指南

这份文档聚焦 `hah` 的输入侧能力，覆盖两个核心包：

- `bind`：负责把 path / query / header / body 绑定到 Go 目标值
- `reqx`：负责 request helper、Normalize、请求级规则和字段校验

`hah` 是 `net/http`-first 的设计，不提供额外的请求上下文抽象，而是围绕标准库 `*http.Request`、显式 binding 和显式 validation 组合能力来组织 API。

## 先看选型

| 目标 | 推荐 API | 说明 |
| --- | --- | --- |
| 单字段 path / query 读取并顺手做常见验证 | `hah.Path` / `hah.Query` | 适合不定义 DTO，但希望直接返回 source-aware `required` / `invalid` 错误 |
| 自定义类型或结构化输入 | `bind.Bind*` | 复杂类型、多值 query、自定义解码统一走 `bind`，避免再扩单字段 helper |
| 标准 handler 的默认 happy path | `hah.BindAndValidate` | 默认执行 `path -> query(GET/DELETE/HEAD) -> body`，再做 Normalize / RequestValidator / validator |
| 只做 body 绑定，不做校验 | `hah.BindBody` | 适合只需要 JSON 解码的场景 |
| 显式只绑定 query / path / header / body | `hah.BindQueryParams` / `hah.BindPathValues` / `hah.BindHeaders` / `hah.BindBody` | 常用 source-specific binding；底层实现仍在 `bind` 包 |
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

### 单参数读取与常见验证

如果你不想定义 DTO，但希望 path/query 单字段读取时直接得到 `required` / `invalid` 风格错误，优先用 `reqx` 或根包 facade：

```go
import (
	"github.com/google/uuid"
	"github.com/kanata996/hah"
)

func handler(w http.ResponseWriter, r *http.Request) {
	accountID, err := hah.Path(r, "account_id").
		UUID().
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

几个公开语义需要注意：

- `Path(r, name)` / `Query(r, name)` 先指定来源，再选择类型，例如 `String()`、`Int()`、`UUID()`、`Time()`
- 这些链式 builder 的返回类型（例如 `reqx.StringParam`、`reqx.IntParam`、`reqx.TimeParam`）也是公开 API，但大多数调用方不需要直接声明它们
- `Required()`：参数缺失时返回 `required` violation
- `Default(v)`：参数缺失时使用默认值；与 `Required()` 互斥
- 常见快捷校验直接链式表达，例如 `Min`、`Max`、`MinLen`、`MaxLen`、`OneOf`、`Match`、`Before`、`After`
- `Check(...)` 作为通用兜底校验；返回的非 nil error 会映射成 `invalid` violation
- `Get()` 返回最终值；参数存在但解析失败或校验失败时，返回 `invalid_request`
- `?name=` 这类空串算“存在”；如果要限制空串，配合 `MinLen(1)`、`Match(...)` 或 `Check(...)`

#### `Path` / `Query` 能力表

| 类型选择器 | 返回值类型 | 输入格式 | 可链式校验 | 备注 |
| --- | --- | --- | --- | --- |
| `String()` | `string` | 原样读取字符串 | `MinLen` / `MaxLen` / `OneOf` / `Match` / `Check` | 长度按 rune 数计算；`?name=` 解析为空字符串且算“存在” |
| `Int()` | `int` | 十进制整数 | `Min` / `Max` / `Check` | 空串按 `0` 解析；宽度跟随当前平台的 `int` |
| `Int64()` | `int64` | 十进制整数 | `Min` / `Max` / `Check` | 空串按 `0` 解析 |
| `Uint()` | `uint` | 无符号十进制整数 | `Min` / `Max` / `Check` | 空串按 `0` 解析 |
| `Uint64()` | `uint64` | 无符号十进制整数 | `Min` / `Max` / `Check` | 空串按 `0` 解析 |
| `Bool()` | `bool` | 符合 `strconv.ParseBool` 的布尔字面量 | `Check` | 空串按 `false` 解析 |
| `Float64()` | `float64` | 符合 `strconv.ParseFloat(..., 64)` 的数字字面量 | `Min` / `Max` / `Check` | 空串按 `0.0` 解析 |
| `Duration()` | `time.Duration` | 符合 `time.ParseDuration` 的时长字面量 | `Min` / `Max` / `Check` | 空串按 `0` 解析 |
| `UUID()` | `uuid.UUID` | 符合 `github.com/google/uuid.Parse` 的 UUID 字符串 | `Check` | 适合 path / query 中的 ID 字段 |
| `Time()` | `time.Time` | RFC3339 时间字符串 | `After` / `Before` / `Check` | `After` / `Before` 为含边界比较 |
| `UnixTime()` | `time.Time` | 10 位秒级 Unix 时间戳 | `After` / `Before` / `Check` | 解析结果为 UTC 时间 |
| `UnixMilliTime()` | `time.Time` | 13 位毫秒级 Unix 时间戳 | `After` / `Before` / `Check` | 解析结果为 UTC 时间 |

补充说明：

- 所有类型都支持 `Required()`、`Default(v)`、`Get()`；其中 `Required()` 和 `Default(v)` 互斥
- 参数缺失时，`Required()` 返回 `required` violation；参数存在但解析失败或校验失败时，返回 `invalid` violation
- 如果输入已经超出这些常见标量类型，例如自定义类型、多值 query、重复参数或结构化解码，优先改用 `bind.Bind*`

### 自定义类型输入

如果单参数不是内建标量，或者你已经需要自定义解码、重复 query、多值语义，直接交给 `bind`。这一层只保留常见标量 builder，不再为复杂类型单独扩 request helper。

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

这类自定义解码主要服务于 `bind.Bind*`。如果输入已经超出常见标量，优先定义 DTO 并让 `bind` 接管，而不是继续堆单字段 helper。

## 用 `reqx` 做校验和请求级规则

`reqx` 负责 request helper、Normalize、请求级规则、字段校验和 violation 包络，把输入后的校验流程集中在同一层。

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

如果你显式用了单一来源 binding，校验阶段应明确告诉 `reqx` 当前来源：

```go
var headers DeleteHeaders
if err := hah.BindHeaders(r, &headers); err != nil {
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
	if err := hah.BindHeaders(r, &headers); err != nil {
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
orgID, err := hah.Path(r, "org_id").String().Required().Get()
limit, err := hah.Query(r, "limit").Int().Default(20).Min(1).Max(100).Get()
```

这种场景不必为了一两个参数单独创建 DTO。

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
