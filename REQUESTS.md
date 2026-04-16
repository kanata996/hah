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
| 批量 query DTO 绑定 | `hah.BindQuery` / `reqx.BindQuery` | 适合筛选条件、分页参数、复杂多值 query |
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

如果 query 需要保留重复 key 的全部原始值，可以直接走 `Values()`：

```go
tags, err := hah.Query(r, "tag").Values().Get()
```

公开语义：

- `Path(r, name)` / `Query(r, name)` 先指定来源，再选择类型
- `Path` 只暴露 path 适用的单值能力
- `Query` 额外支持 `Values()` 读取重复 query 的原始值
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

## 用 `reqx` 绑定 DTO

`reqx` 里的 DTO binder 只负责 source-to-DTO 映射，不做 Normalize、请求级规则或字段校验。

### query DTO 绑定

`reqx.BindQuery` / `hah.BindQuery` 只从 query 参数绑定数据。

```go
type ListAccountsQuery struct {
	Name  string   `query:"name"`
	Tags  []string `query:"tag"`
	Limit int      `query:"limit"`
}

var query ListAccountsQuery
if err := hah.BindQuery(r, &query); err != nil {
	return err
}
```

当前 query binder 的公开语义：

- 只消费 `query:"..."` tag
- 目标必须是 struct、`map[string]string`、`map[string][]string` 或 `map[string]any`
- query 名字按精确值匹配
- 重复 query 绑定到切片字段时会保留全部值
- 重复 query 绑定到标量字段时默认只消费第一个值
- 缺失参数不会覆盖 DTO 现有值
- 绑定不是事务性的；如果返回错误，DTO 可能已经被部分更新，调用方不应继续依赖其精确状态
- 绑定在遇到第一个字段错误时停止；后续字段不会继续处理
- 命名的未打 `query` tag 的 `*struct` 字段属于不支持的 DTO 形状，会直接返回普通错误
- 如果目标 DTO/tag 形状本身非法，直接返回普通错误，不收敛成 `400 bad_request`

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
- 这个 no-op 发生在 `Content-Type` 检查之前
- 非空 body 只接受 `application/json`
- 默认使用标准库 `encoding/json`
- 不接受 `application/*+json`
- 解码直接作用在传入目标上；JSON 里缺失的字段不会覆盖 DTO 现有值
- 绑定不是事务性的；如果返回错误，DTO 可能已经被部分更新，调用方不应继续依赖其精确状态

`reqx.RequireBody` / `hah.RequireBody` 与 `BindBody` 共享同一个非破坏性 body-presence probe：

- 可以先 `RequireBody(...)` 再 `BindBody(...)`
- 也可以先 `BindBody(...)` 再决定是否显式要求 body 必填
- 这两条路径都不会因为额外探测而把 body 提前消费掉

如果 body 非法，会返回稳定的公开错误，例如：

- `invalid_json`
- `unsupported_media_type`
- `request_too_large`

### 自定义解码

对于 query 这类字符串输入，`reqx` 支持自定义解码：

```go
type Timestamp time.Time

func (t *Timestamp) UnmarshalParam(src string) error {
	parsed, err := time.Parse(time.RFC3339, src)
	*t = Timestamp(parsed)
	return err
}
```

除 `reqx.BindUnmarshaler`（`UnmarshalParam(string) error`）外，当前默认 query binder 还支持：

- 字段实现 `encoding.TextUnmarshaler`
- `time.Duration` 字段，使用和 `Query(...).Duration()` 一致的 duration 字符串语法
- `time.Time` 字段配合 `format:"..."` tag 做按格式解析
- 重复 query 值绑定到切片字段
- 如果目标字段需要自行消费全部重复值，可以实现 `reqx.BindMultipleUnmarshaler`（`UnmarshalParams([]string) error`）

示例：

```go
type SearchQuery struct {
	At      time.Time     `query:"at" format:"2006-01-02"`
	Timeout time.Duration `query:"timeout"`
	Tags    []string      `query:"tag"`
}
```

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
