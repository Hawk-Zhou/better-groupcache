package main

import (
	"context"
	"time"

	geecache "github.com/Hawk-Zhou/better-groupcache"
)

// used in tests, start up by ./script.sh
func main() {
	g := geecache.NewGroup("getRemoteGroup", 10, geecache.GetterFunc(func(key string) ([]byte, error) {
		// don't change this, it's relied on by a test
		return []byte("remote"), nil
	}))
	p := geecache.NewHTTPPool(4971)
	s := p.NewServer()
	g.RegisterPeers(p)

	go func() {
		s.ListenAndServe()
	}()

	time.Sleep(5 * time.Second)
	s.Shutdown(context.TODO())
}
