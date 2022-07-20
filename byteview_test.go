package geecache

import (
	"reflect"
	"testing"
)

func TestWriteToCopy(t *testing.T) {
	bv := ByteView{b: []byte("fml")}
	if bv.Len() != 3 || bv.String() != "fml" {
		t.Error("something wrong with Len or String method")
	}

	cp := bv.Get()
	copy(cp, make([]byte, len(cp)))

	if reflect.DeepEqual(cp, bv.b) {
		t.Error("modification to cp shouldn't propagate to bv.b")
	}
}
