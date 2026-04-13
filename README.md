# hah

[![Go Reference](https://pkg.go.dev/badge/github.com/kanata996/hah.svg)](https://pkg.go.dev/github.com/kanata996/hah)
[![CI](https://github.com/kanata996/hah/workflows/CI/badge.svg)](https://github.com/kanata996/hah/actions/workflows/ci.yml)
[![Codecov](https://codecov.io/github/kanata996/hah/graph/badge.svg)](https://codecov.io/github/kanata996/hah)

`hah` 是一个面向 `net/http` 的 JSON API 边界层，专注于把请求绑定、输入治理和响应写回收敛成一套稳定、克制、可组合的接口。

它只处理 HTTP 边界。它不接管 router，不定义新的 handler 协议，也不包装整个 HTTP 生命周期。你可以把它接到 `ServeMux`、`chi` 或现有中间件栈后面。

## 特性

- 面向 `net/http` 设计，保留标准 handler 和 router 控制权
- 支持通过根包 facade 直接读取 path/query 参数
- 支持把 path、query、header、body `Bind` 到 DTO
- 提供默认组合入口：`Bind` -> Normalize -> `RequestValidator` -> `validator/v10`
- 把常见请求违规收敛为稳定的公开 HTTP 错误
- 内置 JSON 成功响应与 `application/problem+json` 错误响应
- 在 `5xx` 场景输出独立错误日志，并支持 request log 注解
- 根包提供常用 facade，也支持直接使用 `bind`、`reqx`、`resp`、`errx`
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
	"github.com/kanata996/hah/bind"
	"github.com/kanata996/hah/errx"
	"github.com/kanata996/hah/reqx"
	"github.com/kanata996/hah/resp"
)
```

## 包边界

仓库分成五个包：

- `hah`：根包 facade，聚合常用的绑定、校验和响应写回入口
- `bind`：请求绑定层，负责 path/query/header/body 到目标值的映射
- `reqx`：请求 helper、请求规则与校验层，负责 `Path` / `Query`、`Validate`、`Normalize`、`RequestValidator`、`RequireBody` 和 `validator/v10`
- `errx`：共享公共 HTTP 错误模型
- `resp`：响应侧能力，负责 JSON 成功响应和结构化错误响应

## 适用范围

`hah` 负责：

- 绑定 path/query/header/body 到结构体
- 在 `BindAndValidate` 或 `reqx.Validate` 路径中执行 `Normalize`、`RequestValidator` 和 `validator/v10` 校验
- 把常见请求违规收敛成稳定的公开 HTTP 错误
- 写回标准 JSON 成功响应
- 写回 `application/problem+json` 错误响应
- 在 `5xx` 场景通过 `slog.Default()` 输出独立错误日志
- 通过 `ErrorResponder` 自定义错误归一化、独立错误日志和 request log 注解

`hah` 不负责：

- auth / challenge / rate limit / CORS / redirect
- router 级 `404/405`
- panic recover
- tracing / access log / metrics 基础设施
- websocket / streaming runtime

## 快速示例

```go
package main

import (
	"log"
	"net/http"

	"github.com/kanata996/hah"
)

type createAccountRequest struct {
	OrgID string `param:"org_id"`
	Name  string `json:"name" validate:"required"`
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /orgs/{org_id}/accounts", func(w http.ResponseWriter, r *http.Request) {
		var req createAccountRequest
		if err := hah.BindAndValidate(r, &req); err != nil {
			if writeErr := hah.WriteError(w, r, err); writeErr != nil {
				log.Printf("write error response failed: %v", writeErr)
			}
			return
		}

		if err := hah.Created(w, map[string]any{
			"id":     "acct_123",
			"org_id": req.OrgID,
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
- `Bind`
- `BindBody`
- `BindQueryParams`
- `BindPathValues`
- `BindHeaders`
- `BindAndValidate`
- `RequireBody`
- `WriteError`
- `ErrorResponder`
- `NewErrorResponder`
- `JSON`
- `JSONBlob`
- `OK`
- `Created`
- `NoContent`

## 来源绑定与校验

- `hah.BindQueryParams` / `hah.BindPathValues` / `hah.BindHeaders` / `hah.BindBody`
- `bind.BindQueryParams` / `bind.BindPathValues` / `bind.BindHeaders` / `bind.BindBody`
- `reqx.Validate(..., reqx.SourceQuery)` / `reqx.SourcePath` / `reqx.SourceHeader` / `reqx.SourceBody`
- `hah.Path` / `hah.Query` 适合单字段 path/query 读取；query 同名参数重复出现时默认只消费第一个值，多值语义请改用 `bind.Bind*`

## 请求输入文档

- [`REQUESTS.md`](./REQUESTS.md)：`bind` / `reqx` 的 request helper、binding、validation、请求级规则和常见组合模式
  其中也包含 `bind` 的自定义解码契约，例如 `UnmarshalParam`、`encoding.TextUnmarshaler`、`time.Time` + `format:"..."`，以及重复值输入的处理方式

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

默认场景直接用 `WriteError(...)` 即可。需要自定义错误归一化、logger 或 request log 注解时，再使用 `ErrorResponder`。

`WriteError(...)` / `ErrorResponder.Respond(...)` 的返回值表示响应边界自身异常。比如响应已开始写出，或错误响应写出失败。生产代码通常至少要记录这个错误。

```go
responder := hah.NewErrorResponder()
responder.Logger = slog.Default()
responder.AnnotateRequestLog = func(r *http.Request, attrs []slog.Attr) {
	// 把 attrs 桥接到你自己的 request logger。
}

if writeErr := responder.Respond(w, r, err); writeErr != nil {
	slog.Error("write error response failed", "err", writeErr)
}
```

约束：

- `4xx` 不额外输出独立错误日志
- 正常 `5xx` 会通过 `slog.Default()` 输出一条独立错误日志
- 如果错误响应本身写出失败，还会额外记录一条写失败日志
- `HEAD` 场景沿用 `net/http` 默认语义：handler 正常写回，对外是否发送响应体由底层决定
- 如果响应已经开始写出，不再尝试二次改写响应

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
