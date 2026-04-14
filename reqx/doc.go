// Package reqx 为基于 net/http 的 JSON API 提供 request helper 与输入辅助错误模型。
//
// 它聚焦在显式输入读取与请求级 helper：
//   - 直接读取 path/query 参数，并提供单参数读取/校验 builder
//   - 将常见请求违规统一收敛为稳定的 HTTP 错误
//   - 提供 body-required 等显式 helper，供调用方在绑定后自行组合规则
//
// 当前项目里，reqx.Path(...) / reqx.Query(...) 是请求侧的核心公开 API 之一。
// 它们负责“不定义 DTO 也能安全读取输入”的主路径；bind 包则补足 DTO 场景下的
// source-to-struct 映射。调整 Path / Query 的形状、链式能力或错误语义时，
// 应按核心 public API 变更看待。
//
// 典型用法：
//   - 读取单个 path/query 参数时，优先使用 Path(...) / Query(...)
//   - 绑定 DTO 时，使用 bind.Bind* 或根包 hah 的 Bind*
//   - 需要返回统一 invalid_request 错误时，使用 InvalidRequest(...)
//   - 需要显式要求 body 必填时，使用 RequireBody(...)
//
// 公开 API：
//   - request helper 入口：Path、Query
//   - request helper root 类型：PathParam、QueryParam
//   - PathParam builder 返回类型：StringParam、IntParam、Int64Param、
//     UintParam、Uint64Param、UUIDParam
//   - QueryParam builder 返回类型：StringParam、IntParam、Int64Param、
//     UintParam、Uint64Param、BoolParam、Float64Param、DurationParam、
//     UUIDParam、TimeParam、ValuesParam
//   - 请求级规则 helper：RequireBody、InvalidRequest
//   - 公开错误码常量：CodeInvalidRequest
//   - 公开违规模型：Violation（字段为 Field、In、Code、Detail）
//   - 公开 violation code 常量：ViolationCodeInvalid、ViolationCodeRequired、
//     ViolationCodeUnknown、ViolationCodeType、ViolationCodeMultiple
//   - 公开 violation in 常量：ViolationInBody、ViolationInQuery、
//     ViolationInPath、ViolationInHeader
//
// 新增、移除、重命名以上导出符号，或改变其公开语义时，应同步更新本注释与 CHANGELOG。
//
// body-required 契约：
//   - RequireBody 沿用默认 binder 的 empty-body 判定：实际读取到零字节 body 视为“没有 body”。
//   - 综合绑定入口对空 body 不主动报错；是否必填由调用方显式决定。
//
// path 输入只依赖 net/http 暴露的 PathValue / Pattern 命名 wildcard 语义，
// 不依赖 chi.RouteContext，也不承诺 chi 专有 `*` catch-all 的兼容行为。
package reqx
