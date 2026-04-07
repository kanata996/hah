// Package reqx 为基于 net/http 的 JSON API 提供请求侧辅助能力。
//
// 它聚焦在 HTTP 输入边界：
//   - 将 JSON / query / path / header 值绑定到结构体
//   - 使用 validator/v10 校验绑定后的输入
//   - 将常见请求违规统一收敛为稳定的 HTTP 错误
//
// 典型用法：
//   - 使用 BindAndValidate 处理 path/query/body 综合绑定
//   - 使用 BindBody 或 BindAndValidateBody 处理 JSON 请求体
//   - 使用 BindAndValidateQuery 处理 query DTO
//   - 使用 BindPathValues / BindHeaders 处理单一来源绑定
//   - 使用 ParamString / ParamInt / ParamUUID 读取简单 path 参数
//
// 公开 API：
//   - 绑定入口：Bind、BindBody、BindQueryParams、BindPathValues、BindHeaders
//   - 绑定并校验入口：BindAndValidate、BindAndValidateBody、
//     BindAndValidateQuery、BindAndValidatePath、BindAndValidateHeaders
//   - path helper：ParamString、ParamInt、ParamUUID
//   - 绑定 option：BindOption、BindBodyOption、BindQueryParamsOption、
//     BindHeadersOption、WithMaxBodyBytes、DefaultMaxBodyBytes
//   - DTO 扩展点：Normalizer
//   - 公开错误码常量：CodeInvalidJSON、CodeUnsupportedMediaType、
//     CodeRequestTooLarge、CodeInvalidRequest
//   - 公开违规模型：Violation（字段为 Field、In、Code、Detail）
//   - 公开 violation code 常量：ViolationCodeInvalid、ViolationCodeRequired、
//     ViolationCodeUnknown、ViolationCodeType、ViolationCodeMultiple
//   - 公开 violation in 常量：ViolationInBody、ViolationInQuery、
//     ViolationInPath、ViolationInHeader、ViolationInRequest
//
// 新增、移除、重命名以上导出符号，或改变其公开语义时，应同步更新本注释与 CHANGELOG。
//
// path 输入只依赖 net/http 暴露的 PathValue / Pattern 命名 wildcard 语义，
// 不依赖 chi.RouteContext，也不承诺 chi 专有 `*` catch-all 的兼容行为。
package reqx
