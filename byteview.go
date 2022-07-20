package geecache

type ByteView struct {
	// my question: why this is private
	// won't this cause any trouble
	b []byte
}

func (v ByteView) Len() int {
	return len(v.b)
}

func (v ByteView) Get() []byte {
	return cloneBytes(v.b)
}

func (v ByteView) String() string {
	return string(v.b)
}

func cloneBytes(b []byte) []byte {
	dup := make([]byte, len(b))
	copy(dup, b)
	return dup
}
