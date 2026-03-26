# 日志记录策略

本文面向 `hah` 维护者与内部 review，说明当前错误日志策略、职责边界、默认实现、已知限制与后续变更原则。

本文不是公开 API 文档。

- 对外公开契约仍以 `README.md` 为准
- 实现原则仍以 `docs/TECHNICAL_GUIDE.md` 为总纲
- 本文只讨论日志记录与错误观测策略

## 1. 文档目标

本文解决四个问题：

- 当前有哪些日志会被记录
- 哪些日志不会被记录，为什么
- 不接集中观测平台时，默认 stderr 策略是否足够
- 以后调整日志策略时，应该按什么原则 review

## 2. 当前结论

当前默认策略如下：

- `5xx`：记录错误摘要日志，并追加一条当前 goroutine 的 stack 日志
- `401/403`：记录安全审计日志
- 普通 `4xx`：默认不单独记录错误日志
- panic：不由 `hah` 记录，交给外层 recoverer，例如 `chi/middleware.Recoverer`
- 请求访问日志：不由 `hah` 记录，交给外层 access log middleware，例如 `chi/middleware.Logger`

当前默认策略的目标不是替代集中观测平台，而是给本地、Docker、Kubernetes stderr 提供一个足够实用的基础排障基线。

## 3. 设计目标

日志策略需要同时满足以下目标：

- 保证真正重要的内部失败不会静默丢失
- 让常见排障路径在只看 stderr 的情况下仍然可用
- 避免把高频、低价值的 `4xx` 全量打成错误日志
- 不让日志策略反向污染公开错误契约
- 不要求调用方理解一套复杂的内部分类模型

## 4. 非目标

以下内容不属于当前默认策略的目标：

- 不保证替代 Sentry、ELK、OTel 这类集中观测能力
- 不保证为普通 `error` 自动还原“真实错误发生点”的原始 stack
- 不保证自动区分 handler、service、repository 的精细阶段
- 不对所有 `4xx` 逐条产生日志
- 不接管 panic capture

## 5. 职责边界

### 5.1 `hah`

`hah` 负责：

- 统一接收显式送入边界的错误
- 统一做错误映射
- 统一产出内部错误观测
- 在错误响应写回失败时追加二次观测

`hah` 不负责：

- 全量访问日志
- panic recover
- 集中式日志存储、索引、告警、检索

### 5.2 `chi` 或外层 HTTP runtime

外层 middleware 通常负责：

- access log
- panic recover
- request id 注入或桥接
- tracing

如果外层没有显式设置 request id，`hah` 会在第一次发送错误观测时惰性生成一个，仅用于当前错误链路的内部关联。

### 5.3 Kubernetes / Docker

当前默认 logger 输出到 `stderr`。

这意味着：

- 在本地运行时，可以直接从终端看到
- 在 Docker 中，会进入容器标准错误输出
- 在 Kubernetes 中，可通过 `kubectl logs` 或集群日志采集方案读取

这不等于已经具备集中观测能力。stderr 只是默认输出介质，不是日志平台。

## 6. 观测模型

### 6.1 `ErrorReport`

`hah` 内部错误观测通过 `ErrorReport` 产出，关键字段包括：

- `Error`：当前这次观测对应的原始 `error`
- `PublicError`：对外公开的边界错误
- `Stage`：内部阶段标签
- `RequestID`：当前错误观测实际使用的请求标识
- `ResponseStarted`：错误发生时响应是否已经开始写回

`RequestID` 的更完整策略见 `docs/REQUEST_ID_STRATEGY.md`。

### 6.2 `stage`

当前稳定阶段值只有四个：

- `decode`
- `validate`
- `processing`
- `write_response`

语义说明：

- `decode`：请求解码失败
- `validate`：请求校验失败
- `processing`：请求已进入业务处理链，覆盖 handler、service、repository 这一整段
- `write_response`：成功响应写回失败，或错误响应序列化/写回失败

### 6.3 为什么没有 handler / service / repository

当前实现不会自动推导 handler、service、repository 三个细粒度阶段。

原因：

- 到边界统一出口时，只剩下 `error` 值，通常无法可靠判断错误首次出现于哪一层
- 强行自动区分会制造伪精确
- `processing` 更诚实，也更稳定

如果以后要引入更细粒度阶段，应通过明确埋点或专门装饰器完成，而不是靠字符串启发式猜测。

## 7. 当前默认日志策略

### 7.1 `5xx`

当前默认会记录两条 stderr 日志：

1. 错误摘要日志
2. stack 日志

错误摘要日志当前包含：

- `err`
- `err_type`
- `status`
- `code`
- `stage`
- `method`
- `target`
- `remote`
- `request_id`
- `started`

stack 日志当前包含：

- `request_id`
- `stage`
- `debug.Stack()` 输出

这里要特别说明：

- 当前记录的是 reporter 执行点的 goroutine stack
- 这不是业务错误对象天然携带的 origin stack
- 它主要用于提高基础排障效率，而不是替代真正的 error tracing

### 7.2 `401/403`

当前默认会记录安全审计日志，但不会追加 stack。

原因：

- `401/403` 往往是预期内边界拒绝
- 它们有审计价值，但通常没有必要按内部故障处理
- 打 stack 噪音通常大于价值

当前安全审计日志字段与 `5xx` 摘要日志保持同一风格，便于统一检索。

### 7.3 普通 `4xx`

当前默认不单独记录错误日志。

原因：

- 普通 `400/404/405/409/422` 在 API 服务中往往高频且预期
- 这类错误更适合通过 access log、metrics、看板和采样分析观察
- 全量记录错误日志会显著增加噪音，降低真正内部故障的可见性

### 7.4 panic

panic 不属于 `hah` 默认处理范围。

推荐由外层 recoverer 负责：

- 记录 panic
- 记录 stack
- 产出 500

### 7.5 错误响应再次失败

如果错误响应自身在序列化或写回阶段再次失败，当前策略会：

- 追加一条 `Stage == "write_response"` 的内部观测
- 在响应尚未提交时，尽可能保守回退为 `500 internal_error`
- 如果响应已经开始或写入过程中失败，则不再承诺改写客户端最终看到的结果

这条策略的目标是确保：

- 当客户端最终看到 `500` 时，内部日志中一定有对应记录
- 写错误响应失败不会静默吞掉

## 8. 典型链路

### 8.1 参数错误

请求链路：

- 请求进入
- `DecodeJSON` 失败
- `hah` 生成 `400 invalid_json`
- `stage == decode`

默认日志结果：

- 有 access log
- 没有 `hah` 错误日志

### 8.2 鉴权失败

请求链路：

- auth middleware 拒绝请求
- 不进入 `hah` 业务边界
- 直接返回 `401` 或 `403`

默认日志结果：

- 有 access log
- 有安全审计日志
- 没有 stack 日志

### 8.3 业务内部失败

请求链路：

- 业务处理链返回未识别错误
- `hah` 映射为 `500 internal_error`
- `stage == processing`

默认日志结果：

- 有 access log
- 有 `5xx` 摘要日志
- 有 stack 日志

### 8.4 写错误响应失败

请求链路：

- 原始错误被映射
- 写错误响应时序列化或写 socket 失败
- `stage == write_response`

默认日志结果：

- 原始错误会先产出一条观测
- 写回失败会再产出第二条观测
- 对外尽可能保守回退

## 9. 为什么当前策略是合理的

当前策略的工程取舍是：

- 访问日志负责“所有请求都发生了什么”
- `hah` 错误日志负责“哪些内部失败值得人工关注”
- 安全日志负责“哪些拒绝事件值得审计”

这比“所有 `4xx`/`5xx` 都打错误日志”更适合大多数 API 服务：

- 内部 `5xx` 更容易被看见
- `401/403` 有专门出口
- 普通参数错误不会淹没日志流

## 10. 已知限制

当前策略仍然存在以下限制：

- 普通 `error` 的 stack 不是 origin stack，只是 reporter 当下的 goroutine stack
- 不接集中观测平台时，日志检索能力依赖 stderr 和运行环境
- `processing` 阶段无法自动细分到 handler、service、repository
- 普通 `4xx` 只能从 access log 和指标看，不会出现在 `hah` 默认错误日志里

这些限制是当前策略的有意取舍，不是遗漏。

## 11. 何时考虑调整 `4xx` 策略

如果出现以下情况，可以重新评估是否要扩大 `4xx` 日志范围：

- 某类 `4xx` 对安全、合规、审计有明确要求
- 某类 `4xx` 的异常激增需要快速定位
- access log 和 metrics 已不足以支撑排障
- 某类 `4xx` 实际上代表内部配置错误或网关异常

调整时优先顺序建议如下：

1. 优先只扩大特定状态码或特定 error code
2. 优先新增采样或聚合策略
3. 最后才考虑对所有 `4xx` 全量记错误日志

推荐优先评估的候选：

- `401`
- `403`
- `429`
- 异常激增的 `400/422`

## 12. 以后接入观测平台时的建议

如果以后接入 Sentry、ELK、OpenTelemetry 或其他平台，推荐做法是：

- 保持 `ErrorReport` 作为统一输入
- 用自定义 `ErrorReporter` 把 `ErrorReport.Error` 原样发送到平台
- 不要把 stderr logger 当成最终归宿
- 保留当前 stderr 作为本地与最小部署环境的 fallback

接入平台后，stderr 策略仍然有价值：

- 本地开发可见
- 集成环境和临时环境不依赖平台配置也能排障
- 平台故障时仍保留最低限度的观察能力

## 13. 变更 review 原则

以后改日志策略时，review 至少应回答以下问题：

- 这次改动是否改变了默认会记录哪些状态码
- 这次改动是否改变了 `stage` 语义
- 这次改动是否增加了高频低价值日志
- 这次改动是否会泄漏不应公开的内部信息
- 这次改动是否影响 K8s stderr 的可用性
- 这次改动是否会与 `chi.Logger` / `chi.Recoverer` 的职责冲突
- 这次改动是否仍保留 `write_response` 二次观测
- 这次改动是否需要同步 `README.md` 与 `docs/TECHNICAL_GUIDE.md`

## 14. 变更 checklist

如果后续需要改默认日志策略，提交前至少应检查：

- 修改后的 `ErrorReport` 语义是否清晰
- 默认 stderr 文案是否仍便于 grep / 检索
- `request_id` 是否仍在所有关键路径出现
- `5xx` 是否仍有足够排障信息
- `401/403` 是否仍有审计信息
- 普通 `4xx` 是否真的需要新增错误日志
- `go test ./...` 是否通过
- `make test-cover` 是否仍保持预期

## 15. 测试建议

与日志策略相关的测试，至少应覆盖：

- `5xx` 会产出错误摘要日志
- `5xx` 会产出 stack 日志
- `401/403` 会产出安全审计日志
- 普通 `4xx` 默认不会产生日志
- `write_response` 二次失败会追加观测
- nil request / nil reporter 等 guard 分支

## 16. 当前推荐结论

在没有集中观测平台的前提下，当前推荐继续保持：

- access log：交给外层 middleware
- panic log：交给外层 recoverer
- `5xx`：`hah` 记录摘要和 stack
- `401/403`：`hah` 记录安全审计日志
- 普通 `4xx`：先不单独记错误日志

这是当前阶段最平衡的默认策略。
