# 响应输出指南

这份文档聚焦 `hah` 的响应侧能力，尤其是“如何在保留稳定错误语义的前提下，输出你自己的 JSON 响应结构”。

`hah` 是 `net/http`-first 的设计。它不引入新的 response context 抽象，响应侧仍然围绕标准库 `http.ResponseWriter` 组织 API。

当前设计里：

- `hah.OK(...)` / `hah.Accepted(...)` / `hah.Created(...)` / `hah.NoContent(...)` / `hah.WriteError(...)` 是默认响应侧 API
- `hah.JSON(...)` 是 raw JSON escape hatch，不参与默认 envelope 协议
- `hah.NormalizeError(...)` 是自定义错误响应结构时复用公开错误语义的推荐入口

## 先看选型

| 目标 | 推荐 API | 说明 |
| --- | --- | --- |
| 直接使用默认成功/失败协议 | `hah.OK` / `hah.Accepted` / `hah.Created` / `hah.NoContent` / `hah.WriteError` | 主路径，最简单，也最稳定 |
| 自己决定整个成功 JSON body 形状 | `hah.JSON` | 只负责写 raw JSON，不提供默认 success envelope |
| 自定义错误响应结构，但想复用稳定错误语义 | `hah.NormalizeError` + `hah.JSON` | 推荐路径 |
| 想继续使用默认错误 envelope | `hah.WriteError` | 直接复用默认错误协议 |

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

自定义响应结构时，建议把成功和失败分开看：

1. 成功路径直接定义你自己的 DTO，然后 `hah.JSON(...)`
2. 失败路径先用 `hah.NormalizeError(...)` 收敛错误，再映射到你自己的 DTO

### 自定义成功响应

成功路径通常不需要额外 helper，直接自己组 DTO：

```go
type SuccessResponse struct {
	Success bool `json:"success"`
	Data    any  `json:"data,omitempty"`
	TraceID string `json:"trace_id,omitempty"`
}

func writeSuccess(w http.ResponseWriter, status int, data any, traceID string) error {
	return hah.JSON(w, status, SuccessResponse{
		Success: true,
		Data:    data,
		TraceID: traceID,
	})
}
```

这比先导出默认 success envelope 再拆开更直接，也更稳定。

### 自定义错误响应

错误路径推荐先归一化：

```go
type ErrorDetail struct {
	Field  string `json:"field,omitempty"`
	In     string `json:"in,omitempty"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type ErrorResponse struct {
	Success bool          `json:"success"`
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details,omitempty"`
	TraceID string        `json:"trace_id,omitempty"`
}

func writeError(w http.ResponseWriter, err error, traceID string) error {
	httpErr := hah.NormalizeError(err)
	if httpErr == nil {
		return nil
	}

	details := make([]ErrorDetail, len(httpErr.Errors()))
	for i, item := range httpErr.Errors() {
		details[i] = ErrorDetail{
			Field:  item.Field,
			In:     string(item.In),
			Code:   string(item.Code),
			Detail: item.Detail,
		}
	}

	return hah.JSON(w, httpErr.Status(), ErrorResponse{
		Success: false,
		Code:    httpErr.Code(),
		Message: httpErr.Detail(),
		Details: details,
		TraceID: traceID,
	})
}
```

这个路径的好处是：

- 你自己的 JSON 结构由你控制
- `hah` 继续负责错误链归一化
- `reason`、字段错误顺序、context 映射、unknown error 兜底等公开错误语义仍然稳定
- HTTP 状态码仍然来自统一的公开错误模型

## NormalizeError 的公开边界

`hah.NormalizeError(...)` 当前公开语义是：

- `hah.NormalizeError(nil)` 返回 `nil`
- 如果错误链中已经有公共 `*hah.HTTPError`，优先复用第一个可见值
- `context.Canceled` 会归一化成 `499 client_closed_request`
- `context.DeadlineExceeded` 会归一化成 `504 timeout`
- 其他未知错误会归一化成 `500 internal_error`

归一化之后，你可以稳定地使用：

- `httpErr.Status()`
- `httpErr.Code()`
- `httpErr.Detail()`
- `httpErr.Errors()`

## 什么时候不要自定义

以下场景通常不值得自己包装：

- 你只是想返回默认的成功 envelope
- 你只是想加几个响应头
- 你并不需要改变 JSON body 结构
- 你希望直接沿用默认错误 envelope

这时继续直接用 `hah.OK(...)` / `hah.Accepted(...)` / `hah.Created(...)` / `hah.WriteError(...)` 更简单。

## 什么时候用 NoContent

如果你明确不想返回响应体，直接用 `hah.NoContent(w)`。

`204 No Content` 本身就不应该带 body，所以它不适合“自定义 envelope”场景。需要 body 时，请继续使用 `200` / `201` / `202` 加 `hah.JSON(...)` 或默认成功 helper。
