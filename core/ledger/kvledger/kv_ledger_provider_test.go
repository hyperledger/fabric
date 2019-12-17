/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/dataformat"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	lgr "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/msgs"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLedgerProvider(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t)
	numLedgers := 10
	existingLedgerIDs, err := provider.List()
	assert.NoError(t, err)
	assert.Len(t, existingLedgerIDs, 0)
	genesisBlocks := make([]*common.Block, numLedgers)
	for i := 0; i < numLedgers; i++ {
		genesisBlock, _ := configtxtest.MakeGenesisBlock(constructTestLedgerID(i))
		genesisBlocks[i] = genesisBlock
		provider.Create(genesisBlock)
	}
	existingLedgerIDs, err = provider.List()
	assert.NoError(t, err)
	assert.Len(t, existingLedgerIDs, numLedgers)

	// verify formatKey is present in idStore
	s := provider.idStore
	val, err := s.db.Get(formatKey)
	require.NoError(t, err)
	require.Equal(t, []byte(dataformat.Version20), val)

	provider.Close()

	provider = testutilNewProvider(conf, t)
	defer provider.Close()
	ledgerIds, _ := provider.List()
	assert.Len(t, ledgerIds, numLedgers)
	for i := 0; i < numLedgers; i++ {
		assert.Equal(t, constructTestLedgerID(i), ledgerIds[i])
	}
	for i := 0; i < numLedgers; i++ {
		ledgerid := constructTestLedgerID(i)
		status, _ := provider.Exists(ledgerid)
		assert.True(t, status)
		ledger, err := provider.Open(ledgerid)
		assert.NoError(t, err)
		bcInfo, err := ledger.GetBlockchainInfo()
		ledger.Close()
		assert.NoError(t, err)
		assert.Equal(t, uint64(1), bcInfo.Height)

		// check that the genesis block was persisted in the provider's db
		s := provider.idStore
		gbBytesInProviderStore, err := s.db.Get(s.encodeLedgerKey(ledgerid, ledgerKeyPrefix))
		assert.NoError(t, err)
		gb := &common.Block{}
		assert.NoError(t, proto.Unmarshal(gbBytesInProviderStore, gb))
		assert.True(t, proto.Equal(gb, genesisBlocks[i]), "proto messages are not equal")

		// check that ledger metadata keys were persisted in idStore with active status
		val, err := s.db.Get(s.encodeLedgerKey(ledgerid, metadataKeyPrefix))
		require.NoError(t, err)
		metadata := &msgs.LedgerMetadata{}
		require.NoError(t, proto.Unmarshal(val, metadata))
		require.Equal(t, msgs.Status_ACTIVE, metadata.Status)
	}
	gb, _ := configtxtest.MakeGenesisBlock(constructTestLedgerID(2))
	_, err = provider.Create(gb)
	assert.Equal(t, ErrLedgerIDExists, err)

	status, err := provider.Exists(constructTestLedgerID(numLedgers))
	assert.NoError(t, err, "Failed to check for ledger existence")
	assert.Equal(t, status, false)

	_, err = provider.Open(constructTestLedgerID(numLedgers))
	assert.Equal(t, ErrNonExistingLedgerID, err)
}

func TestGetActiveLedgerIDsIteratorError(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t)

	for i := 0; i < 2; i++ {
		genesisBlock, _ := configtxtest.MakeGenesisBlock(constructTestLedgerID(i))
		provider.Create(genesisBlock)
	}

	// close provider to trigger db error
	provider.Close()
	_, err := provider.idStore.getActiveLedgerIDs()
	require.EqualError(t, err, "error getting ledger ids from idStore: leveldb: closed")
}

func TestLedgerMetataDataUnmarshalError(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t)
	defer provider.Close()

	ledgerID := constructTestLedgerID(0)
	genesisBlock, _ := configtxtest.MakeGenesisBlock(ledgerID)
	provider.Create(genesisBlock)

	// put invalid bytes for the metatdata key
	provider.idStore.db.Put(provider.idStore.encodeLedgerKey(ledgerID, metadataKeyPrefix), []byte("invalid"), true)

	_, err := provider.List()
	require.EqualError(t, err, "error unmarshalling ledger metadata: unexpected EOF")

	_, err = provider.Open(ledgerID)
	require.EqualError(t, err, "error unmarshalling ledger metadata: unexpected EOF")
}

func TestNewProviderIdStoreFormatError(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()

	require.NoError(t, testutil.Unzip("tests/testdata/v11/sample_ledgers/ledgersData.zip", conf.RootFSPath, false))

	// NewProvider fails because ledgerProvider (idStore) has old format
	_, err := NewProvider(
		&lgr.Initializer{
			DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
			MetricsProvider:               &disabled.Provider{},
			Config:                        conf,
		},
	)
	require.EqualError(t, err, fmt.Sprintf("unexpected format. db info = [leveldb for channel-IDs at [%s]], data format = [], expected format = [2.0]", LedgerProviderPath(conf.RootFSPath)))
}

func TestUpgradeIDStoreFormatDBError(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t)
	provider.Close()

	err := provider.idStore.upgradeFormat()
	require.EqualError(t, err, "error retrieving leveldb key [[]byte{0x66}]: leveldb: closed")
}

func TestLedgerProviderHistoryDBDisabled(t *testing.T) {
	conf, cleanup := testConfig(t)
	conf.HistoryDBConfig.Enabled = false
	defer cleanup()
	provider := testutilNewProvider(conf, t)
	numLedgers := 10
	existingLedgerIDs, err := provider.List()
	assert.NoError(t, err)
	assert.Len(t, existingLedgerIDs, 0)
	genesisBlocks := make([]*common.Block, numLedgers)
	for i := 0; i < numLedgers; i++ {
		genesisBlock, _ := configtxtest.MakeGenesisBlock(constructTestLedgerID(i))
		genesisBlocks[i] = genesisBlock
		provider.Create(genesisBlock)
	}
	existingLedgerIDs, err = provider.List()
	assert.NoError(t, err)
	assert.Len(t, existingLedgerIDs, numLedgers)

	provider.Close()

	provider = testutilNewProvider(conf, t)
	defer provider.Close()
	ledgerIds, _ := provider.List()
	assert.Len(t, ledgerIds, numLedgers)
	t.Logf("ledgerIDs=%#v", ledgerIds)
	for i := 0; i < numLedgers; i++ {
		assert.Equal(t, constructTestLedgerID(i), ledgerIds[i])
	}
	for i := 0; i < numLedgers; i++ {
		ledgerid := constructTestLedgerID(i)
		status, _ := provider.Exists(ledgerid)
		assert.True(t, status)
		ledger, err := provider.Open(ledgerid)
		assert.NoError(t, err)
		bcInfo, err := ledger.GetBlockchainInfo()
		ledger.Close()
		assert.NoError(t, err)
		assert.Equal(t, uint64(1), bcInfo.Height)

		// check that the genesis block was persisted in the provider's db
		s := provider.idStore
		gbBytesInProviderStore, err := s.db.Get(s.encodeLedgerKey(ledgerid, ledgerKeyPrefix))
		assert.NoError(t, err)
		gb := &common.Block{}
		assert.NoError(t, proto.Unmarshal(gbBytesInProviderStore, gb))
		assert.True(t, proto.Equal(gb, genesisBlocks[i]), "proto messages are not equal")
	}
	gb, _ := configtxtest.MakeGenesisBlock(constructTestLedgerID(2))
	_, err = provider.Create(gb)
	assert.Equal(t, ErrLedgerIDExists, err)

	status, err := provider.Exists(constructTestLedgerID(numLedgers))
	assert.NoError(t, err, "Failed to check for ledger existence")
	assert.Equal(t, status, false)

	_, err = provider.Open(constructTestLedgerID(numLedgers))
	assert.Equal(t, ErrNonExistingLedgerID, err)

}

func TestRecovery(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider1 := testutilNewProvider(conf, t)
	defer provider1.Close()

	// now create the genesis block
	genesisBlock, _ := configtxtest.MakeGenesisBlock(constructTestLedgerID(1))
	ledger, err := provider1.openInternal(constructTestLedgerID(1))
	ledger.CommitLegacy(&lgr.BlockAndPvtData{Block: genesisBlock}, &lgr.CommitOptions{})
	ledger.Close()

	// Case 1: assume a crash happens, force underconstruction flag to be set to simulate
	// a failure where ledgerid is being created - ie., block is written but flag is not unset
	provider1.idStore.setUnderConstructionFlag(constructTestLedgerID(1))
	provider1.Close()

	// construct a new provider1 to invoke recovery
	provider1 = testutilNewProvider(conf, t)
	// verify the underecoveryflag and open the ledger
	flag, err := provider1.idStore.getUnderConstructionFlag()
	assert.NoError(t, err, "Failed to read the underconstruction flag")
	assert.Equal(t, "", flag)
	ledger, err = provider1.Open(constructTestLedgerID(1))
	assert.NoError(t, err, "Failed to open the ledger")
	ledger.Close()

	// Case 0: assume a crash happens before the genesis block of ledger 2 is committed
	// Open the ID store (inventory of chainIds/ledgerIds)
	provider1.idStore.setUnderConstructionFlag(constructTestLedgerID(2))
	provider1.Close()

	// construct a new provider to invoke recovery
	provider2 := testutilNewProvider(conf, t)
	defer provider2.Close()
	assert.NoError(t, err, "Provider failed to recover an underConstructionLedger")
	flag, err = provider2.idStore.getUnderConstructionFlag()
	assert.NoError(t, err, "Failed to read the underconstruction flag")
	assert.Equal(t, "", flag)
}

func TestRecoveryHistoryDBDisabled(t *testing.T) {
	conf, cleanup := testConfig(t)
	conf.HistoryDBConfig.Enabled = false
	defer cleanup()
	provider1 := testutilNewProvider(conf, t)
	defer provider1.Close()

	// now create the genesis block
	genesisBlock, _ := configtxtest.MakeGenesisBlock(constructTestLedgerID(1))
	ledger, err := provider1.openInternal(constructTestLedgerID(1))
	assert.NoError(t, err, "Failed to open the ledger")
	ledger.CommitLegacy(&lgr.BlockAndPvtData{Block: genesisBlock}, &lgr.CommitOptions{})
	ledger.Close()

	// Case 1: assume a crash happens, force underconstruction flag to be set to simulate
	// a failure where ledgerid is being created - ie., block is written but flag is not unset
	provider1.idStore.setUnderConstructionFlag(constructTestLedgerID(1))
	provider1.Close()

	// construct a new provider to invoke recovery
	provider2 := testutilNewProvider(conf, t)
	defer provider2.Close()
	// verify the underecoveryflag and open the ledger
	flag, err := provider2.idStore.getUnderConstructionFlag()
	assert.NoError(t, err, "Failed to read the underconstruction flag")
	assert.Equal(t, "", flag)
	ledger, err = provider2.Open(constructTestLedgerID(1))
	assert.NoError(t, err, "Failed to open the ledger")
	ledger.Close()

	// Case 0: assume a crash happens before the genesis block of ledger 2 is committed
	// Open the ID store (inventory of chainIds/ledgerIds)
	provider2.idStore.setUnderConstructionFlag(constructTestLedgerID(2))
	provider2.Close()

	// construct a new provider to invoke recovery
	provider3 := testutilNewProvider(conf, t)
	defer provider3.Close()
	assert.NoError(t, err, "Provider failed to recover an underConstructionLedger")
	flag, err = provider3.idStore.getUnderConstructionFlag()
	assert.NoError(t, err, "Failed to read the underconstruction flag")
	assert.Equal(t, "", flag)
}

func TestMultipleLedgerBasicRW(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider1 := testutilNewProvider(conf, t)
	defer provider1.Close()

	numLedgers := 10
	ledgers := make([]lgr.PeerLedger, numLedgers)
	for i := 0; i < numLedgers; i++ {
		bg, gb := testutil.NewBlockGenerator(t, constructTestLedgerID(i), false)
		l, err := provider1.Create(gb)
		assert.NoError(t, err)
		ledgers[i] = l
		txid := util.GenerateUUID()
		s, _ := l.NewTxSimulator(txid)
		err = s.SetState("ns", "testKey", []byte(fmt.Sprintf("testValue_%d", i)))
		s.Done()
		assert.NoError(t, err)
		res, err := s.GetTxSimulationResults()
		assert.NoError(t, err)
		pubSimBytes, _ := res.GetPubSimulationBytes()
		b := bg.NextBlock([][]byte{pubSimBytes})
		err = l.CommitLegacy(&lgr.BlockAndPvtData{Block: b}, &ledger.CommitOptions{})
		l.Close()
		assert.NoError(t, err)
	}

	provider1.Close()

	provider2 := testutilNewProvider(conf, t)
	defer provider2.Close()
	ledgers = make([]lgr.PeerLedger, numLedgers)
	for i := 0; i < numLedgers; i++ {
		l, err := provider2.Open(constructTestLedgerID(i))
		assert.NoError(t, err)
		ledgers[i] = l
	}

	for i, l := range ledgers {
		q, _ := l.NewQueryExecutor()
		val, err := q.GetState("ns", "testKey")
		q.Done()
		assert.NoError(t, err)
		assert.Equal(t, []byte(fmt.Sprintf("testValue_%d", i)), val)
		l.Close()
	}
}

func TestLedgerBackup(t *testing.T) {
	ledgerid := "TestLedger"
	basePath, err := ioutil.TempDir("", "kvledger")
	require.NoError(t, err, "Failed to create ledger directory")
	defer os.RemoveAll(basePath)
	originalPath := filepath.Join(basePath, "kvledger1")
	restorePath := filepath.Join(basePath, "kvledger2")

	// create and populate a ledger in the original environment
	origConf := &lgr.Config{
		RootFSPath:    originalPath,
		StateDBConfig: &lgr.StateDBConfig{},
		PrivateDataConfig: &lgr.PrivateDataConfig{
			MaxBatchSize:    5000,
			BatchesInterval: 1000,
			PurgeInterval:   100,
		},
		HistoryDBConfig: &lgr.HistoryDBConfig{
			Enabled: true,
		},
	}
	provider := testutilNewProvider(origConf, t)
	bg, gb := testutil.NewBlockGenerator(t, ledgerid, false)
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	ledger, _ := provider.Create(gb)

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

	ledger.Close()
	provider.Close()

	// remove the statedb, historydb, and block indexes (they are supposed to be auto created during opening of an existing ledger)
	// and rename the originalPath to restorePath
	assert.NoError(t, os.RemoveAll(StateDBPath(originalPath)))
	assert.NoError(t, os.RemoveAll(HistoryDBPath(originalPath)))
	assert.NoError(t, os.RemoveAll(filepath.Join(BlockStorePath(originalPath), fsblkstorage.IndexDir)))
	assert.NoError(t, os.Rename(originalPath, restorePath))

	// Instantiate the ledger from restore environment and this should behave exactly as it would have in the original environment
	restoreConf := &lgr.Config{
		RootFSPath:    restorePath,
		StateDBConfig: &lgr.StateDBConfig{},
		PrivateDataConfig: &lgr.PrivateDataConfig{
			MaxBatchSize:    5000,
			BatchesInterval: 1000,
			PurgeInterval:   100,
		},
		HistoryDBConfig: &lgr.HistoryDBConfig{
			Enabled: true,
		},
	}
	provider = testutilNewProvider(restoreConf, t)
	defer provider.Close()

	_, err = provider.Create(gb)
	assert.Equal(t, ErrLedgerIDExists, err)

	ledger, _ = provider.Open(ledgerid)
	defer ledger.Close()

	block1Hash := protoutil.BlockHeaderHash(block1.Header)
	block2Hash := protoutil.BlockHeaderHash(block2.Header)
	bcInfo, _ := ledger.GetBlockchainInfo()
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

	qe, _ := ledger.NewQueryExecutor()
	value1, _ := qe.GetState("ns1", "key1")
	assert.Equal(t, []byte("value4"), value1)

	hqe, err := ledger.NewHistoryQueryExecutor()
	assert.NoError(t, err)
	itr, err := hqe.GetHistoryForKey("ns1", "key1")
	assert.NoError(t, err)
	defer itr.Close()

	result1, err := itr.Next()
	assert.NoError(t, err)
	assert.Equal(t, []byte("value4"), result1.(*queryresult.KeyModification).Value)
	result2, err := itr.Next()
	assert.NoError(t, err)
	assert.Equal(t, []byte("value1"), result2.(*queryresult.KeyModification).Value)
}

func constructTestLedgerID(i int) string {
	return fmt.Sprintf("ledger_%06d", i)
}

func testConfig(t *testing.T) (conf *lgr.Config, cleanup func()) {
	path, err := ioutil.TempDir("", "kvledger")
	require.NoError(t, err, "Failed to create test ledger directory")
	conf = &lgr.Config{
		RootFSPath:    path,
		StateDBConfig: &lgr.StateDBConfig{},
		PrivateDataConfig: &lgr.PrivateDataConfig{
			MaxBatchSize:    5000,
			BatchesInterval: 1000,
			PurgeInterval:   100,
		},
		HistoryDBConfig: &lgr.HistoryDBConfig{
			Enabled: true,
		},
	}
	cleanup = func() {
		os.RemoveAll(path)
	}

	return conf, cleanup
}

func testutilNewProvider(conf *lgr.Config, t *testing.T) *Provider {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	provider, err := NewProvider(
		&lgr.Initializer{
			DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
			MetricsProvider:               &disabled.Provider{},
			Config:                        conf,
			Hasher:                        cryptoProvider,
		},
	)
	require.NoError(t, err, "Failed to create new Provider")
	return provider
}

func testutilNewProviderWithCollectionConfig(
	t *testing.T,
	namespace string,
	btlConfigs map[string]uint64,
	conf *lgr.Config,
) *Provider {
	provider := testutilNewProvider(conf, t)
	mockCCInfoProvider := provider.initializer.DeployedChaincodeInfoProvider.(*mock.DeployedChaincodeInfoProvider)
	collMap := map[string]*peer.StaticCollectionConfig{}
	var collConf []*peer.CollectionConfig
	for collName, btl := range btlConfigs {
		staticConf := &peer.StaticCollectionConfig{Name: collName, BlockToLive: btl}
		collMap[collName] = staticConf
		collectionConf := &peer.CollectionConfig{}
		collectionConf.Payload = &peer.CollectionConfig_StaticCollectionConfig{StaticCollectionConfig: staticConf}
		collConf = append(collConf, collectionConf)
	}
	collectionConfPkg := &peer.CollectionConfigPackage{Config: collConf}

	mockCCInfoProvider.ChaincodeInfoStub = func(channelName, ccName string, qe lgr.SimpleQueryExecutor) (*lgr.DeployedChaincodeInfo, error) {
		if ccName == namespace {
			return &lgr.DeployedChaincodeInfo{
				Name: namespace, ExplicitCollectionConfigPkg: collectionConfPkg}, nil
		}
		return nil, nil
	}

	mockCCInfoProvider.AllCollectionsConfigPkgStub = func(channelName, ccName string, qe lgr.SimpleQueryExecutor) (*peer.CollectionConfigPackage, error) {
		if ccName == namespace {
			return collectionConfPkg, nil
		}
		return nil, nil
	}

	mockCCInfoProvider.CollectionInfoStub = func(channelName, ccName, collName string, qe lgr.SimpleQueryExecutor) (*peer.StaticCollectionConfig, error) {
		if ccName == namespace {
			return collMap[collName], nil
		}
		return nil, nil
	}
	return provider
}
