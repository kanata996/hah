// Package bind 为基于 net/http 的 JSON API 提供请求绑定能力。
//
// 它只负责把 HTTP 输入映射到目标值，不内建 Normalize、请求级规则或字段校验。
// 如果调用方需要 DTO 标准化、结构校验或更高层规则，应在 BindQuery / BindBody 返回后自行显式执行。
//
// 当前支持的数据源：
//   - query：query 参数
//   - json：请求 body
//
// 名称匹配规则：
//   - query 标签按精确名字匹配
//
// json body 当前只支持 application/json，并使用 Go 标准库 encoding/json 解码；
// 也不接受 application/*+json。
//
// 公开 API：
//   - 绑定入口：BindQuery、BindBody
//   - 自定义解码接口：BindUnmarshaler
//   - 公开错误码常量：CodeInvalidJSON、CodeUnsupportedMediaType、CodeRequestTooLarge
//
// BindQuery 的当前职责：
//   - 只负责 query -> DTO 的 source-to-struct 映射
//   - 不处理 path/header
//   - 不内建 Required / Default / OneOf / Min / Max 等请求级规则
//
// 需要显式读取或校验单个 path/query 参数时，优先使用 reqx.Path(...) / reqx.Query(...)。
//
// BindBody 的当前契约：
//   - 只要实际读取到零字节 body，就直接视为 no-op
//   - 这个 no-op 发生在 Content-Type 检查之前
//   - 非空 body 必须是 application/json
//
// 为避免把不该由请求写入的字段暴露给外部输入，建议为 binding 单独定义 DTO，
// 再显式映射到业务对象，而不是直接把业务 struct 作为绑定目标。
//
// 根包 hah 也提供常用 facade：hah.BindQuery、hah.BindBody。需要 bind 包的
// 错误码常量、底层类型或更明确的来源分层时，再直接导入 bind 包。
package bind
