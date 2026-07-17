# MCP、RPC 与 Eino ToolInfo 的关系

> 来源：基于一次关于 MCP / RPC / Eino ToolInfo 的问答整理
> 记录时间：2026-07-17

---

## 核心观点

MCP 本质上也是一种应用层协议，但它不是泛化 RPC 的简单别名，而是**面向大模型 / Agent 场景，对外部能力进行标准化暴露的一套协议约定**。可以把它理解为：**RPC 风格的调用机制 + 面向模型的能力语义层**。

---

## 要点整理

### 1. MCP 和 RPC 的相同点：本质上都在做“远程调用”

如果只看最底层交互模型，MCP 和 RPC 很像：

- 都有 client / server 两端
- 都有 method、params、result、error
- 都可以跑在 stdio / HTTP 等传输层之上
- 都属于应用层协议，核心目标都是“调用远端能力”

所以从抽象层看，说 **MCP 和 RPC 没有本质鸿沟** 是成立的。

### 2. MCP 和 RPC 的关键差异：面向对象不同

RPC 更关注：

- 怎么调用某个远端函数 / 服务
- 怎么序列化参数和结果
- 怎么做服务治理、性能、超时、重试

MCP 更关注：

- 怎么把外部能力以模型可理解的方式暴露出来
- 怎么让模型发现有哪些 tool / resource / prompt
- 怎么让模型根据描述和 schema 决定“该不该调用”

一句话说：

- **RPC 主要服务程序员和业务系统**
- **MCP 主要服务模型运行时和 Agent 框架**

### 3. MCP 比 RPC 多出来的是“面向模型的语义标准化”

MCP 不只是“调一个方法”，它额外标准化了模型使用外部能力时最关键的几类对象：

#### Tool
让模型以结构化方式调用外部能力。
常见字段包括：

- `name`
- `description`
- `inputSchema`

这里的 `description` 不只是文档，而是给模型做决策的提示：

- 这个工具是干嘛的
- 什么场景该调用
- 参数分别代表什么

#### Resource
让模型读取上下文资源，而不只是执行动作。

#### Prompt
让模型复用一段标准化提示模板，而不是每次从头拼装。

所以 MCP 标准化的不只是“远程函数调用”，而是**模型工作台里的能力接口面**。

### 4. 更合适的理解方式：MCP 像 AI 世界里的 LSP

一个很好的类比是：

- JSON-RPC / RPC：提供通用调用壳子
- LSP：在这个壳子上标准化“编辑器如何调用语言服务”
- MCP：在这个壳子上标准化“模型如何调用工具、读取资源、使用提示”

所以更准确的表述不是：

> MCP 发明了新的远程调用原理

而是：

> MCP 在 RPC 风格交互之上，为模型场景定义了统一能力协议

### 5. “MCP 是 agent 之间的协议”这个说法为什么不够准确

这么说不完全错，但不够精确。

更准确地说，MCP 主要是：

> **模型 / Agent 与外部能力提供方之间的协议**

这个能力提供方可以是：

- 工具服务
- 文件系统适配层
- 数据库查询层
- 第三方平台能力
- 甚至另一个 Agent

所以：

- 如果 Agent A 把自己的能力暴露成 MCP server
- Agent B 再通过 MCP 调用它

看上去像是“agent 之间通信”，但本质上仍然是：

> 一个 agent 以 tool / resource provider 的身份暴露能力

它不是专门为“多 agent 协作任务编排”设计的协议。真正完整的 agent-to-agent 协议通常还要定义：

- 任务委托
- 生命周期
- 状态同步
- 权限边界
- 协作和回传语义

这些并不是 MCP 的核心职责。

### 6. Eino 的 `schema.ToolInfo` 属于什么层

Eino 的 `schema.ToolInfo` 不是 MCP 原生协议对象，而是 **Eino 框架内部统一的 tool 描述结构**。

它主要表达：

- 工具名
- 工具描述
- 参数 schema

参数描述本质上会收敛到 **JSON Schema 语义**，这样模型侧就能把它当作 function calling / tool calling 的工具定义使用。

也就是说：

- **`schema.ToolInfo` 是 Eino 内部抽象**
- **MCP 是跨进程 / 跨服务标准协议**
- 二者之间通常需要一层映射或桥接

### 7. 一个实用的三层模型

理解这类系统时，可以拆成三层：

#### 第 1 层：通信层
类似 RPC / JSON-RPC / HTTP，负责请求和响应。

#### 第 2 层：语义层
定义 tool、resource、prompt、schema 等能力对象。

#### 第 3 层：运行时层
由 Agent / LLM runtime 决定：

- 什么时候发现工具
- 什么时候选工具
- 如何编排多轮调用
- 如何处理失败、重试和上下文

MCP 主要覆盖的是 **第 2 层（并部分约束第 1 层）**。

---

## 实践印证

结合当前项目，可以看到三层关系非常清楚：

### 1. 本地工具：Eino Tool 抽象

`agent/mcp/get_bearer_token_mcp.go` 里的 `GetBearerTokenMCPTool` 实现的是 Eino 的本地工具接口：

- `Info(ctx) (*schema.ToolInfo, error)`
- `InvokableRun(ctx, argumentsInJSON string, opts ...tool.Option) (string, error)`

这里返回的 `schema.ToolInfo` 表示：

- `Name = get_bearer_token`
- `Desc = 返回当前服务配置的 BearerToken`
- `ParamsOneOf = 空参数 schema`

这说明它本质是 **Eino 可识别的本地 tool 定义**，而不是 MCP 原生协议报文。

### 2. 桥接层：把 MCP tool 转成 Eino tool

`agent/mcp/eino_tool.go` 里分两类工具：

- 一类是通过 `convertMCPToolToEinoTool(...)` 把外部 MCP 工具转换成 Eino 工具
- 另一类是本地手写的 `candidateTools`，例如 `NewGetBearerTokenMCPTool()`

这正好说明：

- **MCP tool** 和 **Eino tool** 不是同一个对象
- 项目里是通过桥接层把两者统一到 Eino 的工具体系里

### 3. 命名上的“MCPTool”不等于原生 MCP 协议对象

像 `GetBearerTokenMCPTool` 这种命名，更像是：

- 这个工具最终会参与 Agent 的工具体系
- 语义上属于“给 Agent 使用的能力”
- 但实现上仍然是本地 Eino Tool

因此命名上带 `MCPTool`，不等于它已经是一个独立暴露在 MCP server 上的原生 MCP 工具。

---

## 最容易记住的一句话

> **MCP 可以理解成“面向模型场景的标准化能力协议”；它和 RPC 在调用机制上相近，但比 RPC 多了一层给模型用的 tool / resource / prompt 语义。**

---

## 延伸理解

如果要继续深挖，可以再顺着这几个方向理解：

1. **Eino Tool / MCP / Function Calling 的映射关系**
   - Eino 内部抽象如何映射到模型侧 tool schema
2. **MCP 与 CLI 的关系**
   - CLI 是人的工具入口，MCP 是模型的工具入口
3. **为什么 Tool 描述质量会直接影响 Agent 行为**
   - 参数描述不清时，模型会猜测字段含义
4. **为什么 Agent 系统常常需要“协议层 + 编排层 + 验证层”**
   - 协议只解决能力暴露，不解决完整协作闭环
