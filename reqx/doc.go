// Package reqx 为基于 net/http 的 JSON API 提供请求级规则与校验组合层。
//
// 它聚焦在 binding 之后的输入治理：
//   - 直接读取 path/query 参数，并提供单参数读取/校验 builder
//   - 在字段校验前执行 Normalize
//   - 允许 DTO 通过 RequestValidator 声明请求级规则
//   - 使用 validator/v10 校验绑定后的输入
//   - 将常见请求违规统一收敛为稳定的 HTTP 错误
//
// 典型用法：
//   - 只做请求绑定时，使用 bind.Bind* 或 hah.Bind / hah.BindBody
//   - 需要默认 mixed-source 绑定 + 校验时，使用 BindAndValidate
//   - 需要显式来源绑定 + 校验时，组合 bind.Bind* 与 Validate(..., Source*)
//
// 公开 API：
//   - request helper 入口：Path、Query
//   - request helper root 类型：PathParam、QueryParam
//   - PathParam builder 返回类型：StringParam、IntParam、Int64Param、
//     UintParam、Uint64Param、UUIDParam
//   - QueryParam builder 返回类型：StringParam、IntParam、Int64Param、
//     UintParam、Uint64Param、BoolParam、Float64Param、DurationParam、
//     UUIDParam、TimeParam、ValuesParam
//   - 校验入口：BindAndValidate、Validate、Source
//   - 公开 Source 常量：SourceBody、SourceQuery、SourcePath、
//     SourceHeader、SourceRequest
//   - DTO 扩展点：RequestValidator、Normalizer
//   - 请求级规则 helper：RequireBody、InvalidRequest
//   - 公开错误码常量：CodeInvalidRequest
//   - 公开违规模型：Violation（字段为 Field、In、Code、Detail）
//   - 公开 violation code 常量：ViolationCodeInvalid、ViolationCodeRequired、
//     ViolationCodeUnknown、ViolationCodeType、ViolationCodeMultiple
//   - 公开 violation in 常量：ViolationInBody、ViolationInQuery、
//     ViolationInPath、ViolationInHeader、ViolationInRequest
//
// BindAndValidate / Validate 的目标约束：
//   - 组合入口最终会执行结构校验，因此目标必须是非 nil 的 *struct
//   - 这与 bind.BindBody(...) 可绑定到 slice、map 等 JSON 目标的能力不同
//
// SourceRequest 的字段别名契约：
//   - 若同一字段在多个来源 tag（param/query/json/header）上声明了相同输入名，
//     violation 会继续使用该共享输入名
//   - 若声明了不同输入名，请求级 violation 会回退为 struct 字段名，以避免把
//     某个来源名误报成最终的公开字段名
//
// 新增、移除、重命名以上导出符号，或改变其公开语义时，应同步更新本注释与 CHANGELOG。
//
// body-required 契约：
//   - RequireBody 沿用默认 binder 的 empty-body 判定：实际读取到零字节 body 视为“没有 body”。
//   - 综合绑定入口对空 body 不主动报错；是否必填由 RequestValidator 或更上层策略决定。
//
// path 输入只依赖 net/http 暴露的 PathValue / Pattern 命名 wildcard 语义，
// 不依赖 chi.RouteContext，也不承诺 chi 专有 `*` catch-all 的兼容行为。
package reqx
