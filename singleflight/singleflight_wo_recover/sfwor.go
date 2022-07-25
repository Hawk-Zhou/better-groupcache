package main

import (
	"log"
	"sync"
	"time"
)

// call is an in-flight or completed Do call
type call struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

// Group represents a class of work and forms a namespace in which
// units of work can be executed with duplicate suppression.
type Group struct {
	mu sync.Mutex       // protects m
	m  map[string]*call // lazily initialized
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in, the duplicate caller waits for the
// original to complete and receives the same results.
func (g *Group) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}

func main() {
	slowJob := func(msg string) ([]byte, error) {
		time.Sleep(time.Millisecond * 500)
		log.Panic("panic from slowJob:", msg)
		return []byte(msg), nil
	}

	counter := 0

	job := func() (interface{}, error) {
		counter++
		var ret interface{}
		ret, err := slowJob("114514")
		return ret, err
	}

	g := &Group{}

	wg := sync.WaitGroup{}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(x int) {

			defer func() {
				recover()
			}()

			ret, err := g.Do("testDo", job)
			println("got ret:", string(ret.([]byte)))
			if err == nil || counter != 1 {
				log.Println("unexpected err/ret/counter")
			}
			wg.Done()
		}(i)
	}

	wg.Wait()
}
