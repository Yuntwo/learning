# Harness Engineering 生产级实战

> 来源：https://bytetech.info/articles/7623633454748352521  
> 作者：陈超（音乐业务 Server 工程师）  
> 记录时间：2026-05-08

---

## 核心观点

Harness Engineering 是一种**面向 Agent 的新型软件工程范式**，工程师的职责从「写代码」转变为「构建让 Agent 可靠运行的世界」。作者结合 AI Agent 产品开发实践，分享了以 Local Mode 为核心的验证体系落地方案。

---

## 要点整理

### 一、什么是 Harness Engineering

**定义**：HE 围绕 Claude Code、Codex 等 Coding Agent，构建一整套工程框架与基础设施。核心不再是「直接编写代码」，而是通过**精心设计运行环境与高效的反馈闭环**，引导 Agent 高质量地产出。

工程师角色：`代码生产者` → `系统框架与反馈机制的设计者`

**Agent Harness vs Harness Engineering 的区别**：
- Agent Harness：针对单个 Agent 的脚手架/工具
- Harness Engineering：完整的工程体系，关注全链路

---

### 二、OpenAI 提炼的 Harness Engineering 5 要素

#### 1.1 状态机作为控制层
- 状态机节点和配置本地可管理、可追踪
- 「让 Agent 能够直接从代码仓库推理出完整的业务领域」
- 倾向于选择「枯燥」但可完全内化于仓库的依赖和抽象（100% 测试覆盖、行为符合运行时预期）

#### 1.2 代码仓库作为文档系统
- `AGENTS.md` 视为内容目录，代码仓库的知识库统一在 `docs/` 结构化目录中
- OpenAI docs/ 包含：设计文档、执行计划、DB schema、产品说明、参考知识等
- **计划被视为重要工具**：临时轻量计划用于小幅变更，复杂工作则记录在执行计划中，并附带进度和决策日志
- 通过结构化文档体系，可以持续约束 Agent 的行为
- 专职 linter 和 CI 作业会验证知识库的更新状况，是否已交叉链接且结构正确

#### 1.3 规范架构与品味
- 把架构规则从「文档约定」转化为「自动执行的系统规则」
- 通过**自定义代码检查器**和**结构测试**强制保证架构问题（Codex 生成 linter）
- 每个业务域划分为固定的层，依赖方向经严格验证，只允许有限的一组进

#### 1.4 运行时验证闭环（最核心）
「修改代码 → 验证结果 → 再修改」的闭环，由 Agent 自动完成：
- 应用程序根据 Git worktree 启动，Codex 每次更改都重新启动并驱动一个实例
- 集成 Chrome DevTools 协议，创建 DOM 快照、屏幕截图和导航技能
- **Codex 能复现错误、验证修复、并直接推理 UI 行为**

可观测性三件套对 Agent 可见：
- 日志（log）、指标（metrics）、追踪记录（trace）通过本地可观测性堆栈展示给 Codex
- 这些数据在任务完成后全部删除，任务期间是临时的

#### 1.5 CR 与代码垃圾处理
- **自动 CR**：Codex 在本地审核自身更改，并向其他智能体请求代码审核，循环直到所有智能体满意（Ralph Wiggum 循环）
- 人类可以审核 PR，但几乎所有审核工作已调整为智能体对智能体的方式
- **缩短 PR 周期**：吞吐量变了合并的理念，更短、更频繁的合并

---

### 三、作者的生产级实践：Local Mode 验证体系

#### 背景问题
开发 AI Agent 产品时，TCC（动态配置中心）信息主要分布在平台侧，对 Agent 不友好。没有本地配置字段说明，Agent 理解成本会明显上升。

**第一实践点**：要求 Agent 使用内部 CLI 工具和相关 skill 来理解和开发配置相关能力，Agent 可直接通过命令行查询 namespace、配置项、目录、环境和已有值。

#### 2.2 验证方式对比

| 维度 | Local Mode（本地 Mock） | BOE | PPE |
|------|----------------------|-----|-----|
| 安全性 | 最高，只影响本地数据 | 较高，离线环境 | 较低，接近线上 |
| 代码开发成本 | 较高，需 Mock 各类中间件 | 中等 | 极低 |
| 配置接入成本 | 低，几乎不需要平台操作 | 中高 | 低，基建成熟 |
| Agent 友好度 | **最高**，可直接用 bash 操作本地 DB、缓存、配置 | 中低 | 中低 |
| 功能验证能力 | 较高，能验证大部分业务逻辑 | 高 | 极高 |

Local Mode 优势：
- 安全，只影响本地数据
- Agent 可通过 bash 工具直接控制依赖、可观测性强
- **可以低成本重复执行**

#### 3.1 Local Mode 组件对照表

| 组件类别 | 生产实现 | 本地 Mock 实现 |
|---------|---------|--------------|
| 数据库 | MySQL | SQLite (`./data/mysql/local.db`) |
| 缓存 | ABase（兼容 Redis） | JSONL 文件存储 |
| 配置中心 | TCC 远程配置 | 本地 JSON 文件 |
| 日志 | 内部可观测链路 | 本地文件日志 |
| Prompt 管理 | 内部版本控制 | 本地文件目录 |

启动脚本 `local_run.sh`：
```bash
export LOCAL_MODE=true
export HERTZ_CONF_DIR=./conf
export HERTZ_LOG_DIR=./log
```

#### 3.2 关键设计细节

**MySQL → SQLite**：
- 数据库完全保留，只是把 MySQL 替换成 SQLite，结构零改动
- 落盘：`./data/mysql/local.db`

**缓存 → JSONL 文件**：
- ZSET 类型 → JSONL 文件，每行都是一个 JSON
- 选择 JSONL 最重要的原因：**可读性**——更方便 Agent 分析，也便人工直接查看

**TCC & Prompt → 本地文件**：
```go
path := fmt.Sprintf("./local_conf/prompts/%s.txt", e.key)
```
- `local_conf` 下的 TCC、Prompt 等本地配置**纳入 Git 追踪**——配置从临时调试数据变为可同步、可回溯的工程记录
- 配置同步 PPE 与本地：借助内部 CLI Skills 让 Agent 做同步和更新，开发→本地验证→推送 PPE，全程由 Agent 完成

**日志验证示例**：
```bash
rg -n "\[middleware log\]|REQ:|RESP:" log/app/P.S.M.log
```

#### 3.3 核心收益

**收益一：高效闭环验证**
- 验证链路从 PPE 的多步平台操作，变为 Agent 本地闭环：
  1. 发送请求
  2. 检查 `data/mysql/local.db`（核心数据）
  3. 检查 `log/app/P.S.M.log`（关键日志）
  4. 检查 `data/abase/`（状态和事件流）
- 发现问题立刻改代码、改配置并再次发起验证，**循环非常紧凑**
- 实践结果：重构从开始到方案、开发、验证只用了三到四个小时，**只用了预计手工编程 1/10 的时间**

**收益二：安全边界与试错灵活性**
- Agent 可以肆无忌惮地修改 SQLite 的表结构（加字段、加索引）来验证逻辑
- PPE 涉及 Schema 变更需要严格方案审查，Agent 无法进行高自由度的试错

**收益三：Agent 上下文与验证的可靠性**
- CUA（计算机使用 Agent）面对 GUI 界面会产生大量「上下文噪音」，增加幻觉风险
- Local Mode 提供纯净、结构化的上下文（精准的 API 响应和 JSON 日志），保证 Agent 验证的可靠性

#### 3.4 Local Mode 的边界

一个典型 Bad Case：流式输出链路在 PPE 出现明显卡顿，但本地怎么跑都很顺。根因：
1. Local Mode 下，一个安全上报组件没有启用
2. 这个 SDK 在线上做上报时是**同步阻塞**的
3. 阻塞点刚好压在流更新过程里，直接拖慢输出节奏

**结论：Local Mode 更适合承担主链路回归和高频调试；涉及安全组件、网关、外部 SDK 和平台注入能力的链路，仍然要在 PPE 做验证。**

---

### 四、思考与展望

#### 4.1 当前不足
- Harness 工程对 RPC 等能力的验证性较差
- PPE 测试时 Agent 可观测性较差（无法查询 DB、缓存、log）
- 缺少自动 CR、定期代码垃圾清理
- 仓库文档建设相对 OpenAI 还比较薄弱

#### 4.2 遗留系统的应对
- 渐进式推进：先在力所能及的范围内，让 AI 能做到部分闭环验证，再逐步扩大 Harness 覆盖范围
- 先补充单元测试能力；尝试让 Agent 在项目写 PPE 测试
- **对于新系统，建议从第一天起就必须构建 Harness 环境。** 就像今天的项目必须有 CI/CD 一样，未来的项目必须有 Harness。

#### 4.3 对工程师的新要求

未来优秀工程师需具备三种新能力：

1. **结构化表达问题与约束的能力**：把模糊需求、隐性业务规则、架构边界，转化为结构化的输入（文档、Schema、约束规则、测试用例），从而减少 Agent 的不确定性

2. **设计反馈闭环的能力**：效率不再取决于「写代码的速度」，而取决于「验证与迭代的吞吐量」。谁能把反馈回路做短，谁就能真正提升生产力

3. **工程环境抽象能力**：数据库、配置中心、日志系统、外部依赖——这些原本为人设计的系统，需要被重新抽象为 Agent 可读、可控、可验证的形态

**终局**：从需求理解、方案设计、代码实现、功能验证到代码审查，Agent 能够自主完成全链路闭环，工程师只在关键决策点介入。

> 代码正在变得廉价，但正确性、可控性和演进能力，正在变得更加昂贵。
> 
> 我们不再亲手写代码，而是构建让 Agent 可靠运行的世界。

---

## 参考链接

- [原文](https://bytetech.info/articles/7623633454748352521)
- [Ryan Lopopolo - Harness Engineering: Working with Codex in an Agent-First World (OpenAI, 2026.02.11)](https://openai.com/zh-Hans-CN/index/harness-engineering/)
- [BirGitta Böckeler - Harness Engineering (MartinFowler.com, 2026.02.17)](https://martinfowler.com/articles/harness-engineering.html)
