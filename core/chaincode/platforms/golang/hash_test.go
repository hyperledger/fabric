/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package golang

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/util"
)

// TestHashContentChange changes a random byte in a content and checks for hash change
func TestHashContentChange(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeCryptoHash(b)

	b2 := []byte("To be, or not to be- that is the question: Whether 'tis nobler in the mind to suffer The slings and arrows of outrageous fortune Or to take arms against a sea of troubles, And by opposing end them. To die- to sleep- No more; and by a sleep to say we end The heartache, and the thousand natural shocks That flesh is heir to. 'Tis a consummation Devoutly to be wish'd.")

	h1 := computeHash(b2, hash)

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
	h2 := computeHash(b2, hash)

	//the two hashes should be different
	if bytes.Compare(h1, h2) == 0 {
		t.Fail()
		t.Logf("Hash expected to be different but is same")
	}
}

// TestHashLenChange changes a random length of a content and checks for hash change
func TestHashLenChange(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeCryptoHash(b)

	b2 := []byte("To be, or not to be-")

	h1 := computeHash(b2, hash)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randIndex := (int(r.Uint32())) % len(b2)

	b2 = b2[0:randIndex]

	h2 := computeHash(b2, hash)

	//hash should be different
	if bytes.Compare(h1, h2) == 0 {
		t.Fail()
		t.Logf("Hash expected to be different but is same")
	}
}

// TestHashOrderChange changes a order of hash computation over a list of lines and checks for hash change
func TestHashOrderChange(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeCryptoHash(b)

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
		h1 = computeHash(l, h1)
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
		h2 = computeHash(l, hash)
	}

	//hash should be different
	if bytes.Compare(h1, h2) == 0 {
		t.Fail()
		t.Logf("Hash expected to be different but is same")
	}
}

// TestHashOverFiles computes hash over a directory and ensures it matches precomputed, hardcoded, hash
func TestHashOverFiles(t *testing.T) {
	b := []byte("firstcontent")
	hash := util.ComputeCryptoHash(b)

	hash, err := hashFilesInDir(".", "hashtestfiles", hash, nil)

	if err != nil {
		t.Fail()
		t.Logf("error : %s", err)
	}

	//as long as no files under "hashtestfiles" are changed, hash should always compute to the following
	expectedHash := "a4fe18bebf3d7e1c030c042903bdda9019b33829d03d9b95ab1edc8957be70dee6d786ab27b207210d29b5d9f88456ff753b8da5c244458cdcca6eb3c28a17ce"

	computedHash := hex.EncodeToString(hash[:])

	if expectedHash != computedHash {
		t.Fail()
		t.Logf("Hash expected to be unchanged")
	}
}
