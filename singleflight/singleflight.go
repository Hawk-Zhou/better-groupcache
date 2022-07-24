package singleflight

import (
	"fmt"
	"log"
	"sync"
)

// only access with pointer
type call struct {
	wg  sync.WaitGroup
	ret interface{}
	err error
}

// should only be referred with pointer
type Group struct {
	mu sync.Mutex
	m  map[string]*call
}

func (g *Group) Do(key string, fn func() (interface{}, error)) (ret interface{}, retErr error) {
	g.mu.Lock()

	if g.m == nil {
		g.m = make(map[string]*call)
	}

	c, ok := g.m[key]

	// the call is duplicated
	if ok {
		g.mu.Unlock() // release lock
		log.Printf("[singleflight.Do] Blocked dup %s", key)
		c.wg.Wait()
		return c.ret, c.err
	}

	// this is a new call
	log.Printf("[singleflight.Do] Creating new call %s", key)
	c = new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			log.Println("[singleflight.Do] recovered from panic:", r)
			c.err = fmt.Errorf("call panicked and recovered in single flight: %s", r)
			c.wg.Done() // give clearance to all other Do()s waiting on this
			retErr = c.err
		}
		g.Forget(key)
		log.Printf("[singleflight.Do] Cleaning up call %s", key)
	}()

	log.Printf("[singleflight.Do] Doing new call %s", key)
	c.ret, c.err = fn()
	c.wg.Done()

	return c.ret, c.err
}

func (g *Group) Forget(key string) {
	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()
}
