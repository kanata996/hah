// Package errx 提供共享的公共 HTTP 错误模型。
//
// 它聚焦在 HTTP 边界上的公共错误语义：
//   - 定义可公开返回的 HTTPError
//   - 统一状态码、错误码、标题、详情的标准化规则
//   - 提供带 cause 与不带 cause 的错误构造入口
//   - 提供一组常用状态的快捷构造器
//
// 典型用法：
//   - 使用 NewHTTPError 构造不带底层 cause 的公共 HTTP 错误
//   - 使用 NewHTTPErrorWithCause 构造保留底层 cause 的公共 HTTP 错误
//   - 使用 BadRequest、NotFound、UnprocessableEntity 等快捷构造复用常见错误
//
// 公开 API：
//   - 公开错误类型：HTTPError
//   - HTTPError 公开方法：Error、Unwrap、Status、Code、Title、Detail、Errors
//   - 公开错误构造：NewHTTPError、NewHTTPErrorWithCause
//   - 公开状态快捷构造：BadRequest、Unauthorized、Forbidden、NotFound、
//     MethodNotAllowed、Conflict、UnprocessableEntity、TooManyRequests
//
// 新增、移除、重命名以上导出符号，或改变其公开语义时，应同步更新本注释与 CHANGELOG。
package errx
