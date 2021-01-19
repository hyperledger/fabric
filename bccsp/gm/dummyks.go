/*
Copyright Suzhou Tongji Fintech Research Institute 2017 All Rights Reserved.
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
package gm

import (
	"errors"

	"github.com/tjfoc/hyperledger-fabric-gm/bccsp"
)

//模拟实现
func NewDummyKeyStore() bccsp.KeyStore {
	return &dummyKeyStore{}
}

// 模拟的ks，实现 bccsp.KeyStore 接口
type dummyKeyStore struct {
}

// read only
func (ks *dummyKeyStore) ReadOnly() bool {
	return true
}

//test GetKey
func (ks *dummyKeyStore) GetKey(ski []byte) (k bccsp.Key, err error) {
	return nil, errors.New("Key not found. This is a dummy KeyStore")
}

//test StoreKey
func (ks *dummyKeyStore) StoreKey(k bccsp.Key) (err error) {
	return errors.New("Cannot store key. This is a dummy read-only KeyStore")
}
