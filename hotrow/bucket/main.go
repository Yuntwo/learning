// bucket 演示"余额拆分/分桶":把 1 个热点行拆成 N 个桶(各带各的锁),
// 写入分散到 N 把锁上,从而把单行的吞吐天花板抬高 ~N 倍。
//
// 对照实验:固定高并发(256),只改变桶数,看 TPS 怎么随桶数涨、RT 怎么掉。
// 关键彩蛋:桶数超过 CPU 核数后 TPS 不再线性涨 —— 瓶颈从"锁"转移到了"CPU"。
//
// 运行: go run ./bucket
package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// 每次更新在"持锁"状态下的 CPU 忙等,模拟行锁持有时间(与 hotrow 一致)。
const workPerTxn = 50 * time.Microsecond

func busyWork(d time.Duration) {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
	}
}

// bucket 是一个分桶:独立的锁 + 独立的值。N 个 bucket = 把热点行拆成 N 份。
type bucket struct {
	mu  sync.Mutex
	val int64
	_   [56]byte // 填充,避免相邻桶落在同一 cache line 造成伪共享(false sharing)
}

// runOnce: numBuckets 个桶,concurrency 个 goroutine 并发更新,持续 dur。
// 每个 goroutine 固定打到自己 id%numBuckets 的桶上(模拟按 key 路由到某个子账户)。
func runOnce(numBuckets, concurrency int, dur time.Duration) (done, avgRTns int64) {
	buckets := make([]bucket, numBuckets)
	var (
		totalRT  int64
		stopFlag int32
		wg       sync.WaitGroup
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			b := &buckets[id%numBuckets] // 这个工人固定写某一个桶
			for atomic.LoadInt32(&stopFlag) == 0 {
				start := time.Now()
				b.mu.Lock()
				busyWork(workPerTxn)
				b.val--
				b.mu.Unlock()
				atomic.AddInt64(&totalRT, int64(time.Since(start)))
				atomic.AddInt64(&done, 1)
			}
		}(i)
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
	const (
		dur         = 2 * time.Second
		concurrency = 256 // 固定高并发,只变桶数
	)
	cpu := runtime.NumCPU()

	fmt.Printf("固定并发=%d,持锁=%v,CPU核数=%d\n", concurrency, workPerTxn, cpu)
	fmt.Printf("单桶天花板 ≈ 1/持锁 = %.0f TPS;N 桶理论上限 ≈ N× (但受 CPU 核数封顶)\n\n",
		float64(time.Second)/float64(workPerTxn))

	fmt.Printf("%-8s %-14s %-12s %-12s\n", "桶数", "完成事务数", "实测TPS", "平均RT")
	fmt.Println("--------------------------------------------------------")

	for _, n := range []int{1, 2, 4, 8, 12, 16, 32, 64} {
		total, avgRTns := runOnce(n, concurrency, dur)
		tps := float64(total) / dur.Seconds()
		fmt.Printf("%-8d %-14d %-12.0f %-12v\n", n, total, tps, time.Duration(avgRTns))
	}

	fmt.Println("\n结论:")
	fmt.Println("1) 桶数↑ → TPS 近似线性↑、RT 近似线性↓:拆分把单行天花板抬高了 ~N 倍。")
	fmt.Printf("2) 但桶数超过 CPU 核数(%d)后 TPS 不再涨 —— 瓶颈从'锁'转移到了'CPU'。\n", cpu)
	fmt.Println("   瓶颈只会转移、不会消失:解开一个,就会撞上下一个。")
	fmt.Println("3) 对照 hotrow(单桶):那里 TPS 永远封顶 ~20000,这就是拆分的价值。")
}
