# Claude Code Skill 命中调用机制（基于一次 `v1/messages` 抓包的推断）

> 来源：用户提供的抓包分析与上下文片段
> 记录时间：2026-07-22

---

## 核心观点

从单次 `v1/messages` 请求来看，Claude Code 对 skill 的“命中”更像是**客户端或 harness 先收集全量 skill 元数据，再把 skill 名称、description、触发词等内容注入到 system 上下文中，由模型在 prompt 内完成识别与选择**，而不是 API 服务端隐式路由。

---

## 为什么值得看

很多人第一次看到 Claude Code 的 skill 机制，会直觉以为“是不是某个隐藏规则在自动分发”。但抓一次真实请求后会发现，外层机制其实很透明：

- 哪些 skill 可用，往往会直接出现在请求上下文里
- skill 的说明文本本身就是模型决策的重要依据
- 识别 skill 存在、判断是否该用 skill、真正调用 `Skill` tool，可能是三个不同层次的问题

如果想自己做一套类似的 agent skill 体系，这种“先把能力注册表注入上下文，再让模型做选择”的模式很有参考价值。

---

## 关键概念拆解

### 1. Skill 可见性

所谓“模型知道有哪些 skill”，不是因为它天然知道本地目录里装了什么，而是因为**运行时把 skill 列表显式提供给它**。

### 2. Skill 命中

“命中”不是一个单一动作，至少可以拆成三层：

- **发现层**：运行时先知道当前有哪些 skill
- **识别层**：模型根据用户输入和 skill 描述判断某个 skill 是否相关
- **执行层**：真正调用 `Skill` 工具，把控制流交给对应 skill

### 3. Prompt 内检索

在这次案例里，更像是模型在一大段 system prompt 中读取：

- skill 名
- description
- 触发词
- 何时使用

然后基于这些文本做匹配与回答。

### 4. Tool 调用边界

即使模型已经识别到 `learning` skill 存在，也不代表它已经调用了 skill。只有真正出现 `Skill` tool 调用时，才算进入 skill 执行阶段。

---

## 重点对比表

| 维度 | 从这次抓包能看到 | 从这次抓包看不到 | 我的理解 |
|------|------------------|------------------|----------|
| skill 列表是否已知 | 能看到，完整 skills 列表被注入到消息里 | - | 这是最直接的证据 |
| `learning` 是否存在 | 能看到，skills 列表里明确有 `learning` | - | 回答主要来自 prompt 内信息复用 |
| 是否已经执行 skill | 看不到 tool 调用，因此没有执行证据 | 能否真正进入 `Skill` tool | 这次只证明“识别存在”，不能证明“已调用” |
| skills 来源 | 看不到 | 是本地扫描、缓存、还是远程注册表合并 | 很可能由 Claude Code 外层 runtime 预处理 |
| 命中策略 | 只能看到 description/trigger 参与了判断 | 是否还做了额外排序、打分、裁剪 | 大概率是“预处理 + 模型判断”的组合 |
| slash command 机制 | 能看到 `/learning` 被规范为 skill 调用入口 | 内部是否还有更专门分支 | `/learning` 很可能比自然语言触发更直接 |

---

## 要点整理

### 一、最强证据：skills 列表被直接塞进了 system 上下文

抓包里出现了类似下面的结构：

- `The following skills are available for use with the Skill tool:`
- 后面跟着所有 skills 的名称、用途说明、触发词
- 其中明确包含 `learning`

这说明至少在当前会话中，模型并不是临时去文件系统里找 skill，而是直接读取已经注入的“能力目录”。

### 二、assistant 的回答高度复用了 skill 描述

当用户问“有没有 learning 这个 skill”时，assistant 的回答几乎是在复述 skills 列表里的 `learning` 说明：

- 用途
- 默认行为
- 触发词
- 保存路径

这种回答形态非常像：

1. 运行时注入 skill 文本
2. 模型在 prompt 中检索到 `learning`
3. 直接基于该段文字组织回复

### 三、这说明的是“识别机制”，不是“执行机制”

需要区分两件事：

- **知道有这个 skill**
- **真的调用这个 skill**

一次普通问答只需要回答“有/没有”，并不一定要调用 `Skill` tool。只有用户真的输入 `/learning ...`，或任务明显应该交给 `learning` skill 时，才更可能看到真正的 tool use。

### 四、为什么这套机制成立

这种设计有几个明显好处：

- **统一**：skill 不必硬编码在模型里，只要运行时能注册就行
- **可扩展**：新增 skill 的成本主要在 skill 描述与调用链路，而不是改模型
- **可控**：通过 prompt 注入，可以灵活决定当前会话暴露哪些 skill
- **可解释**：抓包后能比较清楚地看到模型为什么会提到某个 skill

### 五、单次抓包的边界也很明确

仅凭一次 `v1/messages` 请求，仍然无法确认：

- skill 是不是每次请求都重新扫描
- 是扫描本地目录，还是读缓存
- 是否对 skills 做了排序和裁剪
- 是否先做触发词预匹配，再决定注入哪些 skill
- `/learning` 这种 slash command 是否走了额外分支逻辑

所以更准确的说法不是“已经完全证明实现方式”，而是：

> 这次抓包**强烈支持**这样一个结论：Claude Code 在请求前把可用 skill 元数据注入 system 上下文，模型再基于这些描述完成识别与选择。

---

## 我的总结

如果只记住一件事，那就是：**从这次抓包看，Claude Code 的 skill 命中机制首先不是“API 暗箱路由”，而是“运行时先把 skill 注册表写进上下文，模型再在 prompt 里做选择”。**

再进一步说，skill 体系的关键不只是“有没有工具”，而是**有没有一份足够清晰的能力描述被喂给模型**。description、触发词、使用边界，其实就是这套系统里的“路由协议”。

---

## 原文链接

> 仅存于本地笔记，发布时绝不带出（本条为纯文本输入，无外部 URL）

- 来源：用户在 `/learning` 中提供的关于 Claude Code skill 命中调用机制的分析文本
- 关键线索：`The following skills are available for use with the Skill tool:`、skills 列表中包含 `learning` 的 description 与触发词、存在 `Skill` tool 但本次问答未实际调用
