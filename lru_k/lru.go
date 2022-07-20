package lru_k

import (
	"container/list"
	"errors"
)

type KCache struct {
	cache    Cache
	fifoll   *list.List
	fifoMap  map[string]*list.Element
	fifoSize int
	fifoLen  int
}

// ll-element-element.Value-*entry-entry.value
type Cache struct {
	maxBytes   int
	usedBytes  int
	ll         *list.List
	cacheMap   map[string]*list.Element
	onEviction func(key string, value Value)
}

type Value interface {
	Len() int
}

type entry struct {
	key   string
	value Value
}

var MaxFifoSize = 10

func New(maxBytes int, onEviction func(string, Value)) *Cache {
	if onEviction == nil {
		onEviction = func(s string, v Value) {}
	}
	return &Cache{
		maxBytes:   maxBytes,
		ll:         list.New(),
		cacheMap:   make(map[string]*list.Element),
		onEviction: onEviction,
	}
}

func (c *Cache) Get(key string) (value Value, ok bool) {
	element, ok := c.cacheMap[key]
	if !ok {
		// the original structure imo is not idiomatic
		// if block is for error handling, not okay cases
		return nil, ok
	}

	c.ll.MoveToFront(element)
	return element.Value.(*entry).value, ok
}

func (c *Cache) RemoveOldest() {
	element := c.ll.Back()
	if element == nil {
		return
	}
	delete(c.cacheMap, element.Value.(*entry).key)
	thisEntry := element.Value.(*entry)
	c.usedBytes -= thisEntry.value.Len()
	c.usedBytes -= len(thisEntry.key)
	if c.onEviction != nil {
		c.onEviction(thisEntry.key, thisEntry.value)
	}
	c.ll.Remove(element)
}

// Add also can serve as Modify
// should not be called with data larger than maxBytes
func (c *Cache) Add(key string, value Value) error {
	if len(key)+value.Len() > c.maxBytes {
		return errors.New("add exceeds max capacity")
	}
	element, ok := c.cacheMap[key]
	deltaSize := 0
	if ok {
		thisEntry := element.Value.(*entry)
		deltaSize = value.Len() - thisEntry.value.Len()
		thisEntry.value = value
		c.ll.MoveToFront(element)
	} else {
		deltaSize = value.Len() + len(key)
		newEntry := &entry{key: key, value: value}
		newElement := c.ll.PushFront(newEntry)
		c.cacheMap[key] = newElement
	}

	c.usedBytes += deltaSize
	for c.usedBytes > c.maxBytes {
		c.RemoveOldest()
	}
	return nil
}

func (c *Cache) Len() int {
	return c.ll.Len()
}

func NewK(maxBytes int, onEviction func(string, Value)) *KCache {
	return &KCache{cache: *New(maxBytes, onEviction),
		fifoll:   list.New(),
		fifoMap:  make(map[string]*list.Element),
		fifoSize: MaxFifoSize,
	}
}

func (kc *KCache) Add(key string, value Value) error {
	if len(key)+value.Len() > kc.cache.maxBytes {
		return errors.New("add exceeds max capacity")
	}
	ev, ok := kc.Get(key)
	if ok {
		deltaSize := value.Len() - ev.Len()
		kc.cache.usedBytes += deltaSize
		for kc.cache.usedBytes > kc.cache.maxBytes {
			kc.RemoveOldest()
		}
		kc.cache.usedBytes -= deltaSize
		kc.cache.Add(key, value)
	}
	// creating new entry
	for kc.fifoLen >= kc.fifoSize {
		kc.RemoveOldest()
	}
	kc.fifoLen += 1

	size := len(key) + value.Len()
	for kc.cache.usedBytes+size > kc.cache.maxBytes {
		kc.RemoveOldest()
	}
	kc.cache.usedBytes += size
	newEntry := &entry{key: key, value: value}
	newElement := kc.fifoll.PushFront(newEntry)
	kc.fifoMap[key] = newElement
	return nil
}

// this removes the list.element and update capacity & map
func (kc *KCache) removeElement(element *list.Element) {
	thisEntry := element.Value.(*entry)
	// update capacity
	kc.cache.usedBytes -= thisEntry.value.Len()
	kc.cache.usedBytes -= len(thisEntry.key)
	kc.fifoLen -= 1
	// delete key
	delete(kc.fifoMap, thisEntry.key)
	kc.fifoll.Remove(element)
}

func (kc *KCache) RemoveOldest() {
	if back := kc.fifoll.Back(); back != nil {
		back := kc.fifoll.Back()
		thisEntry := back.Value.(*entry)
		kc.cache.onEviction(thisEntry.key, thisEntry.value)
		kc.removeElement(back)
	} else {
		kc.cache.RemoveOldest()
	}
}

func (kc *KCache) Get(key string) (value Value, ok bool) {
	element, ok := kc.fifoMap[key]
	if !ok {
		return kc.cache.Get(key)
	}
	thisEntry := element.Value.(*entry)
	kc.removeElement(element)
	kc.cache.Add(thisEntry.key, thisEntry.value)
	return thisEntry.value, true
}

func (kc *KCache) Len() int {
	return kc.cache.ll.Len() + kc.fifoll.Len()
}
