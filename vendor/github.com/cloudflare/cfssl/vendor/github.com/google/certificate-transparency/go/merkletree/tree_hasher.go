package merkletree

const (
	// LeafPrefix is the domain separation prefix for leaf hashes.
	LeafPrefix = 0

	// NodePrefix is the domain separation prefix for internal tree nodes.
	NodePrefix = 1
)

// HasherFunc takes a slice of bytes and returns a cryptographic hash of those bytes.
type HasherFunc func([]byte) []byte

// TreeHasher performs the various hashing operations required when manipulating MerkleTrees.
type TreeHasher struct {
	hasher HasherFunc
}

// NewTreeHasher returns a new TreeHasher based on the passed in hash.
func NewTreeHasher(h HasherFunc) *TreeHasher {
	return &TreeHasher{
		hasher: h,
	}
}

// HashEmpty returns the hash of the empty string.
func (h TreeHasher) HashEmpty() []byte {
	return h.hasher([]byte{})
}

// HashLeaf returns the hash of the passed in leaf, after applying domain separation.
func (h TreeHasher) HashLeaf(leaf []byte) []byte {
	return h.hasher(append([]byte{LeafPrefix}, leaf...))

}

// HashChildren returns the merkle hash of the two passed in children.
func (h TreeHasher) HashChildren(left, right []byte) []byte {
	return h.hasher(append(append([]byte{NodePrefix}, left...), right...))
}
