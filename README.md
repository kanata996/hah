# hah

[![Go Reference](https://pkg.go.dev/badge/github.com/kanata996/hah.svg)](https://pkg.go.dev/github.com/kanata996/hah)
[![CI](https://github.com/kanata996/hah/workflows/CI/badge.svg)](https://github.com/kanata996/hah/actions/workflows/ci.yml)
[![Codecov](https://codecov.io/github/kanata996/hah/graph/badge.svg)](https://codecov.io/github/kanata996/hah)

`hah` 是一个面向 `net/http` 的 JSON API 边界层，专注于把请求绑定、输入治理和响应写回收敛成一套稳定、克制、可组合的接口。

它只处理 HTTP 边界。它不接管 router，不定义新的 handler 协议，也不包装整个 HTTP 生命周期。你可以把它接到 `ServeMux`、`chi` 或现有中间件栈后面。

## 特性

- 面向 `net/http` 设计，保留标准 handler 和 router 控制权
- 以 `reqx.Path(...)` / `reqx.Query(...)` 作为请求侧核心 API，直接读取 path/query 参数
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

- `hah`：根包 facade，聚合常用的 request helper、绑定与响应写回入口
- `reqx`：输入侧核心包，负责 `Path` / `Query`、`BindQuery` / `BindBody`、`RequireBody`、`InvalidRequest` 和公开 violations
- `errx`：共享公共 HTTP 错误模型
- `resp`：响应侧能力，负责 JSON 成功响应和结构化错误响应

## 核心形状

当前设计里，最核心的边界表面是两条线：

- 请求侧：`reqx.Path(...)` / `reqx.Query(...)`
- 响应侧：`resp.WriteError(...)` / `resp.OK(...)` / `resp.Created(...)` / `resp.NoContent(...)`

其中 `reqx.BindQuery` / `reqx.BindBody` 与对应的根包 facade，是同一输入侧 API 中面向 DTO 的补充能力。后续如果继续演进，默认应优先稳定 `Path / Query` 这条核心主路径的语义和用法。

## 适用范围

`hah` 负责：

- 绑定 query/body 到结构体
- 提供 `reqx.RequireBody(...)` / `reqx.InvalidRequest(...)` 这类显式输入 helper
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
	"github.com/kanata996/hah/reqx"
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
		return reqx.InvalidRequest(reqx.Violation{
			Field: "name",
			In:    reqx.ViolationInBody,
			Code:  reqx.ViolationCodeRequired,
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
- `WriteError`
- `JSON`
- `JSONBlob`
- `OK`
- `Created`
- `NoContent`

## 来源绑定与后续校验

- `hah.BindQuery` / `hah.BindBody`
- `reqx.BindQuery` / `reqx.BindBody`
- `reqx.RequireBody(...)` / `reqx.InvalidRequest(...)`

`hah` 只负责把 HTTP 输入绑定到 DTO，不负责选择 validation 方式。绑定完成后，你可以：

- 手写校验函数，再返回 `reqx.InvalidRequest(...)`
- 继续调用你自己的 `validator/v10`、`ozzo-validation` 或其他库
- 把 DTO 映射到应用层命令，再让应用层做校验

单字段 request helper 的边界：

- `hah.Path(...)` 面向 path segment 中的资源标识，只保留 `String()`、`UUID()`、`Int()`、`Int64()`、`Uint()`、`Uint64()`
- `hah.Query(...)` 承载更宽的参数语义，除了常见标量外，还支持 `Bool()`、`Float64()`、`Duration()`、`Time()`、`UnixTime()`、`UnixMilliTime()`
- `hah.Query(...).String()` / `Int()` / `UUID()` 等标量 helper 在重复 query key 上默认只消费第一个值
- `hah.Query(...).Values()` 可直接读取同名 query 参数的全部原始值；如果你需要批量结构化解码，优先用 `reqx.BindQuery`

DTO binding 的边界：

- `hah.BindQuery(...)` / `reqx.BindQuery(...)` 只负责 query -> DTO 的映射，不内建请求级校验
- `hah.BindQuery(...)` / `reqx.BindQuery(...)` 的目标必须是 struct、`map[string]string`、`map[string][]string` 或 `map[string]any`；如果 DTO/tag 形状本身非法，会直接返回普通错误
- `hah.BindBody(...)` / `reqx.BindBody(...)` 只负责 JSON body -> DTO 的解码
- header 通常直接使用标准库 `r.Header.Get(...)` / `r.Header.Values(...)`

示例：

```go
accountID, err := hah.Path(r, "account_id").UUID().Required().Get()
tags, err := hah.Query(r, "tag").Values().Get()
```

这组 `Path / Query` API 是当前请求侧主设计，行为变化应视为核心 public API 变化。

## 请求输入文档

- [`REQUESTS.md`](./REQUESTS.md)：`reqx` 的 request helper、binding、显式 post-bind validation 模式和常见组合方式
  其中也包含 `reqx` 的自定义解码契约，例如 `BindUnmarshaler`、`BindMultipleUnmarshaler`、`encoding.TextUnmarshaler`、`time.Duration`、`time.Time` + `format:"..."`，以及重复值输入的处理方式

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

`WriteError(...)` 的返回值表示响应边界自身异常。比如公开错误对象无法编码，或错误响应写出失败。生产代码通常至少要记录这个错误。

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
