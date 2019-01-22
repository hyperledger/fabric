/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package confighistory

import (
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("confighistory=debug")
	os.Exit(m.Run())
}

func TestWithNoCollectionConfig(t *testing.T) {
	dbPath := "/tmp/fabric/core/ledger/confighistory"
	mockCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	env := newTestEnv(t, dbPath, mockCCInfoProvider)
	mgr := env.mgr
	defer env.cleanup()
	testutilEquipMockCCInfoProviderToReturnDesiredCollConfig(mockCCInfoProvider, "chaincode1", nil)
	err := mgr.HandleStateUpdates(&ledger.StateUpdateTrigger{
		LedgerID:           "ledger1",
		CommittingBlockNum: 50},
	)
	assert.NoError(t, err)
	dummyLedgerInfoRetriever := &dummyLedgerInfoRetriever{
		info: &common.BlockchainInfo{Height: 100},
		qe:   &mock.QueryExecutor{},
	}
	retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
	collConfig, err := retriever.MostRecentCollectionConfigBelow(90, "chaincode1")
	assert.NoError(t, err)
	assert.Nil(t, collConfig)
}

func TestMgr(t *testing.T) {
	dbPath := "/tmp/fabric/core/ledger/confighistory"
	mockCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	env := newTestEnv(t, dbPath, mockCCInfoProvider)
	mgr := env.mgr
	defer env.cleanup()
	chaincodeName := "chaincode1"
	maxBlockNumberInLedger := uint64(2000)
	dummyLedgerInfoRetriever := &dummyLedgerInfoRetriever{
		info: &common.BlockchainInfo{Height: maxBlockNumberInLedger + 1},
		qe:   &mock.QueryExecutor{},
	}
	configCommittingBlockNums := []uint64{5, 10, 15, 100}
	ledgerIds := []string{"ledgerid1", "ledger2"}

	// Populate collection config versions
	for _, ledgerid := range ledgerIds {
		for _, committingBlockNum := range configCommittingBlockNums {
			// for each ledgerid and commitHeight combination, construct a unique collConfigPackage and induce a stateUpdate
			collConfigPackage := sampleCollectionConfigPackage(ledgerid, committingBlockNum)
			testutilEquipMockCCInfoProviderToReturnDesiredCollConfig(mockCCInfoProvider, chaincodeName, collConfigPackage)
			mgr.HandleStateUpdates(&ledger.StateUpdateTrigger{
				LedgerID:           ledgerid,
				CommittingBlockNum: committingBlockNum},
			)
		}
	}

	t.Run("test-api-MostRecentCollectionConfigBelow()", func(t *testing.T) {
		// A map that contains entries such that for each of the entries of type <K, V>,
		// we retrieve the 'MostRecentCollectionConfigBelow' for 'K' and the expected value
		// should be configuration committed at 'V'
		m := map[uint64]uint64{math.MaxUint64: 100, 1000: 100, 50: 15, 12: 10, 7: 5}
		for _, ledgerid := range ledgerIds {
			retriever := mgr.GetRetriever(ledgerid, dummyLedgerInfoRetriever)
			for testHeight, expectedHeight := range m {
				retrievedConfig, err := retriever.MostRecentCollectionConfigBelow(testHeight, chaincodeName)
				assert.NoError(t, err)
				expectedConfig := sampleCollectionConfigPackage(ledgerid, expectedHeight)
				assert.Equal(t, expectedConfig, retrievedConfig.CollectionConfig)
				assert.Equal(t, expectedHeight, retrievedConfig.CommittingBlockNum)
			}

			retrievedConfig, err := retriever.MostRecentCollectionConfigBelow(5, chaincodeName)
			assert.NoError(t, err)
			assert.Nil(t, retrievedConfig)
		}
	})

	t.Run("test-api-CollectionConfigAt()", func(t *testing.T) {
		for _, ledgerid := range ledgerIds {
			retriever := mgr.GetRetriever(ledgerid, dummyLedgerInfoRetriever)
			for _, commitHeight := range configCommittingBlockNums {
				retrievedConfig, err := retriever.CollectionConfigAt(commitHeight, chaincodeName)
				assert.NoError(t, err)
				expectedConfig := sampleCollectionConfigPackage(ledgerid, commitHeight)
				assert.Equal(t, expectedConfig, retrievedConfig.CollectionConfig)
				assert.Equal(t, commitHeight, retrievedConfig.CommittingBlockNum)
			}
		}
	})

	t.Run("test-api-CollectionConfigAt-BoundaryCases()", func(t *testing.T) {
		retriever := mgr.GetRetriever("ledgerid1", dummyLedgerInfoRetriever)
		retrievedConfig, err := retriever.CollectionConfigAt(4, chaincodeName)
		assert.NoError(t, err)
		assert.Nil(t, retrievedConfig)

		retrievedConfig, err = retriever.CollectionConfigAt(5000, chaincodeName)
		typedErr, ok := err.(*ledger.ErrCollectionConfigNotYetAvailable)
		assert.True(t, ok)
		assert.Equal(t, maxBlockNumberInLedger, typedErr.MaxBlockNumCommitted)
	})
}

func TestWithImplicitColls(t *testing.T) {
	dbPath := "/tmp/fabric/core/ledger/confighistory"
	collConfigPackage := testutilCreateCollConfigPkg([]string{"Explicit-coll-1", "Explicit-coll-2"})
	mockCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mockCCInfoProvider.ImplicitCollectionsReturns(
		[]*common.StaticCollectionConfig{
			{
				Name: "Implicit-coll-1",
			},
			{
				Name: "Implicit-coll-2",
			},
		},
		nil,
	)
	env := newTestEnv(t, dbPath, mockCCInfoProvider)
	defer env.cleanup()
	mgr := env.mgr

	// add explicit collections at height 20
	batch, err := prepareDBBatch(
		map[string]*common.CollectionConfigPackage{
			"chaincode1": collConfigPackage,
		},
		20,
	)
	assert.NoError(t, err)
	dbHandle := mgr.dbProvider.getDB("ledger1")
	assert.NoError(t, dbHandle.writeBatch(batch, true))

	onlyImplicitCollections := testutilCreateCollConfigPkg(
		[]string{"Implicit-coll-1", "Implicit-coll-2"},
	)

	explicitAndImplicitCollections := testutilCreateCollConfigPkg(
		[]string{"Explicit-coll-1", "Explicit-coll-2", "Implicit-coll-1", "Implicit-coll-2"},
	)

	dummyLedgerInfoRetriever := &dummyLedgerInfoRetriever{
		info: &common.BlockchainInfo{Height: 1000},
		qe:   &mock.QueryExecutor{},
	}

	t.Run("CheckQueryExecutorCalls", func(t *testing.T) {
		retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
		// function MostRecentCollectionConfigBelow calls Done on query executor
		_, err := retriever.MostRecentCollectionConfigBelow(50, "chaincode1")
		assert.NoError(t, err)
		assert.Equal(t, 1, dummyLedgerInfoRetriever.qe.DoneCallCount())
		// function CollectionConfigAt calls Done on query executor
		_, err = retriever.CollectionConfigAt(50, "chaincode1")
		assert.NoError(t, err)
		assert.Equal(t, 2, dummyLedgerInfoRetriever.qe.DoneCallCount())
	})

	t.Run("MostRecentCollectionConfigBelow50", func(t *testing.T) {
		// explicit collections added at height 20 should be merged with the implicit collections
		retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
		retrievedConfig, err := retriever.MostRecentCollectionConfigBelow(50, "chaincode1")
		assert.NoError(t, err)
		assert.True(t, proto.Equal(retrievedConfig.CollectionConfig, explicitAndImplicitCollections))
	})

	t.Run("MostRecentCollectionConfigBelow10", func(t *testing.T) {
		// No explicit collections below height 10, should return only implicit collections
		retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
		retrievedConfig, err := retriever.MostRecentCollectionConfigBelow(10, "chaincode1")
		assert.NoError(t, err)
		assert.True(t, proto.Equal(retrievedConfig.CollectionConfig, onlyImplicitCollections))
	})

	t.Run("CollectionConfigAt50", func(t *testing.T) {
		// No explicit collections at height 50, should return only implicit collections
		retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
		retrievedConfig, err := retriever.CollectionConfigAt(50, "chaincode1")
		assert.NoError(t, err)
		assert.True(t, proto.Equal(retrievedConfig.CollectionConfig, onlyImplicitCollections))
	})

	t.Run("CollectionConfigAt20", func(t *testing.T) {
		// Explicit collections at height 20, should be merged with implicit collections
		retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
		retrievedConfig, err := retriever.CollectionConfigAt(20, "chaincode1")
		assert.NoError(t, err)
		assert.True(t, proto.Equal(retrievedConfig.CollectionConfig, explicitAndImplicitCollections))
	})

}

type testEnv struct {
	dbPath string
	mgr    *mgr
	t      *testing.T
}

func newTestEnv(t *testing.T, dbPath string, ccInfoProvider ledger.DeployedChaincodeInfoProvider) *testEnv {
	env := &testEnv{dbPath: dbPath, t: t}
	env.cleanup()
	env.mgr = newMgr(ccInfoProvider, dbPath).(*mgr)
	return env
}

func (env *testEnv) cleanup() {
	err := os.RemoveAll(env.dbPath)
	assert.NoError(env.t, err)
}

func sampleCollectionConfigPackage(collNamePart1 string, collNamePart2 uint64) *common.CollectionConfigPackage {
	collName := fmt.Sprintf("%s-%d", collNamePart1, collNamePart2)
	return testutilCreateCollConfigPkg([]string{collName})
}

func testutilEquipMockCCInfoProviderToReturnDesiredCollConfig(
	mockCCInfoProvider *mock.DeployedChaincodeInfoProvider,
	chaincodeName string,
	collConfigPackage *common.CollectionConfigPackage) {
	mockCCInfoProvider.UpdatedChaincodesReturns(
		[]*ledger.ChaincodeLifecycleInfo{
			{Name: chaincodeName},
		},
		nil,
	)
	mockCCInfoProvider.ChaincodeInfoReturns(
		&ledger.DeployedChaincodeInfo{Name: chaincodeName, ExplicitCollectionConfigPkg: collConfigPackage},
		nil,
	)
}

func testutilCreateCollConfigPkg(collNames []string) *common.CollectionConfigPackage {
	pkg := &common.CollectionConfigPackage{
		Config: []*common.CollectionConfig{},
	}
	for _, collName := range collNames {
		pkg.Config = append(pkg.Config,
			&common.CollectionConfig{
				Payload: &common.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &common.StaticCollectionConfig{
						Name: collName,
					},
				},
			},
		)
	}
	return pkg
}

type dummyLedgerInfoRetriever struct {
	info *common.BlockchainInfo
	qe   *mock.QueryExecutor
}

func (d *dummyLedgerInfoRetriever) GetBlockchainInfo() (*common.BlockchainInfo, error) {
	return d.info, nil
}

func (d *dummyLedgerInfoRetriever) NewQueryExecutor() (ledger.QueryExecutor, error) {
	return d.qe, nil
}
