package util

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/util"
)

// TestHashContentChange changes a random byte in a content and checks for hash change
func TestHashContentChange(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	b2 := []byte("To be, or not to be- that is the question: Whether 'tis nobler in the mind to suffer The slings and arrows of outrageous fortune Or to take arms against a sea of troubles, And by opposing end them. To die- to sleep- No more; and by a sleep to say we end The heartache, and the thousand natural shocks That flesh is heir to. 'Tis a consummation Devoutly to be wish'd.")

	h1 := ComputeHash(b2, hash)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randIndex := (int(r.Uint32())) % len(b2)

	randByte := byte((int(r.Uint32())) % 128)

	//make sure the two bytes are different
	for {
		if randByte != b2[randIndex] {
			break
		}

		randByte = byte((int(r.Uint32())) % 128)
	}

	//change a random byte
	b2[randIndex] = randByte

	//this is the core hash func under test
	h2 := ComputeHash(b2, hash)

	//the two hashes should be different
	if bytes.Compare(h1, h2) == 0 {
		t.Error("Hash expected to be different but is same")
	}
}

// TestHashLenChange changes a random length of a content and checks for hash change
func TestHashLenChange(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	b2 := []byte("To be, or not to be-")

	h1 := ComputeHash(b2, hash)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randIndex := (int(r.Uint32())) % len(b2)

	b2 = b2[0:randIndex]

	h2 := ComputeHash(b2, hash)

	//hash should be different
	if bytes.Compare(h1, h2) == 0 {
		t.Error("Hash expected to be different but is same")
	}
}

// TestHashOrderChange changes a order of hash computation over a list of lines and checks for hash change
func TestHashOrderChange(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	b2 := [][]byte{[]byte("To be, or not to be- that is the question:"),
		[]byte("Whether 'tis nobler in the mind to suffer"),
		[]byte("The slings and arrows of outrageous fortune"),
		[]byte("Or to take arms against a sea of troubles,"),
		[]byte("And by opposing end them."),
		[]byte("To die- to sleep- No more; and by a sleep to say we end"),
		[]byte("The heartache, and the thousand natural shocks"),
		[]byte("That flesh is heir to."),
		[]byte("'Tis a consummation Devoutly to be wish'd.")}
	h1 := hash

	for _, l := range b2 {
		h1 = ComputeHash(l, h1)
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randIndex1 := (int(r.Uint32())) % len(b2)
	randIndex2 := (int(r.Uint32())) % len(b2)

	//make sure the two indeces are different
	for {
		if randIndex2 != randIndex1 {
			break
		}

		randIndex2 = (int(r.Uint32())) % len(b2)
	}

	//switch two arbitrary lines
	tmp := b2[randIndex2]
	b2[randIndex2] = b2[randIndex1]
	b2[randIndex1] = tmp

	h2 := hash
	for _, l := range b2 {
		h2 = ComputeHash(l, hash)
	}

	//hash should be different
	if bytes.Compare(h1, h2) == 0 {
		t.Error("Hash expected to be different but is same")
	}
}

// TestHashOverFiles computes hash over a directory and ensures it matches precomputed, hardcoded, hash
func TestHashOverFiles(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	hash, err := HashFilesInDir(".", "hashtestfiles1", hash, nil)

	if err != nil {
		t.Fail()
		t.Logf("error : %s", err)
	}

	//as long as no files under "hashtestfiles1" are changed, hash should always compute to the following
	expectedHash := "0c92180028200dfabd08d606419737f5cdecfcbab403e3f0d79e8d949f4775bc"

	computedHash := hex.EncodeToString(hash[:])

	if expectedHash != computedHash {
		t.Error("Hash expected to be unchanged")
	}
}

func TestHashDiffDir(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	hash1, err := HashFilesInDir(".", "hashtestfiles1", hash, nil)
	if err != nil {
		t.Errorf("Error getting code %s", err)
	}
	hash2, err := HashFilesInDir(".", "hashtestfiles2", hash, nil)
	if err != nil {
		t.Errorf("Error getting code %s", err)
	}
	if bytes.Compare(hash1, hash2) == 0 {
		t.Error("Hash should be different for 2 different remote repos")
	}

}
func TestHashSameDir(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeSHA256(b)

	hash1, err := HashFilesInDir(".", "hashtestfiles1", hash, nil)
	if err != nil {
		t.Errorf("Error getting code %s", err)
	}
	hash2, err := HashFilesInDir(".", "hashtestfiles1", hash, nil)
	if err != nil {
		t.Errorf("Error getting code %s", err)
	}
	if bytes.Compare(hash1, hash2) != 0 {
		t.Error("Hash should be same across multiple downloads")
	}
}
