# hah

[![Go Reference](https://pkg.go.dev/badge/github.com/kanata996/hah.svg)](https://pkg.go.dev/github.com/kanata996/hah)
[![CI](https://github.com/kanata996/hah/workflows/CI/badge.svg)](https://github.com/kanata996/hah/actions/workflows/ci.yml)
[![Codecov](https://codecov.io/github/kanata996/hah/graph/badge.svg)](https://codecov.io/github/kanata996/hah)

`hah` 是一个保持 `net/http` 原生兼容、同时适配 `chi` 等路由栈的业务边界 JSON API 契约库，用来把进入业务边界后的请求解码、错误映射、响应写回和边界日志收敛成一套稳定约定。

## 特性

- 统一业务边界内成功响应与错误响应的 JSON 结构
- 集中映射、观测并写回显式边界错误，错误观测关联 `request_id`
- 兼容 `go-chi` 与标准 `http.Handler` 中间件链
- 兼容原生 `net/http` / `ServeMux`，不依赖第三方框架运行时
- 不接管 router，不绑定项目结构，也不引入新的框架 runtime
- 提供请求体解码、query 解码和输入校验封装
- 默认采用 fail-closed 策略，响应一旦开始写出就不再改写

## 适用场景

- 你在使用 `chi`、原生 `net/http`，或其他构建在 `net/http` 之上的路由/中间件栈，想统一业务边界内的成功响应、错误响应和请求校验行为
- 你希望 middleware、handler、service 继续沿用标准 `net/http` 风格
- 你想把内部错误稳定地映射为公开错误，而不是在深层直接写 HTTP 响应

## 边界

`hah` 负责这些事：

- 统一业务边界内成功响应和显式边界错误的 JSON 契约
- 把 `WriteError(...)` 送进来的错误集中映射、观测和写回
- 提供请求解码、查询参数解码和输入校验封装

`hah` 不负责这些事：

- 接管或包装你的 router
- 统一 auth、rate limit、CORS、redirect、challenge、router 级 `404/405` 等业务边界之前的响应
- 接管系统级 panic recovery
- 提供 body / query / header / path 的全量绑定运行时
- 承诺所有 `500` 都返回统一 JSON，panic 或外层 recoverer 的行为不属于 `hah` 契约

## 核心约定

1. 外层 middleware 继续处理 auth、rate limit、CORS、recover、access log 等接入层职责
2. 推荐在业务边界路由树上显式挂载 `Contract(...)`
3. 用 `Decode*` / `Validate` 处理进入业务边界后的请求输入
4. 用 `WriteError(...)` 在失败点显式写出统一错误响应
5. 用 `Respond` / `RespondWithMeta` / `RespondEmpty` 显式写回成功结果

- `Contract(...)` 是推荐的边界挂载点，但不是 `WriteError(...)` 生效的前提
- 直接返回 `*HTTPError`，或在调用点显式传入 `WithErrorMappers(...)` 时，不挂 `Contract(...)` 也能工作
- `WriteError(...)` 负责立即映射、观测并写回错误响应
- `Respond*` 只负责成功响应
- panic 不属于 `hah` 的默认职责，应由外层 recoverer 处理
- 如果响应已经开始写回，`hah` 不会为了“统一格式”去改写已经发出的结果

如果你只记四组名字，记住这些就够了：

- `Contract`
- `WriteError`
- `Respond*`
- `Decode*` / `Validate`

## 安装

`hah` 当前要求 `Go 1.24+`。

```bash
go get github.com/kanata996/hah
```

## 快速开始

下面是推荐主路径的最小化 `chi` 接法：在业务边界子路由上显式挂载 `Contract(...)`，然后在 handler 内继续显式使用 `WriteError` + `Respond*`。auth、rate limit、recover 这类接入层职责按项目需要自行放在外层 middleware。

```go
package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/kanata996/hah"
)

var errUserNotFound = errors.New("user not found")
const codeUserNotFound = "user_not_found"

func mapUserError(err error) *hah.HTTPError {
	if errors.Is(err, errUserNotFound) {
		return hah.NotFound(codeUserNotFound, "user not found")
	}
	return nil
}

func main() {
	r := chi.NewRouter()

	r.Route("/users", func(r chi.Router) {
		r.Use(hah.Contract(hah.WithContractErrorMappers(mapUserError)))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			var query struct {
				ID string `query:"id"`
			}

			if hah.WriteError(w, r, hah.DecodeQuery(r, &query)) {
				return
			}
			if query.ID == "missing" {
				hah.WriteError(w, r, errUserNotFound)
				return
			}

			if err := hah.Respond(w, http.StatusOK, map[string]any{
				"id": query.ID,
			}); hah.WriteError(w, r, err) {
				return
			}
		})
	})

	log.Fatal(http.ListenAndServe(":8080", r))
}
```

接入建议：

- 外层 middleware 继续处理 auth、rate limit、CORS、recover、access log 等职责
- 推荐在进入业务边界的 route group 上挂一次 `r.Use(hah.Contract(...))`
- 进入业务边界后的 handler 使用 `if hah.WriteError(w, r, err) { return }`
- route 或 feature 级的业务 mapper 更推荐收敛到对应的 `Contract(...)`
- 单个 handler 的 one-shot 覆盖更适合放在 `WriteError(..., hah.WithErrorMappers(...))` 调用点
- `hah` 生成的兜底 request id 只保证错误链路内稳定，不替代外层 access log / tracing 的 request id 策略
- 如果项目已经有 request id 机制，可通过 `hah.SetRequestID(...)` 注入给 `hah`
- 如果 handler / service 已经直接返回 `*hah.HTTPError`，不挂 `Contract(...)` 也可以直接配合 `WriteError(...)` 使用

如果你继续使用标准库，也有 `ServeMux` 版本示例：

- [`_examples/chi`](./_examples/chi)
- [`_examples/nethttp`](./_examples/nethttp)

## 响应契约

`hah` 对外只定义三种由自身写出的响应形态：

1. 带响应体的成功响应
2. 带响应体的错误响应
3. 不带响应体的空响应

成功响应：

```json
{
  "data": {},
  "meta": {}
}
```

错误响应：

```json
{
  "error": {
    "code": "invalid_request",
    "message": "request contains invalid fields",
    "details": []
  }
}
```

约束：

- `data` 必须存在，且不能编码成 `null`
- `meta` 没内容时可省略，如果存在，必须编码成 JSON object
- `error.code` 必须是稳定机器码
- `hah` 通过 `errcode` 子包公开了一组常见 code 常量，见下文；业务错误也可以继续使用你自己定义的稳定字符串常量
- `error.message` 必须是可安全公开的文案
- `error.details` 始终存在，没有内容时输出 `[]`
- `Respond` / `RespondWithMeta` 只能用于允许携带响应体的成功状态码
- `RespondEmpty` 用于不带响应体的成功响应

这个契约只覆盖下面这些入口写出的响应：

- `Respond(...)`
- `RespondWithMeta(...)`
- `RespondEmpty(...)`
- `WriteError(...)`

如果响应来自 auth、rate limit、CORS、redirect 这类外层 middleware，或 router 级 `404/405`、panic、router / framework 级 recoverer 返回的 `500`，是否带响应体、响应体长什么样，都不属于 `hah` 契约。

## 错误处理与映射

`hah` 的错误处理模型不是“在各处直接拼错误 JSON”，而是“让错误在业务边界发生点被显式写回，并由边界统一定型”。

核心 API：

```go
func Contract(opts ...ContractOption) func(http.Handler) http.Handler
func WriteError(w http.ResponseWriter, r *http.Request, err error, opts ...ErrorOption) bool
```

使用约束：

- `Contract(...)` 是可选但推荐的最里层边界 middleware，用来显式声明业务路由子树进入 `hah` 契约层
- `Contract(...)` 会为 `WriteError(...)` 提供 route-scoped 配置，并安装 started-response tracking
- `WriteError(...)` 接收任意 `error`；如果命中 `*HTTPError` 会直接写公开错误，否则再按 mapper 和默认规则归一化
- 对 `WriteError(...)` 传入 `nil` 会返回 `false`
- `WriteError(...)` 会立即完成映射、观测和写回
- 当请求经过 `Contract(...)` 时，`WriteError(...)` 会先继承 route-scoped 配置，再叠加调用点 options
- 如果响应已经开始写回，`WriteError(...)` 会尽量避免改写已发送结果

`Contract(...)` 与 `WriteError(...)` 故意使用两套 option 类型：

- `ContractOption` 负责 route-level 的边界配置
- `ErrorOption` 负责单次 `WriteError(...)` 调用的 one-shot 配置
- 这样可以避免把“挂载边界”和“写一次错误”混成同一层语义

公开用法上，`WriteError(...)` 支持两条都合法的路径：

- 直接传 `*HTTPError` 或能被 `errors.As` 命中 `*HTTPError` 的错误；这时不强制依赖 `Contract(...)`
- 传内部错误，再通过 `WithErrorMappers(...)` / `WithContractErrorMappers(...)` 把公开语义收敛到边界层

公开错误类型：

```go
type HTTPError struct {
	// unexported fields
}

func NewHTTPError(status int, code, message string, details ...any) *HTTPError
func BadRequest(code, message string, details ...any) *HTTPError
func Unauthorized(code, message string, details ...any) *HTTPError
func Forbidden(code, message string, details ...any) *HTTPError
func NotFound(code, message string, details ...any) *HTTPError
func MethodNotAllowed(code, message string, details ...any) *HTTPError
func Conflict(code, message string, details ...any) *HTTPError
func Gone(code, message string, details ...any) *HTTPError
func UnprocessableEntity(code, message string, details ...any) *HTTPError
func TooManyRequests(code, message string, details ...any) *HTTPError

func (e *HTTPError) Error() string
func (e *HTTPError) Status() int
func (e *HTTPError) Code() string
func (e *HTTPError) Message() string
func (e *HTTPError) Details() []any
```

常见公开 code 常量统一放在 `errcode` 子包：

```go
import "github.com/kanata996/hah/errcode"
```

- `errcode` 可复用的公共 code，主要覆盖协议/边界错误、请求解码与校验错误，以及少量跨业务的泛语义错误
- 为避免过早把完整枚举承诺成稳定契约，README 不再列出全部常量，请以当前版本的 `errcode` 包定义为准
- 这些常量只是便捷入口，`NewHTTPError(...)`、`NotFound(...)` 等 helper 仍然接受任意自定义稳定字符串
- 业务错误建议自己定义常量，只有像 `resource_not_found` 这类确实跨业务复用的语义，才值得回收到 `errcode`

约束：

- `4xx` 表达客户端侧失败
- `5xx` 表达服务端侧失败
- `NewHTTPError(...)` 会把非法状态码保守规范化为 `500`
- 常见 `4xx` 可以直接用 helper：`BadRequest(...)`、`Unauthorized(...)`、`Forbidden(...)`、`NotFound(...)`、`MethodNotAllowed(...)`、`Conflict(...)`、`Gone(...)`、`UnprocessableEntity(...)`、`TooManyRequests(...)`
- 当 `code` / `message` 为空时，会按状态码族补默认值
- 未识别错误默认回退为 `500 internal_error`

错误映射与观测：

```go
type ErrorMapper func(err error) *HTTPError

func WithErrorMappers(mappers ...ErrorMapper) ErrorOption
func WithContractErrorMappers(mappers ...ErrorMapper) ContractOption

type ErrorReport struct {
	Request         *http.Request
	Error           error
	PublicError     *HTTPError
	Stage           string
	RequestID       string
	ResponseStarted bool
}

type ErrorReporter func(ErrorReport)

func WithErrorReporter(reporter ErrorReporter) ErrorOption
func WithContractErrorReporter(reporter ErrorReporter) ContractOption
func SetRequestID(r *http.Request, id string) *http.Request
```

- `ErrorReport.Stage` 是内部观测字段，当前主路径稳定值包括 `decode`、`validate`、`processing`、`write_response`
- `ErrorReport.RequestID` 表示当前错误观测实际使用的 request id；优先使用 `SetRequestID(...)` 注入的值
- 如果调用方没有显式设置 request id，`hah` 会在第一次发送错误观测时自动生成一个，并在同一次错误处理链路里复用
- `processing` 表示请求已进入业务处理链，覆盖 handler、service、repository 这段内部处理范围
- 当错误响应在序列化或写回阶段再次失败时，`hah` 会额外发送一条 `Stage == "write_response"` 的内部观测，并保守回退为 `500 internal_error`
- 默认结构化 stderr 日志会记录 `5xx` 与 `write_response` 失败；普通业务 `4xx` 默认不单独记错误日志
- `WithErrorReporter(nil)` 会关闭 `hah` 的错误观测

`RequestID` 接入建议：

- 默认不需要额外安装 request id middleware；缺失时 `hah` 会在错误路径内部自动生成并复用 request id
- 如果项目已经有 request id 机制，可在应用层取出该值，再通过 `hah.SetRequestID(...)` 注入给 `hah`
- `net/http` 项目可在自己的 middleware 里生成 request id，然后把返回的 `*http.Request` 继续向下传递
- 不建议让 `hah` 根包直接依赖某个框架的 request id context 约定
- `RequestID` 不是 `TraceID`，两者不应混用

推荐做法：

- 如果你希望内部层不直接感知 HTTP 语义，就把内部业务错误通过 `ErrorMapper` 映射成公开边界错误
- 如果 handler / service 已经直接返回 `*HTTPError`，`WriteError(...)` 也可以直接工作，不强制要求 `Contract(...)`
- 在交付层定义局部 helper，把常用 mapper 显式传给 `WriteError(...)`
- 单个 handler 的临时覆盖更适合放在 `WriteError(..., hah.WithErrorMappers(...))`
- 如果少量 mapper 确实需要全局复用，可通过 `WithErrorMappers(...)` 复用配置片段
- route 或 feature 级 mapper 更推荐通过 `WithContractErrorMappers(...)` 挂到 `Contract(...)`
- 把鉴权失败、限流、参数错误这类边界错误优先写成 `hah.Unauthorized(...)`、`hah.TooManyRequests(...)`、`hah.BadRequest(...)` 这类 helper；不常见状态再回退到 `hah.NewHTTPError(...)`
- 不再提供 runtime 风格的隐式错误状态传播；错误处理路径统一收敛为显式 `WriteError(...)`

## 成功响应 API

```go
func Respond(w http.ResponseWriter, status int, data any) error
func RespondWithMeta(w http.ResponseWriter, status int, data any, meta any) error
func RespondEmpty(w http.ResponseWriter, status int) error
```

- `Respond(...)` 返回 `{"data": ...}`
- `RespondWithMeta(...)` 返回 `{"data": ..., "meta": ...}`
- `RespondEmpty(...)` 只写状态码，不写响应体
- `RespondWithMeta(...)` 的 `meta` 如果存在，必须编码成 JSON object

## 请求解码与校验

根包的请求输入 helper 是对 `reqx` 子包的一层轻量封装。推荐应用边界代码优先使用 `hah.Decode*` / `hah.Validate`，这样可以把依赖面收敛在 `hah` 根包。

公开 API：

```go
type DecodeOption = reqx.DecodeOption
type QueryOption = reqx.QueryOption
type Violation = reqx.Violation
type ValidateFunc[T any] func(*T) []Violation

func WithMaxBodyBytes(limit int64) DecodeOption
func AllowUnknownFields() DecodeOption
func AllowEmptyBody() DecodeOption
func AllowUnknownQueryFields() QueryOption

func DecodeJSON[T any](r *http.Request, dst *T, opts ...DecodeOption) error
func DecodeAndValidateJSON[T any](r *http.Request, dst *T, fn ValidateFunc[T], opts ...DecodeOption) error
func DecodeQuery[T any](r *http.Request, dst *T, opts ...QueryOption) error
func DecodeAndValidateQuery[T any](r *http.Request, dst *T, fn ValidateFunc[T], opts ...QueryOption) error
func Validate[T any](dst *T, fn ValidateFunc[T]) error
```

这些 helper 会把请求解码和输入校验错误适配到 `hah` 的统一错误响应中。

它们的职责边界是：

- 负责 JSON 请求体解码
- 负责 URL query 解码
- 负责把 `[]Violation` 归一化成稳定的 `422 invalid_request`

它们不负责：

- 拥有 router
- 接管完整响应生命周期
- path param / header / form 的全量 binding runtime
- 业务规则建模本身

如果你明确想直接依赖更窄的子包，也可以直接使用 [`reqx`](./reqx)。`reqx.Problem` 仍然可以通过 `WriteError(...)` 归一化到 `hah` 的统一公开错误契约中。

## 示例

仓库同时提供 `chi` 与 `net/http` 示例，可根据项目接法直接选择：

- [`_examples/chi`](./_examples/chi)：推荐主路径
- [`_examples/nethttp`](./_examples/nethttp)：标准库兼容示例

两个目录都是独立 Go module，可以直接运行：

```bash
cd _examples/chi
go test ./...
go run .

cd ../nethttp
go test ./...
go run .
```

仓库根目录常用命令：

```bash
make test
make test-cover
make test-race
make bench
make ci
```

## 复用 `chi` 的 Request ID

`hah` 默认会在错误链路里惰性生成并复用 request id。只有当你希望 `ErrorReport.RequestID` 与 `chi/middleware.RequestID`、access log 或网关日志对齐时，才需要显式桥接。

```go
func bindChiRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqID := middleware.GetReqID(r.Context()); reqID != "" {
			r = hah.SetRequestID(r, reqID)
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(bindChiRequestID)
	r.Use(middleware.Recoverer)
}
```

说明：

- `chi` 负责生成或传播 request id
- bridge middleware 负责把应用最终采用的值显式注入给 `hah`
- `hah` 根包因此不需要依赖 `chi` 的 context 协议

## 已知限制

- `WriteError(...)` 在拿不到可观测 writer 状态时，只能基于当前 `http.ResponseWriter` 能暴露的信息判断响应是否已经开始；如果项目非常依赖 started-response 保护，可在业务边界最内层使用一层很薄的 tracking middleware
- `Respond(...)` / `RespondWithMeta(...)` 的成功载荷仍然要求可被标准库 `encoding/json` 正常编码；错误响应里的 `details` 如果混入不可编码值，会在内部观测中记录并降级为 `details: []`，而不是把整个公开错误改写成 `500`
- `reqx.DecodeJSON(...)` 当前会在受 `WithMaxBodyBytes(...)` 约束下先完整读取请求体，再执行 JSON decode；这适合常规 JSON API 请求，但不是面向超大 body 或流式 decode 的运行时
- `Respond(...)` / `RespondWithMeta(...)` 是一次性 envelope writer，不提供流式响应能力
- `SetRequestID(...)` 只负责把 request id 桥接进 `hah` 的错误观测链路，不替代外层 access log、trace 或分布式链路追踪策略

## 兼容性

本项目当前的公开兼容边界由本文档中描述的两类内容构成：

- 根包公开 API
- 由 `hah` 自己写出的 HTTP 可观察行为

版本策略：

- 在 `v1.0.0` 之前，minor release 仍可能包含破坏性调整，但会在 [CHANGELOG](./CHANGELOG.md) 中明确标注
- 在 `v1.0.0` 之后，破坏根包公开 API 或 HTTP 契约的变更应只出现在新的 major version

## 许可证

[MIT](./LICENSE)
