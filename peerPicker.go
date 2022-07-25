package geecache

// The whole process goes like: initialize to be group-aware or not.
// Every time looking up a key, call portPicker to look at groupName.
// Regardless of whether portPicker is init'ed to be group-aware or not,
// it returns a PeerPicker, call PickPeer of it to get PeerGetter.
// Call PeerGetter with the group name and key again to get the key.

// PeerGetter ask the peer its related to for the key
type PeerGetter interface {
	Get(group string, key string) ([]byte, error)
}

// PeerPicker is already bound to a group if the portPicker is initialized
// so it doesn't receive group name
type PeerPicker interface {
	PickPeer(key string) (PeerGetter, bool)
}

var portPicker func(group string) PeerPicker
