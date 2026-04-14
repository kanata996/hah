// Package resp 为基于 net/http 的 JSON API 提供响应侧辅助能力。
//
// 它聚焦在 HTTP 输出边界：
//   - JSON 响应写回
//   - 编码成功响应 payload
//   - 编码结构化错误响应 payload
//   - 统一对外 HTTP 错误语义
//
// 公共错误模型由共享包 errx 提供。
//
// 典型用法：
//   - 使用 JSON / JSONBlob 进行底层 JSON 输出
//   - 使用 OK / Created / NoContent 写成功响应
//   - 使用 WriteError 写结构化错误响应
//   - 使用 AsHTTPError 先做错误标准化，再由调用方决定日志策略
//   - 与 errx.HTTPError 配合，按统一错误模型写回响应
//
// 公开 API：
//   - 成功响应入口：JSON、JSONBlob、OK、Created、NoContent
//   - 错误响应入口：WriteError
//   - 错误标准化 helper：AsHTTPError
//   - 自定义错误响应器：ErrorResponder、NewErrorResponder
//   - ErrorResponder 公开字段：AsHTTPError
//   - ErrorResponder 公开方法：Respond
//
// 新增、移除、重命名以上导出符号，或改变其公开语义时，应同步更新本注释与 CHANGELOG。
package resp
