// Package hah 提供根包常用的请求/响应边界入口，聚合 reqx、resp 与 errx 中最常用的一组能力。
//
// 适合在大多数 handler 中直接使用：
//   - 核心 request helper：Path、Query
//   - 明确分离的 DTO 绑定入口：BindQuery、BindBody
//   - 常见请求违规与公共错误模型：InvalidRequest、Violation、HTTPError
//   - 常用 JSON 成功响应辅助
//   - 统一错误响应写回
//
// 当前项目里，reqx.Path / reqx.Query 与 resp 写回 helper 共同构成最核心的
// 请求/响应边界表面；reqx 中的 BindQuery / BindBody 则补足 query/body DTO 绑定场景。
// 根包也额外暴露最常见的 invalid_request helper 与公共错误模型，方便多数 handler
// 在不引入子包的情况下完成输入校验与错误写回。
//
// 公开 API：
//   - request helper：Path、Query
//   - 绑定入口：BindQuery、BindBody
//   - 请求级规则 helper：RequireBody、InvalidRequest
//   - 公共错误模型：Violation、HTTPError、NewHTTPError、NewHTTPErrorWithCause
//   - 公开 violation 常量：Code*、In*
//   - 错误响应入口：WriteError
//   - 成功响应入口：JSON、OK、Created、NoContent
//
// 根包暴露大多数 handler 会直接用到的 facade；如果你需要更细粒度的绑定、
// request helper 或底层类型，请直接导入 reqx、errx 或 resp。
package hah
