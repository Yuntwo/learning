# Claude Code HTTP 请求、上下文编排与流式输出

> 来源：基于一段 Claude Code 到大模型的真实抓包与对响应的逐段分析
> 记录时间：2026-07-22

---

## 核心观点

Claude Code 发给模型的不是“用户的一句话”，而是一份经过 harness 编排的巨大 Messages 请求：其中同时包含系统规则、运行环境、动态能力列表、工具 schema、用户输入，以及推理和上下文管理参数；模型真正消费的是这些信息被服务端序列化与分词后的整体 token 流。

---

## 要点整理

### 1. Claude Code 的请求本质上是“编排后的上下文包”

从真实抓包看，请求不只是简单的：

- `model`
- `system`
- `messages`

而是同时带上了：

- 顶层 `system`
- `messages` 中的 `user` / `system` 消息
- `tools` 及其长描述和输入 schema
- `metadata`
- `thinking`
- `output_config.effort`
- `context_management`
- `stream`

这说明 Claude Code 并不是一个普通聊天框，而是一个带有上下文编排、工具编排和流式代理能力的 agent runtime。

### 2. prompt 不是单一字段，而是分散在多个位置的指令集合

在这个抓包里，真正起提示作用的内容分散在多处：

- 顶层 `system`：定义 Claude Code 身份、安全边界、工具使用规则、memory 规则、运行环境、git 状态等
- `messages` 里的 `role: system`：动态注入当前会话可用的 agent types、skills 等能力说明
- `messages` 里的 `<system-reminder>`：用文本块形式注入当前日期等上下文提醒
- `tools[].description`：告诉模型什么时候该用工具、什么时候不该用、参数如何组织

因此从工程角度理解更准确：

- **prompt**：一组分散在多个字段中的指令内容
- **messages**：显式的消息结构
- **context**：模型本轮可见的全部信息
- **实际模型输入**：这些信息被 Anthropic 服务端进一步拼接、规范化和 tokenization 后的结果

### 3. `messages` 不等于全部上下文

抓包里用户真正输入的内容只有一个很短的 `test`，但最终 `input_tokens` 达到了 32702。

这说明本轮上下文的绝大部分 token 并不来自用户输入，而来自：

- Claude Code 顶层系统提示
- 当前工作目录、平台、git 状态等环境说明
- skills 和 agent types 列表
- 大量工具 schema 与 description
- 额外注入的 system reminders

所以在 agent 系统中，用户输入通常只占上下文的一小部分，框架注入的运行信息才是主要负载。

### 4. 顶层 `system` 与 `messages[].role = system` 是两种不同层次的注入

真实请求同时出现了：

#### 顶层 `system`

用于承载较稳定、较核心的运行规则，例如：

- “You are Claude Code”
- 安全政策
- harness 规则
- memory 写入规则
- 当前环境与仓库信息

#### `messages` 中的 `role: system`

用于承载更动态的系统级信息，例如：

- 当前回合可用的 agent types
- skills 清单
- 与本轮对话相关的能力说明

这说明 Claude Code 不是单一 system prompt 架构，而是会把“稳定系统规则”和“动态系统上下文”分层注入。

### 5. `<system-reminder>` 虽然出现在 user content 里，但语义上不是普通用户输入

抓包里的第一条 `user.content` 实际包含一个 `<system-reminder>` 块，里面放了日期等提示信息。

这代表一件很重要的事：

- API 结构层上，它被放在 `user` 消息里
- 语义层上，它其实是 harness 注入的系统提醒

因此“消息角色”和“语义归属”并不总是一致。工程上经常会把系统性提醒包装进普通消息块中，以便维持统一的消息流水结构。

### 6. tools 不只是 schema，也是 prompt 的一部分

抓包里 `tools` 非常长，每个工具都包含：

- `name`
- `description`
- `input_schema`

这里的 `description` 不是仅供前端显示的文档，而是直接给模型看的行为说明。模型会据此判断：

- 何时使用工具
- 何时不要使用工具
- 参数应该怎么填
- 哪些限制不能违反

所以：

- 对程序来说，`tools` 是可调用能力定义
- 对模型来说，`tools` 也是 prompt 的一部分

这也是 agent 请求体为什么会远大于普通聊天请求的关键原因之一。

### 7. prompt caching 是 Claude Code 必须做的优化

抓包里多个大文本块都带了：

```json
"cache_control": {"type": "ephemeral"}
```

这说明 Claude Code 会把稳定不变的大块前缀显式标记为可缓存内容，例如：

- Claude Code 的稳定系统规则
- 大段环境说明
- 动态能力清单中的稳定部分

这样下一轮请求只需要复用这些 prefix，而不必每次都重新处理整段 3 万多 token 的上下文。对于 agent 产品来说，这种缓存不是锦上添花，而是成本和延迟层面的基础能力。

### 8. `thinking`、`effort` 与 `context_management` 体现了推理控制层

真实请求里还显式设置了：

```json
"thinking": {"type": "adaptive"}
```

```json
"output_config": {"effort": "high"}
```

```json
"context_management": {
  "edits": [{"type": "clear_thinking_20251015", "keep": "all"}]
}
```

这些字段说明 Claude Code 在 runtime 层不仅管理消息和工具，还会直接控制：

- 模型是否启用 adaptive thinking
- 推理强度高低
- 历史 thinking 内容如何清理

这意味着 Claude Code 不是“聊天内容转发器”，而是一个对模型行为有细粒度控制的 orchestration layer。

### 9. 流式输出本质上是 SSE 事件流

抓包里的响应不是一次性 JSON，而是按以下顺序返回的 SSE 事件：

- `message_start`
- `content_block_start`
- 多个 `content_block_delta`
- `content_block_stop`
- `message_delta`
- `message_stop`

其中几个实践点很值得记住：

- 一开始的 `message_start` 里 usage 可能还是 0
- 最终的 `stop_reason` 和 token usage 通常在靠后的 `message_delta` 中给出
- 如果要自己实现客户端，应该在流结束阶段收集完整 usage，而不是只看开始事件

因此流式接口的消费方式应是“事件累计”，而不是“拿第一个响应对象就做完整判断”。

### 10. 当用户输入很弱而框架注入很强时，模型可能给出默认式回应

在这个真实案例里，用户真正输入只有一个短词 `test`，但系统侧注入了大量框架说明。最终模型返回的是一种偏默认、偏接待式的短响应。

这说明：

- 当系统与运行时注入上下文很强时
- 如果用户输入语义非常弱
- 模型会更倾向于把这轮理解成“测试输入”或“空泛输入”

这对设计 agent 系统很重要：如果框架注入过强、而用户请求过弱，就可能出现用户意图被背景上下文“淹没”的现象。

---

## 实践印证

这次分析和当前项目中的适配代码有直接对应关系。

### `ark_adapter.py` 的启发：消息结构映射会压平一部分原始语义

在 `ark_adapter.py:12` 开始的 `convert_req_to_messages` 中，请求里的 `messages` 被转换为 LangChain 的 `HumanMessage`、`SystemMessage`、`AIMessage`、`ToolMessage`。

这段代码说明一个现实问题：

- 上游请求中的复杂 content block
- 在进入下游 agent/runtime 时
- 往往会被重新映射为另一套消息抽象

也就是说，Claude API 的原始消息结构和中间层框架里的消息对象并不总是一一等价。真实生产链路里，消息语义会经历至少一次再封装。

### `handlers.py` 的启发：流式输出最终也是通过事件生成器拼接出来的

在 `handlers.py:21` 开始的 `wrap_chat` 中，服务端通过 `StreamingResponse` 和 `event_generator()` 把 runtime 的结果重新封装为 `text/event-stream`。

尤其是：

- `handlers.py:33` 通过 `react_agent.astream(..., stream_mode='messages')` 获取流式消息
- `handlers.py:38` 调用 `from_astream_model_message` 做中间转换
- `handlers.py:65-66` 在尾部补上 stop 消息和 `[DONE]`

这和抓包里看到的 SSE 事件流本质一致：流式接口不是“模型天然长这样返回”，而是运行时框架把内部消息/事件再封装成外部消费者理解的 SSE 格式。

---

## 值得记住的工程结论

1. **Claude Code 不是普通聊天客户端，而是 agent 编排器**
2. **prompt 是多处内容共同组成的，不要只盯着 user message**
3. **tools 既是能力声明，也是给模型看的行为说明**
4. **messages 只是上下文的一部分，不是全部上下文**
5. **streaming 响应需要按 SSE 事件语义消费**
6. **高质量 agent 体验离不开缓存、thinking 控制和上下文清理**

---

## 原文链接

> 仅存于本地笔记，发布时不带出

- 无外部原文链接；内容来自一段 Claude Code 真实抓包样例与针对该样例的分析
- 项目内关联代码：`/Users/bytedance/go/src/yuntwo/sai_bo_yun_er/ark_adapter.py`
- 项目内关联代码：`/Users/bytedance/go/src/yuntwo/sai_bo_yun_er/handlers.py`
