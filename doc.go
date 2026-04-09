// Package hah 提供根包常用的请求/响应边界入口，聚合 bind、reqx 与 resp 中最常用的一组能力。
//
// 适合在大多数 handler 中直接使用：
//   - 默认请求绑定与校验入口
//   - 常用 JSON 成功响应辅助
//   - 统一错误响应写回
//
// 如果你需要更细粒度的包级 API，请直接导入 bind、reqx 或 resp。
package hah
