package main

import "github.com/google/btree"

type vkey struct {
	hash int
	ptr  []byte
}

func (vk vkey) Less(than btree.Item) bool {
	return vk.hash < than.(vkey).hash
}

func main() {
	tree := btree.New(2)

	for i := 0; i < 1000; i++ {
		if i%20 == 0 {

			tree.ReplaceOrInsert(vkey{hash: i, ptr: make([]byte, i)})
		}
	}
	// tree.AscendGreaterOrEqual(vkey{hash: 990,}, func(item btree.Item) bool {
	tree.Clone().AscendGreaterOrEqual(vkey{hash: 900}, func(item btree.Item) bool {
		val := item.(vkey).hash
		println("it:", item.(vkey).hash)
		if val < 1000 {
			return true
		} else {
			return false
		}
	})
}
