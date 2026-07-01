# 用混沌工程验证 AI Agent 的 Harness Engineering 防线

> 来源：https://bytetech.info/articles/7624103485874667530
> 记录时间：2026-05-11

---

## 核心观点

Agent 系统的稳定性是工程问题，不是模型问题——用混沌工程（Chaos Engineering）对 Harness 约束层进行验证，是 Agent 上线前必不可少的一道防线。

---

## 要点整理

### 一、从强契约到非确定性状态机

微服务时代的稳定性治理依赖强类型接口和确定性契约（Thrift IDL、gRPC Schema、DDD 限界上下文）。只要 Schema 对齐，出参行为空间完全可预测。

Agent 架构打破了这个前提：
- Agent 控制流由 LLM 在运行时动态决定，本质上是**以大模型为核心的非确定性状态机**
- 控制逻辑不再硬编码在 if-else 分支里，而是交给有幻觉风险的概率模型
- 外部工具（MCP）一次瞬时抖动，在非确定性决策链条中可能被逐级放大，演变为**级联错误（Cascading Errors）**

业界由此从 Prompt Engineering/Context Engineering，转向 **Harness Engineering（线束工程/治理工程）**。

核心思路：无法消除 LLM 的不确定性，但可以通过足够坚固的约束层（Harness）来限定行为边界——类似于给一匹难以驯服的马套上缰绳。

### 二、agent-byte-chaos 的定位

`agent-byte-chaos` 的职责是：在外部依赖（MCP / LLM）**返回结果的链路上**，动态拦截并注入故障（429 限流、500 错误、脏数据等），验证 Harness 层的各项约束——循环检测、断路器、错误回注——是否能在真实故障场景下正确生效。

核心使用模式：
```python
# 先定义基线场景
baseline = BaselineScenario(...)

# 通过 baseline.variant() 派生混沌变体
chaos_variant = baseline.variant(
    chaos=tool_error("process_refund", error="ServiceUnavailable, please try again"),
    assertions=[retry_count < 3, graceful_degradation]
)
```

### 三、八个典型失败模式与混沌注入验证

| 失败模式 | 混沌注入方式 | 暴露的问题 | Harness 修复策略 |
|---------|------------|-----------|----------------|
| **上下文耗尽** | `context_mutate` 注入 ~2M 字符 | LLM 调用因 context_length 超限直接崩溃 | 窗口压缩层、外部记忆、异常捕获优雅降级 |
| **中间丢失** | `context_mutate` 注入 80 条伪造商品评价 + 虚假退款历史 | Agent 被伪造退款历史误导，引用虚假退款单号 REF-888 | System Prompt 防护（聚焦最新请求）、必须调用工具核实 |
| **工具误路由** | `tool_mutate` 篡改 `lookup_order` 返回值为支付扣款响应 | Agent 将"已成功扣款 299 元"当订单状态展示，客户恐慌 | 工具响应校验、TOOL_CALL_LEDGER 全局账本工具验证 |
| **幻觉工具调用** | `tool_error` 持续返回 ToolNotFoundError | Agent 在工具报错时编造虚假退款成功信息 | 工具响应校验、意图识别/真实工具调用记录验证 |
| **重试死循环** | `tool_error` 持续返回 503 + "请务必重试" | Agent 无限重试，耗尽递归步骤或导致 429 限流 | 重试器限制重试次数、实现"人工客服"工具优雅降级 |
| **状态腐败** | `history_truncate` 截断对话历史（仅保留最后 1 条） | 丢失所有前序上下文（订单号、退款资格），流程中断 | Session 机制 + 关键信息缺失时调用 `get_session_context` |
| **过早终止** | `context_mutate` 注入伪造"退款已完成"对话历史 | Agent 不真正执行退款，直接告知用户已完成 | System Prompt 防护：禁止信任历史中的退款信息 |
| **超时级联** | 综合延迟注入 | 延迟层层叠加，整体耗时远超预期 | 超时熔断 |

**四大问题场景汇总：**
- 🔴 **Token 消耗**：上下文超限 + 重试死循环 → 外部记忆、重试器限制
- 🟠 **超时级联**：延迟叠加 → 超时熔断
- 🟡 **工具/上下文导致模型幻觉**：中间迷失、工具误路由等 → 工具响应校验 + 上下文校验
- 🔵 **信息丢失**：状态腐败 → 外部记忆、记录恢复机制

### 四、真实案例：631 秒 Doom Loop 连锁反应

在某风险挖掘 Agent 的上线前验证中，使用 `ChaosSpace.tool_focused()` 策略对核心查询工具注入持续性 `tool_error("Temporary unavailable, please try again")`：

- **调用爆炸**：单次请求生命周期内，Agent 向 LLM 发起了 **95 次**无效重试调用
- **延迟雪崩**：单次任务耗时飙升至 **631 秒**，足以打满集群的 TPM 限流配额

根因：Agent 没有实现循环检测——工具返回的错误消息中包含 `"please try again"` 字样，模型忠实地执行了这条"指令"，形成了 Doom Loop。

修复后：Agent 在第 3 次重试后自动切换策略返回降级结果，整体耗时从 631 秒降到 **47 秒**。

这类由环境劣化与模型非确定性共同触发的问题，**在常规的功能测试和集成测试中几乎不可能被覆盖到**。

### 五、接入方式

- 提供自动代码生成的 skills（可在内部 skills 市场搜索 `agent-byte-chaos`）
- 静态代码审计：`agent-resilience-audit`
- 目前适配 Python LangChain/LangGraph 等编写的 Agent 应用
- 接入方式：IDE 插件直接接入、内部 AI 平台接入、SDK 包编写测试

---

## 参考链接

- [原文链接](https://bytetech.info/articles/7624103485874667530)
