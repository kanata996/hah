# hah

[![Go Reference](https://pkg.go.dev/badge/github.com/kanata996/hah.svg)](https://pkg.go.dev/github.com/kanata996/hah)
[![CI](https://github.com/kanata996/hah/workflows/CI/badge.svg)](https://github.com/kanata996/hah/actions/workflows/ci.yml)
[![Codecov](https://codecov.io/github/kanata996/hah/graph/badge.svg)](https://codecov.io/github/kanata996/hah)

`hah` 是一个面向 `net/http` 的 JSON API 边界层，专注于把请求绑定、输入治理和响应写回收敛成一套稳定、克制、可组合的接口。

它只处理 HTTP 边界。它不接管 router，不定义新的 handler 协议，也不包装整个 HTTP 生命周期。你可以把它接到 `ServeMux`、`chi` 或现有中间件栈后面。

## 特性

- 面向 `net/http` 设计，保留标准 handler 和 router 控制权
- 以 `hah.Path(...)` / `hah.Query(...)` 作为默认请求侧 API，直接读取 path/query 参数
- 支持把 query、body `Bind` 到 DTO
- 聚焦 HTTP 输入绑定与边界错误，不内建 validation engine
- 把常见请求违规收敛为稳定的公开 HTTP 错误
- 内置 JSON 成功响应与 `application/problem+json` 错误响应
- 根包提供常用 facade，也支持直接使用 `reqx`、`resp`、`errx`
- 适合渐进接入现有服务，不要求整体迁移

## 安装

环境要求：

- Go `1.25+`

安装模块：

```bash
go get github.com/kanata996/hah@latest
```

大多数场景直接导入根包：

```go
import "github.com/kanata996/hah"
```

需要更细粒度控制时，也可以直接导入子包：

```go
import (
	"github.com/kanata996/hah/errx"
	"github.com/kanata996/hah/reqx"
	"github.com/kanata996/hah/resp"
)
```

## 包边界

仓库分成四个包：

- `hah`：根包 facade，聚合常用的 request helper、绑定、invalid-request helper、公共错误模型入口与响应写回入口
- `reqx`：输入侧核心包，负责 `Path` / `Query`、`BindQuery` / `BindBody`、`RequireBody`、`InvalidRequest` 和公开 violations
- `errx`：共享公共 HTTP 错误模型
- `resp`：响应侧能力，负责 JSON 成功响应和结构化错误响应

## 核心形状

当前设计里，最核心的边界表面是两条线：

- 请求侧：`hah.Path(...)` / `hah.Query(...)`
- 响应侧：`hah.WriteError(...)` / `hah.OK(...)` / `hah.Created(...)` / `hah.NoContent(...)`

其中 `reqx.BindQuery` / `reqx.BindBody` 与对应的根包 facade，是同一输入侧 API 中面向 DTO 的补充能力。`reqx` / `errx` 仍然承载底层契约，但多数 handler 默认可以只导入 `hah`。

## 适用范围

`hah` 负责：

- 绑定 query/body 到结构体
- 提供 `hah.RequireBody(...)` / `hah.InvalidRequest(...)` 这类显式输入 helper
- 暴露 `hah.Violation`、`hah.HTTPError`、`hah.NewHTTPError(...)` 等常见公共错误模型入口
- 把常见请求违规收敛成稳定的公开 HTTP 错误
- 写回标准 JSON 成功响应
- 写回 `application/problem+json` 错误响应

`hah` 不负责：

- 选择或内建 validation library
- auth / challenge / rate limit / CORS / redirect
- router 级 `404/405`
- panic recover, including panics from caller-provided `MarshalJSON` or `Error` implementations
- tracing / access log / metrics 基础设施
- websocket / streaming runtime

## 快速示例

```go
package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/kanata996/hah"
)

type createAccountRequest struct {
	Name  string `json:"name"`
}

func validateCreateAccountRequest(r *http.Request, req *createAccountRequest) error {
	if err := hah.RequireBody(r); err != nil {
		return err
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return hah.InvalidRequest(hah.Violation{
			Field: "name",
			In:    hah.ViolationInBody,
			Code:  hah.ViolationCodeRequired,
		})
	}
	return nil
}

func writeError(w http.ResponseWriter, err error) {
	if writeErr := hah.WriteError(w, err); writeErr != nil {
		log.Printf("write error response failed: %v", writeErr)
	}
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /orgs/{org_id}/accounts", func(w http.ResponseWriter, r *http.Request) {
		orgID, err := hah.Path(r, "org_id").String().Required().Get()
		if err != nil {
			writeError(w, err)
			return
		}

		var req createAccountRequest
		if err := hah.BindBody(r, &req); err != nil {
			writeError(w, err)
			return
		}
		if err := validateCreateAccountRequest(r, &req); err != nil {
			writeError(w, err)
			return
		}

		if err := hah.Created(w, map[string]any{
			"id":     "acct_123",
			"org_id": orgID,
			"name":   req.Name,
		}); err != nil {
			log.Printf("write success response failed: %v", err)
		}
	})

	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

## 根包常用 API

- `Path`
- `Query`
- `BindBody`
- `BindQuery`
- `RequireBody`
- `InvalidRequest`
- `Violation`
- `HTTPError`
- `NewHTTPError`
- `NewHTTPErrorWithCause`
- `WriteError`
- `JSON`
- `JSONBlob`
- `OK`
- `Created`
- `NoContent`

## 来源绑定与后续校验

- `hah.BindQuery` / `hah.BindBody`
- `reqx.BindQuery` / `reqx.BindBody`
- `hah.RequireBody(...)` / `hah.InvalidRequest(...)`
- `hah.NewHTTPError(...)` / `hah.NewHTTPErrorWithCause(...)`

`hah` 只负责把 HTTP 输入绑定到 DTO，不负责选择 validation 方式。绑定完成后，你可以：

- 手写校验函数，再返回 `hah.InvalidRequest(...)`
- 继续调用你自己的 `validator/v10`、`ozzo-validation` 或其他库
- 把 DTO 映射到应用层命令，再让应用层做校验

如果你需要更完整的错误构造器族或更底层的输入辅助类型，再直接导入 `errx` / `reqx`。

单字段 request helper 的边界：

- `hah.Path(...)` 面向 path segment 中的资源标识，只保留 `String()`、`UUID()`、`Int()`、`Int64()`、`Uint()`、`Uint64()`
- `hah.Query(...)` 承载更宽的参数语义，除了常见标量外，还支持 `Bool()`、`Float64()`、`Duration()`、`Time()`、`UnixTime()`、`UnixMilliTime()`
- `hah.Query(...).String()` / `Int()` / `UUID()` 等标量 helper 在重复 query key 上默认只消费第一个值
- `hah.Query(...).Values()` 可直接读取同名 query 参数的全部解析后值；如果你需要批量结构化解码，优先用 `reqx.BindQuery`

DTO binding 的边界：

- `hah.BindQuery(...)` / `reqx.BindQuery(...)` 只负责 query -> DTO 的映射，不内建请求级校验
- `hah.BindQuery(...)` / `reqx.BindQuery(...)` 的目标必须是 struct 或 `map[string]string`；对于 struct，只有显式 `query` tag 的字段会参与绑定，嵌套 DTO 需要显式写 `query:",inline"`，普通 `query:"name"` 字段只支持文档定义的常见内建标量字段及其一级指针
- `hah.BindQuery(...)` / `reqx.BindQuery(...)` 默认忽略未知 query key；重复 query key 只消费第一个值；malformed raw query 返回稳定 `400 bad_request` 且不修改 target；如果 DTO/tag 形状本身非法，也会先返回普通错误且不修改 target
- `hah.BindBody(...)` / `reqx.BindBody(...)` 只负责 JSON body -> DTO 的解码；公开只支持非 `nil` 的 `*struct` target，且字段支持范围按公开表闭集处理，超出表格即 usage error；非空 body 必须恰好构成一个以 object 为顶层值的 JSON 文档，未知字段默认拒绝；截断 JSON 仍收敛为 `invalid_json`，但非 JSON 语义的 body read failure 返回普通 error；绑定先解到临时值，成功后才一次性提交，因此失败不会污染 target，缺失字段也不会继承旧值
- `hah.RequireBody(...)` / `reqx.RequireBody(...)` 与 `BindBody(...)` 共享同一个非破坏性 body 探测；零字节 body 对 `BindBody(...)` 是 no-op、对 `RequireBody(...)` 是缺失 body；仅空白字符 body 对 `RequireBody(...)` 视为存在、对 `BindBody(...)` 视为 `invalid_json`
- `hah.BindQuery(...)` / `reqx.BindQuery(...)` 也不是事务性的；如果返回错误，target 可能已经被部分更新，调用方不应依赖其精确状态
- header 通常直接使用标准库 `r.Header.Get(...)` / `r.Header.Values(...)`

示例：

```go
accountID, err := hah.Path(r, "account_id").UUID().Required().Get()
tags, err := hah.Query(r, "tag").Values().Get()
```

这组 `Path / Query` API 是当前请求侧主设计，行为变化应视为核心 public API 变化。

## 请求输入文档

- [`REQUESTS.md`](./REQUESTS.md)：`reqx` 的 request helper、binding、显式 post-bind validation 模式和常见组合方式
  其中也包含 `BindQuery(...)` 的封闭字段白名单与单值 query 输入的处理方式
- [docs/path-design.md](./docs/path-design.md)：`Path(...)` 的公开 API、source 语义、支持类型与错误边界
- [docs/query-design.md](./docs/query-design.md)：`Query(...)` 的公开 API、支持类型、链式校验与错误边界
- [docs/binding-query-design.md](./docs/binding-query-design.md)：`BindQuery(...)` 的公开契约与演进边界

## 错误响应

`WriteError(...)` 会把任意错误收敛成稳定的公开错误对象，再写成 `application/problem+json`：

```json
{
  "title": "Unprocessable Entity",
  "status": 422,
  "detail": "request contains invalid fields",
  "code": "invalid_request",
  "errors": [
    {
      "field": "name",
      "in": "body",
      "code": "required",
      "detail": "is required"
    }
  ]
}
```

默认场景直接用 `WriteError(...)` 即可。

`WriteError(...)` 的返回值表示响应边界自身异常，例如错误响应写出失败。生产代码通常至少要记录这个错误。

```go
if writeErr := hah.WriteError(w, err); writeErr != nil {
	slog.Error("write error response failed", "err", writeErr)
}
```

约束：

- `WriteError(...)` 只负责错误标准化与响应写回，不内建独立错误日志
- 如果你需要统一的日志或指标策略，在调用方基于原始 error 和业务上下文自行处理
- `HEAD` 场景沿用 `net/http` 默认语义：handler 正常写回，对外是否发送响应体由底层决定
- 调用方应在开始写出响应前调用 `WriteError(...)`

## 示例与命令

示例目录：

- [`_examples/nethttp`](./_examples/nethttp)：纯 `net/http` / `ServeMux` 示例
- [`_examples/chi`](./_examples/chi)：`chi` router + `RequestID` / `traceid` / `httplog` / 常用中间件示例

仓库根目录常用命令：

```bash
go test ./...
go test -race ./...
go test -bench=. ./...
```
