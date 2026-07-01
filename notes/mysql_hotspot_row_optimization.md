# MySQL 热点行优化技术

> 来源：https://www.cnblogs.com/binyue/p/17391553.html
> 记录时间：2026-04-16

---

## 核心观点

秒杀等热点场景下，MySQL 官方版本通常不超过 1000 TPS。通过 **update 转 insert**、**SQL 合并**、**事务排队**、**批量提交** 四种递进式优化，可以将性能从 500 TPS 提升到 100,000 TPS。

---

## 要点整理

### 一、热点行的性能瓶颈

**根因：行锁串行化 + 死锁检测开销**

- InnoDB 为保证 ACID，事务更新时对目标行加锁，直到 commit/rollback 才释放
- 并发更新同一行 → 数据库内部串行化执行
- 高并发下死锁检测（`innodb_deadlock_detect`）成为额外瓶颈
  - 每次加锁都要遍历等锁线程队列做死锁检测
  - 队列越长，检测时间越长，TPS 急剧下降

**两个关键参数：**
- `innodb_deadlock_detect`：MySQL 8.0 新增，控制是否执行死锁检测（默认 ON）
- `innodb_lock_wait_timeout`：死锁超时时间，高并发时禁用检测 + 依赖超时可能更高效

### 二、应用层优化 —— Update 转 Insert

**核心思想：** 引入 slot 概念，将单行热点分散到多行，用 `INSERT ... ON DUPLICATE KEY UPDATE` 替代直接 UPDATE。

**表设计：**

```sql
CREATE TABLE `tb_sku_stock` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `sku_id` bigint(20) NOT NULL,
  `sku_stock` int(11) DEFAULT '0',
  `slot` int(11) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_sku_slot` (`sku_id`,`slot`),
  KEY `idx_sku_id` (`sku_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**库存增加（简单）：**

```sql
INSERT INTO tb_sku_stock (sku_id, sku_stock, slot)
VALUES (101010101, 10, ROUND(RAND()*9)+1)
ON DUPLICATE KEY UPDATE sku_stock = sku_stock + VALUES(sku_stock);
```

- 通过 `ROUND(RAND()*9)+1` 随机分配到 10 个 slot
- unique key 不冲突 → insert；冲突 → 分散的 update

**库存扣减（需前置检查，防止超扣）：**

```sql
INSERT INTO tb_sku_stock (sku_id, sku_stock, slot)
SELECT sku_id, -10 AS sku_stock, ROUND(RAND()*9+1)
FROM (
    SELECT sku_id, SUM(sku_stock) AS ss
    FROM tb_sku_stock
    WHERE sku_id = 101010101
    GROUP BY sku_id HAVING ss >= 10 FOR UPDATE
) AS tmp
ON DUPLICATE KEY UPDATE sku_stock = sku_stock + VALUES(sku_stock);
```

- 用 `FOR UPDATE` 保证实时一致性检查（MVCC 快照读不够）
- 整个操作在一次 DB 交互中完成

### 三、存储引擎层优化

#### 1. SQL 合并 —— 缩短锁持有时间（~2x 提升）

**问题：** 常规流程中 update + select 需要两次网络交互，锁持有时间横跨两次往返。

**方案：** 合并为一条 SQL（如 `SELECT * FROM UPDATE` 语法），减少网络开销。

典型库存扣减事务：
1. `INSERT` 交易流水表（对账用）
2. `UPDATE` 库存明细表
3. `SELECT` 库存明细表
4. `COMMIT`

合并后减少一次网络往返，锁持有时间缩短约一半。

#### 2. 事务排队 —— 降低锁竞争（~10x 提升）

**核心思想：** 对冲突事务预先排队，类似 MySQL 并行复制的 writeSet 机制。

**阿里库存中心的多层排队策略：**

- **应用端排队：** 相同 itemId 的扣减操作在单机内串行化处理
- **DB 端排队（两种 patch）：**
  - **并发控制（InnoDB 层）：** DB 自动判断，不需改业务代码
  - **Queue on PK（Server 层）：** 需要业务写 SQL hint，可精确控制指定 SQL

> 2013 年阿里单减库存 TPS 最高记录：**1,381 次/秒**

**关键洞察：** 官方版本约 500 TPS 的瓶颈不在锁本身，而在死锁检测——大量等锁线程导致每次取锁时队列扫描时间过长。通过排队控制并发数即可跳过这个瓶颈。

#### 3. 批量提交 —— 内存合并 + 分批提交（~40x 提升）

**参考：** 腾讯云 MariaDB 方案

**核心机制：**

1. 业务 SQL 带上 `commit on success` hint 标记热点行
2. 内核维护 hash 表，按主键/唯一键将请求 hash 到同一桶
3. 在时间窗口内（默认 100μs）收集请求，统一批量提交
4. **轮询处理：** 第一批提交时第二批收集，交替进行

**效果：**
- 串行处理 → 批处理，热点行更新无需每次扫描和更新 btree
- 2016 年阿里双 11 通过此方案达到 **100,000 TPS**

**局限性：**
- 热点识别依赖业务 SQL 注释，需事先确定热点行
- 大量非热点数据被收集时会拖累热点处理效率
- 产生大量 binlog，从库可能跟不上导致严重主从延迟

---

## 性能提升对比

| 优化方案 | 性能提升 | 改造层 | 复杂度 |
|---------|---------|-------|--------|
| Update 转 Insert（slot 分散） | 中等 | 应用层 | 低 |
| SQL 合并 | ~2x | 存储引擎 | 中 |
| 事务排队 | ~10x | 存储引擎 | 高 |
| 批量提交 | ~40x | 存储引擎 | 很高 |

---

## 参考链接

- [原文 - MySQL 热点行优化技术](https://www.cnblogs.com/binyue/p/17391553.html)
- [plantegg - MySQL 针对秒杀场景的优化](https://plantegg.github.io/2020/11/18/MySQL%E9%92%88%E5%AF%B9%E7%A7%92%E6%9D%80%E5%9C%BA%E6%99%AF%E7%9A%84%E4%BC%98%E5%8C%96/)
- [腾讯云 MariaDB 热点更新方案](https://cloud.tencent.com/document/product/237/13402)