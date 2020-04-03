/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryutil_test

import (
	"errors"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/state"
	"github.com/hyperledger/fabric/core/ledger/util"

	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	statemock "github.com/hyperledger/fabric/core/ledger/internal/state/mock"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/queryutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/queryutil/mock"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("util,statedb=debug")
	os.Exit(m.Run())
}

func TestCombinerGetState(t *testing.T) {
	batch1 := state.NewUpdateBatch()
	batch1.Put("ns1", "key1", []byte("b1_value1"), nil)
	batch1.Delete("ns1", "key2", nil)
	batch1.Put("ns1", "key3", []byte("b1_value3"), nil)

	batch2 := state.NewUpdateBatch()
	batch2.Put("ns1", "key1", []byte("b2_value1"), nil)
	batch2.Put("ns1", "key2", []byte("b2_value2"), nil)
	batch2.Put("ns1", "key3", []byte("b2_value3"), nil)

	batch3 := state.NewUpdateBatch()
	batch3.Put("ns1", "key1", []byte("b3_value1"), nil)
	batch3.Put("ns1", "key2", []byte("b3_value2"), nil)
	batch3.Delete("ns1", "key3", nil)

	combiner := &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch1},
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch2},
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch3},
		}}

	val, err := combiner.GetState("ns1", "key1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("b1_value1"), val)

	val, err = combiner.GetState("ns1", "key2")
	assert.NoError(t, err)
	assert.Nil(t, val)

	val, err = combiner.GetState("ns1", "key3")
	assert.NoError(t, err)
	assert.Equal(t, []byte("b1_value3"), val)

	combiner = &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch3},
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch2},
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch1},
		}}
	val, err = combiner.GetState("ns1", "key1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("b3_value1"), val)

	val, err = combiner.GetState("ns1", "key2")
	assert.NoError(t, err)
	assert.Equal(t, []byte("b3_value2"), val)

	val, err = combiner.GetState("ns1", "key3")
	assert.NoError(t, err)
	assert.Nil(t, val)
}

func TestCombinerRangeScan(t *testing.T) {
	batch1 := state.NewUpdateBatch()
	batch1.Put("ns1", "key1", []byte("batch1_value1"), nil)
	batch1.Delete("ns1", "key2", nil)
	batch1.Put("ns1", "key3", []byte("batch1_value3"), nil)

	batch2 := state.NewUpdateBatch()
	batch2.Put("ns1", "key1", []byte("batch2_value1"), nil)
	batch2.Put("ns1", "key2", []byte("batch2_value2"), nil)
	batch2.Delete("ns1", "key3", nil)
	batch2.Put("ns1", "key4", []byte("batch2_value4"), nil)

	batch3 := state.NewUpdateBatch()
	batch3.Put("ns1", "key0", []byte("batch3_value0"), nil)
	batch3.Put("ns1", "key1", []byte("batch3_value1"), nil)
	batch3.Put("ns1", "key2", []byte("batch3_value2"), nil)
	batch3.Put("ns1", "key3", []byte("batch3_value3"), nil)
	batch3.Put("ns1", "key4", []byte("batch3_value4"), nil)
	batch3.Put("ns1", "key5", []byte("batch3_value5"), nil)

	combiner := &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch1},
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch2},
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch3},
		},
	}

	itr, err := combiner.GetStateRangeScanIterator("ns1", "key1", "key4")
	assert.NoError(t, err)
	expectedResults := []*queryresult.KV{
		{Namespace: "ns1", Key: "key1", Value: []byte("batch1_value1")},
		{Namespace: "ns1", Key: "key3", Value: []byte("batch1_value3")},
	}
	testutilCheckIteratorResults(t, itr, expectedResults)

	itr, err = combiner.GetStateRangeScanIterator("ns1", "key0", "key6")
	assert.NoError(t, err)
	expectedResults = []*queryresult.KV{
		{Namespace: "ns1", Key: "key0", Value: []byte("batch3_value0")},
		{Namespace: "ns1", Key: "key1", Value: []byte("batch1_value1")},
		{Namespace: "ns1", Key: "key3", Value: []byte("batch1_value3")},
		{Namespace: "ns1", Key: "key4", Value: []byte("batch2_value4")},
		{Namespace: "ns1", Key: "key5", Value: []byte("batch3_value5")},
	}
	testutilCheckIteratorResults(t, itr, expectedResults)

	combiner = &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch3},
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch2},
			&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: batch1},
		},
	}
	itr, err = combiner.GetStateRangeScanIterator("ns1", "key0", "key6")
	assert.NoError(t, err)
	expectedResults = []*queryresult.KV{
		{Namespace: "ns1", Key: "key0", Value: []byte("batch3_value0")},
		{Namespace: "ns1", Key: "key1", Value: []byte("batch3_value1")},
		{Namespace: "ns1", Key: "key2", Value: []byte("batch3_value2")},
		{Namespace: "ns1", Key: "key3", Value: []byte("batch3_value3")},
		{Namespace: "ns1", Key: "key4", Value: []byte("batch3_value4")},
		{Namespace: "ns1", Key: "key5", Value: []byte("batch3_value5")},
	}
	testutilCheckIteratorResults(t, itr, expectedResults)
}

func TestGetStateError(t *testing.T) {
	qe1 := &mock.QueryExecuter{}
	qe1.GetStateReturns(&state.VersionedValue{Value: []byte("testValue")}, nil)
	qe2 := &mock.QueryExecuter{}
	qe2.GetStateReturns(nil, errors.New("Error for testing"))
	combiner1 := &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			qe1, qe2,
		},
	}
	_, err := combiner1.GetState("ns", "key1")
	assert.NoError(t, err)

	combiner2 := &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			qe2, qe1,
		},
	}
	_, err = combiner2.GetState("ns", "key1")
	assert.Error(t, err)
}

func TestGetRangeScanError(t *testing.T) {
	itr1 := &statemock.ResultsIterator{}
	itr1.NextReturns(
		&state.VersionedKV{
			CompositeKey:   state.CompositeKey{Namespace: "ns", Key: "dummyKey"},
			VersionedValue: state.VersionedValue{Value: []byte("dummyVal")},
		},
		nil,
	)

	qe1 := &mock.QueryExecuter{}
	qe1.GetStateRangeScanIteratorReturns(itr1, nil)
	qe2 := &mock.QueryExecuter{}
	qe2.GetStateRangeScanIteratorReturns(nil, errors.New("dummy error on getting the iterator"))
	combiner := &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			qe1, qe2,
		},
	}
	_, err := combiner.GetStateRangeScanIterator("ns", "startKey", "endKey")
	assert.Error(t, err)
}

func TestGetRangeScanUnderlyingIteratorReturnsError(t *testing.T) {
	itr1 := &statemock.ResultsIterator{}
	itr1.NextReturns(
		&state.VersionedKV{
			CompositeKey:   state.CompositeKey{Namespace: "ns", Key: "dummyKey"},
			VersionedValue: state.VersionedValue{Value: []byte("dummyVal")},
		},
		nil,
	)

	itr2 := &statemock.ResultsIterator{}
	itr2.NextReturns(
		nil,
		errors.New("dummyErrorOnIteratorNext"),
	)

	qe1 := &mock.QueryExecuter{}
	qe1.GetStateRangeScanIteratorReturns(itr1, nil)
	qe2 := &mock.QueryExecuter{}
	qe2.GetStateRangeScanIteratorReturns(itr2, nil)
	combiner := &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			qe1, qe2,
		},
	}
	_, err := combiner.GetStateRangeScanIterator("ns", "startKey", "endKey")
	assert.Error(t, err)
}

func TestGetPrivateDataHash(t *testing.T) {
	batch1 := state.NewHashedUpdateBatch()
	key1Hash := util.ComputeStringHash("key1")
	key2Hash := util.ComputeStringHash("key2")
	key3Hash := util.ComputeStringHash("key3")

	batch1.Put("ns1", "coll1", key1Hash, []byte("b1_value1"), nil)
	batch1.Delete("ns1", "coll1", key2Hash, nil)
	batch1.Put("ns1", "coll1", key3Hash, []byte("b1_value3"), nil)

	batch2 := state.NewHashedUpdateBatch()
	batch2.Put("ns1", "coll1", key1Hash, []byte("b2_value1"), nil)
	batch2.Put("ns1", "coll1", key2Hash, []byte("b2_value2"), nil)
	batch2.Put("ns1", "coll1", key3Hash, []byte("b2_value3"), nil)

	batch3 := state.NewHashedUpdateBatch()
	batch3.Put("ns1", "coll1", key1Hash, []byte("b3_value1"), nil)
	batch3.Put("ns1", "coll1", key2Hash, []byte("b3_value2"), nil)

	combiner := &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			&queryutil.UpdateBatchBackedQueryExecuter{HashUpdatesBatch: batch1},
			&queryutil.UpdateBatchBackedQueryExecuter{HashUpdatesBatch: batch2},
			&queryutil.UpdateBatchBackedQueryExecuter{HashUpdatesBatch: batch3},
		}}

	val, err := combiner.GetPrivateDataHash("ns1", "coll1", "key1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("b1_value1"), val)

	val, err = combiner.GetPrivateDataHash("ns1", "coll1", "key2")
	assert.NoError(t, err)
	assert.Nil(t, val)

	val, err = combiner.GetPrivateDataHash("ns1", "coll1", "key3")
	assert.NoError(t, err)
	assert.Equal(t, []byte("b1_value3"), val)

	combiner = &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			&queryutil.UpdateBatchBackedQueryExecuter{HashUpdatesBatch: batch3},
			&queryutil.UpdateBatchBackedQueryExecuter{HashUpdatesBatch: batch2},
			&queryutil.UpdateBatchBackedQueryExecuter{HashUpdatesBatch: batch1},
		}}
	val, err = combiner.GetPrivateDataHash("ns1", "coll1", "key1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("b3_value1"), val)

	val, err = combiner.GetPrivateDataHash("ns1", "coll1", "key2")
	assert.NoError(t, err)
	assert.Equal(t, []byte("b3_value2"), val)

	val, err = combiner.GetPrivateDataHash("ns1", "coll1", "key3")
	assert.NoError(t, err)
	assert.Equal(t, []byte("b2_value3"), val)
}

func TestGetPrivateDataHashError(t *testing.T) {
	qe1 := &mock.QueryExecuter{}
	qe1.GetPrivateDataHashReturns(&state.VersionedValue{Value: []byte("testValue")}, nil)
	qe2 := &mock.QueryExecuter{}
	qe2.GetPrivateDataHashReturns(nil, errors.New("Error for testing"))
	combiner1 := &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			qe1, qe2,
		},
	}
	_, err := combiner1.GetPrivateDataHash("ns", "coll1", "key1")
	assert.NoError(t, err)

	combiner2 := &queryutil.QECombiner{
		QueryExecuters: []queryutil.QueryExecuter{
			qe2, qe1,
		},
	}
	_, err = combiner2.GetPrivateDataHash("ns", "coll1", "key1")
	assert.Error(t, err)
}

func testutilCheckIteratorResults(t *testing.T, itr commonledger.ResultsIterator, expectedResults []*queryresult.KV) {
	results := []*queryresult.KV{}
	for {
		result, err := itr.Next()
		assert.NoError(t, err)
		if result == nil {
			break
		}
		results = append(results, result.(*queryresult.KV))
	}
	assert.Equal(t, expectedResults, results)
}
