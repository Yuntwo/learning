# MySQL 性能优化

> 来源：[pdai.tech - MySQL 性能优化](https://pdai.tech/md/db/sql-mysql/sql-mysql-performance.html)
> 记录时间：2026-04-16

---

## 核心观点

MySQL 性能优化的本质是**减少不必要的数据扫描**：通过 Explain 定位慢查询瓶颈，从数据访问层面减少请求量和扫描行数，从查询结构层面拆分大查询和大连接，最终让每次查询只做必要的工作。

---

## 要点整理

### 一、使用 Explain 进行分析

Explain 用来分析 SELECT 查询语句，是 MySQL 性能优化的第一步工具。

**关键字段：**

| 字段 | 含义 | 关注点 |
|------|------|--------|
| `select_type` | 查询类型 | SIMPLE（简单查询）、UNION（联合查询）、SUBQUERY（子查询）等 |
| `type` | 访问类型 | 从优到差：system > const > eq_ref > ref > range > index > ALL |
| `key` | 实际使用的索引 | NULL 表示未使用索引，需优化 |
| `rows` | 预估扫描行数 | 越小越好，行数越大说明查询效率越低 |
| `Extra` | 额外信息 | Using index（覆盖索引）、Using filesort（需文件排序）、Using temporary（使用临时表） |

**使用示例：**
```sql
EXPLAIN SELECT * FROM users WHERE age > 25;
```

### 二、优化数据访问

#### 1. 减少请求的数据量

**避免 SELECT *：** 只返回必要的列，减少网络传输和内存消耗

```sql
-- 反面示例
SELECT * FROM users;

-- 正面示例
SELECT id, name, email FROM users;
```

**使用 LIMIT 限制返回行数：** 只获取需要的行

```sql
SELECT id, name FROM users WHERE status = 1 LIMIT 20;
```

**缓存重复查询的数据：** 对于经常被重复查询的数据，使用缓存（如 Redis）可以显著提升性能，避免重复访问数据库。

#### 2. 减少服务器端扫描的行数

最有效的方式是**使用索引来覆盖查询**（Covering Index），让查询只需要访问索引而不用回表读取数据行。

```sql
-- 如果有 (name, age) 的联合索引，以下查询可以完全通过索引完成
SELECT name, age FROM users WHERE name = 'Tom';
-- Explain 中会显示 Extra: Using index
```

### 三、重构查询方式

#### 1. 切分大查询

一个大查询一次性执行可能会：
- 锁住大量数据
- 占满整个事务日志
- 耗尽系统资源
- 阻塞其他小但重要的查询

**解决方案：分批执行**

```sql
-- 反面示例：一次性删除大量数据
DELETE FROM messages WHERE create < DATE_SUB(NOW(), INTERVAL 3 MONTH);

-- 正面示例：分批删除，每次 10000 条
rows_affected = 0
do {
    rows_affected = do_query(
        "DELETE FROM messages WHERE create < DATE_SUB(NOW(), INTERVAL 3 MONTH) LIMIT 10000"
    )
} while rows_affected > 0
```

**关键思路：** 用 LIMIT 将一个大操作拆成多个小操作，每次操作后释放锁和资源，给其他查询执行的机会。

#### 2. 分解大连接查询

将一个大的 JOIN 查询分解为多个单表查询，在应用层进行数据关联。

```sql
-- 原始 JOIN 查询
SELECT * FROM tag
JOIN tag_post ON tag_post.tag_id = tag.id
JOIN post ON tag_post.post_id = post.id
WHERE tag.tag = 'mysql';

-- 分解为三个单表查询
SELECT * FROM tag WHERE tag = 'mysql';             -- 得到 tag_id = 1234
SELECT * FROM tag_post WHERE tag_id = 1234;        -- 得到 post_id 列表
SELECT * FROM post WHERE post.id IN (123, 456, 567, 9098, 8904);
```

**分解连接查询的好处：**

| 好处 | 说明 |
|------|------|
| 缓存更高效 | JOIN 查询中任一表变化就使整个缓存失效；单表查询的缓存互不影响 |
| 缓存复用率更高 | 单表查询的缓存结果可以被其他查询复用，减少冗余查询 |
| 减少锁竞争 | 单表查询锁定范围更小 |
| 易于拆分扩展 | 应用层连接让数据库拆分（分库分表）更容易实现 |
| 查询效率可能更高 | IN() 查询让 MySQL 按 ID 顺序查找，比随机连接更高效 |

**适用场景：** 当连接涉及的表数据量大、缓存命中率高、或者有分库分表需求时，分解连接查询收益最大。

---

## 实践要点总结

```
性能优化三板斧：

1. Explain 先行 —— 任何优化前先用 EXPLAIN 分析执行计划
   ↓
2. 减少数据访问 —— 不 SELECT *、用 LIMIT、用缓存、用覆盖索引
   ↓
3. 重构查询 —— 大查询分批、大 JOIN 分解为单表查询 + 应用层组装
```

---

## 参考链接

- [原文：MySQL - 性能优化 | Java 全栈知识体系](https://pdai.tech/md/db/sql-mysql/sql-mysql-performance.html)
- 关联阅读：[MySQL 索引(B+树)](https://pdai.tech/md/db/sql-mysql/sql-mysql-b-tree.html)
- 参考书籍：《高性能 MySQL》（High Performance MySQL）
