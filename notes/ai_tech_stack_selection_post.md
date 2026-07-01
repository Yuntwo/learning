# AI 技术选型 · 发布文案（已脱敏，未发布）

> 配套文档：ai_tech_stack_selection.md
> 配套封面：ai_tech_stack_selection_cover.png
> 记录时间：2026-06-17
> 说明：内部平台名（抖音 AI 平台 / Fornax）已按 learning skill 规则改为中立措辞，仅供对外发布使用。

---

## 小红书

**标题**（≤20字）
AI 应用开发选型，记住分三层就够了

**正文**
```
做 AI 应用，别一上来就纠结"用哪个框架"。先按定制化程度分三层 👇

1️⃣ 低代码平台 —— 快，但定制弱
配置/拖拽搭 Agent，发布链路短，适合业务运营、标准化场景。缺点是复杂逻辑和深度集成受限。

2️⃣ 代码开发框架 —— 灵活、生产级
· Python 最成熟：LangChain 负责组件与集成，LangGraph 负责"图编排"（有状态、可循环、可分支）。两者是分工，不是竞争。
· Go 服务首选 Eino（CloudWeGo 开源，Apache 2.0），强类型 + 图编排，对标 LangGraph；前辈还有 LangChainGo、Google Genkit。
· 运行时层 Go 一直是主力：Ollama、LocalAI。

3️⃣ 托管 / 调优层 —— 横切工程化
Skill 托管 + Trace 观测 + 评估调优，把"能跑"变成"能持续优化、能上生产"。

✅ 决策顺序：先看定制需求 → 再按语言栈选框架 → 生产化叠加调优平台，形成"开发—观测—调优"闭环。

#AI工程 #技术选型 #LangChain #Go #大模型应用
```

**标签**：#AI工程 #技术选型 #LangChain #Go #大模型应用

---

## Twitter / X（≤280 字）

```
AI 应用选型，先分三层再选工具：

1/ 低代码平台：快，定制弱
2/ 代码框架：Python 用 LangChain(组件)+LangGraph(图编排)；Go 用 Eino(CloudWeGo 开源)
3/ 托管调优层：Skill 托管 + Trace，闭环工程化

LangChain 与 LangGraph 是分工，不是竞争。

#AI工程 #LangChain #Golang
```
