# Go 数据同步/清洗 Job 的可复用控制模板

> 来源：项目实战沉淀（wallet_trade_payment_core / jobs/wallet_biz_identity）
> 记录时间：2026-06-30

---

## 核心观点

一个"批量数据清洗/同步" Job 的可靠骨架可以抽象成三层：**动态限流（配置中心热更新 QPS）+ 进度聚合（atomic 计数 + 可视化卡片秒级刷新）+ 同步流程（批外串行、批内并行、批内末尾栅栏等待）**。三者解耦，新写同类 Job 直接套。

---

## 要点整理

### 1. 动态限流 limiter —— 令牌桶 + 后台秒级热更新 QPS

基于 `golang.org/x/time/rate` 令牌桶。关键在于**后台 goroutine 每秒从配置中心拉 TPS 并热更新**，改配置不用重启任务。

```go
type limiter struct {
    TccConfigKey string
    tps          int64
    limiter      *rate.Limiter
}

func NewLimiter(ctx context.Context, configKey string) *limiter {
    defaultTps := 1
    limiterIns := &limiter{
        TccConfigKey: configKey,
        tps:          int64(defaultTps),
        limiter:      rate.NewLimiter(rate.Limit(defaultTps), int(defaultTps)),
    }
    go limiterIns.refreshAsync(ctx) // 后台异步刷新
    return limiterIns
}

// 每秒从配置中心读 TPS，用 SetBurst/SetLimit 热更新限流器
func (l *limiter) refreshAsync(ctx context.Context) {
    if l.TccConfigKey == "" {
        return
    }
    for {
        tps, err := GetScriptTPS(ctx, l.TccConfigKey)
        if err == nil {
            l.tps = tps
            l.limiter.SetBurst(int(l.tps))
            l.limiter.SetLimit(rate.Limit(l.tps))
        }
        time.Sleep(1 * time.Second)
    }
}

func (l *limiter) Wait(ctx context.Context) error {
    return l.limiter.Wait(ctx)
}

func (l *limiter) GetTps() *int64 { return &l.tps } // 返回指针，供监控卡片实时读
```

**注意**：`Wait` 在主协程串行调用，控制的是「**派发/启动处理任务的速率**」，不是「完成速率」。单条处理慢时，在途并发数可能临时大于 TPS。

### 2. 进度聚合 StatModel —— atomic 两层计数 + 计时包装 + 卡片渲染

运行时"仪表盘数据模型"。**全局累计 + 当前批次两层计数**，因为会被并发 goroutine 同时写，所以全用 `atomic`。TPS 用**指针**持有，配置变化自动反映到展示。

```go
type DataClear struct {
    title, prams string
    tps          *int64 // 指针：限流器后台改 tps 时，卡片自动显示最新值

    total, success, failed int64                     // 全局累计
    batchNo, batchTotal, batchSuccess, batchFailed int64 // 当前批次

    batchFetchStart, batchFetchEnd time.Time         // 拉取耗时区间
}

// 每条记录处理完调用，并发安全；一次调用同时累加「批次层 + 全局层」
func (s *DataClear) BatchReport(total, success, failed int64) {
    atomic.AddInt64(&s.batchTotal, total)
    atomic.AddInt64(&s.batchSuccess, success)
    atomic.AddInt64(&s.batchFailed, failed)
    atomic.AddInt64(&s.total, total)
    atomic.AddInt64(&s.success, success)
    atomic.AddInt64(&s.failed, failed)
}

func (s *DataClear) NewBatch(batchNo int64) { // 每批清零批次计数
    atomic.StoreInt64(&s.batchNo, batchNo)
    atomic.StoreInt64(&s.batchTotal, 0)
    atomic.StoreInt64(&s.batchSuccess, 0)
    atomic.StoreInt64(&s.batchFailed, 0)
}

// 计时包装器：把一段逻辑用闭包传进来，前后打时间戳测耗时
// records/params/err 靠闭包捕获外层变量带出，不用返回值
func (s *DataClear) BatchFetch(f func()) {
    s.batchFetchStart = time.Now()
    f()
    s.batchFetchEnd = time.Now()
}
```

调用处：
```go
clearProc.BatchFetch(func() {
    records, params, err = queryH.GetRecords(ctx, params)
})
```

### 3. 可视化上报 Notify —— 一条消息原地秒刷，不刷屏

发消息与数据模型解耦：数据模型只提供 `CardStr()` 渲染字符串，Notify 负责"首发一条 + 后台每秒原地更新同一条 + 收尾定格终态"。

```go
func (n *Notify) Lark() *Notify {
    if n.stat != nil && n.stat.CardStr() != "" {
        n.msgID = SendCard(n.stat.CardStr()) // ① 首发，拿到 msgID
    }
    go func() {
        for {
            time.Sleep(1 * time.Second)       // ② 每秒刷新
            if n.stat == nil || n.stat.CardStr() == "" {
                continue
            }
            if n.msgID == "" {
                n.msgID = SendCard(n.stat.CardStr())     // 补发
            } else {
                _ = UpdateCard(n.msgID, n.stat.CardStr()) // 原地更新同一条
            }
        }
    }()
    return n
}

func (n *Notify) End() { // ③ defer 收尾，定格最终统计
    if n.msgID != "" {
        _ = UpdateCard(n.msgID, n.stat.CardStr())
    } else {
        SendCard(n.stat.CardStr())
    }
}
```

`SendCard` = 发新消息（返回 msgID）；`UpdateCard(msgID, ...)` = 更新旧消息。**靠 msgID 是否为空区分**，全程群里只有一条卡片。

### 4. 主流程 —— 批外串行 / 批内并行 / 批内末尾栅栏

```go
queryH := NewQueryHelper(handler)
for batchNo := int64(1); params != nil; batchNo++ {
    clearProc.NewBatch(batchNo)

    var records []*Record
    clearProc.BatchFetch(func() {
        records, params, err = queryH.GetRecords(ctx, params) // params 既是条件又是分页游标
    })
    if err != nil || len(records) == 0 {
        break // 拉空 / 游标为 nil → 整个任务结束
    }

    var eg errgroup.Group
    for _, record := range records {
        if err = limiter.Wait(ctx); err != nil { // ① 主协程串行放行（限流闸门）
            continue
        }
        tRecord := record
        eg.Go(func() error {                     // ② 批内并发处理
            werr := handler.ProcessSingleRecord(ctx, tRecord)
            if werr != nil {
                clearProc.BatchReport(0, 0, 1)
            } else {
                clearProc.BatchReport(0, 1, 0)
            }
            return werr
        })
    }
    if err = eg.Wait(); err != nil {             // ③ 栅栏：等本批全完成再进下一批
        logs.CtxError(ctx, "eg.Wait failed: %+v", err) // 只打日志不 return → 尽力清洗
    }
}
```

三个机制协作：**派发节奏由 limiter 控、并发执行由 errgroup 管、批次推进由 params 游标驱动**。

### 5. 接口隐式实现（容易混淆点）

数据模型 `*DataClear` 满足 Notify 需要的 `StatI` 接口，**全靠 Go 的隐式实现**，没有任何 `implements` 声明：

```go
type StatI interface {
    CardStr() string
}

// *DataClear 恰好有这个方法 → 自动满足 StatI
func (s *DataClear) CardStr() string { /* 渲染卡片 */ }
```

- 接收者是**指针** `(s *DataClear)`，所以实现接口的是 `*DataClear` 而非 `DataClear`。
- 调用处 `clearProc := &DataClear{}` 本就是指针，匹配；若写成值 `DataClear{}` 传入会编译报错。
- 想做编译期断言可写：`var _ StatI = (*DataClear)(nil)`。

---

## 容易踩的坑

1. **`limiter.Wait` 限的是派发速率不是完成速率** —— 在主协程串行调用控制"每秒开多少个任务"。
2. **TPS 用指针传递** —— 这样限流器后台热更新后，监控展示自动同步，无需手动推送。
3. **计数必须 atomic** —— `BatchReport` 在并发 `eg.Go` 里被调用。
4. **接口隐式实现 + 指针接收者** —— 实现者是 `*T` 不是 `T`，传值会编译失败。
5. **`eg.Wait` 出错不中断** —— 只打日志继续下一批，是"尽力清洗"语义；如需 fail-fast 要显式 return。
6. **后台 goroutine 无退出条件** —— 刷 TPS / 刷卡片两个死循环随进程结束回收；长生命周期服务里要换成可取消的 `ctx.Done()` 版本。

---

## 失败兜底：对账 + 幂等重跑（最容易忽略、却最关键的一环）

主流程对单条处理失败是「**直接跳过**」——只计数 + 上报 metrics，`eg.Wait` 出错也只打日志不中断，**不重试、不落失败队列**。那失败的数据怎么补回来？答案是把完整性保证**下沉到离线**：权威数据在数据仓库（Hive），处理逻辑做成**幂等**，失败记录靠「重跑」收敛。

### 幂等是前提

```go
func (h *Handler) ProcessSingleRecord(ctx, record) error {
    // ... 查源表、查目标表 ...
    if !needSync(ctx, p) { return nil }          // 已写入且一致 → 跳过
    // 写入时：唯一键冲突当成功；已存在的字段跳过更新
}
```

只有处理幂等，重跑同一批数据才安全（已成功的变 no-op，只有失败/未同步的真正被处理）。

### 两种批量补偿方式对比

| 维度 | 方式一：全量窗口重跑 | 方式二：离线对账 diff（推荐日常用） |
|---|---|---|
| 范围 | 时间窗内全部记录，幂等跳过已成功 | 只捞「目标表缺失 / 不一致」的 diff |
| 成本 | 高（已成功的也要查一遍才跳过） | 低（通常少 1~3 个数量级） |
| 待补清单可见性 | 差（只能从失败计数推断，不知道是谁） | 好（SQL 查出来就是确切清单，自验证） |
| 完备性 | 强（走完整在线判定，什么不一致都能修） | 取决于 diff 谓词写得全不全 |
| 数据新鲜度依赖 | 低（只读源表） | 高（依赖目标表的离线快照，有延迟） |

对账 diff 的本质就是一条 **源表 LEFT JOIN 目标表** 的 SQL：

```sql
SELECT l.*
FROM   source_table l
LEFT JOIN target_table r ON l.key = r.key
WHERE  r.key IS NULL          -- 目标表缺失（从没同步成功）
OR     l.some_field != r.some_field  -- 不一致（同步错了）
```

### 关键坑：对账谓词 ≠ 在线幂等判定

对账 SQL 的判定条件是写死的，往往**比在线 `needSync` 的判定窄**。比如目标主表那条存在、主键也对，但某个关联字段/子表没回写——这种不一致对账 SQL 抓不到，只有全量重跑（走完整在线校验）才能发现。

**所以两者互补，不是二选一**：
- 日常补偿用**对账 diff**（精准、便宜、知道补了谁）。
- 周期性兜底用**全量重跑**（覆盖对账谓词漏掉的不一致类型）。
- 想让对账够用，就把它的 JOIN 条件对齐到在线幂等判定的全部维度。

> 工程取舍：正因为有「离线对账 + 幂等重跑」托底，在线主流程才敢把单条失败直接跳过、出错不中断——把完整性从「在线流程」下沉到「离线对账」，在线流程因此可以做得简单、抗抖动。

---

## 实践印证（关联项目代码）

- 限流器：`jobs/jobs_common/limit.go`
- 配置中心读取 TPS：`utils/tcc.go` `GetScriptTPS`（`map[business]tps`，按业务取）
- 数据模型/卡片渲染：`jobs/wallet_biz_identity/notice.go`
- 卡片发送/更新：`jobs/jobs_common/lark_util.go`
- 主流程：`jobs/wallet_biz_identity/main.go`
- 失败补偿（全量 / mend 对账 / 单条 / 批量，按 QueryParams.OrderID 路由）：`biz_handler/ent_trade/sync_ent_order_id.go`（mend 对账 SQL 在 :45-91，needSync 判定在 :219）

---

## 原文链接

> 纯项目实战沉淀，无外部链接。
