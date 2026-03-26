# hah

[![Go Reference](https://pkg.go.dev/badge/github.com/kanata996/hah.svg)](https://pkg.go.dev/github.com/kanata996/hah)
[![CI](https://github.com/kanata996/hah/workflows/CI/badge.svg)](https://github.com/kanata996/hah/actions/workflows/ci.yml)
[![Codecov](https://codecov.io/github/kanata996/hah/graph/badge.svg)](https://codecov.io/github/kanata996/hah)

`hah` 是一个面向 `net/http` 生态的 JSON API 边界层。

它不接管 router，不引入新的 framework runtime，也不要求你改写现有 middleware / handler 组织方式。它只聚焦进入业务边界之后的几件事：

- 用统一的 JSON envelope 写成功响应和错误响应
- 在失败点显式写回错误，而不是依赖隐式控制流
- 把内部错误映射成稳定的公开 HTTP 错误
- 处理 JSON / query 输入解码与校验
- 在错误处理链路里附带可观测信息和 `request_id`

如果你在用 `chi`、原生 `net/http`，或其他构建在 `http.Handler` 之上的路由栈，并且想统一 API 边界行为而不接受“再套一层框架”，`hah` 就是为这个场景设计的。

## 定位

可以把 `hah` 理解成一层很薄的 API 边界辅助层：

- 外层 middleware 继续做 auth、rate limit、CORS、recover、access log
- 业务边界内的 handler 继续保持标准 `http.Handler`
- `hah` 只负责输入解码、错误定型、统一写回和错误观测

它适合：

- 你想统一成功响应和错误响应的 JSON 结构
- 你想把内部错误集中映射成稳定的公开错误码
- 你想继续保留 `chi` / `net/http` 的原生写法
- 你想要显式错误处理，而不是全局魔法式 runtime

它不负责：

- 接管或包装你的 router
- 接管 auth、rate limit、redirect、challenge、panic recover
- 统一 router 级 `404/405` 或框架级 `500`
- 提供 path / header / form 的全量 binding runtime
- 替代 tracing、access log、全局观测基础设施

## 核心模型

`hah` 的接入可以压缩成 5 个动作：

1. 外层 middleware 继续处理接入层职责
2. 在业务边界 route group 上可选地挂 `Contract(...)`
3. 用 `Decode*` / `Validate` 处理输入
4. 在失败点调用 `WriteError(...)`
5. 在成功路径调用 `Respond*`

最重要的几个 API：

- `Contract(...)`
- `WriteError(...)`
- `Respond(...)` / `RespondWithMeta(...)` / `RespondEmpty(...)`
- `DecodeJSON(...)` / `DecodeQuery(...)` / `Validate(...)`

几个需要提前知道的约束：

- `Contract(...)` 是可选的，但当你需要按路由子树生效的 mapper / reporter，以及响应已开始写出的跟踪能力时，推荐显式挂载
- `WriteError(...)` 收到 `nil` 时返回 `false`；收到非 `nil` 错误时会立即完成映射、观测和写回
- 一旦响应已经开始写出，`hah` 不会为了“统一格式”再偷偷改写结果
- `Respond*` 只负责成功响应，`WriteError(...)` 只负责错误响应

## 安装

`hah` 当前要求 `Go 1.24+`。

```bash
go get github.com/kanata996/hah
```

## 推荐接法

推荐主路径是：

- 外层继续用 `chi` / `net/http` middleware
- 在 feature 或 route group 边界挂 `Contract(...)`
- handler 内统一使用 `WriteError(...)` 和 `Respond*`
- service / repository 保持内部错误语义，通过 mapper 转成公开 HTTP 错误

下面是最小化的 `chi` 接法：

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
		r.Use(hah.Contract(hah.WithContractErrorMappers(mapUserError)))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			var query struct {
				ID string `query:"id"`
			}

			if hah.WriteError(w, r, hah.DecodeQuery(r, &query)) {
				return
			}

			user, err := loadUser(query.ID)
			if hah.WriteError(w, r, err) {
				return
			}

			if err := hah.Respond(w, http.StatusOK, user); hah.WriteError(w, r, err) {
				return
			}
		})
	})

	log.Fatal(http.ListenAndServe(":8080", r))
}
```

这条主路径的好处是：

- `chi` 继续负责路由和 middleware 组织
- handler 里没有手写错误 JSON 的样板代码
- service / repository 不需要直接依赖 HTTP 状态码
- 错误映射收敛在业务边界，而不是散落在各层

## 另一种接法：直接返回 `*hah.HTTPError`

如果你的项目规模较小，或者你接受内部层直接携带 HTTP 语义，也可以不挂 `Contract(...)`，直接在 handler 或 service 中构造 `*hah.HTTPError` 并交给 `WriteError(...)`：

```go
func getUser(w http.ResponseWriter, r *http.Request) {
	user, err := loadUser(r.URL.Query().Get("id"))
	if err != nil {
		hah.WriteError(w, r, hah.NotFound("user_not_found", "user not found"))
		return
	}

	_ = hah.Respond(w, http.StatusOK, user)
}
```

这个模式更简单，但会让内部层更早感知公开 HTTP 语义。推荐把它留给：

- 小型服务
- 标准库直写项目
- 明确接受“service 直接返回公开错误”的团队约定

对应示例见：

- [`_examples/chi`](./_examples/chi)
- [`_examples/nethttp`](./_examples/nethttp)

## 响应契约

`hah` 只定义它自己写出的三种响应形态：

1. 带响应体的成功响应
2. 带响应体的错误响应
3. 不带响应体的空成功响应

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

- `Respond(...)` 输出 `{"data": ...}`
- `RespondWithMeta(...)` 输出 `{"data": ..., "meta": ...}`
- `RespondEmpty(...)` 只写状态码，不写响应体
- `data` 必须存在，且不能编码成 JSON `null`
- `meta` 可省略；如果存在，必须编码成 JSON object
- `error.code` 必须是稳定机器码
- `error.message` 必须是可安全公开的文案
- `error.details` 始终存在；没有内容时输出 `[]`

这个契约只覆盖这些入口：

- `Respond(...)`
- `RespondWithMeta(...)`
- `RespondEmpty(...)`
- `WriteError(...)`

如果响应来自 auth、rate limit、redirect、router 级 `404/405`、panic recover 或其他外层 middleware，它的响应体不属于 `hah` 契约。

## 错误处理与映射

`hah` 的错误模型是“在失败点显式写回”，而不是“把错误状态藏在 runtime 里等待统一收尾”。

`WriteError(...)` 的处理顺序是：

1. 如果错误本身是 `*HTTPError`，直接写出
2. 如果错误是 `reqx.Problem`，自动适配成 `hah` 的公开错误
3. 否则按 mapper 顺序匹配
4. 没命中时回退为 `500 internal_error`

常用入口：

- `Contract(hah.WithContractErrorMappers(...))`
- `hah.WriteError(w, r, err, hah.WithErrorMappers(...))`
- `hah.NewHTTPError(...)`
- `hah.BadRequest(...)`
- `hah.Unauthorized(...)`
- `hah.Forbidden(...)`
- `hah.NotFound(...)`
- `hah.MethodNotAllowed(...)`
- `hah.Conflict(...)`
- `hah.Gone(...)`
- `hah.UnprocessableEntity(...)`
- `hah.TooManyRequests(...)`

推荐实践：

- route / feature 级 mapper 放在 `Contract(...)`
- 单个 handler 的 one-shot 覆盖放在 `WriteError(..., hah.WithErrorMappers(...))`
- 业务错误码自己定义稳定字符串常量；公共可复用 code 放在 [`errcode`](./errcode) 子包

`Contract(...)` 不是 `WriteError(...)` 生效的前提，但在下面这些场景里很有价值：

- 你想在一个路由子树下集中挂 mapper
- 你想挂按路由子树生效的 reporter
- 你想获得响应开始写出后的跟踪能力

## 错误观测

`hah` 在处理错误时会发出 `ErrorReport`：

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

- `WithContractErrorReporter(...)` 为整个 `Contract(...)` 子树设置 reporter
- `WithErrorReporter(...)` 为单次 `WriteError(...)` 调用覆盖 reporter
- 传 `nil` 可以关闭 `hah` 的错误观测

默认 reporter 会把内部错误和写响应退化记录到结构化 stderr；普通业务 `4xx` 默认不单独记错误日志。

## 请求解码与校验

根包里的请求输入 helper 是对 [`reqx`](./reqx) 子包的轻量封装，推荐业务边界代码直接使用 `hah` 根包 API：

- `DecodeJSON(...)`
- `DecodeAndValidateJSON(...)`
- `DecodeQuery(...)`
- `DecodeAndValidateQuery(...)`
- `Validate(...)`

JSON 解码特性：

- 接受 `application/json` 和 `+json` Content-Type
- 默认最大请求体大小为 `1 MiB`
- 默认拒绝未知字段
- 可通过 `AllowUnknownFields()` 放宽未知字段限制
- 可通过 `AllowEmptyBody()` 接受空 body

Query 解码特性：

- 解码到带 `query:"..."` tag 的 struct 字段
- 默认拒绝未知 query 参数
- 支持标量、切片、指针，以及实现 `encoding.TextUnmarshaler` 的类型
- 可通过 `AllowUnknownQueryFields()` 放宽未知字段限制

校验特性：

- 通过 `Validate(...)` 或 `DecodeAndValidate*` 返回 `[]hah.Violation`
- 违反约束时统一写成 `422 invalid_request`
- violation 结构包含 `field`、`code`、`message`

如果你明确想依赖更窄的子包，也可以直接使用 `reqx`。`hah.WriteError(...)` 会自动把 `reqx.Problem` 归一化到 `hah` 的公开错误契约里。

## Request ID

`hah` 不要求你预先安装 request id middleware。

默认行为是：

- 第一次需要发送错误观测时，缺失的 request id 会被惰性生成
- 同一个错误处理链路内会复用同一个 request id
- 经过 `Contract(...)` 的请求，如果同一次请求里发生多次 `WriteError(...)`，这些观测也会复用同一个 request id

如果项目已经有 request id 机制，可以显式桥接给 `hah`：

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

`SetRequestID(...)` 只影响 `hah` 的错误观测链路，不替代 access log、trace 或分布式链路追踪方案。

## 示例与常用命令

示例目录：

- [`_examples/chi`](./_examples/chi)：推荐主路径，内部错误通过 mapper 收敛到边界
- [`_examples/nethttp`](./_examples/nethttp)：直接返回 `*hah.HTTPError` 的标准库接法

两个示例目录都是独立 Go module，可以直接运行：

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

## 已知限制

- `Respond(...)` / `RespondWithMeta(...)` 是一次性 JSON envelope writer，不提供流式响应能力
- `DecodeJSON(...)` 会先完整读取请求体再 decode，不适合超大 body 或流式场景
- `WriteError(...)` 在无法拿到带跟踪能力的 writer 时，只能基于已暴露的状态码和已写出字节数判断响应是否已经开始
- 如果错误响应里的 `details` 无法编码，`hah` 会降级为 `details: []`，而不是把整个公开错误改写成另一个不兼容结果
- `HEAD` 请求在真实 `net/http` server 上仍由标准库负责抑制响应体；如果直接用 `httptest.ResponseRecorder` 断言 body，可能会看到编码后的内容
- `SetRequestID(...)` 只负责桥接 request id，不负责全局 tracing 协议

## 兼容性

当前版本的公开兼容边界包括：

- `hah` 根包公开 API
- `hah` 自己写出的 HTTP 可观察行为

版本策略：

- `v1.0.0` 之前，minor release 仍可能包含破坏性调整，但会在 [CHANGELOG](./CHANGELOG.md) 中明确标注
- `v1.0.0` 之后，破坏根包 API 或 HTTP 契约的变更应只出现在新的 major version

## 许可证

[MIT](./LICENSE)
