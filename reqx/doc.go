// Package reqx 为基于 net/http 的 JSON API 提供输入侧能力。
//
// 它只覆盖 request 边界：
//   - 单字段 path/query helper：Path、Query
//   - DTO binder：BindQuery、BindBody
//   - 请求级显式规则：InvalidRequest
//   - request-side field error 公开模型：FieldError、FieldErrorCode、FieldErrorIn、Code*、In*
//
// reqx 是请求输入侧的较低层原生面。
// 对多数 handler 调用方，优先直接使用根包 hah；reqx 主要服务于：
//   - 拆分 request-side 组件
//   - 直接依赖输入层公开契约
//   - 需要显式引用 request-side FieldError / Code* / In* 的场景
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
// 这组 builder family 已视为核心公开面，应保持收敛，不轻易继续扩展新的 family、
// source root 或隐式 binding 模式。
//
// 具体行为以 docs 设计文档为准：
//   - docs/path-design.md
//   - docs/query-design.md
//   - docs/binding-query-design.md
//   - docs/binding-body-design.md
//   - docs/public-api-scope.md
package reqx
