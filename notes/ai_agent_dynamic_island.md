# AI 编程代理的「刘海状态面板 / Dynamic Island」产品形态

> 来源（4 个同形态产品）：
> - VibeIsland（闭源付费，Swift 原生）：https://vibeisland.app/#pricing
> - CodeIsland（MIT 开源，Swift 原生）：https://github.com/wxtsky/CodeIsland
> - Flux Island / flux-desktop-app（字节内部，Electron 实现）：https://code.byted.org/cloud-fe/flux-desktop-app ；介绍文档 bytedance.larkoffice.com/docx/P8Z4d0GJjorqnKxYmNHcahdvnPc
> - 同类鼻祖：claude-island（https://github.com/farouqaldori/claude-island）
> 记录时间：2026-06-02

---

## 核心观点

当开发者同时跑多个 AI 编程代理（Claude Code / Codex / Cursor / Gemini CLI / OpenCode…）时，瓶颈从「写代码」转移到「盯代理」——要在一堆终端 tab 之间来回切，判断哪个在等审批、哪个跑完了、哪个卡住了。

这一类产品的形态是：**把 macOS 刘海区（Dynamic Island 隐喻）变成一个常驻的、环境光式（ambient）的「代理舰队控制塔」**——在不抢占窗口空间的前提下，实时展示所有代理状态，并把"需要人介入的时刻"（权限审批、代理提问、计划确认）直接推到刘海上，让你不切换上下文就能做决策、并一键跳到对应终端。

一句话：**它解决的不是写代码，而是"监督多个并行代理"的注意力分配问题。**

---

## 形态的四个共性设计

所有 4 个产品（无论原生还是 Electron、开源还是付费）都收敛到同一套设计：

### 1. 环境光式 UI，寄生在 OS chrome 上
- 锚定在刘海（notch）或顶部悬浮条，**始终可见但不占窗口空间**
- 非激活式 overlay：默认 `setIgnoreMouseEvents(true)`，鼠标 hover 才变可交互；idle 时收起
- 多屏支持

### 2. 事件驱动，靠 shell hooks 接入，纯本地无云
- 往每个代理的配置里装轻量 hook，代理触发生命周期事件时把事件推出来
- **本地 IPC（Unix socket）**传输，不走 API key、不走云、无 telemetry（隐私友好）
- 典型数据流（CodeIsland）：`AI Tool → Hook event → bridge → Unix socket → App → UI`
- flux-desktop 的 Electron 版数据流：`Agent hook → hooks-cli(stdin→socket) → BridgeServer → SessionState.apply() → IPC 广播 → renderer Zustand store → React`
- OpenCode 这类支持插件的，可用 JS 插件直连 socket，省掉 bridge 二进制

### 3. 决策面（decision surfacing）才是核心价值
- 实时 session 状态、tool call、AI 回复
- **从刘海直接 Allow/Deny 权限请求**、回答代理的多选提问
- 计划预览（Markdown 渲染）后再批准（plan review）
- "Smart suppress"：tab 级检测，只有在看那个 session 时才静音通知

### 4. 极速导航 + 感官反馈
- 一键跳到**精确的**终端 tab / split pane 或 IDE 窗口（VibeIsland 宣称支持 18+ 终端：iTerm2 / Ghostty / Warp / WezTerm / Kitty / Zellij / tmux / VS Code / Cursor…）
- 像素风吉祥物 + 8-bit 音效；额度/用量追踪（Claude / Codex / Kimi / GLM / DeepSeek）；SSH 远程代理监控

---

## 技术选型对比：原生 vs Electron

| 维度 | VibeIsland / CodeIsland | flux-desktop-app |
|---|---|---|
| 技术栈 | 纯 Swift（Apple Silicon，<50MB RAM） | Electron 41 + electron-vite + React 19 + Zustand |
| 进程模型 | 原生 + bridge 二进制（~86KB） | 三进程：main(Node) / renderer(Chromium) / preload(沙箱) |
| 刘海控制 | 原生 AppKit | Node Addon（`panel_fix.node`）做窗口层级控制 |
| 优劣 | 轻、省内存、贴近系统；开发门槛高 | 复用前端生态、迭代快；体积/内存大 |

**取舍点**：刘海这种"非激活悬浮 + 窗口层级 + ignore-mouse"的能力，原生最顺手；Electron 需要靠 native addon 补这块（`panel_fix.node`），其余 UI 用 Web 技术堆。

---

## 进阶能力：从「监控」到「全场景接力」（内部 Flux Island 的延伸）

字节内部的 Flux Island 把这个形态又往前推了几步，定位直接喊出「**AI 管家**」「升级与 AI 协作的**情绪体验**」，几个公共产品里没有的能力值得记：

1. **桌宠模式**：把刘海上的小章鱼图标拖出来，它就在桌面上当桌宠跑（三击退出，拖回岛上收回），还能自定义换肤。把"枯燥的状态监控"做成了有情绪陪伴的桌宠——这是"情绪体验"定位的落点。
2. **IM 接力通知（飞书接力）**：绑定 IM 账号后，**锁屏/离开电脑时，代理的提问与完成状态推到 IM，你能在手机上远程代答 Allow/Deny**。这把"在场调度"延伸成了"离场也能掌控"——本质是 PushNotification 的双向版（不只通知，还能远程决策）。
3. **IM 签名/Token 展示**：把本地各代理的 Token 消耗做成 IM 个性签名展示（带模板）。
4. **AI 代码贡献率 & Token 统计**：本地三方代理无法上报的数据，由这个面板在本地采集，再对接内部研发数据平台做全局统计——面板顺手变成了"AI 编码度量的采集端"。
5. **更广的载体矩阵**：不只 Terminal，还覆盖 IDE 主端 / 独立 APP / SSH 远程开发机；支持的代理多达十余个（含一批内部代理）。

**关键设计原则补充**：**Hooks fail open** —— 面板 app 没开时，代理照常运行，hook 不阻塞。这是这个形态能"零侵入装到生产工作流里"的前提。

---

## 商业形态对比

- **VibeIsland**：一次性买断 $19.99（早鸟），按设备数分档（1/2/3 Mac），license 可转移。主打"no subscription"。
- **CodeIsland**：MIT 开源，Homebrew 安装，~1.7k star。
- **flux-desktop-app**：内部工具，pnpm + 内部构建链打包 macOS arm64/x64。

> 同一形态可以三种路线并存：付费买断、开源引流、公司内部自用。说明这是个**需求真实、实现门槛中等、差异化空间小**的赛道——大家功能高度同质，竞争点落在"原生轻量 / 美术风格 / 支持的代理与终端数量 / 远程能力"这些细节。

---

## 形态背后的趋势判断

1. **"多代理并行"正在成为默认工作方式**：一个人同时挂 3-5 个 agent 跑不同任务，是这个产品形态的存在前提。
2. **人的角色从"作者"变成"调度员/审核员"**：产品的核心交互不是输入，而是"审批 + 答疑 + 跳转"。
3. **OS chrome 是稀缺的环境光载体**：刘海/灵动岛把"系统级常驻、低打扰、即时可达"三者合一，是这个形态选它的根本原因。
4. **本地优先是隐性卖点**：开发者代码敏感，"不上云、无账号、无 telemetry"几乎是必选项。
5. **从"在场调度"到"离场掌控"**：IM 接力让你离开电脑也能远程审批，监督半径从"盯着屏幕"扩展到"随时随地"。
6. **情绪价值正在被显式设计**：桌宠、8-bit 音效、像素风不是噱头——当人长时间处于"审核员"角色，陪伴感与低打扰是留存的关键。

---

## 实践印证（关联 Claude Code）

本机正是重度 Claude Code 用户，且常并行多个 agent / 后台任务（Monitor、background bash、workflow）。这个形态恰好命中痛点：

- Claude Code 的 hook 体系（`settings.json` 里的 PreToolUse / Stop / Notification hooks）就是这类面板的接入点——和它们用的 shell hooks 是同一机制。
- 我现有的 `PushNotification`（桌面/手机推送）解决的是"走开时被叫回来"，而刘海面板解决的是"在场但要在多个 session 间分配注意力"——两者互补。

---

## 参考链接

- VibeIsland: https://vibeisland.app/#pricing
- CodeIsland: https://github.com/wxtsky/CodeIsland
- claude-island（鼻祖）: https://github.com/farouqaldori/claude-island
- flux-desktop-app（内部）: https://code.byted.org/cloud-fe/flux-desktop-app
- 内部设计文档（待登录补充）: https://bytedance.larkoffice.com/docx/P8Z4d0GJjorqnKxYmNHcahdvnPc
