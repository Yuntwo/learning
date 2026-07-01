# GFW DNS 投毒：被墙域名/中转 BASE_URL 访问失败的诊断与解决

> 来源：实战排查沉淀（Claude Code ANTHROPIC_BASE_URL 指向 api.poe.com，公司 WiFi 能连、连 VPN 与海外 VPN 均连不上）
> 记录时间：2026-06-05

---

## 核心观点

被墙域名"某网络能连、换网络/连 VPN 就连不上"，根因往往是 **GFW 对明文 DNS 的注入投毒**，而非 IP 本身不可达。排查的关键是用 `dig`（DNS 对不对）和 `curl --resolve`（正确 IP 通不通）把故障**精确锁定到 DNS 层还是 IP 可达层**——两层独立、修法完全不同。

---

## 原理基础篇（理解"为什么会被投毒"）

> 下面是支撑后面诊断方法论的底层模型。诊断决策树/DoH/dnscrypt 调优见后文，这里只讲原理。

### A. DNS 是什么
DNS（Domain Name System）= 互联网的**通讯录**：计算机间通信只认 IP（数字地址），DNS 负责把人记得住的**域名翻译成 IP**。
- 你设置的 "DNS 服务器"（运营商的、`8.8.8.8`、公司 `10.1.0.2`）指的是**递归解析器（Recursive Resolver）**——它替你跑完整个查询流程，你只问它一次。
- 真正"持有"某域名记录的是**权威服务器（Authoritative Server）**，分**根（.）→ 顶级域（.com）→ 域名（poe.com）** 三层。

### B. 递归查询 vs 迭代查询
- **递归查询**：你 → 解析器，"你帮我查到底，直接给我最终答案"。
- **迭代查询**：解析器 → 各级权威，一级级问下去（根说"去问 .com"，.com 说"去问 poe.com 权威"，权威给出 IP）。
- **缓存是关键优化**：解析器查过一次会按 TTL 缓存，多数查询直接命中缓存、不跑完整链路；你的设备/浏览器本地也有缓存。**缓存命中与否，直接决定一次解析要不要真的出境去问权威**（见 D）。

```
你 →(递归)→ 解析器(10.1.0.2) →(迭代)→ 根(.) → .com → poe.com权威 → 拿到IP并缓存 → 回给你
```

### C. GFW 怎么决定投不投毒（最反直觉的一点）
**GFW 不看 DNS 服务器是国内还是国外**，它做的是：在**国际骨干出入口链路上旁路嗅探**每个经过的**明文 DNS 包（UDP 53）**，解析出域名去匹配黑名单。

成立需要**两个必要条件同时满足**：
1. 这个**明文 DNS 包跨境经过了 GFW**（GFW 物理上能看到它）；
2. 查询的**域名命中黑名单**。

命中即**抢答投毒**：DNS 是无连接 UDP、"谁先到信谁"，境内伪造包永远比真实应答先到 → 你采信假 IP（常是 Facebook 段 `157.240.x.x`）。**打破任一条件即可避免投毒**——这正是后文所有解法（DoH 加密让 GFW 看不到域名 / 公司专线让查询不跨墙）的根本原理。

> 误区澄清："我用国内 DNS 没出国，怎么会被墙？" —— **你没出国，但你的 DNS 替你出国了**（见 D）。决定性因素是"这趟查询的明文有没有跨墙被 GFW 看到"，不是 DNS 服务器的国籍。

### D. 为什么解析 `poe.com` 必然走跨境，而 `baidu.com` 不会
关键在**权威服务器在哪**：
- `poe.com` 的权威服务器**在国外**。国内 DNS 缓存里没有它的记录（冷门/被墙、没人查或早过期）时，**只能替你出境去问国外权威** → 这一跳明文跨墙 → 命中黑名单被投毒。
- `baidu.com` 的权威服务器**在境内**，整条链路不出境，GFW 碰不到 → 又快又准。

所以核心结论:**国内 DNS 不是"不能解析" poe.com，而是"解析得到、但解析错"**——它拿到的是 GFW 伪造的假 IP。"能不能解析"和"解析得对不对"是两回事。

| | 国内公共 DNS（运营商/114） | 公司内网 DNS `10.1.0.2` |
|---|---|---|
| 查 poe.com 要不要出境问权威 | 要 | 要 |
| 出境那一跳走哪条路 | **明文经过 GFW** | **专线/干净出口，GFW 看不到明文** |
| 拿到的 IP | ❌ 投毒假 IP | ✅ 真实 IP |

→ 两者都得跨境查权威，差别只在**跨境那一跳 GFW 能不能看到明文域名**。这就闭环解释了"为什么公司内网 DNS 干净"（详见后文第 2 点）。

### E. Cloudflare IP 的三种含义（呼应后文"DNS 入口段被墙、业务 IP 可达"）
"Cloudflare IP"在不同语境指三种不同东西，别混为一谈：
1. **网站接入 Cloudflare 后对外暴露的边缘 anycast IP**：DNS 解析出来的不是源站真实 IP，而是 Cloudflare 边缘节点；用 anycast 同一 IP 全球广播就近接入，兼做隐藏源站/防 DDoS/WAF/缓存。
2. **官方公布的 IP 段（IP Ranges）**：用于在源站防火墙做**回源白名单**（只放行 Cloudflare 回源、拒绝直连源站）。列表见 https://www.cloudflare.com/ips/ 。
3. **公共 DNS `1.1.1.1` / `1.0.0.1`**：免费递归解析服务。

⚠️ 与本文强相关：**Cloudflare 的"DNS 入口段"和"业务 CDN 段"在墙内待遇不同**——DNS 入口 anycast（如 `1.1.1.1`/`104.16.x`）整段被墙，业务 CDN IP（如 `162.159.152.x`）通常可达。所以"Cloudflare 被墙"不能一概而论（明细见后文第 5 点表格）。

---

## 要点整理

### 1. 投毒机制
- GFW 对黑名单域名做 **DNS 注入**：明文 DNS 查询（UDP 53）跨国境时，GFW 抢在真实应答前回一个**伪造 IP**。DNS "谁先到用谁"，境内伪造包永远更快。
- 伪造目标常是 **Facebook 段**（`157.240.x.x`、`173.252.x.x`）等无关 IP——看到被墙域名解析成 Facebook IP，基本实锤投毒。
- **成立的两个必要条件**：① 查的是被墙域名；② 明文 DNS 包跨过了 GFW。打破任一即可。

### 2. 为什么公司内网 DNS 干净、连 VPN 反而被投毒
- 纯公司 WiFi：系统 DNS = 公司内网 DNS（如 `10.1.0.2`），解析全程**不跨墙**（内网近端 + 公司专线/干净海外出口做递归），GFW 没机会注入 → 干净。
- 连 VPN 的那一刻，**系统 DNS 被顶替成 `::1`**（VPN 客户端起的本地转发器），它把查询漏成明文跨墙 → 被投毒。
- 因果链：`连 VPN → 系统 DNS 换成会漏明文的 ::1 → 明文 DNS 跨墙 → GFW 投毒`。**VPN 不直接投毒，它只是换掉了你用的 DNS**。决定性因素是"用了哪个 DNS + 路径干不干净"。

### 3. 为什么海外 VPN 也不行
两个叠加原因：
- **规则代理分流（rule-based routing）**：Clash/Surge/V2Ray 按规则表分流。Google/YouTube 在"走代理"名单里能开，但小众域名（如 `poe.com`）没命中规则 → 落到默认 **DIRECT 直连** → 用系统 DNS（脏）+ 直连（可能被挡）→ 失败。
- **DNS leak**：VPN 把数据流量送进隧道，却没把 DNS 查询也送进去，DNS 仍明文跨墙被投毒。
- 浏览器能开 Google 是因为走代理时通常是**远程 DNS 解析**（域名原样交给墙外代理解析），而 CLI 工具/DIRECT 流量用本机系统 DNS。

### 4. 诊断决策树（核心方法论）
```bash
# Step 1: DNS 对不对 —— 在"能连/不能连"两种网络各跑对比
dig +short <域名>        # 目标写裸域名, 别写 https://; 看 Server: 行知道用的哪个 DNS

# Step 2: 正确 IP 通不通 —— "判官命令", 绕过系统 DNS 强制用指定 IP
curl -sS -m 12 -o /dev/null -w "%{http_code}\n" \
  --resolve <域名>:443:<正确IP> https://<域名>
```
判定：
- `dig` 两网络结果不同 + 坏的是 Facebook 段 → **DNS 投毒**。
- `curl --resolve` 返回 **200/403/任意 HTTP 码** → 正确 IP 可达，**纯 DNS 问题**，DoH 能解决。
- `curl --resolve` **超时** → 正确 IP 也被挡，**IP 可达层问题**，DoH 救不了，需代理走隧道。

### 5. 重要区分：同一家 CDN，DNS 入口和业务 IP 待遇不同
| IP | 用途 | 墙内状态 |
|---|---|---|
| `1.1.1.1`/`1.0.0.1`/`104.16.x`/`162.159.36.1` | Cloudflare **DNS 专用 anycast** | ❌ 整段被墙 |
| `162.159.152.x`/`.153.x` | Cloudflare **业务 CDN IP** | ✅ 可达 |

别把"Cloudflare 被墙"一概而论——业务 IP 通常没事，被墙的是 DNS 入口段。

### 6. 选对 DoH 上游（墙内最大的坑，必须实测）
| DoH 上游 | 可达性 | 对被墙域名返回 | 结论 |
|---|---|---|---|
| Cloudflare（1.0.0.1 / 104.16.x） | ❌ 超时 | — | DNS 入口被墙，不可用 |
| AliDNS `223.5.5.5` | ✅ 可达 | ❌ Facebook IP | **自身递归也被污染**，不可用 |
| DNSPod `1.12.12.12` | ✅ 可达 | ❌ 假 IP | 同上，不可用 |
| **Google `8.8.8.8` / dns.google** | ✅ 可达 | ✅ 正确 Cloudflare IP | **墙内首选** |

教训：国内公共 DoH 看着可达，但它们的上游递归同样跨墙被污染，对被墙域名照样给脏结果。**加密的只是"你到 DoH 服务器"这一段，DoH 服务器自己再去递归如果跨墙照样中招。**

### 7. dnscrypt-proxy 墙内特调
- **bootstrap 鸡蛋问题**：dnscrypt 默认要先从 GitHub 下载 resolver 列表，而下载依赖明文 bootstrap DNS + netprobe（`9.9.9.9:53`），墙内全被挡 → 启动卡死 `no servers could be reached`。
- 解法：**写死 static stamp（内嵌 IP，免下载）+ `netprobe_timeout = 0`**。
  ```toml
  server_names = ['google']
  netprobe_timeout = 0
  [static]
    [static.'google']
      stamp = 'sdns://AgcAAAAAAAAABzguOC44LjgACmRucy5nb29nbGUKL2Rucy1xdWVyeQ'  # 8.8.8.8 dns.google /dns-query
  ```
- 前台自检确认 `Now listening to 127.0.0.1:53` + `[google] OK (DoH)`，再起服务、`networksetup -setdnsservers Wi-Fi 127.0.0.1`。

### 8. 未解决的坑（诚实记录）
- **brew services 的 launchd 实例不工作**：`brew services start` 报成功、进程也在，但不在 53 端口应答；**同一份配置前台手动跑却完全正常**。怀疑 launchd 封装/环境问题，待改用官方 `-service install` 或查 launchd 日志。
- **VPN 顶替 DNS 会绕过 dnscrypt**：本地 `127.0.0.1:53` 只在系统真用它时有效；连 VPN 后系统 DNS 若被改成 `::1`，dnscrypt 形同虚设。需在代理客户端把上游 DNS 指到 `127.0.0.1`，或用 TUN + fake-IP / 加路由规则让目标走隧道。
- 所以"DoH 修系统 DNS"主要解决**不连 VPN**的场景；VPN 场景的真正解法多半在代理客户端侧（远程 DNS / 路由规则 / TUN）。

---

## 其它处置选项
- **系统级 DoH profile**（macOS `.mobileconfig`）：但 Cloudflare DoH 入口墙内不可用，需换 Google DoH profile。
- **代理 TUN + fake-IP**：DNS 由客户端在墙外解析，对 CLI 工具最省心。
- **应急 hosts 写死**：`echo "<正确IP> <域名>" >> /etc/hosts`，立竿见影但 IP 会变，仅临时。
- **CLI 单独走代理**：`export HTTPS_PROXY=http://127.0.0.1:<port>`，同时绕开投毒和 IP 封锁。

---

## 参考链接
- DNS Stamps 规范：https://dnscrypt.info/stamps-specifications/
- dnscrypt-proxy：https://github.com/DNSCrypt/dnscrypt-proxy
- 配套 skill：`~/.claude/skills/gfw-dns-diagnose/SKILL.md`
