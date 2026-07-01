# 用一笔订单的支付回调，讲透 MySQL InnoDB 事务提交全过程

> 本文以直播电商「支付成功回调」的真实代码为线索，带你看清 redo log、undo log、binlog 在一次事务提交中各自扮演什么角色，存了什么，为什么这样设计。

## 一、场景：一笔订单支付成功

用户在直播间购买商品，支付平台回调通知我们"支付成功"。系统需要在**一个事务**里完成：

```go
// fuse_service.go — orderPaySuccess
repo.TxRepoDo(ctx, func(txRepo *repo.Repo) error {
    // ① 主单更新 — 待支付 → 支付成功
    txRepo.ShopOrderRepo.PaySuccess(...)
    // ② 子单更新 — 同步支付状态
    txRepo.ItemOrderRepo.PaySuccess(...)
    // ③ SKU 单更新 — 写入实付价格
    txRepo.SkuOrderRepo.PaySuccess(...)
    // ④ 发布领域事件 — 通知下游履约
    txRepo.DomainEventRepo.PaySuccessEvent(...)
    return nil
})
```

`TxRepoDo` 通过 GORM 的 `db.Transaction()` 将四步操作包裹在一个 MySQL 事务中。这个事务提交时，MySQL 内部到底发生了什么？

---

## 二、三种日志各记了什么

### 2.1 Redo Log —— "改了哪里，改成什么"

Redo log 是 InnoDB 引擎层的**物理日志**，记录的是数据页上字节级别的变化。

当步骤 ① 执行 `UPDATE content_order SET status=3 WHERE order_id='735xxx' AND status=1` 时，redo log 记录的**不是这条 SQL**，而是：

```
┌─ redo log record ─────────────────────────────────────────┐
│ type:      MLOG_REC_UPDATE_IN_PLACE   ← 操作类型（枚举值）  │
│ space_id:  150                        ← 表空间文件编号      │
│ page_no:   2048                       ← 16KB 数据页编号     │
│ offset:    312                        ← 页内字节偏移        │
│ new_value: 0x0000000000000003         ← status 的新值 3     │
└───────────────────────────────────────────────────────────┘
```

**注意：这里的 space_id + page_no + offset 是磁盘文件中的逻辑坐标，不是内存地址。** Buffer Pool 的内存地址每次重启都不同，记录它没有意义。而表空间文件的页号和偏移量是稳定的，崩溃恢复时可以直接定位到磁盘文件的对应位置。

一条 redo log record 只描述一个页上的一次原子修改。四条 SQL 会产生多条 record，组成一个 **MTR（Mini-Transaction）** 组，确保要么全部重放，要么全不重放。

磁盘上 redo log 以 512 字节的 **log block** 为单位存储：

```
ib_logfile0（默认 48MB，循环写）
┌─── log block (512B) ────────────────────────┐
│ header  (12B): block_no, data_len, checkpoint│
│ body   (492B): [record1][record2][record3]...│
│ trailer  (8B): checksum                      │
└──────────────────────────────────────────────┘
```

### 2.2 Undo Log —— "改之前是什么"

Undo log 是 InnoDB 引擎层的**逻辑日志**，记录的是"如何撤销这次修改"。

同样是步骤 ① 的 UPDATE，undo log 记录的也不是反向 SQL，而是结构化的二进制记录：

```
┌─ undo log record ─────────────────────────────────────────┐
│ type:       TRX_UNDO_UPD_EXIST_REC    ← 操作类型           │
│ undo_no:    3                          ← 本事务内第几条      │
│ table_id:   150                        ← 表的内部 ID        │
│                                                             │
│ 主键定位:                                                    │
│   pk[0] = 892736451 (id)              ← 哪一行               │
│                                                             │
│ 旧值列表:                                                    │
│   n_updated: 3                         ← 改了几个字段         │
│   field[0]: col=14, old=0x01           ← status 旧值 1       │
│   field[1]: col=19, old=0x00           ← pay_price 旧值 0    │
│   field[2]: col=33, old=0x99B42...     ← update_time 旧值    │
│                                                             │
│ 版本链:                                                      │
│   trx_id:   0x1A2B3C                  ← 当前事务 ID          │
│   roll_ptr: → 上一条 undo record       ← 串成版本链           │
└───────────────────────────────────────────────────────────┘
```

**注意定位方式的差异：** redo log 用物理坐标（page + offset），undo log 用逻辑坐标（table_id + 主键）。因为行的物理位置可能因页分裂而改变，但主键值永远不变。回滚时通过主键走 B+ 树索引找到行的当前物理位置，再写回旧值。

不同操作类型记录的内容不同：

| 操作 | undo 类型 | 存什么 | 回滚时做什么 |
|------|----------|--------|-----------|
| INSERT（步骤④插入领域事件） | `TRX_UNDO_INSERT_REC` | 仅主键 | 根据主键 DELETE |
| UPDATE（步骤①②③更新订单） | `TRX_UNDO_UPD_EXIST_REC` | 主键 + 被改列的旧值 | 写回旧值 |
| DELETE | `TRX_UNDO_DEL_MARK_REC` | 主键 + 整行旧数据 | 去掉删除标记 |

### 2.3 Binlog —— "做了什么操作"

Binlog 是 Server 层的**逻辑日志**，ROW 格式下记录的是行变更的前后值：

```
### UPDATE `wallet_trade`.`content_order`
### WHERE
###   @1=892736451          /* id */
###   @14=1                 /* status: 待支付 */
### SET
###   @14=3                 /* status: 支付成功 */
###   @19=9900              /* pay_price */
###   @33='2026-04-07 20:30:00'  /* update_time */

### INSERT INTO `wallet_trade`.`content_trade_domain_event`
### SET
###   @1=5739201
###   @5='pay_success'
###   @7='735xxx'
###   @8='{"order_id":"735xxx","status":3,...}'

Xid = 88921736
COMMIT;
```

---

## 三、三种日志的本质对比

到这里可以看出，redo log 和 undo log 的核心并不是 SQL 语句，而是一种**通用的结构化记录**：

```
Redo = 操作类型 + 物理位置(space/page/offset) + 新值
Undo = 操作类型 + 逻辑位置(table_id/主键)      + 旧值
```

两者合在一起，就是一次修改的完整信息：**在哪、改前、改后**。

| 维度 | Redo Log | Undo Log | Binlog |
|------|---------|---------|--------|
| **记录内容** | page 2048 offset 312 写入 `0x03` | id=892736451 的 status 旧值是 `1` | `UPDATE ... SET status=3 WHERE ...` |
| **日志级别** | 物理（字节级） | 逻辑（行级，结构化旧值） | 逻辑（行变更前后值） |
| **定位方式** | 文件坐标（space+page+offset） | 逻辑坐标（table_id+主键） | 库名+表名+行 |
| **存储位置** | `ib_logfile0/1`（循环写） | undo tablespace | `mysql-bin.000xxx`（追加写） |
| **谁产生** | InnoDB 引擎层 | InnoDB 引擎层 | Server 层 |
| **用途** | 崩溃恢复（重做） | 事务回滚 + MVCC 多版本读 | 主从复制 + 备份恢复 |

这种设计的好处：
- **紧凑高效**：不需要存 SQL 字符串，一条 undo record 可能就几十字节
- **解耦 SQL 层**：不管你用 GORM、原生 SQL 还是 LOAD DATA，引擎层的 redo/undo 格式完全一样
- **机器友好**：回滚/重放时直接按坐标操作内存，不需要走 SQL 优化器

---

## 四、事务提交：两阶段提交

当 `TxRepoDo` 中四步全部成功，GORM 调用 COMMIT，MySQL 内部执行**两阶段提交**保证 redo log 与 binlog 的一致性：

```
Prepare 阶段：
  → 将事务 XID 写入 redo log，状态设为 prepare
  → 刷盘 redo log

Commit 阶段：
  → 将事务 XID 写入 binlog
  → 刷盘 binlog
  → 将 redo log 状态设为 commit
```

**为什么需要两阶段？** 如果只写了 redo log 就宕机，主库订单状态是"支付成功"，但 binlog 没同步到从库，从库还是"待支付"——主从不一致。两阶段提交确保崩溃恢复时：redo log 是 prepare 状态 → 检查 binlog 有没有对应 XID → 有则提交，无则回滚。

---

## 五、完整时序图

```
应用层                         MySQL InnoDB                       磁盘
  │                               │                                │
  │── BEGIN ──────────────────→   │                                │
  │                               │                                │
  │── UPDATE content_order ──→    │── 写 undo log（旧 status=1）     │
  │   SET status=3                │── 改 Buffer Pool（脏页）         │
  │                               │── 写 redo log buffer            │
  │                               │                                │
  │── UPDATE sub_content_order →  │── 写 undo log                   │
  │                               │── 改 Buffer Pool                │
  │                               │── 写 redo log buffer            │
  │                               │                                │
  │── UPDATE sku_content_order →  │── 写 undo log                   │
  │                               │── 改 Buffer Pool                │
  │                               │── 写 redo log buffer            │
  │                               │                                │
  │── INSERT domain_event ────→   │── 写 undo log（仅主键）          │
  │                               │── 改 Buffer Pool                │
  │                               │── 写 redo log buffer            │
  │                               │                                │
  │── COMMIT ─────────────────→   │                                │
  │                               │ ═══ Prepare ═══                │
  │                               │── redo log → prepare ────────→ │ 刷盘
  │                               │                                │
  │                               │ ═══ Commit ═══                 │
  │                               │── binlog 写入 ───────────────→ │ 刷盘
  │                               │── redo log → commit            │
  │                               │                                │
  │←── OK ────────────────────    │                                │
  │                               │                                │
  │                               │ ...后台线程...                   │
  │                               │── 脏页 ──────────────────────→ │ 刷盘到数据文件
```

**关键洞察：** COMMIT 返回 OK 时，四张表的数据并没有真正写到磁盘数据文件——写的是 redo log 和 binlog。Buffer Pool 中的脏页由后台线程异步刷盘。这就是 WAL（Write-Ahead Logging）让事务提交变快的原因：用顺序写日志替代随机写数据页。

---

## 六、如果事务失败了？

假设步骤 ④ 发布领域事件时 INSERT 失败，`TxRepoDo` 返回 error，GORM 自动执行 ROLLBACK。

InnoDB 沿着 undo log 反向执行：
1. 根据 INSERT undo（主键 5739201）→ `DELETE FROM content_trade_domain_event WHERE id=5739201`
2. 根据 UPDATE undo（主键 + 旧值）→ SKU 单价格恢复
3. 根据 UPDATE undo → 子单状态恢复
4. 根据 UPDATE undo → 主单 status 从 3 回到 1

四张表的修改全部撤销，就像这个事务从没发生过——这就是**事务原子性**。

---

## 七、一句话总结

```
Redo  = 操作类型 + 物理位置 + 新值  → 崩溃后重做，保证持久性
Undo  = 操作类型 + 逻辑位置 + 旧值  → 失败时撤销，保证原子性 + MVCC
Binlog = 操作类型 + 行前后值        → 主从同步，保证高可用
两阶段提交                          → 保证 redo log 和 binlog 一致
```

它们不是三份冗余记录，而是在不同层次、用不同粒度、服务不同目的的三个互补机制。理解了这一点，你就能理解为什么 MySQL 的事务既安全又高效。

---

## 参考文档

- [【Mysql-InnoDB 系列】事务提交过程 | InfoQ](https://xie.infoq.cn/article/061d29f60d11bf0fd74919888)

