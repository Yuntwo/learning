# 千亿级点赞系统服务架构设计

> 来源：哔哩哔哩技术（Bilibili Opus）
> 原文发布：2023-02-03
> 记录时间：2026-07-01

---

## 核心观点

点赞系统看起来只是一次“+1”，但真正难的是在高频读写、热点集中、海量存储、跨机房容灾这些约束下，同时保证用户体验基本无感。文章给出的解法很典型：**用记录与计数分离的数据模型承接业务语义，用 TiDB / Redis / LocalCache / KV 形成多层存储，用异步链路和聚合写削峰，用多机房与多副本做兜底。**

---

## 要点整理

### 1. 点赞系统不只是“计数器”

以稿件为例，点赞服务至少要支持：
- 点赞 / 取消点赞
- 点踩 / 取消点踩
- 单个或批量查询点赞状态
- 查询某实体的点赞数
- 查询某用户的点赞列表
- 查询某实体的点赞人列表
- 查询某用户收到的总点赞数

也就是说，点赞系统通常同时维护两类核心数据：
- **点赞记录**：谁在什么时候，对哪个实体，做了什么动作
- **点赞计数**：某个实体当前累计的点赞 / 点踩总数

这也是为什么它不能只靠一个简单计数器完成。

### 2. 真正的压力点：高读写、热点、海量数据、未知灾难

文中给出的量级非常直观：
- 全站读流量（状态查询、点赞数查询等）超过 **300k**
- 写流量（点赞、点踩等）超过 **15k**
- 点赞数据存储规模超过 **千亿级**

这会带来四类典型问题：
1. **高频写入压力**：每次点赞都会影响状态和计数
2. **热点问题**：超级热门稿件会把流量打到同一个分片、同一个缓存 key
3. **存储成本问题**：历史点赞记录规模极大，关系型数据库成本高
4. **故障传播问题**：DB、Redis、消息队列、机房、binlog 任一处异常都可能影响用户体验

### 3. 整体架构：五层拆分

文章把系统拆成五部分：
1. **流量路由层**：决定请求进入哪个机房
2. **业务网关层**：统一鉴权、风控、基础流量筛选
3. **点赞服务层**：提供统一 RPC 能力，对外暴露业务接口
4. **异步任务层**：负责写入持久层、刷新缓存、同步消息
5. **数据层**：DB、KV、Redis、本地缓存等

这类拆分的关键价值在于：**把“同步用户体验”和“异步持久化 / 扩散”解耦**。用户先拿到响应，重任务通过异步链路慢慢摊平。

### 4. 三层数据存储：持久化、热点承接、冷数据迁移

#### 4.1 DB 层：TiDB 负责持久化与回源

核心表有两张：
- `likes`：保存点赞记录，按 `mid + message_id` 等维度建立联合索引
- `counts`：保存实体聚合计数，以 `business_id + message_id` 作为主键

作用主要有两个：
- 作为最终持久化账本
- 当缓存失效时提供回源查询

#### 4.2 Redis 层：承接高频读写

文中给了两个很典型的 Redis 模型：

**点赞数缓存**
```text
count:pattern:{business_id}:{message_id} -> {likes},{disLikes}
```
- key 对应某业务下某实体
- value 保存点赞数 / 点踩数
- 适合高频读取，也适合做快速增量更新

**用户点赞列表**
```text
user:likes:pattern:{mid}:{business_id} -> ZSET(message_id, likeTimestamp)
```
- key 按用户维度切分
- value 用 ZSet 保存该用户最近点赞过的实体列表
- `score` 用点赞时间，便于按时间倒序拉取
- 为防止无限增长，缓存层会按固定长度裁剪，多余数据回源 DB

#### 4.3 LocalCache：应对热点 key

当某个实体极热时，即使 Redis 也可能出现热点。文中的做法是：
- 在可配置时间窗口内统计访问最频繁的 key
- 识别 hot key 后，把结果放进本地内存
- 给本地缓存设定业务可接受的 TTL

这相当于在 Redis 之前再加一层更近的缓存，专门用来顶“瞬时爆发”。

#### 4.4 历史数据迁移到 KV

关系型数据库存放海量历史点赞记录成本较高，且查询模式其实更偏 key-value。文章因此把一部分历史数据迁移到 KV 存储：
- 点赞记录按组合 key 编码
- 用户点赞列表索引按时间维度编码
- 实体维度点赞记录索引按 message_id 维度编码

核心思路是：**热数据留在高性能层，冷数据迁移到更适合长期存放、可水平扩展的 KV 层。**

### 5. 写路径优化：异步化、聚合写、重试补偿

文章提到几个非常实用的优化策略：

#### 5.1 计数聚合写

为了减少 DB I/O，点赞数不会每次都直接刷库，而是做短窗口聚合，例如：
- 聚合 10 秒内的点赞数变化
- 再一次性写入数据库

这在“高频 +1/-1”场景特别有效，因为计数比记录更适合做批量合并。

#### 5.2 写入异步化

数据库写入做全面异步化，让 DB 以合理速率处理请求，避免瞬时写爆。配合异步任务层，可以完成：
- 点赞行为落库
- 点赞状态缓存刷新
- 点赞列表缓存刷新
- 点赞数缓存刷新
- 向下游分发点赞事件和计数变更消息

#### 5.3 纠错与重试

为了保证状态正确性，更新前会取出旧状态作为依据；同时重要写入链路会做重试，关键数据甚至可无限重试。这里体现的是一个很务实的工程原则：**允许短暂不一致，但要有持续纠偏能力。**

### 6. 容灾设计：多机房、多副本、兜底返回

点赞系统属于用户强感知功能，因此故障设计重点不是“绝对零损”，而是“用户尽量无感”。文中的容灾策略主要包括：

#### 6.1 DB 多机房灾备
- 两地机房互为灾备
- 正常情况下一个机房承担全部写流量和部分读流量，另一个机房承担部分读流量
- 当主机房 DB 出故障时，通过代理切换把读写流量转到备机房

#### 6.2 Redis 双集群 + 缓存同步
- 不同机房部署两套 Redis 集群
- 通过 binlog 消费维护缓存一致性
- 切机房时尽量避免冷启动把流量压回 DB

#### 6.3 多层存储互为兜底
- 热数据在 Redis
- 全量数据在 KV
- TiDB 作为最终账本仍保存全量数据
- 当缓存、KV、DB 某一层异常时，由其他层和限流机制兜底

#### 6.4 接口降级

对点赞数、点赞状态、点赞列表等核心接口，即便全链路兜底失败，也尽量不直接把错误暴露给用户，而是：
- 返回空值或默认态
- 等服务恢复后再补写故障期间的数据

这说明在强交互业务里，**“先保交互，再保最终一致”** 往往比同步强一致更重要。

### 7. binlog 不可靠时，业务消息做旁路容灾

文中提到 TiDB 的 binlog 在生产环境中可能出现延迟甚至断流。为减少对下游的影响，系统做了两件事：
- 监控 binlog 的实时性和断流情况
- 当出现延迟时，由点赞服务直接发送关键容灾消息（点赞状态变化、点赞数变化）给下游

这很值得借鉴：**不要把关键链路完全押宝在单一同步通道上。** 能有旁路就尽量有旁路。

---

## 可迁移的设计启发

把这篇文章抽象一下，可以总结成 4 条适用于大多数互动系统的经验：

1. **记录和计数一定要拆开建模**
   - 记录是审计和回放依据
   - 计数是高频查询结果
   - 二者访问模式完全不同，不应混在一起

2. **热点问题要单独设计，不要指望普通缓存自动解决**
   - 热 key 识别
   - 本地缓存兜底
   - 分桶、聚合、限流都是必要手段

3. **用户链路同步返回，重操作异步化**
   - 写缓存、刷持久层、发消息最好解耦
   - 让慢链路不要拖垮主交互链路

4. **允许有边界的最终一致，但必须具备修正机制**
   - 重试
   - 对账
   - 旁路补偿
   - 降级恢复后的补写

---

## 代码 Demo：用 Redis + Lua 做一个简化版“点赞状态 + 点赞计数 + 用户点赞列表”

下面这个示例不是完整生产实现，但它很好地对应了文章里的三个核心数据：
- 用户是否点赞过（状态）
- 某实体点赞总数（计数）
- 某用户最近点赞列表（ZSet）

### Lua 脚本：原子执行点赞 / 取消点赞

```lua
-- KEYS[1] = like:state:{biz}:{user}:{item}
-- KEYS[2] = like:count:{biz}:{item}
-- KEYS[3] = user:likes:{user}:{biz}
-- ARGV[1] = action(like|unlike)
-- ARGV[2] = timestamp
-- ARGV[3] = itemId

local action = ARGV[1]
local current = redis.call('GET', KEYS[1])

if action == 'like' then
  if current == '1' then
    return 0 -- 已点赞，幂等返回
  end
  redis.call('SET', KEYS[1], '1')
  redis.call('HINCRBY', KEYS[2], 'likes', 1)
  redis.call('ZADD', KEYS[3], ARGV[2], ARGV[3])
  return 1
end

if action == 'unlike' then
  if current ~= '1' then
    return 0 -- 本来就没点赞
  end
  redis.call('SET', KEYS[1], '0')
  redis.call('HINCRBY', KEYS[2], 'likes', -1)
  redis.call('ZREM', KEYS[3], ARGV[3])
  return -1
end

return -2 -- 未知动作
```

### Go 调用示例（`go-redis`）

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

var ctx = context.Background()

const likeScript = `
local action = ARGV[1]
local current = redis.call('GET', KEYS[1])

if action == 'like' then
  if current == '1' then
    return 0
  end
  redis.call('SET', KEYS[1], '1')
  redis.call('HINCRBY', KEYS[2], 'likes', 1)
  redis.call('ZADD', KEYS[3], ARGV[2], ARGV[3])
  return 1
end

if action == 'unlike' then
  if current ~= '1' then
    return 0
  end
  redis.call('SET', KEYS[1], '0')
  redis.call('HINCRBY', KEYS[2], 'likes', -1)
  redis.call('ZREM', KEYS[3], ARGV[3])
  return -1
end

return -2
`

func main() {
    rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})

    bizID := "archive"
    userID := "10001"
    itemID := "987654321"
    ts := time.Now().UnixMilli()

    keys := []string{
        fmt.Sprintf("like:state:%s:%s:%s", bizID, userID, itemID),
        fmt.Sprintf("like:count:%s:%s", bizID, itemID),
        fmt.Sprintf("user:likes:%s:%s", userID, bizID),
    }

    n, err := rdb.Eval(ctx, likeScript, keys, "like", ts, itemID).Int()
    if err != nil {
        panic(err)
    }

    likes, _ := rdb.HGet(ctx, keys[1], "likes").Int64()
    items, _ := rdb.ZRevRange(ctx, keys[2], 0, 9).Result()

    fmt.Printf("like result=%d likes=%d recent=%v\n", n, likes, items)
}
```

### 可直接运行的 companion demo

如果想直接运行一个**无需本地 Redis** 的版本，可以执行：

```bash
go run ./notes/billion_scale_like_system_demo
```

这个 companion demo 用内存 + `sync.Mutex` 模拟 Redis 单线程执行 Lua 脚本的原子语义，方便在本地直接观察：
- 点赞状态如何保证幂等
- 点赞计数如何与状态保持一致
- 用户最近点赞列表如何一起更新与裁剪
- 为什么这三类数据必须作为一个原子单元处理

因此，笔记中的 Lua / `go-redis` 代码更偏**生产接入示意**，而 `notes/billion_scale_like_system_demo/main.go` 则是**本地可运行教学版**。

### 这个 Demo 对应了文章中的哪些点？

- **状态和计数分离**：`like:state` 记录用户是否点赞；`like:count` 记录聚合数
- **列表缓存单独存**：`user:likes` 用 ZSet 保存最近点赞列表
- **原子性**：点赞 / 取消点赞在 Lua 内一次完成，避免“状态更新了但计数没变”的竞态
- **可扩展性**：生产上可进一步叠加裁剪用户列表、异步刷库、消息分发、对账补偿

如果把这份 Demo 再往工程化方向推进，一般会继续补上：
- 用户点赞列表长度裁剪（如 `ZREMRANGEBYRANK`）
- 点赞数聚合写 DB
- MQ 异步分发点赞事件
- 失败重试与离线对账
- 热 key 本地缓存 / 分桶治理

---

## 实践印证

结合当前仓库里已有的示例代码，这篇文章里的几个思想是能对应上的：

1. **Redis + Lua 原子语义**
   - `hotrow/redislua/main.go` 演示了用 Lua 保证“读 → 判断 → 扣减”整体原子
   - 我已实际运行 `go run ./hotrow/redislua`，输出显示：单 key 和分桶方案都满足 `售出 + 剩余 == 初始库存`，说明原子更新和最终对账逻辑是成立的
   - 把库存键换成点赞状态 / 点赞计数键，本质上就是同一类问题：把热点写操作压在内存原子执行层里

2. **go-redis 基础接入**
   - `framework/go-redis/main.go` 提供了最基础的 `Set` / `Get` 示例
   - 如果要把上面的点赞 Demo 变成可运行版本，可以直接在这个示例基础上加入 `Eval` 调用

3. **热点治理思路一致**
   - 仓库里的 `hotrow/README.md` 和多个 demo 都在讨论“热点更新、分桶、Redis 原子脚本、热点键冲击”这些问题
   - 虽然业务场景不同，但技术本质与点赞系统的热点 key、热点实体、聚合写入是相通的

---

## 原文链接

> 仅存于本地笔记，发布时不带出

- 原文：https://www.bilibili.com/opus/758312609901445367
- 标题：【点个赞吧】 - B站千亿级点赞系统服务架构设计
- 作者：哔哩哔哩技术
