package geecache

import (
	"errors"
	"log"
	"sync"
)

// function type is a pattern that allows more than struct
// to be passed to an interface type with easy conversion
// eg. func someGet(key string) ([]byte error)
//		we want to pass it to Getter interface
// 		just convert it with our function type
// 		var foo Getter = GetterFunc(someGet)
//		this GetterFunc() here is type conversion
// another eg.
// 		var example_conversion Getter = GetterFunc(func(key string) ([]byte, error) {
// 			return nil,nil
// 		})

// Getter is a callback function
// 	used to load data to cache if not hit
type Getter interface {
	Get(key string) ([]byte, error)
}

type GetterFunc func(key string) ([]byte, error)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

type Group struct {
	name      string
	mainCache cache
	getter    Getter
}

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

func NewGroup(name string, maxBytes int, getter Getter) *Group {
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name: name,
		// there's no new function for cache
		// which makes filling fields here difficult
		mainCache: cache{maxBytes: maxBytes},
		getter:    getter,
	}
	groups[name] = g
	return g
}

// GetGroup doesn't create if not exist
func GetGroup(name string) (retGroup *Group, ok bool) {
	mu.RLock()
	defer mu.RUnlock()
	retGroup, ok = groups[name]
	return retGroup, ok
}

func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, errors.New("key is empty at group.Get()")
	}

	bv, ok := g.mainCache.get(key)
	if !ok {
		log.Println("[Group.Get] cache miss")
		ret, err := g.load(key)
		if err != nil {
			log.Println("[Group.Get] can't get key after miss:", err.Error())
		}
		return ret, err
	}

	log.Println("[Group.Get] cache hit")
	return bv, nil
}

func (g *Group) load(key string) (value ByteView, err error) {
	return g.getLocally(key)
}

func (g *Group) getLocally(key string) (ByteView, error) {
	retBytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
	}
	ret := ByteView{b: retBytes}
	err = g.populateCache(key, ret)
	return ret, err
}

func (g *Group) populateCache(key string, value ByteView) error {
	return g.mainCache.lru.Add(key, value)
}
