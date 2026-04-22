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
- 把常见请求字段错误收敛为稳定的公开 HTTP 错误
- 内置统一 JSON envelope 成功响应与错误响应
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

func validateCreateAccountRequest(req *createAccountRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return hah.InvalidRequest(hah.FieldError{
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
		if err := validateCreateAccountRequest(&req); err != nil {
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

这个例子展示的是 `hah` 的默认使用路径：读取 path 参数，绑定 body，显式补充输入规则，然后统一写回默认 JSON envelope。
`hah` 是默认且唯一推荐的公开入口。只有在你明确拆分 request-side 能力、或直接依赖输入层契约时，才退到 `reqx`。

## 上手路径

主流程通常是四步：

- 用 `hah.Path(...)` / `hah.Query(...)` 读取单字段 path/query 参数
- 用 `hah.BindQuery(...)` / `hah.BindBody(...)` 绑定 DTO
- 用 `hah.InvalidRequest(...)` 补充显式请求规则
- 用 `hah.WriteError(...)` / `hah.OK(...)` / `hah.Accepted(...)` / `hah.Created(...)` / `hah.NoContent(...)` 写回响应

## 公开 API 速览

请求输入：

- `hah.Path(...)` 面向 path segment 中的资源标识，只保留 `String()`、`UUID()`、`Int()`、`Int64()`、`Uint()`、`Uint64()`
- `hah.Query(...)` 承载更宽的参数语义，除了常见标量外，还支持 `Bool()`、`Float64()`、`Duration()`、`Time()`、`UnixTime()`；其中 `Time()` 要求严格 RFC3339 时间戳语法，`UnixTime()` 只接受恰好 10 个十进制数字
- `hah.Query(...).String()` / `Int()` / `UUID()` 等单值 helper 在重复 query key 上会返回稳定 `invalid_request`
- `hah.Query(...).Values()` 可直接读取同名 query 参数的全部解析后值；如果你需要批量结构化解码，优先用 `hah.BindQuery(...)`

DTO binding 与显式规则：

- `hah.BindQuery(...)` 只负责 query -> DTO 的映射，不内建请求级校验
- `hah.BindBody(...)` 只负责 JSON body -> DTO 的解码，不替代业务层或 validation library 的规则
- `hah.InvalidRequest(...)` 负责把显式输入错误收敛到稳定的 `invalid_request`
- header 通常直接使用标准库 `r.Header.Get(...)` / `r.Header.Values(...)`

错误与响应：

- `hah.FieldError`、`hah.HTTPError`、`hah.NewHTTPError(...)`、`hah.NewHTTPErrorWithCause(...)` 是根包暴露的公共错误模型入口
- `hah.BadRequest(...)`、`hah.NotFound(...)`、`hah.Conflict(...)`、`hah.UnprocessableEntity(...)`、`hah.InternalServer(...)` 等快捷构造器适合在已明确公开错误语义的更深层直接返回
- `hah.WriteError(...)` 会把任意错误收敛成稳定的公开错误对象，再写成统一 JSON error envelope
- `hah.OK(...)` / `hah.Accepted(...)` / `hah.Created(...)` 会写默认成功 envelope：顶层固定 `code = 0`、`message = "success"`，业务数据放在可选 `data`
- `hah.NoContent(...)` 会显式写 `204 No Content`，同时清理冲突的 `Content-Type` / `Content-Length`
- `hah.WriteError(...)` 会写默认错误 envelope：顶层 `code` 是五位业务错误码，未显式传入时按 `status * 100` 生成；顶层 `message` 直接来自 `hah.HTTPError.Detail()`
- 默认错误 envelope 的 `error` 对象固定包含稳定的 `reason`；如果有 field errors，再按顺序附带 `details`
- 若 `hah.HTTPError` 未显式提供 `detail`，共享错误模型会基于公开 `reason` 生成默认短语，例如 `internal_error -> "internal error"`
- `hah.JSON(...)` 仍是调用方指定状态码与原始 JSON body 的 escape hatch，不参与默认 envelope 协议
- `hah.WriteError(...)` 的返回值表示响应边界自身异常，例如响应写出失败；生产代码通常至少要记录这个错误

默认错误 envelope 形状类似：

```json
{
  "code": 42200,
  "message": "request contains invalid fields",
  "error": {
    "reason": "invalid_request",
    "details": [
      {
        "field": "name",
        "in": "body",
        "code": "required",
        "detail": "is required"
      }
    ]
  }
}
```

## 公开契约要点

`hah` 对公开行为的约束重点在输入与响应边界，而不是 handler 框架本身。

请求输入的关键边界：

- `hah.BindQuery(...)` 的目标必须是 `*struct` 或 `*map[string]string`
- 对于 struct，只有显式 `query` tag 的顶层字段会参与绑定，其他字段保持原值；`BindQuery(...)` 不展开嵌套 DTO
- `hah.Query(...).Time()` 以及 `BindQuery(...)` 中的 `time.Time` / `*time.Time` 字段都要求严格 RFC3339 时间戳语法，且时区 offset 必须合法
- `hah.BindQuery(...)` 默认忽略未知 query key；同名 query key 只要出现多个值就返回稳定 `400 bad_request`
- malformed raw query 返回稳定 `400 bad_request` 且不修改 target；DTO 或 tag 形状非法时，先返回普通错误且不修改 target
- `hah.BindBody(...)` 公开只支持非 `nil`、且根 DTO 不自定义 `UnmarshalJSON` 的 `*struct` target
- 非空 body 只接受且只接受一个主媒体类型为 `application/json` 的 `Content-Type`
- 零字节 body 不要求 `Content-Type` 为 JSON
- body 超过 `1 MiB` 返回稳定 `request_too_large`
- 非空 body 必须恰好构成一个以 object 为顶层值的 JSON 文档，未知字段默认拒绝
- struct 字段解码直接跟随标准库 `encoding/json`；像 `json.RawMessage`、字段级自定义 `UnmarshalJSON` / `UnmarshalText` 类型默认允许
- 绑定先解到临时值，成功后才一次性提交，因此失败不会污染 target
- 同名 JSON object key 跟随标准库 `encoding/json` 语义，后值覆盖前值
- 零字节 body 对 `BindBody(...)` 是 no-op；仅空白字符 body 和顶层 `null` 对 `BindBody(...)` 视为 `invalid_json`

响应边界的关键约束：

- `WriteError(...)` 只负责错误标准化与响应写回，不内建独立错误日志
- 如果你需要统一的日志或指标策略，在调用方基于原始 error 和业务上下文自行处理
- 默认带 body 的成功协议提供 `OK(...)` / `Accepted(...)` / `Created(...)`；无 payload 成功也允许继续返回 envelope
- 只有显式调用 `NoContent(...)` 时，才会写 `204 No Content` 且不返回响应体
- 默认错误协议不输出 `error.title` / `error.detail` / `error.code`；稳定错误类型统一看 `error.reason`
- `WriteError(w, err)` 的默认顶层错误码固定按 `status * 100` 生成；`WriteError(w, err, code)` 只接受单个五位正整数业务码
- `HEAD` 场景沿用 `net/http` 默认语义：handler 正常写回，对外是否发送响应体由底层决定
- 调用方应在开始写出响应前调用 `WriteError(...)`

示例：

```go
accountID, err := hah.Path(r, "account_id").UUID().Required().Get()
tags, err := hah.Query(r, "tag").Values().Get()
```

## 包边界

对外主要分成两个包：

- `hah`：默认公开 HTTP 边界，也是唯一推荐的主入口；聚合常用 request helper、绑定、显式请求规则、公共错误模型入口与响应写回入口
- `reqx`：较低层的请求侧公开包，负责 `Path` / `Query`、`BindQuery` / `BindBody`、`InvalidRequest` 以及 request-side field error 规范化
  只有当你直接依赖输入层时，才把 `FieldError` / `Code*` / `In*` 等 request-side 契约视为 `reqx` 的公开面；常规 handler 路径仍优先用 `hah`

实现层还包含 `internal/errx`（共享 HTTP 错误模型）与 `internal/resp`（默认 JSON success/error envelope 写回），但它们都不属于公开 API。

## 深入文档

请求输入：

- [`REQUESTS.md`](./REQUESTS.md)：以 `hah.xx` 为主路径的 request helper、binding、显式 post-bind validation 模式和常见组合方式
- [`docs/public-api-scope.md`](./docs/public-api-scope.md)：公开 API 的定位、非目标与演进约束

## 示例与命令

示例目录：

- [`_examples/nethttp`](./_examples/nethttp)：纯 `net/http` / `ServeMux` 示例
- [`_examples/chi`](./_examples/chi)：`chi` router + `RequestID` / `traceid` / `httplog` / 常用中间件示例
