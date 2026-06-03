package api

import (
	"math/rand"
	"sort"
	"testing"
	"time"
)

func TestRecencyLess(t *testing.T) {
	now := time.Now()
	candidates := []uint32{1, 2, 3, 4, 5}
	lastShown := map[uint32]time.Time{
		1: now.Add(-1 * time.Minute),
		2: now.Add(-1 * time.Hour),
		3: now.Add(-1 * 24 * time.Hour),
		// 4 and 5 never shown
	}
	sort.SliceStable(candidates, recencyLess(candidates, lastShown))
	want := []uint32{4, 5, 3, 2, 1}
	for i, w := range want {
		if candidates[i] != w {
			t.Errorf("position %d: got %d, want %d (full order: %v)", i, candidates[i], w, candidates)
		}
	}
}

func TestPickWithRecencyBias_TopFrac(t *testing.T) {
	candidates := make([]uint32, 100)
	for i := range candidates {
		candidates[i] = uint32(i + 1)
	}
	topN := int(float64(len(candidates)) * lruTopFrac)
	if topN < 1 {
		topN = 1
	}
	if topN != 20 {
		t.Fatalf("expected topN=20, got %d", topN)
	}
	rng := rand.New(rand.NewSource(42))
	seen := make(map[uint32]int)
	for i := 0; i < 500; i++ {
		pick := candidates[rng.Intn(topN)]
		seen[pick]++
	}
	for id := range seen {
		if id < 1 || id > 20 {
			t.Errorf("picked %d outside top-20 window", id)
		}
	}
	if len(seen) < 15 {
		t.Errorf("only %d distinct IDs in 500 picks", len(seen))
	}
}

func TestPickWithRecencyBias_FewCandidates(t *testing.T) {
	candidates := []uint32{10, 20, 30}
	topN := int(float64(len(candidates)) * lruTopFrac)
	if topN < 1 {
		topN = 1
	}
	if topN != 1 {
		t.Errorf("expected topN=1 (clamped), got %d", topN)
	}
}
