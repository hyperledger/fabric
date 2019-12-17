/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package history

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/peer"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("leveldbhelper,history=debug")
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
	assert.Equal(t, "history", env.testHistoryDB.Name())

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

	qhistory, err := env.testHistoryDB.NewQueryExecutor(store1)
	assert.NoError(t, err, "Error upon NewQueryExecutor")

	itr, err2 := qhistory.GetHistoryForKey("ns1", "key7")
	assert.NoError(t, err2, "Error upon GetHistoryForKey()")

	count := 0
	for {
		// iterator will return entries in the order of newest to oldest
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
		if count != 1 {
			// entries 2, 3, 4 are block2:tran2, block2:tran1 and block1:tran1
			expectedValue := []byte("value" + strconv.Itoa(5-count))
			assert.Equal(t, expectedValue, retrievedValue)
			assert.NotNil(t, retrievedTimestamp)
			assert.False(t, retrievedIsDelete)
		} else {
			// entry 1 is block3:tran1
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

	qhistory, err := env.testHistoryDB.NewQueryExecutor(store1)
	assert.NoError(t, err, "Error upon NewQueryExecutor")

	itr, err2 := qhistory.GetHistoryForKey("ns1", "key7")
	assert.NoError(t, err2, "Error upon GetHistoryForKey()")

	// test that there are no history values, since the tran was marked as invalid
	kmod, _ := itr.Next()
	assert.Nil(t, kmod)
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

	// key1 should not fall in the range
	key1 := "\x00key\x00\x01\x01\x15"
	simulator2.SetState("ns1", key1, []byte("dummyVal1"))

	// add other keys that contain nil byte(s) - such that when a range query is formed, these keys fall in the range
	// key2 is skipped due to tran num decoding error (decode size 21 > 8)
	// blockNumTranNumBytes are 0x1, 0x1, 0x15, 0x0 (separator), 0x1, 0x2, 0x1, 0x1
	key2 := "key\x00\x01\x01\x15" // \x15 is 21
	simulator2.SetState("ns1", key2, []byte("dummyVal2"))

	// key3 is skipped due to block num decoding error (decoded size 12 > 8)
	// blockNumTranNumBytes are 0xc, 0x0 (separtor), 0x1, 0x2, 0x1, 0x1
	key3 := "key\x00\x0c" // \x0c is 12
	simulator2.SetState("ns1", key3, []byte("dummyVal3"))

	// key4 is skipped because blockBytesConsumed (2) + tranBytesConsumed (2) != len(blockNumTranNum) (6)
	// blockNumTranNumBytes are 0x1, 0x0 (separator), 0x1, 0x2, 0x1, 0x1
	key4 := "key\x00\x01"
	simulator2.SetState("ns1", key4, []byte("dummyVal4"))

	// key5 is skipped due to ErrNotFoundInIndex, where history key is <ns, key\x00\x04\x01, 2, 1>, same as <ns, key, 16777474, 1>.
	// blockNumTranNumBytes are 0x4, 0x1, 0x0 (separator), 0x1, 0x2, 0x1, 0x1
	key5 := "key\x00\x04\x01"
	simulator2.SetState("ns1", key5, []byte("dummyVal5"))

	// commit block2
	simulator2.Done()
	simRes2, _ := simulator2.GetTxSimulationResults()
	pubSimResBytes2, _ := simRes2.GetPubSimulationBytes()
	simulationResults = append(simulationResults, pubSimResBytes2)
	block2 := bg.NextBlock(simulationResults)
	err = store1.AddBlock(block2)
	assert.NoError(t, err)
	err = env.testHistoryDB.Commit(block2)
	assert.NoError(t, err)

	qhistory, err := env.testHistoryDB.NewQueryExecutor(store1)
	assert.NoError(t, err, "Error upon NewQueryExecutor")

	// verify the results for each key, in the order of newest to oldest
	testutilVerifyResults(t, qhistory, "ns1", "key", []string{"value2", "value1"})
	testutilVerifyResults(t, qhistory, "ns1", key1, []string{"dummyVal1"})
	testutilVerifyResults(t, qhistory, "ns1", key2, []string{"dummyVal2"})
	testutilVerifyResults(t, qhistory, "ns1", key3, []string{"dummyVal3"})
	testutilVerifyResults(t, qhistory, "ns1", key4, []string{"dummyVal4"})
	testutilVerifyResults(t, qhistory, "ns1", key5, []string{"dummyVal5"})

	// verify none of key1-key5 falls in the range of history query for "key"
	testutilCheckKeyNotInRange(t, qhistory, "ns1", "key", key1)
	testutilCheckKeyNotInRange(t, qhistory, "ns1", "key", key2)
	testutilCheckKeyNotInRange(t, qhistory, "ns1", "key", key3)
	testutilCheckKeyNotInRange(t, qhistory, "ns1", "key", key4)
	testutilCheckKeyNotInRange(t, qhistory, "ns1", "key", key5)
}

// TestHistoryWithBlockNumber256 creates 256 blocks and then
// queries historydb to verify that all 256 blocks are returned in the right orderer.
// This test also verifies that block256 is returned correctly even if its key contains nil byte.
func TestHistoryWithBlockNumber256(t *testing.T) {
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

	// add 256 blocks, each block has 1 transaction setting state for "ns1" and "key", value is "value<blockNum>"
	for i := 1; i <= 256; i++ {
		txid := util2.GenerateUUID()
		simulator, _ := env.txmgr.NewTxSimulator(txid)
		value := fmt.Sprintf("value%d", i)
		simulator.SetState("ns1", "key", []byte(value))
		simulator.Done()
		simRes, _ := simulator.GetTxSimulationResults()
		pubSimResBytes, _ := simRes.GetPubSimulationBytes()
		block := bg.NextBlock([][]byte{pubSimResBytes})
		err = store1.AddBlock(block)
		assert.NoError(t, err)
		err = env.testHistoryDB.Commit(block)
		assert.NoError(t, err)
	}

	// query history db for "ns1", "key"
	qhistory, err := env.testHistoryDB.NewQueryExecutor(store1)
	assert.NoError(t, err, "Error upon NewQueryExecutor")

	// verify history query returns the expected results in the orderer of block256, 255, 254 .... 1.
	expectedHistoryResults := make([]string, 0)
	for i := 256; i >= 1; i-- {
		expectedHistoryResults = append(expectedHistoryResults, fmt.Sprintf("value%d", i))
	}
	testutilVerifyResults(t, qhistory, "ns1", "key", expectedHistoryResults)
}

func TestName(t *testing.T) {
	env := newTestHistoryEnv(t)
	defer env.cleanup()
	assert.Equal(t, "history", env.testHistoryDB.Name())
}

// verify history results
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

// testutilCheckKeyNotInRange verifies that a (false) key is not returned in range query when searching for the desired key
func testutilCheckKeyNotInRange(t *testing.T, hqe ledger.HistoryQueryExecutor, ns, desiredKey, falseKey string) {
	itr, err := hqe.GetHistoryForKey(ns, desiredKey)
	assert.NoError(t, err, "Error upon GetHistoryForKey()")
	scanner := itr.(*historyScanner)
	rangeScanKeys := constructRangeScan(ns, falseKey)
	for {
		if !scanner.dbItr.Next() {
			break
		}
		historyKey := scanner.dbItr.Key()
		if bytes.Contains(historyKey, rangeScanKeys.startKey) {
			assert.Failf(t, "false key %s should not be returned in range query for key %s", falseKey, desiredKey)
		}
	}
}
