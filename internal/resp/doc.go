// Package resp 为基于 net/http 的 JSON API 提供响应侧辅助能力的内部实现。
//
// 它聚焦在 HTTP 输出边界：
//   - JSON 响应写回
//   - 编码默认成功 envelope
//   - 编码默认错误 envelope
//   - 统一对外 HTTP 错误语义
//
// 公共错误模型由内部共享包 errx 提供，对外统一通过根包 hah 暴露。
//
// 典型用法：
//   - 使用 JSON 进行底层 JSON 输出
//   - 使用 OK / Accepted / Created 写默认成功响应
//   - 使用 NoContent 写显式 204 成功响应
//   - 使用 WriteError 写默认错误响应
//   - 与 errx.HTTPError 配合，按统一错误模型写回响应
//
// 这里的导出符号服务于仓库内部复用；对外公开契约由根包 hah 统一承诺。
package resp
