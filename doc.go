// Package hah 提供默认的根包 HTTP 边界入口，聚合请求输入、公共错误模型与 JSON 响应写回。
//
// 适合在大多数 handler 中直接使用：
//   - 核心 request helper：Path、Query
//   - 明确分离的 DTO 绑定入口：BindQuery、BindBody
//   - 常见请求字段错误与公共错误模型：InvalidRequest、FieldError、HTTPError
//   - 常用 JSON 成功响应辅助
//   - 统一错误响应写回
//
// 当前项目里，hah 是默认入口；多数调用方不需要直接 import reqx。
// 只有当你明确在拆分 request-side 组件、并需要更低层的输入侧公开面时，
// 才退到 reqx.xx。
//
// 公开 API：
//   - request helper：Path、Query
//   - 绑定入口：BindQuery、BindBody
//   - 请求级规则 helper：InvalidRequest
//   - 公共错误模型：FieldError、HTTPError、NewHTTPError、NewHTTPErrorWithCause
//   - 常用错误快捷构造：BadRequest、Unauthorized、Forbidden、NotFound、
//     MethodNotAllowed、Conflict、UnprocessableEntity、TooManyRequests、
//     InternalServer
//   - 公开 field error 常量：Code*、In*
//   - 错误响应入口：WriteError
//   - 成功响应入口：JSON、OK、Accepted、Created、NoContent
//
// 当前根包是默认且唯一推荐的公开入口；错误与响应边界固定收敛在这里。
// reqx 仍然是公开包，但定位为较低层的 request-side 原生面，而不是并列主入口。
package hah
