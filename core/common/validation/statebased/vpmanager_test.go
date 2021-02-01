/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	"fmt"
	"math/rand"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/policydsl"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

type mockState struct {
	GetStateMetadataRv              map[string][]byte
	GetStateMetadataErr             error
	GetPrivateDataMetadataByHashRv  map[string][]byte
	GetPrivateDataMetadataByHashErr error
	DoneCalled                      bool
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

func (ms *mockState) GetPrivateDataMetadataByHash(namespace, collection string, keyhash []byte) (map[string][]byte, error) {
	return ms.GetPrivateDataMetadataByHashRv, ms.GetPrivateDataMetadataByHashErr
}

func (ms *mockState) Done() {
	ms.DoneCalled = true
}

type mockStateFetcher struct {
	mutex          sync.Mutex
	returnedStates []*mockState
	FetchStateRv   *mockState
	FetchStateErr  error
}

func (ms *mockStateFetcher) DoneCalled() bool {
	for _, s := range ms.returnedStates {
		if !s.DoneCalled {
			return false
		}
	}
	return true
}

type mockTranslator struct {
	TranslateError error
}

func (n *mockTranslator) Translate(b []byte) ([]byte, error) {
	return b, n.TranslateError
}

func (ms *mockStateFetcher) FetchState() (validation.State, error) {
	var rv *mockState
	if ms.FetchStateRv != nil {
		rv = &mockState{
			GetPrivateDataMetadataByHashErr: ms.FetchStateRv.GetPrivateDataMetadataByHashErr,
			GetStateMetadataErr:             ms.FetchStateRv.GetStateMetadataErr,
			GetPrivateDataMetadataByHashRv:  ms.FetchStateRv.GetPrivateDataMetadataByHashRv,
			GetStateMetadataRv:              ms.FetchStateRv.GetStateMetadataRv,
		}
		ms.mutex.Lock()
		if ms.returnedStates != nil {
			ms.returnedStates = make([]*mockState, 0, 1)
		}
		ms.returnedStates = append(ms.returnedStates, rv)
		ms.mutex.Unlock()
	}
	return rv, ms.FetchStateErr
}

func TestSimple(t *testing.T) {
	t.Parallel()

	// Scenario: validation parameter is retrieved with no dependency

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{
		PolicyTranslator: &mockTranslator{},
		StateFetcher:     ms,
	}

	sp, err := pm.GetValidationParameterForKey("cc", "coll", "key", 0, 0)
	require.NoError(t, err)
	require.Equal(t, protoutil.MarshalOrPanic(spe), sp)
	require.True(t, ms.DoneCalled())
}

func rwsetUpdatingMetadataFor(cc, key string) []byte {
	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	return protoutil.MarshalOrPanic(
		&rwset.TxReadWriteSet{
			NsRwset: []*rwset.NsReadWriteSet{
				{
					Namespace: cc,
					Rwset: protoutil.MarshalOrPanic(&kvrwset.KVRWSet{
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
			},
		})
}

func pvtRwsetUpdatingMetadataFor(cc, coll, key string) []byte {
	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	return protoutil.MarshalOrPanic(
		&rwset.TxReadWriteSet{
			NsRwset: []*rwset.NsReadWriteSet{
				{
					Namespace: cc,
					CollectionHashedRwset: []*rwset.CollectionHashedReadWriteSet{
						{
							CollectionName: coll,
							HashedRwset: protoutil.MarshalOrPanic(&kvrwset.HashedRWSet{
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
			},
		})
}

func runFunctions(t *testing.T, seed int64, funcs ...func()) {
	r := rand.New(rand.NewSource(seed))
	c := make(chan struct{})
	for _, i := range r.Perm(len(funcs)) {
		iLcl := i
		go func() {
			require.NotPanics(t, funcs[iLcl], "assert failure occurred with seed %d", seed)
			c <- struct{}{}
		}()
	}
	for range funcs {
		<-c
	}
}

func TestTranslatorBadPolicy(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: we verify that translation from SignaturePolicyEnvelope to ApplicationPolicy fails appropriately

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: []byte("barf")}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: []byte("barf")}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	mt := &mockTranslator{}
	mt.TranslateError = errors.New("you shall not pass")
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: mt, StateFetcher: ms}

	cc, coll, key := "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte, 1)
	errC := make(chan error, 1)
	runFunctions(t, seed,
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
		})

	sp := <-resC
	err := <-errC
	require.Errorf(t, err, "assert failure occurred with seed %d", seed)
	require.Contains(t, err.Error(), "could not translate policy for cc:key: you shall not pass", "assert failure occurred with seed %d", seed)
	require.True(t, ms.DoneCalled(), "assert failure occurred with seed %d", seed)
	require.Nil(t, sp, "assert failure occurred with seed %d", seed)
}

func TestTranslatorBadPolicyPvt(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: we verify that translation from SignaturePolicyEnvelope to ApplicationPolicy fails appropriately with private data

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: []byte("barf")}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: []byte("barf")}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	mt := &mockTranslator{}
	mt.TranslateError = errors.New("you shall not pass")
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: mt, StateFetcher: ms}

	cc, coll, key := "cc", "coll", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte, 1)
	errC := make(chan error, 1)
	runFunctions(t, seed,
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
		})

	sp := <-resC
	err := <-errC
	require.Errorf(t, err, "assert failure occurred with seed %d", seed)
	require.Contains(t, err.Error(), "could not translate policy for cc:coll:6b6579: you shall not pass", "assert failure occurred with seed %d", seed)
	require.True(t, ms.DoneCalled(), "assert failure occurred with seed %d", seed)
	require.Nil(t, sp, "assert failure occurred with seed %d", seed)
}

func TestDependencyNoConflict(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: validation parameter is retrieved successfully
	// for a ledger key for transaction (1,1) after waiting for
	// the dependency introduced by transaction (1,0). Retrieval
	// was successful because transaction (1,0) was invalid and
	// so we could safely retrieve the validation parameter from
	// the ledger.

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc, coll, key := "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte, 1)
	errC := make(chan error, 1)
	runFunctions(t, seed,
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
		})

	sp := <-resC
	err := <-errC
	require.NoError(t, err, "assert failure occurred with seed %d", seed)
	require.Equal(t, protoutil.MarshalOrPanic(spe), sp, "assert failure occurred with seed %d", seed)
	require.True(t, ms.DoneCalled(), "assert failure occurred with seed %d", seed)
}

func TestDependencyConflict(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: validation parameter is retrieved
	// for a ledger key for transaction (1,1) after waiting for
	// the dependency introduced by transaction (1,0). Retrieval
	// fails because transaction (1,0) was valid and so we cannot
	// retrieve the validation parameter from the ledger, given
	// that transaction (1,0) may or may not be valid according
	// to the ledger component because of MVCC checks

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc, coll, key := "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte, 1)
	errC := make(chan error, 1)
	runFunctions(t, seed,
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
		})

	sp := <-resC
	err := <-errC
	require.Errorf(t, err, "assert failure occurred with seed %d", seed)
	require.IsType(t, &ValidationParameterUpdatedError{}, err, "assert failure occurred with seed %d", seed)
	require.Nil(t, sp, "assert failure occurred with seed %d", seed)
}

func TestMultipleDependencyNoConflict(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: validation parameter is retrieved successfully
	// for a ledger key for transaction (1,2) after waiting for
	// the dependency introduced by transaction (1,0) and (1,1).
	// Retrieval was successful because both were invalid and
	// so we could safely retrieve the validation parameter from
	// the ledger.

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc, coll, key := "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte, 1)
	errC := make(chan error, 1)
	runFunctions(t, seed,
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
		})

	sp := <-resC
	err := <-errC
	require.NoError(t, err, "assert failure occurred with seed %d", seed)
	require.Equal(t, protoutil.MarshalOrPanic(spe), sp, "assert failure occurred with seed %d", seed)
	require.True(t, ms.DoneCalled(), "assert failure occurred with seed %d", seed)
}

func TestMultipleDependencyConflict(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: validation parameter is retrieved
	// for a ledger key for transaction (1,2) after waiting for
	// the dependency introduced by transaction (1,0) and (1,1).
	// Retrieval fails because transaction (1,1) is valid and so
	// we cannot retrieve the validation parameter from the ledger,
	// given that transaction (1,0) may or may not be valid according
	// to the ledger component because of MVCC checks

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc, coll, key := "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte, 1)
	errC := make(chan error, 1)
	runFunctions(t, seed,
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
		})

	sp := <-resC
	err := <-errC
	require.Errorf(t, err, "assert failure occurred with seed %d", seed)
	require.IsType(t, &ValidationParameterUpdatedError{}, err, "assert failure occurred with seed %d", seed)
	require.Nil(t, sp, "assert failure occurred with seed %d", seed)
}

func TestPvtDependencyNoConflict(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: like TestDependencyNoConflict but for private data

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc, coll, key := "cc", "coll", "key"

	rwsetBytes := pvtRwsetUpdatingMetadataFor(cc, coll, key)

	resC := make(chan []byte, 1)
	errC := make(chan error, 1)
	runFunctions(t, seed,
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
		})

	sp := <-resC
	err := <-errC
	require.NoError(t, err, "assert failure occurred with seed %d", seed)
	require.Equal(t, protoutil.MarshalOrPanic(spe), sp, "assert failure occurred with seed %d", seed)
	require.True(t, ms.DoneCalled(), "assert failure occurred with seed %d", seed)
}

func TestPvtDependencyConflict(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: like TestDependencyConflict but for private data

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc, coll, key := "cc", "coll", "key"

	rwsetBytes := pvtRwsetUpdatingMetadataFor(cc, coll, key)

	resC := make(chan []byte, 1)
	errC := make(chan error, 1)
	runFunctions(t, seed,
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
		})

	sp := <-resC
	err := <-errC
	require.Errorf(t, err, "assert failure occurred with seed %d", seed)
	require.IsType(t, &ValidationParameterUpdatedError{}, err, "assert failure occurred with seed %d", seed)
	require.True(t, len(err.Error()) > 0, "assert failure occurred with seed %d", seed)
	require.Nil(t, sp, "assert failure occurred with seed %d", seed)
}

func TestBlockValidationTerminatesBeforeNewBlock(t *testing.T) {
	t.Parallel()

	// Scenario: due to a programming error, validation for
	// transaction (1,0) is scheduled after that for transaction
	// (2,0). This cannot happen and so we panic

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc, coll, key := "cc", "coll", "key"

	rwsetBytes := pvtRwsetUpdatingMetadataFor(cc, coll, key)

	pm.ExtractValidationParameterDependency(2, 0, rwsetBytes)
	panickingFunc := func() {
		pm.ExtractValidationParameterDependency(1, 0, rwsetBytes)
	}
	require.Panics(t, panickingFunc)
}

func TestLedgerErrors(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: we check that if a ledger error occurs,
	// GetValidationParameterForKey returns an error

	mr := &mockState{
		GetStateMetadataErr:             fmt.Errorf("Ledger error"),
		GetPrivateDataMetadataByHashErr: fmt.Errorf("Ledger error"),
	}
	ms := &mockStateFetcher{FetchStateRv: mr, FetchStateErr: fmt.Errorf("Ledger error")}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc, coll, key := "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	errC := make(chan error, 1)
	runFunctions(t, seed,
		func() {
			pm.ExtractValidationParameterDependency(1, 0, rwsetbytes)
		},
		func() {
			pm.SetTxValidationResult(cc, 1, 0, errors.New(""))
		},
		func() {
			_, err := pm.GetValidationParameterForKey(cc, coll, key, 1, 1)
			errC <- err
		})

	err := <-errC
	require.Errorf(t, err, "assert failure occurred with seed %d", seed)

	ms.FetchStateErr = nil

	runFunctions(t, seed,
		func() {
			pm.ExtractValidationParameterDependency(2, 0, rwsetbytes)
		},
		func() {
			pm.SetTxValidationResult(cc, 2, 0, errors.New(""))
		},
		func() {
			_, err := pm.GetValidationParameterForKey(cc, coll, key, 2, 1)
			errC <- err
		})

	err = <-errC
	require.Error(t, err)

	cc, coll, key = "cc", "coll", "key"

	rwsetbytes = pvtRwsetUpdatingMetadataFor(cc, coll, key)

	runFunctions(t, seed,
		func() {
			pm.ExtractValidationParameterDependency(3, 0, rwsetbytes)
		},
		func() {
			pm.SetTxValidationResult(cc, 3, 0, errors.New(""))
		},
		func() {
			_, err = pm.GetValidationParameterForKey(cc, coll, key, 3, 1)
			errC <- err
		})

	err = <-errC
	require.Errorf(t, err, "assert failure occurred with seed %d", seed)
	require.True(t, ms.DoneCalled(), "assert failure occurred with seed %d", seed)
}

func TestBadRwsetIsNoDependency(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: a transaction has a bogus read-write set.
	// While the transaction will fail eventually, we check
	// that our code doesn't break

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc, coll, key := "cc", "", "key"

	resC := make(chan []byte, 1)
	errC := make(chan error, 1)
	runFunctions(t, seed,
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
		})

	sp := <-resC
	err := <-errC
	require.NoError(t, err, "assert failure occurred with seed %d", seed)
	require.Equal(t, protoutil.MarshalOrPanic(spe), sp, "assert failure occurred with seed %d", seed)
	require.True(t, ms.DoneCalled(), "assert failure occurred with seed %d", seed)
}

func TestWritesIntoDifferentNamespaces(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: transaction (1,0) writes to namespace cc1.
	// Transaction (1,1) attempts to retrieve validation
	// parameters for cc. This does not constitute a dependency

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc, othercc, coll, key := "cc1", "cc", "", "key"

	rwsetbytes := rwsetUpdatingMetadataFor(cc, key)

	resC := make(chan []byte, 1)
	errC := make(chan error, 1)
	runFunctions(t, seed,
		func() {
			pm.SetTxValidationResult(cc, 1, 0, nil)
		},
		func() {
			pm.ExtractValidationParameterDependency(1, 0, rwsetbytes)
			sp, err := pm.GetValidationParameterForKey(othercc, coll, key, 1, 1)
			resC <- sp
			errC <- err
		})

	sp := <-resC
	err := <-errC
	require.NoError(t, err, "assert failure occurred with seed %d", seed)
	require.Equal(t, protoutil.MarshalOrPanic(spe), sp, "assert failure occurred with seed %d", seed)
	require.True(t, ms.DoneCalled(), "assert failure occurred with seed %d", seed)
}

func TestCombinedCalls(t *testing.T) {
	t.Parallel()
	seed := time.Now().Unix()

	// Scenario: transaction (1,3) requests validation parameters
	// for different keys - one succeeds and one fails.

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc := "cc"
	coll := ""
	key1 := "key1"
	key2 := "key2"

	res1C := make(chan []byte, 1)
	err1C := make(chan error, 1)
	res2C := make(chan []byte, 1)
	err2C := make(chan error, 1)
	runFunctions(t, seed,
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
		})

	sp := <-res1C
	err := <-err1C
	require.NoError(t, err, "assert failure occurred with seed %d", seed)
	require.Equal(t, protoutil.MarshalOrPanic(spe), sp, "assert failure occurred with seed %d", seed)

	sp = <-res2C
	err = <-err2C
	require.Errorf(t, err, "assert failure occurred with seed %d", seed)
	require.IsType(t, &ValidationParameterUpdatedError{}, err, "assert failure occurred with seed %d", seed)
	require.Nil(t, sp, "assert failure occurred with seed %d", seed)

	require.True(t, ms.DoneCalled(), "assert failure occurred with seed %d", seed)
}

func TestForRaces(t *testing.T) {
	seed := time.Now().Unix()

	// scenario to stress test the parallel validation
	// this is an extended combined test
	// run with go test -race

	vpMetadataKey := pb.MetaDataKeys_VALIDATION_PARAMETER.String()
	spe := policydsl.SignedByMspMember("foo")
	mr := &mockState{GetStateMetadataRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}, GetPrivateDataMetadataByHashRv: map[string][]byte{vpMetadataKey: protoutil.MarshalOrPanic(spe)}}
	ms := &mockStateFetcher{FetchStateRv: mr}
	pm := &KeyLevelValidationParameterManagerImpl{PolicyTranslator: &mockTranslator{}, StateFetcher: ms}

	cc := "cc"
	coll := ""

	nRoutines := 1000
	funcArray := make([]func(), nRoutines)
	for i := 0; i < nRoutines; i++ {
		txnum := i
		funcArray[i] = func() {
			key := strconv.Itoa(txnum)
			pm.ExtractValidationParameterDependency(1, uint64(txnum), rwsetUpdatingMetadataFor(cc, key))

			// we yield in an attempt to create a more varied scheduling pattern in the hope of unearthing races
			runtime.Gosched()

			pm.SetTxValidationResult(cc, 1, uint64(txnum), errors.New(""))

			// we yield in an attempt to create a more varied scheduling pattern in the hope of unearthing races
			runtime.Gosched()

			sp, err := pm.GetValidationParameterForKey(cc, coll, key, 1, 2)
			require.Equal(t, protoutil.MarshalOrPanic(spe), sp)
			require.NoError(t, err)
		}
	}

	runFunctions(t, seed, funcArray...)

	require.True(t, ms.DoneCalled(), "assert failure occurred with seed %d", seed)
}
