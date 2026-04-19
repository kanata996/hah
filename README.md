# hah

[![Go Reference](https://pkg.go.dev/badge/github.com/kanata996/hah.svg)](https://pkg.go.dev/github.com/kanata996/hah)
[![CI](https://github.com/kanata996/hah/workflows/CI/badge.svg)](https://github.com/kanata996/hah/actions/workflows/ci.yml)
[![Codecov](https://codecov.io/github/kanata996/hah/graph/badge.svg)](https://codecov.io/github/kanata996/hah)

`hah` 是一个面向 `net/http` 的 JSON API 边界层，专注于把请求绑定、输入治理和响应写回收敛成一套稳定、克制、可组合的接口。

它只处理 HTTP 边界。它不接管 router，不定义新的 handler 协议，也不包装整个 HTTP 生命周期。你可以把它接到 `ServeMux`、`chi` 或现有中间件栈后面。

## 为什么用 hah

- 面向 `net/http` 设计，保留标准 handler 和 router 控制权
- 以 `hah.Path(...)` / `hah.Query(...)` 作为默认请求侧 API，直接读取 path/query 参数
- 支持把 query、body 绑定到 DTO，再由调用方显式做后续校验
- 把常见请求违规收敛为稳定的公开 HTTP 错误
- 内置 JSON 成功响应与 `application/problem+json` 错误响应
- 根包提供默认且完整的公开 HTTP 边界
- 适合渐进接入现有服务，不要求整体迁移

## 不负责什么

- 选择或内建 validation library
- auth / challenge / rate limit / CORS / redirect
- router 级 `404/405`
- panic recover，包括调用方自定义 `MarshalJSON` 或 `Error` 实现触发的 panic
- tracing / access log / metrics 基础设施
- websocket / streaming runtime

## 安装

环境要求：

- Go 版本以 `go.mod` 为准，当前为 `1.25.9`

安装模块：

```bash
go get github.com/kanata996/hah@latest
```

导入根包：

```go
import "github.com/kanata996/hah"
```

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
	Name string `json:"name"`
}

func validateCreateAccountRequest(r *http.Request, req *createAccountRequest) error {
	if err := hah.RequireBody(r); err != nil {
		return err
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return hah.InvalidRequest(hah.Violation{
			Field: "name",
			In:    hah.InBody,
			Code:  hah.CodeRequired,
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

这个例子展示的是 `hah` 的默认使用路径：读取 path 参数，绑定 body，显式补充输入规则，然后统一写回 JSON 成功响应或 `problem+json` 错误响应。

## 上手路径

主流程通常是四步：

- 用 `hah.Path(...)` / `hah.Query(...)` 读取单字段 path/query 参数
- 用 `hah.BindQuery(...)` / `hah.BindBody(...)` 绑定 DTO
- 用 `hah.RequireBody(...)` / `hah.InvalidRequest(...)` 补充显式请求规则
- 用 `hah.WriteError(...)` / `hah.OK(...)` / `hah.Created(...)` / `hah.NoContent(...)` 写回响应

## 公开 API 速览

请求输入：

- `hah.Path(...)` 面向 path segment 中的资源标识，只保留 `String()`、`UUID()`、`Int()`、`Int64()`、`Uint()`、`Uint64()`
- `hah.Query(...)` 承载更宽的参数语义，除了常见标量外，还支持 `Bool()`、`Float64()`、`Duration()`、`Time()`、`UnixTime()`
- `hah.Query(...).String()` / `Int()` / `UUID()` 等单值 helper 在重复 query key 上会返回稳定 `invalid_request`
- `hah.Query(...).Values()` 可直接读取同名 query 参数的全部解析后值；如果你需要批量结构化解码，优先用 `hah.BindQuery(...)`

DTO binding 与显式规则：

- `hah.BindQuery(...)` 只负责 query -> DTO 的映射，不内建请求级校验
- `hah.BindBody(...)` 只负责 JSON body -> DTO 的解码，不替代业务层或 validation library 的规则
- `hah.RequireBody(...)` 用于显式声明 body-required 契约，可按调用方需要在 `BindBody(...)` 前后组合使用
- `hah.InvalidRequest(...)` 负责把显式输入错误收敛到稳定的 `invalid_request`
- header 通常直接使用标准库 `r.Header.Get(...)` / `r.Header.Values(...)`

错误与响应：

- `hah.Violation`、`hah.HTTPError`、`hah.NewHTTPError(...)`、`hah.NewHTTPErrorWithCause(...)` 是根包暴露的公共错误模型入口
- `hah.BadRequest(...)`、`hah.NotFound(...)`、`hah.Conflict(...)`、`hah.UnprocessableEntity(...)` 等快捷构造器适合在已明确公开错误语义的更深层直接返回
- `hah.WriteError(...)` 会把任意错误收敛成稳定的公开错误对象，再写成 `application/problem+json`
- `hah.JSON(...)` 写回调用方指定状态的 JSON 响应；`hah.OK(...)`、`hah.Created(...)`、`hah.NoContent(...)` 是成功响应快捷入口
- `hah.WriteError(...)` 的返回值表示响应边界自身异常，例如响应写出失败；生产代码通常至少要记录这个错误

## 公开契约要点

`hah` 对公开行为的约束重点在输入与响应边界，而不是 handler 框架本身。

请求输入的关键边界：

- `hah.BindQuery(...)` 的目标必须是 `*struct` 或 `*map[string]string`
- 对于 struct，只有显式 `query` tag 的顶层字段会参与绑定；`BindQuery(...)` 不展开嵌套 DTO
- `hah.BindQuery(...)` 默认忽略未知 query key；同名 query key 只要出现多个值就返回稳定 `400 bad_request`
- malformed raw query 返回稳定 `400 bad_request` 且不修改 target；DTO 或 tag 形状非法时，先返回普通错误且不修改 target
- `hah.BindBody(...)` 公开只支持非 `nil` 的 `*struct` DTO target
- `hah.BindBody(...)` 可和 `hah.RequireBody(...)` 在同一个 request 上按任意顺序组合
- 非空 body 只接受且只接受一个主媒体类型为 `application/json` 的 `Content-Type`
- 零字节 body 不要求 `Content-Type` 为 JSON
- 非空 body 必须恰好构成一个以 object 为顶层值的 JSON 文档，未知字段默认拒绝
- struct 字段解码直接跟随标准库 `encoding/json`；像 `json.RawMessage`、自定义 `UnmarshalJSON` / `UnmarshalText` 类型默认允许
- 绑定先解到临时值，成功后才一次性提交，因此失败不会污染 target
- 同名 JSON object key 跟随标准库 `encoding/json` 语义，后值覆盖前值
- 零字节 body 对 `BindBody(...)` 是 no-op、对 `RequireBody(...)` 是缺失 body；仅空白字符 body 对 `RequireBody(...)` 视为存在、对 `BindBody(...)` 视为 `invalid_json`

响应边界的关键约束：

- `WriteError(...)` 只负责错误标准化与响应写回，不内建独立错误日志
- 如果你需要统一的日志或指标策略，在调用方基于原始 error 和业务上下文自行处理
- `HEAD` 场景沿用 `net/http` 默认语义：handler 正常写回，对外是否发送响应体由底层决定
- 调用方应在开始写出响应前调用 `WriteError(...)`

示例：

```go
accountID, err := hah.Path(r, "account_id").UUID().Required().Get()
tags, err := hah.Query(r, "tag").Values().Get()
```

## 包边界

对外主要分成两个包：

- `hah`：默认公开 HTTP 边界，聚合常用 request helper、绑定、显式请求规则、公共错误模型入口与响应写回入口
- `reqx`：请求侧辅助包，负责 `Path` / `Query`、`BindQuery` / `BindBody`、`RequireBody`、`InvalidRequest` 以及 request-side violation 规范化

实现层还包含 `internal/errx` 与 `internal/resp`，但它们不属于公开 API。

## 深入文档

请求输入：

- [`REQUESTS.md`](./REQUESTS.md)：以 `hah.xx` 为主路径的 request helper、binding、显式 post-bind validation 模式和常见组合方式

## 示例与命令

示例目录：

- [`_examples/nethttp`](./_examples/nethttp)：纯 `net/http` / `ServeMux` 示例
- [`_examples/chi`](./_examples/chi)：`chi` router + `RequestID` / `traceid` / `httplog` / 常用中间件示例

补充：本文默认直接使用 `hah.xx`。只有当你在请求侧需要更细粒度 builder 或绑定入口时，才退到同契约的 `reqx.xx`。
