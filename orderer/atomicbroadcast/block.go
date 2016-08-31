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

package atomicbroadcast

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/util"
)

func (b *Block) Hash() []byte {
	data, err := proto.Marshal(b) // XXX this is wrong, protobuf is not the right mechanism to serialize for a hash
	if err != nil {
		panic("This should never fail and is generally irrecoverable")
	}

	return util.ComputeCryptoHash(data)
}
