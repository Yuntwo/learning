# Anthropic Messages 和 OpenAI 协议到底差在哪

> 来源：基于与 Claude 的对话整理，并结合当前仓库适配代码观察
> 记录时间：2026-07-22

---

## 核心观点

Anthropic Messages API 和 OpenAI Chat / Responses API 本质上都在解决“把对话、系统提示、工具调用和流式输出发给模型并拿回结果”这个问题，但它们不是同一套协议：概念相近，字段设计、消息编排、工具调用语义和流式事件格式都不一样，所以做兼容层时通常需要显式做协议转换，不能直接透传。

---

## 为什么这个问题容易混

很多人第一次接触时会觉得它们“看起来差不多”：

- 都有 `model`
- 都支持多轮对话
- 都有 `system` / 系统提示的概念
- 都支持工具调用
- 都支持 streaming

但真正落到网关、中转层、SDK 封装或者前端流式解析时，差异就会立刻暴露出来。最常见的误区是：

1. 以为只要把 `model` 名字换掉就能兼容
2. 以为 `messages` 数组格式天然通用
3. 以为工具调用只是字段名不同
4. 以为 SSE 流式事件可以原样转发

实际上，上面四点都不成立。

---

## 先看入口：两种协议常见 path 长什么样

很多时候你甚至不用先看请求体，光看 URL 就能先猜个大概。

| 协议风格 | 常见 path | 说明 |
|----------|-----------|------|
| Anthropic Messages | `/v1/messages` | Anthropic 原生 Messages API 最常见入口 |
| OpenAI Chat Completions | `/v1/chat/completions` | 最常见的 OpenAI 兼容入口，注意是 **completions 复数** |
| OpenAI Responses | `/v1/responses` | OpenAI 新一代统一接口风格 |

这里有两个特别容易混淆的点：

1. 很多人会口误成 `/v1/chat/completion`，但常见兼容接口其实是 **`/v1/chat/completions`**。
2. 如果一个服务对外暴露的是 `/chat/completions`，并且返回 `chat.completion.chunk`、`choices[].delta` 这一类结构，那它通常更像 **OpenAI 风格协议**，即使底层模型并不一定是 OpenAI。

---

## 一张表先看懂：核心差异总览

| 维度 | Anthropic Messages API | OpenAI Chat / Responses API | 实际影响 |
|------|------------------------|-----------------------------|----------|
| 常见 path | `/v1/messages` | `/v1/chat/completions`、`/v1/responses` | 看 URL 往往就能先判断协议家族 |
| 系统提示位置 | 顶层 `system` 字段 | Chat 常放在 `messages[0]` 的 `system`；Responses 常用 `instructions` | 做请求转换时要重组系统提示 |
| 用户输入入口 | `messages` | Chat 用 `messages`，Responses 常用 `input` | 同一套业务抽象要适配两种入口 |
| `content` 形态 | 更偏向 block 数组 | Chat 常见字符串；Responses/多模态也有结构化输入 | 多模态和工具场景转换更明显 |
| 工具定义 | `tools` + `input_schema` | `tools` + `function.parameters` | JSON Schema 包裹层级不同 |
| 工具调用返回 | `tool_use` block | `tool_calls` | 解析逻辑不能共用 |
| 工具结果回传 | `tool_result` block | 追加 `role: tool` 消息 | 对话编排方式不同 |
| 停止原因字段 | `stop_reason` | `finish_reason` 或 Responses 事件状态 | SDK 兼容层要映射字段 |
| token 统计 | `input_tokens` / `output_tokens` | `prompt_tokens` / `completion_tokens` | 计费和监控字段要统一映射 |
| 流式返回 | 事件粒度更细，含 message / content block 生命周期 | 常见是 delta 增量块或 Responses event stream | 前端流式解析通常不能直接复用 |
| 协议关系 | 与 OpenAI 是“同类接口” | 与 Anthropic 是“同类接口” | 相似但不兼容 |

---

## 1. 最大的直观差异：system 放哪

### Anthropic Messages

Anthropic 会把系统提示放在顶层：

```json
{
  "model": "claude-sonnet-4-5",
  "max_tokens": 1024,
  "system": "You are a helpful assistant.",
  "messages": [
    { "role": "user", "content": "你好" }
  ]
}
```

### OpenAI Chat Completions

OpenAI Chat 通常把系统提示塞进 `messages` 里：

```json
{
  "model": "gpt-4o",
  "messages": [
    { "role": "system", "content": "You are a helpful assistant." },
    { "role": "user", "content": "你好" }
  ]
}
```

### OpenAI Responses

Responses API 又进一步把系统级说明抽成了 `instructions`：

```json
{
  "model": "gpt-4.1",
  "instructions": "You are a helpful assistant.",
  "input": "你好"
}
```

### 这一点为什么重要

如果你在做：

- API 网关转发
- 模型供应商切换
- 统一 SDK
- 兼容 OpenAI / Anthropic 的代理服务

那你第一步通常就是重组 system prompt。这个差异虽然看起来小，但它决定了你不能只做字段拷贝。

---

## 2. content 结构不完全一样

### OpenAI 的常见直觉：字符串内容

很多开发者熟悉的是这种写法：

```json
{ "role": "user", "content": "你好" }
```

### Anthropic 的原生思路：内容块

Anthropic 更强调 content block：

```json
{
  "role": "assistant",
  "content": [
    { "type": "text", "text": "你好！" }
  ]
}
```

这意味着：

- 简单文本场景下，两者都能“看起来差不多”
- 一旦进入多模态、工具调用、结构化输出，Anthropic 的 block 语义会更明显
- 兼容层最好尽早把内部抽象建成“内容块”而不是“纯字符串”

### 我的理解

如果你只做最简单的文本问答，用字符串思维没问题；但如果你做的是长期演进的网关或者 agent 框架，最好直接用统一的 block 抽象，否则后面会越补越乱。

---

## 3. 工具调用：看起来类似，其实编排方式差很多

这是第二个最容易踩坑的地方。

### Anthropic 的工具定义

```json
{
  "tools": [
    {
      "name": "get_weather",
      "description": "Get weather",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": { "type": "string" }
        },
        "required": ["city"]
      }
    }
  ]
}
```

模型返回时，常见的是 `tool_use`：

```json
{
  "content": [
    {
      "type": "tool_use",
      "id": "toolu_123",
      "name": "get_weather",
      "input": {
        "city": "北京"
      }
    }
  ]
}
```

工具执行结果再通过 `tool_result` 回给模型。

### OpenAI 的工具定义

```json
{
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather",
        "parameters": {
          "type": "object",
          "properties": {
            "city": { "type": "string" }
          },
          "required": ["city"]
        }
      }
    }
  ]
}
```

模型返回常见的是：

```json
{
  "tool_calls": [
    {
      "id": "call_123",
      "type": "function",
      "function": {
        "name": "get_weather",
        "arguments": "{\"city\":\"北京\"}"
      }
    }
  ]
}
```

### 最关键的不同

| 维度 | Anthropic | OpenAI |
|------|-----------|--------|
| 工具调用载体 | `tool_use` 内容块 | `tool_calls` 数组 |
| 参数形态 | 常是结构化对象 `input` | 常是 JSON 字符串 `arguments` |
| 工具结果回传 | `tool_result` | `role: tool` 消息 |
| 消息组织方式 | block 驱动 | role/message 驱动 |

### 实践建议

如果你在做中转层：

- 不要把 OpenAI 的 `arguments` 直接当对象用，先 parse
- 不要假设所有供应商都会把工具参数作为 JSON 字符串返回
- 最好内部统一成：`toolName + structuredArgs + toolCallId + toolResult`

这样你在对接不同厂商时，转换层会更薄。

---

## 4. 多轮对话里的消息编排也不同

从抽象层看，两家都支持“模型请求工具 → 业务执行工具 → 把结果喂回模型 → 模型继续说话”。

但组织方式不同：

### Anthropic 更像

1. assistant 产生 `tool_use`
2. client 执行工具
3. client 把 `tool_result` 放回消息流
4. model 再继续生成

### OpenAI 更像

1. assistant 返回 `tool_calls`
2. client 执行工具
3. client 追加 `role: tool` 消息
4. model 再继续生成

这就是为什么很多“OpenAI 兼容层”如果要支持 Anthropic，不能只改 URL 和鉴权头，消息组装器通常也得重写一段。

---

## 5. 参数命名不同，不要想当然映射

### Anthropic 常见参数

- `model`
- `messages`
- `system`
- `max_tokens`
- `temperature`
- `top_p`
- `stop_sequences`

### OpenAI 常见参数

- `model`
- `messages` 或 `input`
- `temperature`
- `top_p`
- `stop`
- 不同接口下输出 token 控制字段略有差异

其中最容易忽略的是：

| 语义 | Anthropic | OpenAI |
|------|-----------|--------|
| 停止序列 | `stop_sequences` | `stop` |
| 停止原因 | `stop_reason` | `finish_reason` |
| 输入 token | `input_tokens` | `prompt_tokens` |
| 输出 token | `output_tokens` | `completion_tokens` |

如果你们团队有统一的埋点、监控、计费报表，这一层一定要做标准化，不然后面统计口径会乱。

---

## 6. Streaming：最像，但也最不能直接通用

两家都支持流式返回，但事件模型不一样。

### Anthropic 常见事件风格

你会看到类似：

- `message_start`
- `content_block_start`
- `content_block_delta`
- `content_block_stop`
- `message_delta`
- `message_stop`

这说明它的流不只是“吐文本”，而是把整条消息和内容块的生命周期都拆开了。

### OpenAI 常见事件风格

更常见的是：

- Chat Completions 的 `choices[].delta`
- Responses API 的事件流对象

整体更偏“增量片段 append”。

### 这对前端或代理层意味着什么

- Anthropic 适合按事件类型驱动状态机
- OpenAI 更常见的是 delta 拼接
- 如果你要做统一的流式 UI，最好内部抽象成：
  - 文本增量
  - 工具调用开始
  - 工具参数增量
  - 消息结束
  - 错误 / 中断

也就是说，**内部统一抽象应该高于供应商原始事件格式**。

---

## 7. 返回结果结构对照

### Anthropic 风格

```json
{
  "id": "msg_xxx",
  "type": "message",
  "role": "assistant",
  "content": [
    { "type": "text", "text": "你好！" }
  ],
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 10,
    "output_tokens": 20
  }
}
```

### OpenAI Chat 风格

```json
{
  "id": "chatcmpl-xxx",
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "你好！"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 20,
    "total_tokens": 30
  }
}
```

### 一眼记忆法

| 你想找的东西 | Anthropic 常在哪 | OpenAI 常在哪 |
|--------------|------------------|---------------|
| 模型正文 | `content[]` | `choices[0].message.content` |
| 工具调用 | `content[]` 里的 `tool_use` | `tool_calls` |
| 停止原因 | `stop_reason` | `finish_reason` |
| token 用量 | `usage.input_tokens` / `usage.output_tokens` | `usage.prompt_tokens` / `usage.completion_tokens` |

---

## 8. 如果你在做兼容层，最实用的字段映射思路

与其死记每家的字段，不如把内部抽象先定好。比如内部统一成：

- `systemPrompt`
- `messages[]`
- `contentBlocks[]`
- `tools[]`
- `toolCalls[]`
- `toolResults[]`
- `stopReason`
- `usage.input`
- `usage.output`
- `streamEvents[]`

然后做双向映射：

| 内部抽象 | Anthropic 来源 | OpenAI 来源 |
|----------|----------------|-------------|
| `systemPrompt` | 顶层 `system` | `messages[].role=system` 或 `instructions` |
| `userInput` | `messages` | `messages` / `input` |
| `contentBlocks` | 原生 `content[]` | 由字符串、parts、delta 归一化而来 |
| `toolCall.args` | `input` | parse `function.arguments` |
| `toolResult` | `tool_result` | `role: tool` |
| `stopReason` | `stop_reason` | `finish_reason` |
| `usage.input` | `input_tokens` | `prompt_tokens` |
| `usage.output` | `output_tokens` | `completion_tokens` |

这套方法的核心好处是：你的业务层不需要知道底层到底连的是哪一家。

---

## 9. 用当前仓库看一眼：这份适配器更像哪种协议

当前仓库里这套对外接口，明显更接近 **OpenAI Chat Completions 风格**，而不是 Anthropic 原生 Messages。

### 证据 1：对外 path 长得像 Chat Completions

在 `handlers.py:85`，服务注册的是：

```python
app.post("/api/v3/bots/chat/completions")
```

这不是 Anthropic 风格的 `/v1/messages`，而是明显贴近 OpenAI 家族的 `/chat/completions` 命名。

### 证据 2：输入读取的是 `messages`

`ark_adapter.py:14` 里直接遍历 `req['messages']`，再按 `role` 转成 LangChain 消息对象。这种输入形态也更贴近 Chat Completions 的直觉。

### 证据 3：流式输出对象名是 `chat.completion.chunk`

`ark_adapter.py:58` 把 chunk 的对象类型固定成：

```python
object: Literal["chat.completion.chunk"] = "chat.completion.chunk"
```

而 `handlers.py:65-66` 最终也是按 SSE 方式输出 `data: ...` 和 `data:[DONE]`。这整套组合都在说明：

- **对外协议层**：更像 OpenAI Chat Completions
- **内部执行层**：再转成 LangChain / agent 消息流
- **不是 Anthropic 原生 Messages wire format**

### 我的理解

这也是现实工程里很常见的一种形态：底层模型可能并不只接一种供应商，但为了兼容前端、网关或现有 SDK，对外通常会先固定成一套更常见的协议外观。

### 再拆细一点：这份代码到底做了哪三层转换

如果把这份适配器按数据流拆开看，其实非常适合拿来理解“协议外观”和“内部执行模型”是怎么解耦的。

#### 第 1 层：HTTP 请求外观保持 OpenAI 风格

入口在 `handlers.py:85`：

```python
app.post("/api/v3/bots/chat/completions")
```

这说明对调用方来说，最外层看到的是一个 Chat Completions 风格接口，而不是 Anthropic 原生的 `/v1/messages`。

#### 第 2 层：把 OpenAI 风格 `messages` 转成 LangChain 内部消息

真正做协议拆解的是 `convert_req_to_messages()`，在 `ark_adapter.py:12-33`。

它做的事情可以概括成：

| 输入 role | 转成的内部对象 | 说明 |
|-----------|----------------|------|
| `user` | `HumanMessage` | 普通用户输入 |
| `system` | `SystemMessage` | 系统提示 |
| `assistant` | `AIMessage` | 历史助手消息 |
| `tool` | `ToolMessage` | 工具返回结果 |

也就是说，这一步其实在做的是：

- **协议层 role/message 结构**
- → **框架层统一消息对象**

对应代码是：

```python
if role == "user":
    new_messages.append(HumanMessage(content=content))
elif role == "system":
    new_messages.append(SystemMessage(content=content))
elif role == "assistant":
    new_messages.append(AIMessage(content=content))
elif role == "tool":
    new_messages.append(ToolMessage(content=content, tool_call_id=str(tool_call_id)))
```

这段代码特别能说明一个事实：**很多兼容层真正做的不是“转模型”，而是“转消息语义”。**

#### 第 3 层：把内部消息再包装回 Chat Completions 流式输出

模型执行之后，`handlers.py:32-66` 会根据是否 `stream` 走 `astream()` 或 `ainvoke()`；而真正负责把内部消息重新包装成对外 chunk 的，是 `ark_adapter.py:72-182`。

最明显的两个信号是：

1. `ark_adapter.py:58` 把对象类型写死成：

```python
object: Literal["chat.completion.chunk"] = "chat.completion.chunk"
```

2. `handlers.py:65-66` 结尾输出：

```python
yield f"data:{end_stop_message().model_dump_json(...)}\r\n\r\n"
yield "data:[DONE]\r\n\r\n"
```

这就是非常典型的 OpenAI Chat Completions SSE 外观：

- 中间不断吐 `chat.completion.chunk`
- 最后补一个 `[DONE]`

#### 一个最值得记住的工程判断

所以这份代码不是在“实现 Anthropic 协议”，而是在：

1. **接收 OpenAI 风格输入**
2. **转成 LangChain 内部消息模型**
3. **执行 agent / model**
4. **再转回 OpenAI 风格流式输出**

如果未来要同时支持 Anthropic 原生 `/v1/messages`，通常就不是简单改一个 path，而是至少还要再补一层：

- Anthropic request → 内部消息对象
- 内部消息对象 → Anthropic response / SSE event

也正因为如此，真正稳的做法从来不是把某一家字段写死在业务里，而是尽早收敛成内部统一抽象。

---

## 10. 到底该怎么一句话区分它们

如果只想记最核心的 5 点，我觉得记这五句就够了：

1. **Anthropic 的常见 path 是 `/v1/messages`，OpenAI 常见 path 是 `/v1/chat/completions` 或 `/v1/responses`。**
2. **Anthropic 的 system 在顶层，OpenAI 常在 messages 或 instructions 里。**
3. **Anthropic 更强调 content block，OpenAI 更常见字符串 / delta 思维。**
4. **Anthropic 用 `tool_use` / `tool_result`，OpenAI 用 `tool_calls` / `role: tool`。**
5. **两者是同类接口，不是同协议；做兼容必须做转换。**

---

## 一个最小的判断标准

当你看到一个接口时，可以快速问自己：

| 判断问题 | 如果答案是这样，更像 Anthropic | 如果答案是这样，更像 OpenAI |
|----------|-------------------------------|------------------------------|
| path 长什么样？ | `/v1/messages` | `/v1/chat/completions` / `/v1/responses` |
| system 在哪？ | 顶层字段 | `messages` 或 `instructions` |
| 工具调用长什么样？ | `tool_use` | `tool_calls` |
| 工具结果怎么回？ | `tool_result` | `role: tool` |
| 流式事件像什么？ | block 生命周期事件 | delta 增量片段 |

如果这五项里有三项以上对上，基本就能判断是哪一类协议风格。

---

## 最佳实践 / 注意事项

### 对后端 / 网关

- 不要做“看起来差不多”的硬映射，先统一内部抽象
- path 兼容、消息兼容、streaming 兼容要拆开看，不要混为一谈
- 流式协议一定单独适配，不要幻想原样透传
- 工具参数要做显式 parse 和 schema 校验
- 监控埋点要统一 usage 字段命名

### 对前端 / 客户端

- 不要把 streaming 简化成“每次来一段文本”
- 最好抽象成事件驱动状态机
- 对工具调用、文本增量、结束信号分开处理
- 提前考虑多供应商兼容，不要把某家的事件名写死到 UI 逻辑里

### 对 SDK / Agent 框架

- 尽量使用高层语义对象，不要让业务侧直接操作供应商原始字段
- 把系统提示、工具调用、结构化输出、流式事件都收敛到统一模型
- 做好“简单文本模式”和“高级工具模式”的分层

---

## 我的结论

Anthropic Messages API 和 OpenAI 协议的关系，不是“谁替代谁”，而是“它们都在描述同一类能力，但各自有不同的话语体系”。开发时最容易犯的错，不是记不住字段名，而是误把“概念相似”当成“协议兼容”。

所以更准确的理解应该是：

- **Anthropic Messages ≈ OpenAI Chat / Responses 的同类产品形态**
- **但两者不是一套 wire protocol**
- **要兼容，必须做 path、字段、消息编排、工具调用和 streaming 语义的转换**

---

## 原文链接

> 本文来自与 Claude 的问答整理；并结合当前仓库 `handlers.py`、`ark_adapter.py` 的协议适配代码观察，无外部原文链接。
