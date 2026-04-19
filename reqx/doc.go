// Package reqx 为基于 net/http 的 JSON API 提供输入侧能力。
//
// 它只覆盖 request 边界：
//   - 单字段 path/query helper：Path、Query
//   - DTO binder：BindQuery、BindBody
//   - 请求级显式规则：InvalidRequest
//
// Path / Query 是请求侧主路径。它们暴露两个 source root：
//   - PathParam
//   - QueryParam
//
// root 之后的公开 builder family 收敛为：
//   - StringParam
//   - ValueParam[T]
//   - OrderedParam[T]
//   - TimeParam
//   - MultiParam[T]
//
// 具体行为以 docs 设计文档为准：
//   - docs/path-design.md
//   - docs/query-design.md
//   - docs/binding-query-design.md
//   - docs/binding-body-design.md
package reqx
