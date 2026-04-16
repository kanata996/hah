// Package reqx 为基于 net/http 的 JSON API 提供 request helper、DTO binding 与输入辅助错误模型。
//
// 它聚焦在输入侧能力：
//   - 直接读取 path/query 参数，并提供单参数读取/校验 builder
//   - 把 query/body 输入绑定到 DTO
//   - 将常见请求违规统一收敛为稳定的 HTTP 错误
//   - 提供 body-required 等显式 helper，供调用方在绑定后自行组合规则
//
// 当前项目里，reqx.Path(...) / reqx.Query(...) 是请求侧的核心公开 API 之一。
// 它们负责“不定义 DTO 也能安全读取输入”的主路径；BindQuery / BindBody 则补足
// query/body DTO 场景下的 source-to-struct 映射。调整 Path / Query 的形状、
// 链式能力或错误语义时，应按核心 public API 变更看待。
//
// BindQuery 的目标当前限定为 struct、map[string]string、map[string][]string
// 或 map[string]any；如果 DTO/tag 形状本身非法（例如命名的未打 query tag
// 的 *struct 字段），会直接返回普通错误，而不是按客户端输入错误收敛。
//
// 典型用法：
//   - 读取单个 path/query 参数时，优先使用 Path(...) / Query(...)
//   - 绑定 query/body DTO 时，使用 BindQuery / BindBody 或根包 hah 的对应 facade
//   - 需要返回统一 invalid_request 错误时，使用 InvalidRequest(errx.Violation{...})
//     或根包 hah 的对应 facade
//   - 需要显式要求 body 必填时，使用 RequireBody(...)
//
// 公开 API：
//   - request helper 入口：Path、Query
//   - DTO binding 入口：BindQuery、BindBody
//   - DTO 自定义解码接口：BindUnmarshaler、BindMultipleUnmarshaler
//   - request helper root 类型：PathParam、QueryParam
//   - PathParam builder 返回类型：StringParam、IntParam、Int64Param、
//     UintParam、Uint64Param、UUIDParam
//   - QueryParam builder 返回类型：StringParam、IntParam、Int64Param、
//     UintParam、Uint64Param、BoolParam、Float64Param、DurationParam、
//     UUIDParam、TimeParam、ValuesParam
//   - 请求级规则 helper：RequireBody、InvalidRequest
//   - 公开 body decode 错误码常量：CodeInvalidJSON、CodeUnsupportedMediaType、
//     CodeRequestTooLarge
//
// 新增、移除、重命名以上导出符号，或改变其公开语义时，应同步更新本注释与 CHANGELOG。
//
// body-required 契约：
//   - RequireBody 沿用 BindBody 的 empty-body 判定：实际读取到零字节 body 视为“没有 body”。
//   - BindBody 对空 body 不主动报错；是否必填由调用方显式决定。
//   - RequireBody 与 BindBody 共享同一个非破坏性 body 探测；二者可按调用方需要自由前后组合。
//   - BindBody 直接解码到传入目标；JSON 缺失字段不会清空目标已有值。
//   - BindBody / BindQuery 都不是事务性的；若返回错误，目标值可能已经被部分更新。
//
// path 输入只依赖 net/http 暴露的 PathValue / Pattern 命名 wildcard 语义。
// 如果上层 router 有自定义 pattern 语法，应在桥接层先归一化后再写入 Pattern。
// reqx 不依赖 chi.RouteContext，也不承诺 chi 专有 `*` catch-all 或 `:{regexp}` 之类
// router-specific token 的兼容行为。
package reqx
