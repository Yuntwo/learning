# Claude Code 接不上 Anthropic 时：用 coconut 外挂模型代理 + 用 Proxyman 抓模型输入输出的调试笔记

> 来源：本机实际配置与排查记录（使用 coconut 作为 Trae 代理工具）
> 记录时间：2026-07-22

---

## 核心观点

当 Claude Code 直连 Anthropic 不可用时，可以通过 **本地模型代理网关** 把请求转到其他兼容模型服务；我这里使用的是 **coconut** 作为 **Trae 代理工具**，本地监听 `127.0.0.1:8787`，再让 Claude Code 用 `ANTHROPIC_BASE_URL` 指向这个本地入口。若还想观察模型请求与响应，不需要把 Proxyman 的监听端口改成 `8787`，而是应保持 Proxyman 监听 `9090`，并让 Claude Code 的流量先经过 Proxyman，再由 Proxyman 转发到 `8787`。

---

## 要点整理

### 1. 适用场景

这个方案适合以下几类情况：

- Claude Code 直连 Anthropic 不稳定、不可达，或当前网络环境无法正常访问。
- 已有一个本地兼容网关，可以把 Anthropic 风格请求转发到其他模型服务。
- 需要观察 Claude Code 发给模型的请求体、响应体、流式返回或报错细节。
- 需要验证本地代理是否真的被命中，还是被系统直连/绕过。

我这里的实际形态是：

- **coconut**：作为 **Trae 代理工具**，本地监听 `127.0.0.1:8787`
- **Claude Code**：通过 `ANTHROPIC_BASE_URL` 指向 `http://127.0.0.1:8787`
- **Proxyman**：本地监听 `127.0.0.1:9090`，用于抓包与转发

### 2. 正确的代理链路

在“情况 1”里，`8787` 不是前向代理端口，而是**本地模型网关/代理服务端口**。因此抓包时不应该把 Proxyman 改成 `8787`，而应该让链路变成：

```text
Claude Code
   │
   ├─ ANTHROPIC_BASE_URL = http://127.0.0.1:8787
   ├─ HTTP_PROXY / HTTPS_PROXY = http://127.0.0.1:9090
   │
   ▼
Proxyman :9090
   │
   ▼
coconut / Trae 代理 :8787
   │
   ▼
目标模型服务
```

核心判断标准只有一句：

> **只要 Claude Code 的流量先到了 Proxyman，Proxyman 就能抓到；端口是否和 8787 相同并不重要。**

### 3. 推荐配置方式

推荐把相关变量写到 Claude Code 的 settings `env` 中，而不是只写在当前 shell 里。这样无论是当前会话还是后续拉起的 agent，行为都会更一致。

可参考的配置示例：

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:8787",
    "HTTP_PROXY": "http://127.0.0.1:9090",
    "HTTPS_PROXY": "http://127.0.0.1:9090"
  }
}
```

如果你习惯在 shell 环境里临时测试，也可以先用下面这种方式验证：

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export HTTP_PROXY=http://127.0.0.1:9090
export HTTPS_PROXY=http://127.0.0.1:9090
claude
```

常见放置位置：

- 用户级：`~/.claude/settings.json`
- 项目级：`./.claude/settings.local.json`

### 4. NO_PROXY 是最容易忽略的坑

如果设置了 `NO_PROXY`，一定要检查里面**不要包含**以下值：

- `127.0.0.1`
- `localhost`

原因是：

- 你的 `ANTHROPIC_BASE_URL` 指向的是 `127.0.0.1:8787`
- 一旦 `127.0.0.1` 或 `localhost` 在 `NO_PROXY` 里
- 那么这段请求就会**直接绕过 Proxyman**
- 最终表现就是：模型调用能成功，但 Proxyman 里看不到任何流量

一个容易踩坑的错误示例：

```bash
export NO_PROXY=127.0.0.1,localhost
```

在这个例子里，请求本地 `8787` 时通常不会走 `9090`，因此抓不到。

### 5. HTTPS 抓包与证书注意事项

如果你要抓的是 HTTPS 请求，仅仅把代理端口配对还不够，还要保证 Proxyman 的证书在系统中被信任。否则常见结果是：

- 只能看到 CONNECT，不能看到明文请求体/响应体
- 或直接出现 TLS 握手失败
- 或报证书校验错误

建议检查：

1. Proxyman CA 证书是否已经安装并被系统信任。
2. Claude Code 进程是否能使用系统 CA。
3. 若仍然报证书错误，可显式补充：

```bash
export NODE_EXTRA_CA_CERTS=/path/to/proxyman-ca.pem
```

适用情形：

- 系统已导入证书，但 Node 进程仍提示证书链不被信任
- 抓 HTTPS 明文时出现 `self signed certificate`、`SSL certificate verification failed` 一类报错

不建议为了抓包而关闭 TLS 校验。能用受信 CA 解决时，优先走受信 CA 方案。

### 6. 我实际采用的工作模式

我这里的调试思路可以拆成两层：

#### 模型转发层

- 使用 **coconut** 作为 **Trae 代理工具**
- 在本地暴露 Anthropic 风格入口：`127.0.0.1:8787`
- Claude Code 通过 `ANTHROPIC_BASE_URL` 连接它

#### 抓包观测层

- 使用 **Proxyman** 监听 `127.0.0.1:9090`
- Claude Code 的 `HTTP_PROXY` / `HTTPS_PROXY` 指向 `9090`
- Proxyman 再把请求转发到 `8787`

这样就能同时满足：

- Claude Code 仍然使用 Anthropic 风格调用链
- 实际模型请求被转发到其他服务
- 中间链路可观测，可调试输入输出与错误细节

### 7. 一套最小排障 checklist

如果配置后仍然抓不到包，按下面顺序查：

#### 第一步：确认 8787 真有服务在监听

```bash
lsof -nP -iTCP:8787 -sTCP:LISTEN
```

如果没有监听，说明 coconut / Trae 代理本身没有正常启动。

#### 第二步：确认 Proxyman 的 9090 正在监听

```bash
lsof -nP -iTCP:9090 -sTCP:LISTEN
```

如果 9090 没有被 Proxyman 占用，说明抓包入口本身没起来。

#### 第三步：确认 Claude Code 环境变量是否生效

重点确认：

- `ANTHROPIC_BASE_URL`
- `HTTP_PROXY`
- `HTTPS_PROXY`
- `NO_PROXY`

尤其注意：

- `ANTHROPIC_BASE_URL` 应为 `http://127.0.0.1:8787`
- `HTTP_PROXY` / `HTTPS_PROXY` 应为 `http://127.0.0.1:9090`
- `NO_PROXY` 不应包含 `127.0.0.1` 或 `localhost`

#### 第四步：确认请求有没有先到 Proxyman

如果模型能正常返回，但 Proxyman 看不到流量，优先怀疑：

- `NO_PROXY` 把本地地址绕过了
- 变量只在某个 shell 里生效，但 Claude Code 不是从那个环境启动的
- settings 中的 `env` 与当前 shell 变量冲突

#### 第五步：确认是否是 HTTPS 证书问题

如果能看到连接尝试，但请求失败，优先检查：

- Proxyman 证书是否已信任
- 是否需要显式设置 `NODE_EXTRA_CA_CERTS`
- 是否存在 TLS 握手失败或证书链错误

### 8. 经验结论

这类问题里，最容易犯的误区是把“Proxyman 的监听端口”和“业务网关的端口”混为一谈。

正确理解应该是：

- `8787` 负责**真正处理模型兼容请求**
- `9090` 负责**拦截和观察请求**
- Proxyman 不需要和 `8787` 用同一个端口
- 只要流量顺序是“Claude Code → Proxyman → 8787”，抓包目标就达成了

---

## 实用命令备忘

```bash
# 查看 8787 是否监听
lsof -nP -iTCP:8787 -sTCP:LISTEN

# 查看 9090 是否监听
lsof -nP -iTCP:9090 -sTCP:LISTEN

# 查看代理相关环境变量
env | grep -iE 'ANTHROPIC_BASE_URL|HTTP_PROXY|HTTPS_PROXY|NO_PROXY'

# 临时补充 Proxyman 证书
export NODE_EXTRA_CA_CERTS=/path/to/proxyman-ca.pem
```

---

## 原文链接

> 无外部链接，来源为本机实际配置与排查记录；场景为 Claude Code 通过 coconut（Trae 代理工具）转发 Anthropic 风格请求，并借助 Proxyman 调试模型输入输出。
