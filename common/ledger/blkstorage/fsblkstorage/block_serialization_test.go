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
	"github.com/stretchr/testify/assert"
)

func TestBlockSerialization(t *testing.T) {
	block := testutil.ConstructTestBlock(t, 1, 10, 100)
	bb, _, err := serializeBlock(block)
	assert.NoError(t, err)
	deserializedBlock, err := deserializeBlock(bb)
	assert.NoError(t, err)
	assert.Equal(t, block, deserializedBlock)
}

func TestExtractTxid(t *testing.T) {
	txEnv, txid, _ := testutil.ConstructTransaction(t, testutil.ConstructRandomBytes(t, 50), "", false)
	txEnvBytes, _ := putils.GetBytesEnvelope(txEnv)
	extractedTxid, err := extractTxID(txEnvBytes)
	assert.NoError(t, err)
	assert.Equal(t, txid, extractedTxid)
}

func TestSerializedBlockInfo(t *testing.T) {
	block := testutil.ConstructTestBlock(t, 1, 10, 100)
	bb, info, err := serializeBlock(block)
	assert.NoError(t, err)
	infoFromBB, err := extractSerializedBlockInfo(bb)
	assert.NoError(t, err)
	assert.Equal(t, info, infoFromBB)
	assert.Equal(t, len(block.Data.Data), len(info.txOffsets))
	for txIndex, txEnvBytes := range block.Data.Data {
		txid, err := extractTxID(txEnvBytes)
		assert.NoError(t, err)

		indexInfo := info.txOffsets[txIndex]
		indexTxID := indexInfo.txID
		indexOffset := indexInfo.loc

		assert.Equal(t, indexTxID, txid)
		b := bb[indexOffset.offset:]
		length, num := proto.DecodeVarint(b)
		txEnvBytesFromBB := b[num : num+int(length)]
		assert.Equal(t, txEnvBytes, txEnvBytesFromBB)
	}
}
