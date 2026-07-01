# 营销项目库存设计实战(对照 5 个热点扣减 demo)

> 来源:内场 `webcast/wallet_promotion_core` 营销服务源码调研
> 记录时间:2026-06-21
> ⚠️ 含内部源码,仅本地沉淀,**不对外发布**
> 关联:[[single_row_hotspot_tps_deep_dive]](从 0 手写的 5 个 Go demo:single/bucket/stock/dbdeduct/redislua)

---

## 核心观点

营销项目里的"库存"= 预算 / 券库存 / 红包池。其 V1 券库存扣减就是「分桶 + 随机均衡 + 借库存 + 求和对账」的工业实现,和自己手写的 `stock` demo 一一对应,且更成熟;红包用「预生成奖池 + RPOP」这种更高阶的物化模型;并且演进方向是把整套复杂逻辑**下沉到独立库存中台**(`wallet_promotion_base`),业务只调 `DeductStock` RPC。还意外发现内部 SQL 有 `AUTO_LOGIC_COMMIT_ROLLBACK` hint,正是腾讯云文章那套"热点更新"的字节版。

---

## 一、demo ↔ 真实实现 对照表

| demo 概念 | 真实实现 | 文件:行 |
|------|------|------|
| 单条原子 UPDATE(dbdeduct) | 券 V2-DB 扣减 `UPDATE...WHERE avail>=?` + RowsAffected | `dal/db/coupon_stock.go:120-152` |
| 分桶(bucket) | 券 V1 每分片独立容量 + 用量 key `{metaNo}_{i}` | `service/coupon/record/send_direct_coupon.go:219` |
| 借库存(stock 策略B) | 遍历**全部**分片,直到某片成功才停 | `send_direct_coupon.go:218-239` |
| 合并对账(stock merge) | 读取时所有分片用量求和,`remain=total-Σused` | `service/stock/coupon_stock.go:183-211` |
| Redis Lua 原子(redislua) | 抢红包 Lua:SISMEMBER+RPOP+SADD | `dal/kv/red_packet.go:230-241` |
| Redis 挡洪峰+异步落库 | Redis 扣 → ByteState 异步落库 → 补偿 | `service/red_packet/room_red_packet.go:246→260` + `handler/red_packet/compensate.go` |

---

## 二、券 V1:分桶 + 随机均衡 + 借库存(对照 stock demo)

`service/coupon/record/send_direct_coupon.go:214-245`

```go
func dealShardStock(ctx context.Context, metaInfo *coupon.PromotionCouponMeta, userMemory bool) error {
	canUse := false
	// ① 把所有分片下标随机打乱(主动均衡,不固定从 0 号片开始)
	randomIndexs := generateRandomNumber(0, metaInfo.StockShardCount, metaInfo.StockShardCount)
	for _, randomIndex := range randomIndexs {              // ② 逐个分片尝试 = 借库存
		shardMetaNo := fmt.Sprintf("%s_%d", metaInfo.MetaNo, randomIndex)
		ShardMetaStock, mErr := dal.GetShardMetaStock(ctx, shardMetaNo, userMemory) // 该片容量
		if mErr != nil {
			continue
		}
		shardUseNum, sErr := kv.IncrShardMetaUseNum(ctx, shardMetaNo)  // ③ 原子 INCR 用量
		if sErr != nil {
			continue
		}
		if shardUseNum > ShardMetaStock.ShardStockNumber {  // ④ 这片满了
			_ = kv.SetShardMetaUseNum(ctx, shardMetaNo, ShardMetaStock.ShardStockNumber) // 修正溢出(幂等)
			continue                                         //    → 借下一片
		}
		canUse = true
		break                                                // ⑤ 扣成功
	}
	if canUse {
		return nil
	}
	return werror.WalletStockNotEnough                       // ⑥ 全部片都满才真售罄
}
```

随机全排列(主动均摊热点,比 demo 里 `id%N` 固定路由更均衡):

```go
// 生成 count 个 [start,end) 不重复随机数
func generateRandomNumber(start int, end int, count int) []int {
	nums := make([]int, 0)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for len(nums) < count {
		num := r.Intn(end-start) + start
		exist := false
		for _, v := range nums {
			if v == num { exist = true; break }
		}
		if !exist { nums = append(nums, num) }
	}
	return nums
}
```

**少卖?没有** —— 遍历全部分片,只有每片都满才返回 `WalletStockNotEnough`,等价于 stock demo 的"策略B 借库存"。
**超卖?防住了** —— 先 `INCR` 拿唯一递增值再比容量,恰好踩中容量的只有一个请求成功,溢出者失败并把用量 `Set` 回容量(幂等修正)。

读取侧求和对账 `service/stock/coupon_stock.go:183-211`:

```go
func calShardStockWithError(ctx context.Context, meta *coupon.PromotionCouponMeta, stockMap map[string]int64) error {
	shardMetaNos := make([]string, 0)
	for i := 0; i < meta.StockShardCount; i++ {
		shardMetaNos = append(shardMetaNos, fmt.Sprintf("%s_%d", meta.MetaNo, i))
	}
	shardMetaStockMap, sErr := kv.MGetCouponShardStockNum(ctx, shardMetaNos)  // 拿各片用量
	if sErr != nil { return sErr }
	totalUse := int64(0)
	for _, num := range shardMetaStockMap {  // ← 合并:所有分片用量求和
		totalUse += num
	}
	if meta.StockNumber-totalUse <= 0 {
		stockMap[meta.MetaNo] = 0
	} else {
		stockMap[meta.MetaNo] = meta.StockNumber - totalUse  // 剩余 = 总量 - Σ用量
	}
	return nil
}
```

---

## 三、券 V2-DB:标准原子 UPDATE(对照 dbdeduct demo)

`dal/db/coupon_stock.go:115-153`,和手写 demo 几乎一字不差,且带字节版"热点更新"hint:

```go
case core.CouponStockOperateType_Send:
	sqlFormat = fmt.Sprintf("UPDATE /*+ AUTO_LOGIC_COMMIT_ROLLBACK */ %s"+
		" SET avail_stock_num = avail_stock_num - ?, send_num = send_num + ?"+
		" WHERE biz_id = ? AND coupon_meta_no = ? AND avail_stock_num >= ?", ...) // ← 条件防超卖
	sqlArgs = []any{changeNum, changeNum, bizID, couponMetaNo, changeNum}
...
result := d.tx.WithContext(ctx).Exec(sqlFormat, sqlArgs...)
if result.RowsAffected == 0 {
	return 0, werror.WalletStockNotEnough  // ← 改 0 行 = 库存不足
}
```

> **彩蛋:`/*+ AUTO_LOGIC_COMMIT_ROLLBACK */` 是内部 DB 的 SQL hint,正是腾讯云文章
> `COMMIT_ON_SUCCESS / ROLLBACK_ON_FAIL` 的字节等价版**——让这条语句在引擎里成功即提交、
> 失败即回滚,最小化热点行锁占用。即"那套私有语法"在内场是真实存在的,只是换了 hint 形式。

---

## 四、红包:预生成奖池 + RPOP(比减计数器更高阶)

拼手气红包每个金额不同,所以把库存**物化成一队具体红包**,抢 = 弹一个(扣量 + 分金额一步到位)。

奖池初始化 `dal/kv/red_packet.go:208-222`:

```lua
-- SetAndLPushScript: 红包信息 + 奖池一次性写入(NX 幂等)
if redis.call("SET", infoKey, infoValue, "NX") then
    redis.call("LPUSH", poolKey, unpack(listValues))   -- 把 N 个红包记录压进 list
    redis.call("EXPIRE", poolKey, expireTime)
    redis.call("EXPIRE", infoKey, expireTime)
    return 1
else
    return 0
end
```

抢红包 `dal/kv/red_packet.go:230-241`(对照 redislua demo,整段原子):

```lua
-- GrabScript: KEYS[1]=奖池队列 KEYS[2]=已领用户集合 ARGV[1]=userID
if redis.call('SISMEMBER', KEYS[2], ARGV[1]) == 1 then
    return {err = "GRABBED"}   -- 幂等:已抢过
end
local record = redis.call('RPOP', KEYS[1])
if not record then
    return {err = "NO_STOCK"}  -- 弹空了 = 没库存
end
redis.call('SADD', KEYS[2], ARGV[1])   -- 标记已抢
redis.call("EXPIRE", KEYS[2], ARGV[2])
return record                          -- 返回具体红包(含金额+recordID)
```

抢成功后异步落库 + 补偿:`service/red_packet/room_red_packet.go:246→260`(Redis 挡洪峰,DB 最终账本),失败有 `handler/red_packet/compensate.go` 兜底。

---

## 五、架构演进:库存逻辑下沉到中台

- V1(Redis 分片):`coupon_stock.go:154` 注释明确"待下线"。
- V2-Base:真实扣减走 RPC `wallet_promotion_base.WCall.DeductStock`(`service/promo_manage/confirm_consume_promotion.go:50`),查询走 `MGetStockSumData`(`coupon_stock.go:134`)。

```go
// service/promo_manage/confirm_consume_promotion.go:50
_, wErr = wallet_promotion_base.WCall.DeductStock(ctx, &base.DeductStockReq{ ... })
```

> 方向:把"分桶+借库存+对账"这套复杂逻辑从业务侧收敛到**专门的库存中台**,业务只调一个
> `DeductStock`。呼应"热点账户/库存这类通用难题,应由基础设施统一解决,而非每个业务重写"。

---

## 六、一句话总结

手写的 5 个 demo 不是玩具:营销项目把它们**全用上了,而且是更成熟的工业版**——
分片计数器(bucket)、随机均衡 + 借库存(stock 的 borrow)、求和对账(merge)、
Redis 预扣 + 异步落库 + 补偿(redislua + 最终账本)、标准原子 UPDATE 带热点更新 hint(dbdeduct),
最终演进到库存中台化。红包则额外用了"物化奖池 RPOP"这种比减计数器更聪明的模型。

---

## 原文链接 / 关联

> 仅本地留底,不外发。

- 内部代码:`webcast/wallet_promotion_core`(`service/coupon/record/`、`dal/db/coupon_stock.go`、`dal/kv/red_packet.go`、`service/stock/coupon_stock.go`)
- 本地 demo:`yuntwo/go_learning/hotrow/`(single / bucket / stock / dbdeduct / redislua)
- 相关笔记:[[single_row_hotspot_tps_deep_dive]]、`mysql_hotspot_row_optimization.md`
- 腾讯云《利用热点更新技术应对秒杀场景》: https://www.tencentcloud.com/zh/document/product/237/13402
