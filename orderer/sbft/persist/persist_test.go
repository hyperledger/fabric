/*
Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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

package persist

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestCreatePersist(t *testing.T) {
	tempdir := fmt.Sprintf("/tmp/persist_test_%d", rand.Int())
	p := New(tempdir)
	if p == nil {
		t.Fatalf("Failed to create Persist.")
	}
}

func TestStoreState(t *testing.T) {
	tempdir := fmt.Sprintf("/tmp/persist_test_%d", rand.Int())
	p := New(tempdir)
	err := p.StoreState("key", []byte{0, 1})
	if err != nil {
		t.Fatalf("Failed to store state.")
	}
}

func TestReadState(t *testing.T) {
	tempdir := fmt.Sprintf("/tmp/persist_test_%d", rand.Int())
	key := "key"
	var v byte = 22
	value := []byte{v}
	p := New(tempdir)
	err := p.StoreState(key, value)
	if err != nil {
		t.Fatalf("Failed to store state.")
	}
	value2, err := p.ReadState(key)
	if err != nil {
		t.Fatalf("Failed to read state.")
	}
	if len(value2) != 1 || value2[0] != v {
		t.Fatalf("The read state is not equal to the one written.")
	}
}

func TestErroneousReadState(t *testing.T) {
	tempdir := fmt.Sprintf("/tmp/persist_test_%d", rand.Int())
	key := "key"
	p := New(tempdir)
	_, err := p.ReadState(key)
	if err == nil {
		t.Fatalf("A non-existing key cannot be read.")
	}
}

func TestDelState(t *testing.T) {
	tempdir := fmt.Sprintf("/tmp/persist_test_%d", rand.Int())
	key := "key"
	var v byte = 22
	value := []byte{v}
	p := New(tempdir)
	err := p.StoreState(key, value)
	if err != nil {
		t.Fatalf("Failed to store state.")
	}
	p.DelState(key)
	_, err = p.ReadState(key)
	if err == nil {
		t.Fatalf("Failed to delete state.")
	}
}

func TestReadStateSet(t *testing.T) {
	tempdir := fmt.Sprintf("/tmp/persist_test_%d", rand.Int())
	prefix := "keys"
	key1 := "keys.one"
	key2 := "keys.two"
	var v1 byte = 22
	value1 := []byte{v1}
	var v2 byte = 57
	value2 := []byte{v2}

	p := New(tempdir)

	err1 := p.StoreState(key1, value1)
	if err1 != nil {
		t.Fatalf("Failed to store state: %s (1)", err1)
	}
	err2 := p.StoreState(key2, value2)
	if err2 != nil {
		t.Fatalf("Failed to store state: %s (2)", err2)
	}

	set, err := p.ReadStateSet(prefix)
	if err != nil {
		t.Fatalf("Failed to read state set.")
	}
	retrv1, ok1 := set[key1]
	if !ok1 || len(retrv1) != 1 || retrv1[0] != v1 {
		t.Fatalf("The read state is not equal to the one written. (1)")
	}
	retrv2, ok2 := set[key2]
	if !ok2 || len(retrv2) != 1 || retrv2[0] != v2 {
		t.Fatalf("The read state is not equal to the one written. (2)")
	}

	if len(set) != 2 {
		t.Fatalf("Too much item in the read state set.")
	}
}
