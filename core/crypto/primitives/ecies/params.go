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

package ecies

import (
	"crypto"
	"crypto/cipher"
	"hash"
)

// Params ECIES parameters
type Params struct {
	Hash      func() hash.Hash
	hashAlgo  crypto.Hash
	Cipher    func([]byte) (cipher.Block, error)
	BlockSize int
	KeyLen    int
}
