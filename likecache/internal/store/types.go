package store

// Metrics 用来统计不同缓存模型在同一组 workload 下的结构性差异。
type Metrics struct {
	ReadOps           int
	WriteOps          int
	TTLOps            int
	ColdPathFallbacks int
	KeyCount          int
	HotspotSummary    string
}

func (m Metrics) TotalOps() int {
	return m.ReadOps + m.WriteOps + m.TTLOps
}

// LikeStore 抽象两种点赞缓存模型的公共能力,方便 compare 主程序统一驱动。
type LikeStore interface {
	SchemeName() string
	Like(userID, contentID int64)
	Unlike(userID, contentID int64)
	BatchLiked(userID int64, contentIDs []int64) map[int64]bool
	LikeCount(contentID int64) int
	Metrics() Metrics
	ResetMetrics()
	DebugState() []string
}
