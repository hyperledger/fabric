/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

type mockState struct {
	GetStateMetadataRv        map[string][]byte
	GetStateMetadataErr       error
	GetPrivateDataMetadataRv  map[string][]byte
	GetPrivateDataMetadataErr error
	DoneCalled                bool
}

func (ms *mockState) GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error) {
	return nil, nil
}

func (ms *mockState) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (validation.ResultsIterator, error) {
	return nil, nil
}

func (ms *mockState) GetStateMetadata(namespace, key string) (map[string][]byte, error) {
	return ms.GetStateMetadataRv, ms.GetStateMetadataErr
}

func (ms *mockState) GetPrivateDataMetadata(namespace, collection, key string) (map[string][]byte, error) {
	return ms.GetPrivateDataMetadataRv, ms.GetPrivateDataMetadataErr
}

func (ms *mockState) Done() {
	ms.DoneCalled = true
}

type mockStateFetcher struct {
	FetchStateRv  *mockState
	FetchStateErr error
}

func (ms *mockStateFetcher) FetchState() (validation.State, error) {
	return ms.FetchStateRv, ms.FetchStateErr
}

func TestSimple(t *testing.T) {
	t.Parallel()

	// Scenario: validation parameter is retrieved with no dependency

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{
		Support: ms,
	}

	sp, err := pm.GetValidationParameterForKey("cc", "coll", "key", 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, utils.MarshalOrPanic(spe), sp)
	assert.True(t, mr.DoneCalled)
}

func rwsetUpdatingMetadataFor(cc, key string) []byte {
	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	return utils.MarshalOrPanic(
		&rwset.TxReadWriteSet{
			NsRwset: []*rwset.NsReadWriteSet{
				{
					Namespace: cc,
					Rwset: utils.MarshalOrPanic(&kvrwset.KVRWSet{
						MetadataWrites: []*kvrwset.KVMetadataWrite{
							{
								Key: key,
								Entries: []*kvrwset.KVMetadataEntry{
									{
										Name: vpMetadataKey,
									},
								},
							},
						},
					}),
				},
			}})
}

func pvtRwsetUpdatingMetadataFor(cc, coll, key string) []byte {
	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	return utils.MarshalOrPanic(
		&rwset.TxReadWriteSet{
			NsRwset: []*rwset.NsReadWriteSet{
				{
					Namespace: cc,
					CollectionHashedRwset: []*rwset.CollectionHashedReadWriteSet{
						{
							CollectionName: coll,
							HashedRwset: utils.MarshalOrPanic(&kvrwset.HashedRWSet{
								MetadataWrites: []*kvrwset.KVMetadataWriteHash{
									{
										KeyHash: []byte(key),
										Entries: []*kvrwset.KVMetadataEntry{
											{
												Name: vpMetadataKey,
											},
										},
									},
								},
							}),
						},
					},
				},
			}})
}

func runFunctions(funcArray []func(), t *testing.T) {
	rand.Seed(time.Now().Unix())
	for _, i := range rand.Perm(len(funcArray)) {
		iLcl := i
		go func() {
			assert.NotPanics(t, funcArray[iLcl])
		}()
	}
}

func TestDependencyNoConflict(t *testing.T) {
	t.Parallel()

	// Scenario: validation parameter is retrieved successfully
	// for a ledger key for transaction (1,1) after waiting for
	// the dependency introduced by transaction (1,0). Retrieval
	// was successful because transaction (1,0) was invalid and
	// so we could safely retrieve the validation parameter from
	// the ledger.

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc, coll, key := "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte)
	errC := make(chan error)
	runFunctions(
		[]func(){
			func() {
				pm.ExtractValidationParameterDependency(1, 0, rwsetbytes)
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 0, errors.New(""))
			},
			func() {
				sp, err := pm.GetValidationParameterForKey(cc, coll, key, 1, 1)
				resC <- sp
				errC <- err
			},
		}, t)

	sp := <-resC
	err := <-errC
	assert.NoError(t, err)
	assert.Equal(t, utils.MarshalOrPanic(spe), sp)
	assert.True(t, mr.DoneCalled)
}

func TestDependencyConflict(t *testing.T) {
	t.Parallel()

	// Scenario: validation parameter is retrieved
	// for a ledger key for transaction (1,1) after waiting for
	// the dependency introduced by transaction (1,0). Retrieval
	// fails because transaction (1,0) was valid and so we cannot
	// retrieve the validation parameter from the ledger, given
	// that transaction (1,0) may or may not be valid according
	// to the ledger component because of MVCC checks

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc, coll, key := "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte)
	errC := make(chan error)
	runFunctions(
		[]func(){
			func() {
				pm.ExtractValidationParameterDependency(1, 0, rwsetbytes)
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 0, nil)
			},
			func() {
				sp, err := pm.GetValidationParameterForKey(cc, coll, key, 1, 1)
				resC <- sp
				errC <- err
			},
		}, t)

	sp := <-resC
	err := <-errC
	assert.Error(t, err)
	assert.IsType(t, &ValidationParameterUpdatedErr{}, err)
	assert.Nil(t, sp)
	assert.True(t, mr.DoneCalled)
}

func TestMultipleDependencyNoConflict(t *testing.T) {
	t.Parallel()

	// Scenario: validation parameter is retrieved successfully
	// for a ledger key for transaction (1,2) after waiting for
	// the dependency introduced by transaction (1,0) and (1,1).
	// Retrieval was successful because both were invalid and
	// so we could safely retrieve the validation parameter from
	// the ledger.

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc, coll, key := "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte)
	errC := make(chan error)
	runFunctions(
		[]func(){
			func() {
				pm.ExtractValidationParameterDependency(1, 0, rwsetbytes)
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 0, errors.New(""))
			},
			func() {
				pm.ExtractValidationParameterDependency(1, 1, rwsetbytes)
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 1, errors.New(""))
			},
			func() {
				sp, err := pm.GetValidationParameterForKey(cc, coll, key, 1, 2)
				resC <- sp
				errC <- err
			},
		}, t)

	sp := <-resC
	err := <-errC
	assert.NoError(t, err)
	assert.Equal(t, utils.MarshalOrPanic(spe), sp)
	assert.True(t, mr.DoneCalled)
}

func TestMultipleDependencyConflict(t *testing.T) {
	t.Parallel()

	// Scenario: validation parameter is retrieved
	// for a ledger key for transaction (1,2) after waiting for
	// the dependency introduced by transaction (1,0) and (1,1).
	// Retrieval fails because transaction (1,1) is valid and so
	// we cannot retrieve the validation parameter from the ledger,
	// given that transaction (1,0) may or may not be valid according
	// to the ledger component because of MVCC checks

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc, coll, key := "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte)
	errC := make(chan error)
	runFunctions(
		[]func(){
			func() {
				pm.ExtractValidationParameterDependency(1, 0, rwsetbytes)
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 0, errors.New(""))
			},
			func() {
				pm.ExtractValidationParameterDependency(1, 1, rwsetbytes)
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 1, nil)
			},
			func() {
				sp, err := pm.GetValidationParameterForKey(cc, coll, key, 1, 2)
				resC <- sp
				errC <- err
			},
		}, t)

	sp := <-resC
	err := <-errC
	assert.Error(t, err)
	assert.IsType(t, &ValidationParameterUpdatedErr{}, err)
	assert.Nil(t, sp)
	assert.True(t, mr.DoneCalled)
}

func TestPvtDependencyNoConflict(t *testing.T) {
	t.Parallel()

	// Scenario: like TestDependencyNoConflict but for private data

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc, coll, key := "cc", "coll", "key"

	rwsetBytes := pvtRwsetUpdatingMetadataFor(cc, coll, key)

	resC := make(chan []byte)
	errC := make(chan error)
	runFunctions(
		[]func(){
			func() {
				pm.ExtractValidationParameterDependency(1, 0, rwsetBytes)
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 0, errors.New(""))
			},
			func() {
				sp, err := pm.GetValidationParameterForKey(cc, coll, key, 1, 1)
				resC <- sp
				errC <- err
			},
		}, t)

	sp := <-resC
	err := <-errC
	assert.NoError(t, err)
	assert.Equal(t, utils.MarshalOrPanic(spe), sp)
	assert.True(t, mr.DoneCalled)
}

func TestPvtDependencyConflict(t *testing.T) {
	t.Parallel()

	// Scenario: like TestDependencyConflict but for private data

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc, coll, key := "cc", "coll", "key"

	rwsetBytes := pvtRwsetUpdatingMetadataFor(cc, coll, key)

	resC := make(chan []byte)
	errC := make(chan error)
	runFunctions(
		[]func(){
			func() {
				pm.ExtractValidationParameterDependency(1, 0, rwsetBytes)
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 0, nil)
			},
			func() {
				sp, err := pm.GetValidationParameterForKey(cc, coll, key, 1, 1)
				resC <- sp
				errC <- err
			},
		}, t)

	sp := <-resC
	err := <-errC
	assert.Error(t, err)
	assert.IsType(t, &ValidationParameterUpdatedErr{}, err)
	assert.True(t, len(err.Error()) > 0)
	assert.Nil(t, sp)
	assert.True(t, mr.DoneCalled)
}

func TestBlockValidationTerminatesBeforeNewBlock(t *testing.T) {
	t.Parallel()

	// Scenario: due to a programming error, validation for
	// transaction (1,0) is scheduled after that for transaction
	// (2,0). This cannot happen and so we panic

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc, coll, key := "cc", "coll", "key"

	rwsetBytes := pvtRwsetUpdatingMetadataFor(cc, coll, key)

	pm.ExtractValidationParameterDependency(2, 0, rwsetBytes)
	panickingFunc := func() {
		pm.ExtractValidationParameterDependency(1, 0, rwsetBytes)
	}
	assert.Panics(t, panickingFunc)
	assert.True(t, mr.DoneCalled)
}

func TestLedgerErrors(t *testing.T) {
	t.Parallel()

	// Scenario: we check that if a ledger error occurs,
	// GetValidationParameterForKey returns an error

	mr := &mockState{
		GetStateMetadataErr:       fmt.Errorf("Ledger error"),
		GetPrivateDataMetadataErr: fmt.Errorf("Ledger error"),
	}
	ms := &mockStateFetcher{mr, fmt.Errorf("Ledger error")}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc, coll, key := "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	errC := make(chan error)
	runFunctions(
		[]func(){
			func() {
				pm.ExtractValidationParameterDependency(1, 0, rwsetbytes)
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 0, errors.New(""))
			},
			func() {
				_, err := pm.GetValidationParameterForKey(cc, coll, key, 1, 1)
				errC <- err
			},
		}, t)

	err := <-errC
	assert.Error(t, err)

	ms.FetchStateErr = nil

	runFunctions(
		[]func(){
			func() {
				pm.ExtractValidationParameterDependency(2, 0, rwsetbytes)
			},
			func() {
				pm.SetTxValidationResult(cc, 2, 0, errors.New(""))
			},
			func() {
				_, err := pm.GetValidationParameterForKey(cc, coll, key, 2, 1)
				errC <- err
			},
		}, t)

	err = <-errC
	assert.Error(t, err)

	cc, coll, key = "cc", "coll", "key"

	rwsetbytes = pvtRwsetUpdatingMetadataFor(cc, coll, key)

	runFunctions(
		[]func(){
			func() {
				pm.ExtractValidationParameterDependency(3, 0, rwsetbytes)
			},
			func() {
				pm.SetTxValidationResult(cc, 3, 0, errors.New(""))
			},
			func() {
				_, err = pm.GetValidationParameterForKey(cc, coll, key, 3, 1)
				errC <- err
			},
		}, t)

	err = <-errC
	assert.Error(t, err)
	assert.True(t, mr.DoneCalled)
}

func TestBadRwsetIsNoDependency(t *testing.T) {
	t.Parallel()

	// Scenario: a transaction has a bogus read-write set.
	// While the transaction will fail eventually, we check
	// that our code doesn't break

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc, coll, key := "cc", "", "key"

	resC := make(chan []byte)
	errC := make(chan error)
	runFunctions(
		[]func(){
			func() {
				pm.ExtractValidationParameterDependency(1, 0, []byte("barf"))
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 0, errors.New(""))
			},
			func() {
				sp, err := pm.GetValidationParameterForKey(cc, coll, key, 1, 1)
				resC <- sp
				errC <- err
			},
		}, t)

	sp := <-resC
	err := <-errC
	assert.NoError(t, err)
	assert.Equal(t, utils.MarshalOrPanic(spe), sp)
	assert.True(t, mr.DoneCalled)
}

func TestWritesIntoDifferentNamespaces(t *testing.T) {
	t.Parallel()

	// Scenario: transaction (1,0) writes to namespace cc1.
	// Transaction (1,1) attempts to retrieve validation
	// parameters for cc. This does not constitute a dependency

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc, othercc, coll, key := "cc1", "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte)
	errC := make(chan error)
	runFunctions(
		[]func(){
			func() {
				pm.SetTxValidationResult(cc, 1, 0, nil)
			},
			func() {
				pm.ExtractValidationParameterDependency(1, 0, rwsetbytes)
				sp, err := pm.GetValidationParameterForKey(othercc, coll, key, 1, 1)
				resC <- sp
				errC <- err
			},
		}, t)

	sp := <-resC
	err := <-errC
	assert.NoError(t, err)
	assert.Equal(t, utils.MarshalOrPanic(spe), sp)
	assert.True(t, mr.DoneCalled)
}

func TestCombinedCalls(t *testing.T) {
	t.Parallel()

	// Scenario: transaction (1,3) requests validation parameters
	// for different keys - one succeeds and one fails.

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc := "cc"
	coll := ""
	key1 := "key1"
	key2 := "key2"

	res1C := make(chan []byte)
	err1C := make(chan error)
	res2C := make(chan []byte)
	err2C := make(chan error)
	runFunctions(
		[]func(){
			func() {
				pm.ExtractValidationParameterDependency(1, 0, rwsetUpdatingMetadataFor(cc, key1))
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 0, errors.New(""))
			},
			func() {
				pm.ExtractValidationParameterDependency(1, 1, rwsetUpdatingMetadataFor(cc, key2))
			},
			func() {
				pm.SetTxValidationResult(cc, 1, 1, nil)
			},
			func() {
				sp, err := pm.GetValidationParameterForKey(cc, coll, key1, 1, 2)
				res1C <- sp
				err1C <- err
			},
			func() {
				sp, err := pm.GetValidationParameterForKey(cc, coll, key2, 1, 2)
				res2C <- sp
				err2C <- err
			},
		}, t)

	sp := <-res1C
	err := <-err1C
	assert.NoError(t, err)
	assert.Equal(t, utils.MarshalOrPanic(spe), sp)

	sp = <-res2C
	err = <-err2C
	assert.Error(t, err)
	assert.IsType(t, &ValidationParameterUpdatedErr{}, err)
	assert.Nil(t, sp)

	assert.True(t, mr.DoneCalled)
}

func TestForRaces(t *testing.T) {
	// scenario to stress test the parallel validation
	// this is an extended combined test
	// run with go test -race and GOMAXPROCS >> 1

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := cauthdsl.SignedByMspMember("foo")
	mr := &mockState{map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, map[string][]byte{vpMetadataKey: utils.MarshalOrPanic(spe)}, nil, true}
	ms := &mockStateFetcher{mr, nil}
	pm := &KeyLevelValidationParameterManagerImpl{Support: ms}

	cc := "cc"
	coll := ""

	nRoutines := 8000 // race detector can track 8192 goroutines max
	funcArray := make([]func(), nRoutines)
	for i := 0; i < 8000; i++ {
		txnum := i
		funcArray[i] = func() {
			key := strconv.Itoa(txnum)
			pm.ExtractValidationParameterDependency(1, uint64(txnum), rwsetUpdatingMetadataFor(cc, key))
			time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
			pm.SetTxValidationResult(cc, 1, uint64(txnum), errors.New(""))
			time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
			sp, err := pm.GetValidationParameterForKey(cc, coll, key, 1, 2)
			assert.Equal(t, utils.MarshalOrPanic(spe), sp)
			assert.NoError(t, err)
		}
	}

	runFunctions(funcArray, t)

	assert.True(t, mr.DoneCalled)
}
