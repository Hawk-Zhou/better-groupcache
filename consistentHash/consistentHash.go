package consistentHash

import (
	"errors"
	"fmt"
	"hash/crc32"
	"log"
	"math/rand"
	"time"

	"github.com/google/btree"
)

// cHash (consistent Hash) provides conversions:
// 	query(str) -> hash(int) -> (find on ring) -> virtual node(int) -> physical node(str)
// 	and the inverse of the above process

const (
	defaultSaltLen  = 1 //bytes
	defaultVtFactor = 4
)

type vNode struct {
	hash uint32
	name string
}

func (vn vNode) Less(than btree.Item) bool {
	return vn.hash < than.(vNode).hash
}

type cHash struct {
	// when hashing, salt is added to avoid possible collision
	hasher     Hasher
	NameToSalt map[string][]byte
	vNodes     *btree.BTree
	vtFactor   int // how many vNode a physical node is mapped to
	saltLen    int
}

type Hasher func(b []byte) uint32

func init() {
	seed := time.Now().UnixNano()
	rand.Seed(seed)
	log.Println("seed is set to", seed)
}

func getSalt(size int) ([]byte, error) {
	ret := make([]byte, size)
	rn, err := rand.Read(ret)
	if err != nil {
		return nil, err
	}
	if rn != size {
		return nil, errors.New("can't read enough bytes from rand")
	}
	return ret, err
}

func NewCHash(hasher Hasher) *cHash {
	if hasher == nil {
		hasher = crc32.ChecksumIEEE
	}
	return &cHash{
		hasher:     hasher,
		vtFactor:   defaultVtFactor,
		NameToSalt: make(map[string][]byte),
		vNodes:     btree.New(2),
		saltLen:    defaultSaltLen,
	}
}

// groupHash hashes a name to a set of vNodes with salt
func (ch *cHash) groupHash(name string, salt []byte) []uint32 {
	ret := make([]uint32, 0, ch.vtFactor)
	for v := 0; v < ch.vtFactor; v++ {
		suffixedName := name + fmt.Sprint(v)
		b := make([]byte, 0, len(suffixedName)+ch.saltLen)
		b = append(b, []byte(suffixedName)...)
		b = append(b, salt...)
		ret = append(ret, ch.hasher(b))
	}
	return ret
}

func (ch *cHash) insertVNode(hash uint32, salt []byte, name string) error {
	replaced := ch.vNodes.ReplaceOrInsert(vNode{hash: hash, name: name})
	if replaced != nil {
		return errors.New("shouldn't be replacing existing vNode")
	}
	return nil
}

func (ch *cHash) deleteVNode(hash uint32) error {
	deleted := ch.vNodes.Delete(vNode{hash: hash})
	if deleted == nil {
		return errors.New("shouldn't be deleting non-existent vNode")
	}
	return nil
}

func (ch *cHash) getNearestNode(queryHash uint32) (name string) {
	if ch.vNodes.Len() == 0 {
		panic("No vNode exists!")
	}
	name = ch.vNodes.Min().(vNode).name
	ch.vNodes.AscendGreaterOrEqual(vNode{hash: queryHash}, func(item btree.Item) bool {
		if thisNode := item.(vNode); thisNode.hash >= queryHash {
			name = thisNode.name
			return false
		}
		// according to test coverage, this true will just be skipped
		// because it's guaranteed by the pivot that thisNode.hash >= queryHash
		// keep it to make linter happy (no return val)
		return true
	})
	return name
}

// this deletes all virtual node under the name
// along with other bookkeeping info
func (ch *cHash) RemoveNode(name string) error {
	salt, ok := ch.NameToSalt[name]
	if !ok {
		return errors.New("node name doesn't exist")
	}
	vNodes := ch.groupHash(name, salt)
	var err error = nil
	for _, h := range vNodes {
		err = ch.deleteVNode(h)
		if err != nil {
			log.Panicf("fatal error at RemoveNode: +%v", err.Error())
		}
	}
	delete(ch.NameToSalt, name)
	return nil
}

func (ch *cHash) ifDuplicatedHashes(hashes []uint32) bool {
	for _, u := range hashes {
		got := ch.vNodes.Get(vNode{hash: uint32(u)})
		if got != nil {
			return true
		}
	}
	return false
}

func (ch *cHash) AddNode(name string) error {
	if _, ok := ch.NameToSalt[name]; ok {
		return errors.New("the node already exists")
	}

changeSalt:
	for i := 0; i < 10; i++ {
		salt, err := getSalt(ch.saltLen)
		if err != nil {
			return fmt.Errorf("can't add node: %w", err)
		}
		generatedVNodes := ch.groupHash(name, salt)
		if ch.ifDuplicatedHashes(generatedVNodes) {
			continue changeSalt
		}
		// success

		for _, hash := range generatedVNodes {
			err := ch.insertVNode(hash, salt, name)
			if err != nil {
				// this is bad, now it's inconsistent
				log.Panicf("unexpected error at AddNote: %+v", err)
			}
		}

		ch.NameToSalt[name] = salt
		return nil
	}
	// fail
	// don't change the prompt, it's tested and compared
	return errors.New("too many vNode number collisions after retries")
}

func (ch *cHash) FindNode(query string) (name string) {
	return ch.getNearestNode(ch.hasher([]byte(query)))
}

func (ch *cHash) Len() int {
	return ch.vNodes.Len() / ch.vtFactor
}
