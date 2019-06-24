/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/stretchr/testify/assert"
)

func TestStateListener(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	provider, _ := NewProvider()
	defer provider.Close()

	// create a listener and register it to listen to state change in a namespace
	channelid := "testLedger"
	namespace := "testchaincode"
	mockListener := &mockStateListener{namespace: namespace}
	provider.Initialize(&ledger.Initializer{
		DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
		StateListeners:                []ledger.StateListener{mockListener},
		MetricsProvider:               &disabled.Provider{},
	})

	bg, gb := testutil.NewBlockGenerator(t, channelid, false)
	lgr, err := provider.Create(gb)
	defer lgr.Close()

	// Simulate tx1
	sim1, err := lgr.NewTxSimulator("test_tx_1")
	assert.NoError(t, err)
	sim1.GetState(namespace, "key1")
	sim1.SetState(namespace, "key1", []byte("value1"))
	sim1.SetState(namespace, "key2", []byte("value2"))
	sim1.Done()

	// Simulate tx2 - this has a conflict with tx1 because it reads "key1"
	sim2, err := lgr.NewTxSimulator("test_tx_2")
	assert.NoError(t, err)
	sim2.GetState(namespace, "key1")
	sim2.SetState(namespace, "key3", []byte("value3"))
	sim2.Done()

	// Simulate tx3 - this neighter conflicts with tx1 nor with tx2
	sim3, err := lgr.NewTxSimulator("test_tx_3")
	assert.NoError(t, err)
	sim3.SetState(namespace, "key4", []byte("value4"))
	sim3.Done()

	// commit tx1 and this should cause mock listener to recieve the state changes made by tx1
	mockListener.reset()
	sim1Res, _ := sim1.GetTxSimulationResults()
	sim1ResBytes, _ := sim1Res.GetPubSimulationBytes()
	assert.NoError(t, err)
	blk1 := bg.NextBlock([][]byte{sim1ResBytes})
	assert.NoError(t, lgr.CommitWithPvtData(&ledger.BlockAndPvtData{Block: blk1}, &ledger.CommitOptions{}))
	assert.Equal(t, channelid, mockListener.channelName)
	assert.Contains(t, mockListener.kvWrites, &kvrwset.KVWrite{Key: "key1", Value: []byte("value1")})
	assert.Contains(t, mockListener.kvWrites, &kvrwset.KVWrite{Key: "key2", Value: []byte("value2")})
	// commit tx2 and this should not cause mock listener to recieve the state changes made by tx2
	// (because, tx2 should be found as invalid)
	mockListener.reset()
	sim2Res, _ := sim2.GetTxSimulationResults()
	sim2ResBytes, _ := sim2Res.GetPubSimulationBytes()
	assert.NoError(t, err)
	blk2 := bg.NextBlock([][]byte{sim2ResBytes})
	assert.NoError(t, lgr.CommitWithPvtData(&ledger.BlockAndPvtData{Block: blk2}, &ledger.CommitOptions{}))
	assert.Equal(t, "", mockListener.channelName)
	assert.Nil(t, mockListener.kvWrites)

	// commit tx3 and thsi should cause mock listener to recieve changes made by tx3
	mockListener.reset()
	sim3Res, _ := sim3.GetTxSimulationResults()
	sim3ResBytes, _ := sim3Res.GetPubSimulationBytes()
	assert.NoError(t, err)
	blk3 := bg.NextBlock([][]byte{sim3ResBytes})
	assert.NoError(t, lgr.CommitWithPvtData(&ledger.BlockAndPvtData{Block: blk3}, &ledger.CommitOptions{}))
	assert.Equal(t, channelid, mockListener.channelName)
	assert.Equal(t, []*kvrwset.KVWrite{
		{Key: "key4", Value: []byte("value4")},
	}, mockListener.kvWrites)
}

type mockStateListener struct {
	channelName string
	namespace   string
	kvWrites    []*kvrwset.KVWrite
}

func (l *mockStateListener) InterestedInNamespaces() []string {
	return []string{l.namespace}
}

func (l *mockStateListener) HandleStateUpdates(trigger *ledger.StateUpdateTrigger) error {
	channelName, stateUpdates := trigger.LedgerID, trigger.StateUpdates
	l.channelName = channelName
	l.kvWrites = stateUpdates[l.namespace].([]*kvrwset.KVWrite)
	return nil
}

func (l *mockStateListener) StateCommitDone(channelID string) {
	// NOOP
}

func (l *mockStateListener) reset() {
	l.channelName = ""
	l.kvWrites = nil
}
