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

package util

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestComputeCryptoHash(t *testing.T) {
	if bytes.Compare(ComputeCryptoHash([]byte("foobar")), ComputeCryptoHash([]byte("foobar"))) != 0 {
		t.Fatalf("Expected hashes to match, but they did not match")
	}
	if bytes.Compare(ComputeCryptoHash([]byte("foobar1")), ComputeCryptoHash([]byte("foobar2"))) == 0 {
		t.Fatalf("Expected hashes to be different, but they match")
	}
}

func TestUUIDGeneration(t *testing.T) {
	uuid := GenerateUUID()
	if len(uuid) != 36 {
		t.Fatalf("UUID length is not correct. Expected = 36, Got = %d", len(uuid))
	}
	uuid2 := GenerateUUID()
	if uuid == uuid2 {
		t.Fatalf("Two UUIDs are equal. This should never occur")
	}
}

func TestIntUUIDGeneration(t *testing.T) {
	uuid := GenerateIntUUID()

	uuid2 := GenerateIntUUID()
	if uuid == uuid2 {
		t.Fatalf("Two UUIDs are equal. This should never occur")
	}
}
func TestTimestamp(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Logf("timestamp now: %v", CreateUtcTimestamp())
		time.Sleep(200 * time.Millisecond)
	}
}

func TestGenerateHashFromSignature(t *testing.T) {
	if bytes.Compare(GenerateHashFromSignature("aPath", []byte("aCtor12")),
		GenerateHashFromSignature("aPath", []byte("aCtor12"))) != 0 {
		t.Fatalf("Expected hashes to match, but they did not match")
	}
	if bytes.Compare(GenerateHashFromSignature("aPath", []byte("aCtor12")),
		GenerateHashFromSignature("bPath", []byte("bCtor34"))) == 0 {
		t.Fatalf("Expected hashes to be different, but they match")
	}
}

func TestGeneratIDfromTxSHAHash(t *testing.T) {
	txid := GenerateIDfromTxSHAHash([]byte("foobar"))
	txid2 := GenerateIDfromTxSHAHash([]byte("foobar1"))
	if txid == txid2 {
		t.Fatalf("Two TxIDs are equal. This should never occur")
	}
}

func TestGenerateIDWithAlg(t *testing.T) {
	_, err := GenerateIDWithAlg("sha256", []byte{1, 1, 1, 1})
	if err != nil {
		t.Fatalf("Decoder failure: %v", err)
	}
}

func TestFindMissingElements(t *testing.T) {
	all := []string{"a", "b", "c", "d"}
	some := []string{"b", "c"}
	expectedDelta := []string{"a", "d"}
	actualDelta := FindMissingElements(all, some)
	if len(expectedDelta) != len(actualDelta) {
		t.Fatalf("Got %v, expected %v", actualDelta, expectedDelta)
	}
	for i := range expectedDelta {
		if strings.Compare(expectedDelta[i], actualDelta[i]) != 0 {
			t.Fatalf("Got %v, expected %v", actualDelta, expectedDelta)
		}
	}
}
