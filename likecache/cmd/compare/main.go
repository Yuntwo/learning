package main

import (
	"fmt"
	"slices"
	"strings"

	"likecache-demo/internal/store"
)

type feedScenario struct {
	userID        int64
	feedContents  []int64
	likeContents  []int64
	unlikeContent int64
}

func main() {
	fmt.Println("社区点赞缓存设计优化探索: v2 Set vs v4 Hash")
	fmt.Println(strings.Repeat("=", 68))
	fmt.Println("场景: 用同一组点赞事件,对比内容维度 Set 与用户维度 Hash 在 Feed 批量判断中的差异。")
	fmt.Println()

	scenarios := []feedScenario{
		{userID: 101, feedContents: []int64{1001, 1002, 1003, 1004, 1005, 1006}, likeContents: []int64{1001, 1002, 1004, 1007, 1008, 1009, 1010}},
		{userID: 102, feedContents: []int64{1001, 1002, 1003, 1004, 1005, 1006}, likeContents: []int64{1001, 1003, 1004, 1010, 1011, 1012}},
		{userID: 103, feedContents: []int64{1001, 1002, 1003, 1004, 1005, 1006}, likeContents: []int64{1001, 1002, 1003, 1008, 1013, 1014}},
		{userID: 104, feedContents: []int64{1001, 1002, 1003, 1004, 1005, 1006}, likeContents: []int64{1002, 1003, 1004, 1005, 1015, 1016}},
	}

	stores := []store.LikeStore{
		store.NewV2SetStore(),
		store.NewV4HashStore(),
	}

	for _, s := range stores {
		runScenario(s, scenarios)
		fmt.Println(strings.Repeat("-", 68))
	}
}

func runScenario(s store.LikeStore, scenarios []feedScenario) {
	fmt.Printf("方案: %s\n", s.SchemeName())
	for _, line := range s.DebugState() {
		fmt.Printf("  - %s\n", line)
	}

	seedWorkload(s, scenarios)
	s.ResetMetrics()

	fmt.Println("\n  批量 Feed 判断结果:")
	for _, scenario := range scenarios {
		liked := s.BatchLiked(scenario.userID, scenario.feedContents)
		fmt.Printf("    user=%d feed=%v -> liked=%s\n", scenario.userID, scenario.feedContents, likedSummary(liked))
	}

	fmt.Println("\n  点赞/取消赞与计数检查:")
	first := scenarios[0]
	s.Unlike(first.userID, first.likeContents[0])
	s.Like(first.userID, first.likeContents[0])
	for _, cid := range []int64{1001, 1002, 1004, 1010} {
		fmt.Printf("    content=%d likeCount=%d\n", cid, s.LikeCount(cid))
	}

	metrics := s.Metrics()
	fmt.Println("\n  对比摘要:")
	fmt.Printf("    readOps=%d writeOps=%d ttlOps=%d totalOps=%d\n", metrics.ReadOps, metrics.WriteOps, metrics.TTLOps, metrics.TotalOps())
	fmt.Printf("    coldPathFallbacks=%d keyCount=%d\n", metrics.ColdPathFallbacks, metrics.KeyCount)
	fmt.Printf("    hotspot=%s\n", metrics.HotspotSummary)
	fmt.Println("    insight:")
	if metrics.TTLOps > 0 {
		fmt.Println("      * Feed 中每篇内容都要单独判断,还可能顺手维护 TTL,读路径会被放大。")
	} else {
		fmt.Println("      * 同一用户看一批内容时,状态缓存聚合在 user hash 上,更接近一次批量读取。")
	}
	if metrics.ColdPathFallbacks > 0 {
		fmt.Println("      * v4 通过 minCid 把冷内容踢出热缓存范围,老内容不再长期占住高价值缓存空间。")
	}
	fmt.Println()
}

func seedWorkload(s store.LikeStore, scenarios []feedScenario) {
	for _, scenario := range scenarios {
		for _, cid := range scenario.likeContents {
			s.Like(scenario.userID, cid)
		}
	}
}

func likedSummary(m map[int64]bool) string {
	ids := make([]int64, 0, len(m))
	for cid := range m {
		ids = append(ids, cid)
	}
	slices.Sort(ids)

	parts := make([]string, 0, len(ids))
	for _, cid := range ids {
		if m[cid] {
			parts = append(parts, fmt.Sprintf("%d:赞", cid))
		} else {
			parts = append(parts, fmt.Sprintf("%d:否", cid))
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
