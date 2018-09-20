/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/privdata"
	lgr "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	ledgertestutil.SetupCoreYAMLConfig()
	flogging.SetModuleLevel("lockbasedtxmgr", "debug")
	flogging.SetModuleLevel("statevalidator", "debug")
	flogging.SetModuleLevel("valimpl", "debug")
	flogging.SetModuleLevel("confighistory", "debug")
	flogging.SetModuleLevel("pvtstatepurgemgmt", "debug")
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger")
	viper.Set("ledger.history.enableHistoryDatabase", true)
	os.Exit(m.Run())
}

func TestKVLedgerBlockStorage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	provider := testutilNewProvider(t)
	defer provider.Close()

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := gb.Header.Hash()
	ledger, _ := provider.Create(gb)
	defer ledger.Close()

	bcInfo, _ := ledger.GetBlockchainInfo()
	assert.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	txid := util.GenerateUUID()
	simulator, _ := ledger.NewTxSimulator(txid)
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimBytes, _ := simRes.GetPubSimulationBytes()
	block1 := bg.NextBlock([][]byte{pubSimBytes})
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block1})

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := block1.Header.Hash()
	assert.Equal(t, &common.BlockchainInfo{
		Height: 2, CurrentBlockHash: block1Hash, PreviousBlockHash: gbHash,
	}, bcInfo)

	txid = util.GenerateUUID()
	simulator, _ = ledger.NewTxSimulator(txid)
	simulator.SetState("ns1", "key1", []byte("value4"))
	simulator.SetState("ns1", "key2", []byte("value5"))
	simulator.SetState("ns1", "key3", []byte("value6"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	pubSimBytes, _ = simRes.GetPubSimulationBytes()
	block2 := bg.NextBlock([][]byte{pubSimBytes})
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block2})

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := block2.Header.Hash()
	assert.Equal(t, &common.BlockchainInfo{
		Height: 3, CurrentBlockHash: block2Hash, PreviousBlockHash: block1Hash}, bcInfo)

	b0, _ := ledger.GetBlockByHash(gbHash)
	assert.True(t, proto.Equal(b0, gb), "proto messages are not equal")

	b1, _ := ledger.GetBlockByHash(block1Hash)
	assert.True(t, proto.Equal(b1, block1), "proto messages are not equal")

	b0, _ = ledger.GetBlockByNumber(0)
	assert.True(t, proto.Equal(b0, gb), "proto messages are not equal")

	b1, _ = ledger.GetBlockByNumber(1)
	assert.Equal(t, block1, b1)

	// get the tran id from the 2nd block, then use it to test GetTransactionByID()
	txEnvBytes2 := block1.Data.Data[0]
	txEnv2, err := putils.GetEnvelopeFromBlock(txEnvBytes2)
	assert.NoError(t, err, "Error upon GetEnvelopeFromBlock")
	payload2, err := putils.GetPayload(txEnv2)
	assert.NoError(t, err, "Error upon GetPayload")
	chdr, err := putils.UnmarshalChannelHeader(payload2.Header.ChannelHeader)
	assert.NoError(t, err, "Error upon GetChannelHeaderFromBytes")
	txID2 := chdr.TxId
	processedTran2, err := ledger.GetTransactionByID(txID2)
	assert.NoError(t, err, "Error upon GetTransactionByID")
	// get the tran envelope from the retrieved ProcessedTransaction
	retrievedTxEnv2 := processedTran2.TransactionEnvelope
	assert.Equal(t, txEnv2, retrievedTxEnv2)

	//  get the tran id from the 2nd block, then use it to test GetBlockByTxID
	b1, _ = ledger.GetBlockByTxID(txID2)
	assert.True(t, proto.Equal(b1, block1), "proto messages are not equal")

	// get the transaction validation code for this transaction id
	validCode, _ := ledger.GetTxValidationCodeByTxID(txID2)
	assert.Equal(t, peer.TxValidationCode_VALID, validCode)
}

func TestKVLedgerBlockStorageWithPvtdata(t *testing.T) {
	t.Skip()
	env := newTestEnv(t)
	defer env.cleanup()
	provider := testutilNewProvider(t)
	defer provider.Close()

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := gb.Header.Hash()
	ledger, _ := provider.Create(gb)
	defer ledger.Close()

	bcInfo, _ := ledger.GetBlockchainInfo()
	assert.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	txid := util.GenerateUUID()
	simulator, _ := ledger.NewTxSimulator(txid)
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetPrivateData("ns1", "coll1", "key2", []byte("value2"))
	simulator.SetPrivateData("ns1", "coll2", "key2", []byte("value3"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimBytes, _ := simRes.GetPubSimulationBytes()
	block1 := bg.NextBlockWithTxid([][]byte{pubSimBytes}, []string{txid})
	assert.NoError(t, ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block1}))

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := block1.Header.Hash()
	assert.Equal(t, &common.BlockchainInfo{
		Height: 2, CurrentBlockHash: block1Hash, PreviousBlockHash: gbHash,
	}, bcInfo)

	txid = util.GenerateUUID()
	simulator, _ = ledger.NewTxSimulator(txid)
	simulator.SetState("ns1", "key1", []byte("value4"))
	simulator.SetState("ns1", "key2", []byte("value5"))
	simulator.SetState("ns1", "key3", []byte("value6"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	pubSimBytes, _ = simRes.GetPubSimulationBytes()
	block2 := bg.NextBlock([][]byte{pubSimBytes})
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block2})

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := block2.Header.Hash()
	assert.Equal(t, &common.BlockchainInfo{
		Height: 3, CurrentBlockHash: block2Hash, PreviousBlockHash: block1Hash,
	}, bcInfo)

	pvtdataAndBlock, _ := ledger.GetPvtDataAndBlockByNum(0, nil)
	assert.Equal(t, gb, pvtdataAndBlock.Block)
	assert.Nil(t, pvtdataAndBlock.BlockPvtData)

	pvtdataAndBlock, _ = ledger.GetPvtDataAndBlockByNum(1, nil)
	assert.Equal(t, block1, pvtdataAndBlock.Block)
	assert.NotNil(t, pvtdataAndBlock.BlockPvtData)
	assert.True(t, pvtdataAndBlock.BlockPvtData[0].Has("ns1", "coll1"))
	assert.True(t, pvtdataAndBlock.BlockPvtData[0].Has("ns1", "coll2"))

	pvtdataAndBlock, _ = ledger.GetPvtDataAndBlockByNum(2, nil)
	assert.Equal(t, block2, pvtdataAndBlock.Block)
	assert.Nil(t, pvtdataAndBlock.BlockPvtData)
}

func TestKVLedgerDBRecovery(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	provider := testutilNewProvider(t)
	defer provider.Close()
	testLedgerid := "testLedger"
	bg, gb := testutil.NewBlockGenerator(t, testLedgerid, false)
	ledger, _ := provider.Create(gb)
	defer ledger.Close()
	gbHash := gb.Header.Hash()
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil},
		},
	)

	// creating and committing the first block with collection configs
	collectionConfigBlk := prepareNextBlockForTestCollectionConfigs(t, ledger, bg, "simulationForCollConfig", "ns", map[string]uint64{"coll": 0})
	assert.NoError(t, ledger.CommitWithPvtData(collectionConfigBlk))
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 2,
				CurrentBlockHash:  collectionConfigBlk.Block.Header.Hash(),
				PreviousBlockHash: gbHash},
		},
	)

	// TODO because of above collection configuration block, the block numbering for the following blocks
	// should be increased by 1 in the comments and variable names
	// creating and committing the second data block
	blockAndPvtdata1 := prepareNextBlockForTest(t, ledger, bg, "SimulateForBlk1",
		map[string]string{"key1": "value1.1", "key2": "value2.1", "key3": "value3.1"},
		map[string]string{"key1": "pvtValue1.1", "key2": "pvtValue2.1", "key3": "pvtValue3.1"})
	assert.NoError(t, ledger.CommitWithPvtData(blockAndPvtdata1))
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 3,
				CurrentBlockHash:  blockAndPvtdata1.Block.Header.Hash(),
				PreviousBlockHash: collectionConfigBlk.Block.Header.Hash()},
		},
	)

	//======================================================================================
	// SCENARIO 1: peer writes the second block to the block storage and fails
	// before committing the block to state DB and history DB
	//======================================================================================
	blockAndPvtdata2 := prepareNextBlockForTest(t, ledger, bg, "SimulateForBlk2",
		map[string]string{"key1": "value1.2", "key2": "value2.2", "key3": "value3.2"},
		map[string]string{"key1": "pvtValue1.2", "key2": "pvtValue2.2", "key3": "pvtValue3.2"})

	assert.NoError(t, ledger.(*kvLedger).txtmgmt.ValidateAndPrepare(blockAndPvtdata2, true))
	assert.NoError(t, ledger.(*kvLedger).blockStore.CommitWithPvtData(blockAndPvtdata2))

	// block storage should be as of block-2 but the state and history db should be as of block-1
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 4,
				CurrentBlockHash:  blockAndPvtdata2.Block.Header.Hash(),
				PreviousBlockHash: blockAndPvtdata1.Block.Header.Hash()},

			stateDBSavePoint: uint64(2),
			stateDBKVs:       map[string]string{"key1": "value1.1", "key2": "value2.1", "key3": "value3.1"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.1", "key2": "pvtValue2.1", "key3": "pvtValue3.1"},

			historyDBSavePoint: uint64(2),
			historyKey:         "key1",
			historyVals:        []string{"value1.1"},
		},
	)
	// Now, assume that peer fails here before committing the transaction to the statedb and historydb
	ledger.Close()
	provider.Close()

	// Here the peer comes online and calls NewKVLedger to get a handler for the ledger
	// StateDB and HistoryDB should be recovered before returning from NewKVLedger call
	provider = testutilNewProvider(t)
	ledger, _ = provider.Open(testLedgerid)
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			stateDBSavePoint: uint64(3),
			stateDBKVs:       map[string]string{"key1": "value1.2", "key2": "value2.2", "key3": "value3.2"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.2", "key2": "pvtValue2.2", "key3": "pvtValue3.2"},

			historyDBSavePoint: uint64(3),
			historyKey:         "key1",
			historyVals:        []string{"value1.1", "value1.2"},
		},
	)

	//======================================================================================
	// SCENARIO 2: peer fails after committing the third block to the block storage and state DB
	// but before committing to history DB
	//======================================================================================
	blockAndPvtdata3 := prepareNextBlockForTest(t, ledger, bg, "SimulateForBlk3",
		map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
		map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"},
	)
	assert.NoError(t, ledger.(*kvLedger).txtmgmt.ValidateAndPrepare(blockAndPvtdata3, true))
	assert.NoError(t, ledger.(*kvLedger).blockStore.CommitWithPvtData(blockAndPvtdata3))
	// committing the transaction to state DB
	assert.NoError(t, ledger.(*kvLedger).txtmgmt.Commit())

	// assume that peer fails here after committing the transaction to state DB but before history DB
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 5,
				CurrentBlockHash:  blockAndPvtdata3.Block.Header.Hash(),
				PreviousBlockHash: blockAndPvtdata2.Block.Header.Hash()},

			stateDBSavePoint: uint64(4),
			stateDBKVs:       map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"},

			historyDBSavePoint: uint64(3),
			historyKey:         "key1",
			historyVals:        []string{"value1.1", "value1.2"},
		},
	)
	ledger.Close()
	provider.Close()

	// we assume here that the peer comes online and calls NewKVLedger to get a handler for the ledger
	// history DB should be recovered before returning from NewKVLedger call
	provider = testutilNewProvider(t)
	ledger, _ = provider.Open(testLedgerid)

	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			stateDBSavePoint: uint64(4),
			stateDBKVs:       map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"},

			historyDBSavePoint: uint64(4),
			historyKey:         "key1",
			historyVals:        []string{"value1.1", "value1.2", "value1.3"},
		},
	)

	// Rare scenario
	//======================================================================================
	// SCENARIO 3: peer fails after committing the fourth block to the block storgae
	// and history DB but before committing to state DB
	//======================================================================================
	blockAndPvtdata4 := prepareNextBlockForTest(t, ledger, bg, "SimulateForBlk4",
		map[string]string{"key1": "value1.4", "key2": "value2.4", "key3": "value3.4"},
		map[string]string{"key1": "pvtValue1.4", "key2": "pvtValue2.4", "key3": "pvtValue3.4"},
	)

	assert.NoError(t, ledger.(*kvLedger).txtmgmt.ValidateAndPrepare(blockAndPvtdata4, true))
	assert.NoError(t, ledger.(*kvLedger).blockStore.CommitWithPvtData(blockAndPvtdata4))
	assert.NoError(t, ledger.(*kvLedger).historyDB.Commit(blockAndPvtdata4.Block))

	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 6,
				CurrentBlockHash:  blockAndPvtdata4.Block.Header.Hash(),
				PreviousBlockHash: blockAndPvtdata3.Block.Header.Hash()},

			stateDBSavePoint: uint64(4),
			stateDBKVs:       map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"},

			historyDBSavePoint: uint64(5),
			historyKey:         "key1",
			historyVals:        []string{"value1.1", "value1.2", "value1.3", "value1.4"},
		},
	)
	ledger.Close()
	provider.Close()

	// we assume here that the peer comes online and calls NewKVLedger to get a handler for the ledger
	// state DB should be recovered before returning from NewKVLedger call
	provider = testutilNewProvider(t)
	ledger, _ = provider.Open(testLedgerid)
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			stateDBSavePoint: uint64(5),
			stateDBKVs:       map[string]string{"key1": "value1.4", "key2": "value2.4", "key3": "value3.4"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.4", "key2": "pvtValue2.4", "key3": "pvtValue3.4"},

			historyDBSavePoint: uint64(5),
			historyKey:         "key1",
			historyVals:        []string{"value1.1", "value1.2", "value1.3", "value1.4"},
		},
	)
}

func TestLedgerWithCouchDbEnabledWithBinaryAndJSONData(t *testing.T) {

	//call a helper method to load the core.yaml
	ledgertestutil.SetupCoreYAMLConfig()

	logger.Debugf("TestLedgerWithCouchDbEnabledWithBinaryAndJSONData  IsCouchDBEnabled()value: %v , IsHistoryDBEnabled()value: %v\n",
		ledgerconfig.IsCouchDBEnabled(), ledgerconfig.IsHistoryDBEnabled())

	env := newTestEnv(t)
	defer env.cleanup()
	provider := testutilNewProvider(t)
	defer provider.Close()
	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := gb.Header.Hash()
	ledger, _ := provider.Create(gb)
	defer ledger.Close()

	bcInfo, _ := ledger.GetBlockchainInfo()
	assert.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil}, bcInfo)

	txid := util.GenerateUUID()
	simulator, _ := ledger.NewTxSimulator(txid)
	simulator.SetState("ns1", "key4", []byte("value1"))
	simulator.SetState("ns1", "key5", []byte("value2"))
	simulator.SetState("ns1", "key6", []byte("{\"shipmentID\":\"161003PKC7300\",\"customsInvoice\":{\"methodOfTransport\":\"GROUND\",\"invoiceNumber\":\"00091622\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.SetState("ns1", "key7", []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"AIR MAYBE\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimBytes, _ := simRes.GetPubSimulationBytes()
	block1 := bg.NextBlock([][]byte{pubSimBytes})

	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block1})

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := block1.Header.Hash()
	assert.Equal(t, &common.BlockchainInfo{
		Height: 2, CurrentBlockHash: block1Hash, PreviousBlockHash: gbHash}, bcInfo)

	simulationResults := [][]byte{}
	txid = util.GenerateUUID()
	simulator, _ = ledger.NewTxSimulator(txid)
	simulator.SetState("ns1", "key4", []byte("value3"))
	simulator.SetState("ns1", "key5", []byte("{\"shipmentID\":\"161003PKC7500\",\"customsInvoice\":{\"methodOfTransport\":\"AIR FREIGHT\",\"invoiceNumber\":\"00091623\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.SetState("ns1", "key6", []byte("value4"))
	simulator.SetState("ns1", "key7", []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"GROUND\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.SetState("ns1", "key8", []byte("{\"shipmentID\":\"161003PKC7700\",\"customsInvoice\":{\"methodOfTransport\":\"SHIP\",\"invoiceNumber\":\"00091625\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	pubSimBytes, _ = simRes.GetPubSimulationBytes()
	simulationResults = append(simulationResults, pubSimBytes)
	//add a 2nd transaction
	txid2 := util.GenerateUUID()
	simulator2, _ := ledger.NewTxSimulator(txid2)
	simulator2.SetState("ns1", "key7", []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"TRAIN\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator2.SetState("ns1", "key9", []byte("value5"))
	simulator2.SetState("ns1", "key10", []byte("{\"shipmentID\":\"261003PKC8000\",\"customsInvoice\":{\"methodOfTransport\":\"DONKEY\",\"invoiceNumber\":\"00091626\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}"))
	simulator2.Done()
	simRes2, _ := simulator2.GetTxSimulationResults()
	pubSimBytes2, _ := simRes2.GetPubSimulationBytes()
	simulationResults = append(simulationResults, pubSimBytes2)

	block2 := bg.NextBlock(simulationResults)
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block2})

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := block2.Header.Hash()
	assert.Equal(t, &common.BlockchainInfo{
		Height: 3, CurrentBlockHash: block2Hash, PreviousBlockHash: block1Hash,
	}, bcInfo)

	b0, _ := ledger.GetBlockByHash(gbHash)
	assert.True(t, proto.Equal(b0, gb), "proto messages are not equal")

	b1, _ := ledger.GetBlockByHash(block1Hash)
	assert.True(t, proto.Equal(b1, block1), "proto messages are not equal")

	b2, _ := ledger.GetBlockByHash(block2Hash)
	assert.True(t, proto.Equal(b2, block2), "proto messages are not equal")

	b0, _ = ledger.GetBlockByNumber(0)
	assert.True(t, proto.Equal(b0, gb), "proto messages are not equal")

	b1, _ = ledger.GetBlockByNumber(1)
	assert.True(t, proto.Equal(b1, block1), "proto messages are not equal")

	b2, _ = ledger.GetBlockByNumber(2)
	assert.True(t, proto.Equal(b2, block2), "proto messages are not equal")

	//Similar test has been pushed down to historyleveldb_test.go as well
	if ledgerconfig.IsHistoryDBEnabled() == true {
		logger.Debugf("History is enabled\n")
		qhistory, err := ledger.NewHistoryQueryExecutor()
		assert.NoError(t, err, "Error when trying to retrieve history database executor")

		itr, err2 := qhistory.GetHistoryForKey("ns1", "key7")
		assert.NoError(t, err2, "Error upon GetHistoryForKey")

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
		assert.Equal(t, 3, count)
		// test the last value in the history matches the last value set for key7
		expectedValue := []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"TRAIN\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")
		assert.Equal(t, expectedValue, retrievedValue)

	}
}

func prepareNextBlockForTest(t *testing.T, l lgr.PeerLedger, bg *testutil.BlockGenerator,
	txid string, pubKVs map[string]string, pvtKVs map[string]string) *lgr.BlockAndPvtData {
	simulator, _ := l.NewTxSimulator(txid)
	//simulating transaction
	for k, v := range pubKVs {
		simulator.SetState("ns", k, []byte(v))
	}
	for k, v := range pvtKVs {
		simulator.SetPrivateData("ns", "coll", k, []byte(v))
	}
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimBytes, _ := simRes.GetPubSimulationBytes()
	block := bg.NextBlock([][]byte{pubSimBytes})
	return &lgr.BlockAndPvtData{Block: block,
		BlockPvtData: map[uint64]*lgr.TxPvtData{0: {SeqInBlock: 0, WriteSet: simRes.PvtSimulationResults}},
	}
}

func checkBCSummaryForTest(t *testing.T, l lgr.PeerLedger, expectedBCSummary *bcSummary) {
	if expectedBCSummary.bcInfo != nil {
		actualBCInfo, _ := l.GetBlockchainInfo()
		assert.Equal(t, expectedBCSummary.bcInfo, actualBCInfo)
	}

	if expectedBCSummary.stateDBSavePoint != 0 {
		actualStateDBSavepoint, _ := l.(*kvLedger).txtmgmt.GetLastSavepoint()
		assert.Equal(t, expectedBCSummary.stateDBSavePoint, actualStateDBSavepoint.BlockNum)
	}

	if !(expectedBCSummary.stateDBKVs == nil && expectedBCSummary.stateDBPvtKVs == nil) {
		checkStateDBForTest(t, l, expectedBCSummary.stateDBKVs, expectedBCSummary.stateDBPvtKVs)
	}

	if expectedBCSummary.historyDBSavePoint != 0 {
		actualHistoryDBSavepoint, _ := l.(*kvLedger).historyDB.GetLastSavepoint()
		assert.Equal(t, expectedBCSummary.historyDBSavePoint, actualHistoryDBSavepoint.BlockNum)
	}

	if expectedBCSummary.historyKey != "" {
		checkHistoryDBForTest(t, l, expectedBCSummary.historyKey, expectedBCSummary.historyVals)
	}
}

func checkStateDBForTest(t *testing.T, l lgr.PeerLedger, expectedKVs map[string]string, expectedPvtKVs map[string]string) {
	simulator, _ := l.NewTxSimulator("checkStateDBForTest")
	defer simulator.Done()
	for expectedKey, expectedVal := range expectedKVs {
		actualVal, _ := simulator.GetState("ns", expectedKey)
		assert.Equal(t, []byte(expectedVal), actualVal)
	}

	for expectedPvtKey, expectedPvtVal := range expectedPvtKVs {
		actualPvtVal, _ := simulator.GetPrivateData("ns", "coll", expectedPvtKey)
		assert.Equal(t, []byte(expectedPvtVal), actualPvtVal)
	}
}

func checkHistoryDBForTest(t *testing.T, l lgr.PeerLedger, key string, expectedVals []string) {
	qhistory, _ := l.NewHistoryQueryExecutor()
	itr, _ := qhistory.GetHistoryForKey("ns", key)
	var actualVals []string
	for {
		kmod, err := itr.Next()
		assert.NoError(t, err, "Error upon Next()")
		if kmod == nil {
			break
		}
		retrievedValue := kmod.(*queryresult.KeyModification).Value
		actualVals = append(actualVals, string(retrievedValue))
	}
	assert.Equal(t, expectedVals, actualVals)
}

type bcSummary struct {
	bcInfo             *common.BlockchainInfo
	stateDBSavePoint   uint64
	stateDBKVs         map[string]string
	stateDBPvtKVs      map[string]string
	historyDBSavePoint uint64
	historyKey         string
	historyVals        []string
}

func prepareNextBlockForTestCollectionConfigs(t *testing.T, l lgr.PeerLedger, bg *testutil.BlockGenerator,
	txid string, namespace string, btlConfigs map[string]uint64) *lgr.BlockAndPvtData {
	simulator, _ := l.NewTxSimulator(txid)
	key := privdata.BuildCollectionKVSKey(namespace)
	var conf []*common.CollectionConfig
	for collName, btl := range btlConfigs {
		staticConf := &common.StaticCollectionConfig{Name: collName, BlockToLive: btl}
		collectionConf := &common.CollectionConfig{}
		collectionConf.Payload = &common.CollectionConfig_StaticCollectionConfig{StaticCollectionConfig: staticConf}
		conf = append(conf, collectionConf)
	}
	collectionConfPkg := &common.CollectionConfigPackage{Config: conf}
	value, err := proto.Marshal(collectionConfPkg)
	assert.NoError(t, err)
	simulator.SetState("lscc", key, value)
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimBytes, _ := simRes.GetPubSimulationBytes()
	block := bg.NextBlock([][]byte{pubSimBytes})
	return &lgr.BlockAndPvtData{Block: block}
}
