/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txmgr

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
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
	txmgr := testEnv.getTxMgr()
	txmgr.stateListeners = []ledger.StateListener{ml1, ml2, ml3}

	// Mimic commit of block 1 with updates in namespaces ns1, ns2, and ns3
	// This should cause callback to ml1 and ml2 but not to ml3
	sampleBatch := privacyenabledstate.NewUpdateBatch()
	sampleBatch.PubUpdates.Put("ns1", "key1_1", []byte("value1_1"), version.NewHeight(1, 1))
	sampleBatch.PubUpdates.Put("ns1", "key1_2", []byte("value1_2"), version.NewHeight(1, 2))
	sampleBatch.PubUpdates.Put("ns2", "key2_1", []byte("value2_1"), version.NewHeight(1, 3))
	sampleBatch.PubUpdates.Put("ns3", "key3_1", []byte("value3_1"), version.NewHeight(1, 4))
	dummyBlock := protoutil.NewBlock(1, []byte("dummyHash"))
	txmgr.currentUpdates = &currentUpdates{block: dummyBlock, batch: sampleBatch}
	require.NoError(t, txmgr.invokeNamespaceListeners())
	require.Equal(t, 1, ml1.HandleStateUpdatesCallCount())
	require.Equal(t, 1, ml2.HandleStateUpdatesCallCount())
	require.Equal(t, 0, ml3.HandleStateUpdatesCallCount())
	expectedLedgerid, expectedStateUpdate, expectedHt :=
		testLedgerid,
		ledger.StateUpdates{
			"ns1": &ledger.KVStateUpdates{
				PublicUpdates: []*kvrwset.KVWrite{
					{Key: "key1_1", Value: []byte("value1_1")},
					{Key: "key1_2", Value: []byte("value1_2")},
				},
			},
			"ns2": &ledger.KVStateUpdates{
				PublicUpdates: []*kvrwset.KVWrite{
					{Key: "key2_1", Value: []byte("value2_1")},
				},
			},
		},
		uint64(1)
	checkHandleStateUpdatesCallback(t, ml1, 0, expectedLedgerid, expectedStateUpdate, expectedHt)
	expectedLedgerid, expectedStateUpdate, expectedHt =
		testLedgerid,
		ledger.StateUpdates{
			"ns2": &ledger.KVStateUpdates{
				PublicUpdates: []*kvrwset.KVWrite{
					{Key: "key2_1", Value: []byte("value2_1")},
				},
			},
			"ns3": &ledger.KVStateUpdates{
				PublicUpdates: []*kvrwset.KVWrite{
					{Key: "key3_1", Value: []byte("value3_1")},
				},
			},
		},
		uint64(1)
	checkHandleStateUpdatesCallback(t, ml2, 0, expectedLedgerid, expectedStateUpdate, expectedHt)
	require.NoError(t, txmgr.Commit())
	require.Equal(t, 1, ml1.StateCommitDoneCallCount())
	require.Equal(t, 1, ml2.StateCommitDoneCallCount())
	require.Equal(t, 0, ml3.StateCommitDoneCallCount())

	// Mimic commit of block 2 with updates only in ns4 namespace
	// This should cause callback only to ml3
	sampleBatch = privacyenabledstate.NewUpdateBatch()
	sampleBatch.PubUpdates.Put("ns4", "key4_1", []byte("value4_1"), version.NewHeight(2, 1))
	sampleBatch.HashUpdates.Put("ns4", "coll1", []byte("key-hash-1"), []byte("value-hash-1"), version.NewHeight(2, 2))
	sampleBatch.HashUpdates.Put("ns4", "coll1", []byte("key-hash-2"), []byte("value-hash-2"), version.NewHeight(2, 2))
	sampleBatch.HashUpdates.Put("ns4", "coll2", []byte("key-hash-3"), []byte("value-hash-3"), version.NewHeight(2, 3))
	sampleBatch.HashUpdates.Delete("ns4", "coll2", []byte("key-hash-4"), version.NewHeight(2, 4))

	txmgr.currentUpdates = &currentUpdates{block: protoutil.NewBlock(2, []byte("anotherDummyHash")), batch: sampleBatch}
	require.NoError(t, txmgr.invokeNamespaceListeners())
	require.Equal(t, 1, ml1.HandleStateUpdatesCallCount())
	require.Equal(t, 1, ml2.HandleStateUpdatesCallCount())
	require.Equal(t, 1, ml3.HandleStateUpdatesCallCount())

	expectedLedgerid, expectedStateUpdate, expectedHt =
		testLedgerid,
		ledger.StateUpdates{
			"ns4": &ledger.KVStateUpdates{
				PublicUpdates: []*kvrwset.KVWrite{
					{Key: "key4_1", Value: []byte("value4_1")},
				},
				CollHashUpdates: map[string][]*kvrwset.KVWriteHash{
					"coll1": {
						{KeyHash: []byte("key-hash-1"), ValueHash: []byte("value-hash-1")},
						{KeyHash: []byte("key-hash-2"), ValueHash: []byte("value-hash-2")},
					},
					"coll2": {
						{KeyHash: []byte("key-hash-3"), ValueHash: []byte("value-hash-3")},
						{KeyHash: []byte("key-hash-4"), IsDelete: true},
					},
				},
			},
		},
		uint64(2)

	checkHandleStateUpdatesCallback(t, ml3, 0, expectedLedgerid, expectedStateUpdate, expectedHt)

	require.NoError(t, txmgr.Commit())
	require.Equal(t, 1, ml1.StateCommitDoneCallCount())
	require.Equal(t, 1, ml2.StateCommitDoneCallCount())
	require.Equal(t, 1, ml3.StateCommitDoneCallCount())
}

func TestStateListenerQueryExecutor(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, "testLedger", nil)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr()

	namespace := "ns"
	populateCollConfigForTest(t, txMgr,
		[]collConfigkey{
			{"ns", "coll"},
		},
		version.NewHeight(1, 0),
	)

	initialData := []*queryresult.KV{
		{Namespace: namespace, Key: "key1", Value: []byte("value1")},
		{Namespace: namespace, Key: "key2", Value: []byte("value2")},
		{Namespace: namespace, Key: "key3", Value: []byte("value3")},
	}

	initialPvtdata := []*testutilPvtdata{
		{coll: "coll", key: "key1", value: []byte("value1")},
		{coll: "coll", key: "key2", value: []byte("value2")},
	}
	// populate initial data in db
	testutilPopulateDB(t, txMgr, namespace, initialData, initialPvtdata, version.NewHeight(1, 1))

	sl := new(mock.StateListener)
	sl.InterestedInNamespacesStub = func() []string { return []string{"ns"} }
	txMgr.stateListeners = []ledger.StateListener{sl}

	// Create next block
	sim, err := txMgr.NewTxSimulator("tx1")
	require.NoError(t, err)
	require.NoError(t, sim.SetState(namespace, "key1", []byte("value1_new")))
	require.NoError(t, sim.DeleteState(namespace, "key2"))
	require.NoError(t, sim.SetState(namespace, "key4", []byte("value4_new")))
	require.NoError(t, sim.SetPrivateData(namespace, "coll", "key1", []byte("value1_new"))) // change value for key1
	require.NoError(t, sim.DeletePrivateData(namespace, "coll", "key2"))                    // delete key2
	simRes, err := sim.GetTxSimulationResults()
	require.NoError(t, err)
	simResBytes, err := simRes.GetPubSimulationBytes()
	require.NoError(t, err)
	block := testutil.ConstructBlock(t, 1, nil, [][]byte{simResBytes}, false)

	// invoke ValidateAndPrepare function
	_, _, _, err = txMgr.ValidateAndPrepare(&ledger.BlockAndPvtData{Block: block}, false)
	require.NoError(t, err)

	// validate that the query executors passed to the state listener
	trigger := sl.HandleStateUpdatesArgsForCall(0)
	require.NotNil(t, trigger)
	expectedCommittedData := initialData
	checkQueryExecutor(t, trigger.CommittedStateQueryExecutor, namespace, expectedCommittedData)
	expectedCommittedPvtdata := initialPvtdata
	checkQueryExecutorForPvtdataHashes(t, trigger.CommittedStateQueryExecutor, namespace, expectedCommittedPvtdata)

	expectedPostCommitData := []*queryresult.KV{
		{Namespace: namespace, Key: "key1", Value: []byte("value1_new")},
		{Namespace: namespace, Key: "key3", Value: []byte("value3")},
		{Namespace: namespace, Key: "key4", Value: []byte("value4_new")},
	}
	checkQueryExecutor(t, trigger.PostCommitQueryExecutor, namespace, expectedPostCommitData)
	expectedPostCommitPvtdata := []*testutilPvtdata{
		{coll: "coll", key: "key1", value: []byte("value1_new")},
		{coll: "coll", key: "key2", value: nil},
	}
	checkQueryExecutorForPvtdataHashes(t, trigger.PostCommitQueryExecutor, namespace, expectedPostCommitPvtdata)
}

func checkHandleStateUpdatesCallback(t *testing.T, ml *mock.StateListener, callNumber int,
	expectedLedgerid string,
	expectedUpdates ledger.StateUpdates,
	expectedCommitHt uint64) {
	actualTrigger := ml.HandleStateUpdatesArgsForCall(callNumber)
	require.Equal(t, expectedLedgerid, actualTrigger.LedgerID)
	checkEqualUpdates(t, expectedUpdates, actualTrigger.StateUpdates)
	require.Equal(t, expectedCommitHt, actualTrigger.CommittingBlockNum)
}

func checkEqualUpdates(t *testing.T, expected, actual ledger.StateUpdates) {
	require.Equal(t, len(expected), len(actual))
	for ns, e := range expected {
		require.ElementsMatch(t, e.PublicUpdates, actual[ns].PublicUpdates)
		checkEqualCollsUpdates(t, e.CollHashUpdates, actual[ns].CollHashUpdates)
	}
}

func checkEqualCollsUpdates(t *testing.T, expected, actual map[string][]*kvrwset.KVWriteHash) {
	require.Equal(t, len(expected), len(actual))
	for coll, e := range expected {
		require.ElementsMatch(t, e, actual[coll])
	}
}

func checkQueryExecutor(t *testing.T, qe ledger.SimpleQueryExecutor, namespace string, expectedResults []*queryresult.KV) {
	for _, kv := range expectedResults {
		val, err := qe.GetState(namespace, kv.Key)
		require.NoError(t, err)
		require.Equal(t, kv.Value, val)
	}

	itr, err := qe.GetStateRangeScanIterator(namespace, "", "")
	require.NoError(t, err)
	defer itr.Close()

	actualRes := []*queryresult.KV{}
	for {
		res, err := itr.Next()
		if err != nil {
			require.NoError(t, err)
		}
		if res == nil {
			break
		}
		actualRes = append(actualRes, res.(*queryresult.KV))
	}
	require.Equal(t, expectedResults, actualRes)
}

func checkQueryExecutorForPvtdataHashes(t *testing.T, qe ledger.SimpleQueryExecutor, namespace string, expectedPvtdata []*testutilPvtdata) {
	for _, p := range expectedPvtdata {
		valueHash, err := qe.GetPrivateDataHash(namespace, p.coll, p.key)
		require.NoError(t, err)
		if p.value == nil {
			require.Nil(t, valueHash) // key does not exist
		} else {
			require.Equal(t, util.ComputeHash(p.value), valueHash)
		}
	}
}
