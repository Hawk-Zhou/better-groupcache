package geecache

import (
	"errors"
	"geecache/singleflight"
	"log"
	"math/rand"
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
	mainCache cache // authoritative
	hotCache  cache // not authoritative but hot
	getter    Getter
	peers     PeerPicker
	sfGroup   *singleflight.Group // singleflight group
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
		hotCache:  cache{maxBytes: maxBytes},
		sfGroup:   &singleflight.Group{},
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
			return ret, err
		}
		return ret, err
	}

	log.Println("[Group.Get] cache hit")
	return bv, nil
}

// load is called when the key can't be found in local cache
// It will ask its peers for that if not authoritative
// Otherwise it will call getter
func (g *Group) load(key string) (value ByteView, err error) {

	sfRet, err := g.sfGroup.Do(key, func() (interface{}, error) {
		if pGetter, ok := g.peers.PickPeer(key); ok {
			// a peer is authoritative
			log.Println("[Group.load] Getting from peers")
			ret, err := g.getFromPeers(pGetter, key)
			if err != nil {
				log.Printf("[Group.load] Failed to get from peers: %v", err)
				return ret, err
			}
			return ret, err
		}

		return g.getLocally(key)
	})

	ret := ByteView{}
	if err == nil {
		ret = sfRet.(ByteView)
	}
	return ret, err
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
	return g.mainCache.add(key, value)
}

func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("peers of a group initialized more than once")
	}
	g.peers = peers
	g.AddPeers()
}

func (g *Group) AddPeers(peers ...string) {
	switch x := g.peers.(type) {
	case (*HTTPPool):
		x.AddPeers(peers...)
	default:
		panic("unknown type encountered when adding peers")
	}
}

// getFromPeers should be called if known caller is not authoritative
// shouldn't validate whether authoritative here
func (g *Group) getFromPeers(pGetter PeerGetter, key string) (ByteView, error) {
	// if not ok means g itself is authoritative

	b, err := pGetter.Get(g.name, key)

	if err != nil {
		return ByteView{}, err
	}

	ret := ByteView{b: b}

	if rand.Intn(10) == 0 {
		g.hotCache.add(key, ret)
	}

	return ret, nil
}
