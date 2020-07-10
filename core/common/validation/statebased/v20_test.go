/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	"testing"

	verr "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/core/common/validation/statebased/mocks"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test0(t *testing.T) {
	t.Parallel()

	// SCENARIO: no writes anywhere -> check the ccep

	cc := "cc"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{}

	pm := &mocks.KeyLevelValidationParameterManager{}

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(ccep): errors.New("nope")}}

	cr := &mocks.CollectionResources{}

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCEndorsementPolicyError{})
}

func Test1(t *testing.T) {
	t.Parallel()

	// SCENARIO: only one write in the public namespace -> check the ccep

	cc := "cc"
	key := "key"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(ccep): errors.New("nope")}}

	cr := &mocks.CollectionResources{}

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToWriteSet(cc, key, []byte("value"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCEndorsementPolicyError{})
}

func Test1Multiple(t *testing.T) {
	t.Parallel()

	// SCENARIO: only one write in the public namespace -> check the ccep

	cc := "cc"
	key := "key"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(ccep): nil}, EvaluateRV: errors.New("nope")}

	cr := &mocks.CollectionResources{}

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToWriteSet(cc, key, []byte("value"))
	rwsb.AddToWriteSet(cc, key+"1", []byte("value"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.NoError(t, err)
}

func Test1NoErr(t *testing.T) {
	t.Parallel()

	// SCENARIO: only one write in the public namespace -> check the ccep

	cc := "cc"
	key := "key"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, &ledger.CollConfigNotDefinedError{})

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(ccep): errors.New("nope")}}

	cr := &mocks.CollectionResources{}

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToWriteSet(cc, key, []byte("value"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCEndorsementPolicyError{})
}

func Test1Err1(t *testing.T) {
	t.Parallel()

	// SCENARIO: error fetching validation params

	cc := "cc"
	key := "key"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, &ValidationParameterUpdatedError{})

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(ccep): errors.New("nope")}}

	cr := &mocks.CollectionResources{}

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToWriteSet(cc, key, []byte("value"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCEndorsementPolicyError{})
}

func Test1Err2(t *testing.T) {
	t.Parallel()

	// SCENARIO: error fetching validation params

	cc := "cc"
	key := "key"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("heavy metal hamster"))

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(ccep): errors.New("nope")}}

	cr := &mocks.CollectionResources{}

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToWriteSet(cc, key, []byte("value"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCExecutionFailureError{})
}

func Test1Meta(t *testing.T) {
	t.Parallel()

	// SCENARIO: only one meta write in the public namespace -> check the ccep

	cc := "cc"
	key := "key"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(ccep): errors.New("nope")}}

	cr := &mocks.CollectionResources{}

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToMetadataWriteSet(cc, key, nil)
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCEndorsementPolicyError{})
}

func Test1MetaMultiple(t *testing.T) {
	t.Parallel()

	// SCENARIO: only one meta write in the public namespace -> check the ccep

	cc := "cc"
	key := "key"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(ccep): nil}, EvaluateRV: errors.New("nope")}

	cr := &mocks.CollectionResources{}

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToMetadataWriteSet(cc, key, nil)
	rwsb.AddToMetadataWriteSet(cc, key+"1", nil)
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.NoError(t, err)
}

func Test2(t *testing.T) {
	t.Parallel()

	// SCENARIO: only one write in the public namespace with SBEP -> check the SBEP

	cc := "cc"
	key := "key"
	ccep := []byte("ccep")
	sbep := []byte("sbep")

	ms := &mockStateFetcher{}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", cc, "", key, mock.Anything, mock.Anything).Return(sbep, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(sbep): errors.New("nope")}}

	cr := &mocks.CollectionResources{}

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToWriteSet(cc, key, []byte("value"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCEndorsementPolicyError{})
}

func Test3(t *testing.T) {
	t.Parallel()

	// SCENARIO: only one write in a collection without COLLEP or SBEP -> check the CCEP

	cc := "cc"
	key := "key"
	hashedKey := ",p\xe1+z\x06F\xf9\"y\xf4'ǳ\x8es4\xd8\xe58\x9c\xff\x16z\x1d\xc3\x0es\xf8&\xb6\x83"
	coll := "coll"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{FetchStateRv: &mockState{}}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", cc, coll, hashedKey, mock.Anything, mock.Anything).Return(nil, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(ccep): errors.New("nope")}}

	cr := &mocks.CollectionResources{}
	cr.On("CollectionValidationInfo", cc, coll, mock.Anything).Return(nil, nil, nil)

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToPvtAndHashedWriteSet(cc, coll, key, []byte("Well I guess you took my youth and gave it all away"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCEndorsementPolicyError{})
}

func Test3Meta(t *testing.T) {
	t.Parallel()

	// SCENARIO: only one meta write in a collection without COLLEP or SBEP -> check the CCEP

	cc := "cc"
	key := "key"
	hashedKey := ",p\xe1+z\x06F\xf9\"y\xf4'ǳ\x8es4\xd8\xe58\x9c\xff\x16z\x1d\xc3\x0es\xf8&\xb6\x83"
	coll := "coll"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{FetchStateRv: &mockState{}}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", cc, coll, hashedKey, mock.Anything, mock.Anything).Return(nil, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(ccep): errors.New("nope")}}

	cr := &mocks.CollectionResources{}
	cr.On("CollectionValidationInfo", cc, coll, mock.Anything).Return(nil, nil, nil)

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToHashedMetadataWriteSet(cc, coll, key, nil)
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCEndorsementPolicyError{})
}

func Test4(t *testing.T) {
	t.Parallel()

	// SCENARIO: only one write in a collection with COLLEP -> check COLLEP

	cc := "cc"
	key := "key"
	hashedKey := ",p\xe1+z\x06F\xf9\"y\xf4'ǳ\x8es4\xd8\xe58\x9c\xff\x16z\x1d\xc3\x0es\xf8&\xb6\x83"
	coll := "coll"
	ccep := []byte("ccep")
	collep := []byte("collep")

	ms := &mockStateFetcher{FetchStateRv: &mockState{}}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", cc, coll, hashedKey, mock.Anything, mock.Anything).Return(nil, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(collep): errors.New("nope")}}

	cr := &mocks.CollectionResources{}
	cr.On("CollectionValidationInfo", cc, coll, mock.Anything).Return(collep, nil, nil)

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToPvtAndHashedWriteSet(cc, coll, key, []byte("Well I guess you took my youth"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCEndorsementPolicyError{})
}

func Test4Multiple(t *testing.T) {
	t.Parallel()

	// SCENARIO: only one write in a collection with COLLEP -> check COLLEP

	cc := "cc"
	key := "key"
	coll := "coll"
	ccep := []byte("ccep")
	collep := []byte("collep")

	ms := &mockStateFetcher{FetchStateRv: &mockState{}}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", cc, coll, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(collep): nil}, EvaluateRV: errors.New("nope")}

	cr := &mocks.CollectionResources{}
	cr.On("CollectionValidationInfo", cc, coll, mock.Anything).Return(collep, nil, nil)

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToPvtAndHashedWriteSet(cc, coll, key, []byte("As I was goin' over"))
	rwsb.AddToPvtAndHashedWriteSet(cc, coll, key+"1", []byte("The Cork and Kerry Mountains"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.NoError(t, err)
}

func Test4Err(t *testing.T) {
	t.Parallel()

	// SCENARIO: error while retrieving COLLEP

	cc := "cc"
	key := "key"
	hashedKey := ",p\xe1+z\x06F\xf9\"y\xf4'ǳ\x8es4\xd8\xe58\x9c\xff\x16z\x1d\xc3\x0es\xf8&\xb6\x83"
	coll := "coll"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{FetchStateErr: errors.New("Dr. Stein grows funny creatures")}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", cc, coll, hashedKey, mock.Anything, mock.Anything).Return(nil, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{}}

	cr := &mocks.CollectionResources{}

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToPvtAndHashedWriteSet(cc, coll, key, []byte("Well I guess you took my youth"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCExecutionFailureError{})
}

func Test5(t *testing.T) {
	t.Parallel()

	// SCENARIO: only one write in a collection with SBEP -> check SBEP

	cc := "cc"
	key := "key"
	hashedKey := ",p\xe1+z\x06F\xf9\"y\xf4'ǳ\x8es4\xd8\xe58\x9c\xff\x16z\x1d\xc3\x0es\xf8&\xb6\x83"
	coll := "coll"
	ccep := []byte("ccep")
	collep := []byte("collep")
	sbep := []byte("sbep")

	ms := &mockStateFetcher{FetchStateRv: &mockState{}}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", cc, coll, hashedKey, mock.Anything, mock.Anything).Return(sbep, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{string(sbep): errors.New("nope")}}

	cr := &mocks.CollectionResources{}
	cr.On("CollectionValidationInfo", cc, coll, mock.Anything).Return(collep, nil, nil)

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToPvtAndHashedWriteSet(cc, coll, key, []byte("Well I guess you took my youth and gave it all away"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCEndorsementPolicyError{})
}

func Test6(t *testing.T) {
	t.Parallel()

	// SCENARIO: unexpected error while fetching collection validation info

	cc := "cc"
	key := "key"
	hashedKey := ",p\xe1+z\x06F\xf9\"y\xf4'ǳ\x8es4\xd8\xe58\x9c\xff\x16z\x1d\xc3\x0es\xf8&\xb6\x83"
	coll := "coll"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{FetchStateRv: &mockState{}}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", cc, coll, hashedKey, mock.Anything, mock.Anything).Return(nil, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{}}

	cr := &mocks.CollectionResources{}
	cr.On("CollectionValidationInfo", cc, coll, mock.Anything).Return(nil, errors.New("two minutes to midnight"), nil)

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToPvtAndHashedWriteSet(cc, coll, key, []byte("Well I guess you took my youth and gave it all away"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCExecutionFailureError{})
}

func Test7(t *testing.T) {
	t.Parallel()

	// SCENARIO: deterministic error while fetching collection validation info

	cc := "cc"
	key := "key"
	hashedKey := ",p\xe1+z\x06F\xf9\"y\xf4'ǳ\x8es4\xd8\xe58\x9c\xff\x16z\x1d\xc3\x0es\xf8&\xb6\x83"
	coll := "coll"
	ccep := []byte("ccep")

	ms := &mockStateFetcher{FetchStateRv: &mockState{}}

	pm := &mocks.KeyLevelValidationParameterManager{}
	pm.On("GetValidationParameterForKey", cc, coll, hashedKey, mock.Anything, mock.Anything).Return(nil, nil)

	pe := &mockPolicyEvaluator{EvaluateResByPolicy: map[string]error{}}

	cr := &mocks.CollectionResources{}
	cr.On("CollectionValidationInfo", cc, coll, mock.Anything).Return(nil, nil, errors.New("nope"))

	pcf := NewV20Evaluator(pm, pe, cr, ms)

	ev := pcf.Evaluator(ccep)

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToPvtAndHashedWriteSet(cc, coll, key, []byte("Well I guess you took my youth and gave it all away"))
	rws := rwsb.GetTxReadWriteSet()

	err := ev.Evaluate(1, 1, rws.NsRwSets, cc, []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.IsType(t, err, &verr.VSCCEndorsementPolicyError{})
}
