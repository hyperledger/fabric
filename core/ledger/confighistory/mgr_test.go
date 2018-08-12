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

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.SetModuleLevel("confighistory", "debug")
	os.Exit(m.Run())
}

func TestMgr(t *testing.T) {
	dbPath := "/tmp/fabric/core/ledger/confighistory"
	env := newTestEnv(t, dbPath)
	mgr := env.mgr
	defer env.cleanup()
	chaincodeName := "chaincode1"
	maxBlockNumberInLedger := uint64(2000)
	dummyLedgerInfoRetriever := &dummyLedgerInfoRetriever{info: &common.BlockchainInfo{Height: maxBlockNumberInLedger + 1}}
	configCommittingBlockNums := []uint64{5, 10, 15, 100}
	ledgerIds := []string{"ledgerid1", "ledger2"}

	// Populate collection config versions
	for _, ledgerid := range ledgerIds {
		for _, committingBlockNum := range configCommittingBlockNums {
			// for each ledgerid and commitHeight combination, construct a unique collConfigPackage and induce a stateUpdate
			collConfigPackage := sampleCollectionConfigPackage(ledgerid, committingBlockNum)
			stateUpdate := sampleStateUpdate(t, chaincodeName, collConfigPackage)
			mgr.HandleStateUpdates(&ledger.StateUpdateTrigger{
				LedgerID:           ledgerid,
				StateUpdates:       stateUpdate,
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

type testEnv struct {
	dbPath string
	mgr    Mgr
	t      *testing.T
}

func newTestEnv(t *testing.T, dbPath string) *testEnv {
	env := &testEnv{dbPath: dbPath, t: t}
	env.cleanup()
	env.mgr = newMgr(dbPath)
	return env
}

func (env *testEnv) cleanup() {
	err := os.RemoveAll(env.dbPath)
	assert.NoError(env.t, err)
}

func sampleCollectionConfigPackage(collNamePart1 string, collNamePart2 uint64) *common.CollectionConfigPackage {
	collName := fmt.Sprintf("%s-%d", collNamePart1, collNamePart2)
	collConfig := &common.CollectionConfig{Payload: &common.CollectionConfig_StaticCollectionConfig{
		StaticCollectionConfig: &common.StaticCollectionConfig{Name: collName}}}
	collConfigPackage := &common.CollectionConfigPackage{Config: []*common.CollectionConfig{collConfig}}
	return collConfigPackage
}

func sampleStateUpdate(t *testing.T, ccName string, collConfigPackage *common.CollectionConfigPackage) ledger.StateUpdates {
	b, err := proto.Marshal(collConfigPackage)
	assert.NoError(t, err)
	return ledger.StateUpdates{
		lsccNamespace: []*kvrwset.KVWrite{
			{Key: ccName, Value: []byte("ccHash")},
			{Key: constructCollectionConfigKey(ccName), Value: b},
		},
	}
}

type dummyLedgerInfoRetriever struct {
	info *common.BlockchainInfo
}

func (d *dummyLedgerInfoRetriever) GetBlockchainInfo() (*common.BlockchainInfo, error) {
	return d.info, nil
}
