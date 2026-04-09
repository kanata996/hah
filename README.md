# hah

[![Go Reference](https://pkg.go.dev/badge/github.com/kanata996/hah.svg)](https://pkg.go.dev/github.com/kanata996/hah)
[![CI](https://github.com/kanata996/hah/workflows/CI/badge.svg)](https://github.com/kanata996/hah/actions/workflows/ci.yml)
[![Codecov](https://codecov.io/github/kanata996/hah/graph/badge.svg)](https://codecov.io/github.com/kanata996/hah)

`hah` 是一个面向 `net/http` 的 JSON API 边界层。

它不接管 router，不定义新的 handler 协议，也不试图包装整个 HTTP 生命周期。当前仓库拆成五个清晰的包边界：

- `hah`：根包 facade，聚合最常用的绑定、校验和响应写回入口
- `bind`：请求绑定层，负责 path/query/header/body 到目标值的映射
- `reqx`：请求规则与校验层，负责 `Normalize`、`RequestValidator` 和 `validator/v10`
- `errx`：公共 HTTP 错误模型
- `resp`：响应侧能力，负责 JSON 成功响应和结构化错误响应

## 能力范围

- 绑定 path/query/header/body 到结构体
- 在绑定后执行 Normalize 和 `validator/v10` 校验
- 把常见请求违规收敛成稳定的公开 HTTP 错误
- 写回标准 JSON 成功响应
- 写回 `application/problem+json` 错误响应
- 在 5xx 场景通过 `slog.Default()` 输出独立错误日志
- 通过 `ErrorResponder` 自定义错误归一化、独立错误日志和 request log 注解

不负责：

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
			_ = hah.WriteError(w, r, err)
			return
		}

		_ = hah.Created(w, r, map[string]any{
			"id":     "acct_123",
			"org_id": req.OrgID,
			"name":   req.Name,
		})
	})

	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

## 根包常用 API

- `Bind`
- `BindBody`
- `BindQueryParams`
- `BindPathValues`
- `BindHeaders`
- `BindAndValidate`
- `BindAndValidateBody`
- `BindAndValidateQuery`
- `BindAndValidatePath`
- `BindAndValidateHeaders`
- `RequireBody`
- `WriteError`
- `ErrorResponder`
- `NewErrorResponder`
- `JSON`
- `JSONBlob`
- `OK`
- `Created`
- `NoContent`

## 错误响应

`WriteError(...)` 会把任意错误收敛成稳定的公开错误对象，并写成 `application/problem+json`：

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

`WriteError(...)` 适合默认行为；如果你需要自定义错误归一化、logger 或 request log 注解，
可以使用 `ErrorResponder`：

```go
responder := hah.NewErrorResponder()
responder.Logger = slog.Default()
responder.AnnotateRequestLog = func(r *http.Request, attrs []slog.Attr) {
	// 把 attrs 桥接到你自己的 request logger。
}

if err := responder.Respond(w, r, err); err != nil {
	// 只在响应已开始写出或错误响应写出失败时返回非 nil。
}
```

约束：

- `4xx` 不额外输出独立错误日志
- 正常 `5xx` 会通过 `slog.Default()` 输出一条独立错误日志
- 如果错误响应本身写出失败，还会额外记录一条写失败日志
- `HEAD` 请求只写状态和头，不写响应体
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

## 许可证

[MIT](./LICENSE)
