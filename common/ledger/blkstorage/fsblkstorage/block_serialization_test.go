/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/protoutil"
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
	txEnvBytes, _ := protoutil.GetBytesEnvelope(txEnv)
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
