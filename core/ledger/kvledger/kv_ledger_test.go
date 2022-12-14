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
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/validation"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

var (
	couchDBAddress  string
	stopCouchDBFunc func()
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("lockbasedtxmgr,statevalidator,valimpl,confighistory,pvtstatepurgemgmt=debug")
	exitCode := m.Run()
	if couchDBAddress != "" {
		couchDBAddress = ""
		stopCouchDBFunc()
	}
	os.Exit(exitCode)
}

func TestKVLedgerNilHistoryDBProvider(t *testing.T) {
	kvl := &kvLedger{}
	qe, err := kvl.NewHistoryQueryExecutor()
	require.Nil(
		t,
		qe,
		"NewHistoryQueryExecutor should return nil when history db provider is nil",
	)
	require.NoError(
		t,
		err,
		"NewHistoryQueryExecutor should return an error when history db provider is nil",
	)
}

func TestKVLedgerBlockStorage(t *testing.T) {
	t.Run("green-path", func(t *testing.T) {
		conf := testConfig(t)
		provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
		defer provider.Close()

		bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
		gbHash := protoutil.BlockHeaderHash(gb.Header)
		lgr, err := provider.CreateFromGenesisBlock(gb)
		require.NoError(t, err)
		defer lgr.Close()

		bcInfo, _ := lgr.GetBlockchainInfo()
		require.Equal(t, &common.BlockchainInfo{
			Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
		}, bcInfo)

		txid := util.GenerateUUID()
		simulator, _ := lgr.NewTxSimulator(txid)
		require.NoError(t, simulator.SetState("ns1", "key1", []byte("value1")))
		require.NoError(t, simulator.SetState("ns1", "key2", []byte("value2")))
		require.NoError(t, simulator.SetState("ns1", "key3", []byte("value3")))
		simulator.Done()
		simRes, _ := simulator.GetTxSimulationResults()
		pubSimBytes, _ := simRes.GetPubSimulationBytes()
		block1 := bg.NextBlock([][]byte{pubSimBytes})
		require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: block1}, &ledger.CommitOptions{}))

		bcInfo, _ = lgr.GetBlockchainInfo()
		block1Hash := protoutil.BlockHeaderHash(block1.Header)
		require.Equal(t, &common.BlockchainInfo{
			Height: 2, CurrentBlockHash: block1Hash, PreviousBlockHash: gbHash,
		}, bcInfo)

		txid = util.GenerateUUID()
		simulator, _ = lgr.NewTxSimulator(txid)
		require.NoError(t, simulator.SetState("ns1", "key1", []byte("value4")))
		require.NoError(t, simulator.SetState("ns1", "key2", []byte("value5")))
		require.NoError(t, simulator.SetState("ns1", "key3", []byte("value6")))
		simulator.Done()
		simRes, _ = simulator.GetTxSimulationResults()
		pubSimBytes, _ = simRes.GetPubSimulationBytes()
		block2 := bg.NextBlock([][]byte{pubSimBytes})
		require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: block2}, &ledger.CommitOptions{}))

		bcInfo, _ = lgr.GetBlockchainInfo()
		block2Hash := protoutil.BlockHeaderHash(block2.Header)
		require.Equal(t, &common.BlockchainInfo{
			Height: 3, CurrentBlockHash: block2Hash, PreviousBlockHash: block1Hash,
		}, bcInfo)

		b0, _ := lgr.GetBlockByHash(gbHash)
		require.True(t, proto.Equal(b0, gb), "proto messages are not equal")

		b1, _ := lgr.GetBlockByHash(block1Hash)
		require.True(t, proto.Equal(b1, block1), "proto messages are not equal")

		b0, _ = lgr.GetBlockByNumber(0)
		require.True(t, proto.Equal(b0, gb), "proto messages are not equal")

		b1, _ = lgr.GetBlockByNumber(1)
		require.Equal(t, block1, b1)

		// get the tran id from the 2nd block, then use it to test GetTransactionByID()
		txEnvBytes2 := block1.Data.Data[0]
		txEnv2, err := protoutil.GetEnvelopeFromBlock(txEnvBytes2)
		require.NoError(t, err, "Error upon GetEnvelopeFromBlock")
		payload2, err := protoutil.UnmarshalPayload(txEnv2.Payload)
		require.NoError(t, err, "Error upon GetPayload")
		chdr, err := protoutil.UnmarshalChannelHeader(payload2.Header.ChannelHeader)
		require.NoError(t, err, "Error upon GetChannelHeaderFromBytes")
		txID2 := chdr.TxId

		exists, err := lgr.TxIDExists(txID2)
		require.NoError(t, err)
		require.True(t, exists)

		processedTran2, err := lgr.GetTransactionByID(txID2)
		require.NoError(t, err, "Error upon GetTransactionByID")
		// get the tran envelope from the retrieved ProcessedTransaction
		retrievedTxEnv2 := processedTran2.TransactionEnvelope
		require.Equal(t, txEnv2, retrievedTxEnv2)

		//  get the tran id from the 2nd block, then use it to test GetBlockByTxID
		b1, _ = lgr.GetBlockByTxID(txID2)
		require.True(t, proto.Equal(b1, block1), "proto messages are not equal")

		// get the transaction validation code for this transaction id
		validCode, blkNum, err := lgr.GetTxValidationCodeByTxID(txID2)
		require.NoError(t, err)
		require.Equal(t, peer.TxValidationCode_VALID, validCode)
		require.Equal(t, uint64(1), blkNum)

		exists, err = lgr.TxIDExists("random-txid")
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("error-path", func(t *testing.T) {
		conf := testConfig(t)
		provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
		defer provider.Close()

		_, gb := testutil.NewBlockGenerator(t, "testLedger", false)
		lgr, err := provider.CreateFromGenesisBlock(gb)
		require.NoError(t, err)
		defer lgr.Close()

		provider.blkStoreProvider.Close()
		exists, err := lgr.TxIDExists("random-txid")
		require.EqualError(t, err, "error while trying to check the presence of TXID [random-txid]: internal leveldb error while obtaining db iterator: leveldb: closed")
		require.False(t, exists)
	})
}

func TestAddCommitHash(t *testing.T) {
	conf := testConfig(t)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	lgr, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	defer lgr.Close()

	// metadata associated with the above created geneis block is
	// empty. Hence, no commitHash would be empty.
	commitHash, err := lgr.(*kvLedger).lastPersistedCommitHash()
	require.NoError(t, err)
	require.Equal(t, commitHash, lgr.(*kvLedger).commitHash)
	require.Equal(t, len(commitHash), 0)

	bcInfo, _ := lgr.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	txid := util.GenerateUUID()
	simulator, _ := lgr.NewTxSimulator(txid)
	require.NoError(t, simulator.SetState("ns1", "key1", []byte("value1")))
	require.NoError(t, simulator.SetState("ns1", "key2", []byte("value2")))
	require.NoError(t, simulator.SetState("ns1", "key3", []byte("value3")))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimBytes, _ := simRes.GetPubSimulationBytes()
	block1 := bg.NextBlock([][]byte{pubSimBytes})
	require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: block1}, &ledger.CommitOptions{}))

	commitHash, err = lgr.(*kvLedger).lastPersistedCommitHash()
	require.NoError(t, err)
	require.Equal(t, commitHash, lgr.(*kvLedger).commitHash)
	require.Equal(t, len(commitHash), 32)

	// if the kvledger.commitHash is nil and the block number is > 1, the
	// commitHash should not be added to the block
	block2 := bg.NextBlock([][]byte{pubSimBytes})
	lgr.(*kvLedger).commitHash = nil
	require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: block2}, &ledger.CommitOptions{}))

	commitHash, err = lgr.(*kvLedger).lastPersistedCommitHash()
	require.NoError(t, err)
	require.Equal(t, commitHash, lgr.(*kvLedger).commitHash)
	require.Equal(t, len(commitHash), 0)
}

func TestKVLedgerBlockStorageWithPvtdata(t *testing.T) {
	t.Skip()
	conf := testConfig(t)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	lgr, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	defer lgr.Close()

	bcInfo, _ := lgr.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	txid := util.GenerateUUID()
	simulator, _ := lgr.NewTxSimulator(txid)
	require.NoError(t, simulator.SetState("ns1", "key1", []byte("value1")))
	require.NoError(t, simulator.SetPrivateData("ns1", "coll1", "key2", []byte("value2")))
	require.NoError(t, simulator.SetPrivateData("ns1", "coll2", "key2", []byte("value3")))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimBytes, _ := simRes.GetPubSimulationBytes()
	block1 := bg.NextBlockWithTxid([][]byte{pubSimBytes}, []string{txid})
	require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: block1}, &ledger.CommitOptions{}))

	bcInfo, _ = lgr.GetBlockchainInfo()
	block1Hash := protoutil.BlockHeaderHash(block1.Header)
	require.Equal(t, &common.BlockchainInfo{
		Height: 2, CurrentBlockHash: block1Hash, PreviousBlockHash: gbHash,
	}, bcInfo)

	txid = util.GenerateUUID()
	simulator, _ = lgr.NewTxSimulator(txid)
	require.NoError(t, simulator.SetState("ns1", "key1", []byte("value4")))
	require.NoError(t, simulator.SetState("ns1", "key2", []byte("value5")))
	require.NoError(t, simulator.SetState("ns1", "key3", []byte("value6")))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	pubSimBytes, _ = simRes.GetPubSimulationBytes()
	block2 := bg.NextBlock([][]byte{pubSimBytes})
	require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: block2}, &ledger.CommitOptions{}))

	bcInfo, _ = lgr.GetBlockchainInfo()
	block2Hash := protoutil.BlockHeaderHash(block2.Header)
	require.Equal(t, &common.BlockchainInfo{
		Height: 3, CurrentBlockHash: block2Hash, PreviousBlockHash: block1Hash,
	}, bcInfo)

	pvtdataAndBlock, _ := lgr.GetPvtDataAndBlockByNum(0, nil)
	require.Equal(t, gb, pvtdataAndBlock.Block)
	require.Nil(t, pvtdataAndBlock.PvtData)

	pvtdataAndBlock, _ = lgr.GetPvtDataAndBlockByNum(1, nil)
	require.Equal(t, block1, pvtdataAndBlock.Block)
	require.NotNil(t, pvtdataAndBlock.PvtData)
	require.True(t, pvtdataAndBlock.PvtData[0].Has("ns1", "coll1"))
	require.True(t, pvtdataAndBlock.PvtData[0].Has("ns1", "coll2"))

	pvtdataAndBlock, _ = lgr.GetPvtDataAndBlockByNum(2, nil)
	require.Equal(t, block2, pvtdataAndBlock.Block)
	require.Nil(t, pvtdataAndBlock.PvtData)
}

func TestKVLedgerDBRecovery(t *testing.T) {
	conf := testConfig(t)
	nsCollBtlConfs := []*nsCollBtlConfig{
		{
			namespace: "ns",
			btlConfig: map[string]uint64{"coll": 0},
		},
	}
	provider1 := testutilNewProviderWithCollectionConfig(
		t,
		nsCollBtlConfs,
		conf,
	)
	defer provider1.Close()

	testLedgerid := "testLedger"
	bg, gb := testutil.NewBlockGenerator(t, testLedgerid, false)
	ledger1, err := provider1.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
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
	require.NoError(t, ledger1.CommitLegacy(blockAndPvtdata1, &ledger.CommitOptions{}))
	checkBCSummaryForTest(t, ledger1,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{
				Height:            2,
				CurrentBlockHash:  protoutil.BlockHeaderHash(blockAndPvtdata1.Block.Header),
				PreviousBlockHash: gbHash,
			},
		},
	)

	//======================================================================================
	// SCENARIO 1: peer writes the second block to the block storage and fails
	// before committing the block to state DB and history DB
	//======================================================================================
	blockAndPvtdata2 := prepareNextBlockForTest(t, ledger1, bg, "SimulateForBlk2",
		map[string]string{"key1": "value1.2", "key2": "value2.2", "key3": "value3.2"},
		map[string]string{"key1": "pvtValue1.2", "key2": "pvtValue2.2", "key3": "pvtValue3.2"})

	_, _, _, err = ledger1.(*kvLedger).txmgr.ValidateAndPrepare(blockAndPvtdata2, true)
	require.NoError(t, err)
	require.NoError(t, ledger1.(*kvLedger).commitToPvtAndBlockStore(blockAndPvtdata2, nil))

	// block storage should be as of block-2 but the state and history db should be as of block-1
	checkBCSummaryForTest(t, ledger1,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{
				Height:            3,
				CurrentBlockHash:  protoutil.BlockHeaderHash(blockAndPvtdata2.Block.Header),
				PreviousBlockHash: protoutil.BlockHeaderHash(blockAndPvtdata1.Block.Header),
			},

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
		nsCollBtlConfs,
		conf,
	)
	defer provider2.Close()
	ledger2, err := provider2.Open(testLedgerid)
	require.NoError(t, err)
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
	_, _, _, err = ledger2.(*kvLedger).txmgr.ValidateAndPrepare(blockAndPvtdata3, true)
	require.NoError(t, err)
	require.NoError(t, ledger2.(*kvLedger).commitToPvtAndBlockStore(blockAndPvtdata3, nil))
	// committing the transaction to state DB
	require.NoError(t, ledger2.(*kvLedger).txmgr.Commit())

	// assume that peer fails here after committing the transaction to state DB but before history DB
	checkBCSummaryForTest(t, ledger2,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{
				Height:            4,
				CurrentBlockHash:  protoutil.BlockHeaderHash(blockAndPvtdata3.Block.Header),
				PreviousBlockHash: protoutil.BlockHeaderHash(blockAndPvtdata2.Block.Header),
			},

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
		nsCollBtlConfs,
		conf,
	)
	defer provider3.Close()
	ledger3, err := provider3.Open(testLedgerid)
	require.NoError(t, err)
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

	_, _, _, err = ledger3.(*kvLedger).txmgr.ValidateAndPrepare(blockAndPvtdata4, true)
	require.NoError(t, err)
	require.NoError(t, ledger3.(*kvLedger).commitToPvtAndBlockStore(blockAndPvtdata4, nil))
	require.NoError(t, ledger3.(*kvLedger).historyDB.Commit(blockAndPvtdata4.Block))

	checkBCSummaryForTest(t, ledger3,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{
				Height:            5,
				CurrentBlockHash:  protoutil.BlockHeaderHash(blockAndPvtdata4.Block.Header),
				PreviousBlockHash: protoutil.BlockHeaderHash(blockAndPvtdata3.Block.Header),
			},

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
		nsCollBtlConfs,
		conf,
	)
	defer provider4.Close()
	ledger4, err := provider4.Open(testLedgerid)
	require.NoError(t, err)
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
	conf := testConfig(t)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()
	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	lgr, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	defer lgr.Close()

	bcInfo, _ := lgr.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	txid := util.GenerateUUID()
	simulator, _ := lgr.NewTxSimulator(txid)
	require.NoError(t, simulator.SetState("ns1", "key4", []byte("value1")))
	require.NoError(t, simulator.SetState("ns1", "key5", []byte("value2")))
	require.NoError(t, simulator.SetState("ns1", "key6", []byte("{\"shipmentID\":\"161003PKC7300\",\"customsInvoice\":{\"methodOfTransport\":\"GROUND\",\"invoiceNumber\":\"00091622\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")))
	require.NoError(t, simulator.SetState("ns1", "key7", []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"AIR MAYBE\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimBytes, _ := simRes.GetPubSimulationBytes()
	block1 := bg.NextBlock([][]byte{pubSimBytes})

	require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: block1}, &ledger.CommitOptions{}))

	bcInfo, _ = lgr.GetBlockchainInfo()
	block1Hash := protoutil.BlockHeaderHash(block1.Header)
	require.Equal(t, &common.BlockchainInfo{
		Height: 2, CurrentBlockHash: block1Hash, PreviousBlockHash: gbHash,
	}, bcInfo)

	simulationResults := [][]byte{}
	txid = util.GenerateUUID()
	simulator, _ = lgr.NewTxSimulator(txid)
	require.NoError(t, simulator.SetState("ns1", "key4", []byte("value3")))
	require.NoError(t, simulator.SetState("ns1", "key5", []byte("{\"shipmentID\":\"161003PKC7500\",\"customsInvoice\":{\"methodOfTransport\":\"AIR FREIGHT\",\"invoiceNumber\":\"00091623\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")))
	require.NoError(t, simulator.SetState("ns1", "key6", []byte("value4")))
	require.NoError(t, simulator.SetState("ns1", "key7", []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"GROUND\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")))
	require.NoError(t, simulator.SetState("ns1", "key8", []byte("{\"shipmentID\":\"161003PKC7700\",\"customsInvoice\":{\"methodOfTransport\":\"SHIP\",\"invoiceNumber\":\"00091625\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	pubSimBytes, _ = simRes.GetPubSimulationBytes()
	simulationResults = append(simulationResults, pubSimBytes)
	// add a 2nd transaction
	txid2 := util.GenerateUUID()
	simulator2, _ := lgr.NewTxSimulator(txid2)
	require.NoError(t, simulator2.SetState("ns1", "key7", []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"TRAIN\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")))
	require.NoError(t, simulator2.SetState("ns1", "key9", []byte("value5")))
	require.NoError(t, simulator2.SetState("ns1", "key10", []byte("{\"shipmentID\":\"261003PKC8000\",\"customsInvoice\":{\"methodOfTransport\":\"DONKEY\",\"invoiceNumber\":\"00091626\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")))
	simulator2.Done()
	simRes2, _ := simulator2.GetTxSimulationResults()
	pubSimBytes2, _ := simRes2.GetPubSimulationBytes()
	simulationResults = append(simulationResults, pubSimBytes2)

	block2 := bg.NextBlock(simulationResults)
	require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: block2}, &ledger.CommitOptions{}))

	bcInfo, _ = lgr.GetBlockchainInfo()
	block2Hash := protoutil.BlockHeaderHash(block2.Header)
	require.Equal(t, &common.BlockchainInfo{
		Height: 3, CurrentBlockHash: block2Hash, PreviousBlockHash: block1Hash,
	}, bcInfo)

	b0, _ := lgr.GetBlockByHash(gbHash)
	require.True(t, proto.Equal(b0, gb), "proto messages are not equal")

	b1, _ := lgr.GetBlockByHash(block1Hash)
	require.True(t, proto.Equal(b1, block1), "proto messages are not equal")

	b2, _ := lgr.GetBlockByHash(block2Hash)
	require.True(t, proto.Equal(b2, block2), "proto messages are not equal")

	b0, _ = lgr.GetBlockByNumber(0)
	require.True(t, proto.Equal(b0, gb), "proto messages are not equal")

	b1, _ = lgr.GetBlockByNumber(1)
	require.True(t, proto.Equal(b1, block1), "proto messages are not equal")

	b2, _ = lgr.GetBlockByNumber(2)
	require.True(t, proto.Equal(b2, block2), "proto messages are not equal")

	// Similar test has been pushed down to historyleveldb_test.go as well
	if conf.HistoryDBConfig.Enabled {
		logger.Debugf("History is enabled\n")
		qhistory, err := lgr.NewHistoryQueryExecutor()
		require.NoError(t, err, "Error when trying to retrieve history database executor")

		itr, err2 := qhistory.GetHistoryForKey("ns1", "key7")
		require.NoError(t, err2, "Error upon GetHistoryForKey")

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
		require.Equal(t, 3, count)
		// test the last value in the history matches the first value set for key7
		expectedValue := []byte("{\"shipmentID\":\"161003PKC7600\",\"customsInvoice\":{\"methodOfTransport\":\"AIR MAYBE\",\"invoiceNumber\":\"00091624\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")
		require.Equal(t, expectedValue, retrievedValue)

	}
}

func TestPvtDataAPIs(t *testing.T) {
	conf := testConfig(t)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	ledgerID := "testLedger"
	bg, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	lgr, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	defer lgr.Close()
	lgr.(*kvLedger).pvtdataStore.Init(btlPolicyForSampleData())

	bcInfo, _ := lgr.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	kvlgr := lgr.(*kvLedger)

	sampleData := sampleDataWithPvtdataForSelectiveTx(t, bg)
	for _, sampleDatum := range sampleData {
		require.NoError(t, kvlgr.commitToPvtAndBlockStore(sampleDatum, nil))
	}

	// block 2 has no pvt data
	pvtdata, err := lgr.GetPvtDataByNum(2, nil)
	require.NoError(t, err)
	require.Nil(t, pvtdata)

	// block 5 has no pvt data
	pvtdata, err = lgr.GetPvtDataByNum(5, nil)
	require.NoError(t, err)
	require.Equal(t, 0, len(pvtdata))

	// block 3 has pvt data for tx 3, 5 and 6. Though the tx 6
	// is marked as invalid in the block, the pvtData should
	// have been stored
	pvtdata, err = lgr.GetPvtDataByNum(3, nil)
	require.NoError(t, err)
	require.Equal(t, 3, len(pvtdata))
	require.Equal(t, uint64(3), pvtdata[0].SeqInBlock)
	require.Equal(t, uint64(5), pvtdata[1].SeqInBlock)
	require.Equal(t, uint64(6), pvtdata[2].SeqInBlock)

	// block 4 has pvt data for tx 4 and 6 only
	pvtdata, err = lgr.GetPvtDataByNum(4, nil)
	require.NoError(t, err)
	require.Equal(t, 2, len(pvtdata))
	require.Equal(t, uint64(4), pvtdata[0].SeqInBlock)
	require.Equal(t, uint64(6), pvtdata[1].SeqInBlock)

	blockAndPvtdata, err := lgr.GetPvtDataAndBlockByNum(3, nil)
	require.NoError(t, err)
	require.True(t, proto.Equal(sampleData[2].Block, blockAndPvtdata.Block))

	blockAndPvtdata, err = lgr.GetPvtDataAndBlockByNum(4, nil)
	require.NoError(t, err)
	require.True(t, proto.Equal(sampleData[3].Block, blockAndPvtdata.Block))

	// pvt data retrieval for block 3 with filter should return filtered pvtdata
	filter := ledger.NewPvtNsCollFilter()
	filter.Add("ns-1", "coll-1")
	blockAndPvtdata, err = lgr.GetPvtDataAndBlockByNum(4, filter)
	require.NoError(t, err)
	require.Equal(t, sampleData[3].Block, blockAndPvtdata.Block)
	// two transactions should be present
	require.Equal(t, 2, len(blockAndPvtdata.PvtData))
	// both tran number 4 and 6 should have only one collection because of filter
	require.Equal(t, 1, len(blockAndPvtdata.PvtData[4].WriteSet.NsPvtRwset))
	require.Equal(t, 1, len(blockAndPvtdata.PvtData[6].WriteSet.NsPvtRwset))
	// any other transaction entry should be nil
	require.Nil(t, blockAndPvtdata.PvtData[2])

	// test missing data retrieval in the presence of invalid tx. Block 7 had
	// the missing data for tx1 and tx2, and Block 6 had missing data (for tx4 and tx5).
	// Though tx5 was marked as invalid tx, both tx4 and tx5 missing data should be returned
	missingDataTracker, err := lgr.GetMissingPvtDataTracker()
	require.NoError(t, err)

	expectedMissingDataInfoBlk7 := ledger.MissingPvtDataInfo{}
	expectedMissingDataInfoBlk7.Add(7, 1, "ns-1", "coll-1")
	expectedMissingDataInfoBlk7.Add(7, 2, "ns-2", "coll-2")
	missingDataInfo, err := missingDataTracker.GetMissingPvtDataInfoForMostRecentBlocks(1)
	require.NoError(t, err)
	require.Equal(t, expectedMissingDataInfoBlk7, missingDataInfo)

	// The usage of the same missing data tracker instance should return next set of missing data now
	expectedMissingDataInfoBlk6 := make(ledger.MissingPvtDataInfo)
	expectedMissingDataInfoBlk6.Add(6, 4, "ns-4", "coll-4")
	expectedMissingDataInfoBlk6.Add(6, 5, "ns-5", "coll-5")
	missingDataInfo, err = missingDataTracker.GetMissingPvtDataInfoForMostRecentBlocks(1)
	require.NoError(t, err)
	require.Equal(t, expectedMissingDataInfoBlk6, missingDataInfo)
}

func TestCrashAfterPvtdataStoreCommit(t *testing.T) {
	conf := testConfig(t)
	ccInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	ccInfoProvider.CollectionInfoReturns(&peer.StaticCollectionConfig{BlockToLive: 0}, nil)
	provider := testutilNewProvider(conf, t, ccInfoProvider)
	defer provider.Close()

	ledgerID := "testLedger"
	bg, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	lgr, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	defer lgr.Close()

	bcInfo, _ := lgr.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	sampleData := sampleDataWithPvtdataForAllTxs(t, bg)
	dataBeforeCrash := sampleData[0:3]
	dataAtCrash := sampleData[3]

	for _, sampleDatum := range dataBeforeCrash {
		require.NoError(t, lgr.(*kvLedger).commitToPvtAndBlockStore(sampleDatum, nil))
	}
	blockNumAtCrash := dataAtCrash.Block.Header.Number
	var pvtdataAtCrash []*ledger.TxPvtData
	for _, p := range dataAtCrash.PvtData {
		pvtdataAtCrash = append(pvtdataAtCrash, p)
	}
	// call Commit on pvt data store and mimic a crash before committing the block to block store
	require.NoError(t, lgr.(*kvLedger).pvtdataStore.Commit(blockNumAtCrash, pvtdataAtCrash, nil, nil))

	// Now, assume that peer fails here before committing the block to blockstore.
	lgr.Close()
	provider.Close()

	// mimic peer restart
	provider1 := testutilNewProvider(conf, t, ccInfoProvider)
	defer provider1.Close()
	lgr1, err := provider1.Open(ledgerID)
	require.NoError(t, err)
	defer lgr1.Close()

	isPvtStoreAhead, err := lgr1.(*kvLedger).isPvtDataStoreAheadOfBlockStore()
	require.NoError(t, err)
	require.True(t, isPvtStoreAhead)

	// When starting the storage after a crash, we should be able to fetch the pvtData from pvtStore
	testVerifyPvtData(t, lgr1, blockNumAtCrash, dataAtCrash.PvtData)
	bcInfo, err = lgr.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, blockNumAtCrash, bcInfo.Height)

	// we should be able to write the last block again
	// to ensure that the pvtdataStore is not updated, we send a different pvtData for
	// the same block such that we can retrieve the pvtData and compare.
	expectedPvtData := dataAtCrash.PvtData
	dataAtCrash.PvtData = make(ledger.TxPvtDataMap)
	dataAtCrash.PvtData[0] = &ledger.TxPvtData{
		SeqInBlock: 0,
		WriteSet: &rwset.TxPvtReadWriteSet{
			NsPvtRwset: []*rwset.NsPvtReadWriteSet{
				{
					Namespace: "ns-1",
					CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
						{
							CollectionName: "coll-1",
							Rwset:          []byte("pvtdata"),
						},
					},
				},
			},
		},
	}
	require.NoError(t, lgr1.(*kvLedger).commitToPvtAndBlockStore(dataAtCrash, nil))
	testVerifyPvtData(t, lgr1, blockNumAtCrash, expectedPvtData)
	bcInfo, err = lgr1.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, blockNumAtCrash+1, bcInfo.Height)

	isPvtStoreAhead, err = lgr1.(*kvLedger).isPvtDataStoreAheadOfBlockStore()
	require.NoError(t, err)
	require.False(t, isPvtStoreAhead)
}

func testVerifyPvtData(t *testing.T, lgr ledger.PeerLedger, blockNum uint64, expectedPvtData ledger.TxPvtDataMap) {
	pvtdata, err := lgr.GetPvtDataByNum(blockNum, nil)
	require.NoError(t, err)
	constructed := constructPvtdataMap(pvtdata)
	require.Equal(t, len(expectedPvtData), len(constructed))
	for k, v := range expectedPvtData {
		ov, ok := constructed[k]
		require.True(t, ok)
		require.Equal(t, v.SeqInBlock, ov.SeqInBlock)
		require.True(t, proto.Equal(v.WriteSet, ov.WriteSet))
	}
}

func TestPvtStoreAheadOfBlockStore(t *testing.T) {
	conf := testConfig(t)
	ccInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	ccInfoProvider.CollectionInfoReturns(&peer.StaticCollectionConfig{BlockToLive: 0}, nil)
	provider := testutilNewProvider(conf, t, ccInfoProvider)
	defer provider.Close()

	ledgerID := "testLedger"
	bg, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	lgr, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	defer lgr.Close()

	bcInfo, _ := lgr.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	// when both stores contain genesis block only, isPvtstoreAheadOfBlockstore should be false
	kvlgr := lgr.(*kvLedger)
	isPvtStoreAhead, err := kvlgr.isPvtDataStoreAheadOfBlockStore()
	require.NoError(t, err)
	require.False(t, isPvtStoreAhead)

	sampleData := sampleDataWithPvtdataForSelectiveTx(t, bg)
	for _, d := range sampleData[0:9] { // commit block number 0 to 8
		require.NoError(t, kvlgr.commitToPvtAndBlockStore(d, nil))
	}

	isPvtStoreAhead, err = kvlgr.isPvtDataStoreAheadOfBlockStore()
	require.NoError(t, err)
	require.False(t, isPvtStoreAhead)

	// close and reopen.
	lgr.Close()
	provider.Close()

	provider1 := testutilNewProvider(conf, t, ccInfoProvider)
	defer provider1.Close()
	lgr1, err := provider1.Open(ledgerID)
	require.NoError(t, err)
	defer lgr1.Close()
	kvlgr = lgr1.(*kvLedger)

	// as both stores are at the same block height, isPvtstoreAheadOfBlockstore should be false
	info, err := lgr1.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, uint64(10), info.Height)
	pvtStoreHt, err := kvlgr.pvtdataStore.LastCommittedBlockHeight()
	require.NoError(t, err)
	require.Equal(t, uint64(10), pvtStoreHt)
	isPvtStoreAhead, err = kvlgr.isPvtDataStoreAheadOfBlockStore()
	require.NoError(t, err)
	require.False(t, isPvtStoreAhead)

	lastBlkAndPvtData := sampleData[9]
	// Add the last block directly to the pvtdataStore but not to blockstore. This would make
	// the pvtdatastore height greater than the block store height.
	validTxPvtData, validTxMissingPvtData := constructPvtDataAndMissingData(lastBlkAndPvtData)
	err = kvlgr.pvtdataStore.Commit(lastBlkAndPvtData.Block.Header.Number, validTxPvtData, validTxMissingPvtData, nil)
	require.NoError(t, err)

	// close and reopen.
	lgr1.Close()
	provider1.Close()

	provider2 := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider2.Close()
	lgr2, err := provider2.Open(ledgerID)
	require.NoError(t, err)
	defer lgr2.Close()
	kvlgr = lgr2.(*kvLedger)

	// pvtdataStore should be ahead of blockstore
	info, err = lgr2.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, uint64(10), info.Height)
	pvtStoreHt, err = kvlgr.pvtdataStore.LastCommittedBlockHeight()
	require.NoError(t, err)
	require.Equal(t, uint64(11), pvtStoreHt)
	isPvtStoreAhead, err = kvlgr.isPvtDataStoreAheadOfBlockStore()
	require.NoError(t, err)
	require.True(t, isPvtStoreAhead)

	// bring the height of BlockStore equal to pvtdataStore
	require.NoError(t, kvlgr.commitToPvtAndBlockStore(lastBlkAndPvtData, nil))
	info, err = lgr2.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, uint64(11), info.Height)
	pvtStoreHt, err = kvlgr.pvtdataStore.LastCommittedBlockHeight()
	require.NoError(t, err)
	require.Equal(t, uint64(11), pvtStoreHt)
	isPvtStoreAhead, err = kvlgr.isPvtDataStoreAheadOfBlockStore()
	require.NoError(t, err)
	require.False(t, isPvtStoreAhead)
}

func TestCommitToPvtAndBlockstoreError(t *testing.T) {
	conf := testConfig(t)
	ccInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	ccInfoProvider.CollectionInfoReturns(&peer.StaticCollectionConfig{BlockToLive: 0}, nil)
	provider1 := testutilNewProvider(conf, t, ccInfoProvider)
	defer provider1.Close()

	ledgerID := "testLedger"
	bg, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	lgr1, err := provider1.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	defer lgr1.Close()

	bcInfo, _ := lgr1.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	kvlgr := lgr1.(*kvLedger)
	sampleData := sampleDataWithPvtdataForSelectiveTx(t, bg)
	for _, d := range sampleData[0:9] { // commit block number 1 to 9
		require.NoError(t, kvlgr.commitToPvtAndBlockStore(d, nil))
	}

	// try to write the last block again. The function should return an
	// error from the private data store.
	err = kvlgr.commitToPvtAndBlockStore(sampleData[8], nil) // block 9
	require.EqualError(t, err, "expected block number=10, received block number=9")

	lastBlkAndPvtData := sampleData[9] // block 10
	// Add the block directly to blockstore
	require.NoError(t, kvlgr.blockStore.AddBlock(lastBlkAndPvtData.Block))
	// Adding the same block should cause passing on the error caused by the block storgae
	err = kvlgr.commitToPvtAndBlockStore(lastBlkAndPvtData, nil)
	require.EqualError(t, err, "block number should have been 11 but was 10")
	// At the end, the pvt store status should be changed
	pvtStoreCommitHt, err := kvlgr.pvtdataStore.LastCommittedBlockHeight()
	require.NoError(t, err)
	require.Equal(t, uint64(11), pvtStoreCommitHt)
}

func TestCollectionConfigHistoryRetriever(t *testing.T) {
	var cleanup func()
	ledgerID := "testLedger"
	chaincodeName := "testChaincode"

	var provider *Provider
	var mockDeployedCCInfoProvider *mock.DeployedChaincodeInfoProvider
	var lgr ledger.PeerLedger

	init := func() {
		var err error
		conf := testConfig(t)
		mockDeployedCCInfoProvider = &mock.DeployedChaincodeInfoProvider{}
		provider = testutilNewProvider(conf, t, mockDeployedCCInfoProvider)
		ledgerID := "testLedger"
		_, gb := testutil.NewBlockGenerator(t, ledgerID, false)
		lgr, err = provider.CreateFromGenesisBlock(gb)
		require.NoError(t, err)
		cleanup = func() {
			lgr.Close()
			provider.Close()
		}
	}

	testcases := []struct {
		name                        string
		implicitCollConfigs         []*peer.StaticCollectionConfig
		explicitCollConfigs         *peer.CollectionConfigPackage
		explicitCollConfigsBlockNum uint64
		expectedOutput              *ledger.CollectionConfigInfo
	}{
		{
			name: "both-implicit-and-explicit-coll-configs-exist",
			implicitCollConfigs: []*peer.StaticCollectionConfig{
				{
					Name: "implicit-coll",
				},
			},
			explicitCollConfigs: testutilCollConfigPkg(
				[]*peer.StaticCollectionConfig{
					{
						Name: "explicit-coll",
					},
				},
			),
			explicitCollConfigsBlockNum: 25,

			expectedOutput: &ledger.CollectionConfigInfo{
				CollectionConfig: testutilCollConfigPkg(
					[]*peer.StaticCollectionConfig{
						{
							Name: "explicit-coll",
						},
						{
							Name: "implicit-coll",
						},
					},
				),
				CommittingBlockNum: 25,
			},
		},

		{
			name: "only-implicit-coll-configs-exist",
			implicitCollConfigs: []*peer.StaticCollectionConfig{
				{
					Name: "implicit-coll",
				},
			},
			explicitCollConfigs: nil,
			expectedOutput: &ledger.CollectionConfigInfo{
				CollectionConfig: testutilCollConfigPkg(
					[]*peer.StaticCollectionConfig{
						{
							Name: "implicit-coll",
						},
					},
				),
			},
		},

		{
			name: "only-explicit-coll-configs-exist",
			explicitCollConfigs: testutilCollConfigPkg(
				[]*peer.StaticCollectionConfig{
					{
						Name: "explicit-coll",
					},
				},
			),
			explicitCollConfigsBlockNum: 25,
			expectedOutput: &ledger.CollectionConfigInfo{
				CollectionConfig: testutilCollConfigPkg(
					[]*peer.StaticCollectionConfig{
						{
							Name: "explicit-coll",
						},
					},
				),
				CommittingBlockNum: 25,
			},
		},

		{
			name:           "no-coll-configs-exist",
			expectedOutput: nil,
		},
	}

	for _, testcase := range testcases {
		t.Run(
			testcase.name,
			func(t *testing.T) {
				init()
				defer cleanup()
				// setup mock for implicit collections
				mockDeployedCCInfoProvider.ImplicitCollectionsReturns(testcase.implicitCollConfigs, nil)
				// setup mock so that it causes persisting the explicit collections in collection config history mgr
				if testcase.explicitCollConfigs != nil {
					testutilPersistExplicitCollectionConfig(
						t,
						provider,
						mockDeployedCCInfoProvider,
						ledgerID,
						chaincodeName,
						testcase.explicitCollConfigs,
						testcase.explicitCollConfigsBlockNum,
					)
				}

				r, err := lgr.GetConfigHistoryRetriever()
				require.NoError(t, err)

				actualOutput, err := r.MostRecentCollectionConfigBelow(testcase.explicitCollConfigsBlockNum+1, chaincodeName)
				require.NoError(t, err)
				require.Equal(t, testcase.expectedOutput, actualOutput)
			},
		)
	}

	t.Run("implicit-collection-retrieval-causes-error", func(t *testing.T) {
		init()
		defer cleanup()

		mockDeployedCCInfoProvider.ImplicitCollectionsReturns(nil, errors.New("cannot-serve-implicit-collections"))
		r, err := lgr.GetConfigHistoryRetriever()
		require.NoError(t, err)

		_, err = r.MostRecentCollectionConfigBelow(50, chaincodeName)
		require.EqualError(t, err, "error while retrieving implicit collections: cannot-serve-implicit-collections")
	})

	t.Run("explicit-collection-retrieval-causes-error", func(t *testing.T) {
		init()
		defer cleanup()
		provider.configHistoryMgr.Close()

		r, err := lgr.GetConfigHistoryRetriever()
		require.NoError(t, err)

		_, err = r.MostRecentCollectionConfigBelow(50, chaincodeName)
		require.Contains(t, err.Error(), "error while retrieving explicit collections")
	})
}

func TestCommitNotifications(t *testing.T) {
	var lgr *kvLedger
	var doneChannel chan struct{}
	var dataChannel <-chan *ledger.CommitNotification

	setup := func() {
		var err error
		lgr = &kvLedger{}
		doneChannel = make(chan struct{})
		dataChannel, err = lgr.CommitNotificationsChannel(doneChannel)
		require.NoError(t, err)
	}

	t.Run("only first txid is included in notification", func(t *testing.T) {
		setup()
		lgr.sendCommitNotification(1, []*validation.TxStatInfo{
			{
				TxIDFromChannelHeader: "txid_1",
				ValidationCode:        peer.TxValidationCode_BAD_RWSET,
				ChaincodeID:           &peer.ChaincodeID{Name: "cc1"},
				ChaincodeEventData:    []byte("cc1_event"),
				TxType:                common.HeaderType_ENDORSER_TRANSACTION,
			},
			{
				TxIDFromChannelHeader: "txid_1",
				ValidationCode:        peer.TxValidationCode_DUPLICATE_TXID,
			},
			{
				TxIDFromChannelHeader: "txid_2",
				ValidationCode:        peer.TxValidationCode_VALID,
				ChaincodeID:           &peer.ChaincodeID{Name: "cc2"},
				ChaincodeEventData:    []byte("cc2_event"),
				TxType:                common.HeaderType_ENDORSER_TRANSACTION,
			},
		})

		commitNotification := <-dataChannel
		require.Equal(t,
			&ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:               "txid_1",
						TxType:             common.HeaderType_ENDORSER_TRANSACTION,
						ValidationCode:     peer.TxValidationCode_BAD_RWSET,
						ChaincodeID:        &peer.ChaincodeID{Name: "cc1"},
						ChaincodeEventData: []byte("cc1_event"),
					},
					{
						TxID:               "txid_2",
						TxType:             common.HeaderType_ENDORSER_TRANSACTION,
						ValidationCode:     peer.TxValidationCode_VALID,
						ChaincodeID:        &peer.ChaincodeID{Name: "cc2"},
						ChaincodeEventData: []byte("cc2_event"),
					},
				},
			},
			commitNotification,
		)
	})

	t.Run("empty txids are not included in notification", func(t *testing.T) {
		setup()
		lgr.sendCommitNotification(1, []*validation.TxStatInfo{
			{
				TxIDFromChannelHeader: "",
				ValidationCode:        peer.TxValidationCode_BAD_RWSET,
			},
			{
				TxIDFromChannelHeader: "",
				ValidationCode:        peer.TxValidationCode_DUPLICATE_TXID,
			},
		})

		commitNotification := <-dataChannel
		require.Equal(t,
			&ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo:     []*ledger.CommitNotificationTxInfo{},
			},
			commitNotification,
		)
	})

	t.Run("second time calling CommitNotificationsChannel returns error", func(t *testing.T) {
		setup()
		_, err := lgr.CommitNotificationsChannel(make(chan struct{}))
		require.EqualError(t, err, "only one commit notifications channel is allowed at a time")
	})

	t.Run("closing done channel closes the data channel on next commit", func(t *testing.T) {
		setup()
		lgr.sendCommitNotification(1, []*validation.TxStatInfo{})
		_, ok := <-dataChannel
		require.True(t, ok)

		close(doneChannel)
		lgr.sendCommitNotification(2, []*validation.TxStatInfo{})

		_, ok = <-dataChannel
		require.False(t, ok)
	})
}

func TestCommitNotificationsOnBlockCommit(t *testing.T) {
	conf := testConfig(t)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	_, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	l, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	defer l.Close()

	lgr := l.(*kvLedger)
	doneChannel := make(chan struct{})
	dataChannel, err := lgr.CommitNotificationsChannel(doneChannel)
	require.NoError(t, err)

	s, err := lgr.NewTxSimulator("")
	require.NoError(t, err)
	_, err = s.GetState("ns1", "key1")
	require.NoError(t, err)
	require.NoError(t, s.SetState("ns1", "key1", []byte("val1")))
	sr, err := s.GetTxSimulationResults()
	require.NoError(t, err)
	srBytes, err := sr.GetPubSimulationBytes()
	require.NoError(t, err)
	s.Done()

	block := testutil.ConstructBlockFromBlockDetails(
		t, &testutil.BlockDetails{
			BlockNum:     1,
			PreviousHash: protoutil.BlockHeaderHash(gb.Header),
			Txs: []*testutil.TxDetails{
				{
					Type:              common.HeaderType_ENDORSER_TRANSACTION,
					TxID:              "txid_1",
					ChaincodeName:     "foo",
					ChaincodeVersion:  "v1",
					SimulationResults: srBytes,
					ChaincodeEvents:   []byte("foo-event"),
				},

				{
					Type:              common.HeaderType_ENDORSER_TRANSACTION,
					TxID:              "txid_2",
					ChaincodeName:     "bar",
					ChaincodeVersion:  "v2",
					SimulationResults: srBytes, // same read-sets: should cause mvcc conflict
					ChaincodeEvents:   []byte("bar-event"),
				},
			},
		}, false,
	)

	require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: block}, &ledger.CommitOptions{}))
	commitNotification := <-dataChannel
	require.Equal(t,
		&ledger.CommitNotification{
			BlockNumber: 1,
			TxsInfo: []*ledger.CommitNotificationTxInfo{
				{
					TxType:             common.HeaderType_ENDORSER_TRANSACTION,
					TxID:               "txid_1",
					ValidationCode:     peer.TxValidationCode_VALID,
					ChaincodeID:        &peer.ChaincodeID{Name: "foo", Version: "v1"},
					ChaincodeEventData: []byte("foo-event"),
				},
				{
					TxType:             common.HeaderType_ENDORSER_TRANSACTION,
					TxID:               "txid_2",
					ValidationCode:     peer.TxValidationCode_MVCC_READ_CONFLICT,
					ChaincodeID:        &peer.ChaincodeID{Name: "bar", Version: "v2"},
					ChaincodeEventData: []byte("bar-event"),
				},
			},
		},
		commitNotification,
	)
}

func testutilPersistExplicitCollectionConfig(
	t *testing.T,
	provider *Provider,
	mockDeployedCCInfoProvider *mock.DeployedChaincodeInfoProvider,
	ledgerID string,
	chaincodeName string,
	collConfigPkg *peer.CollectionConfigPackage,
	committingBlockNum uint64,
) {
	mockDeployedCCInfoProvider.UpdatedChaincodesReturns(
		[]*ledger.ChaincodeLifecycleInfo{
			{
				Name: chaincodeName,
			},
		},
		nil,
	)
	mockDeployedCCInfoProvider.ChaincodeInfoReturns(
		&ledger.DeployedChaincodeInfo{
			Name:                        chaincodeName,
			ExplicitCollectionConfigPkg: collConfigPkg,
		},
		nil,
	)
	err := provider.configHistoryMgr.HandleStateUpdates(
		&ledger.StateUpdateTrigger{
			LedgerID:           ledgerID,
			CommittingBlockNum: committingBlockNum,
		},
	)
	require.NoError(t, err)
}

func testutilCollConfigPkg(colls []*peer.StaticCollectionConfig) *peer.CollectionConfigPackage {
	if len(colls) == 0 {
		return nil
	}
	pkg := &peer.CollectionConfigPackage{
		Config: []*peer.CollectionConfig{},
	}
	for _, coll := range colls {
		pkg.Config = append(pkg.Config,
			&peer.CollectionConfig{
				Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: coll,
				},
			},
		)
	}
	return pkg
}

func sampleDataWithPvtdataForSelectiveTx(t *testing.T, bg *testutil.BlockGenerator) []*ledger.BlockAndPvtData {
	var blockAndpvtdata []*ledger.BlockAndPvtData
	blocks := bg.NextTestBlocks(10)
	for i := 0; i < 10; i++ {
		blockAndpvtdata = append(blockAndpvtdata, &ledger.BlockAndPvtData{Block: blocks[i]})
	}

	// txNum 3, 5, 6 in block 2 has pvtdata but txNum 6 is invalid
	blockAndpvtdata[2].PvtData = samplePvtData(t, []uint64{3, 5, 6})
	txFilter := txflags.ValidationFlags(blockAndpvtdata[2].Block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	txFilter.SetFlag(6, peer.TxValidationCode_INVALID_WRITESET)
	blockAndpvtdata[2].Block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txFilter

	// txNum 4, 6 in block 3 has pvtdata
	blockAndpvtdata[3].PvtData = samplePvtData(t, []uint64{4, 6})

	// txNum 4, 5 in block 5 has missing pvt data but txNum 5 is invalid
	missingData := make(ledger.TxMissingPvtData)
	missingData.Add(4, "ns-4", "coll-4", true)
	missingData.Add(5, "ns-5", "coll-5", true)
	blockAndpvtdata[5].MissingPvtData = missingData
	txFilter = txflags.ValidationFlags(blockAndpvtdata[5].Block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	txFilter.SetFlag(5, peer.TxValidationCode_INVALID_WRITESET)
	blockAndpvtdata[5].Block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txFilter

	// add missing data for txNum 1, and 2 in block 6
	missingData = ledger.TxMissingPvtData{}
	missingData.Add(1, "ns-1", "coll-1", true)
	missingData.Add(2, "ns-2", "coll-2", true)
	blockAndpvtdata[6].MissingPvtData = missingData

	return blockAndpvtdata
}

func sampleDataWithPvtdataForAllTxs(t *testing.T, bg *testutil.BlockGenerator) []*ledger.BlockAndPvtData {
	var blockAndpvtdata []*ledger.BlockAndPvtData
	blocks := bg.NextTestBlocks(10)
	for i := 0; i < 10; i++ {
		blockAndpvtdata = append(blockAndpvtdata,
			&ledger.BlockAndPvtData{
				Block:   blocks[i],
				PvtData: samplePvtData(t, []uint64{uint64(i), uint64(i + 1)}),
			},
		)
	}
	return blockAndpvtdata
}

func samplePvtData(t *testing.T, txNums []uint64) map[uint64]*ledger.TxPvtData {
	txPvtWS := &rwsetutil.TxPvtRwSet{
		NsPvtRwSet: []*rwsetutil.NsPvtRwSet{
			{
				NameSpace: "ns-1",
				CollPvtRwSets: []*rwsetutil.CollPvtRwSet{
					{
						CollectionName: "coll-1",
						KvRwSet: &kvrwset.KVRWSet{
							Writes: []*kvrwset.KVWrite{
								{
									Key:   "testKey",
									Value: []byte("testValue"),
								},
							},
						},
					},
					{
						CollectionName: "coll-2",
						KvRwSet: &kvrwset.KVRWSet{
							Writes: []*kvrwset.KVWrite{
								{
									Key:   "testKey",
									Value: []byte("testValue"),
								},
							},
						},
					},
				},
			},
		},
	}

	pvtWriteSet, err := txPvtWS.ToProtoMsg()
	require.NoError(t, err)

	var pvtData []*ledger.TxPvtData
	for _, txNum := range txNums {
		pvtData = append(pvtData, &ledger.TxPvtData{SeqInBlock: txNum, WriteSet: pvtWriteSet})
	}
	return constructPvtdataMap(pvtData)
}

func btlPolicyForSampleData() pvtdatapolicy.BTLPolicy {
	return btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 0,
			{"ns-1", "coll-2"}: 0,
		},
	)
}

func prepareNextBlockForTest(t *testing.T, l ledger.PeerLedger, bg *testutil.BlockGenerator,
	txid string, pubKVs map[string]string, pvtKVs map[string]string) *ledger.BlockAndPvtData {
	simulator, _ := l.NewTxSimulator(txid)
	// simulating transaction
	for k, v := range pubKVs {
		require.NoError(t, simulator.SetState("ns", k, []byte(v)))
	}
	for k, v := range pvtKVs {
		require.NoError(t, simulator.SetPrivateData("ns", "coll", k, []byte(v)))
	}
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimBytes, _ := simRes.GetPubSimulationBytes()
	block := bg.NextBlock([][]byte{pubSimBytes})
	blkAndPvtData := &ledger.BlockAndPvtData{Block: block}
	if len(pvtKVs) != 0 {
		blkAndPvtData.PvtData = ledger.TxPvtDataMap{
			0: {SeqInBlock: 0, WriteSet: simRes.PvtSimulationResults},
		}
	}
	return blkAndPvtData
}

func checkBCSummaryForTest(t *testing.T, l ledger.PeerLedger, expectedBCSummary *bcSummary) {
	if expectedBCSummary.bcInfo != nil {
		actualBCInfo, _ := l.GetBlockchainInfo()
		require.Equal(t, expectedBCSummary.bcInfo, actualBCInfo)
	}

	if expectedBCSummary.stateDBSavePoint != 0 {
		actualStateDBSavepoint, _ := l.(*kvLedger).txmgr.GetLastSavepoint()
		require.Equal(t, expectedBCSummary.stateDBSavePoint, actualStateDBSavepoint.BlockNum)
	}

	if !(expectedBCSummary.stateDBKVs == nil && expectedBCSummary.stateDBPvtKVs == nil) {
		checkStateDBForTest(t, l, expectedBCSummary.stateDBKVs, expectedBCSummary.stateDBPvtKVs)
	}

	if expectedBCSummary.historyDBSavePoint != 0 {
		actualHistoryDBSavepoint, _ := l.(*kvLedger).historyDB.GetLastSavepoint()
		require.Equal(t, expectedBCSummary.historyDBSavePoint, actualHistoryDBSavepoint.BlockNum)
	}

	if expectedBCSummary.historyKey != "" {
		checkHistoryDBForTest(t, l, expectedBCSummary.historyKey, expectedBCSummary.historyVals)
	}
}

func checkStateDBForTest(t *testing.T, l ledger.PeerLedger, expectedKVs map[string]string, expectedPvtKVs map[string]string) {
	simulator, _ := l.NewTxSimulator("checkStateDBForTest")
	defer simulator.Done()
	for expectedKey, expectedVal := range expectedKVs {
		actualVal, _ := simulator.GetState("ns", expectedKey)
		require.Equal(t, []byte(expectedVal), actualVal)
	}

	for expectedPvtKey, expectedPvtVal := range expectedPvtKVs {
		actualPvtVal, _ := simulator.GetPrivateData("ns", "coll", expectedPvtKey)
		require.Equal(t, []byte(expectedPvtVal), actualPvtVal)
	}
}

func checkHistoryDBForTest(t *testing.T, l ledger.PeerLedger, key string, expectedVals []string) {
	qhistory, _ := l.NewHistoryQueryExecutor()
	itr, _ := qhistory.GetHistoryForKey("ns", key)
	var actualVals []string
	for {
		kmod, err := itr.Next()
		require.NoError(t, err, "Error upon Next()")
		if kmod == nil {
			break
		}
		retrievedValue := kmod.(*queryresult.KeyModification).Value
		actualVals = append(actualVals, string(retrievedValue))
	}
	require.Equal(t, expectedVals, actualVals)
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
