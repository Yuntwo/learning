package store

import (
	"fmt"
	"slices"
)

// V4HashStore 模拟文章 v4.0 的用户维度 Hash 方案。
//
// 关键特征:
//   - user:{uid}:likes -> Hash
//   - field 存最近点赞过的 contentID
//   - ttl/minCid 作为额外元数据,用于缓存有效期与冷热边界判断
//   - 内容点赞总数从状态缓存里拆开,单独交给 counter service / counter key
//
// 为了突出“批量判断某个用户对多篇内容是否点赞”的优势,
// 这里把一次 BatchLiked 视作一次用户维度读操作。
type V4HashStore struct {
	userLikes     map[int64]map[int64]struct{}
	userMeta      map[int64]UserMeta
	contentCounts map[int64]int
	metrics       Metrics
}

type UserMeta struct {
	TTLMarker int64
	MinCID    int64
}

func NewV4HashStore() *V4HashStore {
	return &V4HashStore{
		userLikes:     make(map[int64]map[int64]struct{}),
		userMeta:      make(map[int64]UserMeta),
		contentCounts: make(map[int64]int),
	}
}

func (s *V4HashStore) SchemeName() string { return "v4-user-hash" }

func (s *V4HashStore) Like(userID, contentID int64) {
	hash := s.ensureUserHash(userID)
	if _, exists := hash[contentID]; exists {
		return
	}

	s.metrics.WriteOps++ // HSET content field
	hash[contentID] = struct{}{}
	s.contentCounts[contentID]++ // 独立计数服务更新
	s.metrics.WriteOps++

	s.touchMeta(userID)
}

func (s *V4HashStore) Unlike(userID, contentID int64) {
	hash := s.ensureUserHash(userID)
	if _, exists := hash[contentID]; !exists {
		return
	}

	s.metrics.WriteOps++ // HDEL
	delete(hash, contentID)
	if s.contentCounts[contentID] > 0 {
		s.contentCounts[contentID]--
	}
	s.metrics.WriteOps++ // 独立计数服务更新

	s.touchMeta(userID)
}

func (s *V4HashStore) BatchLiked(userID int64, contentIDs []int64) map[int64]bool {
	result := make(map[int64]bool, len(contentIDs))
	hash, ok := s.userLikes[userID]
	meta := s.userMeta[userID]

	// 一次用户维度批量读取,近似对应 HMGET / H[M]SCAN 风格的单次 Redis 交互。
	s.metrics.ReadOps++

	for _, contentID := range contentIDs {
		if !ok {
			result[contentID] = false
			continue
		}
		if meta.MinCID != 0 && contentID < meta.MinCID {
			// 冷数据不在用户 hash 中维护,视为走冷路径。
			s.metrics.ColdPathFallbacks++
			result[contentID] = false
			continue
		}
		_, result[contentID] = hash[contentID]
	}

	return result
}

func (s *V4HashStore) LikeCount(contentID int64) int {
	// 点赞总数从用户状态缓存拆出来,走独立计数 key。
	s.metrics.ReadOps++
	return s.contentCounts[contentID]
}

func (s *V4HashStore) Metrics() Metrics {
	m := s.metrics
	m.KeyCount = len(s.userLikes) + len(s.contentCounts)
	m.HotspotSummary = fmt.Sprintf("状态分散在 user hash；用户 hash=%d, 独立计数 key=%d, 最小minCid=%d", len(s.userLikes), len(s.contentCounts), s.globalMinCID())
	return m
}

func (s *V4HashStore) ResetMetrics() { s.metrics = Metrics{} }

func (s *V4HashStore) DebugState() []string {
	return []string{
		"结构1: user:{uid}:likes -> Hash(contentID=1, ttl=..., minCid=...)",
		"结构2: content:{cid}:like_count -> String/Counter",
		"状态查询: 同一用户看一批内容时,按用户维度集中读取一次",
		"热点收益: 热点不再集中砸到单个 content Set,而是分散到活跃用户 Hash",
	}
}

func (s *V4HashStore) ensureUserHash(userID int64) map[int64]struct{} {
	hash, ok := s.userLikes[userID]
	if !ok {
		hash = make(map[int64]struct{})
		s.userLikes[userID] = hash
	}
	return hash
}

func (s *V4HashStore) touchMeta(userID int64) {
	hash := s.userLikes[userID]
	ids := make([]int64, 0, len(hash))
	for contentID := range hash {
		ids = append(ids, contentID)
	}
	slices.Sort(ids)

	meta := UserMeta{TTLMarker: int64(len(ids))}
	if len(ids) > 0 {
		meta.MinCID = ids[0]
	}
	if len(ids) > 6 {
		// 只保留最近一段时间 / 一定数量的 contentID,借 minCid 做冷热边界。
		meta.MinCID = ids[len(ids)-6]
	}
	s.userMeta[userID] = meta
}

func (s *V4HashStore) globalMinCID() int64 {
	var min int64
	for _, meta := range s.userMeta {
		if meta.MinCID == 0 {
			continue
		}
		if min == 0 || meta.MinCID < min {
			min = meta.MinCID
		}
	}
	return min
}
