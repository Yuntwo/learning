# MySQL 性能优化 — 社交媒体发布内容

## Twitter/X

MySQL 性能优化三板斧：

1⃣ EXPLAIN 先行 — 查 select_type/key/rows 定位瓶颈
2⃣ 减少数据访问 — 不 SELECT *、用 LIMIT、覆盖索引避免回表
3⃣ 切分大查询 — LIMIT 分批，避免锁表耗尽资源
4⃣ 分解大 JOIN — 单表查询+应用层组装，缓存更高效

核心思路：让每次查询只做必要的工作。

#MySQL #数据库 #性能优化 #Backend

---

## 小红书

**标题：** MySQL性能优化三板斧｜后端必知

**正文：**

MySQL 查询慢？试试这套优化思路👇

🔍 第一步：EXPLAIN 先行
任何优化前先用 EXPLAIN 分析执行计划，重点关注：
- select_type：查询类型（简单/联合/子查询）
- key：是否命中索引
- rows：扫描行数，越小越好

📉 第二步：减少数据访问
- 不用 SELECT *，只查需要的列
- 用 LIMIT 限制返回行数
- 用覆盖索引（Covering Index）避免回表

✂️ 第三步：切分大查询
大 DELETE/UPDATE 一次执行会锁表、占满事务日志、阻塞其他查询。
解决方案：用 LIMIT 分批执行，每次处理一小部分。

🔧 第四步：分解大 JOIN
把多表 JOIN 拆成多个单表查询，在应用层组装数据。
好处：缓存更高效、锁竞争更小、易于分库分表。

一句话总结：让每次查询只做必要的工作。

**标签：** #MySQL #数据库优化 #后端开发 #性能调优 #程序员

**封面图：** learning/mysql_performance_cover.png
