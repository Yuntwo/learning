# SQL 核心语法

> 牛客 SQL快速入门题单（SQL1–SQL42）沉淀 + 日常踩坑整理。每个 section 自成一个"语法点"，按查询执行顺序 + 函数族组织。

---

## 目录

1. [SUBSTRING_INDEX 与别名作用域（WHERE / HAVING / GROUP BY / ORDER BY）](#1-substring_index-与别名作用域)
2. [SELECT 基础五件套（SELECT / DISTINCT / AS / LIMIT / ORDER BY）](#2-select-基础五件套)
3. [WHERE 过滤操作符大全（比较 / 逻辑 / IN / BETWEEN / LIKE / NULL / REGEXP）](#3-where-过滤操作符大全)
4. [聚合函数与 GROUP BY / HAVING](#4-聚合函数与-group-by--having)
5. [CASE WHEN / IF / IFNULL / COALESCE：条件表达式](#5-case-when--if--ifnull--coalesce条件表达式)
6. [JOIN 四种形态与坑](#6-join-四种形态与坑)
7. [子查询、派生表、CTE](#7-子查询派生表cte)
8. [UNION / UNION ALL](#8-union--union-all)
9. [日期函数（DATE_FORMAT / DATEDIFF / 次日留存）](#9-日期函数)
10. [文本函数（CONCAT / SUBSTRING / LEFT / RIGHT / LENGTH / REGEXP）](#10-文本函数)
11. [窗口函数（排名 / 聚合窗口 / 累计值）](#11-窗口函数)
12. [数学函数（ROUND / CEIL / FLOOR / ABS / MOD）](#12-数学函数)
13. [要点卡片（速记版）](#要点卡片速记版)

---

## 1. SUBSTRING_INDEX 与别名作用域

> **一句话总结**：`SUBSTRING_INDEX` 按分隔符切字符串；在 `WHERE` 里**看不到** `SELECT` 的别名，要么重复写表达式，要么套子查询，而 `GROUP BY` / `HAVING` / `ORDER BY` 大多数数据库是能看到别名的。

### 1.1 SUBSTRING_INDEX 语法

```sql
SUBSTRING_INDEX(str, delim, count)
```

- `str`：源字符串
- `delim`：分隔符（区分大小写，支持多字符）
- `count`：
  - **正数 n**：返回前 n 个分隔符**之前**的部分（从左往右数）
  - **负数 -n**：返回后 n 个分隔符**之后**的部分（从右往左数）
  - **0**：返回空字符串
- 若 `delim` 未在 `str` 中出现，返回原字符串
- 任意参数为 `NULL` 结果为 `NULL`

#### 基础例子

```sql
SELECT SUBSTRING_INDEX('a.b.c.d', '.', 1);    -- 'a'
SELECT SUBSTRING_INDEX('a.b.c.d', '.', 2);    -- 'a.b'
SELECT SUBSTRING_INDEX('a.b.c.d', '.', -1);   -- 'd'
SELECT SUBSTRING_INDEX('a.b.c.d', '.', -2);   -- 'c.d'
SELECT SUBSTRING_INDEX('a.b.c.d', '.', 10);   -- 'a.b.c.d'（超出返回原串）
SELECT SUBSTRING_INDEX('a.b.c.d', '.', 0);    -- ''
```

#### 取"第 N 段"惯用法（嵌套）

```sql
-- 取第 2 段 'b'：先取前 2 段 'a.b'，再从右取 1 段
SELECT SUBSTRING_INDEX(SUBSTRING_INDEX('a.b.c.d', '.', 2), '.', -1);  -- 'b'

-- 取第 3 段 'c'
SELECT SUBSTRING_INDEX(SUBSTRING_INDEX('a.b.c.d', '.', 3), '.', -1);  -- 'c'
```

**规律**：`SUBSTRING_INDEX(SUBSTRING_INDEX(str, delim, N), delim, -1)` → 第 N 段（1-based）。

#### 其他数据库的等价函数

| 数据库 | 写法 |
|------|------|
| MySQL / Hive / Spark SQL | `SUBSTRING_INDEX(str, delim, n)` |
| PostgreSQL | `SPLIT_PART(str, delim, n)`（1-based） |
| Oracle | `REGEXP_SUBSTR(str, '[^,]+', 1, n)` |
| SQL Server | 无原生等价，需 `STRING_SPLIT` + `ROW_NUMBER` |

### 1.2 别名作用域：SQL 逻辑执行顺序

```
FROM → WHERE → GROUP BY → HAVING → SELECT → DISTINCT → ORDER BY → LIMIT
```

**关键推论**：

- `WHERE` 在 `SELECT` **之前**执行 → `WHERE` 里**不能**用 `SELECT` 定义的别名
- `GROUP BY` / `HAVING` / `ORDER BY` 理论上也在 `SELECT` 之前或同时，但 **MySQL / PostgreSQL / Hive 等做了扩展**，允许在这些子句里使用别名

| 子句 | MySQL | PostgreSQL | Hive/Spark | Oracle | SQL Server |
|------|:-----:|:----------:|:----------:|:------:|:----------:|
| `WHERE` | ❌ | ❌ | ❌ | ❌ | ❌ |
| `GROUP BY` | ✅ | ✅ | ✅ | ❌ | ❌ |
| `HAVING` | ✅ | ✅ | ✅ | ❌ | ❌ |
| `ORDER BY` | ✅ | ✅ | ✅ | ✅ | ✅ |

`ORDER BY` 跨库都支持别名——因为它在 `SELECT` **之后**执行。

### 1.3 WHERE 用不了别名时的两种实现

```sql
-- ❌ Unknown column 'age' in 'where clause'
SELECT SUBSTRING_INDEX(SUBSTRING_INDEX(profile, ',', -2), ',', 1) AS age
FROM user_submit
WHERE age >= '20';
```

**方案一：重复表达式**

```sql
SELECT SUBSTRING_INDEX(SUBSTRING_INDEX(profile, ',', -2), ',', 1) AS age
FROM user_submit
WHERE SUBSTRING_INDEX(SUBSTRING_INDEX(profile, ',', -2), ',', 1) >= '20';
```

**方案二：派生表**

```sql
SELECT age FROM (
    SELECT SUBSTRING_INDEX(SUBSTRING_INDEX(profile, ',', -2), ',', 1) AS age
    FROM user_submit
) t
WHERE age >= '20';
```

现代优化器（MySQL 5.7+、PG）会做 **subquery flattening**，两种执行计划通常等价。**但列被函数包裹后都走不了普通索引**，热点查询考虑生成列 / 拆字段。

---

## 2. SELECT 基础五件套

> 对应题单 SQL1–SQL8。

### 2.1 SELECT 列 / 全部列 / 去重

```sql
SELECT * FROM user_profile;                      -- 所有列（生产慎用）
SELECT device_id, gender, age FROM user_profile; -- 指定列
SELECT DISTINCT university FROM user_profile;    -- 单列去重
SELECT DISTINCT university, gender FROM user_profile;  -- 多列组合去重
```

`DISTINCT` 作用于**整行（所有选中列的组合）**，不是某一列；`DISTINCT a, b` ≠ 只对 `a` 去重。

### 2.2 AS 别名

```sql
SELECT device_id AS user_infos_example FROM user_profile;
SELECT device_id   user_infos_example FROM user_profile;  -- AS 可省
```

### 2.3 LIMIT 与 OFFSET

```sql
SELECT device_id FROM user_profile LIMIT 2;            -- 前 2 行
SELECT device_id FROM user_profile LIMIT 2 OFFSET 3;   -- 跳过 3，取 2 行
SELECT device_id FROM user_profile LIMIT 3, 2;         -- 等价写法：OFFSET, COUNT
```

**两种写法容易写反**：`LIMIT m, n` 是**跳 m 取 n**；`LIMIT n OFFSET m` 才是"先 LIMIT 后 OFFSET"的语义。推荐只用后者，语义清晰。

### 2.4 ORDER BY 单列 / 多列 / 升降序

```sql
-- 单列升序（默认 ASC）
SELECT device_id, age FROM user_profile ORDER BY age;

-- 多列排序：gpa 升序，age 相同再按 age 升序
SELECT device_id, gpa, age FROM user_profile
ORDER BY gpa ASC, age ASC;

-- 降序
SELECT device_id, gpa, age FROM user_profile
ORDER BY gpa DESC, age DESC;
```

- 多列排序先看第一个关键字，第一个相等才比较第二个
- `NULL` 在 MySQL 默认排最前（升序）/最后（降序），PG 可用 `NULLS FIRST/LAST` 控制
- `ORDER BY` 可以用列号（`ORDER BY 2, 3`）但不推荐，列顺序一改就错

---

## 3. WHERE 过滤操作符大全

> 对应 SQL9–SQL18、SQL40。

### 3.1 比较与逻辑

```sql
WHERE university = '北京大学'
WHERE age > 24
WHERE age >= 20 AND age <= 23      -- 范围
WHERE university != '复旦大学'      -- 不等于（也可写 <>）
WHERE NOT university = '复旦大学'   -- 同上
```

`AND` 优先级高于 `OR`，混用务必加括号：

```sql
-- ❌ 实际是 (A AND B) OR C
WHERE gender = 'male' AND age > 20 OR university = '北京大学'

-- ✅ 明确括号
WHERE (gender = 'male' AND age > 20) OR university = '北京大学'
```

### 3.2 BETWEEN / IN / NOT IN

```sql
WHERE age BETWEEN 20 AND 23        -- 含两端 [20, 23]
WHERE university IN ('北京大学', '复旦大学')
WHERE university NOT IN ('复旦大学', '北京大学')
```

**`NOT IN` 的 NULL 陷阱**：列表里有 `NULL` 时，`NOT IN` 恒为 `UNKNOWN`，等价于空结果集。

```sql
WHERE id NOT IN (1, 2, NULL)  -- 一行都选不出来
```
改用 `NOT EXISTS` 或先 `WHERE ... IS NOT NULL`。

### 3.3 LIKE 模糊匹配

```sql
WHERE university LIKE '%北京%'     -- 包含 "北京"
WHERE university LIKE '北京%'      -- 以 "北京" 开头
WHERE name LIKE 'A_'              -- A 后恰好一个字符
WHERE path LIKE '100\\%折扣' ESCAPE '\\'   -- 转义 %
```

- `%` 匹配任意长度，`_` 匹配单个字符
- `LIKE '%x'` 走不了索引（左侧通配）；`LIKE 'x%'` 可以走前缀索引
- 不区分/区分大小写取决于字符集 collation（`utf8mb4_general_ci` 不区分）

### 3.4 NULL 判断

```sql
WHERE age IS NULL
WHERE age IS NOT NULL
-- ❌ 错误写法，永远 false
WHERE age = NULL
```

**NULL 不等于任何值（包括另一个 NULL）**。聚合函数里 `COUNT(col)` 忽略 `NULL`，`COUNT(*)` 不忽略。

### 3.5 REGEXP 正则

```sql
-- 中国手机号：1 开头，第二位 3-9，后 9 位数字
WHERE mobile REGEXP '^1[3-9][0-9]{9}$'

-- 邮箱简易
WHERE email REGEXP '^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}$'
```

常用元字符：`^` 开头、`$` 结尾、`.` 任意字符、`[abc]` 字符集、`[^abc]` 取反、`{n,m}` 次数、`|` 或。**MySQL 8.0 前的正则引擎不支持 `\d`/`\w`**，要写 `[0-9]`/`[A-Za-z0-9_]`。

---

## 4. 聚合函数与 GROUP BY / HAVING

> 对应 SQL19–SQL23。

### 4.1 五个基础聚合

```sql
SELECT
    COUNT(*)            AS total_rows,         -- 行数，包含 NULL
    COUNT(gpa)          AS non_null_gpa,       -- 忽略 NULL
    COUNT(DISTINCT gpa) AS distinct_gpa_count,
    SUM(gpa)            AS gpa_sum,
    AVG(gpa)            AS gpa_avg,
    MAX(gpa)            AS gpa_max,
    MIN(gpa)            AS gpa_min
FROM user_profile;
```

- `SUM`/`AVG` 对**全 NULL** 的分组返回 `NULL`（不是 0）——这是 CASE+SUM 写正确率时最常见的坑（见第 5 节）
- `AVG(col)` 等价于 `SUM(col) / COUNT(col)`，分母是**非 NULL 行数**，不是总行数

### 4.2 GROUP BY 与 SELECT 列表的纪律

```sql
SELECT gender, university, COUNT(*) AS num
FROM user_profile
GROUP BY gender, university;
```

**纪律**：`SELECT` 里的非聚合列必须全部出现在 `GROUP BY` 里。MySQL 5.7+ 默认开启 `ONLY_FULL_GROUP_BY`，违反会报错。

```sql
-- ❌ 报错：gpa 不是聚合列也不在 GROUP BY 里
SELECT university, gpa, COUNT(*) FROM user_profile GROUP BY university;
```

### 4.3 HAVING vs WHERE

| 维度 | WHERE | HAVING |
|------|-------|--------|
| 执行时机 | 分组前（逐行过滤） | 分组后（对聚合结果过滤） |
| 能用聚合函数 | ❌ | ✅ |
| 能用 SELECT 别名 | ❌ | ✅（MySQL/PG/Hive） |
| 性能 | 快（先缩小数据量） | 慢（先分组再过滤） |

```sql
-- 过滤聚合结果：必须 HAVING
SELECT university, AVG(gpa) AS avg_gpa
FROM user_profile
GROUP BY university
HAVING AVG(gpa) > 3.5;

-- 过滤原始行：优先 WHERE（能走索引）
SELECT university, AVG(gpa) AS avg_gpa
FROM user_profile
WHERE gpa IS NOT NULL
GROUP BY university;
```

**原则**：能用 `WHERE` 就不用 `HAVING`，`HAVING` 专治聚合结果。

### 4.4 分组 + 排序：TOP-N 里的分组

```sql
-- 每个学校平均 GPA 最高的 3 所
SELECT university, AVG(gpa) AS avg_gpa
FROM user_profile
GROUP BY university
ORDER BY avg_gpa DESC
LIMIT 3;
```

每个组内的 TOP-N（如"每个学校 GPA 最低的同学"）要用**窗口函数**，见第 11 节。

---

## 5. CASE WHEN / IF / IFNULL / COALESCE：条件表达式

> 对应 SQL29–SQL30、SQL38。

### 5.1 CASE 两种写法

`CASE` 是 SQL 里的"条件表达式"——在任何能放列/值的位置都能用（`SELECT`、`WHERE`、`ORDER BY`、`GROUP BY`、聚合函数的参数里……），用来根据条件返回不同的值。

#### (1) 简单 CASE：等值分支

```sql
CASE q.result
    WHEN 'right' THEN 1
    WHEN 'wrong' THEN 0
    ELSE NULL
END
```

- 结构：`CASE <表达式> WHEN <值1> THEN <结果1> WHEN <值2> THEN <结果2> ... ELSE <默认> END`
- 每个 `WHEN` 是**等值比较**（隐式 `=`），简短但只能做等值
- `CASE col WHEN NULL THEN ...` **永远不会命中**（等值比较对 NULL 永远不成立）——要判 NULL 必须用搜索 CASE

#### (2) 搜索 CASE：任意条件分支（日常默认用这个）

```sql
CASE
    WHEN age < 25              THEN '25岁以下'
    WHEN age BETWEEN 25 AND 29 THEN '25-29岁'
    WHEN age >= 30             THEN '30岁及以上'
    ELSE '其他'
END AS age_bucket
```

- 结构：`CASE WHEN <条件1> THEN ... WHEN <条件2> THEN ... ELSE ... END`
- 每个 `WHEN` 是完整的布尔表达式，可用 `>/</BETWEEN/IN/IS NULL/AND/OR`
- 简单 CASE 永远能改写成搜索 CASE；反之不行

#### (3) 分支匹配规则

- 从上到下**逐个试**，**第一个命中的 `THEN` 生效**，后面的 `WHEN` 直接跳过
- 分支顺序有语义，写得不对结果就错：

```sql
-- ❌ 大的条件写在前面，小的永远进不来
CASE
    WHEN age >= 18 THEN '成年'
    WHEN age >= 22 THEN '大学毕业以上'   -- 永远命中不到
END

-- ✅ 窄条件在前，宽条件兜底
CASE
    WHEN age >= 22 THEN '大学毕业以上'
    WHEN age >= 18 THEN '成年'
END
```

#### (4) ELSE 可省，但省了默认就是 NULL

```sql
CASE WHEN q.result = 'right' THEN 1 END
-- 当 result != 'right' 或 result IS NULL 时，返回 NULL（不是 0）
```

这是**最容易踩的坑**——套上 `SUM` 再除 `COUNT(*)` 求正确率时，如果某分组一条 'right' 都没有，结果会是 `NULL` 而不是 `0`。具体案例见 [5.2](#52-case-外套聚合最常见的正确率--分桶统计)。

#### (5) 别名放哪里：踩过的坑

**规则**：别名是给**整个 `CASE...END` 表达式**取名的，要放在 `END` **之后**，`AS` 可省。

```sql
-- ✅ 三种都对
SELECT CASE q.result WHEN 'right' THEN 1 ELSE 0 END AS is_right FROM quiz q;
SELECT CASE q.result WHEN 'right' THEN 1 ELSE 0 END    is_right FROM quiz q;  -- AS 省略
SELECT CASE WHEN q.result='right' THEN 1 ELSE 0 END "正确"        FROM quiz q; -- 双引号包中文名
```

**常见错法 1：别名塞进 `CASE` 内部**

```sql
-- ❌ 语法错误
SELECT
    CASE q.result
        WHEN 'right' THEN 1
        ELSE 0
    END is_right    -- 别名可以放这（在 CASE 外面）
FROM quiz q;

-- ❌ 这才是错的，把别名写进 CASE 内：
CASE q.result AS is_right WHEN 'right' THEN 1 END
```

**常见错法 2：别名写在函数参数里面**

```sql
-- ❌ 语法错误
COUNT(CASE WHEN q.result = 'right' THEN 1 END alias)
--                                              ↑ 别名不能塞进 COUNT() 的参数

-- ✅ 别名是给最外层表达式（COUNT(...) 整体）用的，要放 COUNT() 外面
COUNT(CASE WHEN q.result = 'right' THEN 1 END) AS right_count
```

**直觉**：`END` 只是 `CASE` 表达式的结束符；**别名属于 `SELECT` 列表里的那个"最外层表达式"**，最外层是谁，别名就写在谁后面。

#### (6) CASE 能出现在哪

不仅能在 `SELECT` 列表里，以下位置都合法：

```sql
-- 聚合函数的参数里（正确率统计最常见）
SUM(CASE WHEN result='right' THEN 1 ELSE 0 END)

-- ORDER BY 里做自定义排序
ORDER BY CASE level WHEN 'easy' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END

-- GROUP BY 里做动态分组
GROUP BY CASE WHEN age < 25 THEN 'young' ELSE 'old' END

-- WHERE 里（虽然大多场景可以用 AND/OR 替代）
WHERE 1 = CASE WHEN gender='male' AND age>=25 THEN 1 ELSE 0 END
```

### 5.2 CASE 外套聚合：最常见的正确率 / 分桶统计

```sql
-- 各难度正确率（SQL38 套路）
SELECT
    d.difficult_level,
    SUM(CASE WHEN q.result = 'right' THEN 1 ELSE 0 END) / COUNT(*) AS correct_rate
FROM question_practice_detail q
JOIN question_detail d ON q.question_id = d.question_id
GROUP BY d.difficult_level
ORDER BY correct_rate;
```

**⚠️ 踩坑：SUM(CASE ...) 不写 ELSE 0 的话，全不命中分组会返回 NULL**

```sql
-- ❌ 分组里一条 'right' 都没有时，整列全 NULL，SUM 返回 NULL，不是 0
SUM(CASE WHEN q.result = 'right' THEN 1 END) / COUNT(*)   -- → NULL / n = NULL

-- ✅ 加 ELSE 0，SUM 正常返回 0
SUM(CASE WHEN q.result = 'right' THEN 1 ELSE 0 END) / COUNT(*)
```

**SQL38 实际遇到的现象**：输出里某个难度的 `correct_rate` 显示成 `None`（或 `NULL`）：

```
difficult_level | correct_rate
hard            | None           ← 该难度所有题目都答错了，SUM=NULL
easy            | 0.5000
medium          | 0.6667
```

`None` 就是 `SUM(全是 NULL) = NULL`，再 `NULL / COUNT(*) = NULL` 的结果。修掉的两个方向：加 `ELSE 0` 让 SUM 拿到实数，或者直接换成 `COUNT(CASE ... END)`。

**链路回顾**：`CASE` 没 `ELSE` → 不命中返回 `NULL` → 整个分组的 CASE 列全 `NULL` → `SUM(全NULL) = NULL`（而非 0）→ `NULL / n = NULL`。任何一环换掉都能救：加 `ELSE 0`（改 CASE）、或用 `COUNT`（改聚合函数）、或用 `COALESCE(SUM(...), 0)`（改外层）。

**更简洁的写法：用 COUNT 替代 SUM**——`COUNT` 对全 NULL 返回 0，不需要 `ELSE`：

```sql
COUNT(CASE WHEN q.result = 'right' THEN 1 END) / COUNT(*) AS correct_rate
```

**整数除法陷阱**：MySQL 里 `1/3` 会自动转 decimal（`0.3333`），但 `COUNT/COUNT` 如果数据库严格按整数除法会截断为 0。稳妥写法乘 1.0：

```sql
SUM(CASE WHEN q.result = 'right' THEN 1 ELSE 0 END) * 1.0 / COUNT(*) AS rate
```

### 5.3 IF 简写（MySQL / Hive）

```sql
IF(condition, value_if_true, value_if_false)

-- 等价于 CASE WHEN age >= 25 THEN '25岁及以上' ELSE '25岁以下' END
IF(age >= 25, '25岁及以上', '25岁以下') AS age_cut
```

- 只能二选一，复杂分支还是用 `CASE`
- 不通用，PG / Oracle 没有（PG 有 `CASE` 或 `IIF`）

### 5.4 IFNULL / COALESCE：NULL 替换

```sql
-- 若 nick_name 为 NULL，替换为 '匿名'
SELECT IFNULL(nick_name, '匿名') FROM users;   -- MySQL 两参数
SELECT COALESCE(nick_name, real_name, '匿名') FROM users;  -- 通用，任意参数
```

- `IFNULL(a, b)` ≡ `COALESCE(a, b)`（只是参数数量不同）
- `COALESCE` 返回**第一个非 NULL** 的参数，SQL 标准
- 对 `0 / NULL` 的 `0` 做保护：`COALESCE(a / NULLIF(b, 0), 0)`

---

## 6. JOIN 四种形态与坑

> 对应 SQL24–SQL27。

### 6.1 四种 JOIN

| 类型 | 语义 | 左/右表匹配失败时 |
|------|------|---------------------|
| `INNER JOIN` | 两表都有 | 丢弃 |
| `LEFT JOIN` | 保留左表全部 | 右表列为 `NULL` |
| `RIGHT JOIN` | 保留右表全部 | 左表列为 `NULL` |
| `FULL OUTER JOIN` | 两表全保留 | 缺失侧为 `NULL`（MySQL 不支持，用 `UNION` 模拟） |

```sql
-- 只看答过题的用户（INNER）：SQL25 常见写法
SELECT u.university, COUNT(q.question_id) / COUNT(DISTINCT u.device_id) AS avg_cnt
FROM user_profile u
JOIN question_practice_detail q ON u.device_id = q.device_id
GROUP BY u.university;

-- 保留所有用户，没答题的也算进来（LEFT）
SELECT u.university, COUNT(q.question_id) AS total
FROM user_profile u
LEFT JOIN question_practice_detail q ON u.device_id = q.device_id
GROUP BY u.university;
```

### 6.2 ON vs WHERE：LEFT JOIN 的关键区别

```sql
-- A：条件在 ON，LEFT JOIN 行为保留左表全部
SELECT u.*, q.question_id
FROM user_profile u
LEFT JOIN question_practice_detail q
    ON u.device_id = q.device_id AND q.result = 'right';

-- B：条件在 WHERE，LEFT JOIN 退化成 INNER JOIN
SELECT u.*, q.question_id
FROM user_profile u
LEFT JOIN question_practice_detail q ON u.device_id = q.device_id
WHERE q.result = 'right';    -- 左表没匹配到的行 q.result 是 NULL，被过滤
```

- **主表条件** 放 `WHERE`
- **从表条件 + 决定是否匹配** 放 `ON`
- 要在 `LEFT JOIN` 后保留"没匹配到"的行，右表相关条件**必须**在 `ON` 里

### 6.3 多表 JOIN：链式写法

```sql
-- SQL26：学校 + 难度 + 平均刷题数
SELECT
    u.university,
    d.difficult_level,
    COUNT(q.question_id) / COUNT(DISTINCT q.device_id) AS avg_answer_cnt
FROM user_profile u
JOIN question_practice_detail q ON u.device_id = q.device_id
JOIN question_detail d          ON q.question_id = d.question_id
WHERE u.university = '山东大学'
GROUP BY u.university, d.difficult_level;
```

顺序上左到右两两 JOIN，可读性最好的是**先 JOIN 主表、再 JOIN 字典表**。

### 6.4 USING 简写

```sql
-- ON a.device_id = b.device_id 可以写成：
JOIN question_practice_detail USING (device_id)
```

前提：两表列名相同。结果里只保留一份该列。

---

## 7. 子查询、派生表、CTE

> 对应 SQL24（子查询首次出现）。

### 7.1 三种位置

```sql
-- 1) WHERE 中：标量 / 列子查询
SELECT * FROM question_practice_detail
WHERE device_id IN (SELECT device_id FROM user_profile WHERE university = '浙江大学');

-- 2) FROM 中：派生表（必须起别名）
SELECT t.age, COUNT(*)
FROM (
    SELECT SUBSTRING_INDEX(SUBSTRING_INDEX(profile, ',', -2), ',', 1) AS age
    FROM user_submit
) t        -- 没这个别名会报错
GROUP BY t.age;

-- 3) SELECT 中：标量子查询（每行一个值）
SELECT u.device_id,
    (SELECT COUNT(*) FROM question_practice_detail q
     WHERE q.device_id = u.device_id) AS answer_cnt
FROM user_profile u;
```

### 7.2 CTE（WITH 子句，MySQL 8.0+）

```sql
WITH zj_users AS (
    SELECT device_id FROM user_profile WHERE university = '浙江大学'
)
SELECT question_id, result
FROM question_practice_detail
WHERE device_id IN (SELECT device_id FROM zj_users);
```

- 可读性优于嵌套子查询，尤其多处复用同一派生表时
- 递归 CTE（`WITH RECURSIVE`）能处理树形 / 图遍历

### 7.3 子查询 vs JOIN

- 大多数**半连接**（`WHERE col IN (subq)`）和**等价的 JOIN** 优化器可以互转
- 子查询更适合表达"是否存在" / "不存在"：`EXISTS` / `NOT EXISTS`
- JOIN 更适合需要同时取两表列的场景

---

## 8. UNION / UNION ALL

> 对应 SQL28。

```sql
-- 山东大学 或 男性用户
SELECT device_id, gender, age, gpa FROM user_profile WHERE university = '山东大学'
UNION
SELECT device_id, gender, age, gpa FROM user_profile WHERE gender = 'male';
```

| | UNION | UNION ALL |
|---|---|---|
| 去重 | ✅ | ❌ |
| 性能 | 慢（多一步排序去重） | 快 |
| 场景 | 结果天然可能重复 | 结果集不会重复，或允许重复 |

**要求**：各 `SELECT` 列数相同、对应列类型兼容；列名取**第一个** SELECT 的。`ORDER BY` 只能放最外层：

```sql
(SELECT ...) UNION (SELECT ...) ORDER BY col1 DESC;
```

---

## 9. 日期函数

> 对应 SQL31（每天练题数）、SQL32（次日留存率）、SQL37（8 月练题）。

### 9.1 格式化与取部分

```sql
DATE_FORMAT(date, '%Y-%m-%d')    -- '2021-08-14'
DATE_FORMAT(date, '%Y%m')        -- '202108'
YEAR(date), MONTH(date), DAY(date), HOUR(ts), MINUTE(ts)
DATE(datetime_col)               -- 截断到日
```

常用格式符：`%Y` 4 位年、`%y` 2 位、`%m` 2 位月、`%c` 不补零月、`%d` 2 位日、`%H` 24 小时。

### 9.2 日期运算

```sql
DATE_ADD(date, INTERVAL 1 DAY)          -- 加 1 天
DATE_SUB(date, INTERVAL 7 DAY)
DATEDIFF(end, start)                    -- 日期差（天），end - start
TIMESTAMPDIFF(MINUTE, t1, t2)           -- 任意粒度差
CURDATE(), NOW(), CURRENT_DATE
```

### 9.3 月份/时间段过滤（能走索引 vs 不能）

```sql
-- ❌ 对列套函数，走不了索引
WHERE YEAR(date) = 2021 AND MONTH(date) = 8

-- ✅ 范围写法，能走 date 列的索引
WHERE date >= '2021-08-01' AND date < '2021-09-01'
```

### 9.4 次日留存率（SQL32 经典套路）

```sql
-- 核心：self-join，匹配 "同一用户 + 相差 1 天" 的记录
SELECT
    COUNT(DISTINCT q2.device_id) / COUNT(DISTINCT q1.device_id) AS avg_ret
FROM question_practice_detail q1
LEFT JOIN question_practice_detail q2
    ON q1.device_id = q2.device_id
   AND DATEDIFF(q2.date, q1.date) = 1;
```

思路：`q1` 是所有答题日，`q2` 左连接到"次日也答题"的记录；没次日的 `q2.*` 为 `NULL`，用 `COUNT(DISTINCT q2.device_id)` 剔除掉。

---

## 10. 文本函数

> 对应 SQL33（统计性别）、SQL34（从 URL 提取用户名）、SQL35（截取年龄）。

### 10.1 常用函数

```sql
CONCAT('a', 'b', 'c')              -- 'abc'
CONCAT_WS('-', 'a', 'b', 'c')      -- 'a-b-c'（分隔符）
LENGTH(str)                        -- 字节数（UTF-8 汉字 3 字节）
CHAR_LENGTH(str)                   -- 字符数（统计汉字用这个）
SUBSTRING(str, start, len)         -- 1-based，start 可负
LEFT(str, n), RIGHT(str, n)
LOWER(str), UPPER(str)
TRIM(str), LTRIM(str), RTRIM(str)
REPLACE(str, from, to)
LOCATE(sub, str)                   -- sub 在 str 中的位置，找不到 0
```

### 10.2 从字符串里提取字段的两种套路

**套路一：`SUBSTRING_INDEX` 切分（结构化分隔符）**

```sql
-- profile='男,25,北京,本科'，取第 2 段（age）
SELECT SUBSTRING_INDEX(SUBSTRING_INDEX(profile, ',', 2), ',', -1) AS age
FROM user_submit;
```

**套路二：`SUBSTRING` + `LOCATE`（定位分隔符）**

```sql
-- 从 URL "http://url/username" 中取 username
SELECT SUBSTRING(url, LOCATE('/', url, 8) + 1) AS user_name FROM user_submit;

-- 起点从 8 开始跳过 'http://'，取最后一个 '/' 之后
```

### 10.3 GROUP_CONCAT（字段拼接）

```sql
-- 把某组的所有值拼成一个字符串
SELECT university, GROUP_CONCAT(DISTINCT gender ORDER BY gender SEPARATOR ',') AS genders
FROM user_profile
GROUP BY university;
```

默认上限 1024 字节，大文本需调 `group_concat_max_len`。

---

## 11. 窗口函数

> 对应 SQL36（每校 GPA 最低）、SQL41（累计利润）。MySQL 8.0+ 支持。

### 11.1 排名三兄弟

```sql
ROW_NUMBER() OVER (PARTITION BY col ORDER BY x)   -- 1,2,3,4（唯一名次）
RANK()       OVER (PARTITION BY col ORDER BY x)   -- 1,2,2,4（跳号）
DENSE_RANK() OVER (PARTITION BY col ORDER BY x)   -- 1,2,2,3（不跳号）
```

**每校 GPA 最低（SQL36）**：

```sql
SELECT device_id, university, gpa
FROM (
    SELECT *, ROW_NUMBER() OVER (PARTITION BY university ORDER BY gpa ASC) AS rn
    FROM user_profile
) t
WHERE rn = 1
ORDER BY university;
```

并列最低时想都留 → 用 `RANK()` / `DENSE_RANK()` 再取 `= 1`。

### 11.2 聚合窗口：累计 / 同环比

```sql
-- 每日累计利润（SQL41）
SELECT dt,
    SUM(profit) OVER (ORDER BY dt ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS cum
FROM daily_profit;

-- 简写：SUM(profit) OVER (ORDER BY dt) 默认就是 UNBOUNDED PRECEDING 到 CURRENT ROW
```

frame 可选：

- `ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW` 累计
- `ROWS BETWEEN 6 PRECEDING AND CURRENT ROW` 7 天滑动
- `ROWS BETWEEN 1 PRECEDING AND 1 FOLLOWING` 邻居 3 行

### 11.3 偏移 / 首尾

```sql
LAG(x, 1)  OVER (PARTITION BY id ORDER BY dt)  -- 前一行值（环比）
LEAD(x, 1) OVER (PARTITION BY id ORDER BY dt)  -- 后一行值
FIRST_VALUE(x), LAST_VALUE(x), NTH_VALUE(x, n)
NTILE(4) OVER (ORDER BY x)     -- 分成 4 个桶
```

### 11.4 窗口 vs GROUP BY

| 维度 | GROUP BY | 窗口函数 |
|------|----------|----------|
| 行数 | 聚合后变少 | 原行数保留 |
| 场景 | "每组的总量" | "每行相对于组内的位置 / 累计" |
| 能否组合 | 聚合后再 JOIN 回原表 | 一步搞定 |

---

## 12. 数学函数

> 对应 SQL42。

```sql
ROUND(x, n)      -- 四舍五入到小数 n 位
CEIL(x), CEILING(x), FLOOR(x)   -- 向上 / 向下取整
ABS(x)
MOD(a, b)        -- 或 a % b
POWER(x, n), SQRT(x), EXP(x), LOG(x)
PI(), RAND()     -- RAND() 每次调用都重算，注意幂等性
TRUNCATE(x, n)   -- 截断（非四舍五入）
```

**百分比常见写法**：

```sql
ROUND(SUM(right_cnt) / COUNT(*) * 100, 2) AS correct_rate_pct
```

---

## 要点卡片（速记版）

- **执行顺序**：`FROM → WHERE → GROUP BY → HAVING → SELECT → DISTINCT → ORDER BY → LIMIT`。
- **别名**：`WHERE` 用不了，`GROUP BY/HAVING/ORDER BY`（MySQL/PG/Hive）能用。
- **CASE 别名**：写在 `END` 之后，`AS` 可省；`SUM(CASE ...)` 要么 `ELSE 0`、要么换 `COUNT(CASE ...)`，避免全 NULL 分组返回 NULL。
- **GROUP BY 纪律**：`SELECT` 里非聚合列必须在 `GROUP BY` 里（`ONLY_FULL_GROUP_BY`）。
- **HAVING 只治聚合**：能用 `WHERE` 过滤就别用 `HAVING`。
- **NULL**：`= NULL` 永远 false，用 `IS NULL`；`NOT IN` 遇 NULL 恒空；`SUM` 全 NULL 返回 NULL，`COUNT` 全 NULL 返回 0。
- **JOIN 条件位置**：主表条件放 `WHERE`、从表条件放 `ON`；`LEFT JOIN` 的从表条件写 `WHERE` 会退化成 `INNER`。
- **UNION 去重；UNION ALL 不去重**，能用后者就用后者。
- **SUBSTRING_INDEX 取第 N 段**：`SUBSTRING_INDEX(SUBSTRING_INDEX(s, d, N), d, -1)`。
- **日期过滤走索引**：别 `YEAR(col) = 2021`，用 `col >= '2021-01-01' AND col < '2022-01-01'`。
- **次日留存**：self-join + `DATEDIFF = 1` + `COUNT DISTINCT` 分子分母。
- **窗口函数**：`PARTITION BY + ORDER BY + frame`，排名三兄弟区别在跳号规则；`SUM() OVER (ORDER BY)` 默认累计。
- **列被函数包裹走不了索引**，热点查询考虑生成列或字段拆分。
- **整数除法**：有截断风险时乘 `1.0` 或用 `CAST(... AS DECIMAL(10,4))`。
- **LIMIT 写法**：推荐 `LIMIT n OFFSET m`，`LIMIT m, n` 顺序容易记反。
