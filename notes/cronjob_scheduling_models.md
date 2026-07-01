# 定时任务调度模型：单机 vs 简单分片 vs MapReduce

> 来源：字节 Cronjob 调度平台文档 + 实践总结
> 记录时间：2026-04-16

---

## 核心观点

Cronjob 平台提供三种调度模型，复杂度递增：**单机**（单点执行）→ **简单分片**（静态并行）→ **MapReduce**（动态并行 + 可选汇总）。选型的关键在于数据划分方式和是否需要结果聚合。

---

## 一、三种模型对比总览

| 维度 | 单机 | 简单分片 | MapReduce |
|------|------|----------|-----------|
| 并行度 | 1 个实例 | N 个实例（配置时固定） | M 个实例（运行时动态） |
| Worker 收到的参数 | 无特殊参数 | `shard_index` + `total_shards` | `MapReqData`（自定义字符串） |
| 子任务定义 | 无 | 静态：配置时确定分片数 | 动态：initHandler 运行时生成 |
| 结果汇总 | 无 | 无 | 可选 Reduce 阶段 |
| 数据划分方式 | 全量处理 | `id % N = shard_index` | 按业务逻辑自由切分 |
| 适用数据量 | 小 | 中等，分布均匀 | 大，分布不均匀或需汇总 |

---

## 二、模型详解

### 1. 单机模型

最简单的模式——平台从实例池中选一台执行，整个任务在单机上完成。

```
┌──────────┐      ┌──────────┐
│ 调度平台  │─────▶│ 实例 A    │  ← 选中执行
│          │      └──────────┘
│          │      ┌──────────┐
│          │      │ 实例 B    │  ← 空闲
│          │      └──────────┘
└──────────┘
```

**适用场景：**
- 数据量小，单机能处理完
- 不需要并行的简单定时任务（如每日报表、缓存预热）

### 2. 简单分片

任务被拆成 **固定 N 个分片**，每个分片分配给不同实例并行执行。每个实例拿到 `shard_index` 和 `total_shards`，自行决定处理哪些数据。

```
                    ┌──────────────────────┐
                    │ 实例 A               │
              ┌────▶│ shard_index=0, N=3   │──▶ WHERE id % 3 = 0
              │     └──────────────────────┘
┌──────────┐  │     ┌──────────────────────┐
│ 调度平台  │──┼────▶│ 实例 B               │
│ 分片数=3  │  │     │ shard_index=1, N=3   │──▶ WHERE id % 3 = 1
└──────────┘  │     └──────────────────────┘
              │     ┌──────────────────────┐
              └────▶│ 实例 C               │
                    │ shard_index=2, N=3   │──▶ WHERE id % 3 = 2
                    └──────────────────────┘
```

**核心特点：**
- 分片数 N 在**配置时固定**
- 平台只给编号，数据划分逻辑在业务代码里（通常 `id % N`）
- 各分片独立执行，没有汇总步骤

**适用场景：**
- 数据能按 ID 均匀切分
- 各分片处理逻辑相同
- 不需要汇总结果

**局限性：**
- 数据分布不均匀时会倾斜（某些分片数据量远大于其他）
- 分片数改变需要重新配置

### 3. MapReduce 模型

分为三个阶段：Init（生成子任务）→ Process（并行处理）→ Reduce（可选汇总）。

```
┌─────────────┐
│  调度平台     │
└──────┬──────┘
       │ 触发
       ▼
┌─────────────────────────────────────────────────┐
│ 阶段1: Init (Master 实例)                         │
│                                                   │
│  initHandler() {                                  │
│    查库/计算 → 生成子任务列表                        │
│    return ["[1,100]", "[100,200]", "[200,350]"]   │
│  }                                                │
└──────────────────────┬──────────────────────────┘
                       │ 分发
          ┌────────────┼────────────┐
          ▼            ▼            ▼
   ┌────────────┐┌────────────┐┌────────────┐
   │ Worker A   ││ Worker B   ││ Worker C   │
   │            ││            ││            │
   │ MapReqData ││ MapReqData ││ MapReqData │
   │ ="[1,100]" ││ ="[100,200]"│ ="[200,350]"│
   │            ││            ││            │
   │ 处理id 1~100│ 处理id 100~200│ 处理id 200~350│
   └─────┬──────┘└─────┬──────┘└─────┬──────┘
         │ result      │ result      │ result
         └─────────────┼─────────────┘
                       ▼
   ┌───────────────────────────────────────────┐
   │ 阶段3: Reduce (可选)                       │
   │                                            │
   │  reduceHandler() {                         │
   │    汇总所有 Worker 的结果                    │
   │    生成报告 / 更新统计 / 发通知               │
   │  }                                         │
   └───────────────────────────────────────────┘
```

---

## 三、MapReduce 有 Reduce vs 无 Reduce

| | Map Only（无 Reduce） | Map + Reduce |
|---|---|---|
| 流程 | Init → Process ×N → 结束 | Init → Process ×N → Reduce → 结束 |
| 场景 | 每个子任务独立完成即可 | 需要汇总所有子任务的结果 |
| 例子 | 批量发通知、批量迁移数据 | 统计总处理数、生成汇总报告 |

---

## 四、代码 Demo

### 简单分片 Demo

```go
import "code.byted.org/webcast/scheduler"

handler := func(ctx context.Context, req *scheduler.ProcessRequest) (*scheduler.ProcessResponse, error) {
    // 平台传入的分片参数
    shardIndex := req.ShardIndex  // 当前分片编号，如 0
    totalShards := req.TotalShards // 总分片数，如 3

    // 业务逻辑：按分片扫库
    orders, err := db.Query(ctx,
        "SELECT * FROM orders WHERE id % ? = ? AND status = 'pending'",
        totalShards, shardIndex,
    )
    if err != nil {
        return nil, err
    }

    for _, order := range orders {
        processOrder(ctx, order)
    }

    return scheduler.NewProcessResponse(ctx, true), nil
}

// 注册为分片任务
err = scheduler.RegisterHandler("my.psm", "shard_task", handler)
```

### MapReduce Demo（完整）

```go
import "code.byted.org/webcast/scheduler"

// 阶段1: Init — 动态生成子任务
initHandler := func(ctx context.Context, req *scheduler.ProcessRequest) (*scheduler.ProcessResponse, error) {
    // 查库确定待处理的数据范围
    minID, maxID := getOrderIDRange(ctx) // 如 1 ~ 3500

    // 按每 1000 条切一个子任务
    var tasks []string
    for start := minID; start <= maxID; start += 1000 {
        end := start + 999
        if end > maxID {
            end = maxID
        }
        tasks = append(tasks, fmt.Sprintf("%d,%d", start, end))
    }
    // tasks = ["1,1000", "1001,2000", "2001,3000", "3001,3500"]

    logs.CtxInfo(ctx, "生成 %d 个子任务", len(tasks))
    return scheduler.Map(ctx, tasks)
}

// 阶段2: Process — 每个 Worker 处理一个子任务
processHandler := func(ctx context.Context, req *scheduler.ProcessRequest) (*scheduler.ProcessResponse, error) {
    // 解析业务参数
    parts := strings.Split(*req.MapReqData, ",")
    startID, _ := strconv.Atoi(parts[0])
    endID, _ := strconv.Atoi(parts[1])

    // 直接按范围查询，不需要 id % N
    count, err := syncOrders(ctx, startID, endID)
    if err != nil {
        return scheduler.NewMapResultResponse(ctx, false, err.Error())
    }

    return scheduler.NewMapResultResponse(ctx, true, fmt.Sprintf("synced:%d", count))
}

// 阶段3: Reduce — 汇总结果
reduceHandler := func(ctx context.Context, req *scheduler.ProcessRequest) (*scheduler.ProcessResponse, error) {
    totalSynced := 0
    for _, result := range req.ReduceReq.Results {
        // 解析每个 Worker 的返回
        count, _ := strconv.Atoi(strings.TrimPrefix(result, "synced:"))
        totalSynced += count
    }

    logs.CtxInfo(ctx, "全部完成，共同步 %d 条订单", totalSynced)
    // 可以发飞书通知、更新统计表等
    return scheduler.NewProcessResponse(ctx, true), nil
}

err = scheduler.RegisterMapReduceHandler("my.psm", "migrate_orders",
    initHandler, processHandler, reduceHandler)
```

---

## 五、简单分片 vs MapReduce 的选型决策

```
需要并行处理？
  ├─ 否 → 单机模型
  └─ 是 → 数据能按 ID 均匀取模吗？
           ├─ 是 → 需要汇总结果吗？
           │        ├─ 否 → 简单分片 ✅
           │        └─ 是 → MapReduce（平台配置，用 shard_id 扫库 + Reduce 汇总）
           └─ 否 → MapReduce + SDK ✅
                    （动态生成子任务，按时间/范围/业务维度灵活切分）
```

**关键判断点：**

1. **数据分布均匀 + 不需要汇总** → 简单分片（最简单）
2. **数据分布均匀 + 需要汇总** → MapReduce 平台模式（shard_id 扫库 + Reduce）
3. **数据分布不均匀 / 需要动态切分** → MapReduce SDK 模式（initHandler 动态生成）

---

## 六、实践印证

当前项目 `wallet_content_trade_order` 中的 `tasks/sync_history_auth_data.go` 等存量数据同步任务，就是典型的 MapReduce 使用场景：
- 数据量大，需要并行处理
- 按时间范围切分比按 ID 取模更合理（避免数据倾斜）
- 同步完成后可能需要汇总统计

---

## 参考链接

- [Map 模型文档](https://bytedance.larkoffice.com/wiki/wikcnurhz8IDdbfe3dQsj77HA3m)
- [MapReduce 模型文档](https://bytedance.larkoffice.com/wiki/wikcnZuwrdIgNhGV37CWpDe0hDd)
- `code.byted.org/webcast/scheduler` SDK 文档
