# 字节小程序/passport 登录态(Session/uid)解析机制

> 来源：基于 user_order_manager 服务源码排查 + Session SDK 接入指南(Lark wiki: wikcnD3xPbjxMApmzu39H9dwASv)
> 记录时间：2026-06-15

---

## 核心观点

后端服务拿用户 uid 的方式分"推荐"和"不推荐"两类，**判断是否不推荐的唯一硬指标:服务端有没有自己 RPC 调 `toutiao.passport.session_v2.Get` 把 sessionid 换成 uid**。推荐方式是网关在边缘用短票解析好身份、以元数据形式注入 ctx，后端只读结果；不推荐方式(即将下线)是后端自己拿 sessionid 再 RPC 反查 passport。

---

## 要点整理

### 1. 四种登录态解析方式分类

| 方式 | 推荐? | 关键特征 / 代码 | 额外 RPC |
|---|---|---|---|
| 网关 SessionLoader 注入(AGW/Janus/JanusMini) | ✅ | `common_params.GetSession(ctx)` → `session.UserID`；kitex 服务靠网关把登录态注入 Request | 否(短票反解) |
| SessionLib SDK(短票反解，**仅 http/非 rpc**) | ✅ | `session.GetRequestSessionWithContext`；⚠️ **不支持 rpc(KiteX) 服务** | 否 |
| RPC方式·Cookie 取 sessionid | ❌ | 读 Cookie `sessionid`→`sessionid_ss`→`sid_tt` + 调 `session_v2.Get` | 是 |
| RPC方式·header 取 sessionid | ❌ | 读 `x-tma-host-sessionid` → `GetUidBySessionID` → `rpcGetSessionData` → `session_v2.Get` | 是 |

**易混点(两个不同的 header):**
- 网关注入 header(janus `Tt-Agw-Loader-Session-Rsp`、`Rpc-Persist-Bytetim-User-Uid`)装的是**已解析好的 session/uid** → 推荐
- `x-tma-host-sessionid` 装的是**原始 sessionid 字符串(没解析)** → 是 RPC 方式的输入 → 不推荐

`x-tma-host-sessionid` 不是独立的第三种方式，它只是"sessionid 来源"，拿到后仍要 RPC 调 session_v2 —— 本质就是 RPC 方式的 header 变体。真正的对立是「网关/短票」vs「自己 RPC 调 session_v2」。

### 2. RPC 方式为什么不推荐(文档原因)

1. **用不上短票 X-Tt-Token** —— 短票能在业务端直接反解 Session，省掉 RPC、降延时、避免跨机房 Session 同步延迟导致取不到。
2. **App 端 Cookie 有丢失/不一致风险**。
3. **Cookie name/优先级可能因安全改造变化**，自己读 Cookie 不受 Session 系统监控，有掉线隐患。

文档定义的标准 RPC 方式两步:① 从 Cookie 按 `sessionid`→`sessionid_ss`→`sid_tt` 取 sessionid(隔离域名加后缀如 `sessionid_ads`)；② 调 `toutiao.passport.session_v2` 的 `Get`(default 集群)，返回 SessionData JSON 取 `_spipe_user_id`。注意 StatusCode 非 0 且不在 [132096,132103,132105] 时按服务异常处理，不要当非登录态(否则抖动会误判掉线)。

### 3. 网关 SessionLoader 注入的端到端机制

```
① 客户端 HTTP 请求(带 X-Tt-Token 短票 / Cookie)
② AGW/Janus 网关 SessionLoader 在边缘:
   - 用短票/cookie 解析出 SessionInfo
   - 序列化成 gateway session token
   - 注入为 RPC 元数据:
       Tt-Agw-Loader-Session-Rsp       = <序列化 token>
       Tt-Agw-Loader-Session-Available = 1
       rpc-persist-bytetim-user-uid    = <uid>   (curl 里的 Rpc-Persist-*)
③ 网关作为 RPC client 调后端 kitex 服务，元数据随 TTHeader 传输
④ kitex server framework 读传输层 KV → context.WithValue(ctx, ctxKey, node) 存进 ctx
⑤ handler 只拿到 ctx，但数据已在 ctx 的 metainfo node 里
```

后端"反解"代码链:
```
utils.GetUidByContext(ctx)
  → common_params.GetSession(&KitexContext{ctx})   // key="Session"
      janusRspKey = "Tt-Agw-Loader-Session-Rsp"
      val = KitexContext.GetHeader(janusRspKey)
          → metainfo.GetValue(ctx, "Tt-Agw-Loader-Session-Rsp")
          → getNode(ctx).transient 查到序列化 token
  → token.DeserializeGatewaySessionToken(val) → SessionInfo{UserID}
  → return session.UserID
```
所以"反解"不是后端解析 cookie/短票，而是从 ctx 取出网关已解析好的 token 反序列化。

### 4. metainfo / 元数据 / context / header 的结构关系

**关键认知:`context.Context` 不只是取消信号，它的 `Value(key)` 是一条 request-scoped KV 通道，metainfo 寄生在上面。**

- **元数据(metadata)** = 描述请求的附带信息(uid/device/trace/lane/机房)，**不是业务 body**。HTTP 世界装在 header，RPC 世界叫 metadata/透传 KV。
- **KV 对** = `{key,val}`，是 header / metainfo / 网关注入数据的统一形态。
- **metainfo** = gopkg 的机制，在调用链携带元数据并存在 ctx 里。语义:`persistent`(全链路，HTTP 前缀 `rpc-persist-`)、`transient`(下一跳，`rpc-transit-`)、`stale`(上游传入的瞬态)。

真实结构体:
```go
type http.Header map[string][]string          // header 本质是 map 形态 KV
type http.Request struct { Header http.Header; Body io.ReadCloser; ... }  // header=元数据, body=业务数据
type gin.Context struct { Request *http.Request; ... }   // 实现了 context.Context, 读 header 走 c.Request.Header.Get

// metainfo 存储(github.com/bytedance/gopkg/cloud/metainfo)
type kv struct { key, val string }
type node struct { persistent []kv; transient []kv; stale []kv }
type ctxKeyType struct{}                       // 私有 key
// 存: context.WithValue(ctx, ctxKeyType{}, node)
// 取: GetValue(ctx,k) → getNode(ctx) → search(node.transient/stale, k)

// janus 包装(janus_sdk/consts/const.go)
type KitexContext struct { context.Context }   // 内嵌 ctx
func (c *KitexContext) GetHeader(k string) string {
    v,_ := metainfo.GetValue(c, textproto.CanonicalMIMEHeaderKey(k)); return v
}
type ProxyContext interface { GetHeader(string) string; GetContext() context.Context }
```

**关系总结:**
- `gin.Context` **is-a** `context.Context`(实现接口)，内部 **has-a** `*http.Request`，元数据在 `Request.Header`(map)。
- `KitexContext` **has-a** `context.Context`(内嵌)，元数据在 ctx 的 metainfo `node` 里。
- 二者都实现 `ProxyContext`，上层 `common_params` 代码与"元数据来源"解耦 —— 同一套代码 HTTP/RPC 通用。
- HTTP header ↔ ctx metainfo 通过前缀约定 + `metainfo.FromHTTPHeader`/`ToHTTPHeader` 互转:剥 `rpc-persist-`/`rpc-transit-` 前缀，`abc-def`→`ABC_DEF`(CGI 变量格式)，塞进/写出 node。
- curl 里的 `Rpc-Persist-Bytetim-User-Uid` = 一条 metainfo persistent KV 用 HTTP header 承载的样子。

| 服务类型 | ProxyContext 实现 | GetHeader 读哪 | 元数据物理位置 |
|---|---|---|---|
| HTTP/ginex | gin 包装 | `c.Request.Header.Get(k)` | `http.Request.Header` |
| RPC/kitex | `KitexContext` | `metainfo.GetValue(ctx,k)` | ctx 里的 `metainfo.node` |

### 5. 实践印证(user_order_manager)

`utils/session.go` `getAwemeUidByContext`:
- 主路径 `common_params.GetSession`(session.go:45) → `session.UserID`(✅ 网关短票)，`session.go:51-54` 非 0 直接 return。
- 兜底 `x-tma-host-sessionid` → `ma_common_passport.GetUidBySessionID`(session.go:63-67) = ❌ RPC 方式 header 变体，仅 `UserID==0` 触发。
- 实测 3 条真实 curl(带 Cookie/空 Cookie/换账号)均无 `x-tma-host-sessionid`、均带 `X-Tt-Token`，走网关路径，RPC 兜底零命中。
- 结论:QueryUserOrder 对"RPC方式获取session"无实质依赖，兜底分支可按下线计划清理，**清理前先在 session.go:67 加埋点确认线上命中量为 0**。
- 排查其他服务用 skill `session-auth-audit`。

---

## 原文链接

> 仅存于本地笔记

- Session SDK 接入指南: https://bytedance.larkoffice.com/wiki/wikcnD3xPbjxMApmzu39H9dwASv
- Session Oncall 手册: wikcngyBnV45cRYtxHjLPA3LDqf
- 相关代码: code.byted.org/iesarch/janus_sdk(common_params/consts)、code.byted.org/developer/ma_common/ma_common_passport、github.com/bytedance/gopkg/cloud/metainfo、code.byted.org/ucenter/user_identity_token_lib
- 排查 skill: ~/.claude/skills/session-auth-audit/
