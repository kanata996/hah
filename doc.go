// Package hah 提供根包常用的请求/响应边界入口，聚合 bind、reqx 与 resp 中最常用的一组能力。
//
// 适合在大多数 handler 中直接使用：
//   - 默认请求绑定与校验入口
//   - 常用 JSON 成功响应辅助
//   - 统一错误响应写回
//   - 可定制错误响应器 ErrorResponder
//
// 公开 API：
//   - 绑定入口：Bind、BindBody、BindQueryParams、BindPathValues、BindHeaders
//   - 绑定相关类型：Binder、DefaultBinder、BindUnmarshaler
//   - 绑定并校验入口：BindAndValidate、BindAndValidateBody、
//     BindAndValidateQuery、BindAndValidatePath、BindAndValidateHeaders
//   - DTO 扩展点：RequestValidator、Normalizer
//   - 请求级规则 helper：RequireBody
//   - 错误响应入口：WriteError
//   - 自定义错误响应器：ErrorResponder、NewErrorResponder
//   - 成功响应入口：JSON、JSONBlob、OK、Created、NoContent
//
// 根包只暴露最常用的 facade；如果你需要更细粒度的包级 API，
// 请直接导入 bind、reqx、errx 或 resp。
package hah
