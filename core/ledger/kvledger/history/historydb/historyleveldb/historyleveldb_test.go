/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package historyleveldb

import (
	"os"
	"strconv"
	"testing"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger/history/historydb/historyleveldb")
	flogging.SetModuleLevel("leveldbhelper", "debug")
	flogging.SetModuleLevel("historyleveldb", "debug")
	os.Exit(m.Run())
}

//TestSavepoint tests that save points get written after each block and get returned via GetBlockNumfromSavepoint
func TestSavepoint(t *testing.T) {
	env := newTestHistoryEnv(t)
	defer env.cleanup()

	// read the savepoint, it should not exist and should return nil Height object
	savepoint, err := env.testHistoryDB.GetLastSavepoint()
	assert.NoError(t, err, "Error upon historyDatabase.GetLastSavepoint()")
	assert.Nil(t, savepoint)

	// ShouldRecover should return true when no savepoint is found and recovery from block 0
	status, blockNum, err := env.testHistoryDB.ShouldRecover(0)
	assert.NoError(t, err, "Error upon historyDatabase.ShouldRecover()")
	assert.True(t, status)
	assert.Equal(t, uint64(0), blockNum)

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	assert.NoError(t, env.testHistoryDB.Commit(gb))
	// read the savepoint, it should now exist and return a Height object with BlockNum 0
	savepoint, err = env.testHistoryDB.GetLastSavepoint()
	assert.NoError(t, err, "Error upon historyDatabase.GetLastSavepoint()")
	assert.Equal(t, uint64(0), savepoint.BlockNum)

	// create the next block (block 1)
	txid := util2.GenerateUUID()
	simulator, _ := env.txmgr.NewTxSimulator(txid)
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimResBytes, _ := simRes.GetPubSimulationBytes()
	block1 := bg.NextBlock([][]byte{pubSimResBytes})
	assert.NoError(t, env.testHistoryDB.Commit(block1))
	savepoint, err = env.testHistoryDB.GetLastSavepoint()
	assert.NoError(t, err, "Error upon historyDatabase.GetLastSavepoint()")
	assert.Equal(t, uint64(1), savepoint.BlockNum)

	// Should Recover should return false
	status, blockNum, err = env.testHistoryDB.ShouldRecover(1)
	assert.NoError(t, err, "Error upon historyDatabase.ShouldRecover()")
	assert.False(t, status)
	assert.Equal(t, uint64(2), blockNum)

	// create the next block (block 2)
	txid = util2.GenerateUUID()
	simulator, _ = env.txmgr.NewTxSimulator(txid)
	simulator.SetState("ns1", "key1", []byte("value2"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	pubSimResBytes, _ = simRes.GetPubSimulationBytes()
	block2 := bg.NextBlock([][]byte{pubSimResBytes})

	// assume that the peer failed to commit this block to historyDB and is being recovered now
	env.testHistoryDB.CommitLostBlock(&ledger.BlockAndPvtData{Block: block2})
	savepoint, err = env.testHistoryDB.GetLastSavepoint()
	assert.NoError(t, err, "Error upon historyDatabase.GetLastSavepoint()")
	assert.Equal(t, uint64(2), savepoint.BlockNum)

	//Pass high blockNum, ShouldRecover should return true with 3 as blocknum to recover from
	status, blockNum, err = env.testHistoryDB.ShouldRecover(10)
	assert.NoError(t, err, "Error upon historyDatabase.ShouldRecover()")
	assert.True(t, status)
	assert.Equal(t, uint64(3), blockNum)
}

func TestHistory(t *testing.T) {
	env := newTestHistoryEnv(t)
	defer env.cleanup()
	provider := env.testBlockStorageEnv.provider
	ledger1id := "ledger1"
	store1, err := provider.OpenBlockStore(ledger1id)
	assert.NoError(t, err, "Error upon provider.OpenBlockStore()")
	defer store1.Shutdown()

	bg, gb := testutil.NewBlockGenerator(t, ledger1id, false)
	assert.NoError(t, store1.AddBlock(gb))
	assert.NoError(t, env.testHistoryDB.Commit(gb))

	//block1
	txid := util2.GenerateUUID()
	simulator, _ := env.txmgr.NewTxSimulator(txid)
	value1 := []byte("value1")
	simulator.SetState("ns1", "key7", value1)
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimResBytes, _ := simRes.GetPubSimulationBytes()
	block1 := bg.NextBlock([][]byte{pubSimResBytes})
	err = store1.AddBlock(block1)
	assert.NoError(t, err)
	err = env.testHistoryDB.Commit(block1)
	assert.NoError(t, err)

	//block2 tran1
	simulationResults := [][]byte{}
	txid = util2.GenerateUUID()
	simulator, _ = env.txmgr.NewTxSimulator(txid)
	value2 := []byte("value2")
	simulator.SetState("ns1", "key7", value2)
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	pubSimResBytes, _ = simRes.GetPubSimulationBytes()
	simulationResults = append(simulationResults, pubSimResBytes)
	//block2 tran2
	txid2 := util2.GenerateUUID()
	simulator2, _ := env.txmgr.NewTxSimulator(txid2)
	value3 := []byte("value3")
	simulator2.SetState("ns1", "key7", value3)
	simulator2.Done()
	simRes2, _ := simulator2.GetTxSimulationResults()
	pubSimResBytes2, _ := simRes2.GetPubSimulationBytes()
	simulationResults = append(simulationResults, pubSimResBytes2)
	block2 := bg.NextBlock(simulationResults)
	err = store1.AddBlock(block2)
	assert.NoError(t, err)
	err = env.testHistoryDB.Commit(block2)
	assert.NoError(t, err)

	//block3
	txid = util2.GenerateUUID()
	simulator, _ = env.txmgr.NewTxSimulator(txid)
	simulator.DeleteState("ns1", "key7")
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	pubSimResBytes, _ = simRes.GetPubSimulationBytes()
	block3 := bg.NextBlock([][]byte{pubSimResBytes})
	err = store1.AddBlock(block3)
	assert.NoError(t, err)
	err = env.testHistoryDB.Commit(block3)
	assert.NoError(t, err)
	t.Logf("Inserted all 3 blocks")

	qhistory, err := env.testHistoryDB.NewHistoryQueryExecutor(store1)
	assert.NoError(t, err, "Error upon NewHistoryQueryExecutor")

	itr, err2 := qhistory.GetHistoryForKey("ns1", "key7")
	assert.NoError(t, err2, "Error upon GetHistoryForKey()")

	count := 0
	for {
		kmod, _ := itr.Next()
		if kmod == nil {
			break
		}
		txid = kmod.(*queryresult.KeyModification).TxId
		retrievedValue := kmod.(*queryresult.KeyModification).Value
		retrievedTimestamp := kmod.(*queryresult.KeyModification).Timestamp
		retrievedIsDelete := kmod.(*queryresult.KeyModification).IsDelete
		t.Logf("Retrieved history record for key=key7 at TxId=%s with value %v and timestamp %v",
			txid, retrievedValue, retrievedTimestamp)
		count++
		if count != 4 {
			expectedValue := []byte("value" + strconv.Itoa(count))
			assert.Equal(t, expectedValue, retrievedValue)
			assert.NotNil(t, retrievedTimestamp)
			assert.False(t, retrievedIsDelete)
		} else {
			assert.Equal(t, []uint8(nil), retrievedValue)
			assert.NotNil(t, retrievedTimestamp)
			assert.True(t, retrievedIsDelete)
		}
	}
	assert.Equal(t, 4, count)
}

func TestHistoryForInvalidTran(t *testing.T) {
	env := newTestHistoryEnv(t)
	defer env.cleanup()
	provider := env.testBlockStorageEnv.provider
	ledger1id := "ledger1"
	store1, err := provider.OpenBlockStore(ledger1id)
	assert.NoError(t, err, "Error upon provider.OpenBlockStore()")
	defer store1.Shutdown()

	bg, gb := testutil.NewBlockGenerator(t, ledger1id, false)
	assert.NoError(t, store1.AddBlock(gb))
	assert.NoError(t, env.testHistoryDB.Commit(gb))

	//block1
	txid := util2.GenerateUUID()
	simulator, _ := env.txmgr.NewTxSimulator(txid)
	value1 := []byte("value1")
	simulator.SetState("ns1", "key7", value1)
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimResBytes, _ := simRes.GetPubSimulationBytes()
	block1 := bg.NextBlock([][]byte{pubSimResBytes})

	//for this invalid tran test, set the transaction to invalid
	txsFilter := util.TxValidationFlags(block1.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	txsFilter.SetFlag(0, peer.TxValidationCode_INVALID_OTHER_REASON)
	block1.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter

	err = store1.AddBlock(block1)
	assert.NoError(t, err)
	err = env.testHistoryDB.Commit(block1)
	assert.NoError(t, err)

	qhistory, err := env.testHistoryDB.NewHistoryQueryExecutor(store1)
	assert.NoError(t, err, "Error upon NewHistoryQueryExecutor")

	itr, err2 := qhistory.GetHistoryForKey("ns1", "key7")
	assert.NoError(t, err2, "Error upon GetHistoryForKey()")

	// test that there are no history values, since the tran was marked as invalid
	kmod, _ := itr.Next()
	assert.Nil(t, kmod)
}

//TestSavepoint tests that save points get written after each block and get returned via GetBlockNumfromSavepoint
func TestHistoryDisabled(t *testing.T) {
	env := newTestHistoryEnv(t)
	defer env.cleanup()
	viper.Set("ledger.history.enableHistoryDatabase", "false")
	//no need to pass blockstore into history executore, it won't be used in this test
	qhistory, err := env.testHistoryDB.NewHistoryQueryExecutor(nil)
	assert.NoError(t, err, "Error upon NewHistoryQueryExecutor")
	_, err2 := qhistory.GetHistoryForKey("ns1", "key7")
	assert.Error(t, err2, "Error should have been returned for GetHistoryForKey() when history disabled")
}

//TestGenesisBlockNoError tests that Genesis blocks are ignored by history processing
// since we only persist history of chaincode key writes
func TestGenesisBlockNoError(t *testing.T) {
	env := newTestHistoryEnv(t)
	defer env.cleanup()
	block, err := configtxtest.MakeGenesisBlock("test_chainid")
	assert.NoError(t, err)
	err = env.testHistoryDB.Commit(block)
	assert.NoError(t, err)
}

// TestHistoryWithKeyContainingNilBytes tests historydb when keys contains nil bytes (FAB-11244) -
// which happens to be used as a separator in the composite keys that is formed for the entries in the historydb
func TestHistoryWithKeyContainingNilBytes(t *testing.T) {
	env := newTestHistoryEnv(t)
	defer env.cleanup()
	provider := env.testBlockStorageEnv.provider
	ledger1id := "ledger1"
	store1, err := provider.OpenBlockStore(ledger1id)
	assert.NoError(t, err, "Error upon provider.OpenBlockStore()")
	defer store1.Shutdown()

	bg, gb := testutil.NewBlockGenerator(t, ledger1id, false)
	assert.NoError(t, store1.AddBlock(gb))
	assert.NoError(t, env.testHistoryDB.Commit(gb))

	//block1
	txid := util2.GenerateUUID()
	simulator, _ := env.txmgr.NewTxSimulator(txid)
	simulator.SetState("ns1", "key", []byte("value1")) // add a key <key> that contains no nil byte
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimResBytes, _ := simRes.GetPubSimulationBytes()
	block1 := bg.NextBlock([][]byte{pubSimResBytes})
	err = store1.AddBlock(block1)
	assert.NoError(t, err)
	err = env.testHistoryDB.Commit(block1)
	assert.NoError(t, err)

	//block2 tran1
	simulationResults := [][]byte{}
	txid = util2.GenerateUUID()
	simulator, _ = env.txmgr.NewTxSimulator(txid)
	simulator.SetState("ns1", "key", []byte("value2")) // add another value for the key <key>
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	pubSimResBytes, _ = simRes.GetPubSimulationBytes()
	simulationResults = append(simulationResults, pubSimResBytes)

	//block2 tran2
	txid2 := util2.GenerateUUID()
	simulator2, _ := env.txmgr.NewTxSimulator(txid2)
	// add another key <key\x00\x01\x01\x15> that contains a nil byte - such that when a range query is formed using old format, this key falls in the range
	simulator2.SetState("ns1", "key\x00\x01\x01\x15", []byte("dummyVal1"))
	simulator2.SetState("ns1", "\x00key\x00\x01\x01\x15", []byte("dummyVal2"))
	simulator2.Done()
	simRes2, _ := simulator2.GetTxSimulationResults()
	pubSimResBytes2, _ := simRes2.GetPubSimulationBytes()
	simulationResults = append(simulationResults, pubSimResBytes2)
	block2 := bg.NextBlock(simulationResults)
	err = store1.AddBlock(block2)
	assert.NoError(t, err)
	err = env.testHistoryDB.Commit(block2)
	assert.NoError(t, err)

	qhistory, err := env.testHistoryDB.NewHistoryQueryExecutor(store1)
	assert.NoError(t, err, "Error upon NewHistoryQueryExecutor")
	testutilVerifyResults(t, qhistory, "ns1", "key", []string{"value1", "value2"})
	testutilVerifyResults(t, qhistory, "ns1", "key\x00\x01\x01\x15", []string{"dummyVal1"})
	testutilVerifyResults(t, qhistory, "ns1", "\x00key\x00\x01\x01\x15", []string{"dummyVal2"})
}

func testutilVerifyResults(t *testing.T, hqe ledger.HistoryQueryExecutor, ns, key string, expectedVals []string) {
	itr, err := hqe.GetHistoryForKey(ns, key)
	assert.NoError(t, err, "Error upon GetHistoryForKey()")
	retrievedVals := []string{}
	for {
		kmod, _ := itr.Next()
		if kmod == nil {
			break
		}
		txid := kmod.(*queryresult.KeyModification).TxId
		retrievedValue := string(kmod.(*queryresult.KeyModification).Value)
		retrievedVals = append(retrievedVals, retrievedValue)
		t.Logf("Retrieved history record at TxId=%s with value %s", txid, retrievedValue)
	}
	assert.Equal(t, expectedVals, retrievedVals)
}
