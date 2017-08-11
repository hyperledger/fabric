/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatatxmgr

import (
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.SetModuleLevel("pvtdatatxmgr", "debug")
	flogging.SetModuleLevel("statebasedval", "debug")
	flogging.SetModuleLevel("valimpl", "debug")

	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger/txmgmt/txmgr/pvtdatatxmgr")
	os.Exit(m.Run())
}

func TestTransientHandlerTxmgr(t *testing.T) {
	for _, testEnv := range TestEnvs {
		testEnv.Init(t, "testledger")
		defer testEnv.Cleanup()
		testTransientHandlerTxmgr(t, testEnv)
	}
}

func testTransientHandlerTxmgr(t *testing.T, testEnv *TestEnv) {
	testcase := testEnv.Name
	t.Run(testcase, func(t *testing.T) {
		// initially the transient store is empty
		txid := "test-tx-id"
		initialEntries := retrieveTestEntriesFromTStore(t, testEnv.TStore, txid)
		t.Logf("len(initialEntries)=%d", len(initialEntries))
		assert.Nil(t, initialEntries)
		txmgr := testEnv.Txmgr

		// run a simulation with only public data and the transient store should be empty at the end
		sim1, err := txmgr.NewTxSimulator(txid)
		assert.NoError(t, err)
		sim1.GetState("ns1", "key1")
		sim1.SetState("ns1", "key1", []byte("value1"))
		_, err = sim1.GetTxSimulationResults()
		assert.NoError(t, err)
		entriesAfterPubSimulation := retrieveTestEntriesFromTStore(t, testEnv.TStore, txid)
		assert.Nil(t, entriesAfterPubSimulation)

		// run a read-only private simulation and the transient store should be empty at the end
		sim2, err := txmgr.NewTxSimulator(txid)
		assert.NoError(t, err)
		sim2.GetState("ns1", "key1")
		sim2.SetState("ns1", "key1", []byte("value1"))
		sim2.GetPrivateData("ns1", "key1", "coll1")
		_, err = sim2.GetTxSimulationResults()
		assert.NoError(t, err)
		entriesAfterReadOnlyPvtSimulation := retrieveTestEntriesFromTStore(t, testEnv.TStore, txid)
		assert.Nil(t, entriesAfterReadOnlyPvtSimulation)

		// run a private simulation that inlovles writes and the transient store should have a corresponding entry at the end
		sim3, err := txmgr.NewTxSimulator(txid)
		assert.NoError(t, err)
		sim3.GetState("ns1", "key1")
		sim3.SetState("ns1", "key1", []byte("value1"))
		sim3.GetPrivateData("ns1", "key1", "coll1")
		sim3.SetPrivateData("ns1", "key1", "coll1", []byte("value1"))
		sim3Res, err := sim3.GetTxSimulationResults()
		assert.NoError(t, err)
		sim3ResBytes, err := sim3Res.GetPvtSimulationBytes()
		assert.NoError(t, err)
		entriesAfterWritePvtSimulation := retrieveTestEntriesFromTStore(t, testEnv.TStore, txid)
		assert.Equal(t, 1, len(entriesAfterWritePvtSimulation))
		assert.Equal(t, sim3ResBytes, entriesAfterWritePvtSimulation[0].PvtSimulationResults)
	})
}

func TestTxsWithPvtData(t *testing.T) {
	for _, testEnv := range TestEnvs {
		testTxsWithPvtData(t, testEnv)
	}
}

func testTxsWithPvtData(t *testing.T, testEnv *TestEnv) {
	t.Run(testEnv.Name, func(t *testing.T) {
		testEnv.Init(t, "testledger")
		defer testEnv.Cleanup()
		bg, _ := testutil.NewBlockGenerator(t, testEnv.LedgerID, false)

		// simulate first tx and commit it
		txid1 := "txid1"
		sim1, _ := testEnv.Txmgr.NewTxSimulator(txid1)
		sim1.SetState("ns", "key1", []byte(txid1+"value1"))
		sim1.SetPrivateData("ns", "coll1", "key1", []byte(txid1+"pvt-value1"))
		sim1.SetPrivateData("ns", "coll1", "key2", []byte(txid1+"pvt-value2"))
		sim1.Done()
		res1, _ := sim1.GetTxSimulationResults()
		checkValidateAndCommit(t, bg, testEnv.Txmgr,
			[]*TestTx{&TestTx{ID: txid1, SimulationResults: res1}},
			[]int{},
		)
		checkPvtCommittedValue(t, testEnv, "ns", "coll1", "key1", []byte(txid1+"pvt-value1"), version.NewHeight(1, 0))

		// simulate two txs - tx2 and tx3
		txid2 := "txid2"
		sim2, _ := testEnv.Txmgr.NewTxSimulator(txid2)
		_, err := sim2.GetPrivateData("ns", "coll1", "key1")
		assert.NoError(t, err)
		sim2.SetPrivateData("ns", "coll1", "key1", []byte(txid2+"pvt-value1"))
		sim2.Done()
		res2, _ := sim2.GetTxSimulationResults()

		txid3 := "txid3"
		sim3, _ := testEnv.Txmgr.NewTxSimulator(txid3)
		_, err = sim3.GetPrivateData("ns", "coll1", "key1")
		assert.NoError(t, err)
		sim3.SetPrivateData("ns", "coll1", "key1", []byte(txid3+"pvt-value1"))
		sim3.Done()
		res3, _ := sim3.GetTxSimulationResults()
		// tx2 should pass but tx3 should fail the validation (because tx3 conflicts with tx2 in the same block on pvt data)
		checkValidateAndCommit(t, bg, testEnv.Txmgr,
			[]*TestTx{
				&TestTx{ID: txid2, SimulationResults: res2},
				&TestTx{ID: txid3, SimulationResults: res3},
			},
			[]int{1},
		)
		checkPvtCommittedValue(t, testEnv, "ns", "coll1", "key2", []byte(txid1+"pvt-value2"), version.NewHeight(1, 0))
		checkPvtCommittedValue(t, testEnv, "ns", "coll1", "key1", []byte(txid2+"pvt-value1"), version.NewHeight(2, 0))
	})
}

func TestTxsWithPvtDataReadOnly(t *testing.T) {
	for _, testEnv := range TestEnvs {
		testTxsWithPvtDataReadOnly(t, testEnv)
	}
}

func testTxsWithPvtDataReadOnly(t *testing.T, testEnv *TestEnv) {
	t.Run(testEnv.Name, func(t *testing.T) {
		testEnv.Init(t, "testledger")
		defer testEnv.Cleanup()
		bg, _ := testutil.NewBlockGenerator(t, testEnv.LedgerID, false)

		// simulate first tx and commit it
		txid1 := "txid1"
		sim1, _ := testEnv.Txmgr.NewTxSimulator(txid1)
		sim1.GetPrivateData("ns", "coll1", "key2")
		sim1.SetState("ns", "key1", []byte(txid1+"value1"))
		sim1.Done()
		res1, _ := sim1.GetTxSimulationResults()
		checkValidateAndCommit(t, bg, testEnv.Txmgr,
			[]*TestTx{&TestTx{ID: txid1, SimulationResults: res1}},
			[]int{},
		)
		checkPubCommittedValue(t, testEnv, "ns", "key1", []byte(txid1+"value1"), version.NewHeight(1, 0))
	})
}

func TestPvtDataWrongHash(t *testing.T) {
	for _, testEnv := range TestEnvs {
		testPvtDataWrongHash(t, testEnv)
	}
}

func testPvtDataWrongHash(t *testing.T, testEnv *TestEnv) {
	t.Run(testEnv.Name, func(t *testing.T) {
		testEnv.Init(t, "testledger")
		defer testEnv.Cleanup()
		bg, _ := testutil.NewBlockGenerator(t, testEnv.LedgerID, false)

		txid1 := "txid1"
		sim1, _ := testEnv.Txmgr.NewTxSimulator(txid1)
		sim1.SetState("ns", "key1", []byte(txid1+"value1"))
		sim1.SetPrivateData("ns", "coll1", "key1", []byte(txid1+"pvt-value1"))
		sim1.SetPrivateData("ns", "coll1", "key2", []byte(txid1+"pvt-value2"))
		sim1.Done()
		res1, _ := sim1.GetTxSimulationResults()
		pubSim1 := res1.PubSimulationResults
		pubSim1.NsRwset[0].CollectionHashedRwset[0].PvtRwsetHash = []byte("wrong-hash-for-testing")
		pubSimBytes1, _ := proto.Marshal(pubSim1)
		block := bg.NextBlockWithTxid([][]byte{pubSimBytes1}, []string{txid1})
		err := testEnv.Txmgr.ValidateAndPrepare(
			buildBlockAndPvtdataForTest(block, res1.PvtSimulationResults),
			true)
		t.Logf("Error is expected.. err:%s", err)
		testutil.AssertError(t, err, "An error is expected because the actual hash generated at simulation time is replaced")
	})
}

func retrieveTestEntriesFromTStore(t *testing.T, tStore transientstore.Store, txid string) []*transientstore.EndorserPvtSimulationResults {
	itr, err := tStore.GetTxPvtRWSetByTxid(txid)
	assert.NoError(t, err)
	var results []*transientstore.EndorserPvtSimulationResults
	for {
		result, err := itr.Next()
		assert.NoError(t, err)
		if result == nil {
			break
		}
		results = append(results, result.(*transientstore.EndorserPvtSimulationResults))
	}
	return results
}

func checkValidateAndCommit(t *testing.T, bg *testutil.BlockGenerator, txmgr txmgr.TxMgr, txs []*TestTx, expectedInvalidTxIndexs []int) {
	var pubSimData [][]byte
	var pvtSimData []*rwset.TxPvtReadWriteSet
	var txids []string
	for _, tx := range txs {
		PubSimBytes, err := tx.SimulationResults.GetPubSimulationBytes()
		testutil.AssertNoError(t, err, "")

		pubSimData = append(pubSimData, PubSimBytes)
		pvtSimData = append(pvtSimData, tx.SimulationResults.PvtSimulationResults)
		txids = append(txids, tx.ID)
	}
	block := bg.NextBlockWithTxid(pubSimData, txids)
	testutil.AssertNoError(t, txmgr.ValidateAndPrepare(buildBlockAndPvtdataForTest(block, pvtSimData...), true), "")
	testutil.AssertNoError(t, txmgr.Commit(), "")

	txsFltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	expectedInvalidTxs := make(map[int]bool)
	for _, i := range expectedInvalidTxIndexs {
		expectedInvalidTxs[i] = true
	}

	for i := 0; i < len(block.Data.Data); i++ {
		if txsFltr.IsValid(i) && expectedInvalidTxs[i] {
			t.Fatalf("Tx at index %d is expected to be invalid", i)
		}
		if txsFltr.IsInvalid(i) && !expectedInvalidTxs[i] {
			t.Fatalf("Tx at index %d is expected to be valid", i)
		}
	}
}

func checkPvtCommittedValue(t *testing.T, testEnv *TestEnv, ns, coll, key string, expectedVal []byte, expectedVer *version.Height) {
	vv, _ := testEnv.DB.GetPrivateData(ns, coll, key)
	testutil.AssertEquals(t, vv.Value, expectedVal)
	testutil.AssertEquals(t, vv.Version, expectedVer)
}

func checkPubCommittedValue(t *testing.T, testEnv *TestEnv, ns, key string, expectedVal []byte, expectedVer *version.Height) {
	vv, _ := testEnv.DB.GetState(ns, key)
	testutil.AssertEquals(t, vv.Value, expectedVal)
	testutil.AssertEquals(t, vv.Version, expectedVer)
}

func buildBlockAndPvtdataForTest(block *common.Block, pvtdata ...*rwset.TxPvtReadWriteSet) *ledger.BlockAndPvtData {
	var m map[uint64]*ledger.TxPvtData
	if len(pvtdata) != 0 {
		m = make(map[uint64]*ledger.TxPvtData)
	}
	for i, txPvtdata := range pvtdata {
		m[uint64(i)] = &ledger.TxPvtData{SeqInBlock: uint64(i), WriteSet: txPvtdata}
	}
	return &ledger.BlockAndPvtData{Block: block, BlockPvtData: m}
}
