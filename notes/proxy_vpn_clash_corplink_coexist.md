# 橘子加速(Clash) 与 飞连(CorpLink) 同时运行的工作原理

> 来源：本机实测排查 + 原理推导（2026-06-15 对话沉淀）
> 记录时间：2026-06-15

---

## 核心观点

橘子加速和飞连不是并列竞争关系，而是工作在**两个不同的层**：
**橘子加速是「代理层」(上层)，飞连是「路由层」(下层)**。
一个数据包先问橘子加速"要不要代理我"，再问系统路由表"该从哪块网卡出去"，飞连就藏在路由表里负责把内网流量塞进 VPN 隧道。因此两者能和平共存。

---

## 要点整理

### 1. 两者工作原理截然不同

- **橘子加速 = 本地代理模式 (Clash 内核)**
  - 进程：`AtlasCore_arm64`（应用名「橘子加速」，数据目录 `io.juzijiasu.cc/clash`）
  - 在本机监听 `127.0.0.1:6384`（HTTP/HTTPS/SOCKS 同端口），并写入**系统代理**设置
  - 端口是动态分配的，重启/重连后**可能变**，用 `scutil --proxy` 查最新值
  - 应用把流量发给它，它按规则决定：直连 or 转发到远端节点（翻墙）

- **飞连 = 系统级 L3 VPN (英文名 CorpLink)**
  - 应用路径 `/Applications/CorpLink.app`，含系统网络扩展 `/Library/CorpLink/CorpLink System Extension.app`
  - 通过 `utun` 虚拟网卡 + 路由表接管流量，**不暴露任何 proxy host:port**
  - 所以"飞连的代理地址端口"这个东西**根本不存在**，连上后应用无需配置代理即可走内网
  - 看连接状态：`/usr/local/corplink/corplink-cli swg status`（注意 `swg disconnected` 指的是"安全网关 SWG"子组件，跟 VPN 隧道本身是否起来是两回事）

### 2. 飞连是「分流模式 (split tunnel)」

- 飞连只在路由表里抢公司内网网段（`10.0.0.0/8` 一大票 `10.x/16 → utun6`），公网流量它不碰
- utun6 拿到内网 IP（如 `10.254.229.250`），内网网段全部指向 utun6

### 3. 一个数据包的完整决策流程

```
应用发起连接
   │
   ├─ 应用走系统代理吗?(系统代理 = 127.0.0.1:6384)
   │
   ├─ 是 → 先进【橘子加速/Clash】，按规则判断:
   │        ├─ 命中"直连/内网"规则 → Clash 自己发起直连
   │        │     └─ 交给系统路由表 → 目标 10.x? → 走飞连 utun6
   │        │                          否则      → 走 en0 直出
   │        └─ 命中"代理"规则 → Clash 连海外节点(走 en0 公网) → 翻墙
   │
   └─ 否(不认代理的程序，如 curl --noproxy)
          └─ 直接进系统路由表 → 10.x 走飞连，其余直出 en0
```

### 4. 实测证据（同时开启两者时）

| 测试 | 出口 | 说明 |
|------|------|------|
| 直连外网（绕过代理 `curl --noproxy`） | 家宽带 IPv6 | 没走任何工具，直接出公网 |
| 走橘子加速（`curl -x 127.0.0.1:6384`） | 海外节点 IP | 转发到远端节点翻墙 |
| 访问 10.x 内网 | 经 utun6 | 飞连 VPN 隧道接管 |

### 5. 实际影响与注意事项（坑）

1. **正常不打架**：飞连管内网、橘子加速管公网翻墙，各管一段。
2. **访问内网（10.x / 内网域名）靠飞连**，别指望橘子加速。
3. **翻墙靠橘子加速**，它连海外节点走公网 en0，飞连不拦截，不冲突。
4. **若飞连切「全局/全流量模式」**（default 全进 utun6），橘子加速连海外节点的流量也会被飞连吞掉 → 变慢/翻墙失效/被公司网关拦。分流模式下没这问题。
5. **橘子加速规则若把内网网段/域名误配成"走代理"**，会绕开飞连导致内网访问异常。排查内网问题时先把规则切直连或临时退出橘子加速验证。
6. **两者改的系统设置不同**：飞连改**路由表**，橘子加速改**系统代理**。互不直接干扰，但端口可能变。

### 常用排查命令

```bash
scutil --proxy                              # 看系统代理(橘子加速)的地址端口
lsof -nP -iTCP:6384 -sTCP:LISTEN            # 确认监听端口的进程
netstat -rn -f inet | grep utun            # 看 VPN 路由(飞连接管的网段)
ifconfig | grep -A1 utun                    # 看 utun 虚拟网卡的内网 IP
/usr/local/corplink/corplink-cli swg status # 飞连状态
curl -s --noproxy '*' https://ifconfig.me   # 直连出口 IP
curl -s -x http://127.0.0.1:6384 https://ifconfig.me  # 走代理出口 IP
```

---

## 补充：git clone 访问 GitHub 的实际走向（2026-06-15 追加）

### 前提：git 默认不读系统代理

macOS 系统代理设置对 git 命令行**无效**。git 是否走代理取决于：
- 环境变量 `http_proxy` / `https_proxy` / `all_proxy`（git 底层用 curl，认这些）
- `git config http.proxy` / `http.https://github.com/.proxy`
- SSH 方式则看 `~/.ssh/config` 里有没有 `ProxyCommand`

本机实际配置：
- 设了 `HTTPS_PROXY=http://127.0.0.1:6384`（指向橘子加速）
- git 没配 http.proxy
- `~/.ssh/config` 里 github.com 无 ProxyCommand
- **关键**：`github.com` 被飞连 DNS 解析成 `30.100.0.7`（字节内网 GitHub 镜像网关）

### 两种克隆方式走完全不同的两条路

| 方式 | 路径 | 谁在起作用 |
|------|------|-----------|
| `git clone git@github.com:...`(SSH) | DNS→30.100.0.7 → 路由 utun6 → 飞连隧道 → 内网 GitHub 网关 | **飞连** |
| `git clone https://github.com/...`(HTTPS) | 读 HTTPS_PROXY → 橘子加速 127.0.0.1:6384 → CONNECT 隧道 → 翻墙海外节点 → 真实 github | **橘子加速** |

- SSH：`~/.ssh/config` 无 ProxyCommand，按路由表走，落进飞连隧道到内网网关。实测用个人密钥 `id_ed25519_personal` 认证成功（账号 Yuntwo）。
- HTTPS：被 `HTTPS_PROXY` 环境变量送进橘子加速，翻墙到真实 github。

### 为什么走橘子加速时"前面能动、最后一步卡住报错"

`git clone over HTTPS` 分两段，性质不同：
```
第1段 ref发现  GET .../info/refs    ← 几KB，一问一答，极快  → 所以"看起来橘子加速能用"
第2段 拉 pack  POST git-upload-pack ← 整个仓库几十~几百MB，长连接持续传输 → 卡在这
```
走橘子加速的完整路径：`git → 橘子加速 → 翻墙海外节点 → 真实 github`，是 HTTP/2 长连接 + 高延迟 + 节点限速。

- 第1段小数据瞬间完成（握手成功，造成"代理能用"的错觉）
- 第2段大流量长连接在翻墙链路上崩，典型报错：
  - `RPC failed; curl 92 HTTP/2 stream was not closed cleanly`
  - `early EOF` / `fetch-pack: unexpected disconnect`
  - `Recv failure` / `GnuTLS recv error`
- 两个叠加根因：
  1. **HTTP/2**（日志确认 `using HTTP/2`）：git 大包 + 代理 + HTTP/2 流控，在高延迟链路上易在收尾时 stream 卡死（git over proxy 经典坑）
  2. **翻墙节点**对长时间大流量传输限速/超时断流，越到最后越易断

### 核心结论：拉 github 根本不需要橘子加速

因为 `github.com → 30.100.0.7` 是字节内网镜像网关，实测**绕过橘子加速、直接走飞连内网网关 clone 成功且稳定**。橘子加速那条翻墙路是多此一举且更不稳。

### 解决办法（按推荐度）

```bash
# ① 最省心：只让 github 不走橘子加速 → 走飞连内网网关(稳)，其它 HTTPS 仍走代理
git config --global http.https://github.com/.proxy ""

# ② 直接用 SSH(走飞连内网网关)
git clone git@github.com:owner/repo.git

# ③ 若某些仓库必须翻墙拉：关 HTTP/2 + 加大缓冲，配合浅克隆
git config --global http.version HTTP/1.1
git config --global http.postBuffer 524288000
git clone --depth 1 https://github.com/owner/repo.git
```

### 排查 git 走向的命令

```bash
git config --global --get-regexp 'http.*proxy'      # git 代理配置
env | grep -iE 'http_proxy|https_proxy|all_proxy'   # 代理环境变量
dscacheutil -q host -a name github.com              # github 解析到哪(30.x=内网)
route -n get <ip> | grep interface                  # 该 IP 走哪个网卡
ssh -T git@github.com                               # 测 SSH 通路(走飞连)
GIT_CURL_VERBOSE=1 git ls-remote https://github.com/<repo> # 看 HTTPS 是否走代理/HTTP版本
```

---

## 原文链接

> 无外部链接，来源为本机 macOS 实测排查（用户：bytedance，平台 darwin）。
