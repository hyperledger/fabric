/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transientstore

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/ledger/rwset"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/core/transientdata")
	os.Exit(m.Run())
}

func TestPurgeIndexKeyCodingEncoding(t *testing.T) {
	assert := assert.New(t)
	blkHts := []uint64{0, 10, 20000}
	txids := []string{"txid", ""}
	uuids := []string{"uuid", ""}
	for _, blkHt := range blkHts {
		for _, txid := range txids {
			for _, uuid := range uuids {
				testCase := fmt.Sprintf("blkHt=%d,txid=%s,uuid=%s", blkHt, txid, uuid)
				t.Run(testCase, func(t *testing.T) {
					t.Logf("Running test case [%s]", testCase)
					purgeIndexKey := createCompositeKeyForPurgeIndex(blkHt, txid, uuid)
					txid1, uuid1, blkHt1 := splitCompositeKeyOfPurgeIndex(purgeIndexKey)
					assert.Equal(txid, txid1)
					assert.Equal(uuid, uuid1)
					assert.Equal(blkHt, blkHt1)
				})
			}
		}
	}
}

func TestRWSetKeyCodingEncoding(t *testing.T) {
	assert := assert.New(t)
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
					uuid1, blkHt1 := splitCompositeKeyOfPvtRWSet(rwsetKey)
					assert.Equal(uuid, uuid1)
					assert.Equal(blkHt, blkHt1)
				})
			}
		}
	}
}

func TestTransientStorePersistAndRetrieve(t *testing.T) {
	env := NewTestStoreEnv(t)
	assert := assert.New(t)
	txid := "txid-1"
	samplePvtRWSet := samplePvtData(t)

	// Create private simulation results for txid-1
	var endorsersResults []*EndorserPvtSimulationResults

	// Results produced by endorser 1
	endorser0SimulationResults := &EndorserPvtSimulationResults{
		EndorsementBlockHeight: 10,
		PvtSimulationResults:   samplePvtRWSet,
	}
	endorsersResults = append(endorsersResults, endorser0SimulationResults)

	// Results produced by endorser 2
	endorser1SimulationResults := &EndorserPvtSimulationResults{
		EndorsementBlockHeight: 10,
		PvtSimulationResults:   samplePvtRWSet,
	}
	endorsersResults = append(endorsersResults, endorser1SimulationResults)

	// Persist simulation results into  store
	var err error
	for i := 0; i < len(endorsersResults); i++ {
		err = env.TestStore.Persist(txid, endorsersResults[i].EndorsementBlockHeight,
			endorsersResults[i].PvtSimulationResults)
		assert.NoError(err)
	}

	// Retrieve simulation results of txid-1 from  store
	iter, err := env.TestStore.GetTxPvtRWSetByTxid(txid, nil)
	assert.NoError(err)

	var actualEndorsersResults []*EndorserPvtSimulationResults
	for {
		result, err := iter.Next()
		assert.NoError(err)
		if result == nil {
			break
		}
		actualEndorsersResults = append(actualEndorsersResults, result)
	}
	iter.Close()
	sortResults(endorsersResults)
	sortResults(actualEndorsersResults)
	assert.Equal(endorsersResults, actualEndorsersResults)
}

func TestStorePurge(t *testing.T) {
	env := NewTestStoreEnv(t)
	assert := assert.New(t)

	txid := "txid-1"
	samplePvtRWSet := samplePvtData(t)

	// Create private simulation results for txid-1
	var endorsersResults []*EndorserPvtSimulationResults

	// Results produced by endorser 1
	endorser0SimulationResults := &EndorserPvtSimulationResults{
		EndorsementBlockHeight: 10,
		PvtSimulationResults:   samplePvtRWSet,
	}
	endorsersResults = append(endorsersResults, endorser0SimulationResults)

	// Results produced by endorser 2
	endorser1SimulationResults := &EndorserPvtSimulationResults{
		EndorsementBlockHeight: 11,
		PvtSimulationResults:   samplePvtRWSet,
	}
	endorsersResults = append(endorsersResults, endorser1SimulationResults)

	// Results produced by endorser 3
	endorser2SimulationResults := &EndorserPvtSimulationResults{
		EndorsementBlockHeight: 12,
		PvtSimulationResults:   samplePvtRWSet,
	}
	endorsersResults = append(endorsersResults, endorser2SimulationResults)

	// Results produced by endorser 3
	endorser3SimulationResults := &EndorserPvtSimulationResults{
		EndorsementBlockHeight: 12,
		PvtSimulationResults:   samplePvtRWSet,
	}
	endorsersResults = append(endorsersResults, endorser3SimulationResults)

	// Results produced by endorser 3
	endorser4SimulationResults := &EndorserPvtSimulationResults{
		EndorsementBlockHeight: 13,
		PvtSimulationResults:   samplePvtRWSet,
	}
	endorsersResults = append(endorsersResults, endorser4SimulationResults)

	// Persist simulation results into  store
	var err error
	for i := 0; i < 5; i++ {
		err = env.TestStore.Persist(txid, endorsersResults[i].EndorsementBlockHeight,
			endorsersResults[i].PvtSimulationResults)
		assert.NoError(err)
	}

	// Retain results generate at block height greater than or equal to 12
	minEndorsementBlkHtToRetain := uint64(12)
	err = env.TestStore.Purge(minEndorsementBlkHtToRetain)
	assert.NoError(err)

	// Retrieve simulation results of txid-1 from  store
	iter, err := env.TestStore.GetTxPvtRWSetByTxid(txid, nil)
	assert.NoError(err)

	// Expected results for txid-1
	var expectedEndorsersResults []*EndorserPvtSimulationResults
	expectedEndorsersResults = append(expectedEndorsersResults, endorser2SimulationResults) //endorsed at height 12
	expectedEndorsersResults = append(expectedEndorsersResults, endorser3SimulationResults) //endorsed at height 12
	expectedEndorsersResults = append(expectedEndorsersResults, endorser4SimulationResults) //endorsed at height 13

	// Check whether actual results and expected results are same
	var actualEndorsersResults []*EndorserPvtSimulationResults
	for true {
		result, err := iter.Next()
		assert.NoError(err)
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

	assert.Equal(expectedEndorsersResults, actualEndorsersResults)

	// Get the minimum retained endorsement block height
	var actualMinEndorsementBlkHt uint64
	actualMinEndorsementBlkHt, err = env.TestStore.GetMinEndorsementBlkHt()
	assert.NoError(err)
	assert.Equal(minEndorsementBlkHtToRetain, actualMinEndorsementBlkHt)

	// Retain results generate at block height greater than or equal to 15
	minEndorsementBlkHtToRetain = uint64(15)
	err = env.TestStore.Purge(minEndorsementBlkHtToRetain)
	assert.NoError(err)

	// There should be no entries in the  store
	actualMinEndorsementBlkHt, err = env.TestStore.GetMinEndorsementBlkHt()
	assert.Equal(err, ErrStoreEmpty)

	// Retain results generate at block height greater than or equal to 15
	minEndorsementBlkHtToRetain = uint64(15)
	err = env.TestStore.Purge(minEndorsementBlkHtToRetain)
	// Should not return any error
	assert.NoError(err)

	env.Cleanup()
}

func TestTransientStoreRetrievalWithFilter(t *testing.T) {
	env := NewTestStoreEnv(t)
	store := env.TestStore

	samplePvtSimRes := samplePvtData(t)

	testTxid := "testTxid"
	numEntries := 5
	for i := 0; i < numEntries; i++ {
		store.Persist(testTxid, uint64(i), samplePvtSimRes)
	}

	filter := ledger.NewPvtNsCollFilter()
	filter.Add("ns-1", "coll-1")
	filter.Add("ns-2", "coll-2")

	itr, err := store.GetTxPvtRWSetByTxid(testTxid, filter)
	assert.NoError(t, err)

	var actualRes []*EndorserPvtSimulationResults
	for {
		res, err := itr.Next()
		if res == nil || err != nil {
			assert.NoError(t, err)
			break
		}
		actualRes = append(actualRes, res)
	}

	// prepare the trimmed pvtrwset manually - retain only "ns-1/coll-1" and "ns-2/coll-2"
	expectedSimulationRes := samplePvtSimRes
	expectedSimulationRes.NsPvtRwset[0].CollectionPvtRwset = expectedSimulationRes.NsPvtRwset[0].CollectionPvtRwset[0:1]
	expectedSimulationRes.NsPvtRwset[1].CollectionPvtRwset = expectedSimulationRes.NsPvtRwset[1].CollectionPvtRwset[1:]

	var expectedRes []*EndorserPvtSimulationResults
	for i := 0; i < numEntries; i++ {
		expectedRes = append(expectedRes, &EndorserPvtSimulationResults{uint64(i), expectedSimulationRes})
	}

	// Note that the ordering of actualRes and expectedRes is dependent on the uuid. Hence, we are sorting
	// expectedRes and actualRes.
	sortResults(expectedRes)
	sortResults(actualRes)
	assert.Equal(t, expectedRes, actualRes)
	t.Logf("Actual Res = %s", spew.Sdump(actualRes))
}

func sortResults(res []*EndorserPvtSimulationResults) {
	// Results are sorted by ascending order of endorsement block height. When the endorsement block
	// heights are same, we sort by comparing the hash of private write set.
	var sortCondition = func(i, j int) bool {
		if res[i].EndorsementBlockHeight == res[j].EndorsementBlockHeight {
			res_i, _ := proto.Marshal(res[i].PvtSimulationResults)
			res_j, _ := proto.Marshal(res[j].PvtSimulationResults)
			// if hashes are same, any order would work.
			return string(util.ComputeHash(res_i)) < string(util.ComputeHash(res_j))
		}
		return res[i].EndorsementBlockHeight < res[j].EndorsementBlockHeight
	}
	sort.SliceStable(res, sortCondition)
}

func samplePvtData(t *testing.T) *rwset.TxPvtReadWriteSet {
	pvtWriteSet := &rwset.TxPvtReadWriteSet{DataModel: rwset.TxReadWriteSet_KV}
	pvtWriteSet.NsPvtRwset = []*rwset.NsPvtReadWriteSet{
		&rwset.NsPvtReadWriteSet{
			Namespace: "ns-1",
			CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
				&rwset.CollectionPvtReadWriteSet{
					CollectionName: "coll-1",
					Rwset:          []byte("RandomBytes-PvtRWSet-ns1-coll1"),
				},
				&rwset.CollectionPvtReadWriteSet{
					CollectionName: "coll-2",
					Rwset:          []byte("RandomBytes-PvtRWSet-ns1-coll2"),
				},
			},
		},

		&rwset.NsPvtReadWriteSet{
			Namespace: "ns-2",
			CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
				&rwset.CollectionPvtReadWriteSet{
					CollectionName: "coll-1",
					Rwset:          []byte("RandomBytes-PvtRWSet-ns2-coll1"),
				},
				&rwset.CollectionPvtReadWriteSet{
					CollectionName: "coll-2",
					Rwset:          []byte("RandomBytes-PvtRWSet-ns2-coll2"),
				},
			},
		},
	}
	return pvtWriteSet
}
