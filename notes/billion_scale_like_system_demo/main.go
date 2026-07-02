package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// billion_scale_like_system_demo 演示:如何把“点赞状态 + 点赞计数 + 用户最近点赞列表”
// 放进一次原子更新里处理。
//
// 为什么不用真实 Redis:
//   - 学习 demo 的目标是先把数据模型和原子语义讲清楚
//   - 用一把 mutex 模拟 Redis 单线程执行 Lua 脚本的效果,本地无需额外环境即可运行
//   - 真正接 Redis 时,把 evalLikeLuaLikeSemantics 换成 EVAL / EVALSHA 即可
//
// 运行: go run ./notes/billion_scale_like_system_demo.go

type recentLike struct {
	itemID string
	ts     int64
}

type fakeRedisLikeStore struct {
	mu        sync.Mutex
	states    map[string]bool             // like:state:{biz}:{user}:{item} -> liked?
	counts    map[string]int64            // like:count:{biz}:{item} -> likes
	userLikes map[string]map[string]int64 // user:likes:{user}:{biz} -> itemID -> ts
	maxRecent int
}

func newFakeRedisLikeStore(maxRecent int) *fakeRedisLikeStore {
	return &fakeRedisLikeStore{
		states:    make(map[string]bool),
		counts:    make(map[string]int64),
		userLikes: make(map[string]map[string]int64),
		maxRecent: maxRecent,
	}
}

func stateKey(bizID, userID, itemID string) string {
	return fmt.Sprintf("like:state:%s:%s:%s", bizID, userID, itemID)
}

func countKey(bizID, itemID string) string {
	return fmt.Sprintf("like:count:%s:%s", bizID, itemID)
}

func userLikesKey(userID, bizID string) string {
	return fmt.Sprintf("user:likes:%s:%s", userID, bizID)
}

// evalLikeLuaLikeSemantics 模拟 Lua 脚本的整段原子语义。
// 返回值与笔记中的脚本保持一致:
//
//	1  -> 点赞成功
//	-1 -> 取消点赞成功
//	0  -> 幂等 no-op (重复点赞 / 重复取消)
//	-2 -> 未知动作
func (s *fakeRedisLikeStore) evalLikeLuaLikeSemantics(action, bizID, userID, itemID string, ts int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	sKey := stateKey(bizID, userID, itemID)
	cKey := countKey(bizID, itemID)
	uKey := userLikesKey(userID, bizID)
	current := s.states[sKey]

	switch action {
	case "like":
		if current {
			return 0
		}
		s.states[sKey] = true
		s.counts[cKey]++
		if s.userLikes[uKey] == nil {
			s.userLikes[uKey] = make(map[string]int64)
		}
		s.userLikes[uKey][itemID] = ts
		s.trimRecentLocked(uKey)
		return 1
	case "unlike":
		if !current {
			return 0
		}
		delete(s.states, sKey)
		if s.counts[cKey] > 0 {
			s.counts[cKey]--
		}
		if likes := s.userLikes[uKey]; likes != nil {
			delete(likes, itemID)
			if len(likes) == 0 {
				delete(s.userLikes, uKey)
			}
		}
		return -1
	default:
		return -2
	}
}

func (s *fakeRedisLikeStore) trimRecentLocked(uKey string) {
	likes := s.userLikes[uKey]
	if len(likes) <= s.maxRecent {
		return
	}
	items := make([]recentLike, 0, len(likes))
	for itemID, ts := range likes {
		items = append(items, recentLike{itemID: itemID, ts: ts})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ts == items[j].ts {
			return items[i].itemID > items[j].itemID
		}
		return items[i].ts > items[j].ts
	})
	for _, item := range items[s.maxRecent:] {
		delete(likes, item.itemID)
	}
}

func (s *fakeRedisLikeStore) count(bizID, itemID string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[countKey(bizID, itemID)]
}

func (s *fakeRedisLikeStore) isLiked(bizID, userID, itemID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.states[stateKey(bizID, userID, itemID)]
}

func (s *fakeRedisLikeStore) recentLikes(userID, bizID string) []recentLike {
	s.mu.Lock()
	defer s.mu.Unlock()
	likes := s.userLikes[userLikesKey(userID, bizID)]
	items := make([]recentLike, 0, len(likes))
	for itemID, ts := range likes {
		items = append(items, recentLike{itemID: itemID, ts: ts})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ts == items[j].ts {
			return items[i].itemID > items[j].itemID
		}
		return items[i].ts > items[j].ts
	})
	return items
}

func (s *fakeRedisLikeStore) recomputeCountFromState(bizID, itemID string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	needle := ":" + itemID
	prefix := "like:state:" + bizID + ":"
	var total int64
	for key, liked := range s.states {
		if liked && strings.HasPrefix(key, prefix) && strings.HasSuffix(key, needle) {
			total++
		}
	}
	return total
}

func printRecent(items []recentLike) {
	if len(items) == 0 {
		fmt.Println("   recent=[]")
		return
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s@%d", item.itemID, item.ts))
	}
	fmt.Printf("   recent=%v\n", parts)
}

func main() {
	const bizID = "archive"
	store := newFakeRedisLikeStore(3)

	fmt.Println("点赞系统原子更新 demo:状态 + 计数 + 最近点赞列表")
	fmt.Println("说明:用内存 + mutex 模拟 Redis Lua 的整段原子语义,无需本地 Redis。\n")

	fmt.Println("① 单用户 like / unlike 幂等性")
	fmt.Printf("   like #1   -> %d\n", store.evalLikeLuaLikeSemantics("like", bizID, "u1001", "item-1", 1001))
	fmt.Printf("   like #2   -> %d (重复点赞幂等)\n", store.evalLikeLuaLikeSemantics("like", bizID, "u1001", "item-1", 1002))
	fmt.Printf("   unlike #1 -> %d\n", store.evalLikeLuaLikeSemantics("unlike", bizID, "u1001", "item-1", 1003))
	fmt.Printf("   unlike #2 -> %d (重复取消幂等)\n", store.evalLikeLuaLikeSemantics("unlike", bizID, "u1001", "item-1", 1004))
	fmt.Printf("   unknown   -> %d\n\n", store.evalLikeLuaLikeSemantics("toggle", bizID, "u1001", "item-1", 1005))

	fmt.Println("② 多用户点赞同一内容,校验计数与状态一致")
	users := []string{"u2001", "u2002", "u2003", "u2004"}
	for i, userID := range users {
		ret := store.evalLikeLuaLikeSemantics("like", bizID, userID, "item-hot", int64(2000+i))
		fmt.Printf("   %s like item-hot -> %d\n", userID, ret)
	}
	fmt.Printf("   storedCount=%d recomputed=%d\n", store.count(bizID, "item-hot"), store.recomputeCountFromState(bizID, "item-hot"))
	fmt.Printf("   u2003 liked? %v\n\n", store.isLiked(bizID, "u2003", "item-hot"))

	fmt.Println("③ 用户最近点赞列表(只保留最近 3 条)")
	store.evalLikeLuaLikeSemantics("like", bizID, "u3001", "item-a", 3001)
	store.evalLikeLuaLikeSemantics("like", bizID, "u3001", "item-b", 3002)
	store.evalLikeLuaLikeSemantics("like", bizID, "u3001", "item-c", 3003)
	store.evalLikeLuaLikeSemantics("like", bizID, "u3001", "item-d", 3004)
	printRecent(store.recentLikes("u3001", bizID))
	fmt.Println("   说明: item-a 被裁剪掉,这对应缓存层只保留最近 N 条点赞记录。\n")

	fmt.Println("④ 小规模热点并发模拟,验证最终 invariant")
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			userID := fmt.Sprintf("hot-user-%02d", worker)
			baseTS := int64(4000 + worker*10)
			store.evalLikeLuaLikeSemantics("like", bizID, userID, "item-super-hot", baseTS)
			store.evalLikeLuaLikeSemantics("like", bizID, userID, "item-super-hot", baseTS+1)
			if worker%2 == 0 {
				store.evalLikeLuaLikeSemantics("unlike", bizID, userID, "item-super-hot", baseTS+2)
			}
			store.evalLikeLuaLikeSemantics("like", bizID, userID, "item-super-hot", baseTS+3)
		}(worker)
	}
	wg.Wait()
	stored := store.count(bizID, "item-super-hot")
	recomputed := store.recomputeCountFromState(bizID, "item-super-hot")
	fmt.Printf("   storedCount=%d recomputed=%d invariant=%v\n\n", stored, recomputed, stored == recomputed)

	fmt.Println("结论:")
	fmt.Println("1) 点赞状态、点赞计数、最近点赞列表必须作为一个原子单元更新。")
	fmt.Println("2) 重复 like / unlike 必须幂等,否则计数很容易写坏。")
	fmt.Println("3) 热点内容的高频写操作,适合先在内存原子层处理,再异步扩散到持久层。\n")

	fmt.Println("──────── 真实接入 Redis + Lua 的参考脚本 ────────")
	fmt.Println(`local action = ARGV[1]
local current = redis.call('GET', KEYS[1])

if action == 'like' then
  if current == '1' then return 0 end
  redis.call('SET', KEYS[1], '1')
  redis.call('HINCRBY', KEYS[2], 'likes', 1)
  redis.call('ZADD', KEYS[3], ARGV[2], ARGV[3])
  return 1
end

if action == 'unlike' then
  if current ~= '1' then return 0 end
  redis.call('SET', KEYS[1], '0')
  redis.call('HINCRBY', KEYS[2], 'likes', -1)
  redis.call('ZREM', KEYS[3], ARGV[3])
  return -1
end

return -2`)
	fmt.Println()
	fmt.Println(`// go get github.com/redis/go-redis/v9
// script := redis.NewScript(likeLua)
// ret, err := script.Run(ctx, rdb, []string{
//     "like:state:archive:u1001:item-1",
//     "like:count:archive:item-1",
//     "user:likes:u1001:archive",
// }, "like", 1710000000000, "item-1").Int()`)
}
