# AI 应用开发技术选型：分层视角

> 来源：对话沉淀（LangChain/LangGraph/Eino 及 Go AI 框架讨论 + 平台分层补充）
> 记录时间：2026-06-17

---

## 核心观点

AI 应用的技术选型不是"选哪个框架"的单点问题，而应该按 **定制化程度** 分三层来看：
**低代码平台（快、但定制弱） → 代码开发框架（灵活、生产级） → Skill/Prompt/Tool 托管与调优平台（横切，给前两层做工程化支撑）**。
选型先定层次，再在层内选具体工具。

---

## 一、三层技术选型框架

### 第 1 层：低代码 / 平台化（面向业务快速交付）

特征：拖拽 / 配置式搭建 Agent，发布快，但**代码定制程度低**，适合业务运营、非工程角色快速产出。

- **抖音 AI 平台**：面向抖音生态，能快捷发布各种形式的 Agent；内置如**经营分析**等业务能力。
  - 优势：贴合生态、上手快、发布链路短。
  - 局限：代码定制程度低，复杂逻辑 / 深度集成受限。
- 适用场景：业务侧快速验证、运营类 Agent、标准化场景。

### 第 2 层：代码开发框架（面向工程，深度定制）

特征：用代码组合 LLM、工具、检索、记忆、编排，生产可控、可深度定制。按语言分：

**Python 生态（最成熟）**
- **LangChain**：基础框架，把 LLM/Prompt/工具/检索/记忆封装成可组合的"链（Chain / Runnable）"，集成生态最大。偏线性管道。
- **LangGraph**：同公司（LangChain Inc.）出品，在 LangChain 之上的**编排层**，用"图（StateGraph：节点+边+状态）"构建有状态、可循环、可分支、可人工介入的 Agent 流程。
  - 关系：**LangChain 提供组件与集成，LangGraph 负责把组件编排成复杂、可控、带状态的流程**。LangGraph 可独立用，但通常与 LangChain 组件配合。两者都主要是 Python（也有官方 JS/TS 版本）。

**Go 生态**
- **Eino**：字节 CloudWeGo 团队开源（Apache 2.0，`github.com/cloudwego/eino`，与 Kitex/Hertz 同门）。可理解为"Go 版 LangChain/LangGraph"——组件抽象（ChatModel/Tool/Retriever）+ 编排（Chain 链式 + Graph 图式），面向 Go 强类型与高性能服务场景。出现于 2024 年底，补齐了"Go 缺生产级原生 AI 编排框架"的空白。
- **LangChainGo**（社区，tmc 等维护）：最知名的"LangChain Go 移植版"，出现早；但相比 Python 版功能滞后、更新偏慢，更像社区跟随项目。
- **Genkit (Go)**（Google/Firebase）：2024 年起官方支持 Go，主打可观测、可评估的生产级 flow，和 Gemini / Google Cloud 集成好。

**Go 偏推理运行时（非应用编排框架）**
- **Ollama**：Go 写的本地大模型运行时（封装 llama.cpp），Go 圈最有影响力的 AI 项目之一。
- **LocalAI**：Go 写的本地推理服务，提供 OpenAI 兼容 API。
- **llama.go / GoMLX**：纯 Go 推理 / ML 计算尝试，偏底层、实验性。

**SDK / 轻量客户端（严格说不算框架）**
- **go-openai**（sashabaranov/go-openai）：Go 圈使用量极大的 OpenAI 客户端，很多人直接基于它手搓 Agent。
- 各家官方 Go SDK：Anthropic、Google GenAI 等。

> 现实：Eino 之前，Go 的 AI **应用层**生态远不如 Python——很多 Go 服务直接用 `go-openai` 自己拼，而非用重型框架。Go 在**推理运行时**层（Ollama/LocalAI）一直是主力。

### 第 3 层：Skill / Prompt / Tool 托管与调优平台（横切工程化）

特征：不直接"搭 Agent"，而是给上两层提供 **托管、观测、调优** 的工程基建。

- **Fornax**：主要用于托管 **Skill、Trace 等调优工具**，做 Prompt / Skill 的管理与链路调优（LLMOps 方向）。
- 作用：可观测（trace）、可评估、可迭代——把"能跑"变成"能持续优化、能上生产"。

---

## 二、选型决策建议

| 维度 | 低代码平台 | 代码框架 | 托管/调优平台 |
|---|---|---|---|
| 定制化 | 低 | 高 | —（横切） |
| 交付速度 | 最快 | 中 | —— |
| 适用角色 | 业务/运营 | 工程 | 工程/平台 |
| 典型代表 | 抖音 AI 平台 | LangChain/LangGraph、Eino | Fornax |

**决策顺序**：
1. 先判断**定制需求**：标准化业务 → 低代码层；需要深度逻辑/集成/性能 → 代码框架层。
2. 代码框架层按**语言栈**选：Python 选 LangChain + LangGraph；Go 服务选 Eino（次选 Genkit/LangChainGo）。
3. 无论选哪层，生产化都应叠加**第 3 层**的托管/trace/调优能力（如 Fornax），形成"开发—观测—调优"闭环。

---

## 三、关键结论

- **LangChain 与 LangGraph 是同生态分工**：组件 vs 编排，不是竞争关系。
- **Eino = Go 的生产级 AI 编排框架**，开源（Apache 2.0），定位对标 LangChain/LangGraph，面向高性能 Go 服务。
- **Go AI 应用框架前辈**：LangChainGo（社区移植）、Genkit Go（Google）；运行时层是 Ollama/LocalAI。
- **技术选型要分层**：低代码（快）/ 代码（灵活）/ 托管调优（工程化），三层互补而非互斥。

---

## 原文链接

> 本节仅存于本地笔记，发布产物中不出现。

- 对话沉淀，无外部 URL。
- 相关开源项目：`github.com/cloudwego/eino`、`github.com/tmc/langchaingo`、`github.com/langchain-ai/langchain`、`github.com/langchain-ai/langgraph`、`github.com/ollama/ollama`、Google Genkit（`firebase.google.com/docs/genkit`）。
