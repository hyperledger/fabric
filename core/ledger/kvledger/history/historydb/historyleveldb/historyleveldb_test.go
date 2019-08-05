/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package historyleveldb

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"testing"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb/historyleveldb/fakes"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger/history/historydb/historyleveldb")
	flogging.ActivateSpec("leveldbhelper,historyleveldb=debug")
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

	qhistory, err := env.testHistoryDB.NewHistoryQueryExecutor(store1)
	assert.NoError(t, err, "Error upon NewHistoryQueryExecutor")
	testutilVerifyResults(t, qhistory, "ns1", "key", []string{"value1", "value2"})
	testutilVerifyResults(t, qhistory, "ns1", key1, []string{"dummyVal1"})
	testutilVerifyResults(t, qhistory, "ns1", key2, []string{"dummyVal2"})
	testutilVerifyResults(t, qhistory, "ns1", key3, []string{"dummyVal3"})
	testutilVerifyResults(t, qhistory, "ns1", key4, []string{"dummyVal4"})
	testutilVerifyResults(t, qhistory, "ns1", key5, []string{"dummyVal5"})

	// verify key2-key5 fall in range but key1 is not in range
	testutilCheckKeyInRange(t, qhistory, "ns1", "key", key1, 0)
	testutilCheckKeyInRange(t, qhistory, "ns1", "key", key2, 1)
	testutilCheckKeyInRange(t, qhistory, "ns1", "key", key3, 1)
	testutilCheckKeyInRange(t, qhistory, "ns1", "key", key4, 1)
	testutilCheckKeyInRange(t, qhistory, "ns1", "key", key5, 1)

	// temporarily replace logger with a fake logger so that we can verify Warnf message call count and call args
	origLogger := logger
	fakeLogger := &fakes.HistoryleveldbLogger{HistorydbLogger: &fakes.HistorydbLogger{}}
	logger = fakeLogger
	defer func() {
		logger = origLogger
	}()
	exhaustQueryResultsForKey(t, qhistory, "ns1", "key")
	assert.Equalf(t, 4, fakeLogger.WarnfCallCount(), "Get expected number of warning messages")
	testutilVerifyWarningMsg(t, fakeLogger, "ns1", key2,
		"Some other key [%#v] found in the range while scanning history for key [%#v]. Skipping (decoding error: %s)",
		"decoded size from DecodeVarint is invalid, expected <=8, but got 21")
	testutilVerifyWarningMsg(t, fakeLogger, "ns1", key3,
		"Some other key [%#v] found in the range while scanning history for key [%#v]. Skipping (decoding error: %s)",
		"decoded size from DecodeVarint is invalid, expected <=8, but got 12")
	testutilVerifyWarningMsg(t, fakeLogger, "ns1", key4,
		"Some other key [%#v] found in the range while scanning history for key [%#v]. Skipping (decoding error: %s)",
		"number of decoded bytes (4) is not equal to the length of blockNumTranNumBytes (6)")
	testutilVerifyWarningMsg(t, fakeLogger, "ns1", key5,
		"Some other clashing key [%#v] found in the range while scanning history for key [%#v]. Skipping (cannot find block:tx)",
		"")
}

// TestNoKeyFoundForFalseKey creates a false key that maps to block 257: tran 0.
// The test creates block 257, but the false key is skipped because the key is not found in block 257.
func TestNoKeyFoundForFalseKey(t *testing.T) {
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

	// block1 is used to test a false key that is decoded to valid blockNum(257):tranNum(0),
	// but it is skipped because "key" is not found in block(257):tran(0).
	// In this case, otherKey's history key is <ns, key\x00\x04\x00, 1, 0> that would be decoded
	// to the same blockNum:tranNum as history key <ns, key, 257, 0>.
	// blockNumTranNumBytes are 0x4, 0x0, 0x0 (separator), 0x1, 0x1, 0x0 when getting history for "key".
	otherKey := "key\x00\x04\x00"
	txid := util2.GenerateUUID()
	simulator, _ := env.txmgr.NewTxSimulator(txid)
	simulator.SetState("ns1", otherKey, []byte("otherValue"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimResBytes, _ := simRes.GetPubSimulationBytes()
	block1 := bg.NextBlock([][]byte{pubSimResBytes})
	err = store1.AddBlock(block1)
	assert.NoError(t, err)
	err = env.testHistoryDB.Commit(block1)
	assert.NoError(t, err)

	// add blocks 2-256, each block has 1 transaction setting state for "ns1" and "key", value is "value<blockNum>"
	for i := 2; i <= 256; i++ {
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

	// add block 257 with a different key so that otherKey cannot find "key" in this block(257):tran(0)
	txid = util2.GenerateUUID()
	simulator, _ = env.txmgr.NewTxSimulator(txid)
	simulator.SetState("ns1", "key2", []byte("key2Value"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	pubSimResBytes, _ = simRes.GetPubSimulationBytes()
	block257 := bg.NextBlock([][]byte{pubSimResBytes})
	err = store1.AddBlock(block257)
	assert.NoError(t, err)
	err = env.testHistoryDB.Commit(block257)
	assert.NoError(t, err)

	// query history db for "ns1", "key"
	qhistory, err := env.testHistoryDB.NewHistoryQueryExecutor(store1)
	assert.NoError(t, err, "Error upon NewHistoryQueryExecutor")

	// temporarily replace logger with a fake logger so that we can verify Warnf call count and call args
	origLogger := logger
	fakeLogger := &fakes.HistoryleveldbLogger{HistorydbLogger: &fakes.HistorydbLogger{}}
	logger = fakeLogger
	defer func() {
		logger = origLogger
	}()
	exhaustQueryResultsForKey(t, qhistory, "ns1", "key")
	assert.Equalf(t, 1, fakeLogger.WarnfCallCount(), "Get expected number of warning messages")
	msg, _ := fakeLogger.WarnfArgsForCall(0)
	// key "ns, key\x00\x04\x00, 1, 0" is returned as a false key in range query for "key".
	// Its "x04\x00, 1, 0" portion happens to be decoded to a valid blockNum:tranNum (257:0)
	// but the desired ns/key is not present in the blockNum:tranNum.
	assert.Equal(t, "Some other key [%#v] found in the range while scanning history for key [%#v]. Skipping (namespace or key not found)", msg)
}

// TestHistoryWithBlockNumber256 creates 256 blocks and then
// search historydb to verify that block number 256 is correctly returned
// even if its blockNumTranNumBytes contains a nil byte (FAB-15450).
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
	qhistory, err := env.testHistoryDB.NewHistoryQueryExecutor(store1)
	assert.NoError(t, err, "Error upon NewHistoryQueryExecutor")
	itr, err := qhistory.GetHistoryForKey("ns1", "key")
	assert.NoError(t, err, "Error upon GetHistoryForKey()")

	// iterate query result - there should be 256 entries
	numEntries := 0
	valueInBlock256 := "unknown"
	for {
		kmod, _ := itr.Next()
		if kmod == nil {
			break
		}
		numEntries++
		retrievedValue := string(kmod.(*queryresult.KeyModification).Value)
		if numEntries == 256 {
			valueInBlock256 = retrievedValue
		}
	}
	assert.Equal(t, 256, numEntries)
	assert.Equal(t, "value256", valueInBlock256)
}

func TestName(t *testing.T) {
	env := newTestHistoryEnv(t)
	defer env.cleanup()
	assert.Equal(t, "history", env.testHistoryDB.Name())
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

// testutilCheckKeyInRange check if falseKey falls in range query when searching for desiredKey
func testutilCheckKeyInRange(t *testing.T, hqe ledger.HistoryQueryExecutor, ns, desiredKey, falseKey string, expectedMatchCount int) {
	itr, err := hqe.GetHistoryForKey(ns, desiredKey)
	assert.NoError(t, err, "Error upon GetHistoryForKey()")
	scanner := itr.(*historyScanner)
	compositePartialKey := historydb.ConstructPartialCompositeHistoryKey(ns, falseKey, false)
	count := 0
	for {
		if !scanner.dbItr.Next() {
			break
		}
		historyKey := scanner.dbItr.Key()
		if bytes.Contains(historyKey, compositePartialKey) {
			count++
		}
	}
	assert.Equal(t, expectedMatchCount, count)
}

// exhaustQueryResultsForKey is a helper method to exhaust all the results returned by history query.
// A fake logger can be set before calling this method to verify if a false key is skipped.
func exhaustQueryResultsForKey(t *testing.T, hqe ledger.HistoryQueryExecutor, ns, key string) {
	itr, err := hqe.GetHistoryForKey(ns, key)
	assert.NoError(t, err, "Error upon GetHistoryForKey()")
	for {
		kmod, _ := itr.Next()
		if kmod == nil {
			break
		}
	}
}

// testutilVerifyWarningMsg verifies fakeLogger had a Warnf call for the falseKey.
// It iterates fakeLogger.WarnfCallCount() to find a history key that matches compositePartialKey
// and then verifies warnMsg and decodeErrMsg (if not empty).
func testutilVerifyWarningMsg(t *testing.T, fakeLogger *fakes.HistoryleveldbLogger, ns, falseKey, warnMsg, decodeErrMsg string) {
	// because there is no particular order, iterate each call to find the matched one
	compositePartialKey := historydb.ConstructPartialCompositeHistoryKey(ns, falseKey, false)
	for i := 0; i < fakeLogger.WarnfCallCount(); i++ {
		msg, args := fakeLogger.WarnfArgsForCall(i)

		if !bytes.Contains(args[0].([]byte), compositePartialKey) {
			// not matched
			continue
		}

		assert.Equal(t, warnMsg, msg)

		if decodeErrMsg == "" {
			assert.Equal(t, 2, len(args))
		} else {
			assert.Equal(t, 3, len(args))
			assert.Equal(t, decodeErrMsg, fmt.Sprintf("%s", args[2]))
		}
		return
	}
	assert.Fail(t, fmt.Sprintf("did not find false key %s in range", falseKey))
}
