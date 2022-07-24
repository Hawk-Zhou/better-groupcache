package geecache

import (
	"geecache/lru_k"
	"sync"
)

// cache is a thread safe encapsulation of lru cache
type cache struct {
	// the reason why we don't use RWMutex
	// is that even Get() will write something (to maintain lru-2)
	mu       sync.Mutex
	lru      *lru_k.KCache
	maxBytes int
}

// thread safe
func (c *cache) get(key string) (value ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		c.lru = lru_k.NewK(c.maxBytes, func(s string, v lru_k.Value) {})
		return
	}
	// Be very careful with ok pattern,
	// don't omit it and return, CHECK IT!
	// if we write
	// 	return ret.(ByteView), ok
	// when ret is nil, we will screw up hard.
	ret, ok := c.lru.Get(key)
	if !ok {
		return ByteView{}, ok
	}
	return ret.(ByteView), ok
}

// thread safe
func (c *cache) add(key string, value ByteView) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		c.lru = lru_k.NewK(c.maxBytes, nil)
	}
	return c.lru.Add(key, value)
}
