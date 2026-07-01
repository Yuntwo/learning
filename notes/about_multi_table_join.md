# 关于多表查询 — 单表优先原则

> 来源：[RuoYi-Vue-Plus 框架文档](https://plus-doc.dromara.org/#/ruoyi-vue-plus/framework/explain/about_join)

---

## 核心观点

**多表 Join 容易造成性能下降与结果集膨胀，建议优先拆分单表查询。**

---

## 为什么要拆分 JOIN 查询？

将大连接查询分解为多个单表查询，有以下好处：

### 1. 缓存效率更高
- 单表查询结果更容易被缓存（应用层缓存或数据库缓存）
- JOIN 查询中任一表数据变化都会导致缓存失效

### 2. 减少锁竞争
- 单表查询持锁时间短，并发性能更好
- 多表 JOIN 可能锁住多张表的行

### 3. 应用层 JOIN 更易扩展
- 数据库拆分（分库分表）后，跨库 JOIN 无法使用
- 应用层组装数据天然支持分布式架构

### 4. 查询性能更稳定
- 单表查询复杂度可控，不受表数据量相互影响
- 多表 JOIN 随数据量增长性能可能急剧下降

### 5. 可维护性好
- 业务逻辑清晰，单表查询更易理解和调试
- 各查询可独立优化索引

---

## MyBatis-Plus 多表查询方案（应用层 JOIN）

参考阿里云开发者文章，使用 MyBatis-Plus 实现应用层 JOIN 的三种模式：

### 一对一查询
- 两次数据库查询完成
- 时间复杂度 O(1)

### 一对多查询
- 两次数据库查询
- 通过 Java Stream 流式分组操作将结果合并

### 多对多查询
- "空间置换时间"策略
- 借助流式运算和 HashMap 批量取值
- 查询性能稳定，数据量增大时仍保持 O(1) 时间复杂度

**关键优势：**
- 业务逻辑清晰，可维护性、可修改性好
- 可与二级缓存配合使用进一步提升效率
- 天然支持分库分表架构

---

## 权威参考

以下截图出自《高性能 MySQL》：

- ![性能对比1](https://foruda.gitee.com/images/1678979482724037085/1e74f3e1_1766278.png)
- ![性能对比2](https://foruda.gitee.com/images/1666336728402711844/52788205_1766278.png)
- ![性能对比3](https://foruda.gitee.com/images/1666336945935088277/f60e3288_1766278.png)
- ![性能对比4](https://foruda.gitee.com/images/1666336954686520161/c6c83adc_1766278.png)

---

## 参考链接

- [大连接查询分解好处](https://java.isture.com/db/mysql/mysql-x-optimize-decompose-connection.html)
- [如何用 MP 多表查询性能测试（阿里云）](https://developer.aliyun.com/article/858927)

---

## 实践印证

在 `wallet_content_trade_order` 项目中，所有数据库查询均采用单表查询 + 应用层组装模式。
例如 `dao/aggr_order.go` 中的 `queryAggrOrder` 函数：

```go
// 1. 查主单（content_order 表）
contentOrder, err := tradeDao.QueryContentOrderForAggr(orderID, walletBizIdentity, userID, false)
// 2. 查 sku 单（sku_content_order 表）
skuOrderList, err := tradeDao.QuerySkuOrder(orderID, userID, walletBizIdentity)
// 3. 查子单（sub_content_order 表）
subOrderList, err := tradeDao.QuerySubOrder(orderID, userID, walletBizIdentity)
// 4. 业务层组装
return &aggr_model.OrderPOAgg{
    ContentOrder: contentOrder,
    SkuOrderList: skuOrderList,
    SubOrderList: subOrderList,
}
```

这种模式的好处：每张表可独立按 user_id 分片，查询逻辑简单，符合 DDD 聚合根查询模式。
