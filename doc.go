// Package hah 提供默认的根包请求/响应边界入口，聚合 reqx、resp 与 errx 中最常用的一组能力。
//
// 适合在大多数 handler 中直接使用：
//   - 核心 request helper：Path、Query
//   - 明确分离的 DTO 绑定入口：BindQuery、BindBody
//   - 常见请求违规与公共错误模型：InvalidRequest、Violation、HTTPError
//   - 常用 JSON 成功响应辅助
//   - 统一错误响应写回
//
// 当前项目里，Path / Query、BindQuery / BindBody、RequireBody /
// InvalidRequest 与响应写回 helper 都可以直接从这里使用。
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
// 只有当根包 facade 不满足包边界或导入约束时，才退到同契约的 reqx.xx。
package hah
