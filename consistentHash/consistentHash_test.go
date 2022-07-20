package consistentHash

import (
	"container/list"
	"crypto/rand"
	"fmt"
	"reflect"
	"testing"
)

var expected_strings = list.New()
var determined_hash = list.New()

func TestSalt(t *testing.T) {
	b, err := getSalt(1)
	if err != nil {
		t.Error("unexpected error")
	}
	t.Logf("got %+v\n", b)
}

func riggedHash(salted []byte) uint32 {
	// salted = byte(name + incr_idx + salt)
	name := string(salted[:len(salted)-defaultSaltLen])
	if name != expected_strings.Remove(expected_strings.Front()).(string) {
		println(name)
		println(expected_strings.Front().Value.(string))
		panic("got unexpected name to hash")
	}
	return determined_hash.Remove(determined_hash.Front()).(uint32)
}

func TestGroupHash(t *testing.T) {
	ch := NewCHash(riggedHash)
	name := "foo"
	salt := make([]byte, defaultSaltLen)
	expect_hashes := make([]uint32, 0, defaultVtFactor)
	for i := 0; i < defaultVtFactor; i++ {
		// foo + str(i) for i in range(defaultVtFactor)
		// expecting (foo1, foo2,..., fooN)
		// giving hash (1,2,...,N)
		expected_strings.PushBack(name + fmt.Sprint(i))
		determined_hash.PushBack(uint32(i))
		expect_hashes = append(expect_hashes, uint32(i))
	}
	got_hashes := ch.groupHash(name, salt)
	if !reflect.DeepEqual(got_hashes, expect_hashes) {
		t.Error("got unexpected hash value")
		println(expect_hashes)
		println(got_hashes)
	}
}

func TestVNodeOps(t *testing.T) {
	// this tests normal logic, another test deals with errors
	ch := NewCHash(nil)
	data := []struct {
		hash uint32
	}{
		{114514},
		{114},
		{514},
	}
	// test insertVNode
	for _, d := range data {
		t.Run("insert"+fmt.Sprintln(d.hash), func(t *testing.T) {
			err := ch.insertVNode(d.hash, []byte("foo"), fmt.Sprint(d.hash))
			if err != nil {
				t.Error("unexpected error")
			}
		})
	}
	if ch.vNodes.Len() != len(data) {
		t.Error("wrong # of vNodes")
	}
	// test getNearestNode
	for _, d := range data {
		t.Run("insert"+fmt.Sprintln(d.hash), func(t *testing.T) {
			if ch.getNearestNode(d.hash) != fmt.Sprint(d.hash) {
				t.Error("got wrong name")
			}
		})
	}
	if ch.getNearestNode(113) != "114" ||
		ch.getNearestNode(114515) != "114" ||
		ch.getNearestNode(400) != "514" {
		t.Error("got wrong name in additional test")
	}
	// test ifDuplicatedHashes
	for i := 0; i < 1919; i++ {
		if i != 114 && i != 514 {
			if ch.ifDuplicatedHashes([]uint32{uint32(i)}) != false {
				t.Error("ifDuplicatedHashes gives false positive")
			}
		} else {
			if ch.ifDuplicatedHashes([]uint32{uint32(i)}) != true {
				t.Error("ifDuplicatedHashes gives false negative")
			}
		}
	}
	for i := 114500; i < 114600; i++ {
		if i != 114514 {
			if ch.ifDuplicatedHashes([]uint32{uint32(i)}) != false {
				t.Error("ifDuplicatedHashes gives false positive")
			}
		} else {
			if ch.ifDuplicatedHashes([]uint32{uint32(i)}) != true {
				t.Error("ifDuplicatedHashes gives false negative")
			}
		}
	}
}

func TestAbnormalVNodeOps(t *testing.T) {
	ch := NewCHash(nil)

	// insert
	err := ch.insertVNode(114, nil, "114")
	if err != nil {
		t.Error("unexpected error")
	}
	err = ch.insertVNode(114, nil, "115")
	if err == nil {
		t.Error("should be an error")
	}

	// delete
	err = ch.deleteVNode(114)
	if err != nil {
		t.Error("unexpected error")
	}
	err = ch.deleteVNode(114)
	if err == nil {
		t.Error("should be an error")
	}

	defer func() {
		if recover() == nil {
			t.Error("should panic")
		}
	}()
	// getNearestNode
	ch.getNearestNode(114)
}

func TestAddNote(t *testing.T) {
	// this focus on normal logic, another test tries the abnormal
	ch := NewCHash(nil)
	for i := 0; i < 100; i++ {
		ch.AddNode(fmt.Sprint(i) + "Node")
	}
	mapNodeCount := make(map[string]int)
	querySize := 100000 //
	for i := 0; i < 0+querySize; i++ {
		randBytes := make([]byte, 20)
		rand.Read(randBytes)
		mapNodeCount[ch.FindNode(string(randBytes))]++
	}
	for k := range mapNodeCount {
		// we have 100 nodes, ideally each node handles 1% of all queries
		// if a node is handling more than > 5.14%
		// that's super SUS
		if float64(mapNodeCount[k])/float64(querySize) > 0.0514 {
			t.Error("Detected very unbalanced hash, a node is handling > 10% of all queries")
		}
	}
	fmt.Println(mapNodeCount)
}
