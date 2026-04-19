// Package errx 提供共享 HTTP 错误模型的内部实现。
//
// 它聚焦在 HTTP 边界上的公共错误语义：
//   - 定义可公开返回的 HTTPError
//   - 定义公开 violation 模型
//   - 统一状态码、错误码、标题、详情的标准化规则
//   - 提供带 cause 与不带 cause 的错误构造入口
//   - 提供一组常用状态的快捷构造器
//
// 典型用法：
//   - 使用 NewHTTPError 构造不带底层 cause 的公共 HTTP 错误
//   - 使用 NewHTTPErrorWithCause 构造保留底层 cause 的公共 HTTP 错误
//   - 使用 WithViolations 绑定公开 violation 列表
//   - 使用 BadRequest、NotFound、UnprocessableEntity 等快捷构造复用常见错误
//   - 大多数调用方应直接使用根包 hah 暴露的常用错误模型入口
//   - 如果仓库内部某个更深层已经明确要返回稳定公共 HTTP 错误，也可以直接返回 errx.HTTPError
//   - BadRequest、NotFound、UnprocessableEntity 等状态快捷构造器供仓库内部复用
//
// 这里的导出符号服务于仓库内部包协作；对外公开契约由根包 hah 统一承诺。
package errx
