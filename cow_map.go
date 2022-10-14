package dscope

import (
	"sync"
	"sync/atomic"
)

type CowMap[K comparable, V any] struct {
	mutex sync.Mutex
	value atomic.Pointer[map[K]V]
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

func (c *CowMap[K, V]) Set(k K, v V) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	ptr := c.value.Load()
	newMap := make(map[K]V)
	for k, v := range *ptr {
		newMap[k] = v
	}
	newMap[k] = v
	c.value.Store(&newMap)
}
