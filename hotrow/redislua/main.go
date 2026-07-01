// redislua 演示秒杀最常用的扣减方案:Redis + Lua 原子扣减。
//
// 为什么用 Lua:Redis 单线程执行,一段 Lua 脚本【整体原子】,中途不会插入别的命令。
// 于是"读库存→判断够不够→扣减"这三步合成一次原子操作、一次网络往返,
// 既不会超卖(对比 GET 再 DECR 会有竞态),又把热点全挡在内存里、不压 DB。
//
// 这正是前面三个 demo 的思想搬到内存:
//   - 单 key 原子扣减     = single 的"最小持锁",但快几个数量级(内存 vs 行锁)
//   - 多 key(分桶)+ 借库存 = bucket/stock 的拆分与借库存,用一段 Lua 原子完成
//
// 本文件用内存模拟 Redis 的原子语义即可直接运行(不依赖装 Redis)。
// 真实接入见文件末尾打印的 Lua 脚本 + go-redis 调用参考。
//
// 运行: go run ./hotrow/redislua
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// ───────────────────────────────────────────────────────────────────
// 真实生产用的 Lua 脚本(下面的内存模拟严格按这两段脚本的语义实现)
// ───────────────────────────────────────────────────────────────────

// 单 key 原子扣减:KEYS[1]=库存key, ARGV[1]=数量。返回 1成功/0不足/-1不存在。
const luaDeduct = `
local stock = redis.call('GET', KEYS[1])
if stock == false then return -1 end
if tonumber(stock) < tonumber(ARGV[1]) then return 0 end
redis.call('DECRBY', KEYS[1], ARGV[1])
return 1`

// 多 key 分桶 + 借库存:KEYS=N个桶key, ARGV[1]=数量。
// 依次扫描,从第一个有货的桶扣减,返回命中的桶序号(1..N);全空返回 0。
// 整段在 Redis 里原子执行,所以"借库存"天然不会和别的请求竞态。
const luaDeductBucket = `
for i = 1, #KEYS do
  local s = redis.call('GET', KEYS[i])
  if s ~= false and tonumber(s) >= tonumber(ARGV[1]) then
    redis.call('DECRBY', KEYS[i], ARGV[1])
    return i
  end
end
return 0`

// ───────────────────────────────────────────────────────────────────
// 内存模拟 Redis:用一把全局锁代表 Redis 的单线程,脚本体在锁内执行 = 原子。
// (真实环境把下面两个方法换成 rdb.Eval(ctx, lua脚本, keys, n) 即可)
// ───────────────────────────────────────────────────────────────────
type fakeRedis struct {
	mu sync.Mutex
	m  map[string]int64
}

func newFakeRedis() *fakeRedis { return &fakeRedis{m: map[string]int64{}} }

func (r *fakeRedis) set(k string, v int64) { r.m[k] = v }

// evalDeduct 模拟 luaDeduct 的原子语义。
func (r *fakeRedis) evalDeduct(key string, n int64) int {
	r.mu.Lock() // 代表 Redis 单线程:整段原子
	defer r.mu.Unlock()
	s, ok := r.m[key]
	if !ok {
		return -1
	}
	if s < n {
		return 0
	}
	r.m[key] = s - n
	return 1
}

// evalDeductBucket 模拟 luaDeductBucket 的原子语义(分桶 + 借库存)。
func (r *fakeRedis) evalDeductBucket(keys []string, n int64) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, k := range keys {
		if s, ok := r.m[k]; ok && s >= n {
			r.m[k] = s - n
			return i + 1
		}
	}
	return 0
}

const (
	totalStock  = 10000
	concurrency = 256
	requests    = 25000 // > 库存,需求饱和
	numBuckets  = 8
)

// 并发跑 requests 个请求,deduct 返回 true 表示扣减成功。
func hammer(deduct func() bool) (sold, failed int64) {
	var reqLeft = int64(requests)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for atomic.AddInt64(&reqLeft, -1) >= 0 {
				if deduct() {
					atomic.AddInt64(&sold, 1)
				} else {
					atomic.AddInt64(&failed, 1)
				}
			}
		}()
	}
	wg.Wait()
	return
}

func main() {
	fmt.Printf("Redis+Lua 原子扣减:库存=%d 并发=%d 请求=%d\n\n", totalStock, concurrency, requests)

	// ① 单 key 原子扣减
	r1 := newFakeRedis()
	r1.set("stock:1001", totalStock)
	sold, failed := hammer(func() bool {
		return r1.evalDeduct("stock:1001", 1) == 1
	})
	fmt.Println("① 单 key 原子扣减(luaDeduct)")
	fmt.Printf("   售出=%d 失败=%d 剩余=%d  对账 售出+剩余=%d → %v\n\n",
		sold, failed, r1.m["stock:1001"], sold+r1.m["stock:1001"], sold+r1.m["stock:1001"] == totalStock)

	// ② 分桶 + 借库存,一段 Lua 原子完成
	r2 := newFakeRedis()
	keys := make([]string, numBuckets)
	for i := 0; i < numBuckets; i++ {
		keys[i] = fmt.Sprintf("stock:1001:bucket:%d", i)
		r2.set(keys[i], totalStock/numBuckets)
	}
	sold2, failed2 := hammer(func() bool {
		return r2.evalDeductBucket(keys, 1) > 0
	})
	var remain int64
	for _, k := range keys {
		remain += r2.m[k]
	}
	fmt.Println("② 分桶 + 借库存(luaDeductBucket,借库存在脚本内原子完成)")
	fmt.Printf("   售出=%d 失败=%d 合并剩余=%d  对账 售出+剩余=%d → %v\n\n",
		sold2, failed2, remain, sold2+remain, sold2+remain == totalStock)

	fmt.Println("两种方式都满足 售出+剩余==初始 → Lua 整段原子,绝不超卖。\n")

	fmt.Println("──────── 真实接入(go-redis) ────────")
	fmt.Println("// go get github.com/redis/go-redis/v9")
	fmt.Println("rdb := redis.NewClient(&redis.Options{Addr: \"127.0.0.1:6379\"})")
	fmt.Println("n, err := rdb.Eval(ctx, luaDeduct, []string{\"stock:1001\"}, 1).Int()")
	fmt.Println("// n==1 成功 / n==0 库存不足 / n==-1 key不存在")
	fmt.Println("// 生产建议用 redis.NewScript(luaDeduct),首次 EVAL,之后走 EVALSHA 省带宽")
	fmt.Println("\n单 key Lua:")
	fmt.Println(luaDeduct)
	fmt.Println("\n分桶+借库存 Lua:")
	fmt.Println(luaDeductBucket)
}
