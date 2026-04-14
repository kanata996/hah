// Package hah 提供根包常用的请求/响应边界入口，聚合 bind、reqx 与 resp 中最常用的一组能力。
//
// 适合在大多数 handler 中直接使用：
//   - 核心 request helper：Path、Query
//   - 明确分离的 DTO 绑定入口：BindQuery、BindBody
//   - 常用 JSON 成功响应辅助
//   - 统一错误响应写回
//   - 公开错误标准化 helper：AsHTTPError
//   - 可定制错误响应器 ErrorResponder
//
// 当前项目里，reqx.Path / reqx.Query 与 resp 写回 helper 共同构成最核心的
// 请求/响应边界表面；bind 则补足 query/body DTO 绑定场景。
//
// 公开 API：
//   - request helper：Path、Query
//   - 绑定入口：BindQuery、BindBody
//   - 绑定相关类型：BindUnmarshaler
//   - 请求级规则 helper：RequireBody
//   - 错误响应入口：WriteError
//   - 错误标准化 helper：AsHTTPError
//   - 自定义错误响应器：ErrorResponder、NewErrorResponder
//   - 成功响应入口：JSON、JSONBlob、OK、Created、NoContent
//
// 根包暴露大多数 handler 会直接用到的 facade；如果你需要更细粒度的绑定、
// request helper 或底层类型，请直接导入 bind、reqx、errx 或 resp。
package hah
