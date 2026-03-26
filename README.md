# hah

[![Go Reference](https://pkg.go.dev/badge/github.com/kanata996/hah.svg)](https://pkg.go.dev/github.com/kanata996/hah)
[![CI](https://github.com/kanata996/hah/workflows/CI/badge.svg)](https://github.com/kanata996/hah/actions/workflows/ci.yml)
[![Codecov](https://codecov.io/github/kanata996/hah/graph/badge.svg)](https://codecov.io/github.com/kanata996/hah)

`hah` 是一个面向 `net/http` / `chi` 生态的 JSON API 边界层。

它不接管 router，不定义新的 handler 协议，也不试图包装整个 HTTP 生命周期。它只聚焦业务边界里的几件事：

- 统一 JSON 成功响应和错误响应
- 显式解码请求、显式写回成功结果、显式写回错误结果
- 把内部错误映射成稳定的公开 HTTP 错误
- 在错误路径上发出统一的 `ErrorReport`
- 桥接或生成 `request_id`

## 定位

`hah` 适合下面这种场景：

- 你已经在用 `chi` 或原生 `net/http`
- 你不想再套一层 framework runtime
- 你想统一 JSON API 的公开行为
- 你想把 mapper / reporter 挂在业务边界，而不是散在每个 handler 里

它不负责：

- auth / challenge / rate limit / CORS / redirect
- router 级 `404/405`
- panic recover
- websocket / streaming runtime
- access log / tracing / metrics 基础设施

## 当前模型

推荐接法很简单：

1. 外层 middleware 继续处理接入层职责
2. 在业务边界 route group 上可选地挂 `WithResponses(...)`
3. 用 `Decode*` / `Validate` 处理输入
4. 成功路径用 `Render(...)` / `RenderWithMeta(...)` / `RenderEmpty(...)`
5. 失败路径在发生点调用 `RenderError(...)`

最重要的 API：

- `WithResponses(...)`
- `Render(...)`
- `RenderWithMeta(...)`
- `RenderEmpty(...)`
- `RenderError(...)`
- `SuccessStatus(...)`
- `Status(...)`
- `DecodeJSON(...)` / `DecodeQuery(...)` / `Validate(...)`

## 快速示例

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

func loadUser(id string) (map[string]any, error) {
	if id == "missing" {
		return nil, errUserNotFound
	}
	return map[string]any{"id": id}, nil
}

func mapUserError(err error) *hah.HTTPError {
	if errors.Is(err, errUserNotFound) {
		return hah.NotFound("user_not_found", "user not found")
	}
	return nil
}

func main() {
	r := chi.NewRouter()

	r.Route("/users", func(r chi.Router) {
		r.Use(hah.WithResponses(hah.ErrorMappers(mapUserError)))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			var query struct {
				ID string `query:"id"`
			}

			if err := hah.DecodeQuery(r, &query); err != nil {
				_ = hah.RenderError(w, r, err)
				return
			}

			user, err := loadUser(query.ID)
			if err != nil {
				_ = hah.RenderError(w, r, err)
				return
			}

			if err := hah.Render(w, r, user); err != nil {
				_ = hah.RenderError(w, r, err)
				return
			}
		})
	})

	log.Fatal(http.ListenAndServe(":8080", r))
}
```

## 响应契约

`hah` 只定义它自己写出的三种响应形态：

1. 成功响应
2. 成功响应加 `meta`
3. 错误响应

成功响应：

```json
{
  "data": {}
}
```

成功响应带 `meta`：

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

- `Render(...)` 输出 `{"data": ...}`
- `RenderWithMeta(...)` 输出 `{"data": ..., "meta": ...}`
- `RenderEmpty(...)` 只写状态码，不写 body
- `data` 不能编码成 JSON `null`
- `meta` 如果存在，必须编码成 JSON object
- `error.details` 始终存在；为空时输出 `[]`

## 错误处理与映射

`RenderError(...)` 的行为是：

1. 如果错误本身是 `*HTTPError`，直接写出
2. 如果错误是 `reqx.Problem`，自动桥接到 `hah` 公开错误
3. 否则按 mapper 顺序匹配
4. 没命中时回退为 `500 internal_error`
5. 写回前先发 `ErrorReport`
6. 如果响应已经开始，只做观测，不再改写公开响应

常用构造器：

- `NewHTTPError(...)`
- `BadRequest(...)`
- `Unauthorized(...)`
- `Forbidden(...)`
- `NotFound(...)`
- `MethodNotAllowed(...)`
- `Conflict(...)`
- `Gone(...)`
- `UnprocessableEntity(...)`
- `TooManyRequests(...)`

推荐实践：

- feature / route 级 mapper 放在 `WithResponses(...)`
- 单个调用点的 one-shot 覆盖放在 `RenderError(..., ErrorMappers(...))`
- 业务错误码自己定义稳定字符串常量；通用 code 可复用 [`errcode`](./errcode)

## `WithResponses(...)`

`WithResponses(...)` 是一个很薄的业务边界 middleware。

它当前只负责：

- 提供 route-scoped error mapper / reporter 配置
- 提供最小的 route-scoped 成功响应默认值
- 复用 request-scoped state
- 让同一请求中的多次 `RenderError(...)` 共享 request id 和响应开始状态

它不负责：

- 接管 panic
- 接管整个 HTTP 生命周期
- 自动写成功响应
- 在请求结束时延迟回收错误

目前唯一的 success-side 默认能力是：

- `SuccessStatus(status)`：为 `Render(...)`、`RenderWithMeta(...)` 和 `RenderEmpty(..., 0)` 提供默认成功状态码
- handler 内显式调用 `Status(r, status)` 仍然优先

## 错误观测

`hah` 在错误路径会发出 `ErrorReport`：

```go
type ErrorReport struct {
	Request         *http.Request
	Error           error
	PublicError     *HTTPError
	RequestID       string
	ResponseStarted bool
}
```

使用方式：

- `ErrorMappers(...)` 可用于整个 `WithResponses(...)` 子树，也可用于单次 `RenderError(...)` 调用
- `ErrorReporter(...)` 可用于整个 `WithResponses(...)` 子树，也可用于单次 `RenderError(...)` 调用
- `SuccessStatus(...)` 只用于 `WithResponses(...)`
- 传 `nil` 可关闭 `hah` 默认 reporter

默认 reporter 会记录：

- `401` / `403` 安全事件
- `5xx` 内部错误

普通业务 `4xx` 默认不单独打错误日志。

## 请求解码与校验

根包里的请求输入 helper 是对 [`reqx`](./reqx) 子包的轻量封装：

- `DecodeJSON(...)`
- `DecodeAndValidateJSON(...)`
- `DecodeQuery(...)`
- `DecodeAndValidateQuery(...)`
- `Validate(...)`
- `InvalidRequest(...)`

JSON 解码默认行为：

- 接受 `application/json` 和 `+json`
- 缺失 `Content-Type` 时仍允许解码
- 默认最大 body 为 `1 MiB`
- 默认拒绝 unknown fields
- 默认拒绝空 body

Query 解码默认行为：

- 解码到带 `query:"..."` tag 的 struct 字段
- 默认拒绝未知 query 参数
- 支持标量、切片、指针和 `encoding.TextUnmarshaler`

如果你直接使用 `reqx` 子包，`reqx.Problem` 仍然可以直接交给 `RenderError(...)`。

## Request ID

`hah` 不要求你预先安装 request id middleware。

默认行为：

- 第一次进入错误观测路径时，缺失的 request id 会被惰性生成
- 同一次错误处理链会复用同一个 id
- 如果你已经有自己的 request id，中间件可以通过 `SetRequestID(...)` 桥接给 `hah`

示例：

```go
func bindChiRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqID := middleware.GetReqID(r.Context()); reqID != "" {
			r = hah.SetRequestID(r, reqID)
		}
		next.ServeHTTP(w, r)
	})
}
```

## 示例与命令

示例目录：

- [`_examples/chi`](./_examples/chi)
- [`_examples/nethttp`](./_examples/nethttp)

仓库根目录常用命令：

```bash
make test
make test-cover
make test-race
make bench
make ci
```

## 已知限制

- `Render(...)` / `RenderWithMeta(...)` 是一次性 JSON envelope writer，不提供流式公开 API
- `DecodeJSON(...)` 会先完整读取请求体，不适合超大 body 或流式输入
- `RenderError(...)` 只在 `hah` 自己管理的响应路径上保证统一契约
- 如果错误响应里的 `details` 无法编码，`hah` 会降级为 `details: []`
- `HEAD` 请求在真实 `net/http` server 上仍由标准库抑制响应体
- `SetRequestID(...)` 只负责桥接 request id，不替代 tracing / access log

## 兼容性

当前版本的公开兼容边界包括：

- `hah` 根包公开 API
- `hah` 自己写出的 HTTP 可观察行为

版本策略：

- `v1.0.0` 之前，minor release 仍可能包含破坏性调整
- `v1.0.0` 之后，破坏根包 API 或 HTTP 契约的变更应只出现在新的 major version

## 许可证

[MIT](./LICENSE)
