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
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	lgr "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestLedgerProvider(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	numLedgers := 10
	provider := testutilNewProvider(t)
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

	provider = testutilNewProvider(t)
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
		s := provider.(*Provider).idStore
		gbBytesInProviderStore, err := s.db.Get(s.encodeLedgerKey(ledgerid))
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
	env := newTestEnv(t)
	defer env.cleanup()
	provider := testutilNewProvider(t)

	// now create the genesis block
	genesisBlock, _ := configtxtest.MakeGenesisBlock(constructTestLedgerID(1))
	ledger, err := provider.(*Provider).openInternal(constructTestLedgerID(1))
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: genesisBlock}, &lgr.CommitOptions{})
	ledger.Close()

	// Case 1: assume a crash happens, force underconstruction flag to be set to simulate
	// a failure where ledgerid is being created - ie., block is written but flag is not unset
	provider.(*Provider).idStore.setUnderConstructionFlag(constructTestLedgerID(1))
	provider.Close()

	// construct a new provider to invoke recovery
	provider = testutilNewProvider(t)
	// verify the underecoveryflag and open the ledger
	flag, err := provider.(*Provider).idStore.getUnderConstructionFlag()
	assert.NoError(t, err, "Failed to read the underconstruction flag")
	assert.Equal(t, "", flag)
	ledger, err = provider.Open(constructTestLedgerID(1))
	assert.NoError(t, err, "Failed to open the ledger")
	ledger.Close()

	// Case 0: assume a crash happens before the genesis block of ledger 2 is committed
	// Open the ID store (inventory of chainIds/ledgerIds)
	provider.(*Provider).idStore.setUnderConstructionFlag(constructTestLedgerID(2))
	provider.Close()

	// construct a new provider to invoke recovery
	provider = testutilNewProvider(t)
	assert.NoError(t, err, "Provider failed to recover an underConstructionLedger")
	flag, err = provider.(*Provider).idStore.getUnderConstructionFlag()
	assert.NoError(t, err, "Failed to read the underconstruction flag")
	assert.Equal(t, "", flag)

}

func TestMultipleLedgerBasicRW(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	numLedgers := 10
	provider := testutilNewProvider(t)
	ledgers := make([]lgr.PeerLedger, numLedgers)
	for i := 0; i < numLedgers; i++ {
		bg, gb := testutil.NewBlockGenerator(t, constructTestLedgerID(i), false)
		l, err := provider.Create(gb)
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
		err = l.CommitWithPvtData(&lgr.BlockAndPvtData{Block: b}, &ledger.CommitOptions{})
		l.Close()
		assert.NoError(t, err)
	}

	provider.Close()

	provider = testutilNewProvider(t)
	defer provider.Close()
	ledgers = make([]lgr.PeerLedger, numLedgers)
	for i := 0; i < numLedgers; i++ {
		l, err := provider.Open(constructTestLedgerID(i))
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
	originalPath := "/tmp/fabric/ledgertests/kvledger1"
	restorePath := "/tmp/fabric/ledgertests/kvledger2"
	viper.Set("ledger.history.enableHistoryDatabase", true)

	// create and populate a ledger in the original environment
	env := createTestEnv(t, originalPath)
	provider := testutilNewProvider(t)
	bg, gb := testutil.NewBlockGenerator(t, ledgerid, false)
	gbHash := gb.Header.Hash()
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
	ledger.CommitWithPvtData(&lgr.BlockAndPvtData{Block: block1}, &lgr.CommitOptions{})

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

	ledger.Close()
	provider.Close()

	// Create restore environment
	env = createTestEnv(t, restorePath)

	// remove the statedb, historydb, and block indexes (they are supposed to be auto created during opening of an existing ledger)
	// and rename the originalPath to restorePath
	assert.NoError(t, os.RemoveAll(ledgerconfig.GetStateLevelDBPath()))
	assert.NoError(t, os.RemoveAll(ledgerconfig.GetHistoryLevelDBPath()))
	assert.NoError(t, os.RemoveAll(filepath.Join(ledgerconfig.GetBlockStorePath(), fsblkstorage.IndexDir)))
	assert.NoError(t, os.Rename(originalPath, restorePath))
	defer env.cleanup()

	// Instantiate the ledger from restore environment and this should behave exactly as it would have in the original environment
	provider = testutilNewProvider(t)
	defer provider.Close()

	_, err := provider.Create(gb)
	assert.Equal(t, ErrLedgerIDExists, err)

	ledger, _ = provider.Open(ledgerid)
	defer ledger.Close()

	block1Hash := block1.Header.Hash()
	block2Hash := block2.Header.Hash()
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
	assert.Equal(t, []byte("value1"), result1.(*queryresult.KeyModification).Value)
	result2, err := itr.Next()
	assert.NoError(t, err)
	assert.Equal(t, []byte("value4"), result2.(*queryresult.KeyModification).Value)
}

func constructTestLedgerID(i int) string {
	return fmt.Sprintf("ledger_%06d", i)
}

func testutilNewProvider(t *testing.T) lgr.PeerLedgerProvider {
	provider, err := NewProvider()
	assert.NoError(t, err)
	provider.Initialize(&lgr.Initializer{
		DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
		MetricsProvider:               &disabled.Provider{},
	})
	return provider
}

func testutilNewProviderWithCollectionConfig(t *testing.T, namespace string, btlConfigs map[string]uint64) lgr.PeerLedgerProvider {
	provider := testutilNewProvider(t)
	mockCCInfoProvider := provider.(*Provider).initializer.DeployedChaincodeInfoProvider.(*mock.DeployedChaincodeInfoProvider)
	collMap := map[string]*common.StaticCollectionConfig{}
	var conf []*common.CollectionConfig
	for collName, btl := range btlConfigs {
		staticConf := &common.StaticCollectionConfig{Name: collName, BlockToLive: btl}
		collMap[collName] = staticConf
		collectionConf := &common.CollectionConfig{}
		collectionConf.Payload = &common.CollectionConfig_StaticCollectionConfig{StaticCollectionConfig: staticConf}
		conf = append(conf, collectionConf)
	}
	collectionConfPkg := &common.CollectionConfigPackage{Config: conf}

	mockCCInfoProvider.ChaincodeInfoStub = func(ccName string, qe lgr.SimpleQueryExecutor) (*lgr.DeployedChaincodeInfo, error) {
		if ccName == namespace {
			return &lgr.DeployedChaincodeInfo{
				Name: namespace, CollectionConfigPkg: collectionConfPkg}, nil
		}
		return nil, nil
	}

	mockCCInfoProvider.CollectionInfoStub = func(ccName, collName string, qe lgr.SimpleQueryExecutor) (*common.StaticCollectionConfig, error) {
		if ccName == namespace {
			return collMap[collName], nil
		}
		return nil, nil
	}
	return provider
}
