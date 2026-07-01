// stock 演示真实库存分桶里绕不开的两件事:借库存(匀桶) + 合并对账。
//
// 背景:把总库存拆成 N 个桶分散扣减(解决单行热点,见 ../single 与 ../bucket)。
// 但拆开后会冒出新问题:请求分布不均时,某些桶先卖空,而别的桶还有货 ——
// 如果只认"本桶",就会把还有货的商品误报"售罄",即【少卖】。
//
// 真实系统因此必须配两步:
//   1) 借库存(rebalance/匀桶):本桶空了,去别的桶扣。
//   2) 合并对账(merge):卖完把所有桶剩余【求和归集】,核对 售出+剩余==初始。
//
// 本 demo 用倾斜的请求分布,对照"仅本桶" vs "借库存"两种策略,把少卖跑出来。
//
// 运行: go run ./hotrow/stock
package main

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
)

type bucket struct {
	mu  sync.Mutex
	val int64 // 该桶当前剩余库存
	_   [56]byte
}

const (
	totalStock  = 10000 // 总库存
	numBuckets  = 8      // 拆成 8 个桶
	concurrency = 256    // 并发抢购
	requests    = 25000  // 请求数 > 库存,模拟"远超库存的抢购量"
)

// split 把总库存平均拆进 N 个桶(初始很均匀,问题出在后面的"扣减分布"不均)。
func split(total int64, n int) []bucket {
	bs := make([]bucket, n)
	per := total / int64(n)
	rem := total % int64(n)
	for i := range bs {
		bs[i].val = per
		if int64(i) < rem {
			bs[i].val++
		}
	}
	return bs
}

// merge 合并对账:把所有桶剩余求和归集(模拟卖完后写回一个权威总数)。
func merge(bs []bucket) (remaining int64) {
	for i := range bs {
		remaining += bs[i].val // 真实场景这里是把各子账户/子库存聚合
	}
	return
}

// pickSkewed 返回一个倾斜的桶下标:60% 概率打到 0 号桶,模拟热点 key 分布不均
// (真实里就是按 user_id/商品维度 hash,天然不均)。
func pickSkewed(r *rand.Rand, n int) int {
	if r.Float64() < 0.6 {
		return 0
	}
	return r.Intn(n)
}

// deductOwnOnly 策略A(无借库存):只在自己这个桶扣;本桶空了就报售罄(会少卖)。
func deductOwnOnly(bs []bucket, b int) bool {
	bk := &bs[b]
	bk.mu.Lock()
	defer bk.mu.Unlock()
	if bk.val > 0 {
		bk.val--
		return true
	}
	return false
}

// deductWithBorrow 策略B(借库存):本桶空了,扫描其他桶,从有货的桶里扣。
// 注意:任何时刻只持有一把桶锁(扫描时逐个 lock/unlock),不会死锁。
func deductWithBorrow(bs []bucket, b int) bool {
	if deductOwnOnly(bs, b) {
		return true
	}
	for j := range bs { // 本桶没货,去别的桶借
		if j == b {
			continue
		}
		if deductOwnOnly(bs, j) {
			return true
		}
	}
	return false // 真的全卖光了,才报售罄
}

func run(name string, deduct func(bs []bucket, b int) bool) {
	bs := split(totalStock, numBuckets)
	var (
		sold, failed int64
		reqLeft      = int64(requests)
		wg           sync.WaitGroup
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(id) + 1)) // 每个 worker 独立随机源
			for atomic.AddInt64(&reqLeft, -1) >= 0 {     // 抢完 requests 个请求为止
				if deduct(bs, pickSkewed(r, numBuckets)) {
					atomic.AddInt64(&sold, 1)
				} else {
					atomic.AddInt64(&failed, 1)
				}
			}
		}(i)
	}
	wg.Wait()

	remaining := merge(bs) // ← 合并对账

	fmt.Printf("【%s】\n", name)
	fmt.Printf("  初始库存=%d  请求数=%d\n", totalStock, requests)
	fmt.Printf("  成功售出=%d  报售罄失败=%d\n", sold, failed)
	fmt.Print("  各桶剩余=[")
	for i := range bs {
		if i > 0 {
			fmt.Print(" ")
		}
		fmt.Printf("%d", bs[i].val)
	}
	fmt.Printf("]  合并剩余=%d\n", remaining)
	// 对账1:守恒,售出 + 剩余 必须等于初始(否则超卖或凭空多扣)
	fmt.Printf("  对账 售出+剩余=%d (应=%d) → %v\n",
		sold+remaining, totalStock, sold+remaining == totalStock)
	// 对账2:少卖检测,还有货却报了售罄,就是少卖
	if failed > 0 && remaining > 0 {
		fmt.Printf("  ❌ 少卖!还有 %d 件在桶里,却已对外报售罄(请求分布不均所致)\n", remaining)
	} else if remaining == 0 {
		fmt.Printf("  ✅ 全部售罄,无少卖\n")
	}
	fmt.Println()
}

func main() {
	fmt.Printf("库存分桶:总库存=%d 拆成 %d 桶,并发=%d,请求=%d(远超库存,需求饱和)\n\n",
		totalStock, numBuckets, concurrency, requests)

	run("策略A 仅本桶(无借库存) —— 会少卖", deductOwnOnly)
	run("策略B 借库存+合并对账 —— 不少卖", deductWithBorrow)

	fmt.Println("结论:")
	fmt.Println("1) 拆桶解决了写热点,但带来'库存倾斜':热门桶先空,冷桶还有货。")
	fmt.Println("2) 只认本桶 → 把有货商品误报售罄 = 少卖(对账时 剩余>0 却已失败)。")
	fmt.Println("3) 借库存(匀桶)把扣减兜底到有货的桶 → 卖光为止,不少卖。")
	fmt.Println("4) 两策略都满足'售出+剩余==初始':分桶不会超卖(每桶扣减都在桶锁内校验)。")
	fmt.Println("5) 合并对账(求和归集)是分桶的必备收尾:既核对守恒,又得到权威总数。")
}
