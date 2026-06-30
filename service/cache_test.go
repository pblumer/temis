package service

import "testing"

func sm(id string) *storedModel { return &storedModel{id: id} }

func TestModelCacheStampsCreationSeq(t *testing.T) {
	c := newModelCache(0)
	a, b := sm("a"), sm("b")
	c.add(a)
	c.add(b)
	if a.seq == 0 || b.seq <= a.seq {
		t.Fatalf("seq should increase per new model: a=%d b=%d", a.seq, b.seq)
	}
	// Re-storing the same id keeps its original creation order (history is stable).
	a2 := sm("a")
	c.add(a2)
	if a2.seq != a.seq {
		t.Errorf("re-add seq = %d, want preserved %d", a2.seq, a.seq)
	}
}

func TestModelCacheEvictsLRU(t *testing.T) {
	c := newModelCache(2)
	c.add(sm("a"))
	c.add(sm("b"))
	c.add(sm("c")) // evicts a (least recently used)

	if _, ok := c.get("a"); ok {
		t.Error("a should have been evicted")
	}
	if _, ok := c.get("b"); !ok {
		t.Error("b should still be cached")
	}
	if _, ok := c.get("c"); !ok {
		t.Error("c should still be cached")
	}
	if c.len() != 2 {
		t.Errorf("len = %d, want 2", c.len())
	}
}

func TestModelCacheRecencyRefreshOnGet(t *testing.T) {
	c := newModelCache(2)
	c.add(sm("a"))
	c.add(sm("b"))
	c.get("a")     // a is now most-recently-used
	c.add(sm("c")) // evicts b, not a

	if _, ok := c.get("a"); !ok {
		t.Error("a was refreshed and should survive")
	}
	if _, ok := c.get("b"); ok {
		t.Error("b should have been evicted")
	}
}

func TestModelCacheReaddRefreshes(t *testing.T) {
	c := newModelCache(2)
	c.add(sm("a"))
	c.add(sm("a")) // same id: refresh, not a second entry
	if c.len() != 1 {
		t.Errorf("len = %d, want 1", c.len())
	}
}

func TestModelCacheUnbounded(t *testing.T) {
	c := newModelCache(0)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		c.add(sm(id))
	}
	if c.len() != 5 {
		t.Errorf("len = %d, want 5 (unbounded)", c.len())
	}
}

func TestModelCacheSnapshotOrder(t *testing.T) {
	c := newModelCache(0)
	c.add(sm("a"))
	c.add(sm("b"))
	c.get("a") // a -> most recent
	snap := c.snapshot()
	if len(snap) != 2 || snap[0].id != "a" || snap[1].id != "b" {
		t.Errorf("snapshot order = %v, want [a b] (most→least recent)", []string{snap[0].id, snap[1].id})
	}
}
