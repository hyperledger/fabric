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

package historyleveldb

import (
	"os"
	"strconv"
	"testing"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger/history/historydb/historyleveldb")
	os.Exit(m.Run())
}

//TestSavepoint tests that save points get written after each block and get returned via GetBlockNumfromSavepoint
func TestSavepoint(t *testing.T) {

	env := NewTestHistoryEnv(t)
	defer env.cleanup()

	// read the savepoint, it should not exist and should return nil Height object
	savepoint, err := env.testHistoryDB.GetLastSavepoint()
	testutil.AssertNoError(t, err, "Error upon historyDatabase.GetLastSavepoint()")
	testutil.AssertNil(t, savepoint)

	// ShouldRecover should return true when no savepoint is found and recovery from block 0
	status, blockNum, err := env.testHistoryDB.ShouldRecover(0)
	testutil.AssertNoError(t, err, "Error upon historyDatabase.ShouldRecover()")
	testutil.AssertEquals(t, status, true)
	testutil.AssertEquals(t, blockNum, uint64(0))

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	testutil.AssertNoError(t, env.testHistoryDB.Commit(gb), "")
	// read the savepoint, it should now exist and return a Height object with BlockNum 0
	savepoint, err = env.testHistoryDB.GetLastSavepoint()
	testutil.AssertNoError(t, err, "Error upon historyDatabase.GetLastSavepoint()")
	testutil.AssertEquals(t, savepoint.BlockNum, uint64(0))

	// create the next block (block 1)
	simulator, _ := env.txmgr.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	block1 := bg.NextBlock([][]byte{simRes})
	testutil.AssertNoError(t, env.testHistoryDB.Commit(block1), "")
	savepoint, err = env.testHistoryDB.GetLastSavepoint()
	testutil.AssertNoError(t, err, "Error upon historyDatabase.GetLastSavepoint()")
	testutil.AssertEquals(t, savepoint.BlockNum, uint64(1))

	// Should Recover should return false
	status, blockNum, err = env.testHistoryDB.ShouldRecover(1)
	testutil.AssertNoError(t, err, "Error upon historyDatabase.ShouldRecover()")
	testutil.AssertEquals(t, status, false)
	testutil.AssertEquals(t, blockNum, uint64(2))

	// create the next block (block 2)
	simulator, _ = env.txmgr.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value2"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	block2 := bg.NextBlock([][]byte{simRes})

	// assume that the peer failed to commit this block to historyDB and is being recovered now
	env.testHistoryDB.CommitLostBlock(block2)
	savepoint, err = env.testHistoryDB.GetLastSavepoint()
	testutil.AssertNoError(t, err, "Error upon historyDatabase.GetLastSavepoint()")
	testutil.AssertEquals(t, savepoint.BlockNum, uint64(2))

	//Pass high blockNum, ShouldRecover should return true with 3 as blocknum to recover from
	status, blockNum, err = env.testHistoryDB.ShouldRecover(10)
	testutil.AssertNoError(t, err, "Error upon historyDatabase.ShouldRecover()")
	testutil.AssertEquals(t, status, true)
	testutil.AssertEquals(t, blockNum, uint64(3))
}

func TestHistory(t *testing.T) {

	env := NewTestHistoryEnv(t)
	defer env.cleanup()
	provider := env.testBlockStorageEnv.provider
	ledger1id := "ledger1"
	store1, err := provider.OpenBlockStore(ledger1id)
	testutil.AssertNoError(t, err, "Error upon provider.OpenBlockStore()")
	defer store1.Shutdown()

	bg, gb := testutil.NewBlockGenerator(t, ledger1id, false)
	testutil.AssertNoError(t, store1.AddBlock(gb), "")
	testutil.AssertNoError(t, env.testHistoryDB.Commit(gb), "")

	//block1
	simulator, _ := env.txmgr.NewTxSimulator()
	value1 := []byte("value1")
	simulator.SetState("ns1", "key7", value1)
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	block1 := bg.NextBlock([][]byte{simRes})
	err = store1.AddBlock(block1)
	testutil.AssertNoError(t, err, "")
	err = env.testHistoryDB.Commit(block1)
	testutil.AssertNoError(t, err, "")

	//block2 tran1
	simulationResults := [][]byte{}
	simulator, _ = env.txmgr.NewTxSimulator()
	value2 := []byte("value2")
	simulator.SetState("ns1", "key7", value2)
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	simulationResults = append(simulationResults, simRes)
	//block2 tran2
	simulator2, _ := env.txmgr.NewTxSimulator()
	value3 := []byte("value3")
	simulator2.SetState("ns1", "key7", value3)
	simulator2.Done()
	simRes2, _ := simulator2.GetTxSimulationResults()
	simulationResults = append(simulationResults, simRes2)
	block2 := bg.NextBlock(simulationResults)
	err = store1.AddBlock(block2)
	testutil.AssertNoError(t, err, "")
	err = env.testHistoryDB.Commit(block2)
	testutil.AssertNoError(t, err, "")

	//block3
	simulator, _ = env.txmgr.NewTxSimulator()
	simulator.DeleteState("ns1", "key7")
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	block3 := bg.NextBlock([][]byte{simRes})
	err = store1.AddBlock(block3)
	testutil.AssertNoError(t, err, "")
	err = env.testHistoryDB.Commit(block3)
	testutil.AssertNoError(t, err, "")
	t.Logf("Inserted all 3 blocks")

	qhistory, err := env.testHistoryDB.NewHistoryQueryExecutor(store1)
	testutil.AssertNoError(t, err, "Error upon NewHistoryQueryExecutor")

	itr, err2 := qhistory.GetHistoryForKey("ns1", "key7")
	testutil.AssertNoError(t, err2, "Error upon GetHistoryForKey()")

	count := 0
	for {
		kmod, _ := itr.Next()
		if kmod == nil {
			break
		}
		txid := kmod.(*queryresult.KeyModification).TxId
		retrievedValue := kmod.(*queryresult.KeyModification).Value
		retrievedTimestamp := kmod.(*queryresult.KeyModification).Timestamp
		retrievedIsDelete := kmod.(*queryresult.KeyModification).IsDelete
		t.Logf("Retrieved history record for key=key7 at TxId=%s with value %v and timestamp %v",
			txid, retrievedValue, retrievedTimestamp)
		count++
		if count != 4 {
			expectedValue := []byte("value" + strconv.Itoa(count))
			testutil.AssertEquals(t, retrievedValue, expectedValue)
			testutil.AssertNotEquals(t, retrievedTimestamp, nil)
			testutil.AssertEquals(t, retrievedIsDelete, false)
		} else {
			testutil.AssertEquals(t, retrievedValue, nil)
			testutil.AssertNotEquals(t, retrievedTimestamp, nil)
			testutil.AssertEquals(t, retrievedIsDelete, true)
		}
	}
	testutil.AssertEquals(t, count, 4)
}

func TestHistoryForInvalidTran(t *testing.T) {

	env := NewTestHistoryEnv(t)
	defer env.cleanup()
	provider := env.testBlockStorageEnv.provider
	ledger1id := "ledger1"
	store1, err := provider.OpenBlockStore(ledger1id)
	testutil.AssertNoError(t, err, "Error upon provider.OpenBlockStore()")
	defer store1.Shutdown()

	bg, gb := testutil.NewBlockGenerator(t, ledger1id, false)
	testutil.AssertNoError(t, store1.AddBlock(gb), "")
	testutil.AssertNoError(t, env.testHistoryDB.Commit(gb), "")

	//block1
	simulator, _ := env.txmgr.NewTxSimulator()
	value1 := []byte("value1")
	simulator.SetState("ns1", "key7", value1)
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	block1 := bg.NextBlock([][]byte{simRes})

	//for this invalid tran test, set the transaction to invalid
	txsFilter := util.TxValidationFlags(block1.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	txsFilter.SetFlag(0, peer.TxValidationCode_INVALID_OTHER_REASON)
	block1.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter

	err = store1.AddBlock(block1)
	testutil.AssertNoError(t, err, "")
	err = env.testHistoryDB.Commit(block1)
	testutil.AssertNoError(t, err, "")

	qhistory, err := env.testHistoryDB.NewHistoryQueryExecutor(store1)
	testutil.AssertNoError(t, err, "Error upon NewHistoryQueryExecutor")

	itr, err2 := qhistory.GetHistoryForKey("ns1", "key7")
	testutil.AssertNoError(t, err2, "Error upon GetHistoryForKey()")

	// test that there are no history values, since the tran was marked as invalid
	kmod, _ := itr.Next()
	testutil.AssertNil(t, kmod)
}

//TestSavepoint tests that save points get written after each block and get returned via GetBlockNumfromSavepoint
func TestHistoryDisabled(t *testing.T) {

	env := NewTestHistoryEnv(t)
	defer env.cleanup()

	viper.Set("ledger.history.enableHistoryDatabase", "false")

	//no need to pass blockstore into history executore, it won't be used in this test
	qhistory, err := env.testHistoryDB.NewHistoryQueryExecutor(nil)
	testutil.AssertNoError(t, err, "Error upon NewHistoryQueryExecutor")

	_, err2 := qhistory.GetHistoryForKey("ns1", "key7")
	testutil.AssertError(t, err2, "Error should have been returned for GetHistoryForKey() when history disabled")
}

//TestGenesisBlockNoError tests that Genesis blocks are ignored by history processing
// since we only persist history of chaincode key writes
func TestGenesisBlockNoError(t *testing.T) {

	env := NewTestHistoryEnv(t)
	defer env.cleanup()

	block, err := configtxtest.MakeGenesisBlock("test_chainid")
	testutil.AssertNoError(t, err, "")

	err = env.testHistoryDB.Commit(block)
	testutil.AssertNoError(t, err, "")
}
