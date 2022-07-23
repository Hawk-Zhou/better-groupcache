//go:build testrace
// +build testrace

package geecache

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
)

// hope that go race detects hazard
// test me with go test -race ./.../
func TestParallelRead(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(50)
	maxBytes := 20
	c := cache{maxBytes: maxBytes}
	if runtime.GOMAXPROCS(0) == 1 {
		// indeed this can't detect anything
		// when running on my laptop, I found a error:
		// 		converting nil to byteView @ cache.Get
		// fixed now
		t.Error("we can't detect anything with maxprocs = 1")
		t.SkipNow()
	}
	for i := 0; i < 50; i++ {
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				testStr := fmt.Sprintf("%v", i) + fmt.Sprintf("%v", j)
				valLen := maxBytes - len(testStr)
				val := make([]byte, maxBytes-valLen)
				c.add(testStr, ByteView{b: val})
				c.get(testStr)
			}
		}(i)
	}
}
