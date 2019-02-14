/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger"
	lgrutil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	protopeer "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

// verifier provides functions that help tests with less verbose code for querying the ledger
// and verifying the actual results with the expected results
// For the straight forward functions, tests can call them directly on the ledger
type verifier struct {
	lgr    ledger.PeerLedger
	assert *assert.Assertions
	t      *testing.T
}

func newVerifier(lgr ledger.PeerLedger, t *testing.T) *verifier {
	return &verifier{lgr, assert.New(t), t}
}

func (v *verifier) verifyLedgerHeight(expectedHt uint64) {
	info, err := v.lgr.GetBlockchainInfo()
	v.assert.NoError(err)
	v.assert.Equal(expectedHt, info.Height)
}

func (v *verifier) verifyPubState(ns, key string, expectedVal string) {
	qe, err := v.lgr.NewQueryExecutor()
	v.assert.NoError(err)
	defer qe.Done()
	committedVal, err := qe.GetState(ns, key)
	v.assert.NoError(err)
	v.t.Logf("val=%s", committedVal)
	var expectedValBytes []byte
	if expectedVal != "" {
		expectedValBytes = []byte(expectedVal)
	}
	v.assert.Equal(expectedValBytes, committedVal)
}

func (v *verifier) verifyPvtState(ns, coll, key string, expectedVal string) {
	qe, err := v.lgr.NewQueryExecutor()
	v.assert.NoError(err)
	defer qe.Done()
	committedVal, err := qe.GetPrivateData(ns, coll, key)
	v.assert.NoError(err)
	v.t.Logf("val=%s", committedVal)
	var expectedValBytes []byte
	if expectedVal != "" {
		expectedValBytes = []byte(expectedVal)
	}
	v.assert.Equal(expectedValBytes, committedVal)
}

func (v *verifier) verifyBlockAndPvtData(blockNum uint64, filter ledger.PvtNsCollFilter, verifyLogic func(r *retrievedBlockAndPvtdata)) {
	out, err := v.lgr.GetPvtDataAndBlockByNum(blockNum, filter)
	v.assert.NoError(err)
	v.t.Logf("Retrieved Block = %s, pvtdata = %s", spew.Sdump(out.Block), spew.Sdump(out.BlockPvtData))
	verifyLogic(&retrievedBlockAndPvtdata{out, v.assert})
}

func (v *verifier) verifyBlockAndPvtDataSameAs(blockNum uint64, expectedOut *ledger.BlockAndPvtData) {
	v.verifyBlockAndPvtData(blockNum, nil, func(r *retrievedBlockAndPvtdata) {
		r.sameAs(expectedOut)
	})
}

func (v *verifier) verifyGetTransactionByID(txid string, expectedOut *protopeer.ProcessedTransaction) {
	tran, err := v.lgr.GetTransactionByID(txid)
	v.assert.NoError(err)
	v.assert.Equal(expectedOut.TransactionEnvelope, tran.TransactionEnvelope)
	v.assert.Equal(expectedOut.ValidationCode, tran.ValidationCode)
}

func (v *verifier) verifyTxValidationCode(txid string, expectedCode protopeer.TxValidationCode) {
	tran, err := v.lgr.GetTransactionByID(txid)
	v.assert.NoError(err)
	v.assert.Equal(int32(expectedCode), tran.ValidationCode)
}

////////////  structs used by verifier  //////////////////////////////////////////////////////////////
type expectedCollConfInfo struct {
	committingBlockNum uint64
	collConfs          []*collConf
}

type retrievedBlockAndPvtdata struct {
	*ledger.BlockAndPvtData
	assert *assert.Assertions
}

func (r *retrievedBlockAndPvtdata) hasNumTx(numTx int) {
	r.assert.Len(r.Block.Data.Data, numTx)
}

func (r *retrievedBlockAndPvtdata) hasNoPvtdata() {
	r.assert.Len(r.BlockPvtData, 0)
}

func (r *retrievedBlockAndPvtdata) pvtdataShouldContain(txSeq int, ns, coll, key, value string) {
	txPvtData := r.BlockAndPvtData.BlockPvtData[uint64(txSeq)]
	for _, nsdata := range txPvtData.WriteSet.NsPvtRwset {
		if nsdata.Namespace == ns {
			for _, colldata := range nsdata.CollectionPvtRwset {
				if colldata.CollectionName == coll {
					rwset := &kvrwset.KVRWSet{}
					r.assert.NoError(proto.Unmarshal(colldata.Rwset, rwset))
					for _, w := range rwset.Writes {
						if w.Key == key {
							r.assert.Equal([]byte(value), w.Value)
							return
						}
					}
				}
			}
		}
	}
	r.assert.FailNow("Requested kv not found")
}

func (r *retrievedBlockAndPvtdata) pvtdataShouldNotContain(ns, coll string) {
	allTxPvtData := r.BlockAndPvtData.BlockPvtData
	for _, txPvtData := range allTxPvtData {
		r.assert.False(txPvtData.Has(ns, coll))
	}
}

func (r *retrievedBlockAndPvtdata) sameBlockHeaderAndData(expectedBlock *common.Block) {
	r.assert.Equal(expectedBlock.Data, r.BlockAndPvtData.Block.Data)
	r.assert.Equal(expectedBlock.Header, r.BlockAndPvtData.Block.Header)
}

func (r *retrievedBlockAndPvtdata) sameMetadata(expectedBlock *common.Block) {
	// marshalling/unmarshalling treats a nil byte and empty byte interchangeably (based on which scheme is chosen proto vs gob)
	// so explicitly comparing each metadata
	retrievedMetadata := r.Block.Metadata.Metadata
	expectedMetadata := expectedBlock.Metadata.Metadata
	r.assert.Equal(len(expectedMetadata), len(retrievedMetadata))
	for i := 0; i < len(expectedMetadata); i++ {
		if len(expectedMetadata[i])+len(retrievedMetadata[i]) != 0 {
			if i != int(common.BlockMetadataIndex_COMMIT_HASH) {
				r.assert.Equal(expectedMetadata[i], retrievedMetadata[i])
			} else {
				// in order to compare the exact hash value, we need to duplicate the
				// production code in this test too (which is not recommended).
				commitHash := &common.Metadata{}
				err := proto.Unmarshal(retrievedMetadata[common.BlockMetadataIndex_COMMIT_HASH],
					commitHash)
				r.assert.NoError(err)
				r.assert.Equal(len(commitHash.Value), 32)
			}
		}
	}
}

func (r *retrievedBlockAndPvtdata) containsValidationCode(txSeq int, validationCode protopeer.TxValidationCode) {
	var txFilter lgrutil.TxValidationFlags
	txFilter = r.BlockAndPvtData.Block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER]
	r.assert.Equal(validationCode, txFilter.Flag(txSeq))
}

func (r *retrievedBlockAndPvtdata) sameAs(expectedBlockAndPvtdata *ledger.BlockAndPvtData) {
	r.assert.Equal(len(expectedBlockAndPvtdata.BlockPvtData), len(r.BlockAndPvtData.BlockPvtData))
	if len(expectedBlockAndPvtdata.BlockPvtData) > 0 {
		r.assert.Equal(expectedBlockAndPvtdata.BlockPvtData, r.BlockAndPvtData.BlockPvtData)
	}
	r.sameBlockHeaderAndData(expectedBlockAndPvtdata.Block)
	r.sameMetadata(expectedBlockAndPvtdata.Block)
}
