# 响应输出指南

这份文档聚焦 `hah` 的响应侧能力，尤其是“如何在保留默认错误/状态语义的前提下，输出你自己的 JSON 响应结构”。

`hah` 是 `net/http`-first 的设计。它不引入新的 handler 生命周期或 response context 抽象，响应侧仍然围绕标准库 `http.ResponseWriter` 组织 API。

当前设计里：

- `hah.OK(...)` / `hah.Accepted(...)` / `hah.Created(...)` / `hah.NoContent(...)` / `hah.WriteError(...)` 是默认响应侧 API
- `hah.JSON(...)` 是 raw JSON escape hatch，不参与默认 envelope 协议
- `hah.SuccessResponse(...)` / `hah.ErrorResponse(...)` 导出默认响应视图，适合拿来包装成自定义响应结构

## 先看选型

| 目标 | 推荐 API | 说明 |
| --- | --- | --- |
| 直接使用默认成功/失败协议 | `hah.OK` / `hah.Accepted` / `hah.Created` / `hah.NoContent` / `hah.WriteError` | 主路径，最简单，也最稳定 |
| 自己决定整个 JSON body 形状 | `hah.JSON` | 只负责写 raw JSON，不提供默认 envelope 语义 |
| 想保留默认 status / code / reason / details 语义，但改外层 JSON 结构 | `hah.SuccessResponse` / `hah.ErrorResponse` + `hah.JSON` | 自定义响应结构的推荐路径 |
| 想给错误响应指定自定义顶层业务码 | `hah.ErrorResponse(err, code)` | `code` 只能传一个五位整数 |

## 默认响应怎么用

大多数 handler 直接使用默认 helper 就够了：

```go
func handler(w http.ResponseWriter, r *http.Request) {
	account := map[string]any{"id": "u_1"}

	if err := hah.Created(w, account); err != nil {
		// 记录响应写回失败
	}
}
```

默认成功响应会写成统一 envelope：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "u_1"
  }
}
```

错误路径通常直接写：

```go
if err := hah.WriteError(w, appErr); err != nil {
	// 记录响应写回失败
}
```

如果你只想输出一段原始 JSON，而不想参与默认 envelope，直接用 `hah.JSON(...)`：

```go
_ = hah.JSON(w, http.StatusOK, map[string]any{
	"id":   "u_1",
	"name": "kanata",
})
```

## 自定义响应结构

如果你想输出自己的响应结构，但又不想重写默认错误收敛逻辑，推荐分三步：

1. 用 `hah.SuccessResponse(...)` 或 `hah.ErrorResponse(...)` 先拿到默认响应视图
2. 把这个视图映射到你自己的 DTO
3. 最终用 `hah.JSON(...)` 按 `response.Status` 写回

### 自定义成功/失败外层包络

```go
type APIResponse struct {
	Success bool               `json:"success"`
	Code    int                `json:"code"`
	Message string             `json:"message"`
	Data    any                `json:"data,omitempty"`
	Error   *hah.ResponseError `json:"error,omitempty"`
	TraceID string             `json:"trace_id,omitempty"`
}

func writeSuccess(w http.ResponseWriter, status int, data any, traceID string) error {
	base, err := hah.SuccessResponse(status, data)
	if err != nil {
		return err
	}

	return hah.JSON(w, base.Status, APIResponse{
		Success: true,
		Code:    base.Code,
		Message: base.Message,
		Data:    base.Data,
		TraceID: traceID,
	})
}

func writeError(w http.ResponseWriter, err error, traceID string) error {
	base, buildErr := hah.ErrorResponse(err)
	if buildErr != nil {
		return buildErr
	}
	if base == nil {
		return nil
	}

	return hah.JSON(w, base.Status, APIResponse{
		Success: false,
		Code:    base.Code,
		Message: base.Message,
		Error:   base.Error,
		TraceID: traceID,
	})
}
```

这个路径的好处是：

- 你自己的 JSON 结构由你控制
- 默认错误模型里的 `reason` / `details` 仍然复用 `hah` 的公开契约
- HTTP 状态码仍然来自默认响应视图，不会在多个地方各写一套判断

### 直接嵌入默认响应视图

如果你只是想在默认 envelope 外面再包一层，也可以直接复用 `*hah.Response`：

```go
type Envelope struct {
	TraceID string        `json:"trace_id"`
	Result  *hah.Response `json:"result"`
}

func writeWrappedSuccess(w http.ResponseWriter, data any, traceID string) error {
	base, err := hah.SuccessResponse(http.StatusOK, data)
	if err != nil {
		return err
	}

	return hah.JSON(w, base.Status, Envelope{
		TraceID: traceID,
		Result:  base,
	})
}
```

这里要注意：

- `hah.Response.Status` 只用于 HTTP 写回状态码，不参与 JSON 编码
- 所以把 `*hah.Response` 直接放进自定义 DTO 是安全的，JSON 里不会多出一个 `status` 字段

## 公开边界

`hah.SuccessResponse(...)` / `hah.ErrorResponse(...)` 当前公开语义是：

- `hah.SuccessResponse(status, data)` 目前只接受 `200` / `201` / `202`
- `hah.ErrorResponse(nil)` 返回 `nil, nil`
- `hah.ErrorResponse(err)` 默认把顶层 `code` 设为 `status * 100`
- `hah.ErrorResponse(err, code)` 允许显式传一个五位错误码
- `hah.ErrorResponse(err, code1, code2)` 属于调用错误
- 顶层 `message` 直接来自共享错误模型的 `detail`
- `error.reason` 是稳定公开语义；如果有字段级输入错误，继续放在 `error.details`

如果你只是想“默认协议 + 自定义头部”，通常不需要自定义响应结构。直接先写 header，再调用默认 helper 即可：

```go
w.Header().Set("X-Trace-ID", traceID)
_ = hah.OK(w, data)
```

## 什么时候不要自定义

以下场景通常不值得自己包装：

- 你只是想返回默认的成功 envelope
- 你只是想加几个响应头
- 你并不需要改变 JSON body 结构
- 你只是在 `200/201/202` 成功和统一错误 envelope 之间切换

这时继续直接用 `hah.OK(...)` / `hah.Accepted(...)` / `hah.Created(...)` / `hah.WriteError(...)` 更简单。

## 什么时候用 NoContent

如果你明确不想返回响应体，直接用 `hah.NoContent(w)`。

`204 No Content` 本身就不应该带 body，所以它不适合“自定义 envelope”场景。需要 body 时，请继续使用 `200` / `201` / `202` 加 `hah.JSON(...)` 或默认成功 helper。
