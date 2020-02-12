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
	"testing"
	"time"
)

func TestComputeSHA256(t *testing.T) {
	if !bytes.Equal(ComputeSHA256([]byte("foobar")), ComputeSHA256([]byte("foobar"))) {
		t.Fatalf("Expected hashes to match, but they did not match")
	}
	if bytes.Equal(ComputeSHA256([]byte("foobar1")), ComputeSHA256([]byte("foobar2"))) {
		t.Fatalf("Expected hashes to be different, but they match")
	}
}

func TestComputeSHA3256(t *testing.T) {
	if !bytes.Equal(ComputeSHA3256([]byte("foobar")), ComputeSHA3256([]byte("foobar"))) {
		t.Fatalf("Expected hashes to match, but they did not match")
	}
	if bytes.Equal(ComputeSHA3256([]byte("foobar1")), ComputeSHA3256([]byte("foobar2"))) {
		t.Fatalf("Expected hashed to be different, but they match")
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

func TestTimestamp(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Logf("timestamp now: %v", CreateUtcTimestamp())
		time.Sleep(200 * time.Millisecond)
	}
}

func TestToChaincodeArgs(t *testing.T) {
	expected := [][]byte{[]byte("foo"), []byte("bar")}
	actual := ToChaincodeArgs("foo", "bar")
	if len(expected) != len(actual) {
		t.Fatalf("Got %v, expected %v", actual, expected)
	}
	for i := range expected {
		if !bytes.Equal(expected[i], actual[i]) {
			t.Fatalf("Got %v, expected %v", actual, expected)
		}
	}
}

func TestConcatenateBytesNormal(t *testing.T) {
	first := []byte("first")
	second := []byte("second")
	third := []byte("third")

	result := ConcatenateBytes(first, second, third)
	expected := []byte("firstsecondthird")
	if !bytes.Equal(result, expected) {
		t.Errorf("Did not concatenate bytes correctly, expected %s, got %s", expected, result)
	}
}

func TestConcatenateBytesNil(t *testing.T) {
	first := []byte("first")
	second := []byte(nil)
	third := []byte("third")

	result := ConcatenateBytes(first, second, third)
	expected := []byte("firstthird")
	if !bytes.Equal(result, expected) {
		t.Errorf("Did not concatenate bytes correctly, expected %s, got %s", expected, result)
	}
}
