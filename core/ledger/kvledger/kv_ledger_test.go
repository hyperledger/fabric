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
	lgr "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
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
	flogging.ActivateSpec("lockbasedtxmgr,statevalidator,valimpl,confighistory,pvtstatepurgemgmt=debug")
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
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block1}, &lgr.CommitOptions{})

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
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block2}, &lgr.CommitOptions{})

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

func TestAddCommitHash(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	provider := testutilNewProvider(t)
	defer provider.Close()

	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	gbHash := gb.Header.Hash()
	ledger, _ := provider.Create(gb)
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
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block1}, &lgr.CommitOptions{})

	commitHash, err = ledger.(*kvLedger).lastPersistedCommitHash()
	assert.NoError(t, err)
	assert.Equal(t, commitHash, ledger.(*kvLedger).commitHash)
	assert.Equal(t, len(commitHash), 32)

	// if the kvledger.commitHash is nil and the block number is > 1, the
	// commitHash should not be added to the block
	block2 := bg.NextBlock([][]byte{pubSimBytes})
	ledger.(*kvLedger).commitHash = nil
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block2}, &lgr.CommitOptions{})

	commitHash, err = ledger.(*kvLedger).lastPersistedCommitHash()
	assert.NoError(t, err)
	assert.Equal(t, commitHash, ledger.(*kvLedger).commitHash)
	assert.Equal(t, len(commitHash), 0)

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
	assert.NoError(t, ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block1}, &lgr.CommitOptions{}))

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
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block2}, &lgr.CommitOptions{})

	bcInfo, _ = ledger.GetBlockchainInfo()
	block2Hash := block2.Header.Hash()
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
	testSyncStateAndHistoryDBWithBlockstore(t)
	testSyncStateDBWithPvtdatastore(t)
}

func testSyncStateAndHistoryDBWithBlockstore(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	provider := testutilNewProviderWithCollectionConfig(t,
		"ns", map[string]uint64{"coll": 0},
	)
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

	// creating and committing the second data block
	blockAndPvtdata1 := prepareNextBlockForTest(t, ledger, bg, "SimulateForBlk1",
		map[string]string{"key1": "value1.1", "key2": "value2.1", "key3": "value3.1"},
		map[string]string{"key1": "pvtValue1.1", "key2": "pvtValue2.1", "key3": "pvtValue3.1"})
	assert.NoError(t, ledger.CommitWithPvtData(blockAndPvtdata1, &lgr.CommitOptions{}))
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 2,
				CurrentBlockHash:  blockAndPvtdata1.Block.Header.Hash(),
				PreviousBlockHash: gbHash},
		},
	)

	//======================================================================================
	// SCENARIO 1: peer writes the second block to the block storage and fails
	// before committing the block to state DB and history DB
	//======================================================================================
	blockAndPvtdata2 := prepareNextBlockForTest(t, ledger, bg, "SimulateForBlk2",
		map[string]string{"key1": "value1.2", "key2": "value2.2", "key3": "value3.2"},
		map[string]string{"key1": "pvtValue1.2", "key2": "pvtValue2.2", "key3": "pvtValue3.2"})

	_, _, err := ledger.(*kvLedger).txtmgmt.ValidateAndPrepare(blockAndPvtdata2, true)
	assert.NoError(t, err)
	assert.NoError(t, ledger.(*kvLedger).blockStore.CommitWithPvtData(blockAndPvtdata2))

	// block storage should be as of block-2 but the state and history db should be as of block-1
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 3,
				CurrentBlockHash:  blockAndPvtdata2.Block.Header.Hash(),
				PreviousBlockHash: blockAndPvtdata1.Block.Header.Hash()},

			stateDBSavePoint: uint64(1),
			stateDBKVs:       map[string]string{"key1": "value1.1", "key2": "value2.1", "key3": "value3.1"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.1", "key2": "pvtValue2.1", "key3": "pvtValue3.1"},

			historyDBSavePoint: uint64(1),
			historyKey:         "key1",
			historyVals:        []string{"value1.1"},
		},
	)
	// Now, assume that peer fails here before committing the transaction to the statedb and historydb
	ledger.Close()
	provider.Close()

	// Here the peer comes online and calls NewKVLedger to get a handler for the ledger
	// StateDB and HistoryDB should be recovered before returning from NewKVLedger call
	provider = testutilNewProviderWithCollectionConfig(t,
		"ns", map[string]uint64{"coll": 0},
	)
	ledger, _ = provider.Open(testLedgerid)
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			stateDBSavePoint: uint64(2),
			stateDBKVs:       map[string]string{"key1": "value1.2", "key2": "value2.2", "key3": "value3.2"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.2", "key2": "pvtValue2.2", "key3": "pvtValue3.2"},

			historyDBSavePoint: uint64(2),
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
	_, _, err = ledger.(*kvLedger).txtmgmt.ValidateAndPrepare(blockAndPvtdata3, true)
	assert.NoError(t, err)
	assert.NoError(t, ledger.(*kvLedger).blockStore.CommitWithPvtData(blockAndPvtdata3))
	// committing the transaction to state DB
	assert.NoError(t, ledger.(*kvLedger).txtmgmt.Commit())

	// assume that peer fails here after committing the transaction to state DB but before history DB
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 4,
				CurrentBlockHash:  blockAndPvtdata3.Block.Header.Hash(),
				PreviousBlockHash: blockAndPvtdata2.Block.Header.Hash()},

			stateDBSavePoint: uint64(3),
			stateDBKVs:       map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"},

			historyDBSavePoint: uint64(2),
			historyKey:         "key1",
			historyVals:        []string{"value1.1", "value1.2"},
		},
	)
	ledger.Close()
	provider.Close()

	// we assume here that the peer comes online and calls NewKVLedger to get a handler for the ledger
	// history DB should be recovered before returning from NewKVLedger call
	provider = testutilNewProviderWithCollectionConfig(t,
		"ns", map[string]uint64{"coll": 0},
	)
	ledger, _ = provider.Open(testLedgerid)

	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			stateDBSavePoint: uint64(3),
			stateDBKVs:       map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"},

			historyDBSavePoint: uint64(3),
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

	_, _, err = ledger.(*kvLedger).txtmgmt.ValidateAndPrepare(blockAndPvtdata4, true)
	assert.NoError(t, err)
	assert.NoError(t, ledger.(*kvLedger).blockStore.CommitWithPvtData(blockAndPvtdata4))
	assert.NoError(t, ledger.(*kvLedger).historyDB.Commit(blockAndPvtdata4.Block))

	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			bcInfo: &common.BlockchainInfo{Height: 5,
				CurrentBlockHash:  blockAndPvtdata4.Block.Header.Hash(),
				PreviousBlockHash: blockAndPvtdata3.Block.Header.Hash()},

			stateDBSavePoint: uint64(3),
			stateDBKVs:       map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"},

			historyDBSavePoint: uint64(4),
			historyKey:         "key1",
			historyVals:        []string{"value1.1", "value1.2", "value1.3", "value1.4"},
		},
	)
	ledger.Close()
	provider.Close()

	// we assume here that the peer comes online and calls NewKVLedger to get a handler for the ledger
	// state DB should be recovered before returning from NewKVLedger call
	provider = testutilNewProviderWithCollectionConfig(t,
		"ns", map[string]uint64{"coll": 0},
	)
	ledger, _ = provider.Open(testLedgerid)
	checkBCSummaryForTest(t, ledger,
		&bcSummary{
			stateDBSavePoint: uint64(4),
			stateDBKVs:       map[string]string{"key1": "value1.4", "key2": "value2.4", "key3": "value3.4"},
			stateDBPvtKVs:    map[string]string{"key1": "pvtValue1.4", "key2": "pvtValue2.4", "key3": "pvtValue3.4"},

			historyDBSavePoint: uint64(4),
			historyKey:         "key1",
			historyVals:        []string{"value1.1", "value1.2", "value1.3", "value1.4"},
		},
	)
}

func testSyncStateDBWithPvtdatastore(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	provider := testutilNewProviderWithCollectionConfig(t,
		"ns", map[string]uint64{"coll": 0},
	)
	testLedgerid := "testLedger"
	bg, gb := testutil.NewBlockGenerator(t, testLedgerid, false)
	ledger, _ := provider.Create(gb)

	// create and commit two data block (both with missing pvtdata)
	blockAndPvtdata1, pvtdata1 := prepareNextBlockWithMissingPvtDataForTest(t, ledger, bg, "SimulateForBlk1",
		map[string]string{"key1": "value1.1", "key2": "value2.1", "key3": "value3.1"},
		map[string]string{"key1": "pvtValue1.1", "key2": "pvtValue2.1", "key3": "pvtValue3.1"})

	assert.NoError(t, ledger.CommitWithPvtData(blockAndPvtdata1, &lgr.CommitOptions{}))

	blockAndPvtdata2, pvtdata2 := prepareNextBlockWithMissingPvtDataForTest(t, ledger, bg, "SimulateForBlk2",
		map[string]string{"key1": "value1.2", "key2": "value2.2", "key3": "value3.2"},
		map[string]string{"key1": "pvtValue1.2", "key2": "pvtValue2.2", "key3": "pvtValue3.2"})

	assert.NoError(t, ledger.CommitWithPvtData(blockAndPvtdata2, &lgr.CommitOptions{}))

	blockAndPvtdata3, pvtdata3 := prepareNextBlockWithMissingPvtDataForTest(t, ledger, bg, "SimulateForBlk3",
		map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
		map[string]string{"key1": "pvtValue1.3", "key2": "pvtValue2.3", "key3": "pvtValue3.3"})

	// make a transaction as invalid though it is valid -- to test whether the pvtdata of invalid
	// transactions are being stored.
	blockAndPvtdata3.Block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER][0] = byte(peer.TxValidationCode_MVCC_READ_CONFLICT)
	assert.NoError(t, ledger.CommitWithPvtData(blockAndPvtdata3, &lgr.CommitOptions{}))

	blockAndPvtdata4, pvtdata4 := prepareNextBlockWithMissingPvtDataForTest(t, ledger, bg, "SimulateForBlk4",
		map[string]string{"key4": "value4"},
		map[string]string{"key4": "pvtValue4"})

	assert.NoError(t, ledger.CommitWithPvtData(blockAndPvtdata4, &lgr.CommitOptions{}))

	blockAndPvtdata5, pvtdata5 := prepareNextBlockWithMissingPvtDataForTest(t, ledger, bg, "SimulateForBlk5",
		map[string]string{"key5": "value5"},
		map[string]string{"key5": "pvtValue5"})

	assert.NoError(t, ledger.CommitWithPvtData(blockAndPvtdata5, &lgr.CommitOptions{}))

	txSim, err := ledger.NewTxSimulator("test")
	assert.NoError(t, err)
	value, err := txSim.GetPrivateData("ns", "coll", "key1")
	_, ok := err.(*txmgr.ErrPvtdataNotAvailable)
	assert.True(t, ok)
	assert.Nil(t, value)

	value, err = txSim.GetPrivateData("ns", "coll", "key4")
	_, ok = err.(*txmgr.ErrPvtdataNotAvailable)
	assert.True(t, ok)
	assert.Nil(t, value)

	value, err = txSim.GetPrivateData("ns", "coll", "key5")
	_, ok = err.(*txmgr.ErrPvtdataNotAvailable)
	assert.True(t, ok)
	assert.Nil(t, value)

	blocksPvtData := map[uint64][]*lgr.TxPvtData{
		1: {
			pvtdata1,
		},
		2: {
			pvtdata2,
		},
		3: {
			pvtdata3,
		},
		4: {
			pvtdata4,
		},
		5: {
			pvtdata5,
		},
	}

	assert.NoError(t, ledger.(*kvLedger).blockStore.CommitPvtDataOfOldBlocks(blocksPvtData))
	// ensure that the pvtdata of the invalid transaction in block 3 got stored in the
	// pvtdataStore
	pvtdata, _ := ledger.GetPvtDataByNum(3, nil)
	assert.NotNil(t, pvtdata)
	assert.Equal(t, 1, len(pvtdata))
	assert.True(t, pvtdata[0].Has("ns", "coll"))

	// Now, assume that peer fails here before committing the pvtData to stateDB
	ledger.Close()
	provider.Close()

	// Rollback to block 3
	RollbackKVLedger(testLedgerid, 3)

	// Here the peer comes online and calls NewKVLedger to get a handler for the ledger
	// StateDB and HistoryDB should be recovered before returning from NewKVLedger call
	provider = testutilNewProviderWithCollectionConfig(t,
		"ns", map[string]uint64{"coll": 0},
	)
	defer provider.Close()

	ledger, err = provider.Open(testLedgerid)
	assert.NoError(t, err)
	defer ledger.Close()

	txSim, err = ledger.NewTxSimulator("test")
	assert.NoError(t, err)
	value, err = txSim.GetPrivateData("ns", "coll", "key1")
	assert.NoError(t, err)
	// the value should match the string provided in the block 2 (i.e., pvtValue1.2)
	// rather than block 3 (i.e., pvtValue1.3)
	assert.Equal(t, value, []byte("pvtValue1.2"))

	// block 4 is not committed yet
	value, err = txSim.GetPrivateData("ns", "coll", "key4")
	assert.Nil(t, err)
	assert.Nil(t, value)

	// block 5 is not committed yet
	value, err = txSim.GetPrivateData("ns", "coll", "key5")
	assert.Nil(t, err)
	assert.Nil(t, value)
	txSim.Done()

	// recommit block 4 & 5
	assert.NoError(t, ledger.CommitWithPvtData(blockAndPvtdata4, &lgr.CommitOptions{FetchPvtDataFromLedger: true}))
	assert.NoError(t, ledger.CommitWithPvtData(blockAndPvtdata5, &lgr.CommitOptions{FetchPvtDataFromLedger: true}))

	txSim, err = ledger.NewTxSimulator("test")
	assert.NoError(t, err)
	value, err = txSim.GetPrivateData("ns", "coll", "key4")
	assert.NoError(t, err)
	assert.Equal(t, value, []byte("pvtValue4"))

	value, err = txSim.GetPrivateData("ns", "coll", "key5")
	assert.NoError(t, err)
	assert.Equal(t, value, []byte("pvtValue5"))
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

	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block1}, &lgr.CommitOptions{})

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
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block2}, &lgr.CommitOptions{})

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
	block := bg.NextBlockWithTxid([][]byte{pubSimBytes}, []string{txid})
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
