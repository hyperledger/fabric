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

package persist

import (
	"github.com/hyperledger/fabric/core/db"
)

// Helper provides an abstraction to access the Persist column family
// in the database.
type Helper struct{}

// StoreState stores a key,value pair
func (h *Helper) StoreState(key string, value []byte) error {
	db := db.GetDBHandle()
	return db.Put(db.PersistCF, []byte("consensus."+key), value)
}

// DelState removes a key,value pair
func (h *Helper) DelState(key string) {
	db := db.GetDBHandle()
	db.Delete(db.PersistCF, []byte("consensus."+key))
}

// ReadState retrieves a value to a key
func (h *Helper) ReadState(key string) ([]byte, error) {
	db := db.GetDBHandle()
	return db.Get(db.PersistCF, []byte("consensus."+key))
}

// ReadStateSet retrieves all key,value pairs where the key starts with prefix
func (h *Helper) ReadStateSet(prefix string) (map[string][]byte, error) {
	db := db.GetDBHandle()
	prefixRaw := []byte("consensus." + prefix)

	ret := make(map[string][]byte)
	it := db.GetIterator(db.PersistCF)
	defer it.Close()
	for it.Seek(prefixRaw); it.ValidForPrefix(prefixRaw); it.Next() {
		key := string(it.Key().Data())
		key = key[len("consensus."):]
		// copy data from the slice!
		ret[key] = append([]byte(nil), it.Value().Data()...)
	}
	return ret, nil
}
