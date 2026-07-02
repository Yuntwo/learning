# 热点检测治理

> 来源：https://mp.weixin.qq.com/s/C8CI-1DDiQ4BC_LaMaeDBg?spm_id_from=333.1369.0.0
> 记录时间：2026-07-01

---

## 核心观点

热点治理不能只靠“远端缓存扛流量”，而要把**热点检测**和**本地缓存治理**组合起来：先在流式访问中识别真正的 TopK / 突发热点，再只让热点数据进入本地缓存，才能在有限内存下显著降低 Redis / DB 的回源压力。

---

## 要点整理

### 1. 问题背景：缓存本身也会被热点打崩

常见架构是“数据库 + 分布式缓存”。在普通流量下，这套架构足够好用，因为访问通常满足二八分布：少量热点数据承载了大多数请求。

但到了热门活动、超级热点或突发流量场景，问题会立刻暴露：

- 某些缓存分片被集中打热
- 靠加副本分流成本高，且应对突发热点不及时
- 热点如果事先不可预测，很容易直接把远端缓存打穿

所以核心问题不是“要不要缓存”，而是：

1. 能否**实时识别热点**
2. 能否让**真正的热点优先驻留本地**
3. 能否对**突发热点足够敏感**

### 2. 原有方案为什么不够

文章先回顾了两类常见做法：

#### 小表广播

把小而热的数据全部放到每个实例的本地缓存里，再靠定时更新或推送刷新。

优点：
- 对低变更、高访问的数据很有效

缺点：
- 每个业务都要自己实现 local cache
- 时效性取决于轮询 / 推送机制
- 适用范围偏窄，不适合动态热点

#### 主动预热

先统计高流量对象（如直播间、视频），再把 TopK 数据主动推给应用实例预热本地缓存。

优点：
- 能拦截一部分可预测的热点访问

缺点：
- 准实时统计仍然有延迟
- 对突发热点不够敏感
- 推送链路和缓存逻辑仍要业务自己接

### 3. 核心方案：热点检测 + 本地缓存

作者给出的治理框架把两个模块直接集成进缓存 SDK：

- **热点检测模块**：从访问流中识别热点 key
- **本地缓存模块**：只让热点 key 进入本地 LRU

这样做的收益很直接：

- 可以监控业务中的热点访问
- 可以更快识别突发热点
- 可以在同样内存开销下提升本地命中率
- 支持白名单，把可预知的活动 key 提前纳入本地缓存
- 对业务接入透明，只需要开配置

本质上，这是把“缓存是否准入”从无差别缓存，升级成了**热点感知缓存**。

### 4. 流式 TopK 识别：三类算法对比

文章把热点检测问题抽象为：**如何在流式数据中做 TopK 统计**。

#### Lossy Counting

思路是把数据流切成多个窗口：

- 统计每个元素的频率
- 每轮结束后所有计数减 1
- 删除计数归零的元素

优点：
- 内存可控
- 通过持续淘汰低频元素，保留近似高频项

缺点：
- 更偏“近似频次统计”
- 对实时突发热点的响应不是最优

#### Count-Min Sketch

核心结构：

- 一个 `d x w` 的二维计数数组
- `d` 个独立哈希函数

查询某个元素时：

- 把该元素映射到每一行对应的位置
- 取这些位置中的最小值作为频率估计

优点：
- 内存效率高
- 查询频率快

问题：
- 哈希冲突会把频次估大
- 还需要额外用最小堆维护 TopK

#### HeavyKeeper

HeavyKeeper 继承了 Sketch 类结构，但做了关键改进：

- 每个槽位不只记录 count，还记录哈希指纹
- 遇到冲突时，不是简单 `+1`
- 而是使用概率衰减：`Pdecay = b^C, b < 1`

它的直觉很漂亮：

- 低频元素容易被快速衰减掉
- 高频元素更不容易被冲掉
- 因而在同样内存下，对热点识别更稳定、更精确

文章结论是：**在相同内存开销下，HeavyKeeper 的统计精度更优，因此最终选它实现热点检测。**

### 5. SDK 侧接口设计非常克制

文中把 TopK 检测接口收敛成三个能力：

```go
// Topk algorithm interface.
type Topk interface {
    // Add item and return if item is in the topk.
    Add(item string, incr uint32) bool

    // List all topk items.
    List() []Item

    // Expelled watch at the expelled items.
    Expelled() <-chan Item
}
```

这三个接口分别解决：

- `Add`：处理一次访问并判断是否进入热点集合
- `List`：导出当前热点列表
- `Expelled`：感知哪些 key 被挤出了热点集合

`Expelled` 很关键，因为本地缓存不仅要“进”，还要“退”。热点已经过期时，需要把旧热点从本地缓存中移走，才能保持缓存新鲜度。

### 6. 性能优化：避免指数运算打爆 CPU

HeavyKeeper 的问题在于：冲突时要算指数衰减概率 `b^C`。如果 `C` 很大，这种运算会引入明显 CPU 开销。

文中的工程化优化思路有两个：

#### 计数上限截断

- 取 `b = 0.925`
- 当 `C >= 256` 时，直接把衰减概率近似成 `b^256`

原因是这时概率已经很小，再精确计算收益不大。

#### 查表法

- 预先计算 `b^0 ~ b^255`
- 运行时直接查表，而不是反复做指数计算

这是很典型的线上优化手法：**允许一点数值近似，换明显的 CPU 节省。**

### 7. 为什么要额外引入“统一衰减”来抓突发热点

这是全文最值得记住的部分。

原始 TopK 算法适合统计“长期高频”，但不适合识别“刚刚突然爆发”的热点。

举例：

- 历史上 `a/b/c` 每秒 10 次，持续了 1000 秒
- 它们累计频次都接近 10000
- 第 1001 秒，`d` 突然变成每秒 100 次

如果只看累计频次，`d` 需要积累到超过 10000 才能挤进 Top3，意味着已经过去了 100 秒。对线上系统来说，这个检测速度太慢了。

所以作者做了一个非常有效的增强：

- 每隔 1 秒，把所有元素的统计频次统一衰减一次
- 即 `Ci = Ci / n`，其中 `n > 1`

当 `n = 2` 时，历史热点的累计值会快速收敛到近似 `2x`，也就是“近期访问强度”的量级，而不是长期累计值。

结果是：

- 老热点不会一直靠历史优势霸榜
- 新出现的突发热点能在 1 秒量级内进入 TopK

这是把“长期频次统计”改造成“带时间感知的热点检测”的关键。

### 8. 本地缓存不是简单 LRU，而是“热点准入 + LRU 淘汰”

文章提到本地缓存常见选择是 LRU 或 LFU：

- LFU 对热点命中更好，但实现复杂
- 作者已有独立的热点检测模块，因此选了更易工程化的 **LRU + 热点准入**

核心不是换一种更复杂的淘汰算法，而是在入缓存前先做一道门：

```go
if ok := topk.Add(key, 1); ok {
    lru.Add(key, value, ttl)
}
```

只有被判断为热点的数据，才允许进入本地缓存。

这样做的价值是：

- 低频冷数据不会污染宝贵的本地内存
- 本地缓存容量虽小，但能更聚焦于真正高价值的数据

### 9. 白名单：解决“可预知热点”的第一秒冲击

对活动场景来说，有些热点其实是可预测的，比如活动 ID、专题页 ID、直播间 ID。

因此框架额外支持白名单：

```go
if ok := topk.Add(key, 1); ok {
    lru.Add(key, value, ttl)
    return
}
if ok := inWhileList(key); ok {
    lru.Add(key, value, ttl)
    return
}
```

白名单的作用：

- 活动一开始，请求命中该 key 就立即进入本地缓存
- 不必等热点检测模块累计 enough samples 才识别
- 对“瞬时爆发超级热点”尤其有意义

也就是说：

- **热点检测**解决“未知热点”
- **白名单**解决“已知热点”

两者结合，治理面更完整。

### 10. 集成到缓存 SDK：让业务方无感接入

文中最终把整套能力集成到 Redis SDK 中，大致流程是：

```go
func (r *Redis) Do(ctx context.Context, cmd string, args ...interface{}) (reply interface{}, err error) {
    key := format(cmd, args...)
    if readCmd(cmd) {
        resp, ok := r.getFromLocalCache(key)
        if ok {
            return
        }
        resp, err = r.getFromRemote(cmd, args...)
        r.updateLocalCache(key, resp)
        return
    }

    r.setRemote(cmd, args...)
    if writeCmd(cmd) {
        r.deleteLocalCache(key)
    }
}
```

这里的关键是读路径：

1. 先查本地缓存
2. miss 后回源远端缓存
3. 更新本地缓存时，再结合热点检测 / 白名单决定是否准入

进一步的更新逻辑：

```go
func (r *Redis) updateLocalCache(key, resp) {
    var added bool
    if r.topk != nil {
        added = r.topk.Add(key)
    }
    if added {
        r.lru.Add(key, resp, r.localTTL)
    } else {
        if r.inWhileList(key) {
            r.lru.Add(key, resp, r.localTTL)
        }
    }
}
```

这意味着业务不需要手动实现：

- 热点统计
- 本地缓存策略
- 白名单逻辑
- 热点驱逐逻辑

只要接入 SDK 并打开配置，就能获得热点治理能力。

### 11. 业务效果：几 MB 内存换到可观命中率

文章给出的业务实践数据很有说服力：

- 在热门话题等场景下，仅用几 MB 内存
- 日常高峰期本地缓存命中率可达 **35%**
- 超级热点场景下，本地命中率甚至可达 **85%**

这说明热点治理的收益并不依赖“大本地缓存”，关键在于：

- 热点识别要准
- 本地缓存准入要克制
- 要能快速响应突发流量

---

## 最佳实践 / 注意事项

### 最佳实践

1. **把热点检测做成 SDK 能力，而不是业务定制逻辑**
   - 否则每个业务都会重复造 local cache / 推送 / 预热轮子。

2. **热点检测和本地缓存分层设计**
   - 检测模块负责“发现热点”
   - 本地缓存负责“承接热点”
   - 这样可以分别演进。

3. **对突发热点一定要做时间衰减**
   - 只看累计频次会让历史热点压制新热点。

4. **白名单适合活动、节日、运营位等可预知热点**
   - 对首秒冲击尤其有效。

5. **本地缓存容量小的时候，准入控制比淘汰策略更重要**
   - 与其纠结 LRU / LFU，不如先保证冷数据进不来。

### 注意事项

1. **本地缓存会引入短暂不一致**
   - 对强一致要求极高的场景要谨慎。

2. **热点准入命中的是读场景，不是所有业务都适合**
   - 写多、强事务型场景收益有限。

3. **衰减因子需要结合业务流量节奏调优**
   - 衰减过慢，突发热点不够敏感
   - 衰减过快，热点列表容易抖动

4. **白名单要有治理机制**
   - 过期活动 key 不应长期留在名单里。

---

## 代码 Demo

下面这个 Go demo 不是 HeavyKeeper 的完整实现，而是一个**用于理解文章核心思路的最小可运行版本**：

- 用“统一衰减 + TopK”模拟热点检测
- 用“热点准入 + LRU”模拟本地缓存
- 用白名单模拟活动热点的首秒放行

### 运行方式

```bash
go run hotspot_detection_governance_demo.go
```

### Demo 代码

```go
package main

import (
    "container/heap"
    "container/list"
    "fmt"
    "sort"
    "time"
)

type item struct {
    key   string
    count float64
}

type minHeap []item

func (h minHeap) Len() int            { return len(h) }
func (h minHeap) Less(i, j int) bool  { return h[i].count < h[j].count }
func (h minHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any) { *h = append(*h, x.(item)) }
func (h *minHeap) Pop() any {
    old := *h
    n := len(old)
    x := old[n-1]
    *h = old[:n-1]
    return x
}

type HotspotDetector struct {
    counts    map[string]float64
    decay     float64
    topK      int
    whitelist map[string]struct{}
}

func NewHotspotDetector(topK int, decay float64, whitelist []string) *HotspotDetector {
    w := make(map[string]struct{}, len(whitelist))
    for _, key := range whitelist {
        w[key] = struct{}{}
    }
    return &HotspotDetector{
        counts:    make(map[string]float64),
        decay:     decay,
        topK:      topK,
        whitelist: w,
    }
}

func (d *HotspotDetector) Tick() {
    for key, count := range d.counts {
        count *= d.decay
        if count < 0.5 {
            delete(d.counts, key)
            continue
        }
        d.counts[key] = count
    }
}

func (d *HotspotDetector) Add(key string, incr float64) bool {
    d.counts[key] += incr
    if _, ok := d.whitelist[key]; ok {
        return true
    }
    return d.IsHot(key)
}

func (d *HotspotDetector) IsHot(key string) bool {
    if _, ok := d.whitelist[key]; ok {
        return true
    }
    _, ok := d.topItemsMap()[key]
    return ok
}

func (d *HotspotDetector) topItemsMap() map[string]item {
    h := &minHeap{}
    heap.Init(h)
    for key, count := range d.counts {
        candidate := item{key: key, count: count}
        if h.Len() < d.topK {
            heap.Push(h, candidate)
            continue
        }
        if (*h)[0].count < count {
            heap.Pop(h)
            heap.Push(h, candidate)
        }
    }
    result := make(map[string]item, h.Len())
    for _, it := range *h {
        result[it.key] = it
    }
    return result
}

func (d *HotspotDetector) TopItems() []item {
    items := make([]item, 0, len(d.counts))
    for _, entry := range d.topItemsMap() {
        items = append(items, entry)
    }
    sort.Slice(items, func(i, j int) bool {
        return items[i].count > items[j].count
    })
    return items
}

type cacheEntry struct {
    key   string
    value string
}

type LocalCache struct {
    cap   int
    ll    *list.List
    index map[string]*list.Element
}

func NewLocalCache(cap int) *LocalCache {
    return &LocalCache{cap: cap, ll: list.New(), index: make(map[string]*list.Element)}
}

func (c *LocalCache) Get(key string) (string, bool) {
    if ele, ok := c.index[key]; ok {
        c.ll.MoveToFront(ele)
        return ele.Value.(cacheEntry).value, true
    }
    return "", false
}

func (c *LocalCache) Add(key, value string) {
    if ele, ok := c.index[key]; ok {
        ele.Value = cacheEntry{key: key, value: value}
        c.ll.MoveToFront(ele)
        return
    }
    if c.ll.Len() >= c.cap {
        back := c.ll.Back()
        if back != nil {
            delete(c.index, back.Value.(cacheEntry).key)
            c.ll.Remove(back)
        }
    }
    ele := c.ll.PushFront(cacheEntry{key: key, value: value})
    c.index[key] = ele
}

type CacheSDK struct {
    detector *HotspotDetector
    local    *LocalCache
    remote   map[string]string
}

func NewCacheSDK() *CacheSDK {
    remote := map[string]string{
        "room:1":    "normal-room",
        "room:2":    "other-room",
        "event:618": "flash-sale",
        "video:99":  "viral-video",
    }
    return &CacheSDK{
        detector: NewHotspotDetector(3, 0.5, []string{"event:618"}),
        local:    NewLocalCache(3),
        remote:   remote,
    }
}

func (s *CacheSDK) Read(key string) string {
    if value, ok := s.local.Get(key); ok {
        return "local:" + value
    }
    value := s.remote[key]
    if s.detector.Add(key, 1) {
        s.local.Add(key, value)
    }
    return "remote:" + value
}

func main() {
    sdk := NewCacheSDK()

    traffic := [][]string{
        {"room:1", "room:1", "room:2", "event:618"},
        {"room:1", "room:2", "room:2", "event:618"},
        {"room:1", "room:2", "event:618", "video:99", "video:99", "video:99", "video:99"},
        {"video:99", "video:99", "video:99", "video:99", "video:99", "event:618"},
    }

    for second, keys := range traffic {
        fmt.Printf("\n== second %d ==\n", second+1)
        sdk.detector.Tick() // 模拟对历史频次做统一衰减，提高突发热点识别速度
        for _, key := range keys {
            fmt.Printf("read %-9s -> %s\n", key, sdk.Read(key))
        }
        fmt.Println("topk:")
        for _, it := range sdk.detector.TopItems() {
            fmt.Printf("  %-9s score=%.2f\n", it.key, it.count)
        }
        time.Sleep(150 * time.Millisecond)
    }
}
```

### Demo 可以观察到什么

- `event:618` 在白名单里，因此第一次访问就能被本地缓存承接
- `video:99` 在第三轮突然高频出现，因有统一衰减，能快速成为热点
- 热点进入 TopK 后，再访问会逐步更多地命中本地缓存

如果你想做更贴近原文的版本，可以把这个 demo 继续扩展为：

- 用 Sketch 结构替换当前 map 统计
- 用指纹 + 概率衰减模拟 HeavyKeeper 槽位冲突
- 加上被驱逐热点的回调通道

---

## 原文链接

> 仅存于本地笔记，发布时绝不带出

- 原文：https://mp.weixin.qq.com/s/C8CI-1DDiQ4BC_LaMaeDBg?spm_id_from=333.1369.0.0
- HeavyKeeper 参考实现：https://github.com/go-kratos/aegis/blob/main/topk/heavykeeper.go
- Aegis：https://github.com/go-kratos/aegis
- Kratos：https://github.com/go-kratos/kratos
- Lossy Counting 论文：https://micvog.files.wordpress.com/2015/06/approximate_freq_count_over_data_streams_vldb_2002.pdf
- Count-Min Sketch 论文：http://dimacs.rutgers.edu/~graham/pubs/papers/cmencyc.pdf
- HeavyKeeper 论文：https://www.usenix.org/system/files/conference/atc18/atc18-gong.pdf
