package dscope

import (
	"maps"
	"sync"
	"sync/atomic"
)

type CowMap[K comparable, V any] struct {
	value atomic.Pointer[map[K]V]
	mutex sync.Mutex
}

func NewCowMap[K comparable, V any]() *CowMap[K, V] {
	m := make(map[K]V)
	ret := new(CowMap[K, V])
	ret.value.Store(&m)
	return ret
}

func (c *CowMap[K, V]) Get(k K) (v V, ok bool) {
	ptr := c.value.Load()
	v, ok = (*ptr)[k]
	return
}

func (c *CowMap[K, V]) LoadOrStore(k K, v V) (ret V, loaded bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	m := *c.value.Load()
	ret, loaded = m[k]
	if loaded {
		return
	}
	newMap := maps.Clone(m)
	newMap[k] = v
	c.value.Store(&newMap)
	return v, false
}
