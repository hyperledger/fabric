package merkletree

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

type leafTestVector struct {
	inputLength int64
	input       []byte
	output      []byte
}

// Inputs and outputs are of fixed digest size.
type nodeTestVector struct {
	left   []byte
	right  []byte
	output []byte
}

type testVector struct {
	emptyHash []byte
	leaves    []leafTestVector
	nodes     []nodeTestVector
}

const (
	sha256EmptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

func dh(h string) []byte {
	r, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return r
}

func getTestVector() testVector {
	return testVector{
		emptyHash: dh(sha256EmptyHash),
		leaves: []leafTestVector{
			{0, dh(""), dh("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d")},
			{1, dh("00"), dh("96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7")},
			{16, dh("101112131415161718191a1b1c1d1e1f"), dh("3bfb960453ebaebf33727da7a1f4db38acc051d381b6da20d6d4e88f0eabfd7a")},
		},
		nodes: []nodeTestVector{
			{dh("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
				dh("202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f"),
				dh("1a378704c17da31e2d05b6d121c2bb2c7d76f6ee6fa8f983e596c2d034963c57")},
		},
	}
}

func getTreeHasher() *TreeHasher {
	return NewTreeHasher(func(b []byte) []byte {
		h := sha256.Sum256(b)
		return h[:]
	})
}

const (
	digestSize = sha256.Size
)

func TestCollisionEmptyHashZeroLengthLeaf(t *testing.T) {
	th := getTreeHasher()

	// Check that the empty hash is not the same as the hash of an empty leaf.
	leaf1Digest := th.HashEmpty()
	if got, want := len(leaf1Digest), digestSize; got != want {
		t.Fatalf("Empty digest has length %d, but expected %d", got, want)
	}

	leaf2Digest := th.HashLeaf([]byte{})
	if got, want := len(leaf2Digest), digestSize; got != want {
		t.Fatalf("Empty leaf digest has length %d, but expected %d", got, want)
	}

	if bytes.Equal(leaf1Digest, leaf2Digest) {
		t.Fatalf("Digests of empty string (%v) and zero-length leaf (%v) should differ", leaf1Digest, leaf2Digest)
	}
}

func TestCollisionDifferentLeaves(t *testing.T) {
	th := getTreeHasher()

	// Check that different leaves hash to different digests.
	const leaf1 = "Hello"
	const leaf2 = "World"

	leaf1Digest := th.HashLeaf([]byte(leaf1))
	if got, want := len(leaf1Digest), digestSize; got != want {
		t.Fatalf("Got unexpected leaf1 digest size %d, expected %d", got, want)
	}

	leaf2Digest := th.HashLeaf([]byte(leaf2))
	if got, want := len(leaf2Digest), digestSize; got != want {
		t.Fatalf("Got unexpected leaf2 digest size %d, expected %d", got, want)
	}

	if bytes.Equal(leaf1Digest, leaf2Digest) {
		t.Fatalf("Digests of leaf1 (%v) and leaf2 (%v) should differ", leaf1Digest, leaf2Digest)
	}

	// Compute an intermediate node digest.
	node1Digest := th.HashChildren(leaf1Digest, leaf2Digest)
	if got, want := len(node1Digest), digestSize; got != want {
		t.Fatalf("Got unexpected intermediate digest size %d, expected %d", got, want)
	}

	// Check that this is not the same as a leaf hash of their concatenation.
	image := append(leaf1Digest, leaf2Digest...)
	node2Digest := th.HashLeaf(image)
	if got, want := len(node2Digest), digestSize; got != want {
		t.Fatalf("Got unexpected node2Digest size %d, expected %d", got, want)
	}

	if bytes.Equal(node1Digest, node2Digest) {
		t.Fatalf("node1Digest (%v) and node2Digest (%v) should differ", node1Digest, node2Digest)
	}

	// Swap the order of nodes and check that the hash is different.
	node3Digest := th.HashChildren(leaf2Digest, leaf1Digest)
	if got, want := len(node3Digest), digestSize; got != want {
		t.Fatalf("Got unexpected node3 digest size %d, expected %d", got, want)
	}

	if bytes.Equal(node1Digest, node3Digest) {
		t.Fatalf("node1Digest (%v) and node3Digest (%v) should differ", node1Digest, node3Digest)
	}
}
