/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/util"
	lgr "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("lockbasedtxmgr,statevalidator,valimpl,confighistory,pvtstatepurgemgmt=debug")
	os.Exit(m.Run())
}

func TestKVLedgerNilHistoryDBProvider(t *testing.T) {
	kvl := &kvLedger{}
	qe, err := kvl.NewHistoryQueryExecutor()
	assert.Nil(
		t,
		qe,
		"NewHistoryQueryExecutor should return nil when history db provider is nil",
	)
	assert.NoError(
		t,
		err,
		"NewHistoryQueryExecutor should return an error when history db provider is nil",
	)
}

func TestKVLedgerBlockStorage(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t)
	defer provider.Close()

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	ledger, err := provider.Create(gb)
	assert.NoError(t, err)
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
	ledger.CommitLegacy(&lgr.BlockAndPvtData{Block: block1}, &lgr.CommitOptions{})

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := protoutil.BlockHeaderHash(block1.Header)
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
	ledger.CommitLegacy(&lgr.BlockAndPvtData{Block: block2}, &lgr.CommitOptions{})

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := protoutil.BlockHeaderHash(block2.Header)
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
	txEnv2, err := protoutil.GetEnvelopeFromBlock(txEnvBytes2)
	assert.NoError(t, err, "Error upon GetEnvelopeFromBlock")
	payload2, err := protoutil.UnmarshalPayload(txEnv2.Payload)
	assert.NoError(t, err, "Error upon GetPayload")
	chdr, err := protoutil.UnmarshalChannelHeader(payload2.Header.ChannelHeader)
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

func TestAddCommitHash(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t)
	defer provider.Close()

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	ledger, err := provider.Create(gb)
	assert.NoError(t, err)
	defer ledger.Close()

	// metadata associated with the above created geneis block is
	// empty. Hence, no commitHash would be empty.
	commitHash, err := ledger.(*kvLedger).lastPersistedCommitHash()
	assert.NoError(t, err)
	assert.Equal(t, commitHash, ledger.(*kvLedger).commitHash)
	assert.Equal(t, len(commitHash), 0)

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
	ledger.CommitLegacy(&lgr.BlockAndPvtData{Block: block1}, &lgr.CommitOptions{})

	commitHash, err = ledger.(*kvLedger).lastPersistedCommitHash()
	assert.NoError(t, err)
	assert.Equal(t, commitHash, ledger.(*kvLedger).commitHash)
	assert.Equal(t, len(commitHash), 32)

	// if the kvledger.commitHash is nil and the block number is > 1, the
	// commitHash should not be added to the block
	block2 := bg.NextBlock([][]byte{pubSimBytes})
	ledger.(*kvLedger).commitHash = nil
	ledger.CommitLegacy(&lgr.BlockAndPvtData{Block: block2}, &lgr.CommitOptions{})

	commitHash, err = ledger.(*kvLedger).lastPersistedCommitHash()
	assert.NoError(t, err)
	assert.Equal(t, commitHash, ledger.(*kvLedger).commitHash)
	assert.Equal(t, len(commitHash), 0)

}

func TestKVLedgerBlockStorageWithPvtdata(t *testing.T) {
	t.Skip()
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t)
	defer provider.Close()

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	ledger, err := provider.Create(gb)
	assert.NoError(t, err)
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
	assert.NoError(t, ledger.CommitLegacy(&lgr.BlockAndPvtData{Block: block1}, &lgr.CommitOptions{}))

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := protoutil.BlockHeaderHash(block1.Header)
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
	ledger.CommitLegacy(&lgr.BlockAndPvtData{Block: block2}, &lgr.CommitOptions{})

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := protoutil.BlockHeaderHash(block2.Header)
	assert.Equal(t, &common.BlockchainInfo{
		Height: 3, CurrentBlockHash: block2Hash, PreviousBlockHash: block1Hash,
	}, bcInfo)

	pvtdataAndBlock, _ := ledger.GetPvtDataAndBlockByNum(0, nil)
	assert.Equal(t, gb, pvtdataAndBlock.Block)
	assert.Nil(t, pvtdataAndBlock.PvtData)

	pvtdataAndBlock, _ = ledger.GetPvtDataAndBlockByNum(1, nil)
	assert.Equal(t, block1, pvtdataAndBlock.Block)
	assert.NotNil(t, pvtdataAndBlock.PvtData)
	assert.True(t, pvtdataAndBlock.PvtData[0].Has("ns1", "coll1"))
	assert.True(t, pvtdataAndBlock.PvtData[0].Has("ns1", "coll2"))

	pvtdataAndBlock, _ = ledger.GetPvtDataAndBlockByNum(2, nil)
	assert.Equal(t, block2, pvtdataAndBlock.Block)
	assert.Nil(t, pvtdataAndBlock.PvtData)
}

func TestKVLedgerDBRecovery(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider1 := testutilNewProviderWithCollectionConfig(
		t,
		"ns",
		map[string]uint64{"coll": 0},
		conf,
	)
	defer provider1.Close()

	testLedgerid := "testLedger"
	bg, gb := testutil.NewBlockGenerator(t, testLedgerid, false)
	ledger1, err := provider1.Create(gb)
	assert.NoError(t, err)
	defer ledger1.Close()

	gbHash := protoutil.BlockHeaderHash(gb.Header)
	checkBCSummaryForTest(t, ledger1,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil},
		},
	)

	// creating and committing the second data block
	blockAndPvtdata1 := prepareNextBlockForTest(t, ledger1, bg, "SimulateForBlk1",
		map[string]string{"key1": "value1.1", "key2": "value2.1", "key3": "value3.1"},
		map[string]string{"key1": "pvtValue1.1", "key2": "pvtValue2.1", "key3": "pvtValue3.1"})
	assert.NoError(t, ledger1.CommitLegacy(blockAndPvtdata1, &lgr.CommitOptions{}))
	checkBCSummaryForTest(t, ledger1,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 2,
				CurrentBlockHash:  protoutil.BlockHeaderHash(blockAndPvtdata1.Block.Header),
				PreviousBlockHash: gbHash},
		},
	)

	//======================================================================================
	// SCENARIO 1: peer writes the second block to the block storage and fails
	// before committing the block to state DB and history DB
	//======================================================================================
	blockAndPvtdata2 := prepareNextBlockForTest(t, ledger1, bg, "SimulateForBlk2",
		map[string]string{"key1": "value1.2", "key2": "value2.2", "key3": "value3.2"},
		map[string]string{"key1": "pvtValue1.2", "key2": "pvtValue2.2", "key3": "pvtValue3.2"})

	_, _, err = ledger1.(*kvLedger).txtmgmt.ValidateAndPrepare(blockAndPvtdata2, true)
	assert.NoError(t, err)
	assert.NoError(t, ledger1.(*kvLedger).blockStore.CommitWithPvtData(blockAndPvtdata2))

	// block storage should be as of block-2 but the state and history db should be as of block-1
	checkBCSummaryForTest(t, ledger1,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 3,
				CurrentBlockHash:  protoutil.BlockHeaderHash(blockAndPvtdata2.Block.Header),
				PreviousBlockHash: protoutil.BlockHeaderHash(blockAndPvtdata1.Block.Header)},

			stateDBSavePoint: uint64(1),
			stateDBKVs:       map[string]string{"key1": "value1.1", "key2": "value2.1", "key3": "value3.1"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.1", "key2": "pvtValue2.1", "key3": "pvtValue3.1"},

			historyDBSavePoint: uint64(1),
			historyKey:         "key1",
			historyVals:        []string{"value1.1"},
		},
	)
	// Now, assume that peer fails here before committing the transaction to the statedb and historydb
	ledger1.Close()
	provider1.Close()

	// Here the peer comes online and calls NewKVLedger to get a handler for the ledger
	// StateDB and HistoryDB should be recovered before returning from NewKVLedger call
	provider2 := testutilNewProviderWithCollectionConfig(
		t,
		"ns",
		map[string]uint64{"coll": 0},
		conf,
	)
	defer provider2.Close()
	ledger2, err := provider2.Open(testLedgerid)
	assert.NoError(t, err)
	defer ledger2.Close()
	checkBCSummaryForTest(t, ledger2,
		&bcSummary{
			stateDBSavePoint: uint64(2),
			stateDBKVs:       map[string]string{"key1": "value1.2", "key2": "value2.2", "key3": "value3.2"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.2", "key2": "pvtValue2.2", "key3": "pvtValue3.2"},

			historyDBSavePoint: uint64(2),
			historyKey:         "key1",
			historyVals:        []string{"value1.2", "value1.1"},
		},
	)

	//======================================================================================
	// SCENARIO 2: peer fails after committing the third block to the block storage and state DB
	// but before committing to history DB
	//======================================================================================
	blockAndPvtdata3 := prepareNextBlockForTest(t, ledger2, bg, "SimulateForBlk3",
		map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
		map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"},
	)
	_, _, err = ledger2.(*kvLedger).txtmgmt.ValidateAndPrepare(blockAndPvtdata3, true)
	assert.NoError(t, err)
	assert.NoError(t, ledger2.(*kvLedger).blockStore.CommitWithPvtData(blockAndPvtdata3))
	// committing the transaction to state DB
	assert.NoError(t, ledger2.(*kvLedger).txtmgmt.Commit())

	// assume that peer fails here after committing the transaction to state DB but before history DB
	checkBCSummaryForTest(t, ledger2,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 4,
				CurrentBlockHash:  protoutil.BlockHeaderHash(blockAndPvtdata3.Block.Header),
				PreviousBlockHash: protoutil.BlockHeaderHash(blockAndPvtdata2.Block.Header)},

			stateDBSavePoint: uint64(3),
			stateDBKVs:       map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"},

			historyDBSavePoint: uint64(2),
			historyKey:         "key1",
			historyVals:        []string{"value1.2", "value1.1"},
		},
	)
	ledger2.Close()
	provider2.Close()

	// we assume here that the peer comes online and calls NewKVLedger to get a handler for the ledger
	// history DB should be recovered before returning from NewKVLedger call
	provider3 := testutilNewProviderWithCollectionConfig(
		t,
		"ns",
		map[string]uint64{"coll": 0},
		conf,
	)
	defer provider3.Close()
	ledger3, err := provider3.Open(testLedgerid)
	assert.NoError(t, err)
	defer ledger3.Close()

	checkBCSummaryForTest(t, ledger3,
		&bcSummary{
			stateDBSavePoint: uint64(3),
			stateDBKVs:       map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"},

			historyDBSavePoint: uint64(3),
			historyKey:         "key1",
			historyVals:        []string{"value1.3", "value1.2", "value1.1"},
		},
	)

	// Rare scenario
	//======================================================================================
	// SCENARIO 3: peer fails after committing the fourth block to the block storgae
	// and history DB but before committing to state DB
	//======================================================================================
	blockAndPvtdata4 := prepareNextBlockForTest(t, ledger3, bg, "SimulateForBlk4",
		map[string]string{"key1": "value1.4", "key2": "value2.4", "key3": "value3.4"},
		map[string]string{"key1": "pvtValue1.4", "key2": "pvtValue2.4", "key3": "pvtValue3.4"},
	)

	_, _, err = ledger3.(*kvLedger).txtmgmt.ValidateAndPrepare(blockAndPvtdata4, true)
	assert.NoError(t, err)
	assert.NoError(t, ledger3.(*kvLedger).blockStore.CommitWithPvtData(blockAndPvtdata4))
	assert.NoError(t, ledger3.(*kvLedger).historyDB.Commit(blockAndPvtdata4.Block))

	checkBCSummaryForTest(t, ledger3,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 5,
				CurrentBlockHash:  protoutil.BlockHeaderHash(blockAndPvtdata4.Block.Header),
				PreviousBlockHash: protoutil.BlockHeaderHash(blockAndPvtdata3.Block.Header)},

			stateDBSavePoint: uint64(3),
			stateDBKVs:       map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"},

			historyDBSavePoint: uint64(4),
			historyKey:         "key1",
			historyVals:        []string{"value1.4", "value1.3", "value1.2", "value1.1"},
		},
	)
	ledger3.Close()
	provider3.Close()

	// we assume here that the peer comes online and calls NewKVLedger to get a handler for the ledger
	// state DB should be recovered before returning from NewKVLedger call
	provider4 := testutilNewProviderWithCollectionConfig(
		t,
		"ns",
		map[string]uint64{"coll": 0},
		conf,
	)
	defer provider4.Close()
	ledger4, err := provider4.Open(testLedgerid)
	assert.NoError(t, err)
	defer ledger4.Close()
	checkBCSummaryForTest(t, ledger4,
		&bcSummary{
			stateDBSavePoint: uint64(4),
			stateDBKVs:       map[string]string{"key1": "value1.4", "key2": "value2.4", "key3": "value3.4"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.4", "key2": "pvtValue2.4", "key3": "pvtValue3.4"},

			historyDBSavePoint: uint64(4),
			historyKey:         "key1",
			historyVals:        []string{"value1.4", "value1.3", "value1.2", "value1.1"},
		},
	)
}

func TestLedgerWithCouchDbEnabledWithBinaryAndJSONData(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t)
	defer provider.Close()
	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	ledger, err := provider.Create(gb)
	assert.NoError(t, err)
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

	ledger.CommitLegacy(&lgr.BlockAndPvtData{Block: block1}, &lgr.CommitOptions{})

	bcInfo, _ = ledger.GetBlockchainInfo()
	block1Hash := protoutil.BlockHeaderHash(block1.Header)
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
	ledger.CommitLegacy(&lgr.BlockAndPvtData{Block: block2}, &lgr.CommitOptions{})

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := protoutil.BlockHeaderHash(block2.Header)
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
	if conf.HistoryDBConfig.Enabled {
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
		// test the last value in the history matches the first value set for key7
		expectedValue := []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"AIR MAYBE\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")
		assert.Equal(t, expectedValue, retrievedValue)

	}
}

func prepareNextBlockWithMissingPvtDataForTest(t *testing.T, l lgr.PeerLedger, bg *testutil.BlockGenerator,
	txid string, pubKVs map[string]string, pvtKVs map[string]string) (*lgr.BlockAndPvtData, *lgr.TxPvtData) {

	blockAndPvtData := prepareNextBlockForTest(t, l, bg, txid, pubKVs, pvtKVs)

	blkMissingDataInfo := make(lgr.TxMissingPvtDataMap)
	blkMissingDataInfo.Add(0, "ns", "coll", true)
	blockAndPvtData.MissingPvtData = blkMissingDataInfo

	pvtData := blockAndPvtData.PvtData[0]
	delete(blockAndPvtData.PvtData, 0)

	return blockAndPvtData, pvtData
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
		PvtData: lgr.TxPvtDataMap{0: {SeqInBlock: 0, WriteSet: simRes.PvtSimulationResults}},
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
