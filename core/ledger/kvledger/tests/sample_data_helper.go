/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"fmt"
	"testing"

	protopeer "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/stretchr/testify/require"
)

type sampleDataHelper struct {
	submittedData submittedData
	assert        *require.Assertions
	t             *testing.T
}

func newSampleDataHelper(t *testing.T) *sampleDataHelper {
	return &sampleDataHelper{make(submittedData), require.New(t), t}
}

func (d *sampleDataHelper) populateLedger(l *testLedger) {
	lgrid := l.lgrid
	// blk1 deploys 2 chaincodes
	txdeploy1 := l.simulateDeployTx("cc1", nil)
	txdeploy2 := l.simulateDeployTx("cc2", nil)
	blk1 := l.cutBlockAndCommitLegacy()

	// blk2 contains 2 public data txs
	txdata1 := l.simulateDataTx("txid1", func(s *simulator) {
		s.setState("cc1", "key1", d.sampleVal("value01", lgrid))
		s.setState("cc1", "key2", d.sampleVal("value02", lgrid))
	})

	txdata2 := l.simulateDataTx("txid2", func(s *simulator) {
		s.setState("cc2", "key1", d.sampleVal("value03", lgrid))
		s.setState("cc2", "key2", d.sampleVal("value04", lgrid))
	})
	blk2 := l.cutBlockAndCommitLegacy()

	// blk3 upgrades both chaincodes
	txupgrade1 := l.simulateUpgradeTx("cc1", d.sampleCollConf1(lgrid, "cc1"))
	txupgrade2 := l.simulateUpgradeTx("cc2", d.sampleCollConf1(lgrid, "cc2"))
	blk3 := l.cutBlockAndCommitLegacy()

	// blk4 contains 2 data txs with private data
	txdata3 := l.simulateDataTx("txid3", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key3", d.sampleVal("value05", lgrid))
		s.setPvtdata("cc1", "coll1", "key4", d.sampleVal("value06", lgrid))
	})
	txdata4 := l.simulateDataTx("txid4", func(s *simulator) {
		s.setPvtdata("cc2", "coll1", "key3", d.sampleVal("value07", lgrid))
		s.setPvtdata("cc2", "coll1", "key4", d.sampleVal("value08", lgrid))
	})
	blk4 := l.cutBlockAndCommitLegacy()

	// blk5 upgrades both chaincodes
	txupgrade3 := l.simulateUpgradeTx("cc1", d.sampleCollConf2(lgrid, "cc1"))
	txupgrade4 := l.simulateDeployTx("cc2", d.sampleCollConf2(lgrid, "cc2"))
	blk5 := l.cutBlockAndCommitLegacy()

	// blk6 contains 2 data txs with private data
	txdata5 := l.simulateDataTx("txid5", func(s *simulator) {
		s.setPvtdata("cc1", "coll2", "key3", d.sampleVal("value09", lgrid))
		s.setPvtdata("cc1", "coll2", "key4", d.sampleVal("value10", lgrid))
	})
	txdata6 := l.simulateDataTx("txid6", func(s *simulator) {
		s.setPvtdata("cc2", "coll2", "key3", d.sampleVal("value11", lgrid))
		s.setPvtdata("cc2", "coll2", "key4", d.sampleVal("value12", lgrid))
	})
	blk6 := l.cutBlockAndCommitLegacy()

	// blk7 contains one data txs
	txdata7 := l.simulateDataTx("txid7", func(s *simulator) {
		s.setState("cc1", "key1", d.sampleVal("value13", lgrid))
		require.NoError(d.t, s.DeleteState("cc1", "key2"))
		s.setPvtdata("cc1", "coll1", "key3", d.sampleVal("value14", lgrid))
		require.NoError(d.t, s.DeletePrivateData("cc1", "coll1", "key4"))
	})
	l.simulatedTrans = nil

	// blk8 contains one data txs that should be marked as invalid because of mvcc conflict with tx in blk7
	txdata8 := l.simulateDataTx("txid8", func(s *simulator) {
		s.getState("cc1", "key1")
		s.setState("cc1", "key1", d.sampleVal("value15", lgrid))
	})
	blk7 := l.committer.cutBlockAndCommitLegacy([]*txAndPvtdata{txdata7}, nil)
	blk8 := l.cutBlockAndCommitLegacy()

	d.submittedData.recordSubmittedBlks(lgrid,
		blk1, blk2, blk3, blk4, blk5, blk6, blk7, blk8)
	d.submittedData.recordSubmittedTxs(lgrid,
		txdeploy1, txdeploy2, txdata1, txdata2, txupgrade1, txupgrade2,
		txdata3, txdata4, txupgrade3, txupgrade4, txdata5, txdata6, txdata7, txdata8)
}

func (d *sampleDataHelper) verifyLedgerContent(l *testLedger) {
	d.verifyState(l)
	d.verifyConfigHistory(l)
	d.verifyBlockAndPvtdata(l)
	d.verifyGetTransactionByID(l)
	// TODO: add verifyHistory() -- FAB-15733

	// the submitted data could not be available if the test ledger is loaded from disk in a fresh run
	// (e.g., a backup of a test lesger from a previous fabric version)
	if len(d.submittedData) != 0 {
		d.t.Log("Verifying using submitted data")
		d.verifyBlockAndPvtdataUsingSubmittedData(l)
		d.verifyGetTransactionByIDUsingSubmittedData(l)
	} else {
		d.t.Log("Skipping verifying using submitted data")
	}
}

func (d *sampleDataHelper) verifyState(l *testLedger) {
	lgrid := l.lgrid
	l.verifyPubState("cc1", "key1", d.sampleVal("value13", lgrid))
	l.verifyPubState("cc1", "key2", "")
	l.verifyPvtState("cc1", "coll1", "key3", d.sampleVal("value14", lgrid))
	l.verifyPvtState("cc1", "coll1", "key4", "")
	l.verifyPvtState("cc1", "coll2", "key3", d.sampleVal("value09", lgrid))
	l.verifyPvtState("cc1", "coll2", "key4", d.sampleVal("value10", lgrid))

	l.verifyPubState("cc2", "key1", d.sampleVal("value03", lgrid))
	l.verifyPubState("cc2", "key2", d.sampleVal("value04", lgrid))
	l.verifyPvtState("cc2", "coll1", "key3", d.sampleVal("value07", lgrid))
	l.verifyPvtState("cc2", "coll1", "key4", d.sampleVal("value08", lgrid))
	l.verifyPvtState("cc2", "coll2", "key3", d.sampleVal("value11", lgrid))
	l.verifyPvtState("cc2", "coll2", "key4", d.sampleVal("value12", lgrid))
}

func (d *sampleDataHelper) verifyConfigHistory(l *testLedger) {
	lgrid := l.lgrid
	l.verifyMostRecentCollectionConfigBelow(10, "cc1",
		&expectedCollConfInfo{5, d.sampleCollConf2(lgrid, "cc1")})

	l.verifyMostRecentCollectionConfigBelow(5, "cc1",
		&expectedCollConfInfo{3, d.sampleCollConf1(lgrid, "cc1")})

	l.verifyMostRecentCollectionConfigBelow(10, "cc2",
		&expectedCollConfInfo{5, d.sampleCollConf2(lgrid, "cc2")})

	l.verifyMostRecentCollectionConfigBelow(5, "cc2",
		&expectedCollConfInfo{3, d.sampleCollConf1(lgrid, "cc2")})
}

func (d *sampleDataHelper) verifyBlockAndPvtdata(l *testLedger) {
	lgrid := l.lgrid
	l.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) {
		r.hasNumTx(2)
		r.hasNoPvtdata()
	})

	l.verifyBlockAndPvtData(4, nil, func(r *retrievedBlockAndPvtdata) {
		r.hasNumTx(2)
		r.pvtdataShouldContain(0, "cc1", "coll1", "key3", d.sampleVal("value05", lgrid))
		r.pvtdataShouldContain(1, "cc2", "coll1", "key3", d.sampleVal("value07", lgrid))
	})
}

func (d *sampleDataHelper) verifyGetTransactionByID(l *testLedger) {
	l.verifyTxValidationCode("txid7", protopeer.TxValidationCode_VALID)
	l.verifyTxValidationCode("txid8", protopeer.TxValidationCode_MVCC_READ_CONFLICT)
}

func (d *sampleDataHelper) verifyBlockAndPvtdataUsingSubmittedData(l *testLedger) {
	lgrid := l.lgrid
	submittedData := d.submittedData[lgrid]
	for _, submittedBlk := range submittedData.Blocks {
		blkNum := submittedBlk.Block.Header.Number
		if blkNum != 8 {
			l.verifyBlockAndPvtDataSameAs(uint64(blkNum), submittedBlk)
		} else {
			l.verifyBlockAndPvtData(uint64(8), nil, func(r *retrievedBlockAndPvtdata) {
				r.sameBlockHeaderAndData(submittedBlk.Block)
				r.containsValidationCode(0, protopeer.TxValidationCode_MVCC_READ_CONFLICT)
				r.containsCommitHash()
			})
		}
	}
	l.verifyCommitHashExists()
}

func (d *sampleDataHelper) verifyGetTransactionByIDUsingSubmittedData(l *testLedger) {
	lgrid := l.lgrid
	for _, submittedTx := range d.submittedData[lgrid].Txs {
		expectedValidationCode := protopeer.TxValidationCode_VALID
		if submittedTx.Txid == "txid8" {
			expectedValidationCode = protopeer.TxValidationCode_MVCC_READ_CONFLICT
		}
		l.verifyGetTransactionByID(submittedTx.Txid,
			&protopeer.ProcessedTransaction{TransactionEnvelope: submittedTx.Envelope, ValidationCode: int32(expectedValidationCode)})
	}
}

func (d *sampleDataHelper) sampleVal(val, ledgerid string) string {
	return fmt.Sprintf("%s:%s", val, ledgerid)
}

func (d *sampleDataHelper) sampleCollConf1(ledgerid, ccName string) []*collConf {
	return []*collConf{
		{name: "coll1", members: []string{"org1", "org2"}},
		{name: ledgerid, members: []string{"org1", "org2"}},
		{name: ccName, members: []string{"org1", "org2"}},
	}
}

func (d *sampleDataHelper) sampleCollConf2(ledgerid string, ccName string) []*collConf {
	return []*collConf{
		{name: "coll1", members: []string{"org1", "org2"}},
		{name: "coll2", members: []string{"org1", "org2"}},
		{name: ledgerid, members: []string{"org1", "org2"}},
		{name: ccName, members: []string{"org1", "org2"}},
	}
}

type submittedData map[string]*submittedLedgerData

type submittedLedgerData struct {
	Blocks []*ledger.BlockAndPvtData
	Txs    []*txAndPvtdata
}

func (s submittedData) initForLedger(lgrid string) {
	ld := s[lgrid]
	if ld == nil {
		ld = &submittedLedgerData{}
		s[lgrid] = ld
	}
}

func (s submittedData) recordSubmittedBlks(lgrid string, blk ...*ledger.BlockAndPvtData) {
	s.initForLedger(lgrid)
	s[lgrid].Blocks = append(s[lgrid].Blocks, blk...)
}

func (s submittedData) recordSubmittedTxs(lgrid string, tx ...*txAndPvtdata) {
	s.initForLedger(lgrid)
	s[lgrid].Txs = append(s[lgrid].Txs, tx...)
}
