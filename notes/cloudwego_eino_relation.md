# AI Agent 项目的基础技术选型：Go 选 CloudWeGo + Eino，Python 选什么？

> 来源：围绕 AI Agent 项目技术选型的学习整理
> 记录时间：2026-07-16

---

## 核心观点

如果要做 AI Agent 项目，建议先把技术栈拆成“服务层”和“AI 编排层”两层来看：在 Go 生态里，比较自然的组合是 **CloudWeGo + Eino**；在 Python 生态里，比较常见的组合是 **FastAPI + LangGraph / LangChain**。前者负责服务接入与工程化，后者负责模型调用、Agent、Workflow、Tool Calling 和 RAG 编排。

---

## 要点整理

### 1. 先把 AI Agent 项目拆成两层

很多人在做 Agent 项目时，会把“后端服务框架”和“AI 框架”混在一起讨论。更好的理解方式是把系统拆成两层：

1. **服务层**：负责 HTTP / RPC 接口、鉴权、路由、部署、治理、工程化。
2. **AI 编排层**：负责模型调用、Prompt、Agent、Workflow、Tool Calling、RAG 和记忆。

典型结构如下：

```text
客户端 / 上游系统
        ↓
服务层：HTTP / RPC / 网关 / 鉴权 / 任务入口
        ↓
业务逻辑层
        ↓
AI 编排层：LLM / Agent / Workflow / Tools / RAG
        ↓
模型服务 / 向量库 / 外部工具 / 知识源
```

一个常见误区是想用一个框架同时解决全部问题。实际上，大多数成熟方案都是“**服务框架 + AI 框架**”组合，而不是单框架通吃。

### 2. Go 方案：CloudWeGo + Eino

如果团队主语言是 Go，或者项目本身就是服务端工程化场景，比较自然的组合是：

- **CloudWeGo**：承担服务基础设施能力
- **Eino**：承担 AI 应用编排能力

#### 2.1 CloudWeGo 负责什么

CloudWeGo 是 Go 云原生服务开发生态，更偏底层和工程能力，重点解决：

- **HTTP 服务**：比如用 Hertz 提供 API
- **RPC 通信**：比如用 Kitex 做服务间调用
- **网络与性能**：比如 Netpoll 等高性能能力
- **IDL / 代码生成**：生成服务端和客户端样板代码
- **服务治理与工程化**：支持微服务体系下的标准接入方式

常见组件：

| 组件 | 定位 |
|------|------|
| **Hertz** | 高性能 HTTP 框架 |
| **Kitex** | 高性能 RPC 框架 |
| **Netpoll** | 高性能网络库 |
| **代码生成工具** | IDL 与工程代码生成 |

#### 2.2 Eino 负责什么

Eino 面向 LLM / Agent 场景，更偏 AI 应用层，重点解决：

- 模型调用封装
- Prompt 组织
- Agent / 多 Agent 编排
- Workflow / Graph 执行链路
- Tool Calling / MCP 工具接入
- RAG 与知识检索
- 可观测、回调和执行链路管理

一句话理解：

> **CloudWeGo 管服务底座，Eino 管 AI 逻辑。**

#### 2.3 Go 方案内部关系

Go 技术栈更适合这样分工：

```text
Hertz / Kitex
    └─ 负责把能力以 HTTP / RPC 服务形式暴露出去
业务服务层
    └─ 负责业务鉴权、任务调度、参数处理、结果回写
Eino
    └─ 负责模型调用、Agent 决策、Workflow 编排、Tool 调用、RAG
外部依赖
    └─ 模型服务、知识库、检索系统、工具系统
```

所以如果有人问：**“Go 做 AI Agent 项目，基础选型怎么搭？”**
比较推荐的答案是：

> **服务层用 CloudWeGo，AI 编排层用 Eino。**

### 3. Python 方案：FastAPI + LangGraph / LangChain

如果团队主语言是 Python，或者更强调 AI 生态完整度、原型迭代速度和社区现成能力，常见组合是：

- **FastAPI**：承担服务层能力
- **LangGraph / LangChain**：承担 AI 编排层能力

#### 3.1 FastAPI 负责什么

FastAPI 适合作为 Python 侧服务入口，负责：

- 暴露 HTTP API
- 参数校验与接口组织
- 异步处理与任务接入
- 对接 Web 服务、中台或前端系统

#### 3.2 LangGraph / LangChain 负责什么

在 Python 生态里，AI 编排层通常可以这样理解：

- **LangChain**：偏组件封装与能力拼装，适合快速接入模型、检索器、工具、记忆等基础模块。
- **LangGraph**：偏状态化、多步骤、可回退的 Agent / Workflow 编排，更适合复杂 Agent 系统。

一个实用判断是：

- 想快速拼模型、检索、工具：先看 **LangChain**
- 想做复杂多步骤 Agent、有状态流转、图式编排：优先看 **LangGraph**

#### 3.3 Python 方案内部关系

Python 技术栈一般可以这样分：

```text
FastAPI
    └─ 负责 API、服务入口、参数和网关接入
业务服务层
    └─ 负责任务组织、上下文注入、结果持久化
LangGraph / LangChain
    └─ 负责 Agent、Workflow、模型调用、Tool Calling、RAG
外部依赖
    └─ 模型接口、向量库、搜索、第三方工具
```

因此，如果是 Python 项目，更常见的推荐是：

> **服务层用 FastAPI，AI 编排层用 LangGraph / LangChain。**

### 4. Go 和 Python 怎么选

可以从三个角度判断：

#### 4.1 团队已有工程栈

- 团队本身是 **Go 后端团队**：优先 Go 方案，工程接入更顺滑
- 团队本身是 **Python / 算法 / AI 团队**：优先 Python 方案，生态和样例更多

#### 4.2 项目目标

- 更偏**线上服务化、性能、工程治理**：Go 方案更自然
- 更偏**快速验证、AI 生态复用、原型迭代**：Python 方案更顺手

#### 4.3 系统复杂度

- 如果是“把 AI 能力接到成熟服务体系里”：Go 更合适
- 如果是“先把 Agent 跑起来，再逐步产品化”：Python 更快

可以粗略概括为：

| 场景 | 更推荐 |
|------|--------|
| 现有系统本身是 Go 微服务体系 | Go：CloudWeGo + Eino |
| AI 原型验证、实验速度优先 | Python：FastAPI + LangGraph / LangChain |
| 要把 Agent 做成稳定线上接口 | 两者都可以，但 Go 更适合深度服务化 |
| 要快速接入大量 AI 生态组件 | Python 更占优势 |

### 5. 技术选型内部关系怎么讲更清楚

无论 Go 还是 Python，最重要的是把“谁负责什么”讲清楚：

#### Go 侧

- **CloudWeGo**：服务接入、通信、性能、工程化
- **Eino**：模型调用、Agent、Workflow、Tools、RAG

#### Python 侧

- **FastAPI**：服务接入和 API 暴露
- **LangGraph / LangChain**：模型与 Agent 编排

所以对外讲技术选型时，最好的表达方式不是“推荐某一个框架”，而是：

> **推荐一套分层组合。**

例如：

- Go：**CloudWeGo + Eino**
- Python：**FastAPI + LangGraph / LangChain**

### 6. 实践印证：当前项目中的 Go 组合方式

当前项目就是一个典型的 Go 方案例子：

- `go.mod` 同时引入了 `code.byted.org/middleware/hertz`、`github.com/cloudwego/eino`、`github.com/cloudwego/eino-ext/...`，说明项目同时具备服务框架和 AI 编排框架。
- `router.go` 使用 Hertz 注册 HTTP 路由，例如 `/v1/agent/run`、`/v1/subagent/run` 等入口。
- `agent/init.go` 中初始化 Eino 环境、注册 callbacks、创建 ChatModel，说明 Agent 和模型调用逻辑由 Eino 承接。

这正是“**Hertz 负责入口，Eino 负责 AI 执行链路**”的典型结构。

---

## 总结

- AI Agent 项目最好先拆成“服务层 + AI 编排层”两层。
- **Go 方案**推荐：**CloudWeGo + Eino**。
- **Python 方案**推荐：**FastAPI + LangGraph / LangChain**。
- CloudWeGo / FastAPI 解决的是服务接入与工程化问题。
- Eino / LangGraph / LangChain 解决的是模型调用、Agent、Workflow、Tool Calling 和 RAG 编排问题。
- 真正好的技术选型，不是选一个框架，而是选一套清晰分层、职责明确、方便演进的组合。

---

## 原文链接

- 本笔记来自对 AI Agent 项目技术选型的问答整理，无外部原文链接。
