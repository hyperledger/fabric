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

package fsblkstorage

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	putils "github.com/hyperledger/fabric/protos/utils"
)

func TestBlockSerialization(t *testing.T) {
	block := testutil.ConstructTestBlock(t, 1, 10, 100)
	bb, _, err := serializeBlock(block)
	testutil.AssertNoError(t, err, "")
	deserializedBlock, err := deserializeBlock(bb)
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, deserializedBlock, block)
}

func TestExtractTxid(t *testing.T) {
	txEnv, txid, _ := testutil.ConstructTransaction(t, testutil.ConstructRandomBytes(t, 50), false)
	txEnvBytes, _ := putils.GetBytesEnvelope(txEnv)
	extractedTxid, err := extractTxID(txEnvBytes)
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, extractedTxid, txid)
}

func TestSerializedBlockInfo(t *testing.T) {
	block := testutil.ConstructTestBlock(t, 1, 10, 100)
	bb, info, err := serializeBlock(block)
	testutil.AssertNoError(t, err, "")
	infoFromBB, err := extractSerializedBlockInfo(bb)
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, infoFromBB, info)
	testutil.AssertEquals(t, len(info.txOffsets), len(block.Data.Data))
	for txIndex, txEnvBytes := range block.Data.Data {
		txid, err := extractTxID(txEnvBytes)
		testutil.AssertNoError(t, err, "")

		indexInfo := info.txOffsets[txIndex]
		indexTxID := indexInfo.txID
		indexOffset := indexInfo.loc

		testutil.AssertEquals(t, txid, indexTxID)
		b := bb[indexOffset.offset:]
		len, num := proto.DecodeVarint(b)
		txEnvBytesFromBB := b[num : num+int(len)]
		testutil.AssertEquals(t, txEnvBytesFromBB, txEnvBytes)
	}
}
