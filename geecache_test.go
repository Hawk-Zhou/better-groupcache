package geecache

import (
	"fmt"
	"reflect"
	"testing"
)

func TestGetterCallback(t *testing.T) {
	var callback Getter = GetterFunc(func(key string) ([]byte, error) {
		return []byte(key), nil
	})

	quote := "random word"
	expectRet := []byte(quote)
	if v, _ := callback.Get(quote); !reflect.DeepEqual(v, expectRet) {
		t.Error("getter is a bad callback")
	}
}

func TestGroupWR(t *testing.T) {
	dbStub := map[string][]byte{
		"hello": []byte("world"),
		"my":    []byte("pace"),
	}

	refCount := new(int)
	getGenerator := func() (ret Getter) {
		ret = GetterFunc(func(key string) ([]byte, error) {
			(*refCount)++
			fmt.Printf("[Getter Called] Loading %v from slow storage, count %v\n", key, *refCount)
			return []byte(dbStub[key]), nil
		})
		return ret
	}

	getter := getGenerator()

	g := NewGroup("G1", len("hello"+"world"), getter)

	testdata := []struct {
		name          string
		key           string
		output        []byte
		expectedCount int
		err           error
	}{
		{"populate hello", "hello", dbStub["hello"], 1, nil},
		{"hit hello", "hello", dbStub["hello"], 1, nil},
		{"hit hello", "hello", dbStub["hello"], 1, nil},
		{"populate my", "my", dbStub["my"], 2, nil},
		{"hit my", "my", dbStub["my"], 2, nil},
		{"repopulate hello", "hello", dbStub["hello"], 3, nil},
	}

	for _, d := range testdata {
		t.Run(d.name, func(t *testing.T) {
			ret, err := g.Get(d.key)
			if err != nil {
				t.Error("unexpected error")
			}
			if *refCount != d.expectedCount || !reflect.DeepEqual(ret.Get(), d.output) {
				t.Error("counter @ callback isn't incr'ed / bad ret val, vals:", *refCount, string(ret.Get()))
			}
		})
	}

}
