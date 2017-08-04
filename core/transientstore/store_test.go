/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package transientstore

import (
	"fmt"
	"os"
	"testing"

	commonledger "github.com/hyperledger/fabric/common/ledger"
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
	endorserids := []string{"endorserid", ""}
	for _, blkHt := range blkHts {
		for _, txid := range txids {
			for _, endorserid := range endorserids {
				testCase := fmt.Sprintf("blkHt=%d,txid=%s,endorserid=%s", blkHt, txid, endorserid)
				t.Run(testCase, func(t *testing.T) {
					t.Logf("Running test case [%s]", testCase)
					purgeIndexKey := createCompositeKeyForPurgeIndex(blkHt, txid, endorserid)
					txid1, endorserid1, blkHt1 := splitCompositeKeyOfPurgeIndex(purgeIndexKey)
					assert.Equal(txid, txid1)
					assert.Equal(endorserid, endorserid1)
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
	endorserids := []string{"endorserid", ""}
	for _, blkHt := range blkHts {
		for _, txid := range txids {
			for _, endorserid := range endorserids {
				testCase := fmt.Sprintf("blkHt=%d,txid=%s,endorserid=%s", blkHt, txid, endorserid)
				t.Run(testCase, func(t *testing.T) {
					t.Logf("Running test case [%s]", testCase)
					rwsetKey := createCompositeKeyForPvtRWSet(txid, endorserid, blkHt)
					endorserid1, blkHt1 := splitCompositeKeyOfPvtRWSet(rwsetKey)
					assert.Equal(endorserid, endorserid1)
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

	// Create private simulation results for txid-1
	var endorsersResults []*EndorserPvtSimulationResults

	// Results produced by endorser 1
	endorser0SimulationResults := &EndorserPvtSimulationResults{
		EndorserID:             "endorser0",
		EndorsementBlockHeight: 10,
		PvtSimulationResults:   []byte("results"),
	}
	endorsersResults = append(endorsersResults, endorser0SimulationResults)

	// Results produced by endorser 2
	endorser1SimulationResults := &EndorserPvtSimulationResults{
		EndorserID:             "endorser1",
		EndorsementBlockHeight: 10,
		PvtSimulationResults:   []byte("results"),
	}
	endorsersResults = append(endorsersResults, endorser1SimulationResults)

	// Persist simulation results into  store
	var err error
	for i := 0; i < len(endorsersResults); i++ {
		err = env.TestStore.Persist(txid, endorsersResults[i].EndorserID,
			endorsersResults[i].EndorsementBlockHeight, endorsersResults[i].PvtSimulationResults)
		assert.NoError(err)
	}

	// Retrieve simulation results of txid-1 from  store
	var iter commonledger.ResultsIterator
	iter, err = env.TestStore.GetTxPvtRWSetByTxid(txid)
	assert.NoError(err)

	var result commonledger.QueryResult
	var actualEndorsersResults []*EndorserPvtSimulationResults
	for true {
		result, err = iter.Next()
		assert.NoError(err)
		if result == nil {
			break
		}
		actualEndorsersResults = append(actualEndorsersResults, result.(*EndorserPvtSimulationResults))
	}
	iter.Close()
	assert.Equal(endorsersResults, actualEndorsersResults)

	// No selfSimulated results for the txid. Result and err should be nil
	endorserResults, err := env.TestStore.GetSelfSimulatedTxPvtRWSetByTxid(txid)
	assert.NoError(err)
	assert.Nil(endorserResults)

	// Self simulated results
	selfSimulatedResults := &EndorserPvtSimulationResults{
		EndorserID:             "",
		EndorsementBlockHeight: 10,
		PvtSimulationResults:   []byte("results"),
	}

	// Persist self simulated results
	err = env.TestStore.Persist(txid, selfSimulatedResults.EndorserID,
		selfSimulatedResults.EndorsementBlockHeight, selfSimulatedResults.PvtSimulationResults)
	assert.NoError(err)

	// Retrieve self simulated results
	endorserResults, err = env.TestStore.GetSelfSimulatedTxPvtRWSetByTxid(txid)
	assert.Equal(selfSimulatedResults, endorserResults)
}

func TestStorePurge(t *testing.T) {
	env := NewTestStoreEnv(t)
	assert := assert.New(t)

	txid := "txid-1"

	// Create private simulation results for txid-1
	var endorsersResults []*EndorserPvtSimulationResults

	// Results produced by endorser 1
	endorser0SimulationResults := &EndorserPvtSimulationResults{
		EndorserID:             "endorser0",
		EndorsementBlockHeight: 10,
		PvtSimulationResults:   []byte("results"),
	}
	endorsersResults = append(endorsersResults, endorser0SimulationResults)

	// Results produced by endorser 2
	endorser1SimulationResults := &EndorserPvtSimulationResults{
		EndorserID:             "endorser1",
		EndorsementBlockHeight: 11,
		PvtSimulationResults:   []byte("results"),
	}
	endorsersResults = append(endorsersResults, endorser1SimulationResults)

	// Results produced by endorser 3
	endorser2SimulationResults := &EndorserPvtSimulationResults{
		EndorserID:             "endorser2",
		EndorsementBlockHeight: 12,
		PvtSimulationResults:   []byte("results"),
	}
	endorsersResults = append(endorsersResults, endorser2SimulationResults)

	// Results produced by endorser 3
	endorser3SimulationResults := &EndorserPvtSimulationResults{
		EndorserID:             "endorser3",
		EndorsementBlockHeight: 12,
		PvtSimulationResults:   []byte("results"),
	}
	endorsersResults = append(endorsersResults, endorser3SimulationResults)

	// Results produced by endorser 3
	endorser4SimulationResults := &EndorserPvtSimulationResults{
		EndorserID:             "endorser4",
		EndorsementBlockHeight: 13,
		PvtSimulationResults:   []byte("results"),
	}
	endorsersResults = append(endorsersResults, endorser4SimulationResults)

	// Persist simulation results into  store
	var err error
	for i := 0; i < 5; i++ {
		err = env.TestStore.Persist(txid, endorsersResults[i].EndorserID,
			endorsersResults[i].EndorsementBlockHeight, endorsersResults[i].PvtSimulationResults)
		assert.NoError(err)
	}

	// Retain results generate at block height greater than or equal to 12
	minEndorsementBlkHtToRetain := uint64(12)
	err = env.TestStore.Purge(minEndorsementBlkHtToRetain)
	assert.NoError(err)

	// Retrieve simulation results of txid-1 from  store
	var iter commonledger.ResultsIterator
	iter, err = env.TestStore.GetTxPvtRWSetByTxid(txid)
	assert.NoError(err)

	// Expected results for txid-1
	var expectedEndorsersResults []*EndorserPvtSimulationResults
	expectedEndorsersResults = append(expectedEndorsersResults, endorser2SimulationResults) //endorsed at height 12
	expectedEndorsersResults = append(expectedEndorsersResults, endorser3SimulationResults) //endorsed at height 12
	expectedEndorsersResults = append(expectedEndorsersResults, endorser4SimulationResults) //endorsed at height 13

	// Check whether actual results and expected results are same
	var result commonledger.QueryResult
	var actualEndorsersResults []*EndorserPvtSimulationResults
	for true {
		result, err = iter.Next()
		assert.NoError(err)
		if result == nil {
			break
		}
		actualEndorsersResults = append(actualEndorsersResults, result.(*EndorserPvtSimulationResults))
	}
	iter.Close()
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
