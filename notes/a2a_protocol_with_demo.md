# A2A 协议，最好带着 Demo 一起理解

> 来源：A2A Protocol 官方教程、CloudWeGo Eino A2A 示例、当前仓库代码观察
> 记录时间：2026-07-22

---

## 核心观点

A2A（Agent-to-Agent）协议解决的不是“模型怎么推理”，而是“一个 Agent 如何发现、调用、追踪另一个 Agent 的能力”。它通常跑在 HTTP(S) 之上，消息层采用 JSON-RPC 2.0，流式更新常见通过 SSE 承载。所以它看起来像“Agent 时代的 HTTP”，但更准确地说，它是**构建在 HTTP 之上的 Agent 通信协议**，而不是新的传输层。

如果只记住一件事：

> **A2A 不是把 HTTP 替换掉，而是给 HTTP 定义了一套 Agent 之间互相说话的标准格式。**

---

## 为什么值得看

A2A 值得专门学一下，原因有三个：

1. **它解决的是互操作问题**
   - 一个 Agent 用什么框架写并不重要，重要的是别人能不能发现它、给它发任务、拿回结果。

2. **它把“聊天”升级成了“任务协作”**
   - 普通 LLM API 更像一次对话调用。
   - A2A 更强调 task、history、artifact、streaming、resubscribe 这些协作语义。

3. **它能自然承载多 Agent 系统**
   - 当你的系统里有 planner、executor、retriever、tool agent 或垂直领域 agent 时，A2A 比随手定义一堆私有 HTTP 接口更容易扩展。

适合阅读这类内容的人：

- 正在做 Agent 平台、Agent 网关、Agent 框架的人
- 需要让多个服务互相调用智能体能力的人
- 想搞懂“它和 REST、MCP、普通 LLM API 到底差在哪”的工程同学

---

## 关键概念拆解

### 1. A2A 到底是什么

从官方教程的摘要看，A2A 的核心目标是让独立 AI Agent 能够：

- 发现彼此能力
- 协商输入输出模式
- 安全地协作完成任务
- 交换结构化数据和工件

并且官方资料明确提到：

- **通信基于 HTTP(S) 的 JSON-RPC 2.0**
- **流式更新可通过 SSE 承载**

这两点非常重要，因为它解释了为什么你发请求时既会看到 HTTP path，也会看到 JSON-RPC 的 `method`。

---

### 2. 为什么 `message/send` 看起来像 path，其实不是 path

A2A 常见会长成这样：

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "message/send",
  "params": {
    "message": {
      "role": "user",
      "parts": [
        { "kind": "text", "text": "hello" }
      ]
    }
  }
}
```

这里容易误解的地方是：

- `message/send` **长得像 URL path**
- 但它本质上是 **JSON-RPC method 名**
- 真正的 HTTP 请求通常还是 `POST /某个统一入口`

也就是说，请求分两层理解：

1. **HTTP 层**：请求打到哪个 URL
2. **JSON-RPC 层**：body 里的 `method` 决定要执行哪个 A2A 动作

所以：

- REST 风格是 `POST /message/send`
- A2A/JSON-RPC 风格更像 `POST /`，body 里写 `"method": "message/send"`

---

### 3. Agent Card 是什么

Agent Card 可以理解成“这个 Agent 的公开说明书”。

它通常包含：

- 协议版本
- agent 名称
- 描述
- URL
- provider
- 支持的输入输出模式
- skills
- 是否支持 streaming / push notification 等能力

在工程上，它的意义相当大：

- 发现能力：调用方先知道你是谁、会什么
- 协议协商：知道你支持哪些 mode
- 自动接入：平台能根据 card 生成集成逻辑

如果把 A2A 比作 Web 世界，Agent Card 有点像“面向 Agent 的能力描述文档”。

---

### 4. Message、Task、Artifact 为什么是 A2A 的核心对象

A2A 不是只关心一次输入和一次输出，它更强调任务生命周期。

#### Message
表示一次对话消息，通常有：

- `role`
- `parts`
- `text` / 文件 / 结构化内容等

#### Task
表示一次可追踪的任务。相比“单次 LLM 调用”，A2A 更像：

- 先创建或延续一个 task
- 持续产生日志、状态、结果
- 后续可以查询、取消、重订阅

#### Artifact
表示任务过程中产生的结果物。

例如：

- 文本结论
- 报告
- 文件
- 中间产物

这就是为什么 A2A 比“一个普通聊天接口”更像“任务型协作协议”。

---

### 5. Streaming 在 A2A 里意味着什么

A2A 的流式不是简单“token 一个个吐出来”。

它更常见的是任务级的流式事件更新，比如：

- 任务开始 working
- 中间若干 message 更新
- 状态变化 completed / canceled
- 断开后还能 resubscribe 继续接收

所以它更接近：

> **一个长任务在持续向外报告进度和增量结果**

而不是仅仅“模型流式生成文本”。

---

## 重点对比表

| 维度 | 普通 HTTP / REST | A2A | 我的理解 |
|------|------------------|-----|----------|
| 底层传输 | HTTP | HTTP(S) | A2A 不是替代 HTTP，而是建在其上 |
| 动作表达 | path + verb，例如 `POST /tasks` | JSON-RPC `method`，例如 `message/send` | `message/send` 更像方法名，不是 path |
| 主要对象 | request / response | agent card / message / task / artifact | A2A 把“任务生命周期”显式建模了 |
| 返回方式 | 同步响应为主 | 同步 + streaming + resubscribe | 更适合长任务与协作 |
| 能力发现 | Swagger / 文档 / 自定义约定 | Agent Card | 面向 Agent 的能力说明 |
| 会话延续 | 业务自己约定 | Task ID / History | 跨调用连续性更自然 |
| 典型场景 | 后端服务 API | Agent 平台、Agent 网关、多 Agent 协作 | 比“聊天接口”更偏协作协议 |

---

## 要点整理

### 1. 从调用视角看，A2A 至少包含三类能力

#### 能力发现
调用方先拿 Agent Card。

典型入口通常类似：

```text
/.well-known/agent-card.json
```

#### 非流式消息发送
最典型的方法是：

```text
message/send
```

适合“一次发起、一次返回结果”的场景。

#### 流式消息发送与任务跟踪
常见还会有：

- `message/stream`
- `tasks/get`
- `tasks/cancel`
- `tasks/resubscribe`

这让 A2A 不只是“问答接口”，而更像“可追踪、可中断、可恢复”的任务协议。

---

### 2. 一个最小的 A2A 请求怎么长

如果按 HTTP + JSON-RPC 去理解，一个最小请求示意如下：

```bash
curl -X POST http://127.0.0.1:8080/ \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "message/send",
    "params": {
      "message": {
        "role": "user",
        "parts": [
          { "kind": "text", "text": "hello" }
        ]
      }
    }
  }'
```

这个例子很适合帮助建立直觉：

- URL 是 HTTP 入口
- `message/send` 是 JSON-RPC method
- `message` 是 A2A 语义对象

---

### 3. 一个最小的 Go server demo

下面这个思路来自 CloudWeGo Eino A2A 示例，结构非常适合入门：

```go
ctx := context.Background()
hz := hertz_server.Default()

r, _ := jsonrpc.NewRegistrar(ctx, &jsonrpc.ServerConfig{
    Router:      hz,
    HandlerPath: "/test",
})

_ = server.RegisterHandlers(ctx, r, &server.Config{
    AgentCardConfig: server.AgentCardConfig{
        Name:        "test agent",
        Description: "a agent used for testing",
        URL:         "https://127.0.0.1:8080",
        Version:     "1",
    },
    MessageHandler: func(ctx context.Context, params *server.InputParams) (*models.TaskContent, error) {
        return &models.TaskContent{
            Status: models.TaskStatus{
                State: models.TaskStateCompleted,
                Message: &models.Message{
                    Role: models.RoleAgent,
                    Parts: []models.Part{{
                        Kind: models.PartKindText,
                        Text: ptrOf("hello world"),
                    }},
                },
            },
        }, nil
    },
    CancelTaskHandler: ...,
    TaskEventsConsolidator: ...,
})

hz.Run()
```

这个 demo 体现了 A2A server 的最核心三件事：

1. 注册 JSON-RPC handler
2. 暴露 Agent Card
3. 实现 Message / Task 生命周期逻辑

---

### 4. 一个最小的 Go client demo

A2A client 的调用思路则更偏“先拿 transport，再拿 client”：

```go
transport, _ := jsonrpc.NewTransport(ctx, &jsonrpc.ClientConfig{
    BaseURL:     "http://localhost:8888",
    HandlerPath: "/test",
})

cli, _ := client.NewA2AClient(ctx, &client.Config{
    Transport: transport,
})

card, _ := cli.AgentCard(ctx)
result, _ := cli.SendMessage(ctx, &models.MessageSendParams{
    Message: models.Message{
        Role: models.RoleUser,
        Parts: []models.Part{{Kind: models.PartKindText, Text: ptrOf("hello")}},
    },
})
```

如果继续深入，还能看到：

- `SendMessageStreaming(...)`
- `GetTask(...)`
- `ResubscribeTask(...)`

这些都体现了 A2A 把“任务状态管理”放到了协议层。

---

### 5. 在 CloudWeGo Eino A2A 代码里能直接看到的方法名

本地依赖代码中，可以直接看到服务端注册了这些 JSON-RPC method：

- `message/send`
- `message/stream`
- `tasks/get`
- `tasks/cancel`
- `tasks/resubscribe`

这说明当前这套实现很明确是：

> **HTTP 统一入口 + JSON-RPC 方法分发 + A2A 任务语义对象**

---

## 实践印证：当前项目里 A2A 是怎么落地的

当前仓库刚好就是一个很好的实践样本。

### 1. 服务启动时，把 A2A wrapper 挂到现有 Hertz server 上

代码位置：

- `main.go`
- `agent/a2a.go`

关键逻辑是：

```go
hz := byted.Default(opts...)
bytedaiserver.NewMultiA2AAgentWrapper(hz).AddAgent(ctx, "", agent.InitA2AServerConfig(ctx))
hz.Spin()
```

这说明：

- A2A 不是另起一套独立进程协议栈
- 而是**挂在现有 HTTP server 上**
- 本地运行时，本质上还是同一个 Hertz 进程在处理请求

---

### 2. 本地默认端口就是 8080

从本地启动脚本可见：

```bash
if [[ -z "$PORT0" ]]; then
  export PORT0=8080
fi
```

所以本地验证时，通常直接访问：

```text
http://127.0.0.1:8080
```

---

### 3. 当前项目里 A2A handler 最终还是转成内部 runner.Run

`agent/a2a.go` 里的核心逻辑是：

- 把 A2A 输入的 `parts` 收集出来
- 转成内部 `schema.UserMessage(...)`
- 调 `runner.Run(ctx, userMessages)`

这说明 A2A 对这个项目来说，更像：

> **对外的标准协议接入层**

而不是内部 graph 编排本身。

换句话说：

- 外部看你：是 A2A agent
- 内部执行：还是现有的 runner / graph / tool 体系

---

### 4. 当前项目里的实践结论

这个项目非常适合用来理解下面这句话：

> **A2A 描述的是“Agent 如何被调用和协作”，而不是“Agent 内部必须怎么实现”。**

内部你可以是：

- ReAct agent
- Graph
- Workflow
- 普通函数调用

只要对外满足 Agent Card、message/send、task/query 这些协议约定，就能作为一个 A2A agent 暴露出去。

---

## 我的总结

A2A 真正有价值的地方，不是“又发明了一套 HTTP 写法”，而是它把多 Agent 协作里最容易散落在私有约定里的东西抽象出来了：

- 如何发现 agent
- 如何发起任务
- 如何拿回结构化结果
- 如何做流式更新
- 如何中断和恢复任务

所以它不像普通 REST 那样只解决“接口调用”，而更像是在解决：

> **异构 Agent 之间，如何以统一的任务语义去协作。**

如果只是做一个单体应用内的单模型调用，A2A 可能显得偏重。
但如果你在做：

- 多 Agent 平台
- Agent 网关
- 跨团队智能体协作
- Agent 市场或 Agent 目录

那 A2A 的抽象就会变得非常自然。

---

## 原文链接

> 仅存于本地笔记，发布时不带出

- [A2A 协议入门 | A2A Protocol](https://www.a2aprotocol.org/zh/tutorials/getting-started)
- [在您的应用中实现 A2A | A2A Protocol](https://www.a2aprotocol.org/zh/tutorials/implementing-a2a-in-your-application)
- [A2A Protocol 技术文档社区镜像](https://agent2agent.info/zh-cn/docs/)
- [CloudWeGo Eino A2A server demo（本地依赖路径）](file:///Users/bytedance/go/pkg/mod/github.com/cloudwego/eino-ext/a2a@v0.0.1-alpha.1/examples/server/server.go)
- [CloudWeGo Eino A2A client demo（本地依赖路径）](file:///Users/bytedance/go/pkg/mod/github.com/cloudwego/eino-ext/a2a@v0.0.1-alpha.1/examples/client/client.go)
