/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/protos/common"
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

func TestSerializedBlockInfo(t *testing.T) {
	t.Run("txID is present in all transaction", func(t *testing.T) {
		block := testutil.ConstructTestBlock(t, 1, 10, 100)
		testSerializedBlockInfo(t, block)
	})

	t.Run("txID is not present in the 2nd transaction", func(t *testing.T) {
		block := testutil.ConstructTestBlock(t, 1, 10, 100)
		// unmarshal tx-2
		envelope := protoutil.UnmarshalEnvelopeOrPanic(block.Data.Data[2])
		payload := protoutil.UnmarshalPayloadOrPanic(envelope.Payload)
		chdr := protoutil.UnmarshalChannelHeaderOrPanic(payload.Header.ChannelHeader)
		// unset txid
		chdr.TxId = ""
		// marshal tx-2
		payload.Header.ChannelHeader = protoutil.MarshalOrPanic(chdr)
		envelope.Payload = protoutil.MarshalOrPanic(payload)
		block.Data.Data[2] = protoutil.MarshalOrPanic(envelope)
		testSerializedBlockInfo(t, block)
	})
}

func testSerializedBlockInfo(t *testing.T, block *common.Block) {
	bb, info, err := serializeBlock(block)
	assert.NoError(t, err)
	infoFromBB, err := extractSerializedBlockInfo(bb)
	assert.NoError(t, err)
	assert.Equal(t, info, infoFromBB)
	assert.Equal(t, len(block.Data.Data), len(info.txOffsets))
	for txIndex, txEnvBytes := range block.Data.Data {
		txid, err := protoutil.GetOrComputeTxIDFromEnvelope(txEnvBytes)
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
