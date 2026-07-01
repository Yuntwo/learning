# MySQL 间隙锁六个案例：结合 `aweme_wallet_content_trade_db.content_order`

> 来源：https://www.51cto.com/article/779551.html
> 原文标题：六个案例搞懂间隙锁-51CTO.COM
> 记录时间：2026-04-13

---

## 核心观点

间隙锁不是“锁住某一行”，而是锁住索引区间里的“空档”。  
在 InnoDB 的 `Repeatable Read` 隔离级别下，`for update`、`update`、`delete` 这类当前读如果走到范围扫描，常见结果不是单纯的行锁，而是 `Record Lock`、`Gap Lock`、`Next-Key Lock` 的组合。

对业务排障最关键的不是背定义，而是先判断三件事：

1. 查询到底走的是唯一索引还是非唯一索引
2. 是等值查还是范围查
3. 扫描在什么位置停止

---

## 三种锁先分清

### 记录锁

只锁已存在的那一行。

典型场景：

```sql
SELECT * FROM user WHERE id = 5 FOR UPDATE;
```

如果 `id=5` 存在，并且 `id` 是唯一索引，那么通常退化成记录锁。

### 间隙锁

锁的是两个索引值之间的空档，不允许别的事务往这个区间里插入新记录。

区间特征：

- 间隙锁是左开右开或退化后的纯 gap 区间
- 它的作用重点是“防插入”，不是“防更新已有行”

### 临键锁

`Next-Key Lock = Record Lock + Gap Lock`

区间特征：

- 临键锁的基本单位可以理解为左开右闭
- InnoDB 默认先按 next-key 加，再根据是否是唯一等值命中等情况退化

---

## 原文里的五条规则，压缩成一句人话

1. 默认先按 `Next-Key Lock` 思考
2. 只锁扫描实际访问到的索引对象
3. 唯一索引范围查，会锁到第一个不满足条件的值
4. 唯一索引等值查且记录存在，会退化成记录锁
5. 非唯一索引等值查，往往不会只锁一条，会继续形成更大的范围

---

## 六个案例

### 案例一：唯一索引等值命中，只锁记录

原始形式：

```sql
SELECT * FROM user WHERE id = 5 FOR UPDATE;
```

结论：

- `id` 是唯一索引
- `5` 存在
- 最终退化为记录锁

所以插入 `3`、`6` 都不受影响，因为锁没有扩散到间隙。

### 案例二：唯一索引等值查不存在的值，会锁住相邻间隙

原始形式：

```sql
SELECT * FROM user WHERE id = 3 FOR UPDATE;
```

假设现有主键是 `1, 5, 7, 11`，那么 `3` 落在 `(1, 5)` 之间。

结论：

- 虽然是按唯一索引查
- 但查的是不存在的值
- 扫描会定位到相邻区间，最终表现为锁住 `(1, 5)` 这个 gap

所以插入 `2` 会被阻塞，插入 `6` 不会。

这类问题线上最容易被误判成“为什么明明没查到数据，也把别人插入卡住了”。

### 案例三：唯一索引范围查，锁到第一个不满足条件的值

原始形式：

```sql
SELECT * FROM user WHERE id >= 5 AND id < 6 FOR UPDATE;
```

表面上只想拿 `id=5`，但这是范围条件，不是唯一等值命中。

结论：

- 先命中 `5`
- 再向右找到第一个不满足条件的值 `7`
- 最终锁区间可理解为 `[5, 7]`

因此插入 `6` 或更新可能落入该区间的索引值，都会受影响。

### 案例四：非唯一索引范围查，锁范围通常比你想得更大

原始形式：

```sql
SELECT * FROM user WHERE age >= 5 AND age < 6 FOR UPDATE;
```

`age` 是普通索引，不是唯一索引。

结论：

- 即使你直觉上只想锁 `age=5`
- 非唯一索引不会像唯一等值命中那样轻易退化成单行锁
- 扫描会继续向右找边界

所以锁区间通常会扩成更大的 next-key 范围，而不只是某个点。

### 案例五：间隙锁之间不互斥，但插入意向会造成死锁

两个事务都先对同一 gap 做当前读：

```sql
SELECT * FROM user WHERE id = 3 FOR UPDATE;
SELECT * FROM user WHERE id = 4 FOR UPDATE;
```

如果 `3` 和 `4` 都落在 `(1, 5)` 这个 gap：

- 两边都能拿到 gap 相关锁
- 之后都尝试往这个 gap 插入
- 就可能互相等待，形成死锁

要点不是“gap lock 彼此冲突”，而是“后续插入动作和已有 gap 范围冲突”。

### 案例六：`limit` 会改变扫描终点，从而改变锁范围

原始形式：

```sql
DELETE FROM user WHERE age = 6 LIMIT 1;
```

结论：

- 锁范围不只由 `where` 决定
- 还取决于存储引擎实际扫描到了哪里
- `limit 1` 让扫描提前停止，锁范围也会随之缩小

这是“只锁访问到的对象”最值得记的一条工程化结论。

---

## 放到 `content_order` 上怎么理解

通过 `bytedance-rds` 实查，`aweme_wallet_content_trade_db.content_order` 上最关键的索引包括：

- `uniq_order(order_id, order_type)`
- `idx_user_orders(order_type, user_id, status, create_time)`
- `idx_user_items(order_type, user_id, item_type, item_id, item_sku, status)`
- `idx_wbi_user_items(wallet_biz_identity, user_id, item_id, item_sku, status)`
- `idx_biz_order_id(biz_order_id)`

项目模型也能对上：

- `order_id`、`order_type`、`biz_order_id`、`user_id`、`status`、`create_time` 等字段见 `loader/resource/model_wallet_content_trade_db.go`

### 1. 用唯一索引精确锁单，影响面最小

如果代码里是：

```sql
SELECT *
FROM content_order
WHERE order_id = 'CO_123'
  AND order_type = 1
FOR UPDATE;
```

并且这笔单存在，那么它更接近案例一：

- 走 `uniq_order(order_id, order_type)`
- 锁集中在单条记录
- 不容易把相邻订单的插入也卡住

这类写法适合“拿单后改状态”。

### 2. 查不存在的唯一键，也可能把插入卡住

如果代码里先做幂等探测：

```sql
SELECT *
FROM content_order
WHERE order_id = 'CO_NOT_EXIST'
  AND order_type = 1
FOR UPDATE;
```

而这条订单不存在，那么行为更接近案例二：

- 没查到记录
- 但不是“完全没加锁”
- 仍可能锁住相邻索引区间

所以并发创建订单时，如果多个事务都在“先 `for update` 查不存在，再插入”，就要小心出现插入阻塞。

### 3. 用户订单列表类 SQL，最容易意外扩大锁范围

例如：

```sql
SELECT order_id
FROM content_order
WHERE order_type = 1
  AND user_id = 10001
  AND status = 0
FOR UPDATE;
```

这类 SQL 贴合 `idx_user_orders(order_type, user_id, status, create_time)` 的前缀，但它不是唯一定位。

在 RR 下要这样理解：

- 锁的对象是这段联合索引范围
- 不是某个单点
- 并发事务如果要往同一用户、同一状态的索引范围里插单，可能会被卡

所以“按用户扫一批待支付订单再处理”的写法，比“按唯一单号锁定一笔订单”更容易带出间隙锁问题。

### 4. 非唯一业务索引也会带来 gap 行为

例如：

```sql
SELECT order_id
FROM content_order
WHERE biz_order_id = 'BIZ_001'
FOR UPDATE;
```

`biz_order_id` 只有普通索引 `idx_biz_order_id`，不是唯一索引。

这更接近案例四：

- 即使业务上你觉得一个 `biz_order_id` 应该只对应一笔单
- 存储层并没有唯一性保证
- 加锁语义仍按非唯一索引处理

这类场景尤其容易把“业务唯一”误当成“数据库唯一”。

### 5. `limit` 不是性能细节，还是锁范围控制手段

例如：

```sql
DELETE FROM content_order
WHERE order_type = 1
  AND user_id = 10001
  AND status = 0
LIMIT 1;
```

这类语句即使不建议在线上主交易表直接使用，也能说明一个问题：

- `limit` 不只是减少结果集
- 还会影响扫描停止点
- 进而影响锁区间

因此做补偿、清理、批处理时，`limit + order by + 稳定索引` 一起设计，比只看 `where` 更重要。

---

## 实践建议

1. 要锁单就尽量走唯一索引，优先 `order_id + order_type`
2. 幂等检查如果是“查不存在再插入”，要预判 gap lock 风险
3. 用户维度、商品维度的批量当前读，默认按“会锁范围”思考，不要按“只锁命中行”思考
4. 业务上唯一，不等于数据库唯一；只有真正的唯一索引，才能明显缩小锁影响面
5. 遇到“查不到数据却阻塞插入”，优先排查是不是 RR + 当前读 + 不存在键定位到了 gap
6. 做批量改状态、批量删除时，把 `order by`、`limit`、索引顺序一起看

---

## 参考

- 原文：https://www.51cto.com/article/779551.html
- 项目模型：`loader/resource/model_wallet_content_trade_db.go`
- 数据库：`aweme_wallet_content_trade_db`
- 实查表：`content_order`、`content_order_index`
