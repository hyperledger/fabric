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
	"testing"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
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

	// read the savepoint, it should not exist and should return 0
	blockNum, err := env.TestHistoryDB.GetBlockNumFromSavepoint()
	testutil.AssertNoError(t, err, "Error upon historyDatabase.GetBlockNumFromSavepoint()")
	testutil.AssertEquals(t, blockNum, uint64(0))

	// create a block
	simulator, _ := env.Txmgr.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	bg := testutil.NewBlockGenerator(t)
	block1 := bg.NextBlock([][]byte{simRes}, false)
	err = env.TestHistoryDB.Commit(block1)
	testutil.AssertNoError(t, err, "")

	// read the savepoint, it should now exist and return 1
	blockNum, err = env.TestHistoryDB.GetBlockNumFromSavepoint()
	testutil.AssertNoError(t, err, "Error upon historyDatabase.GetBlockNumFromSavepoint()")
	testutil.AssertEquals(t, blockNum, uint64(1))
}

func TestHistory(t *testing.T) {

	env := NewTestHistoryEnv(t)
	defer env.cleanup()

	//block1
	simulator, _ := env.Txmgr.NewTxSimulator()
	simulator.SetState("ns1", "key7", []byte("value1"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	bg := testutil.NewBlockGenerator(t)
	block1 := bg.NextBlock([][]byte{simRes}, false)
	err := env.TestHistoryDB.Commit(block1)
	testutil.AssertNoError(t, err, "")

	//block2 tran1
	simulationResults := [][]byte{}
	simulator, _ = env.Txmgr.NewTxSimulator()
	simulator.SetState("ns1", "key7", []byte("value2"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	simulationResults = append(simulationResults, simRes)
	//block2 tran2
	simulator2, _ := env.Txmgr.NewTxSimulator()
	simulator2.SetState("ns1", "key7", []byte("value3"))
	simulator2.Done()
	simRes2, _ := simulator2.GetTxSimulationResults()
	simulationResults = append(simulationResults, simRes2)
	block2 := bg.NextBlock(simulationResults, false)
	err = env.TestHistoryDB.Commit(block2)
	testutil.AssertNoError(t, err, "")

	qhistory, err := env.TestHistoryDB.NewHistoryQueryExecutor()
	testutil.AssertNoError(t, err, "Error upon NewHistoryQueryExecutor")

	itr, err2 := qhistory.GetHistoryForKey("ns1", "key7")
	testutil.AssertNoError(t, err2, "Error upon GetHistoryForKey()")

	count := 0
	for {
		kmod, _ := itr.Next()
		if kmod == nil {
			break
		}
		txid := kmod.(*ledger.KeyModification).TxID
		//v := kmod.(*ledger.KeyModification).Value TODO value not populated yet
		t.Logf("Retrieved history record for key=key7 at TxId=%s", txid)
		count++
	}
	testutil.AssertEquals(t, count, 3)
	// TODO add assertions for exact history values once it is populated

}

//TestSavepoint tests that save points get written after each block and get returned via GetBlockNumfromSavepoint
func TestHistoryDisabled(t *testing.T) {

	env := NewTestHistoryEnv(t)
	defer env.cleanup()

	viper.Set("ledger.state.historyDatabase", "false")

	qhistory, err := env.TestHistoryDB.NewHistoryQueryExecutor()
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

	err = env.TestHistoryDB.Commit(block)
	testutil.AssertNoError(t, err, "")
}
