package store

import (
	"fmt"
)

// V2SetStore 模拟文章 v2.0 的内容维度 Set 方案。
//
// 关键特征:
//   - 每篇内容维护一个 likes Set
//   - 是否点赞用 SISMEMBER 判断
//   - 点赞数用 SCARD 统计
//   - 热点内容会频繁发生 TTL 查询/续期
//
// 这里不直接连 Redis,而是用内存结构 + 命令计数模拟访问模式,
// 方便把关注点放在“命令数量 / key 组织方式 / 批量查询放大”上。
type V2SetStore struct {
	likes      map[int64]map[int64]struct{}
	ttlRefresh map[int64]int
	metrics    Metrics
}

func NewV2SetStore() *V2SetStore {
	return &V2SetStore{
		likes:      make(map[int64]map[int64]struct{}),
		ttlRefresh: make(map[int64]int),
	}
}

func (s *V2SetStore) SchemeName() string { return "v2-content-set" }

func (s *V2SetStore) Like(userID, contentID int64) {
	s.metrics.WriteOps++ // SADD
	set := s.ensureSet(contentID)
	if _, exists := set[userID]; !exists {
		set[userID] = struct{}{}
	}

	// 模拟:新增点赞时会去维护 TTL。
	s.metrics.TTLOps++
	s.ttlRefresh[contentID]++
}

func (s *V2SetStore) Unlike(userID, contentID int64) {
	s.metrics.WriteOps++ // SREM
	if set, ok := s.likes[contentID]; ok {
		delete(set, userID)
	}

	s.metrics.TTLOps++
	s.ttlRefresh[contentID]++
}

func (s *V2SetStore) BatchLiked(userID int64, contentIDs []int64) map[int64]bool {
	result := make(map[int64]bool, len(contentIDs))
	for _, contentID := range contentIDs {
		s.metrics.ReadOps++ // SISMEMBER content:{cid}:likes {uid}
		if set, ok := s.likes[contentID]; ok {
			_, result[contentID] = set[userID]
		} else {
			result[contentID] = false
		}

		// 模拟文章里旧方案对热点内容还会做 TTL 检查/续期,
		// 这会让读路径命令数继续膨胀。
		s.metrics.TTLOps++
		s.ttlRefresh[contentID]++
	}
	return result
}

func (s *V2SetStore) LikeCount(contentID int64) int {
	s.metrics.ReadOps++ // SCARD
	if set, ok := s.likes[contentID]; ok {
		return len(set)
	}
	return 0
}

func (s *V2SetStore) Metrics() Metrics {
	m := s.metrics
	m.KeyCount = len(s.likes)
	m.HotspotSummary = fmt.Sprintf("热点集中在 content key；示例 hot key=content:%d:likes，TTL续期次数=%d", hottestContentTTL(s.ttlRefresh), hottestTTLRefresh(s.ttlRefresh))
	return m
}

func (s *V2SetStore) ResetMetrics() { s.metrics = Metrics{} }

func (s *V2SetStore) DebugState() []string {
	return []string{
		"结构: content:{cid}:likes -> Set(userID)",
		"状态查询: 每个 content 一次 SISMEMBER",
		"计数查询: SCARD",
		"热点代价: content 维度 key 容易变大,读写路径还会顺带维护 TTL",
	}
}

func (s *V2SetStore) ensureSet(contentID int64) map[int64]struct{} {
	set, ok := s.likes[contentID]
	if !ok {
		set = make(map[int64]struct{})
		s.likes[contentID] = set
	}
	return set
}

func hottestContentTTL(counter map[int64]int) int64 {
	var (
		hotID int64
		max   int
	)
	for contentID, c := range counter {
		if c > max {
			max = c
			hotID = contentID
		}
	}
	return hotID
}

func hottestTTLRefresh(counter map[int64]int) int {
	max := 0
	for _, c := range counter {
		if c > max {
			max = c
		}
	}
	return max
}
