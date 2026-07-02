package main

import (
	"container/heap"
	"container/list"
	"fmt"
	"sort"
	"time"
)

type item struct {
	key   string
	count float64
}

type minHeap []item

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].count < h[j].count }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)        { *h = append(*h, x.(item)) }
func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

type HotspotDetector struct {
	counts    map[string]float64
	decay     float64
	topK      int
	whitelist map[string]struct{}
}

func NewHotspotDetector(topK int, decay float64, whitelist []string) *HotspotDetector {
	w := make(map[string]struct{}, len(whitelist))
	for _, key := range whitelist {
		w[key] = struct{}{}
	}
	return &HotspotDetector{
		counts:    make(map[string]float64),
		decay:     decay,
		topK:      topK,
		whitelist: w,
	}
}

func (d *HotspotDetector) Tick() {
	for key, count := range d.counts {
		count *= d.decay
		if count < 0.5 {
			delete(d.counts, key)
			continue
		}
		d.counts[key] = count
	}
}

func (d *HotspotDetector) Add(key string, incr float64) bool {
	d.counts[key] += incr
	if _, ok := d.whitelist[key]; ok {
		return true
	}
	return d.IsHot(key)
}

func (d *HotspotDetector) IsHot(key string) bool {
	if _, ok := d.whitelist[key]; ok {
		return true
	}
	_, ok := d.topItemsMap()[key]
	return ok
}

func (d *HotspotDetector) topItemsMap() map[string]item {
	h := &minHeap{}
	heap.Init(h)
	for key, count := range d.counts {
		candidate := item{key: key, count: count}
		if h.Len() < d.topK {
			heap.Push(h, candidate)
			continue
		}
		if (*h)[0].count < count {
			heap.Pop(h)
			heap.Push(h, candidate)
		}
	}
	result := make(map[string]item, h.Len())
	for _, it := range *h {
		result[it.key] = it
	}
	return result
}

func (d *HotspotDetector) TopItems() []item {
	items := make([]item, 0, len(d.counts))
	for _, entry := range d.topItemsMap() {
		items = append(items, entry)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].count > items[j].count
	})
	return items
}

type cacheEntry struct {
	key   string
	value string
}

type LocalCache struct {
	cap   int
	ll    *list.List
	index map[string]*list.Element
}

func NewLocalCache(cap int) *LocalCache {
	return &LocalCache{cap: cap, ll: list.New(), index: make(map[string]*list.Element)}
}

func (c *LocalCache) Get(key string) (string, bool) {
	if ele, ok := c.index[key]; ok {
		c.ll.MoveToFront(ele)
		return ele.Value.(cacheEntry).value, true
	}
	return "", false
}

func (c *LocalCache) Add(key, value string) {
	if ele, ok := c.index[key]; ok {
		ele.Value = cacheEntry{key: key, value: value}
		c.ll.MoveToFront(ele)
		return
	}
	if c.ll.Len() >= c.cap {
		back := c.ll.Back()
		if back != nil {
			delete(c.index, back.Value.(cacheEntry).key)
			c.ll.Remove(back)
		}
	}
	ele := c.ll.PushFront(cacheEntry{key: key, value: value})
	c.index[key] = ele
}

type CacheSDK struct {
	detector *HotspotDetector
	local    *LocalCache
	remote   map[string]string
}

func NewCacheSDK() *CacheSDK {
	remote := map[string]string{
		"room:1":    "normal-room",
		"room:2":    "other-room",
		"event:618": "flash-sale",
		"video:99":  "viral-video",
	}
	return &CacheSDK{
		detector: NewHotspotDetector(3, 0.5, []string{"event:618"}),
		local:    NewLocalCache(3),
		remote:   remote,
	}
}

func (s *CacheSDK) Read(key string) string {
	if value, ok := s.local.Get(key); ok {
		return "local:" + value
	}
	value := s.remote[key]
	if s.detector.Add(key, 1) {
		s.local.Add(key, value)
	}
	return "remote:" + value
}

func main() {
	sdk := NewCacheSDK()

	traffic := [][]string{
		{"room:1", "room:1", "room:2", "event:618"},
		{"room:1", "room:2", "room:2", "event:618"},
		{"room:1", "room:2", "event:618", "video:99", "video:99", "video:99", "video:99"},
		{"video:99", "video:99", "video:99", "video:99", "video:99", "event:618"},
	}

	for second, keys := range traffic {
		fmt.Printf("\n== second %d ==\n", second+1)
		sdk.detector.Tick() // 模拟对历史频次做统一衰减，提高突发热点识别速度
		for _, key := range keys {
			fmt.Printf("read %-9s -> %s\n", key, sdk.Read(key))
		}
		fmt.Println("topk:")
		for _, it := range sdk.detector.TopItems() {
			fmt.Printf("  %-9s score=%.2f\n", it.key, it.count)
		}
		time.Sleep(150 * time.Millisecond)
	}
}
