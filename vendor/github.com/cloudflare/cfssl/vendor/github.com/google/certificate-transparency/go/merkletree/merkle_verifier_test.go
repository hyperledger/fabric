package merkletree

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
)

const (
	sha256EmptyTreeHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

func verifierCheck(v *MerkleVerifier, leafIndex, treeSize int64, proof [][]byte, root []byte, leaf []byte) error {
	// Verify original inclusion proof
	got, err := v.RootFromInclusionProof(leafIndex, treeSize, proof, leaf)
	if err != nil {
		return err
	}
	if want := root; !bytes.Equal(got, want) {
		return fmt.Errorf("got root:\n%v\nexpected:\n%v", got, want)
	}
	if err := v.VerifyInclusionProof(leafIndex, treeSize, proof, root, leaf); err != nil {
		return err
	}

	// Wrong leaf index
	if err := v.VerifyInclusionProof(leafIndex-1, treeSize, proof, root, leaf); err == nil {
		return errors.New("incorrectly verified against leafIndex - 1")
	}
	if err := v.VerifyInclusionProof(leafIndex+1, treeSize, proof, root, leaf); err == nil {
		return errors.New("incorrectly verified against leafIndex + 1")
	}
	if err := v.VerifyInclusionProof(leafIndex^2, treeSize, proof, root, leaf); err == nil {
		return errors.New("incorrectly verified against leafIndex ^ 2")
	}

	// Wrong tree height
	if err := v.VerifyInclusionProof(leafIndex, treeSize*2, proof, root, leaf); err == nil {
		return errors.New("incorrectly verified against treeSize * 2")
	}
	if err := v.VerifyInclusionProof(leafIndex, treeSize/2, proof, root, leaf); err == nil {
		return errors.New("incorrectly verified against treeSize / 2")
	}

	// Wrong leaf
	if err := v.VerifyInclusionProof(leafIndex, treeSize, proof, root, []byte("WrongLeaf")); err == nil {
		return errors.New("incorrectly verified against WrongLeaf")
	}

	// Wrong root
	if err := v.VerifyInclusionProof(leafIndex, treeSize, proof, dh(sha256EmptyTreeHash), leaf); err == nil {
		return errors.New("incorrectly verified against empty root hash")
	}

	// Wrong inclusion proofs

	// Modify single element of the proof
	for i := 0; i < len(proof); i++ {
		tmp := proof[i]
		proof[i] = dh(sha256EmptyTreeHash)
		if err := v.VerifyInclusionProof(leafIndex, treeSize, proof, root, leaf); err == nil {
			return errors.New("incorrectly verified against incorrect inclusion proof")
		}
		proof[i] = tmp
	}

	// Add garbage at the end
	wrongProof := append(proof, []byte(""))
	if err := v.VerifyInclusionProof(leafIndex, treeSize, wrongProof, root, leaf); err == nil {
		return errors.New("incorrectly verified against proof with trailing garbage")
	}

	wrongProof = append(proof, root)
	if err := v.VerifyInclusionProof(leafIndex, treeSize, wrongProof, root, leaf); err == nil {
		return errors.New("incorrectly verified against proof with trailing root")
	}

	if len(proof) > 0 {
		// Remove a node from the end
		wrongProof = proof[:len(proof)-1]
		if err := v.VerifyInclusionProof(leafIndex, treeSize, wrongProof, root, leaf); err == nil {
			return errors.New("incorrectly verified against truncated proof")
		}
	}

	// Add garbage at the front
	wrongProof = append([][]byte{{}}, proof...)
	if err := v.VerifyInclusionProof(leafIndex, treeSize, wrongProof, root, leaf); err == nil {
		return errors.New("incorrectly verified against proof with preceding garbage")
	}

	wrongProof = append([][]byte{root}, proof...)
	if err := v.VerifyInclusionProof(leafIndex, treeSize, wrongProof, root, leaf); err == nil {
		return errors.New("incorrectly verified against proof with preceding garbage")
	}

	return nil
}

func verifierConsistencyCheck(v *MerkleVerifier, snapshot1, snapshot2 int64, root1, root2 []byte, proof [][]byte) error {
	// Verify original consistency proof
	err := v.VerifyConsistencyProof(snapshot1, snapshot2, root1, root2, proof)
	if err != nil {
		return err
	}

	if len(proof) == 0 {
		// For simplicity test only non-trivial proofs that have root1 != root2
		// snapshot1 != 0 and snapshot1 != snapshot2.
		return nil
	}

	// Wrong snapshot index
	if err := v.VerifyConsistencyProof(snapshot1-1, snapshot2, root1, root2, proof); err == nil {
		return errors.New("incorrectly verified against snapshot1-1")
	}
	if err := v.VerifyConsistencyProof(snapshot1+1, snapshot2, root1, root2, proof); err == nil {
		return errors.New("incorrectly verified against snapshot1+1")
	}
	if err := v.VerifyConsistencyProof(snapshot1^2, snapshot2, root1, root2, proof); err == nil {
		return errors.New("incorrectly verified against snapshot1^2")
	}

	// Wrong tree height
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2*2, root1, root2, proof); err == nil {
		return errors.New("incorrectly verified against snapshot2*2")
	}
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2/2, root1, root2, proof); err == nil {
		return errors.New("incorrectly verified against snapshot2/2")
	}

	// Wrong root
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2, []byte("WrongRoot"), root2, proof); err == nil {
		return errors.New("incorrectly verified against wrong root1")
	}
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2, root1, []byte("WrongRoot"), proof); err == nil {
		return errors.New("incorrectly verified against wrong root2")
	}
	// Swap roots
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2*2, root2, root1, proof); err == nil {
		return errors.New("incorrectly verified against swapped roots")
	}

	// Wrong consistency proofs
	// Empty proof
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2, root2, root1, [][]byte{}); err == nil {
		return errors.New("incorrectly verified against empty proof")
	}

	// Modify single element of the proof
	for i := 0; i < len(proof); i++ {
		tmp := proof[i]
		proof[i] = dh(sha256EmptyTreeHash)
		if err := v.VerifyConsistencyProof(snapshot1, snapshot2, root2, root1, proof); err == nil {
			return errors.New("incorrectly verified against incorrect consistency proof")
		}
		proof[i] = tmp
	}

	// Add garbage at the end
	wrongProof := append(proof, []byte(""))
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2, root2, root1, wrongProof); err == nil {
		return errors.New("incorrectly verified against proof with trailing garbage")
	}

	wrongProof = append(proof, root1)
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2, root2, root1, wrongProof); err == nil {
		return errors.New("incorrectly verified against proof with trailing root")
	}

	// Remove a node from the end
	wrongProof = proof[:len(proof)-1]
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2, root2, root1, wrongProof); err == nil {
		return errors.New("incorrectly verified against truncated proof")
	}

	// Add garbage at the front
	wrongProof = append([][]byte{{}}, proof...)
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2, root2, root1, wrongProof); err == nil {
		return errors.New("incorrectly verified against proof with preceding garbage")
	}

	wrongProof = append([][]byte{proof[0]}, proof...)
	if err := v.VerifyConsistencyProof(snapshot1, snapshot2, root2, root1, wrongProof); err == nil {
		return errors.New("incorrectly verified against proof with preceding garbage")
	}

	return nil
}

func getVerifier() MerkleVerifier {
	return NewMerkleVerifier(func(b []byte) []byte {
		h := sha256.Sum256(b)
		return h[:]
	})
}

type proofTestVector struct {
	h []byte
	l int
}

type inclusionProofTestVector struct {
	leaf, snapshot, proofLength int64
	proof                       []proofTestVector
}

func getInputs() []proofTestVector {
	return []proofTestVector{
		{dh(""), 0},
		{dh("00"), 1},
		{dh("10"), 1},
		{dh("2021"), 2},
		{dh("3031"), 2},
		{dh("40414243"), 4},
		{dh("5051525354555657"), 8},
		{dh("606162636465666768696a6b6c6d6e6f"), 16},
	}
}

func getInclusionTestVector() []inclusionProofTestVector {
	return []inclusionProofTestVector{
		{0, 0, 0, []proofTestVector{{dh(""), 0}, {dh(""), 0}, {dh(""), 0}}},
		{1, 1, 0, []proofTestVector{{dh(""), 0}, {dh(""), 0}, {dh(""), 0}}},
		{1,
			8,
			3,
			[]proofTestVector{
				{dh("96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7"), 32},
				{dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"), 32},
				{dh("6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4"), 32}}},
		{6,
			8,
			3,
			[]proofTestVector{
				{dh("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b"), 32},
				{dh("ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0"), 32},
				{dh("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"), 32}}},
		{3,
			3,
			1,
			[]proofTestVector{
				{dh("fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125"), 32},
				{dh(""), 0},
				{dh(""), 0}}},
		{2,
			5,
			3,
			[]proofTestVector{
				{dh("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"), 32},
				{dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"), 32},
				{dh("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b"), 32}},
		}}
}

func getRoots() []proofTestVector {
	return []proofTestVector{
		{dh("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"), 32},
		{dh("fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125"), 32},
		{dh("aeb6bcfe274b70a14fb067a5e5578264db0fa9b51af5e0ba159158f329e06e77"), 32},
		{dh("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"), 32},
		{dh("4e3bbb1f7b478dcfe71fb631631519a3bca12c9aefca1612bfce4c13a86264d4"), 32},
		{dh("76e67dadbcdf1e10e1b74ddc608abd2f98dfb16fbce75277b5232a127f2087ef"), 32},
		{dh("ddb89be403809e325750d3d263cd78929c2942b7942a34b77e122c9594a74c8c"), 32},
		{dh("5dc9da79a70659a9ad559cb701ded9a2ab9d823aad2f4960cfe370eff4604328"), 32}}
}

type consistencyTestVector struct {
	snapshot1, snapshot2, proofLen int64
	proof                          [3]proofTestVector
}

func getConsistencyProofs() []consistencyTestVector {
	return []consistencyTestVector{
		{1, 1, 0, [3]proofTestVector{{dh(""), 0}, {dh(""), 0}, {dh(""), 0}}},
		{1,
			8,
			3,
			[3]proofTestVector{
				{dh("96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7"), 32},
				{dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"), 32},
				{dh("6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4"), 32}}},
		{6,
			8,
			3,
			[3]proofTestVector{
				{dh("0ebc5d3437fbe2db158b9f126a1d118e308181031d0a949f8dededebc558ef6a"), 32},
				{dh("ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0"), 32},
				{dh("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"), 32}}},
		{2,
			5,
			2,
			[3]proofTestVector{
				{dh("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"), 32},
				{dh("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b"), 32},
				{dh(""), 0}}},
	}
}

func TestVerifyInclusionProof(t *testing.T) {
	v := getVerifier()

	path := [][]byte{}
	// Various invalid paths
	if err := v.VerifyInclusionProof(0, 0, path, []byte{}, []byte{}); err == nil {
		t.Fatal("Incorrectly verified invalid path 1")
	}
	if err := v.VerifyInclusionProof(0, 1, path, []byte{}, []byte{}); err == nil {
		t.Fatal("Incorrectly verified invalid path 2")
	}
	if err := v.VerifyInclusionProof(1, 0, path, []byte{}, []byte{}); err == nil {
		t.Fatal("Incorrectly verified invalid path 3")
	}
	if err := v.VerifyInclusionProof(2, 1, path, []byte{}, []byte{}); err == nil {
		t.Fatal("Incorrectly verified invalid path 4")
	}

	if err := v.VerifyInclusionProof(0, 0, path, dh(sha256EmptyTreeHash), []byte{}); err == nil {
		t.Fatal("Incorrectly verified invalid root 1")
	}
	if err := v.VerifyInclusionProof(0, 1, path, dh(sha256EmptyTreeHash), []byte{}); err == nil {
		t.Fatal("Incorrectly verified invalid root 2")
	}
	if err := v.VerifyInclusionProof(1, 0, path, dh(sha256EmptyTreeHash), []byte{}); err == nil {
		t.Fatal("Incorrectly verified invalid root 3")
	}
	if err := v.VerifyInclusionProof(2, 1, path, dh(sha256EmptyTreeHash), []byte{}); err == nil {
		t.Fatal("Incorrectly verified invalid root 4")
	}

	// Known good paths.

	inclusionProofs := getInclusionTestVector()
	inputs := getInputs()
	roots := getRoots()
	// i = 0 is an invalid path.
	for i := 1; i < 6; i++ {
		// Construct the path.
		proof := [][]byte{}
		for j := int64(0); j < inclusionProofs[i].proofLength; j++ {
			proof = append(proof, inclusionProofs[i].proof[j].h)
		}
		err := verifierCheck(&v, inclusionProofs[i].leaf, inclusionProofs[i].snapshot, proof,
			roots[inclusionProofs[i].snapshot-1].h,
			inputs[inclusionProofs[i].leaf-1].h)
		if err != nil {
			t.Fatalf("i=%d: %s", i, err)
		}
	}

}

func TestVerifyConsistencyProof(t *testing.T) {
	v := getVerifier()

	proof := [][]byte{}
	root1 := []byte("don't care")
	root2 := []byte("don't care")

	// Snapshots that are always consistent
	if err := verifierConsistencyCheck(&v, 0, 0, root1, root2, proof); err != nil {
		t.Fatalf("Failed to verify proof: %s", err)
	}
	if err := verifierConsistencyCheck(&v, 0, 1, root1, root2, proof); err != nil {
		t.Fatalf("Failed to verify proof: %s", err)
	}
	if err := verifierConsistencyCheck(&v, 1, 1, root1, root2, proof); err != nil {
		t.Fatalf("Failed to verify proof: %s", err)
	}

	// Invalid consistency proofs.
	// Time travel to the past.
	if err := verifierConsistencyCheck(&v, 1, 0, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified timetravelling proof")
	}
	if err := verifierConsistencyCheck(&v, 2, 1, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified timetravelling proof")
	}

	// Empty proof
	if err := verifierConsistencyCheck(&v, 1, 2, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified timetravelling proof")
	}

	root1 = dh(sha256EmptyTreeHash)
	// Roots don't match.
	if err := verifierConsistencyCheck(&v, 0, 0, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified mismatched root")
	}
	if err := verifierConsistencyCheck(&v, 1, 1, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified mismatched root")
	}

	// Roots match but the proof is not empty.
	root2 = dh(sha256EmptyTreeHash)
	proof = [][]byte{dh(sha256EmptyTreeHash)}

	if err := verifierConsistencyCheck(&v, 0, 0, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified non-empty proof")
	}
	if err := verifierConsistencyCheck(&v, 0, 1, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified non-empty proof")
	}
	if err := verifierConsistencyCheck(&v, 1, 1, root1, root2, proof); err == nil {
		t.Fatal("Incorrectly verified non-empty proof")
	}

	// Known good proofs.
	roots := getRoots()
	proofs := getConsistencyProofs()
	for i := 0; i < 4; i++ {
		proof := [][]byte{}
		for j := int64(0); j < proofs[i].proofLen; j++ {
			proof = append(proof, proofs[i].proof[j].h)
		}
		snapshot1 := proofs[i].snapshot1
		snapshot2 := proofs[i].snapshot2
		err := verifierConsistencyCheck(&v, snapshot1, snapshot2,
			roots[snapshot1-1].h,
			roots[snapshot2-1].h, proof)
		if err != nil {
			t.Fatalf("Failed to verify known good proof for i=%d: %s", i, err)
		}
	}
}
