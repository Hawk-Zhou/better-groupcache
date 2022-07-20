package lru_k

import (
	"fmt"
	"reflect"
	"testing"
)

var cache *Cache

type testBytes []byte

func (tb testBytes) Len() int {
	return len(tb)
}

func TestNew(t *testing.T) {
	cache = New(50, func(s string, v Value) {
		fmt.Printf("evicting key: %v of size %v", s, len(s)+v.Len())
	})
	if cache.maxBytes != 50 {
		t.Error("wrong maxBytes")
	}
	cache.RemoveOldest()
}

func TestWriteToFull(t *testing.T) {
	if cache.usedBytes != 0 {
		t.Error("incorrect initial usedBytes:", cache.usedBytes)
	}
	for i := 0; i < 5; i++ {
		key := fmt.Sprint(i)
		val := make(testBytes, 9)
		val[0] = byte(key[0])
		cache.Add(key, val)
	}
	if cache.usedBytes != 50 {
		t.Error("incorrect usedBytes:", cache.usedBytes)
	}
}

// let make it LRU->[3 4 1 0 2]->evict
func TestRead(t *testing.T) {
	numbers := []int{2, 0, 1, 4, 3}
	for _, i := range numbers {
		t.Run(fmt.Sprintf("read %v", i), func(t *testing.T) {
			ret, ok := cache.Get(fmt.Sprint(i))
			expect := make(testBytes, 9)
			expect[0] = byte(fmt.Sprint(i)[0])
			if !ok {
				t.Errorf("key %v doesn't exist", i)
			}
			if !reflect.DeepEqual(expect, ret) {
				t.Errorf("got wrong value %+v for key %v", ret, i)
			}
		})
	}
	numbers = []int{3, 4, 1, 0, 2}
	fmt.Println(numbers)
	for _, j := range numbers {
		t.Run(fmt.Sprintf("test precedence %v", j), func(t *testing.T) {
			thisEntry := cache.ll.Front().Value
			cache.ll.MoveToBack(cache.ll.Front())
			if x := thisEntry.(*entry).key; x != fmt.Sprint(j) {
				t.Errorf("expecting %v got %v", j, x)
			}
		})
	}
}

func TestModify(t *testing.T) {
	key := "2"
	val := make(testBytes, 9)
	cache.Add(key, val)
	got, ok := cache.Get(key)
	if !ok || !reflect.DeepEqual(got, val) {
		t.Error("got wrong/no value, ok:", ok)
	}
	if cache.ll.Front().Value.(*entry).key != "2" {
		t.Error("precedence is not correct")
	}
}

func TestGetKeyNotExist(t *testing.T) {
	key := "514"
	_, ok := cache.Get(key)
	if ok {
		t.Error("getting non-existent key got okay")
	}
}

func TestWriteAndEvict(t *testing.T) {
	cache.Add("114", make(testBytes, 7))

	numbers := []int{114, 2, 3, 4, 1}
	fmt.Println(numbers)
	for _, j := range numbers {
		t.Run(fmt.Sprintf("after evict test precedence %v", j), func(t *testing.T) {
			thisEntry := cache.ll.Front().Value
			cache.ll.MoveToBack(cache.ll.Front())
			if x := thisEntry.(*entry).key; x != fmt.Sprint(j) {
				t.Errorf("expecting %v got %v", j, x)
				t.Logf("usedbytes: %v", cache.usedBytes)
			}
		})
	}
}

func TestUpdateAndEvict(t *testing.T) {
	localCache := New(50, nil)
	for i := 0; i < 5; i++ {
		key := fmt.Sprint(i)
		val := make(testBytes, 9)
		val[0] = byte(key[0])
		localCache.Add(key, val)
	}
	localCache.Add("0", make(testBytes, 40))
	if localCache.usedBytes != 41 {
		t.Error("incorrect usedBytes:", localCache.usedBytes)
	}
}

func TestAddExceedMaxCap(t *testing.T) {
	if err := cache.Add("1", make(testBytes, 50)); err == nil {
		t.Error("didn't refuse add that exceeds max capacity")
	}
}

func TestLen(t *testing.T) {
	c := New(10, nil)
	c.Add("0", make(testBytes, 2))
	c.Add("1", make(testBytes, 2))
	c.Add("2", make(testBytes, 6))
	c.Add("1", make(testBytes, 9))
	if c.Len() != 1 {
		t.Error()
	}
}
