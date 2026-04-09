// Package bind 为基于 net/http 的 JSON API 提供请求绑定能力。
//
// 它只负责把 HTTP 输入映射到目标值，不处理 Normalize、请求级规则或字段校验。
// 如果需要“绑定 + 请求级规则 + 结构校验”的组合入口，请使用 reqx 或根包 hah 的
// BindAndValidate* 系列。
//
// 当前支持的数据源：
//   - query：query 参数
//   - param：path 参数
//   - header：请求头
//   - json：请求 body
//
// json body 当前只支持 application/json，并使用 Go 标准库 encoding/json 解码；
// 也不接受 application/*+json。
//
// 公开 API：
//   - 绑定入口：Bind、BindBody、BindQueryParams、BindPathValues、BindHeaders
//   - binder 相关类型：Binder、DefaultBinder、BindUnmarshaler
//   - 公开错误码常量：CodeInvalidJSON、CodeUnsupportedMediaType、CodeRequestTooLarge
//
// 默认 Bind 顺序固定为：path -> query(GET/DELETE/HEAD) -> body。
// header 不参与默认 Bind；如需绑定 header，请显式调用 BindHeaders。
//
// BindBody 的当前契约：
//   - 只要实际读取到零字节 body，就直接视为 no-op
//   - 这个 no-op 发生在 Content-Type 检查之前
//   - 非空 body 必须是 application/json
//
// 为避免把不该由请求写入的字段暴露给外部输入，建议为 binding 单独定义 DTO，
// 再显式映射到业务对象，而不是直接把业务 struct 作为绑定目标。
//
// 根包 hah 对这组 API 提供了薄封装：hah.Bind、hah.BindBody、
// hah.BindQueryParams、hah.BindPathValues、hah.BindHeaders。
package bind
