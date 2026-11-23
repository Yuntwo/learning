# DeepSeek Harness 插件机制与 Langfuse Trace 插件实践

> 来源：用户实践整理 + 相关 GitHub 仓库 README
> 记录时间：2026-08-21

---

## 核心观点

DeepSeek Harness（DSH）这波插件生态开始成型后，真正有意思的已经不只是“能装插件”，而是**插件把会话能力、Web UI、工具链和观测体系都开放成了可组合扩展点**。在这个基础上，`dsh-langfuse-plugin` 这类观测插件的价值就非常明确：**不去替代官方 trace，而是沿着官方 telemetry seam 做 sidecar 增强，把 DSH 会话过程更完整地投影到 Langfuse 里**，让 trace、observation、score、dataset 这些后续分析能力都接上。

---

## 为什么值得看

这次值得记录，不只是因为我自己做了一个插件，而是因为我感觉 DSH 已经从“一个能跑大模型对话的壳”进入到了“可以被二次开发、可以长出生态”的阶段。

这类生态一旦跑起来，最值得关注的不是单点功能，而是三个问题：

1. **插件能插到哪里**：是只改前端展示，还是能接到 host 端能力、会话状态、工具协议、观测链路。
2. **插件解决什么真实问题**：是炫技 demo，还是能补齐官方能力之外的空白。
3. **插件之间是否能形成示范效应**：优秀 demo 会让后来者更快理解 profile、bundle、client/host 分层、patch、slot、状态持久化这些关键概念。

从这个角度看，这几个参考仓库都很有代表性：
- `dsh-visualize`：证明对话里可以直接长出可交互 UI。
- `dsh-stock-watch`：证明 Web UI 可以被改造成垂直场景工具面板。
- `dsh-agent-teams`：证明单会话可以被扩成一个可协调的多 Agent 团队。
- `dsh-langfuse-plugin`：证明会话 telemetry 可以被侧挂投影到 Langfuse，做更完整的外部观测和后续分析。

---

## 关键概念拆解

### 1. DSH 插件的核心不是“装包”，而是“组合 profile”

从几个 demo 的 README 可以看出，DSH 插件的常见安装方式都是：

```bash
dsh plugin --profile web add <plugin>
```

这说明它不是简单的浏览器插件思路，而更像是：

- 先有一个 profile，比如 `web` / `headless`
- 再把 bundle patch 合并进去
- 最终形成一份组合后的运行配置

也就是说，插件机制本质上是在扩展 **DSH 的运行时组合层**。这也是为什么很多 README 都会建议安装完再跑：

```bash
dsh --profile web --dump-config
dsh web
```

因为真正重要的是：你的插件有没有进入最终配置里，挂到了正确的位置。

### 2. DSH 插件大致分成三类能力

结合这几个 demo，可以把 DSH 插件理解成三类：

| 类型 | 典型作用 | 代表插件 | 我的理解 |
|------|----------|----------|----------|
| UI 展示型 | 在 Web UI 里渲染新交互、新面板、新卡片 | `dsh-visualize`、`dsh-stock-watch` | 证明前端 slot / overlay / 客户端模块可扩展 |
| 协作编排型 | 把单会话扩成多成员协作系统 | `dsh-agent-teams` | 证明 session 之上还能长状态机、调度器、消息系统 |
| 观测集成型 | 把 DSH 内部 telemetry 投影到外部平台 | `dsh-langfuse-plugin` | 证明插件不仅能改“看见什么”，还能改“如何记录和分析过程” |

### 3. `dsh-langfuse-plugin` 的定位不是替代官方，而是补完整体观测能力

我这次自己做的 `dsh-langfuse-plugin`，核心思路不是接管官方 telemetry 服务键，而是：

- 观察官方暴露出来的 `session-telemetry` 接缝
- 作为 sidecar exporter 挂在旁边
- 在不破坏原有 redaction / consent 时机的前提下
- 把 session telemetry 映射到 Langfuse trace、generation、span、score、dataset item

这类设计的意义在于：

1. **兼容官方链路**：不是暴力替换，而是沿着现有 seam 做增强。
2. **能力更完整**：Langfuse 本身在 trace、observation、dataset、feedback 关联上有一套更成熟的产品能力。
3. **后续更容易扩展**：先打通 trace 上报，后面可以继续往 feedback、评测、数据集沉淀、release/environment 标记等方向延展。

如果用一句话概括这个插件的定位，就是：

**它不是重新发明一套 trace，而是把 DSH 会话过程更完整地接到 Langfuse 的分析体系里。**

### 4. sidecar 方案为什么比“直接接管官方 trace”更稳

从工程角度看，sidecar 方案有几个明显优点：

- **边界清晰**：官方继续负责原本的 session telemetry capture、redaction 和 consent。
- **侵入性更低**：插件只消费已经形成的 telemetry 记录，不需要重写主流程。
- **更容易组合**：官方 exporter 和第三方 exporter 可以同时存在。
- **验证路径简单**：只要 profile 里出现新的 telemetry row，并且 Langfuse 收到 trace，就能完成闭环验证。

这也是我觉得这个方向很适合做开源插件的原因：

**它不像改主流程那样重，也不像纯 UI demo 那样轻，正好处在“有明确工程价值、又适合生态扩展”的位置。**

---

## 重点对比表

| 项目 | 主要做什么 | 有趣点 | 对插件开发者的启发 |
|------|------------|--------|-------------------|
| `dsh-langfuse-plugin` | 把 DSH session telemetry 上报到 Langfuse | 沿官方 seam 做 sidecar，不接管官方 telemetry key | 观测类插件可以走“兼容增强”路线，而不是重写官方链路 |
| `dsh-visualize` | 让模型在对话里直接生成可交互可视化卡片 | 对话不只输出文本，还能直接展示 simulator / chart / mockup | 客户端模块 + Web UI 卡片渲染是个很有想象力的方向 |
| `dsh-stock-watch` | 在 DSH Web UI 右上角挂一个 A 股实时盯盘面板 | 把聊天产品直接改造成垂直工具工作台 | 插件可以做常驻 overlay，甚至长出完整业务面板 |
| `dsh-agent-teams` | 把单个 DSH 会话扩成 captain + 多成员协作团队 | 有任务依赖、调度、消息、持久化和实时面板 | DSH 不只是“对话 + 工具”，还能承载多 Agent 编排层 |

---

## 要点整理

### 一、`dsh-langfuse-plugin`：为什么应该放在最前面讲

如果这篇内容是面向 GitHub / 社区传播，我觉得最应该放在最前面的就是我的插件：

**`dsh-langfuse-plugin`**  
GitHub：<https://github.com/Yuntwo/dsh-langfuse-plugin>

原因很直接：

1. 这是你这次真正的主角内容。
2. 它解决的是一个很实际、很工程化的问题：外部观测与分析能力。
3. 相比单纯展示型 demo，它更容易让同类开发者马上理解“这个插件有实际价值”。
4. “欢迎 star 和 contribute” 这种 CTA 也更适合挂在主角项目上，而不是放在文末角落里。

如果写社交平台内容，最合理的顺序应该是：

- 先讲 DSH harness 出来以后，插件生态开始有意思了。
- 再直接亮出自己的 `dsh-langfuse-plugin`。
- 然后补充几个很有趣的参考 demo，说明生态已经出现不同方向。
- 最后回到自己的项目，邀请大家 star / contribute。

### 二、`dsh-visualize`：把“模型输出”变成“对话内 UI”

这个 demo 最有意思的地方，是它把大模型输出从一段文字，扩展成了**对话中可交互的 HTML 卡片**。

它适合做的东西包括：
- 模拟器
- 图表
- 对比面板
- UI mockup

我觉得这个 demo 的示范意义很强，因为它把一个常见误区打破了：

很多人会觉得大模型应用最终还是文本交互，但 `dsh-visualize` 说明：
**如果客户端 slot 和渲染入口开放，对话本身就可以直接长出可操作的前端界面。**

### 三、`dsh-stock-watch`：把 DSH Web 直接改造成垂直工具台

这个插件也很有趣，它不是在聊天区里插内容，而是在 Web UI 右上角挂了一个**可折叠、可拖动、可持久化位置的股票盯盘面板**。

亮点包括：
- 自选股分组
- 实时行情刷新
- 分时图 / K 线图
- 目标价提醒
- 新建 DSH 对话发起投资研究

这个 demo 最值得关注的点不是“能看股票”，而是它证明了：

**DSH Web UI 可以被插件改造成一个面向特定场景的工作台，而不只是聊天窗口。**

也就是说，插件不是只能“增加一个工具调用”，它还能长时间驻留在 UI 上，变成一个长期使用的业务组件。

### 四、`dsh-agent-teams`：把单会话升维成多 Agent 协作系统

这是另一个很有代表性的方向。

`dsh-agent-teams` 做的事情，是让当前会话变成 captain，然后拉起多个可持续唤醒的成员 agent，围绕任务依赖去协作。

它比较有意思的点包括：
- durable member
- dependency-aware task
- direct message
- automatic scheduler
- live Web UI

这个插件最让我觉得有意思的地方是：

**它说明 DSH 插件不仅能加能力，还能加一层“协作协议 + 状态机 + 可视化控制面板”。**

这已经不是给单个 agent 增强一点工具，而是在 agent 之上再搭一个团队编排层。

### 五、从这几个 demo 看，DSH 插件生态已经出现了 3 条很清晰的路

我现在看下来，DSH 插件的可玩性至少已经有 3 条主线：

1. **UI 增强**：让对话里出现更丰富的交互界面。
2. **工作台增强**：把 Web UI 改造成特定业务场景工具。
3. **系统能力增强**：补齐调度、观测、持久化、协作这些更底层的工程能力。

而 `dsh-langfuse-plugin` 我觉得正好落在第三类里，而且是非常有实际价值的一类。

因为只要有人开始认真做：
- trace 归档
- 反馈打分
- dataset 沉淀
- release / env 维度分析
- 多轮 session 质量回看

那么观测就一定会从“可有可无”变成“必要基础设施”。

### 六、后续可扩展方向

基于当前 README 里已经写到的能力，我觉得这个插件后面还有很多值得继续做的方向：

- 更完整的 feedback -> score 映射
- dataset item 的更细粒度构造
- 针对不同 session / tool / model 的聚合视角
- release / environment 维度的质量回溯
- 更方便的配置模板和验证脚本
- 与评测、回放、人工标注流程联动

也就是说，现在这个插件已经把“trace 上报”打通了，下一步完全可以继续往**观测闭环**做。

---

## 我的总结

如果只把最近这波 DSH 插件当成“几个有趣 demo”，那其实低估了它的意义。

更准确的说法应该是：

**DSH 正在从一个可用的 harness，长成一个可以被社区持续扩展的插件平台。**

而在这个过程中，`dsh-langfuse-plugin` 这类插件的价值很明确：它不只是展示一个 UI 效果，也不只是做一个好玩的场景，而是在补齐真实开发中非常关键的一块——**可观测性**。

如果后面真的有越来越多插件作者开始沿着 UI、工作台、观测、调度这些方向往前做，那 DSH 的插件生态会越来越像一个“能长业务能力、能长工程能力”的平台，而不只是一个支持对话的壳。

对我自己来说，这次最值得记住的一点就是：

**做插件不一定非要推翻官方能力，很多时候顺着官方暴露出来的 seam 做增强，反而更稳、更容易组合、也更容易被社区接受。**

---

## 原文链接

> 仅存于本地笔记，发布时绝不带出

- <https://github.com/Yuntwo/dsh-langfuse-plugin>
- <https://github.com/Nagi-ovo/dsh-visualize>
- <https://github.com/Awu12277/dsh-stock-watch>
- <https://github.com/NanmiCoder/dsh-agent-teams/tree/main>
