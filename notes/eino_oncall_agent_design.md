# OncallAgent 设计分享 — 基于 Eino 框架的智能运维诊断

> 来源：内部技术分享文档
> 记录时间：2026-04-16

---

## 核心观点

基于 Go 语言 AI 应用开发框架 Eino，结合大模型平台、MCP 工具协议和 Skill 机制，构建可配置化的 OncallAgent，替代研发人力处理日常问题排查。

---

## 要点整理

### 1. 背景与解决方案

业务 oncall 问题排查占用大量人力，核心思路是用 AI Agent 替代人工。技术栈：

| 组件 | 作用 |
|------|------|
| **Eino 框架** | 基于 Go 的 AI 应用开发框架，对标 Python 的 LangChain |
| **大模型平台** | 企业级大模型服务，提供精调、评测、推理 |
| **MCP** | Model Context Protocol，封装外部工具供 Agent 调用 |
| **Skill** | 可复用的诊断知识模板，指导 Agent 执行特定任务 |
| **Trace 平台** | 提供 Agent 执行过程的可观测性，辅助调试优化 |

### 2. 迭代历程：从 Workflow 到 DeepAgents

#### Workflow 模式

- 开发人员手动编排流程，使用 Eino 的 Chain/Graph 能力串联原子节点
- 每个节点可有一次 AI 调用，按预设流程顺次执行
- 流程：原始问题 → 问题分类（AI 判断）→ 路由到对应处理节点 → 诊断结果
- **优点**：确定性强，流程可控
- **缺点**：每次新增场景需改代码上线，提示词修改也要发版，碎片化难管理

#### DeepAgents 模式（当前方案）

- 一个 MainAgent 协调、规划、委派任务给多个 SubAgent
- 核心组件：
  - **ChatModel**：具备工具调用能力的大模型，负责推理和决策
  - **WriteTodos**：内置规划工具，将复杂任务拆解为待办列表
  - **TaskTool**：调用子 Agent 的统一入口
  - **Filesystem**：支持 Skill 的文件系统中间件
- **优点**：Agent 自主决策，灵活性高，新增场景无需改代码

### 3. Tools 配置：Agent 的手和脚

工具是 Agent 主动使用的外部功能模块，两种创建方式：

**MCP 工具**（推荐）：
- RPC 导入：基于已有 RPC 接口导入，开箱即用
- 自定义仓库：独立 PSM，底层为 FaaS 函数
- 关键经验：**MCP 参数描述要精确**。案例中查询结算单时，Agent 因参数描述不清而自行猜测参数，优化描述后才能正确 by 天查询

**自定义函数**：
- 基于 Eino 接口实现，适合简单常用工具（如获取当前时间、时间戳转换）

### 4. Skill：Agent 的知识库

Skill Middleware 让 Agent 动态发现和使用预定义技能。核心是 `SKILL.md` 文件，包含元数据和执行说明。

管理方式：
- 代码仓库：用 `embed.FS` 嵌入
- 平台托管：支持版本控制，`skill_key` + `version` 标识

关键经验：**必须在提示词中强制要求 Agent 读取 Skill**。实际案例中，Agent 跳过 Skill 自行猜测诊断流程，导致关键参数未配置而报错。

### 5. Trace 与 Callback：可观测性

- Trace 用于分析诊断流程是否正确，从黑盒到白盒
- Callback 分两种注入方式：
  - **全局注入**：所有实例生效
  - **Option 注入**（推荐）：细分到不同空间，业务互不影响
- 支持实时输出中间过程，便于调试

### 6. 可配置化设计

核心目标：**大部分迭代无需改代码**。通过配置中心管理：

**Agent 配置**（JSON）：
```json
{
  "main_agent": {
    "agent_name": "诊断总控",
    "instruction": "系统提示词..."
  },
  "sub_agent": [
    {
      "agent_name": "子Agent名称",
      "instruction": "子Agent提示词...",
      "tools_list": ["tool1", "tool2"]
    }
  ]
}
```

**迭代流程**：
1. MCP 优化 → 平台上修改工具
2. Skill 优化 → 平台上更新版本
3. 部署 → PPE 环境验证
4. Agent 配置 → 更新 tool_list / skill_list
5. 上线 → 配置中心直接发布

### 7. 新业务接入步骤

1. 创建 Trace 空间 + 配置 Skill
2. 创建 MCP 服务管理工具（可选）
3. 新增 Agent 配置（参考已有格式）
4. 初始化问答机器人 + 消息监听 → 部署即可

### 8. 待优化点

- MCP 服务初始化、问答机器人配置尚未完全从代码解耦，目标是**全流程代码零改动**
- Oncall 流程还未完全打通事前/事中/事后
- 暂不支持多轮对话

---

## 实践经验总结

1. **MCP 参数描述决定 Agent 行为质量** — 描述不精确，Agent 就会猜测参数
2. **提示词必须强制读取 Skill** — 否则 Agent 可能跳过诊断模板自行发挥
3. **Callback 用 Option 注入优于全局注入** — 不同业务空间互不干扰
4. **配置化是 Agent 可维护性的关键** — 提示词、工具、Skill 都应可配置
5. **DeepAgents 模式优于 Workflow 模式** — 灵活性和可维护性更好，但需要更精确的提示词控制

---

## 参考文档

- [Eino Workflow Agent 文档](https://www.cloudwego.io/zh/docs/eino/core_modules/eino_adk/agent_implementation/workflow/)
- [Eino DeepAgents 文档](https://www.cloudwego.io/zh/docs/eino/core_modules/eino_adk/agent_implementation/deepagents/)
- [Eino Tool 创建指南](https://www.cloudwego.io/zh/docs/eino/core_modules/components/tools_node_guide/how_to_create_a_tool/)
- [Eino Skill 机制](https://www.cloudwego.io/zh/docs/eino/core_modules/eino_adk/eino_adk_chatmodelagentmiddleware/middleware_skill/)
- [Eino Callback 与 Trace](https://www.cloudwego.io/zh/docs/eino/quick_start/chapter_06_callback_and_trace/)