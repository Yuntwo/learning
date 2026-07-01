# 单行热点更新:TPS / RT 的真相,到分桶与库存扣减落地

> 来源:由腾讯云《利用热点更新技术应对秒杀场景》一文引出,逐层追问 + 手写 5 个可运行 Go demo 实测沉淀
> 记录时间:2026-06-21

---

## 核心观点

"同一行只能串行更新"**不等于 TPS=1**。单行写入的吞吐被 `1/RT` 焊成一个天花板(通常几百~几千,不是 1);高并发真正的危害不是 TPS 暴跌,而是 **RT 随并发线性恶化**(请求堆在锁后排队)。要突破天花板只能**拆分(分桶)**,而拆分必然带来**借库存(匀桶)+ 合并对账**两个收尾步骤;落地时标准 MySQL 用"单条原子条件更新",秒杀级用"Redis+Lua 原子扣减"。

---

## 要点整理

### 1. 四个最容易踩的误解

| 误解 | 真相 |
|------|------|
| 单行串行 → TPS=1 | TPS ≈ 1/RT。持锁 50µs 则 TPS≈20000。"一次只能一个" ≠ "一秒只能一个" |
| 加并发一定能提 TPS | 只在**瓶颈有空闲**时成立(欠载段)。瓶颈打满后 TPS 封顶,加并发只堆 RT;过载还会反降 |
| RT 大 → 相同时间跑不完、完成数更少 | 漏了并发数。`完成数 = 并发 × (duration/RT)`,RT 随并发线性涨,并发被约掉 → 完成数恒定 |
| RT 升高 = 处理变慢了 | RT 涨的几乎全是**排队等待**,服务时间(50µs)没变。是请求在堆积,不是处理能力退化 |

### 2. 三条核心规律

- **TPS 由瓶颈资源决定**:瓶颈没满→加并发能提;满了→封顶。
- **RT = 排队 + 处理**:瓶颈满后并发翻倍 → RT 翻倍。
- **Little 定律 `并发 L = TPS × RT`**:TPS 封顶后,L 翻倍只能让 RT 翻倍。
  - 实测验证(并发256):`19745 × 0.013s ≈ 257 ≈ 256` ✓

### 3. 单行 demo 实测(持锁50µs,duration 2s)

```
并发    TPS       RT          说明
1      19972     50µs        没人抢,RT=持锁时间本身
2      19656     101µs       RT≈2×
...    ~19700    ...         TPS 全程封顶(单行天花板)
256    19745     13ms        RT≈256×;TPS 没降,降的话是噪声
```

- TPS 平,RT 严格线性翻倍。**看 RT 比看 TPS 抖动更能反映过载。**

### 4. 分桶 demo:突破天花板,但瓶颈会转移

把 1 行拆成 N 个各带锁的桶,固定并发256,12 核机器:

```
桶数    TPS         相对单桶
1      19722       1×
2      39556       2×
4      78636       4×
8      149650      ~7.6×    ← 线性段:拆 N 倍快 N 倍
16     201712      ~10×     ← 撞墙:CPU 12 核成新瓶颈
64     198436      ~10×     ← 平了(12 × ~16700 ≈ 200000)
```

> **最值钱的一句:瓶颈只会转移,不会消失。** 拆开"单行锁"→撞"CPU 核数";真实 DB 还会接着撞磁盘 IO / binlog 刷盘 / 网络。性能优化 = 不断定位并搬走当前瓶颈。

### 5. 真实库存场景:借库存 + 合并对账(缺一不可)

拆桶后请求分布不均(按 key hash 天然倾斜)→ 热门桶先空、冷桶有货 → 只认本桶会**少卖**。

| 风险 | 成因 | 对策 |
|------|------|------|
| 超卖 | 多请求同扣一桶 | 扣减放**桶锁内**校验 `val>0` |
| 少卖 | 库存倾斜,冷桶有货被误报售罄 | **借库存**:本桶空了去别的桶扣 |
| 对不上账 | 拆开后不知总剩余 | **合并归集**:求和所有桶,核对 `售出+剩余==初始` |

实测:仅本桶 → 少卖30件(剩余压在冷桶);借库存 → 卖光不少卖;两者都不超卖。

### 6. 落地一:MySQL(那套私有语法 vs 标准写法)

`COMMIT_ON_SUCCESS / ROLLBACK_ON_FAIL / QUEUE_ON_PK / TARGET_AFFECT_ROW` 是**腾讯云 Percona 定制内核私有语法**,标准 MySQL 不认识。Go 的 `database/sql` 不解析方言:

- 库支持 → 当普通字符串 `db.Exec` 透传。
- 标准 MySQL → 用单条原子条件更新(推荐):

```go
res, _ := db.ExecContext(ctx,
    `UPDATE stock SET num = num - ? WHERE id = ? AND num >= ?`, n, id, n)
affected, _ := res.RowsAffected()
// affected==1 成功;==0 库存不足(改0行,无副作用,无需回滚)
```

关键字映射:`COMMIT_ON_SUCCESS`=autocommit 单语句;`ROLLBACK_ON_FAIL`=WHERE 不命中改0行;`TARGET_AFFECT_ROW 1`=`RowsAffected()==1`;`QUEUE_ON_PK`=`WHERE id=?` 走主键行锁排队。

> 坑:放进事务做"扣库存+建订单",行锁从 UPDATE 持有到 COMMIT → 又长持锁。对策:扣库存放事务最后一步、事务体尽量小。这正是私有特性存在的理由。

### 7. 落地二:秒杀首选 Redis + Lua 原子扣减

Redis 单线程,一段 Lua **整段原子**,"读→判断→扣"合成一次往返,不超卖且不压 DB。

```lua
-- 单key:KEYS[1]=库存key, ARGV[1]=数量。返回 1成功/0不足/-1不存在
local stock = redis.call('GET', KEYS[1])
if stock == false then return -1 end
if tonumber(stock) < tonumber(ARGV[1]) then return 0 end
redis.call('DECRBY', KEYS[1], ARGV[1])
return 1
```

```go
n, _ := rdb.Eval(ctx, luaDeduct, []string{"stock:1001"}, 1).Int()
// 生产用 redis.NewScript:首次 EVAL,之后 EVALSHA 省带宽
```

手写的"借库存"逻辑用一段**多 key Lua** 即可原子完成(遍历桶,从第一个有货的扣)。
注意:Redis 扣减成功后需**异步可靠落库**(MQ + 对账补偿),DB 仍是最终账本。

---

## 实践印证(本地 Go demo,5 个均可独立运行)

目录 `yuntwo/go_learning/hotrow/`:
- `single/` 单行锁:TPS 封顶 + RT 线性涨
- `bucket/` 分桶:TPS×N,撞 CPU 瓶颈
- `stock/` 真实库存:借库存 + 合并对账治少卖
- `dbdeduct/` MySQL:私有语法 vs 标准原子扣减(标准库即可编译)
- `redislua/` Redis+Lua 原子扣减(内存模拟原子语义,无需装 Redis)

整条认知闭环:**为什么单行扛不住 → 怎么突破 → 突破后的新麻烦 → 怎么落 MySQL → 秒杀怎么扛。**

映射到支付/账户:热点账户余额拆成 N 个子账户分散写(分桶)→ 扣款跨子账户兜底(借库存)→ 查总余额/对账聚合求和(合并),三步缺一不可。

---

## 原文链接

> 仅存于本地笔记,发布时不带出。

- 腾讯云《利用热点更新技术应对秒杀场景》: https://www.tencentcloud.com/zh/document/product/237/13402
- 相关本地笔记:`mysql_hotspot_row_optimization.md`、`distributed_transaction_solutions.md`、`mysql_performance_optimization.md`
