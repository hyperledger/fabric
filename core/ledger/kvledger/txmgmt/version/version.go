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

package version

import "github.com/hyperledger/fabric/common/ledger/util"

// Height represents the height of a transaction in blockchain
type Height struct {
	BlockNum uint64
	TxNum    uint64
}

// NewHeight constructs a new instance of Height
func NewHeight(blockNum, txNum uint64) *Height {
	return &Height{blockNum, txNum}
}

// NewHeightFromBytes constructs a new instance of Height from serialized bytes
func NewHeightFromBytes(b []byte) (*Height, int) {
	blockNum, n1 := util.DecodeOrderPreservingVarUint64(b)
	txNum, n2 := util.DecodeOrderPreservingVarUint64(b[n1:])
	return NewHeight(blockNum, txNum), n1 + n2
}

// ToBytes serializes the Height
func (h *Height) ToBytes() []byte {
	blockNumBytes := util.EncodeOrderPreservingVarUint64(h.BlockNum)
	txNumBytes := util.EncodeOrderPreservingVarUint64(h.TxNum)
	return append(blockNumBytes, txNumBytes...)
}

// Compare return a -1, zero, or +1 based on whether this height is
// less than, equals to, or greater than the specified height respectively.
func (h *Height) Compare(h1 *Height) int {
	res := 0
	switch {
	case h.BlockNum != h1.BlockNum:
		res = int(h.BlockNum - h1.BlockNum)
		break
	case h.TxNum != h1.TxNum:
		res = int(h.TxNum - h1.TxNum)
		break
	default:
		return 0
	}
	if res > 0 {
		return 1
	}
	return -1
}

// AreSame returns true if both the heights are either nil or equal
func AreSame(h1 *Height, h2 *Height) bool {
	if h1 == nil {
		return h2 == nil
	}
	if h2 == nil {
		return false
	}
	return h1.Compare(h2) == 0
}
