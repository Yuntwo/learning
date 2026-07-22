# Claude Code 多轮对话上下文机制（messages / system / summarize）

> 来源：用户提供的真实 `v1/messages` 抓包、对话样本与上下文片段
> 记录时间：2026-07-22

---

## 核心观点

Claude Code 的多轮上下文机制，本质上是**客户端 / harness 在每次新请求时，重新组织并发送“当前系统注入 + 会话历史 + 工具轨迹”**。其中：

- **系统级说明**通常放在顶层 `system`
- **多轮对话历史**放在 `messages`
- **用户输入与模型输出不是都塞进 `role=user`**，而是分别出现在 `role=user` 与 `role=assistant`
- **工具调用**会作为 `assistant` 的 `tool_use`
- **工具结果**通常会作为后续 `role=user` 里的 `tool_result` 回传
- 当上下文过长时，**旧内容可能被 summarize**，而不是永远原样全量重放

---

## 为什么值得看

很多人第一次看 Claude Code 抓包时，会把几个层次混在一起：

1. 顶层 `system` 到底是什么
2. `messages` 里是不是只是当前这一轮
3. 为什么工具结果会出现在 `role=user`
4. 长会话是不是会一直把所有历史原样带上

如果这几个点想清楚了，再去看 Claude Code 的 skill 路由、tool use、context management，就会顺很多。你也会更容易理解：

- Claude Code 为什么像“有记忆”
- 这种“记忆”其实是怎么被客户端显式重放出来的
- 长会话为什么有时能看到摘要而不是完整原文

---

## 关键概念拆解

### 1. `system`：运行时注入的全局说明层

`system` 更像“本轮请求的全局规则与背景”，常见内容包括：

- 你是谁（例如 Claude Code）
- harness 规则
- 环境信息
- memory 规则
- 当前工程信息
- skills 列表
- agent types 列表

比如你抓到的这类内容：

- `You are Claude Code, Anthropic's official CLI for Claude.`
- `The following skills are available for use with the Skill tool:`
- 当前工作目录、git status、memory 规则等

这些通常不是普通聊天历史，而是**运行时在请求前注入的系统级上下文**。

### 2. `messages`：结构化的多轮会话历史

`messages` 不是只放当前用户这句话，而是**当前需要模型记住的多轮历史**。

它通常包含：

- 之前的用户消息
- 之前的 assistant 回复
- 中间发生的 `tool_use`
- 工具返回的 `tool_result`
- slash command 注入内容

所以你后面抓到的大请求，看到前面多轮问答、后面 `/learning`、再后面工具链，都是正常现象。

### 3. `role=user` 和 `role=assistant` 是分开的，不是全放到 user 里

一个容易误解的点是：

> 不是“把用户输入和模型输出都放进 `messages` 里的 `role=user` 对象”。

更准确的说法是：

- **用户输入**放在 `role=user`
- **模型输出**放在 `role=assistant`
- **工具结果**很多时候也会以“下一条用户消息中的 `tool_result`”形式回传给模型

也就是说，协议层面上，`tool_result` 虽然看起来跟在 `role=user` 里，但它不是“自然语言用户发言”，而是**客户端代替外部工具把结果喂回模型**。

### 4. `content` 不是一段字符串，而是类型化数组

每条消息通常不是一个纯文本字符串，而是形如：

```json
{
  "role": "assistant",
  "content": [
    { "type": "text", "text": "..." },
    { "type": "tool_use", "id": "...", "name": "Read", "input": {...} }
  ]
}
```

或者：

```json
{
  "role": "user",
  "content": [
    { "type": "text", "text": "继续分析这个请求" },
    { "type": "tool_result", "tool_use_id": "...", "content": "..." }
  ]
}
```

所以 Claude Code 发给模型的不是“聊天记录大字符串”，而是**结构化 transcript**。

### 5. summarize：上下文太长时的压缩机制

从你抓到的系统提示就能看出来，Claude Code 长会话下并不是保证永远原样保留全部历史。常见策略更像：

- 保留最近若干轮原始对话
- 把更早的对话压缩成摘要
- 继续把摘要 + 最近原文一起发送给模型

这意味着：

- **短会话**：你更可能看到很多原始历史都在 `messages` 中
- **长会话**：你更可能看到“摘要 + 最近若干轮原文”的组合

所以“Claude Code 记得前文”，本质上不是服务端永久会话，而是**客户端 / harness 负责上下文重建与压缩**。

---

## 重点对比表

| 维度 | 放在哪里 | 作用 | 我的理解 |
|------|-----------|------|----------|
| 系统规则 | `system` | 告诉模型当前全局约束、环境、skill 列表等 | 更像运行时注入，不是普通聊天历史 |
| 用户自然语言输入 | `messages[].role = user` | 告诉模型用户说了什么 | 普通多轮对话的一部分 |
| 模型文本回复 | `messages[].role = assistant` | 告诉模型自己之前说过什么 | 用于保持对话连续性 |
| 工具调用 | `assistant.content[].type = tool_use` | 表示模型发起了哪个工具调用 | 是 agent 轨迹的一部分 |
| 工具返回 | 常见于 `user.content[].type = tool_result` | 把外部工具结果回喂给模型 | 协议上挂在 user 下，但语义不是“用户自然发言” |
| 超长历史 | 可能先 summarize 再进入后续上下文 | 节省窗口，保留关键信息 | 不保证永远全量原样带上 |

---

## 要点整理

### 一、为什么你现在这份抓包看起来像“多轮全带了”

因为它确实已经是新一轮请求了，客户端在发送时把前面的上下文一起带进来了，包括：

- 你最开始问 `learning` skill 是否存在
- assistant 对该问题的回复
- 你贴出的第一份 `v1/messages`
- assistant 对 skill 机制的分析
- 你发起 `/learning`
- 后续一串 `tool_use` / `tool_result`

所以这不是 API 自动替你补全了会话，而是**Claude Code 客户端显式重发了当前认为重要的历史**。

### 二、一个关键纠偏：不是“都放进 user 对象里”

这个点一定要区分：

- 用户说的话：在 `role=user`
- assistant 说的话：在 `role=assistant`
- tool result：协议上通常作为 `user` 的一部分回传

所以如果从抓包里看到 `tool_result` 出现在 `role=user`，不要误以为“模型输出也全放到了 user 对象里”。

更准确是：

> **自然语言回复属于 `assistant`，工具结果属于后续回灌给模型的输入，因此经常挂在 `user` 这一侧。**

### 三、为什么 `tool_result` 经常出现在 `role=user`

因为从模型角度看，工具不是模型自己内部完成的，而是“模型提出调用请求后，外部系统执行，再把结果返回给模型”。

所以一次完整回路通常长这样：

1. assistant 输出 `tool_use`
2. 客户端执行工具
3. 客户端把结果作为下一条 `role=user` 中的 `tool_result` 喂回来
4. assistant 基于这个结果继续推理 / 回复

这是一种很典型的 agent loop 协议设计。

### 四、长会话不是无限原样累积，而是会做 summarize

你抓到的系统说明已经给出方向：

- 会话变长时，当前上下文可能被总结
- 总结与剩余原文一起进入下一窗口

所以 Claude Code 的行为更像：

```text
短会话：system + 原始多轮 messages
长会话：system + 历史摘要 + 最近若干轮原始 messages
```

这也解释了两个现象：

- 有时你能在抓包里看到很多前文原样出现
- 有时你只会看到“之前发生了什么”的压缩总结，而不是最早的完整原文

### 五、slash command 也会被编码进 messages

你贴出来的 `/learning` 请求里就有：

```text
<command-message>learning</command-message>
<command-name>/learning</command-name>
<command-args>...</command-args>
```

这说明 slash command 不一定是“客户端私下处理后消失”，很多情况下也会作为**结构化命令上下文的一部分进入会话历史**。

### 六、一个最小 mental model

可以把 Claude Code 的一次请求想成三层：

```text
[system]
- 身份 / harness / 环境 / skills / memory / agent types

[messages]
- 多轮 user / assistant 历史
- tool_use / tool_result
- slash command 注入内容

[context management]
- 太长时，总结旧历史，保留最近关键原文
```

这样再看你抓到的 JSON，就不会乱。

---

## Demo 1：最小多轮纯对话示例

```json
{
  "system": [
    { "type": "text", "text": "You are Claude Code." }
  ],
  "messages": [
    {
      "role": "user",
      "content": [
        { "type": "text", "text": "帮我解释这个错误" }
      ]
    },
    {
      "role": "assistant",
      "content": [
        { "type": "text", "text": "先把报错贴出来，我来分析。" }
      ]
    },
    {
      "role": "user",
      "content": [
        { "type": "text", "text": "报错是 connection refused" }
      ]
    }
  ]
}
```

这里很清楚：

- 用户输入在 `user`
- 模型输出在 `assistant`
- 是多轮历史，不是只有当前一句

---

## Demo 2：带工具调用的示例

```json
{
  "messages": [
    {
      "role": "user",
      "content": [
        { "type": "text", "text": "看一下 README 里写了什么" }
      ]
    },
    {
      "role": "assistant",
      "content": [
        {
          "type": "tool_use",
          "id": "call_1",
          "name": "Read",
          "input": { "file_path": "/repo/README.md" }
        }
      ]
    },
    {
      "role": "user",
      "content": [
        {
          "type": "tool_result",
          "tool_use_id": "call_1",
          "content": "1\t# Project Title\n2\tThis repo ..."
        }
      ]
    },
    {
      "role": "assistant",
      "content": [
        { "type": "text", "text": "README 主要介绍了项目用途和启动方式。" }
      ]
    }
  ]
}
```

这个例子最能说明：

- tool call 是 assistant 发起的
- tool result 作为 user 侧消息回灌
- 最终 assistant 再继续输出文本

---

## Demo 3：长会话 summarize 的概念示例

真实实现不一定长这样，但思路大致类似：

```json
{
  "system": [
    { "type": "text", "text": "You are Claude Code." }
  ],
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": "[历史摘要] 之前用户让我排查 skill 命中机制，我已经分析了 system 注入、messages 多轮结构和 tool_result 回灌方式。"
        }
      ]
    },
    {
      "role": "user",
      "content": [
        { "type": "text", "text": "继续解释 summarize 是怎么影响上下文的" }
      ]
    }
  ]
}
```

重点不是摘要一定长这样，而是：

- **旧对话可能不再逐轮原样出现**
- **会被压缩成关键信息后继续传给模型**

---

## 我的总结

如果只记住一件事，那就是：**Claude Code 的“多轮记忆”不是服务端替你永久保存并自动恢复，而是客户端 / harness 在每次请求时，把 system 注入、messages 历史、工具轨迹和必要摘要一起重新组织后再发给模型。**

再进一步说，最容易误解的点有两个：

1. **不是所有内容都塞进 `role=user`**，assistant 回复仍然在 `role=assistant`
2. **`tool_result` 虽常挂在 user 侧，但它代表的是外部工具回灌，不等于普通用户自然发言**

所以看 Claude Code 抓包时，最稳的阅读方法是：

> 先看 `system`，再看 `messages` 的多轮结构，最后看有没有 summarize 或 tool trace 介入。

---

## 原文链接

> 仅存于本地笔记，发布时绝不带出（本条为纯文本输入，无外部 URL）

- 来源：用户在 `/learning` 中提供的关于 Claude Code 多轮对话上下文机制的分析需求
- 关键线索：`messages` 中包含多轮 user / assistant / tool_use / tool_result；`system` 注入技能与环境；上下文变长时可能 summarize
