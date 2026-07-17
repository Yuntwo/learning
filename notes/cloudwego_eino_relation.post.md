# X 文案
做 AI Agent 项目，别只问“选哪个框架”，要先拆成两层：服务层 + AI 编排层。

如果是 Go，我现在更推荐：CloudWeGo + Eino。
- CloudWeGo 负责 HTTP / RPC / 工程化 / 服务治理
- Eino 负责模型调用、Agent、Workflow、Tool Calling、RAG

如果是 Python，常见组合是：FastAPI + LangGraph / LangChain。
- FastAPI 负责服务入口
- LangGraph / LangChain 负责 Agent 与多步骤编排

本质上不是单框架选择，而是分层组合。#AIAgent #Go #Python #LLM

# 小红书标题
AI Agent项目怎么选型

# 小红书正文
做 AI Agent 项目时，我觉得最重要的一件事，是别把“服务框架”和“AI 框架”混在一起选。
更实用的方式，是先拆成两层：服务层 + AI 编排层。

如果是 Go 技术栈，我会优先考虑 CloudWeGo + Eino：
CloudWeGo 负责服务基础设施，比如 HTTP、RPC、网络性能、工程化接入；Eino 负责 AI 编排，比如模型调用、Agent、Workflow、Tool Calling、RAG。
这个组合很适合已经在做后端服务化、希望把 AI 能力稳定接到线上系统里的团队。

如果是 Python 技术栈，常见组合是 FastAPI + LangGraph / LangChain：
FastAPI 负责 API 服务入口；LangGraph / LangChain 负责大模型能力接入、状态流转、多步骤 Agent 编排和工具调用。
这个组合更适合 AI 原型验证、快速迭代和生态复用。

所以技术选型不要只问“Go 还是 Python”，而要问：
1）服务层谁来做？
2）AI 编排层谁来做？
3）这两层之间如何组合？

一句话总结：
Go 方案优先看 CloudWeGo + Eino；Python 方案优先看 FastAPI + LangGraph / LangChain。

# 小红书标签
#AIAgent #Go语言 #Python开发 #LLM应用 #技术选型
