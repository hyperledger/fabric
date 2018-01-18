/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package valimpl

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/util"
	lutils "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func TestPreprocessProtoBlock(t *testing.T) {

	// good block
	//_, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gb := testutil.ConstructTestBlock(t, 10, 1, 1)
	_, err := preprocessProtoBlock(nil, gb, false)
	assert.NoError(t, err)
	// bad envelope
	gb = testutil.ConstructTestBlock(t, 11, 1, 1)
	gb.Data = &common.BlockData{Data: [][]byte{{123}}}
	gb.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] =
		lutils.NewTxValidationFlags(len(gb.Data.Data))
	_, err = preprocessProtoBlock(nil, gb, false)
	assert.Error(t, err)
	t.Log(err)
	// bad payload
	gb = testutil.ConstructTestBlock(t, 12, 1, 1)
	envBytes, _ := putils.GetBytesEnvelope(&common.Envelope{Payload: []byte{123}})
	gb.Data = &common.BlockData{Data: [][]byte{envBytes}}
	_, err = preprocessProtoBlock(nil, gb, false)
	assert.Error(t, err)
	t.Log(err)
	// bad channel header
	gb = testutil.ConstructTestBlock(t, 13, 1, 1)
	payloadBytes, _ := putils.GetBytesPayload(&common.Payload{
		Header: &common.Header{ChannelHeader: []byte{123}},
	})
	envBytes, _ = putils.GetBytesEnvelope(&common.Envelope{Payload: payloadBytes})
	gb.Data = &common.BlockData{Data: [][]byte{envBytes}}
	_, err = preprocessProtoBlock(nil, gb, false)
	assert.Error(t, err)
	t.Log(err)

	// bad channel header with invalid filter set
	gb = testutil.ConstructTestBlock(t, 14, 1, 1)
	payloadBytes, _ = putils.GetBytesPayload(&common.Payload{
		Header: &common.Header{ChannelHeader: []byte{123}},
	})
	envBytes, _ = putils.GetBytesEnvelope(&common.Envelope{Payload: payloadBytes})
	gb.Data = &common.BlockData{Data: [][]byte{envBytes}}
	flags := lutils.NewTxValidationFlags(len(gb.Data.Data))
	flags.SetFlag(0, peer.TxValidationCode_BAD_CHANNEL_HEADER)
	gb.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = flags
	_, err = preprocessProtoBlock(nil, gb, false)
	assert.NoError(t, err) // invalid filter should take precendence

	// new block
	var blockNum uint64 = 15
	txid := "testtxid1234"
	gb = testutil.ConstructBlockWithTxid(t, blockNum, []byte{123},
		[][]byte{{123}}, []string{txid}, false)
	flags = lutils.NewTxValidationFlags(len(gb.Data.Data))
	flags.SetFlag(0, peer.TxValidationCode_BAD_HEADER_EXTENSION)
	gb.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = flags
	// set logging backend for test
	backend := logging.NewMemoryBackend(1)
	logging.SetBackend(backend)
	_, err = preprocessProtoBlock(nil, gb, false)
	assert.NoError(t, err)
	expected := fmt.Sprintf("Channel [%s]: Block [%d] Transaction index [%d] TxId [%s]"+
		" marked as invalid by committer. Reason code [%s]",
		util.GetTestChainID(), blockNum, 0, txid, peer.TxValidationCode_BAD_HEADER_EXTENSION.String())
	t.Log(expected)
	assert.Equal(t, expected, memoryRecordN(backend, 0).Message())
	//assert.Equal(t, message, MemoryRecordN(backend, i).Message())
	t.Log(memoryRecordN(backend, 0).Message())

}

// from go-logging memory_test.go
func memoryRecordN(b *logging.MemoryBackend, n int) *logging.Record {
	node := b.Head()
	for i := 0; i < n; i++ {
		if node == nil {
			break
		}
		node = node.Next()
	}
	if node == nil {
		return nil
	}
	return node.Record
}
