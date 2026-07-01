// hotrow 用 sync.Mutex 模拟数据库"单行行锁"的秒杀场景,
// 实测验证:同一行串行更新时,TPS 不是 1,而是被 1/RT 卡住一个天花板;
// 并发越高,锁竞争的额外开销越大,实测 TPS 反而从峰值往下掉。
//
// 运行: go run ./hotrow
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// 每次更新在"持锁"状态下要做的活儿(模拟行锁持有时间 = RT 的核心部分)。
// 这里用一段纯 CPU 忙等来模拟,而不是 sleep —— 因为真实数据库高并发下,
// 抢锁/唤醒/上下文切换都在抢 CPU,忙等能更真实地体现竞争开销。
const workPerTxn = 50 * time.Microsecond

// busyWork 空转约 d 时长,模拟一次更新在临界区内的耗时。
func busyWork(d time.Duration) {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		// 故意空转,占着 CPU,模拟"持锁干活"
	}
}

// runOnce 用 concurrency 个 goroutine 抢同一把锁(模拟抢同一行),
// 持续 dur 时长,返回:完成的事务数、平均RT(纳秒)。
// RT 从"开始尝试拿锁"算到"放锁完成",包含了排队等锁的时间 —— 这才是
// 调用方真正感受到的延迟。
func runOnce(concurrency int, dur time.Duration) (done int64, avgRTns int64) {
	var (
		mu       sync.Mutex // 这就是"那一行"的行锁
		stock    int64      // 模拟库存(热点行的值)
		totalRT  int64      // 所有事务 RT 累加(纳秒)
		stopFlag int32      // 到点了通知所有 goroutine 收工
		wg       sync.WaitGroup
	)
	stock = 1 << 62 // 给足库存,避免提前卖光影响测量

	for i := 0; i < concurrency; i++ { // ← 外层:只跑 concurrency 次,负责【启动】
		wg.Add(1)
		go func() {
			defer wg.Done()
			for atomic.LoadInt32(&stopFlag) == 0 { // ← 内层:每个 goroutine 自己的循环
				start := time.Now() // RT 从这里开始算(含等锁排队)
				// ↓↓↓ 一个"事务":拿锁 → 改库存 → 放锁(成功即提交)
				mu.Lock()
				busyWork(workPerTxn) // 持锁期间的耗时
				stock--
				mu.Unlock()
				// ↑↑↑ 同一时刻只有一个 goroutine 能走完这段
				atomic.AddInt64(&totalRT, int64(time.Since(start)))
				atomic.AddInt64(&done, 1)
			}
		}()
	}

	time.Sleep(dur)
	atomic.StoreInt32(&stopFlag, 1)
	wg.Wait()

	if done > 0 {
		avgRTns = totalRT / done
	}
	return done, avgRTns
}

func main() {
	const dur = 2 * time.Second

	fmt.Printf("模拟单行行锁:每个事务持锁耗时 = %v\n", workPerTxn)
	fmt.Printf("理论 TPS 天花板 ≈ 1/RT = %.0f\n", float64(time.Second)/float64(workPerTxn))
	fmt.Printf("(注意:不是 1,而是这个值;且实测会因竞争开销低于它)\n\n")

	fmt.Printf("%-8s %-14s %-12s %-12s\n", "并发数", "完成事务数", "实测TPS", "平均RT")
	fmt.Println("--------------------------------------------------------")

	for _, c := range []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096} {
		total, avgRTns := runOnce(c, dur)
		tps := float64(total) / dur.Seconds()
		avgRT := time.Duration(avgRTns)
		fmt.Printf("%-8d %-14d %-12.0f %-12v\n", c, total, tps, avgRT)
	}

	fmt.Println("\n结论:")
	fmt.Println("1) 并发=1 时 TPS 就已经接近天花板,说明串行 ≠ TPS=1。")
	fmt.Println("2) 并发再怎么加,TPS 突破不了 1/RT 这个上限(被单行卡死)。")
	fmt.Println("3) TPS 顶住天花板后,继续加并发 → 平均RT 几乎线性上涨")
	fmt.Println("   (Little定律: 并发=TPS×RT,TPS恒定时加并发只会堆高延迟)。")
	fmt.Println("   ——这就是'高并发危害'最干净的信号,比看TPS抖动靠谱。")
}
