# reqx

`reqx` 是 `hah` 的请求解码与参数校验子包，用来把 API 边界上的“请求形状问题”收敛成稳定、可公开暴露的 `Problem`。

## 定位

`reqx` 只解决 HTTP 边界上的通用输入问题：

- 解码 JSON body
- 解码 URL query parameters
- 提供通用校验入口，把调用方产出的 `[]Violation` 归一化为稳定的 `422 invalid_request`
- 把解码失败和参数校验失败转换成稳定的 4xx 请求问题

`reqx` 不负责这些事情：

- 不接管 router
- 不提供完整的 binding 生命周期
- 不提供 DTO / schema 校验能力本身
- 不承担业务规则校验
- 不依赖特定 validation 库
- 不做 header binding
- 不做 path param binding
- 不自动合并 body / query / header / path 等多个来源

典型用法是：

1. 在 handler 里把请求解码到请求结构体
2. 用自定义函数或第三方 validator 产出 `[]Violation`
3. 让 `reqx` 统一把这些 violation 转成稳定的请求问题
4. 让 service 层只处理业务规则

配合 `hah.WriteError(...)` 使用时，`reqx` 返回的 `Problem` 可以直接进入统一错误响应。

## 适用场景

适合：

- 主要提供 JSON API 的服务
- 希望请求解码行为稳定、可预测
- 希望把输入错误统一映射成结构化 4xx 响应

不适合：

- 需要完整的 framework-style binder
- 需要自动混合 body / query / form / header / path 多来源绑定
- 需要把 router-specific path params 作为 `reqx` 的职责

## 全部公开 API

### Problem

```go
type Problem struct

func NewProblem(status int, code, message string, details ...any) *Problem

func (p *Problem) Error() string
func (p *Problem) Status() int
func (p *Problem) Code() string
func (p *Problem) Message() string
func (p *Problem) Details() []any
```

`Problem` 表示整个请求的公开错误，适合直接作为 4xx 响应返回。

### Validation

```go
type ValidateFunc[T any] func(*T) []Violation

type Violation struct {
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Validate[T any](dst *T, fn ValidateFunc[T]) error
```

`Violation` 表示单个字段或单个输入项的问题。

`reqx` 公开的常见 code 常量：

- 顶层问题码：`CodeRequestError`、`CodeInvalidJSON`、`CodeUnsupportedMediaType`、`CodeRequestTooLarge`、`CodeInvalidRequest`
- violation code：`ViolationCodeInvalid`、`ViolationCodeRequired`、`ViolationCodeUnknown`、`ViolationCodeType`、`ViolationCodeMultiple`
- 这些常量只是便捷入口；`NewProblem(...)` 和 `Violation.Code` 仍然允许你使用自定义稳定字符串

`Validate` 本身不提供 DTO / schema 校验能力。它只做两件事：

- 调用你传入的校验函数
- 把你返回的 `[]Violation` 归一化为统一的 `422 invalid_request`

如果你需要更专业的 DTO / schema 校验能力，应在上层接入第三方库，再把结果适配成 `[]Violation`。

### JSON Body Decode

```go
const DefaultMaxBodyBytes int64 = 1 << 20

func WithMaxBodyBytes(limit int64) DecodeOption
func AllowUnknownFields() DecodeOption
func AllowEmptyBody() DecodeOption

func DecodeJSON[T any](r *http.Request, dst *T, opts ...DecodeOption) error
func DecodeAndValidateJSON[T any](r *http.Request, dst *T, fn ValidateFunc[T], opts ...DecodeOption) error
```

相关选项类型：`DecodeOption`

`DecodeJSON` 的默认行为：

- 仅接受 `application/json` 或 `+json` 的 `Content-Type`
- 缺失 `Content-Type` 时允许继续解码
- 默认限制 body 大小为 `DefaultMaxBodyBytes`
- 默认拒绝空 body
- 默认拒绝 unknown JSON fields
- 默认拒绝多个 JSON value

常用选项：

- `WithMaxBodyBytes`：限制 body 大小
- `AllowUnknownFields`：允许未知 JSON 字段
- `AllowEmptyBody`：允许空 body

### Query Decode

```go
func AllowUnknownQueryFields() QueryOption

func DecodeQuery[T any](r *http.Request, dst *T, opts ...QueryOption) error
func DecodeAndValidateQuery[T any](r *http.Request, dst *T, fn ValidateFunc[T], opts ...QueryOption) error
```

相关选项类型：`QueryOption`

`DecodeQuery` 会把 URL query 参数解码到带 `query:"..."` tag 的结构体字段中。

支持的字段类型：

- `string`
- `bool`
- 有符号整数
- 无符号整数
- 浮点数
- 指向以上标量或 slice 的指针
- 以上标量的 slice
- 实现了 `encoding.TextUnmarshaler` 的类型

默认行为：

- 只处理显式声明了 `query` tag 的字段
- 默认拒绝未知 query 参数
- 标量字段默认不允许重复值

常用选项：

- `AllowUnknownQueryFields`：允许未知 query 参数

## 错误语义

`reqx` 有两个核心错误载体：

- `Problem`：表示整个请求失败
- `Violation`：表示某个字段或某个输入项失败，通常出现在 `Problem.Details()` 中

`reqx` 区分两类错误：

- 解码 / 请求形状错误：通常返回 `400` / `413` / `415`
- 参数校验错误：返回 `422 invalid_request`

常见公开错误语义包括：

- `400 invalid_json`
- `413 request_too_large`
- `415 unsupported_media_type`
- `422 invalid_request`

其中 `422 invalid_request` 的 `details` 一般由一个或多个 `Violation` 组成。

## 使用示例

### JSON

```go
type CreateUserRequest struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

func validateCreateUserRequest(value *CreateUserRequest) []reqx.Violation {
	var violations []reqx.Violation
	if value.Name == "" {
		violations = append(violations, reqx.Violation{
			Field: "name",
			Code:  reqx.ViolationCodeRequired,
		})
	}
	return violations
}

func handleCreateUser(w http.ResponseWriter, r *http.Request) error {
	var req CreateUserRequest
	return reqx.DecodeAndValidateJSON(r, &req, validateCreateUserRequest)
}
```

### 接入第三方校验库

```go
type FieldError struct {
	Field   string
	Code    string
	Message string
}

func externalValidateCreateUser(*CreateUserRequest) []FieldError {
	return nil
}

func adaptFieldErrors(errs []FieldError) []reqx.Violation {
	violations := make([]reqx.Violation, 0, len(errs))
	for _, err := range errs {
		violations = append(violations, reqx.Violation{
			Field:   err.Field,
			Code:    err.Code,
			Message: err.Message,
		})
	}
	return violations
}

func validateCreateUserWithExternal(value *CreateUserRequest) []reqx.Violation {
	return adaptFieldErrors(externalValidateCreateUser(value))
}
```

### Query

```go
type ListUsersQuery struct {
	Page  *int   `query:"page"`
	Limit *int   `query:"limit"`
	Role  string `query:"role"`
}

func validateListUsersQuery(value *ListUsersQuery) []reqx.Violation {
	var violations []reqx.Violation
	if value.Limit != nil && *value.Limit <= 0 {
		violations = append(violations, reqx.Violation{
			Field: "limit",
			Code:  "invalid",
		})
	}
	return violations
}

func handleListUsers(w http.ResponseWriter, r *http.Request) error {
	var query ListUsersQuery
	return reqx.DecodeAndValidateQuery(r, &query, validateListUsersQuery)
}
```

## 设计边界

`reqx` 当前有意只覆盖 body 和 query。

原因是：

- body 和 query 都是标准 `net/http` 请求对象上可直接获得的数据源
- path params 天然依赖 router 的提取方式
- header 往往带有协议或安全语义，更适合上层应用显式处理
- 一旦把这些来源都收编进来，`reqx` 很容易从请求 helper 膨胀成通用 binding framework

如果未来需要扩展，优先保持这些约束：

- API 应该是显式的，而不是自动合并多来源
- 不直接依赖某个 router 的上下文类型
- 不削弱当前稳定的公开错误语义
