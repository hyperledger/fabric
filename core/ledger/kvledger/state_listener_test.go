/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

func TestStateListener(t *testing.T) {
	conf := testConfig(t)

	// create a listener and register it to listen to state change in a namespace
	channelid := "testLedger"
	namespace := "testchaincode"
	mockListener := &mockStateListener{namespace: namespace}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	provider, err := NewProvider(
		&ledger.Initializer{
			DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
			StateListeners:                []ledger.StateListener{mockListener},
			MetricsProvider:               &disabled.Provider{},
			Config:                        conf,
			HashProvider:                  cryptoProvider,
		},
	)
	if err != nil {
		t.Fatalf("Failed to create new Provider: %s", err)
	}

	bg, gb := testutil.NewBlockGenerator(t, channelid, false)
	lgr, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	// Simulate tx1
	sim1, err := lgr.NewTxSimulator("test_tx_1")
	require.NoError(t, err)
	_, err = sim1.GetState(namespace, "key1")
	require.NoError(t, err)
	require.NoError(t, sim1.SetState(namespace, "key1", []byte("value1")))
	require.NoError(t, sim1.SetState(namespace, "key2", []byte("value2")))
	sim1.Done()

	// Simulate tx2 - this has a conflict with tx1 because it reads "key1"
	sim2, err := lgr.NewTxSimulator("test_tx_2")
	require.NoError(t, err)
	_, err = sim2.GetState(namespace, "key1")
	require.NoError(t, err)
	require.NoError(t, sim2.SetState(namespace, "key3", []byte("value3")))
	sim2.Done()

	// Simulate tx3 - this neighter conflicts with tx1 nor with tx2
	sim3, err := lgr.NewTxSimulator("test_tx_3")
	require.NoError(t, err)
	require.NoError(t, sim3.SetState(namespace, "key4", []byte("value4")))
	sim3.Done()

	// commit tx1 and this should cause mock listener to receive the state changes made by tx1
	mockListener.reset()
	sim1Res, _ := sim1.GetTxSimulationResults()
	sim1ResBytes, _ := sim1Res.GetPubSimulationBytes()
	require.NoError(t, err)
	blk1 := bg.NextBlock([][]byte{sim1ResBytes})
	require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: blk1}, &ledger.CommitOptions{}))
	require.Equal(t, channelid, mockListener.channelName)
	require.Contains(t, mockListener.kvWrites, &kvrwset.KVWrite{Key: "key1", Value: []byte("value1")})
	require.Contains(t, mockListener.kvWrites, &kvrwset.KVWrite{Key: "key2", Value: []byte("value2")})
	// commit tx2 and this should not cause mock listener to receive the state changes made by tx2
	// (because, tx2 should be found as invalid)
	mockListener.reset()
	sim2Res, _ := sim2.GetTxSimulationResults()
	sim2ResBytes, _ := sim2Res.GetPubSimulationBytes()
	require.NoError(t, err)
	blk2 := bg.NextBlock([][]byte{sim2ResBytes})
	require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: blk2}, &ledger.CommitOptions{}))
	require.Equal(t, "", mockListener.channelName)
	require.Nil(t, mockListener.kvWrites)

	// commit tx3 and this should cause mock listener to receive changes made by tx3
	mockListener.reset()
	sim3Res, _ := sim3.GetTxSimulationResults()
	sim3ResBytes, _ := sim3Res.GetPubSimulationBytes()
	require.NoError(t, err)
	blk3 := bg.NextBlock([][]byte{sim3ResBytes})
	require.NoError(t, lgr.CommitLegacy(&ledger.BlockAndPvtData{Block: blk3}, &ledger.CommitOptions{}))
	require.Equal(t, channelid, mockListener.channelName)
	require.Equal(t, []*kvrwset.KVWrite{
		{Key: "key4", Value: []byte("value4")},
	}, mockListener.kvWrites)

	provider.Close()

	provider, err = NewProvider(
		&ledger.Initializer{
			DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
			StateListeners:                []ledger.StateListener{mockListener},
			MetricsProvider:               &disabled.Provider{},
			Config:                        conf,
			HashProvider:                  cryptoProvider,
		},
	)
	if err != nil {
		t.Fatalf("Failed to create new Provider: %s", err)
	}
	defer provider.Close()
	lgr, err = provider.Open(channelid)
	require.NoError(t, err)
	defer lgr.Close()
	require.NoError(t, err)
	require.Equal(t,
		[]*queryresult.KV{
			{
				Namespace: namespace,
				Key:       "key1",
				Value:     []byte("value1"),
			},
			{
				Namespace: namespace,
				Key:       "key2",
				Value:     []byte("value2"),
			},
			{
				Namespace: namespace,
				Key:       "key4",
				Value:     []byte("value4"),
			},
		},
		mockListener.queryResultsInInitializeFunc,
	)
}

type mockStateListener struct {
	channelName                  string
	namespace                    string
	kvWrites                     []*kvrwset.KVWrite
	queryResultsInInitializeFunc []*queryresult.KV
}

func (l *mockStateListener) Name() string {
	return "mock state listener"
}

func (l *mockStateListener) Initialize(ledgerID string, qe ledger.SimpleQueryExecutor) error {
	_, err := qe.GetPrivateDataHash(l.namespace, "random-coll", "random-key")
	if err != nil {
		return err
	}
	l.channelName = ledgerID
	itr, err := qe.GetStateRangeScanIterator(l.namespace, "", "")
	if err != nil {
		return err
	}
	for {
		res, err := itr.Next()
		if err != nil {
			return err
		}
		if res == nil {
			break
		}
		kv := res.(*queryresult.KV)
		l.queryResultsInInitializeFunc = append(l.queryResultsInInitializeFunc,
			kv,
		)
	}
	return nil
}

func (l *mockStateListener) InterestedInNamespaces() []string {
	return []string{l.namespace}
}

func (l *mockStateListener) HandleStateUpdates(trigger *ledger.StateUpdateTrigger) error {
	channelName, stateUpdates := trigger.LedgerID, trigger.StateUpdates
	l.channelName = channelName
	l.kvWrites = stateUpdates[l.namespace].PublicUpdates
	return nil
}

func (l *mockStateListener) StateCommitDone(channelID string) {
	// NOOP
}

func (l *mockStateListener) reset() {
	l.channelName = ""
	l.kvWrites = nil
	l.queryResultsInInitializeFunc = nil
}
