package consistentHash

import (
	"container/list"
	"fmt"
	"math/rand"
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

func riggedHashNoExpectation(salted []byte) uint32 {
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

func TestNodeOps(t *testing.T) {
	// this focus on normal logic, another test tries the abnormal
	ch := NewCHash(nil)
	removeLater := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		name := fmt.Sprint(i) + "Node"
		ch.AddNode(name)
		removeLater = append(removeLater, name)
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
		// if a node is handling more than > 5.14% or < 0.1%
		// that's super SUS
		if per := float64(mapNodeCount[k]) / float64(querySize); per < 0.0001 ||
			per > 0.0514 {
			t.Error("Detected very unbalanced hash")
		}
	}
	fmt.Println(mapNodeCount)

	// RemoveNode
	rand.Shuffle(len(removeLater), func(i, j int) {
		removeLater[i], removeLater[j] = removeLater[j], removeLater[i]
	})

	expectedLen := len(removeLater)
	for _, name := range removeLater {
		expectedLen--
		err := ch.RemoveNode(name)
		if err != nil || ch.Len() != expectedLen {
			t.Error("unexpected error when removing nodes")
		}
	}
}

func TestAbnormalNodeOps(t *testing.T) {
	ch := NewCHash(riggedHashNoExpectation)
	for i := 0; i < 11; i++ {
		for j := 0; j < ch.vtFactor; j++ {
			determined_hash.PushBack(uint32(j))
		}
	}
	ch.AddNode("this hashed to 0 1 2 3")
	err := ch.AddNode("this also hashed to 0 1 2 3")
	if err == nil || err.Error() != "too many vNode number collisions after retries" {
		t.Error("it should exceed max tries to find a new salt")
	}
	// if this is not rejected by checking map[name]salt
	// this will hash and hence remove from empty list -> panic
	ch.AddNode("this hashed to 0 1 2 3")

	if err := ch.RemoveNode("this does not exist"); err == nil {
		t.Error("this should prompt removal of nonexistent node")
	}

	defer func() {
		if recover() == nil {
			t.Error("removeNode leads to inconsistent state should panic")
		}
	}()

	// before removing a node
	// remove one of it's vNode XD
	for j := 0; j < ch.vtFactor; j++ {
		determined_hash.PushBack(uint32(j))
	}
	ch.deleteVNode(3) // remains 0 1 2
	ch.RemoveNode("this hashed to 0 1 2 3")

}
