// Package errx 提供共享 HTTP 错误模型的内部实现。
//
// 它聚焦在 HTTP 边界上的公共错误语义：
//   - 定义可公开返回的 HTTPError
//   - 定义公开 field error 模型
//   - 统一状态码、错误码、标题、详情的标准化规则
//   - 提供带 cause 与不带 cause 的错误构造入口
//   - 提供一组常用状态的快捷构造器
//   - 兼容保留 reqx 依赖的 field error 类型与共享常量
//
// 典型用法：
//   - 大多数调用方应直接使用根包 hah 暴露的常用错误模型入口
//   - 如果仓库内部某个更深层已经明确要返回稳定公共 HTTP 错误，也可以直接返回 errx.HTTPError
//   - 需要保留底层错误链时使用 NewHTTPErrorWithCause，否则使用 NewHTTPError
//   - 需要附带公开 field error 列表时使用 WithFieldErrors
//   - BadRequest、NotFound、UnprocessableEntity 等快捷构造器用于复用常见 HTTP 错误
//   - field error 类型与常量用于根包 facade、reqx 和仓库内部协作
//
// 这里的导出符号服务于仓库内部包协作；对外公开契约由根包 hah 统一承诺。
package errx
