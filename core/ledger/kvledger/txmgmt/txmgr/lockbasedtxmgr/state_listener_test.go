/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package lockbasedtxmgr

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/stretchr/testify/assert"
)

func TestStateListener(t *testing.T) {
	testLedgerid := "testLedger"
	ml1 := new(mock.StateListener)
	ml1.InterestedInNamespacesStub = func() []string { return []string{"ns1", "ns2"} }

	ml2 := new(mock.StateListener)
	ml2.InterestedInNamespacesStub = func() []string { return []string{"ns2", "ns3"} }

	ml3 := new(mock.StateListener)
	ml3.InterestedInNamespacesStub = func() []string { return []string{"ns4"} }

	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, testLedgerid, nil)
	defer testEnv.cleanup()
	txmgr := testEnv.getTxMgr().(*LockBasedTxMgr)
	txmgr.stateListeners = []ledger.StateListener{ml1, ml2, ml3}

	// Mimic commit of block 1 with updates in namespaces ns1, ns2, and ns3
	// This should cause callback to ml1 and ml2 but not to ml3
	sampleBatch := privacyenabledstate.NewUpdateBatch()
	sampleBatch.PubUpdates.Put("ns1", "key1_1", []byte("value1_1"), version.NewHeight(1, 1))
	sampleBatch.PubUpdates.Put("ns1", "key1_2", []byte("value1_2"), version.NewHeight(1, 2))
	sampleBatch.PubUpdates.Put("ns2", "key2_1", []byte("value2_1"), version.NewHeight(1, 3))
	sampleBatch.PubUpdates.Put("ns3", "key3_1", []byte("value3_1"), version.NewHeight(1, 4))
	dummyBlock := common.NewBlock(1, []byte("dummyHash"))
	txmgr.current = &current{block: dummyBlock, batch: sampleBatch}
	txmgr.invokeNamespaceListeners()
	assert.Equal(t, 1, ml1.HandleStateUpdatesCallCount())
	assert.Equal(t, 1, ml2.HandleStateUpdatesCallCount())
	assert.Equal(t, 0, ml3.HandleStateUpdatesCallCount())
	expectedLedgerid, expectedStateUpdate, expectedHt :=
		testLedgerid,
		ledger.StateUpdates{
			"ns1": []*kvrwset.KVWrite{
				{Key: "key1_1", Value: []byte("value1_1")}, {Key: "key1_2", Value: []byte("value1_2")}},
			"ns2": []*kvrwset.KVWrite{{Key: "key2_1", Value: []byte("value2_1")}},
		},
		uint64(1)
	checkHandleStateUpdatesCallback(t, ml1, 0, expectedLedgerid, expectedStateUpdate, expectedHt)
	expectedLedgerid, expectedStateUpdate, expectedHt =
		testLedgerid,
		ledger.StateUpdates{
			"ns2": []*kvrwset.KVWrite{{Key: "key2_1", Value: []byte("value2_1")}},
			"ns3": []*kvrwset.KVWrite{{Key: "key3_1", Value: []byte("value3_1")}},
		},
		uint64(1)
	checkHandleStateUpdatesCallback(t, ml2, 0, expectedLedgerid, expectedStateUpdate, expectedHt)
	txmgr.Commit()
	assert.Equal(t, 1, ml1.StateCommitDoneCallCount())
	assert.Equal(t, 1, ml2.StateCommitDoneCallCount())
	assert.Equal(t, 0, ml3.StateCommitDoneCallCount())

	// Mimic commit of block 2 with updates only in ns4 namespace
	// This should cause callback only to ml3
	sampleBatch = privacyenabledstate.NewUpdateBatch()
	sampleBatch.PubUpdates.Put("ns4", "key4_1", []byte("value4_1"), version.NewHeight(2, 1))
	txmgr.current = &current{block: common.NewBlock(2, []byte("anotherDummyHash")), batch: sampleBatch}
	txmgr.invokeNamespaceListeners()
	assert.Equal(t, 1, ml1.HandleStateUpdatesCallCount())
	assert.Equal(t, 1, ml2.HandleStateUpdatesCallCount())
	assert.Equal(t, 1, ml3.HandleStateUpdatesCallCount())

	expectedLedgerid, expectedStateUpdate, expectedHt =
		testLedgerid,
		ledger.StateUpdates{
			"ns4": []*kvrwset.KVWrite{{Key: "key4_1", Value: []byte("value4_1")}},
		},
		uint64(2)

	checkHandleStateUpdatesCallback(t, ml3, 0, expectedLedgerid, expectedStateUpdate, expectedHt)

	txmgr.Commit()
	assert.Equal(t, 1, ml1.StateCommitDoneCallCount())
	assert.Equal(t, 1, ml2.StateCommitDoneCallCount())
	assert.Equal(t, 1, ml3.StateCommitDoneCallCount())
}

func TestStateListenerQueryExecutor(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, "testLedger", nil)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr().(*LockBasedTxMgr)

	namespace := "ns"
	initialData := []*queryresult.KV{
		{Namespace: namespace, Key: "key1", Value: []byte("value1")},
		{Namespace: namespace, Key: "key2", Value: []byte("value2")},
		{Namespace: namespace, Key: "key3", Value: []byte("value3")},
	}
	// populate initial data in db
	testutilPopulateDB(t, txMgr, namespace, initialData, version.NewHeight(1, 1))

	sl := new(mock.StateListener)
	sl.InterestedInNamespacesStub = func() []string { return []string{"ns"} }
	txMgr.stateListeners = []ledger.StateListener{sl}

	// Create next block
	sim, err := txMgr.NewTxSimulator("tx1")
	assert.NoError(t, err)
	sim.SetState(namespace, "key1", []byte("value1_new"))
	sim.DeleteState(namespace, "key2")
	sim.SetState(namespace, "key4", []byte("value4_new"))
	simRes, err := sim.GetTxSimulationResults()
	simResBytes, err := simRes.GetPubSimulationBytes()
	assert.NoError(t, err)
	block := testutil.ConstructBlock(t, 1, nil, [][]byte{simResBytes}, false)

	// invoke ValidateAndPrepare function
	_, _, err = txMgr.ValidateAndPrepare(&ledger.BlockAndPvtData{Block: block}, false)
	assert.NoError(t, err)

	// validate that the query executors passed to the state listener
	trigger := sl.HandleStateUpdatesArgsForCall(0)
	assert.NotNil(t, trigger)

	expectedCommittedData := initialData
	checkQueryExecutor(t, trigger.CommittedStateQueryExecutor, namespace, expectedCommittedData)

	expectedPostCommitData := []*queryresult.KV{
		{Namespace: namespace, Key: "key1", Value: []byte("value1_new")},
		{Namespace: namespace, Key: "key3", Value: []byte("value3")},
		{Namespace: namespace, Key: "key4", Value: []byte("value4_new")},
	}
	checkQueryExecutor(t, trigger.PostCommitQueryExecutor, namespace, expectedPostCommitData)
}

func checkHandleStateUpdatesCallback(t *testing.T, ml *mock.StateListener, callNumber int,
	expectedLedgerid string,
	expectedUpdates ledger.StateUpdates,
	expectedCommitHt uint64) {
	actualTrigger := ml.HandleStateUpdatesArgsForCall(callNumber)
	assert.Equal(t, expectedLedgerid, actualTrigger.LedgerID)
	checkEqualUpdates(t, expectedUpdates, actualTrigger.StateUpdates)
	assert.Equal(t, expectedCommitHt, actualTrigger.CommittingBlockNum)
}

func checkEqualUpdates(t *testing.T, expected, actual ledger.StateUpdates) {
	assert.Equal(t, len(expected), len(actual))
	for ns, expectedUpdates := range expected {
		assert.ElementsMatch(t, expectedUpdates, actual[ns])
	}
}

func checkQueryExecutor(t *testing.T, qe ledger.SimpleQueryExecutor, namespace string, expectedResults []*queryresult.KV) {
	for _, kv := range expectedResults {
		val, err := qe.GetState(namespace, kv.Key)
		assert.NoError(t, err)
		assert.Equal(t, kv.Value, val)
	}

	itr, err := qe.GetStateRangeScanIterator(namespace, "", "")
	assert.NoError(t, err)
	defer itr.Close()

	actualRes := []*queryresult.KV{}
	for {
		res, err := itr.Next()
		if err != nil {
			assert.NoError(t, err)
		}
		if res == nil {
			break
		}
		actualRes = append(actualRes, res.(*queryresult.KV))
	}
	assert.Equal(t, expectedResults, actualRes)
}
