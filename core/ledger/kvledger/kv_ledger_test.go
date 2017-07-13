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

package kvledger

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestKVLedgerBlockStorage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	provider, _ := NewProvider()
	defer provider.Close()

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := gb.Header.Hash()
	ledger, _ := provider.Create(gb)
	defer ledger.Close()

	bcInfo, _ := ledger.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil})

	simulator, _ := ledger.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	block1 := bg.NextBlock([][]byte{simRes})
	ledger.Commit(block1)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := block1.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 2, CurrentBlockHash: block1Hash, PreviousBlockHash: gbHash})

	simulator, _ = ledger.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value4"))
	simulator.SetState("ns1", "key2", []byte("value5"))
	simulator.SetState("ns1", "key3", []byte("value6"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	block2 := bg.NextBlock([][]byte{simRes})
	ledger.Commit(block2)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := block2.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 3, CurrentBlockHash: block2Hash, PreviousBlockHash: block1Hash})

	b0, _ := ledger.GetBlockByHash(gbHash)
	testutil.AssertEquals(t, b0, gb)

	b1, _ := ledger.GetBlockByHash(block1Hash)
	testutil.AssertEquals(t, b1, block1)

	b0, _ = ledger.GetBlockByNumber(0)
	testutil.AssertEquals(t, b0, gb)

	b1, _ = ledger.GetBlockByNumber(1)
	testutil.AssertEquals(t, b1, block1)

	// get the tran id from the 2nd block, then use it to test GetTransactionByID()
	txEnvBytes2 := block1.Data.Data[0]
	txEnv2, err := putils.GetEnvelopeFromBlock(txEnvBytes2)
	testutil.AssertNoError(t, err, "Error upon GetEnvelopeFromBlock")
	payload2, err := putils.GetPayload(txEnv2)
	testutil.AssertNoError(t, err, "Error upon GetPayload")
	chdr, err := putils.UnmarshalChannelHeader(payload2.Header.ChannelHeader)
	testutil.AssertNoError(t, err, "Error upon GetChannelHeaderFromBytes")
	txID2 := chdr.TxId
	processedTran2, err := ledger.GetTransactionByID(txID2)
	testutil.AssertNoError(t, err, "Error upon GetTransactionByID")
	// get the tran envelope from the retrieved ProcessedTransaction
	retrievedTxEnv2 := processedTran2.TransactionEnvelope
	testutil.AssertEquals(t, retrievedTxEnv2, txEnv2)

	//  get the tran id from the 2nd block, then use it to test GetBlockByTxID
	b1, _ = ledger.GetBlockByTxID(txID2)
	testutil.AssertEquals(t, b1, block1)

	// get the transaction validation code for this transaction id
	validCode, _ := ledger.GetTxValidationCodeByTxID(txID2)
	testutil.AssertEquals(t, validCode, peer.TxValidationCode_VALID)

}

func TestKVLedgerDBRecovery(t *testing.T) {
	ledgertestutil.SetupCoreYAMLConfig()
	env := newTestEnv(t)
	defer env.cleanup()
	provider, _ := NewProvider()
	defer provider.Close()

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	ledger, _ := provider.Create(gb)
	defer ledger.Close()
	gbHash := gb.Header.Hash()
	bcInfo, err := ledger.GetBlockchainInfo()
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil})
	//creating and committing the first block
	simulator, _ := ledger.NewTxSimulator()
	//simulating a transaction
	simulator.SetState("ns1", "key1", []byte("value1.1"))
	simulator.SetState("ns1", "key2", []byte("value2.1"))
	simulator.SetState("ns1", "key3", []byte("value3.1"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	//generating a block based on the simulation result
	block1 := bg.NextBlock([][]byte{simRes})
	//performing validation of read and write set to find valid transactions
	ledger.Commit(block1)
	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := block1.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 2, CurrentBlockHash: block1Hash, PreviousBlockHash: gbHash})

	//======================================================================================
	//SCENARIO 1: peer fails before committing the second block to state DB
	//and history DB (if exist)
	//======================================================================================
	simulator, _ = ledger.NewTxSimulator()
	//simulating transaction
	simulator.SetState("ns1", "key1", []byte("value1.2"))
	simulator.SetState("ns1", "key2", []byte("value2.2"))
	simulator.SetState("ns1", "key3", []byte("value3.2"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	//generating a block based on the simulation result
	block2 := bg.NextBlock([][]byte{simRes})

	//performing validation of read and write set to find valid transactions
	ledger.(*kvLedger).txtmgmt.ValidateAndPrepare(block2, true)
	//writing the validated block to block storage but not committing the transaction
	//to state DB and history DB (if exist)
	err = ledger.(*kvLedger).blockStore.AddBlock(block2)

	//assume that peer fails here before committing the transaction
	assert.NoError(t, err)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := block2.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 3, CurrentBlockHash: block2Hash, PreviousBlockHash: block1Hash})

	simulator, _ = ledger.NewTxSimulator()
	value, _ := simulator.GetState("ns1", "key1")
	//value for 'key1' should be 'value1' as the last commit failed
	testutil.AssertEquals(t, value, []byte("value1.1"))
	value, _ = simulator.GetState("ns1", "key2")
	//value for 'key2' should be 'value2' as the last commit failed
	testutil.AssertEquals(t, value, []byte("value2.1"))
	value, _ = simulator.GetState("ns1", "key3")
	//value for 'key3' should be 'value3' as the last commit failed
	testutil.AssertEquals(t, value, []byte("value3.1"))
	//savepoint in state DB should 0 as the last commit failed
	stateDBSavepoint, _ := ledger.(*kvLedger).txtmgmt.GetLastSavepoint()
	testutil.AssertEquals(t, stateDBSavepoint.BlockNum, uint64(1))

	if ledgerconfig.IsHistoryDBEnabled() == true {
		qhistory, _ := ledger.NewHistoryQueryExecutor()
		itr, _ := qhistory.GetHistoryForKey("ns1", "key1")
		count := 0
		for {
			kmod, err := itr.Next()
			testutil.AssertNoError(t, err, "Error upon Next()")
			if kmod == nil {
				break
			}
			retrievedValue := kmod.(*queryresult.KeyModification).Value
			count++
			expectedValue := []byte("value1." + strconv.Itoa(count))
			testutil.AssertEquals(t, retrievedValue, expectedValue)
		}
		testutil.AssertEquals(t, count, 1)

		//savepoint in history DB should 0 as the last commit failed
		historyDBSavepoint, _ := ledger.(*kvLedger).historyDB.GetLastSavepoint()
		testutil.AssertEquals(t, historyDBSavepoint.BlockNum, uint64(1))
	}

	simulator.Done()
	ledger.Close()
	provider.Close()

	//we assume here that the peer comes online and calls NewKVLedger to get a handler for the ledger
	//State DB should be recovered before returning from NewKVLedger call
	provider, _ = NewProvider()
	ledger, _ = provider.Open("testLedger")

	simulator, _ = ledger.NewTxSimulator()
	value, _ = simulator.GetState("ns1", "key1")
	//value for 'key1' should be 'value4' after recovery
	testutil.AssertEquals(t, value, []byte("value1.2"))
	value, _ = simulator.GetState("ns1", "key2")
	//value for 'key2' should be 'value5' after recovery
	testutil.AssertEquals(t, value, []byte("value2.2"))
	value, _ = simulator.GetState("ns1", "key3")
	//value for 'key3' should be 'value6' after recovery
	testutil.AssertEquals(t, value, []byte("value3.2"))
	//savepoint in state DB should 2 after recovery
	stateDBSavepoint, _ = ledger.(*kvLedger).txtmgmt.GetLastSavepoint()
	testutil.AssertEquals(t, stateDBSavepoint.BlockNum, uint64(2))

	if ledgerconfig.IsHistoryDBEnabled() == true {
		qhistory, _ := ledger.NewHistoryQueryExecutor()
		itr, _ := qhistory.GetHistoryForKey("ns1", "key1")
		count := 0
		for {
			kmod, err := itr.Next()
			testutil.AssertNoError(t, err, "Error upon Next()")
			if kmod == nil {
				break
			}
			retrievedValue := kmod.(*queryresult.KeyModification).Value
			count++
			expectedValue := []byte("value1." + strconv.Itoa(count))
			testutil.AssertEquals(t, retrievedValue, expectedValue)
		}
		testutil.AssertEquals(t, count, 2)

		//savepoint in history DB should 2 after recovery
		historyDBSavepoint, _ := ledger.(*kvLedger).historyDB.GetLastSavepoint()
		testutil.AssertEquals(t, historyDBSavepoint.BlockNum, uint64(2))
	}

	simulator.Done()

	//======================================================================================
	//SCENARIO 2: peer fails after committing the third block to state DB
	//but before committing to history DB (if exist)
	//======================================================================================

	simulator, _ = ledger.NewTxSimulator()
	//simulating transaction
	simulator.SetState("ns1", "key1", []byte("value1.3"))
	simulator.SetState("ns1", "key2", []byte("value2.3"))
	simulator.SetState("ns1", "key3", []byte("value3.3"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	//generating a block based on the simulation result
	block3 := bg.NextBlock([][]byte{simRes})
	//performing validation of read and write set to find valid transactions
	ledger.(*kvLedger).txtmgmt.ValidateAndPrepare(block3, true)
	//writing the validated block to block storage
	err = ledger.(*kvLedger).blockStore.AddBlock(block3)
	//committing the transaction to state DB
	err = ledger.(*kvLedger).txtmgmt.Commit()
	//assume that peer fails here after committing the transaction to state DB but before
	//history DB
	assert.NoError(t, err)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block3Hash := block3.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 4, CurrentBlockHash: block3Hash, PreviousBlockHash: block2Hash})

	simulator, _ = ledger.NewTxSimulator()
	value, _ = simulator.GetState("ns1", "key1")
	//value for 'key1' should be 'value7'
	testutil.AssertEquals(t, value, []byte("value1.3"))
	value, _ = simulator.GetState("ns1", "key2")
	//value for 'key2' should be 'value8'
	testutil.AssertEquals(t, value, []byte("value2.3"))
	value, _ = simulator.GetState("ns1", "key3")
	//value for 'key3' should be 'value9'
	testutil.AssertEquals(t, value, []byte("value3.3"))
	//savepoint in state DB should 3
	stateDBSavepoint, _ = ledger.(*kvLedger).txtmgmt.GetLastSavepoint()
	testutil.AssertEquals(t, stateDBSavepoint.BlockNum, uint64(3))

	if ledgerconfig.IsHistoryDBEnabled() == true {
		qhistory, _ := ledger.NewHistoryQueryExecutor()
		itr, _ := qhistory.GetHistoryForKey("ns1", "key1")
		count := 0
		for {
			kmod, err := itr.Next()
			testutil.AssertNoError(t, err, "Error upon Next()")
			if kmod == nil {
				break
			}
			retrievedValue := kmod.(*queryresult.KeyModification).Value
			count++
			expectedValue := []byte("value1." + strconv.Itoa(count))
			testutil.AssertEquals(t, retrievedValue, expectedValue)
		}
		testutil.AssertEquals(t, count, 2)

		//savepoint in history DB should 2 as the last commit failed
		historyDBSavepoint, _ := ledger.(*kvLedger).historyDB.GetLastSavepoint()
		testutil.AssertEquals(t, historyDBSavepoint.BlockNum, uint64(2))
	}
	simulator.Done()
	ledger.Close()
	provider.Close()

	//we assume here that the peer comes online and calls NewKVLedger to get a handler for the ledger
	//history DB should be recovered before returning from NewKVLedger call
	provider, _ = NewProvider()
	ledger, _ = provider.Open("testLedger")
	simulator, _ = ledger.NewTxSimulator()
	stateDBSavepoint, _ = ledger.(*kvLedger).txtmgmt.GetLastSavepoint()
	testutil.AssertEquals(t, stateDBSavepoint.BlockNum, uint64(3))

	if ledgerconfig.IsHistoryDBEnabled() == true {
		qhistory, _ := ledger.NewHistoryQueryExecutor()
		itr, _ := qhistory.GetHistoryForKey("ns1", "key1")
		count := 0
		for {
			kmod, err := itr.Next()
			testutil.AssertNoError(t, err, "Error upon Next()")
			if kmod == nil {
				break
			}
			retrievedValue := kmod.(*queryresult.KeyModification).Value
			count++
			expectedValue := []byte("value1." + strconv.Itoa(count))
			testutil.AssertEquals(t, retrievedValue, expectedValue)
		}
		testutil.AssertEquals(t, count, 3)

		//savepoint in history DB should 3 after recovery
		historyDBSavepoint, _ := ledger.(*kvLedger).historyDB.GetLastSavepoint()
		testutil.AssertEquals(t, historyDBSavepoint.BlockNum, uint64(3))
	}
	simulator.Done()

	//Rare scenario

	//======================================================================================
	//SCENARIO 3: peer fails after committing the fourth block to history DB (if exist)
	//but before committing to state DB
	//======================================================================================
	simulator, _ = ledger.NewTxSimulator()
	//simulating transaction
	simulator.SetState("ns1", "key1", []byte("value1.4"))
	simulator.SetState("ns1", "key2", []byte("value2.4"))
	simulator.SetState("ns1", "key3", []byte("value3.4"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	//generating a block based on the simulation result
	block4 := bg.NextBlock([][]byte{simRes})
	//performing validation of read and write set to find valid transactions
	ledger.(*kvLedger).txtmgmt.ValidateAndPrepare(block4, true)
	//writing the validated block to block storage but fails to commit to state DB but
	//successfully commits to history DB (if exists)
	err = ledger.(*kvLedger).blockStore.AddBlock(block4)
	if ledgerconfig.IsHistoryDBEnabled() == true {
		err = ledger.(*kvLedger).historyDB.Commit(block4)
	}
	assert.NoError(t, err)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block4Hash := block4.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 5, CurrentBlockHash: block4Hash, PreviousBlockHash: block3Hash})

	simulator, _ = ledger.NewTxSimulator()
	value, _ = simulator.GetState("ns1", "key1")
	//value for 'key1' should be 'value7' as the last commit to State DB failed
	testutil.AssertEquals(t, value, []byte("value1.3"))
	value, _ = simulator.GetState("ns1", "key2")
	//value for 'key2' should be 'value8' as the last commit to State DB failed
	testutil.AssertEquals(t, value, []byte("value2.3"))
	value, _ = simulator.GetState("ns1", "key3")
	//value for 'key3' should be 'value9' as the last commit to State DB failed
	testutil.AssertEquals(t, value, []byte("value3.3"))
	//savepoint in state DB should 3 as the last commit failed
	stateDBSavepoint, _ = ledger.(*kvLedger).txtmgmt.GetLastSavepoint()
	testutil.AssertEquals(t, stateDBSavepoint.BlockNum, uint64(3))

	if ledgerconfig.IsHistoryDBEnabled() == true {
		qhistory, _ := ledger.NewHistoryQueryExecutor()
		itr, _ := qhistory.GetHistoryForKey("ns1", "key1")
		count := 0
		for {
			kmod, err := itr.Next()
			testutil.AssertNoError(t, err, "Error upon Next()")
			if kmod == nil {
				break
			}
			retrievedValue := kmod.(*queryresult.KeyModification).Value
			count++
			expectedValue := []byte("value1." + strconv.Itoa(count))
			testutil.AssertEquals(t, retrievedValue, expectedValue)
		}
		testutil.AssertEquals(t, count, 4)
		//savepoint in history DB should 4
		historyDBSavepoint, _ := ledger.(*kvLedger).historyDB.GetLastSavepoint()
		testutil.AssertEquals(t, historyDBSavepoint.BlockNum, uint64(4))
	}
	simulator.Done()
	ledger.Close()
	provider.Close()

	//we assume here that the peer comes online and calls NewKVLedger to get a handler for the ledger
	//state DB should be recovered before returning from NewKVLedger call
	provider, _ = NewProvider()
	ledger, _ = provider.Open("testLedger")
	simulator, _ = ledger.NewTxSimulator()
	value, _ = simulator.GetState("ns1", "key1")
	//value for 'key1' should be 'value10' after state DB recovery
	testutil.AssertEquals(t, value, []byte("value1.4"))
	value, _ = simulator.GetState("ns1", "key2")
	//value for 'key2' should be 'value11' after state DB recovery
	testutil.AssertEquals(t, value, []byte("value2.4"))
	value, _ = simulator.GetState("ns1", "key3")
	//value for 'key3' should be 'value12' after state DB recovery
	testutil.AssertEquals(t, value, []byte("value3.4"))
	//savepoint in state DB should 4 after the recovery
	stateDBSavepoint, _ = ledger.(*kvLedger).txtmgmt.GetLastSavepoint()
	testutil.AssertEquals(t, stateDBSavepoint.BlockNum, uint64(4))
	simulator.Done()
}

func TestLedgerWithCouchDbEnabledWithBinaryAndJSONData(t *testing.T) {

	//call a helper method to load the core.yaml
	ledgertestutil.SetupCoreYAMLConfig()

	logger.Debugf("TestLedgerWithCouchDbEnabledWithBinaryAndJSONData  IsCouchDBEnabled()value: %v , IsHistoryDBEnabled()value: %v\n",
		ledgerconfig.IsCouchDBEnabled(), ledgerconfig.IsHistoryDBEnabled())

	env := newTestEnv(t)
	defer env.cleanup()
	provider, _ := NewProvider()
	defer provider.Close()
	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := gb.Header.Hash()
	ledger, _ := provider.Create(gb)
	defer ledger.Close()

	bcInfo, _ := ledger.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil})

	simulator, _ := ledger.NewTxSimulator()
	simulator.SetState("ns1", "key4", []byte("value1"))
	simulator.SetState("ns1", "key5", []byte("value2"))
	simulator.SetState("ns1", "key6", []byte("{\"shipmentID\":\"161003PKC7300\",\"customsInvoice\":{\"methodOfTransport\":\"GROUND\",\"invoiceNumber\":\"00091622\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.SetState("ns1", "key7", []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"AIR MAYBE\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	block1 := bg.NextBlock([][]byte{simRes})

	ledger.Commit(block1)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := block1.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 2, CurrentBlockHash: block1Hash, PreviousBlockHash: gbHash})

	simulationResults := [][]byte{}
	simulator, _ = ledger.NewTxSimulator()
	simulator.SetState("ns1", "key4", []byte("value3"))
	simulator.SetState("ns1", "key5", []byte("{\"shipmentID\":\"161003PKC7500\",\"customsInvoice\":{\"methodOfTransport\":\"AIR FREIGHT\",\"invoiceNumber\":\"00091623\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.SetState("ns1", "key6", []byte("value4"))
	simulator.SetState("ns1", "key7", []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"GROUND\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.SetState("ns1", "key8", []byte("{\"shipmentID\":\"161003PKC7700\",\"customsInvoice\":{\"methodOfTransport\":\"SHIP\",\"invoiceNumber\":\"00091625\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	simulationResults = append(simulationResults, simRes)
	//add a 2nd transaction
	simulator2, _ := ledger.NewTxSimulator()
	simulator2.SetState("ns1", "key7", []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"TRAIN\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator2.SetState("ns1", "key9", []byte("value5"))
	simulator2.SetState("ns1", "key10", []byte("{\"shipmentID\":\"261003PKC8000\",\"customsInvoice\":{\"methodOfTransport\":\"DONKEY\",\"invoiceNumber\":\"00091626\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator2.Done()
	simRes2, _ := simulator2.GetTxSimulationResults()
	simulationResults = append(simulationResults, simRes2)

	block2 := bg.NextBlock(simulationResults)
	ledger.Commit(block2)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := block2.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &common.BlockchainInfo{
		Height: 3, CurrentBlockHash: block2Hash, PreviousBlockHash: block1Hash})

	b0, _ := ledger.GetBlockByHash(gbHash)
	testutil.AssertEquals(t, b0, gb)

	b1, _ := ledger.GetBlockByHash(block1Hash)
	testutil.AssertEquals(t, b1, block1)

	b2, _ := ledger.GetBlockByHash(block2Hash)
	testutil.AssertEquals(t, b2, block2)

	b0, _ = ledger.GetBlockByNumber(0)
	testutil.AssertEquals(t, b0, gb)

	b1, _ = ledger.GetBlockByNumber(1)
	testutil.AssertEquals(t, b1, block1)

	b2, _ = ledger.GetBlockByNumber(2)
	testutil.AssertEquals(t, b2, block2)

	//Similar test has been pushed down to historyleveldb_test.go as well
	if ledgerconfig.IsHistoryDBEnabled() == true {
		logger.Debugf("History is enabled\n")
		qhistory, err := ledger.NewHistoryQueryExecutor()
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to retrieve history database executor"))

		itr, err2 := qhistory.GetHistoryForKey("ns1", "key7")
		testutil.AssertNoError(t, err2, fmt.Sprintf("Error upon GetHistoryForKey"))

		var retrievedValue []byte
		count := 0
		for {
			kmod, _ := itr.Next()
			if kmod == nil {
				break
			}
			retrievedValue = kmod.(*queryresult.KeyModification).Value
			count++
		}
		testutil.AssertEquals(t, count, 3)
		// test the last value in the history matches the last value set for key7
		expectedValue := []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"TRAIN\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")
		testutil.AssertEquals(t, retrievedValue, expectedValue)

	}
}
