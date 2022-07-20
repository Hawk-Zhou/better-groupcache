package lru_k

import (
	"testing"
)

var kcache *KCache

func TestCreateK(t *testing.T) {
	MaxFifoSize = 2
	kcache = NewK(50, nil)
}

func TestWriteK(t *testing.T) {
	kcache.Add("9", make(testBytes, 0))
	if kcache.fifoLen != 1 {
		t.Error("wrong fifoLen")
	}

	if _, ok := kcache.fifoMap["9"]; !ok {
		t.Error("missing key in fifoMap")
	}

	if kcache.cache.usedBytes != 1 {
		t.Error("wrong used bytes")
	}
}

func TestEvictFIFO(t *testing.T) {
	kcache.Add("8", make(testBytes, 0))
	kcache.Add("7", make(testBytes, 0))
	if kcache.fifoLen != 2 {
		t.Error("wrong fifoLen")
	}

	if _, ok := kcache.fifoMap["9"]; ok {
		t.Error("key not properly evicted in fifo list")
	}

	if back := kcache.cache.ll.Back(); back != nil {
		t.Error("there shouldn't be any element in LRU list")
	}

	kcache.Add("8", make(testBytes, 49))

	if _, ok := kcache.fifoMap["7"]; ok {
		t.Error("key not properly evicted (due to upgrade) in fifo list")
	}
}

func TestAddExceedMaxCapK(t *testing.T) {
	if err := kcache.Add("1", make(testBytes, 50)); err == nil {
		t.Error("didn't refuse add that exceeds max capacity")
	}
}
