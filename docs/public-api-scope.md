# hah 公开 API 定位与演进约束

- 状态：Active
- 适用范围：
  - 根包 `hah`
  - 请求侧公开包 `reqx`
- 目标：
  - 固定默认入口
  - 控制公开面规模
  - 为后续评审提供明确非目标

## 1. 默认入口

对外默认入口固定为根包 `hah`。

它承担：

- 默认 request helper 入口：`hah.Path(...)`、`hah.Query(...)`
- 默认 DTO binder 入口：`hah.BindQuery(...)`、`hah.BindBody(...)`
- 默认显式请求规则入口：`hah.InvalidRequest(...)`
- 默认公共错误模型入口：`hah.HTTPError` 与常用构造器
- 默认响应写回入口：`hah.WriteError(...)`、`hah.OK(...)`、`hah.Created(...)` 等
- 默认错误归一化入口：`hah.NormalizeError(...)`

因此：

- README、示例与主要文档默认都应以 `hah.xx` 为主路径
- 新增能力时，优先判断是否应继续收敛在现有 `hah` 入口下
- 不把 `reqx` 当作并列主入口宣传

## 2. reqx 定位

`reqx` 仍是公开包，但定位固定为较低层的 request-side 原生面。

它主要服务于：

- 拆分输入层组件
- 直接依赖 request-side 契约的库作者
- 需要显式使用 `FieldError` / `Code*` / `In*` 的场景

`reqx` 的存在不是为了提供第二套“推荐上手路径”，而是为了暴露请求输入层的稳定公开契约。

## 3. 核心公开面

以下公开面视为核心能力，应优先保持稳定与收敛：

- `Path(...)` / `Query(...)`
- `BindQuery(...)` / `BindBody(...)`
- `InvalidRequest(...)`
- `FieldError` / `Code*` / `In*`
- `HTTPError` 及根包错误构造器
- `NormalizeError(...)`
- `WriteError(...)` 与默认成功响应 helper

其中 `Path / Query` 的 builder family 已经形成稳定心智模型，不轻易扩展新的 family。

## 4. 非目标

当前不以以下方向为目标：

- 引入新的 router 绑定或 handler 生命周期抽象
- 把 `BindQuery(...)` 演进成通用 form/query decoder
- 支持嵌套 DTO、自动展开、slice/map 多值自动绑定等复杂 query 投影
- 解析 `validate:"..."` 等第三方校验 tag
- 提供 `BindAndValidate` 之类把绑定与校验耦合的入口
- 提供跨 source 的混合绑定，例如同一入口同时处理 path/query/header/body
- 为潜在扩展点预先设计接口或泛化层

如果用户需要更自由、更复杂的绑定模型，应由调用方组合标准库或第三方库解决，而不是让 `hah`/`reqx` 吞并这类职责。

## 5. 演进约束

评审公开 API 变更时，默认先问：

1. 这项能力是否能通过现有 `hah` 入口表达？
2. 它是在澄清既有契约，还是在扩大公开面？
3. 它会不会削弱当前“显式、收窄、可预测”的心智模型？
4. 它是否把应用层规则、router 职责或 validation engine 职责重新拉回边界层？

除非有非常明确的收益与证据，否则默认选择：

- 修正文档与示例，而不是新增 API
- 在现有入口内补足测试，而不是扩展 builder family
- 保持 `usage error` 与客户端输入错误的边界稳定
