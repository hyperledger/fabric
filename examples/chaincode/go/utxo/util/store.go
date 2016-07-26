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

// Store interface describes the storage used by this chaincode. The interface
// was created so either the state database store can be used or a in memory
// store can be used for unit testing.
type Store interface {
	GetState(Key) (*TX_TXOUT, bool, error)
	PutState(Key, *TX_TXOUT) error
	DelState(Key) error
	GetTran(string) ([]byte, bool, error)
	PutTran(string, []byte) error
}

// InMemoryStore used for unit testing
type InMemoryStore struct {
	Map     map[Key]*TX_TXOUT
	TranMap map[string][]byte
}

// MakeInMemoryStore creates a new in memory store
func MakeInMemoryStore() Store {
	ims := &InMemoryStore{}
	ims.Map = make(map[Key]*TX_TXOUT)
	ims.TranMap = make(map[string][]byte)
	return ims
}

// GetState returns the transaction for the given key
func (ims *InMemoryStore) GetState(key Key) (*TX_TXOUT, bool, error) {
	value, ok := ims.Map[key]
	return value, ok, nil
}

// DelState deletes the given key and corresponding transactions
func (ims *InMemoryStore) DelState(key Key) error {
	delete(ims.Map, key)
	return nil
}

// PutState saves the key and transaction in memory
func (ims *InMemoryStore) PutState(key Key, value *TX_TXOUT) error {
	ims.Map[key] = value
	return nil
}

// GetTran returns the transaction for the given hash
func (ims *InMemoryStore) GetTran(key string) ([]byte, bool, error) {
	value, ok := ims.TranMap[key]
	return value, ok, nil
}

// PutTran saves the hash and transaction in memory
func (ims *InMemoryStore) PutTran(key string, value []byte) error {
	ims.TranMap[key] = value
	return nil
}
