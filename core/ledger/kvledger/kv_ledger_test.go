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
	"testing"

	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestKVLedgerBlockStorage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ledger, _ := NewKVLedger(env.conf)
	defer ledger.Close()
	bcInfo, _ := ledger.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 0, CurrentBlockHash: nil, PreviousBlockHash: nil})

	simulator, _ := ledger.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	bg := testutil.NewBlockGenerator(t)
	block1 := bg.NextBlock([][]byte{simRes}, false)
	ledger.Commit(block1)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := block1.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 1, CurrentBlockHash: block1Hash, PreviousBlockHash: []byte{}})

	simulator, _ = ledger.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value4"))
	simulator.SetState("ns1", "key2", []byte("value5"))
	simulator.SetState("ns1", "key3", []byte("value6"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	block2 := bg.NextBlock([][]byte{simRes}, false)
	ledger.Commit(block2)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := block2.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 2, CurrentBlockHash: block2Hash, PreviousBlockHash: block1.Header.Hash()})

	b1, _ := ledger.GetBlockByHash(block1Hash)
	testutil.AssertEquals(t, b1, block1)

	b2, _ := ledger.GetBlockByHash(block2Hash)
	testutil.AssertEquals(t, b2, block2)

	b1, _ = ledger.GetBlockByNumber(1)
	testutil.AssertEquals(t, b1, block1)

	b2, _ = ledger.GetBlockByNumber(2)
	testutil.AssertEquals(t, b2, block2)
}

func TestKVLedgerStateDBRecovery(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ledger, _ := NewKVLedger(env.conf)
	defer ledger.Close()

	bcInfo, _ := ledger.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 0, CurrentBlockHash: nil, PreviousBlockHash: nil})

	//creating and committing the first block
	simulator, _ := ledger.NewTxSimulator()
	//simulating a transaction
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	//generating a block based on the simulation result
	bg := testutil.NewBlockGenerator(t)
	block1 := bg.NextBlock([][]byte{simRes}, false)
	//performing validation of read and write set to find valid transactions
	ledger.Commit(block1)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := block1.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 1, CurrentBlockHash: block1Hash, PreviousBlockHash: []byte{}})

	//creating the second block but peer fails before committing the transaction to state DB
	simulator, _ = ledger.NewTxSimulator()
	//simulating transaction
	simulator.SetState("ns1", "key1", []byte("value4"))
	simulator.SetState("ns1", "key2", []byte("value5"))
	simulator.SetState("ns1", "key3", []byte("value6"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	//generating a block based on the simulation result
	block2 := bg.NextBlock([][]byte{simRes}, false)
	//performing validation of read and write set to find valid transactions
	ledger.txtmgmt.ValidateAndPrepare(block2, true)
	//writing the validated block to block storage but not committing the transaction to state DB
	err := ledger.blockStore.AddBlock(block2)
	//assume that peer fails here before committing the transaction
	assert.NoError(t, err)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := block2.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 2, CurrentBlockHash: block2Hash, PreviousBlockHash: block1.Header.Hash()})

	simulator, _ = ledger.NewTxSimulator()
	value, _ := simulator.GetState("ns1", "key1")
	//value for 'key1' should be 'value1' as the last commit failed
	testutil.AssertEquals(t, value, []byte("value1"))
	value, _ = simulator.GetState("ns1", "key2")
	//value for 'key2' should be 'value2' as the last commit failed
	testutil.AssertEquals(t, value, []byte("value2"))
	value, _ = simulator.GetState("ns1", "key3")
	//value for 'key3' should be 'value3' as the last commit failed
	testutil.AssertEquals(t, value, []byte("value3"))
	simulator.Done()
	ledger.Close()

	//we assume here that the peer comes online and calls NewKVLedger to get a handler for the ledger
	//State DB should be recovered before returning from NewKVLedger call
	ledger, _ = NewKVLedger(env.conf)
	simulator, _ = ledger.NewTxSimulator()
	value, _ = simulator.GetState("ns1", "key1")
	//value for 'key1' should be 'value4' after recovery
	testutil.AssertEquals(t, value, []byte("value4"))
	value, _ = simulator.GetState("ns1", "key2")
	//value for 'key2' should be 'value5' after recovery
	testutil.AssertEquals(t, value, []byte("value5"))
	value, _ = simulator.GetState("ns1", "key3")
	//value for 'key3' should be 'value6' after recovery
	testutil.AssertEquals(t, value, []byte("value6"))
	simulator.Done()
	ledger.Close()
}

func TestLedgerWithCouchDbEnabledWithBinaryAndJSONData(t *testing.T) {

	//call a helper method to load the core.yaml
	testutil.SetupCoreYAMLConfig("./../../../peer")

	logger.Debugf("TestLedgerWithCouchDbEnabledWithBinaryAndJSONData  IsCouchDBEnabled()value: %v , IsHistoryDBEnabled()value: %v\n",
		ledgerconfig.IsCouchDBEnabled(), ledgerconfig.IsHistoryDBEnabled())

	env := newTestEnv(t)
	defer env.cleanup()
	ledger, _ := NewKVLedger(env.conf)
	defer ledger.Close()

	//testNs := "ns1"

	bcInfo, _ := ledger.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 0, CurrentBlockHash: nil, PreviousBlockHash: nil})

	simulator, _ := ledger.NewTxSimulator()
	simulator.SetState("ns1", "key4", []byte("value1"))
	simulator.SetState("ns1", "key5", []byte("value2"))
	simulator.SetState("ns1", "key6", []byte("{\"shipmentID\":\"161003PKC7300\",\"customsInvoice\":{\"methodOfTransport\":\"GROUND\",\"invoiceNumber\":\"00091622\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.SetState("ns1", "key7", []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"AIR MAYBE\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	bg := testutil.NewBlockGenerator(t)
	block1 := bg.NextBlock([][]byte{simRes}, false)

	ledger.Commit(block1)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := block1.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 1, CurrentBlockHash: block1Hash, PreviousBlockHash: []byte{}})

	//Note key 4 and 6 are updates but key 7 is new.  I.E. should see history for key 4 and 6 if history is enabled
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

	block2 := bg.NextBlock(simulationResults, false)
	ledger.Commit(block2)

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := block2.Header.Hash()
	testutil.AssertEquals(t, bcInfo, &pb.BlockchainInfo{
		Height: 2, CurrentBlockHash: block2Hash, PreviousBlockHash: block1.Header.Hash()})

	b1, _ := ledger.GetBlockByHash(block1Hash)
	testutil.AssertEquals(t, b1, block1)

	b2, _ := ledger.GetBlockByHash(block2Hash)
	testutil.AssertEquals(t, b2, block2)

	b1, _ = ledger.GetBlockByNumber(1)
	testutil.AssertEquals(t, b1, block1)

	b2, _ = ledger.GetBlockByNumber(2)
	testutil.AssertEquals(t, b2, block2)

	//TODO move this test to history.
	if ledgerconfig.IsHistoryDBEnabled() == true {
		qhistory, err := ledger.NewHistoryQueryExecutor()
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to retrieve history database executor"))

		itr, err2 := qhistory.GetTransactionsForKey("ns1", "key7", true, false)
		testutil.AssertNoError(t, err2, fmt.Sprintf("Error when trying to retrieve history database executor"))

		count := 0
		for {
			kmod, _ := itr.Next()
			if kmod == nil {
				break
			}
			//TODO TEST CONTENT - need to point to ledger import and not the KVledger
			//TODO MOVE TEST TO HISTORY
			//bt := kmod.(*ledger.KeyModification).TxID
			//v := kmod.(*ledger.KeyModification).Value
			//t.Logf("Retrieved for ns=%s, key=key7  : k=%s, v=%s at count=%d start=%s end=%s", testNs, bt, v, count)
			count++
		}
		testutil.AssertEquals(t, count, 3)
	}
}
