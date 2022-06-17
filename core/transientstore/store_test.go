/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transientstore

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric-protos-go/transientstore"
	"github.com/hyperledger/fabric/common/policydsl"
	commonutil "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	tempdir, err := ioutil.TempDir("", "ts")
	if err != nil {
		panic(err)
	}

	rc := m.Run()

	os.RemoveAll(tempdir)
	os.Exit(rc)
}

type testEnv struct {
	storeProvider StoreProvider
	store         *Store
	tempdir       string
	storedir      string
}

func initTestEnv(t *testing.T) *testEnv {
	tempdir := t.TempDir()

	storedir := filepath.Join(tempdir, "transientstore")
	storeProvider, err := NewStoreProvider(storedir)
	require.NoError(t, err)
	require.NotNil(t, storeProvider)

	store, err := storeProvider.OpenStore("TestStore")
	require.NoError(t, err)
	require.NotNil(t, store)

	return &testEnv{
		storeProvider: storeProvider,
		store:         store,
		tempdir:       tempdir,
		storedir:      storedir,
	}
}

func TestPurgeIndexKeyCodingEncoding(t *testing.T) {
	require := require.New(t)
	blkHts := []uint64{0, 10, 20000}
	txids := []string{"txid", ""}
	uuids := []string{"uuid", ""}
	for _, blkHt := range blkHts {
		for _, txid := range txids {
			for _, uuid := range uuids {
				testCase := fmt.Sprintf("blkHt=%d,txid=%s,uuid=%s", blkHt, txid, uuid)
				t.Run(testCase, func(t *testing.T) {
					t.Logf("Running test case [%s]", testCase)
					purgeIndexKey := createCompositeKeyForPurgeIndexByHeight(blkHt, txid, uuid)
					txid1, uuid1, blkHt1, err := splitCompositeKeyOfPurgeIndexByHeight(purgeIndexKey)
					require.NoError(err)
					require.Equal(txid, txid1)
					require.Equal(uuid, uuid1)
					require.Equal(blkHt, blkHt1)
				})
			}
		}
	}
}

func TestRWSetKeyCodingEncoding(t *testing.T) {
	require := require.New(t)
	blkHts := []uint64{0, 10, 20000}
	txids := []string{"txid", ""}
	uuids := []string{"uuid", ""}
	for _, blkHt := range blkHts {
		for _, txid := range txids {
			for _, uuid := range uuids {
				testCase := fmt.Sprintf("blkHt=%d,txid=%s,uuid=%s", blkHt, txid, uuid)
				t.Run(testCase, func(t *testing.T) {
					t.Logf("Running test case [%s]", testCase)
					rwsetKey := createCompositeKeyForPvtRWSet(txid, uuid, blkHt)
					uuid1, blkHt1, err := splitCompositeKeyOfPvtRWSet(rwsetKey)
					require.NoError(err)
					require.Equal(uuid, uuid1)
					require.Equal(blkHt, blkHt1)
				})
			}
		}
	}
}

func TestTransientStorePersistAndRetrieve(t *testing.T) {
	env := initTestEnv(t)
	testStore := env.store
	require := require.New(t)
	txid := "txid-1"
	samplePvtRWSetWithConfig := samplePvtDataWithConfigInfo(t)

	// Create private simulation results for txid-1
	var endorsersResults []*EndorserPvtSimulationResults

	// Results produced by endorser 1
	endorser0SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          10,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser0SimulationResults)

	// Results produced by endorser 2
	endorser1SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          10,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser1SimulationResults)

	// Persist simulation results into  store
	var err error
	for i := 0; i < len(endorsersResults); i++ {
		err = testStore.Persist(txid, endorsersResults[i].ReceivedAtBlockHeight,
			endorsersResults[i].PvtSimulationResultsWithConfig)
		require.NoError(err)
	}

	// Retrieve simulation results of txid-1 from  store
	iter, err := testStore.GetTxPvtRWSetByTxid(txid, nil)
	require.NoError(err)

	var actualEndorsersResults []*EndorserPvtSimulationResults
	for {
		result, err := iter.Next()
		require.NoError(err)
		if result == nil {
			break
		}
		actualEndorsersResults = append(actualEndorsersResults, result)
	}
	iter.Close()
	sortResults(endorsersResults)
	sortResults(actualEndorsersResults)
	require.Equal(endorsersResults, actualEndorsersResults)
}

func TestTransientStorePersistAndRetrieveBothOldAndNewProto(t *testing.T) {
	env := initTestEnv(t)
	testStore := env.store
	require := require.New(t)
	txid := "txid-1"
	var receivedAtBlockHeight uint64 = 10
	var err error

	// Create and persist private simulation results with old proto for txid-1
	samplePvtRWSet := samplePvtData(t)
	err = testStore.persistOldProto(txid, receivedAtBlockHeight, samplePvtRWSet)
	require.NoError(err)

	// Create and persist private simulation results with new proto for txid-1
	samplePvtRWSetWithConfig := samplePvtDataWithConfigInfo(t)
	err = testStore.Persist(txid, receivedAtBlockHeight, samplePvtRWSetWithConfig)
	require.NoError(err)

	// Construct the expected results
	var expectedEndorsersResults []*EndorserPvtSimulationResults

	pvtRWSetWithConfigInfo := &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: samplePvtRWSet,
	}

	endorser0SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          receivedAtBlockHeight,
		PvtSimulationResultsWithConfig: pvtRWSetWithConfigInfo,
	}
	expectedEndorsersResults = append(expectedEndorsersResults, endorser0SimulationResults)

	endorser1SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          receivedAtBlockHeight,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	expectedEndorsersResults = append(expectedEndorsersResults, endorser1SimulationResults)

	// Retrieve simulation results of txid-1 from  store
	iter, err := testStore.GetTxPvtRWSetByTxid(txid, nil)
	require.NoError(err)

	var actualEndorsersResults []*EndorserPvtSimulationResults
	for {
		result, err := iter.Next()
		require.NoError(err)
		if result == nil {
			break
		}
		actualEndorsersResults = append(actualEndorsersResults, result)
	}
	iter.Close()
	sortResults(expectedEndorsersResults)
	sortResults(actualEndorsersResults)
	require.Equal(expectedEndorsersResults, actualEndorsersResults)
}

func TestTransientStorePurgeByTxids(t *testing.T) {
	env := initTestEnv(t)
	testStore := env.store
	require := require.New(t)

	var txids []string
	var endorsersResults []*EndorserPvtSimulationResults

	samplePvtRWSetWithConfig := samplePvtDataWithConfigInfo(t)

	// Create two private write set entry for txid-1
	txids = append(txids, "txid-1")
	endorser0SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          10,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser0SimulationResults)

	txids = append(txids, "txid-1")
	endorser1SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          11,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser1SimulationResults)

	// Create one private write set entry for txid-2
	txids = append(txids, "txid-2")
	endorser2SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          11,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser2SimulationResults)

	// Create three private write set entry for txid-3
	txids = append(txids, "txid-3")
	endorser3SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          12,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser3SimulationResults)

	txids = append(txids, "txid-3")
	endorser4SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          12,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser4SimulationResults)

	txids = append(txids, "txid-3")
	endorser5SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          13,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser5SimulationResults)

	var err error
	for i := 0; i < len(txids); i++ {
		err = testStore.Persist(txids[i], endorsersResults[i].ReceivedAtBlockHeight,
			endorsersResults[i].PvtSimulationResultsWithConfig)
		require.NoError(err)
	}

	// Retrieve simulation results of txid-2 from  store
	iter, err := testStore.GetTxPvtRWSetByTxid("txid-2", nil)
	require.NoError(err)

	// Expected results for txid-2
	var expectedEndorsersResults []*EndorserPvtSimulationResults
	expectedEndorsersResults = append(expectedEndorsersResults, endorser2SimulationResults)

	// Check whether actual results and expected results are same
	var actualEndorsersResults []*EndorserPvtSimulationResults
	for {
		result, err := iter.Next()
		require.NoError(err)
		if result == nil {
			break
		}
		actualEndorsersResults = append(actualEndorsersResults, result)
	}
	iter.Close()

	// Note that the ordering of actualRes and expectedRes is dependent on the uuid. Hence, we are sorting
	// expectedRes and actualRes.
	sortResults(expectedEndorsersResults)
	sortResults(actualEndorsersResults)

	require.Equal(len(expectedEndorsersResults), len(actualEndorsersResults))
	for i, expected := range expectedEndorsersResults {
		require.Equal(expected.ReceivedAtBlockHeight, actualEndorsersResults[i].ReceivedAtBlockHeight)
		require.True(proto.Equal(expected.PvtSimulationResultsWithConfig, actualEndorsersResults[i].PvtSimulationResultsWithConfig))
	}

	// Remove all private write set of txid-2 and txid-3
	toRemoveTxids := []string{"txid-2", "txid-3"}
	err = testStore.PurgeByTxids(toRemoveTxids)
	require.NoError(err)

	for _, txid := range toRemoveTxids {

		// Check whether private write sets of txid-2 are removed
		var expectedEndorsersResults *EndorserPvtSimulationResults = nil
		iter, err = testStore.GetTxPvtRWSetByTxid(txid, nil)
		require.NoError(err)
		// Should return nil, nil
		result, err := iter.Next()
		require.NoError(err)
		require.Equal(expectedEndorsersResults, result)
	}

	// Retrieve simulation results of txid-1 from store
	iter, err = testStore.GetTxPvtRWSetByTxid("txid-1", nil)
	require.NoError(err)

	// Expected results for txid-1
	expectedEndorsersResults = nil
	expectedEndorsersResults = append(expectedEndorsersResults, endorser0SimulationResults)
	expectedEndorsersResults = append(expectedEndorsersResults, endorser1SimulationResults)

	// Check whether actual results and expected results are same
	actualEndorsersResults = nil
	for {
		result, err := iter.Next()
		require.NoError(err)
		if result == nil {
			break
		}
		actualEndorsersResults = append(actualEndorsersResults, result)
	}
	iter.Close()

	// Note that the ordering of actualRes and expectedRes is dependent on the uuid. Hence, we are sorting
	// expectedRes and actualRes.
	sortResults(expectedEndorsersResults)
	sortResults(actualEndorsersResults)

	require.Equal(len(expectedEndorsersResults), len(actualEndorsersResults))
	for i, expected := range expectedEndorsersResults {
		require.Equal(expected.ReceivedAtBlockHeight, actualEndorsersResults[i].ReceivedAtBlockHeight)
		require.True(proto.Equal(expected.PvtSimulationResultsWithConfig, actualEndorsersResults[i].PvtSimulationResultsWithConfig))
	}

	toRemoveTxids = []string{"txid-1"}
	err = testStore.PurgeByTxids(toRemoveTxids)
	require.NoError(err)

	for _, txid := range toRemoveTxids {

		// Check whether private write sets of txid-1 are removed
		var expectedEndorsersResults *EndorserPvtSimulationResults = nil
		iter, err = testStore.GetTxPvtRWSetByTxid(txid, nil)
		require.NoError(err)
		// Should return nil, nil
		result, err := iter.Next()
		require.NoError(err)
		require.Equal(expectedEndorsersResults, result)
	}

	// There should be no entries in the  store
	_, err = testStore.GetMinTransientBlkHt()
	require.Equal(err, ErrStoreEmpty)
}

func TestTransientStorePurgeBelowHeight(t *testing.T) {
	env := initTestEnv(t)
	testStore := env.store
	require := require.New(t)

	txid := "txid-1"
	samplePvtRWSetWithConfig := samplePvtDataWithConfigInfo(t)

	// Create private simulation results for txid-1
	var endorsersResults []*EndorserPvtSimulationResults

	// Results produced by endorser 1
	endorser0SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          10,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser0SimulationResults)

	// Results produced by endorser 2
	endorser1SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          11,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser1SimulationResults)

	// Results produced by endorser 3
	endorser2SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          12,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser2SimulationResults)

	// Results produced by endorser 4
	endorser3SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          12,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser3SimulationResults)

	// Results produced by endorser 5
	endorser4SimulationResults := &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          13,
		PvtSimulationResultsWithConfig: samplePvtRWSetWithConfig,
	}
	endorsersResults = append(endorsersResults, endorser4SimulationResults)

	// Persist simulation results into  store
	var err error
	for i := 0; i < 5; i++ {
		err = testStore.Persist(txid, endorsersResults[i].ReceivedAtBlockHeight,
			endorsersResults[i].PvtSimulationResultsWithConfig)
		require.NoError(err)
	}

	// Retain results generate at block height greater than or equal to 12
	minTransientBlkHtToRetain := uint64(12)
	err = testStore.PurgeBelowHeight(minTransientBlkHtToRetain)
	require.NoError(err)

	// Retrieve simulation results of txid-1 from  store
	iter, err := testStore.GetTxPvtRWSetByTxid(txid, nil)
	require.NoError(err)

	// Expected results for txid-1
	var expectedEndorsersResults []*EndorserPvtSimulationResults
	expectedEndorsersResults = append(expectedEndorsersResults, endorser2SimulationResults) // endorsed at height 12
	expectedEndorsersResults = append(expectedEndorsersResults, endorser3SimulationResults) // endorsed at height 12
	expectedEndorsersResults = append(expectedEndorsersResults, endorser4SimulationResults) // endorsed at height 13

	// Check whether actual results and expected results are same
	var actualEndorsersResults []*EndorserPvtSimulationResults
	for {
		result, err := iter.Next()
		require.NoError(err)
		if result == nil {
			break
		}
		actualEndorsersResults = append(actualEndorsersResults, result)
	}
	iter.Close()

	// Note that the ordering of actualRes and expectedRes is dependent on the uuid. Hence, we are sorting
	// expectedRes and actualRes.
	sortResults(expectedEndorsersResults)
	sortResults(actualEndorsersResults)

	require.Equal(len(expectedEndorsersResults), len(actualEndorsersResults))
	for i, expected := range expectedEndorsersResults {
		require.Equal(expected.ReceivedAtBlockHeight, actualEndorsersResults[i].ReceivedAtBlockHeight)
		require.True(proto.Equal(expected.PvtSimulationResultsWithConfig, actualEndorsersResults[i].PvtSimulationResultsWithConfig))
	}

	// Get the minimum block height remaining in transient store
	var actualMinTransientBlkHt uint64
	actualMinTransientBlkHt, err = testStore.GetMinTransientBlkHt()
	require.NoError(err)
	require.Equal(minTransientBlkHtToRetain, actualMinTransientBlkHt)

	// Retain results at block height greater than or equal to 15
	minTransientBlkHtToRetain = uint64(15)
	err = testStore.PurgeBelowHeight(minTransientBlkHtToRetain)
	require.NoError(err)

	// There should be no entries in the  store
	actualMinTransientBlkHt, err = testStore.GetMinTransientBlkHt()
	require.Equal(err, ErrStoreEmpty)
	require.Equal(uint64(0), actualMinTransientBlkHt)

	// Retain results at block height greater than or equal to 15
	minTransientBlkHtToRetain = uint64(15)
	err = testStore.PurgeBelowHeight(minTransientBlkHtToRetain)
	// Should not return any error
	require.NoError(err)
}

func TestTransientStoreRetrievalWithFilter(t *testing.T) {
	env := initTestEnv(t)
	testStore := env.store

	samplePvtSimResWithConfig := samplePvtDataWithConfigInfo(t)

	testTxid := "testTxid"
	numEntries := 5
	for i := 0; i < numEntries; i++ {
		testStore.Persist(testTxid, uint64(i), samplePvtSimResWithConfig)
	}

	filter := ledger.NewPvtNsCollFilter()
	filter.Add("ns-1", "coll-1")
	filter.Add("ns-2", "coll-2")

	itr, err := testStore.GetTxPvtRWSetByTxid(testTxid, filter)
	require.NoError(t, err)

	var actualRes []*EndorserPvtSimulationResults
	for {
		res, err := itr.Next()
		if res == nil || err != nil {
			require.NoError(t, err)
			break
		}
		actualRes = append(actualRes, res)
	}

	// prepare the trimmed pvtrwset manually - retain only "ns-1/coll-1" and "ns-2/coll-2"
	expectedSimulationRes := samplePvtSimResWithConfig
	expectedSimulationRes.GetPvtRwset().NsPvtRwset[0].CollectionPvtRwset = expectedSimulationRes.GetPvtRwset().NsPvtRwset[0].CollectionPvtRwset[0:1]
	expectedSimulationRes.GetPvtRwset().NsPvtRwset[1].CollectionPvtRwset = expectedSimulationRes.GetPvtRwset().NsPvtRwset[1].CollectionPvtRwset[1:]
	expectedSimulationRes.CollectionConfigs, err = trimPvtCollectionConfigs(expectedSimulationRes.CollectionConfigs, filter)
	require.NoError(t, err)
	for ns, colName := range map[string]string{"ns-1": "coll-1", "ns-2": "coll-2"} {
		config := expectedSimulationRes.CollectionConfigs[ns]
		require.NotNil(t, config)
		ns1Config := config.Config
		require.Equal(t, len(ns1Config), 1)
		ns1ColConfig := ns1Config[0].GetStaticCollectionConfig()
		require.NotNil(t, ns1ColConfig.Name, colName)
	}

	var expectedRes []*EndorserPvtSimulationResults
	for i := 0; i < numEntries; i++ {
		expectedRes = append(expectedRes, &EndorserPvtSimulationResults{uint64(i), expectedSimulationRes})
	}

	// Note that the ordering of actualRes and expectedRes is dependent on the uuid. Hence, we are sorting
	// expectedRes and actualRes.
	sortResults(expectedRes)
	sortResults(actualRes)
	require.Equal(t, len(expectedRes), len(actualRes))
	for i, expected := range expectedRes {
		require.Equal(t, expected.ReceivedAtBlockHeight, actualRes[i].ReceivedAtBlockHeight)
		require.True(t, proto.Equal(expected.PvtSimulationResultsWithConfig, actualRes[i].PvtSimulationResultsWithConfig))
	}
}

func sortResults(res []*EndorserPvtSimulationResults) {
	// Results are sorted by ascending order of received at block height. When the block
	// heights are same, we sort by comparing the hash of private write set.
	sortCondition := func(i, j int) bool {
		if res[i].ReceivedAtBlockHeight == res[j].ReceivedAtBlockHeight {
			resI, _ := proto.Marshal(res[i].PvtSimulationResultsWithConfig)
			resJ, _ := proto.Marshal(res[j].PvtSimulationResultsWithConfig)
			// if hashes are same, any order would work.
			return string(util.ComputeHash(resI)) < string(util.ComputeHash(resJ))
		}
		return res[i].ReceivedAtBlockHeight < res[j].ReceivedAtBlockHeight
	}
	sort.SliceStable(res, sortCondition)
}

func samplePvtData(t *testing.T) *rwset.TxPvtReadWriteSet {
	pvtWriteSet := &rwset.TxPvtReadWriteSet{DataModel: rwset.TxReadWriteSet_KV}
	pvtWriteSet.NsPvtRwset = []*rwset.NsPvtReadWriteSet{
		{
			Namespace: "ns-1",
			CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
				{
					CollectionName: "coll-1",
					Rwset:          []byte("RandomBytes-PvtRWSet-ns1-coll1"),
				},
				{
					CollectionName: "coll-2",
					Rwset:          []byte("RandomBytes-PvtRWSet-ns1-coll2"),
				},
			},
		},

		{
			Namespace: "ns-2",
			CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
				{
					CollectionName: "coll-1",
					Rwset:          []byte("RandomBytes-PvtRWSet-ns2-coll1"),
				},
				{
					CollectionName: "coll-2",
					Rwset:          []byte("RandomBytes-PvtRWSet-ns2-coll2"),
				},
			},
		},
	}
	return pvtWriteSet
}

func samplePvtDataWithConfigInfo(t *testing.T) *transientstore.TxPvtReadWriteSetWithConfigInfo {
	pvtWriteSet := samplePvtData(t)
	pvtRWSetWithConfigInfo := &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: pvtWriteSet,
		CollectionConfigs: map[string]*peer.CollectionConfigPackage{
			"ns-1": {
				Config: []*peer.CollectionConfig{
					sampleCollectionConfigPackage("coll-1"),
					sampleCollectionConfigPackage("coll-2"),
				},
			},
			"ns-2": {
				Config: []*peer.CollectionConfig{
					sampleCollectionConfigPackage("coll-1"),
					sampleCollectionConfigPackage("coll-2"),
				},
			},
		},
	}
	return pvtRWSetWithConfigInfo
}

func createCollectionConfig(collectionName string, signaturePolicyEnvelope *common.SignaturePolicyEnvelope,
	requiredPeerCount int32, maximumPeerCount int32,
) *peer.CollectionConfig {
	signaturePolicy := &peer.CollectionPolicyConfig_SignaturePolicy{
		SignaturePolicy: signaturePolicyEnvelope,
	}
	accessPolicy := &peer.CollectionPolicyConfig{
		Payload: signaturePolicy,
	}

	return &peer.CollectionConfig{
		Payload: &peer.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &peer.StaticCollectionConfig{
				Name:              collectionName,
				MemberOrgsPolicy:  accessPolicy,
				RequiredPeerCount: requiredPeerCount,
				MaximumPeerCount:  maximumPeerCount,
			},
		},
	}
}

func sampleCollectionConfigPackage(colName string) *peer.CollectionConfig {
	signers := [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := policydsl.Envelope(policydsl.Or(policydsl.SignedBy(0), policydsl.SignedBy(1)), signers)

	var requiredPeerCount, maximumPeerCount int32
	requiredPeerCount = 1
	maximumPeerCount = 2

	return createCollectionConfig(colName, policyEnvelope, requiredPeerCount, maximumPeerCount)
}

// persistOldProto is the code from 1.1 to populate stores with old proto message
// this is used only for testing
func (s *Store) persistOldProto(txid string, blockHeight uint64,
	privateSimulationResults *rwset.TxPvtReadWriteSet) error {
	logger.Debugf("Persisting private data to transient store for txid [%s] at block height [%d]", txid, blockHeight)

	dbBatch := s.db.NewUpdateBatch()

	// Create compositeKey with appropriate prefix, txid, uuid and blockHeight
	// Due to the fact that the txid may have multiple private write sets persisted from different
	// endorsers (via Gossip), we postfix an uuid with the txid to avoid collision.
	uuid := commonutil.GenerateUUID()
	compositeKeyPvtRWSet := createCompositeKeyForPvtRWSet(txid, uuid, blockHeight)
	privateSimulationResultsBytes, err := proto.Marshal(privateSimulationResults)
	if err != nil {
		return err
	}
	dbBatch.Put(compositeKeyPvtRWSet, privateSimulationResultsBytes)

	// Create two index: (i) by txid, and (ii) by height

	// Create compositeKey for purge index by height with appropriate prefix, blockHeight,
	// txid, uuid and store the compositeKey (purge index) with a nil byte as value. Note that
	// the purge index is used to remove orphan entries in the transient store (which are not removed
	// by PurgeTxids()) using BTL policy by PurgeBelowHeight(). Note that orphan entries are due to transaction
	// that gets endorsed but not submitted by the client for commit)
	compositeKeyPurgeIndexByHeight := createCompositeKeyForPurgeIndexByHeight(blockHeight, txid, uuid)
	dbBatch.Put(compositeKeyPurgeIndexByHeight, emptyValue)

	// Create compositeKey for purge index by txid with appropriate prefix, txid, uuid,
	// blockHeight and store the compositeKey (purge index) with a nil byte as value.
	// Though compositeKeyPvtRWSet itself can be used to purge private write set by txid,
	// we create a separate composite key with a nil byte as value. The reason is that
	// if we use compositeKeyPvtRWSet, we unnecessarily read (potentially large) private write
	// set associated with the key from db. Note that this purge index is used to remove non-orphan
	// entries in the transient store and is used by PurgeTxids()
	// Note: We can create compositeKeyPurgeIndexByTxid by just replacing the prefix of compositeKeyPvtRWSet
	// with purgeIndexByTxidPrefix. For code readability and to be expressive, we use a
	// createCompositeKeyForPurgeIndexByTxid() instead.
	compositeKeyPurgeIndexByTxid := createCompositeKeyForPurgeIndexByTxid(txid, uuid, blockHeight)
	dbBatch.Put(compositeKeyPurgeIndexByTxid, emptyValue)

	return s.db.WriteBatch(dbBatch, true)
}

func TestIteratorErrorCases(t *testing.T) {
	env := initTestEnv(t)
	testStore := env.store
	env.storeProvider.Close()

	errStr := "internal leveldb error while obtaining db iterator: leveldb: closed"
	itr, err := testStore.GetTxPvtRWSetByTxid("tx1", nil)
	require.EqualError(t, err, errStr)
	require.Nil(t, itr)

	minHt, err := testStore.GetMinTransientBlkHt()
	require.EqualError(t, err, errStr)
	require.Equal(t, uint64(0), minHt)

	require.EqualError(t, testStore.PurgeBelowHeight(0), errStr)
	require.EqualError(t, testStore.PurgeByTxids([]string{"tx1"}), errStr)
}

func TestDeleteTransientStore(t *testing.T) {
	env := initTestEnv(t)

	ledgerID := "test-deleted-tx-count"
	store, err := env.storeProvider.OpenStore(ledgerID)
	require.NoError(t, err)
	require.NotNil(t, store)

	// Write some transactions into the transient storage.
	samplePvtSimResWithConfig := samplePvtDataWithConfigInfo(t)
	testTxid := "testTxid"
	numEntries := 5
	for i := 0; i < numEntries; i++ {
		store.Persist(testTxid, uint64(i), samplePvtSimResWithConfig)
	}

	height, err := store.GetMinTransientBlkHt()
	require.NoError(t, err)
	require.Equal(t, uint64(0), height)

	// undercut a few blocks, and we should still have a lower tx bound.
	require.NoError(t, store.PurgeBelowHeight(3))
	height, err = store.GetMinTransientBlkHt()
	require.NoError(t, err)
	require.Equal(t, uint64(3), height)

	// Delete the store
	sp := env.storeProvider.(*storeProvider)
	require.NoError(t, sp.deleteStore(ledgerID))

	// After delete the storage should be empty.
	height, err = store.GetMinTransientBlkHt()
	require.EqualError(t, err, "Transient store is empty")
	require.Equal(t, uint64(0), height)

	isEmpty, err := store.db.IsEmpty()
	require.NoError(t, err)
	require.True(t, isEmpty)
}

func TestDeleteMissingTransientStoreIsOK(t *testing.T) {
	env := initTestEnv(t)

	sp := env.storeProvider.(*storeProvider)
	require.NoError(t, sp.deleteStore("_not_a_valid_store"))
}
