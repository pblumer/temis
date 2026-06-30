package service

import (
	"container/list"
	"sync"
)

// defaultCacheSize bounds the model cache by default, so a long-running server
// fed an unbounded stream of distinct models stays within memory rather than
// growing without limit (WP-35).
const defaultCacheSize = 256

// modelCache is a bounded, content-addressed LRU cache of compiled models. It is
// safe for concurrent use. A capacity of 0 means unbounded (no eviction).
//
// Because keys are content hashes, re-uploading a changed model lands under a
// new key and is recompiled automatically (hot reload), while re-uploading an
// unchanged one is served from cache and refreshed as most-recently-used. The
// LRU eviction bounds the working set so stale versions do not accumulate.
type modelCache struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List               // front = most recently used; values are *storedModel
	items    map[string]*list.Element // id → element
	nextSeq  uint64                   // monotonic creation order stamped onto each new model
}

func newModelCache(capacity int) *modelCache {
	if capacity < 0 {
		capacity = 0
	}
	return &modelCache{capacity: capacity, ll: list.New(), items: map[string]*list.Element{}}
}

// get returns the cached model for id, marking it most-recently-used.
func (c *modelCache) get(id string) (*storedModel, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[id]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*storedModel), true
}

// add stores sm under its id as most-recently-used, evicting the least-recently-
// used entry when the capacity would be exceeded. Re-adding an existing id
// refreshes its recency and replaces the entry.
func (c *modelCache) add(sm *storedModel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[sm.id]; ok {
		// Same content re-stored: keep its original creation order.
		sm.seq = el.Value.(*storedModel).seq
		el.Value = sm
		c.ll.MoveToFront(el)
		return
	}
	c.nextSeq++
	sm.seq = c.nextSeq
	c.items[sm.id] = c.ll.PushFront(sm)
	if c.capacity > 0 && c.ll.Len() > c.capacity {
		if oldest := c.ll.Back(); oldest != nil {
			c.ll.Remove(oldest)
			delete(c.items, oldest.Value.(*storedModel).id)
		}
	}
}

// len reports the number of cached models.
func (c *modelCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// snapshot returns the cached models from most- to least-recently-used. It does
// not change recency, so listing the cache does not perturb eviction order.
func (c *modelCache) snapshot() []*storedModel {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*storedModel, 0, c.ll.Len())
	for el := c.ll.Front(); el != nil; el = el.Next() {
		out = append(out, el.Value.(*storedModel))
	}
	return out
}
