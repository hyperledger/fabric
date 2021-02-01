/*
Copyright 2018 Hitachi, Ltd. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/stretchr/testify/require"
)

func TestNewPubAndHashUpdates(t *testing.T) {
	expected := &publicAndHashUpdates{
		privacyenabledstate.NewPubUpdateBatch(),
		privacyenabledstate.NewHashedUpdateBatch(),
	}

	actual := newPubAndHashUpdates()
	require.Equal(t, expected, actual)
}

func TestContainsPostOrderWrites(t *testing.T) {
	u := newPubAndHashUpdates()
	rws := &rwsetutil.TxRwSet{}
	require.NoError(t, u.applyWriteSet(rws, nil, nil, false))
	require.False(t, u.publicUpdates.ContainsPostOrderWrites)
	require.NoError(t, u.applyWriteSet(rws, nil, nil, true))
	require.True(t, u.publicUpdates.ContainsPostOrderWrites)
	// once set to true, should always return true
	require.NoError(t, u.applyWriteSet(rws, nil, nil, false))
	require.True(t, u.publicUpdates.ContainsPostOrderWrites)
}

func TestContainsPvtWrites_ReturnsTrue(t *testing.T) {
	chrs1 := &rwsetutil.CollHashedRwSet{PvtRwSetHash: []byte{0}}
	nrs1 := &rwsetutil.NsRwSet{CollHashedRwSets: []*rwsetutil.CollHashedRwSet{chrs1}}
	trs1 := &rwsetutil.TxRwSet{NsRwSets: []*rwsetutil.NsRwSet{nrs1}}
	tx1 := &transaction{rwset: trs1}

	ret1 := tx1.containsPvtWrites()
	require.True(t, ret1)
}

func TestContainsPvtWrites_ReturnsFalse(t *testing.T) {
	nrs2 := &rwsetutil.NsRwSet{CollHashedRwSets: []*rwsetutil.CollHashedRwSet{}}
	trs2 := &rwsetutil.TxRwSet{NsRwSets: []*rwsetutil.NsRwSet{nrs2}}
	tx2 := &transaction{rwset: trs2}

	ret2 := tx2.containsPvtWrites()
	require.False(t, ret2)
}

func TestRetrieveHash(t *testing.T) {
	expected := []byte{0xde, 0xad, 0xbe, 0xef}
	coll1 := "coll1"
	ns1 := "ns1"
	chrs1 := &rwsetutil.CollHashedRwSet{
		CollectionName: coll1,
		PvtRwSetHash:   expected,
	}
	nrs1 := &rwsetutil.NsRwSet{
		NameSpace:        ns1,
		CollHashedRwSets: []*rwsetutil.CollHashedRwSet{chrs1},
	}
	trs1 := &rwsetutil.TxRwSet{NsRwSets: []*rwsetutil.NsRwSet{nrs1}}
	tx1 := &transaction{rwset: trs1}

	actual := tx1.retrieveHash(ns1, coll1)
	require.Equal(t, expected, actual)
}

func TestRetrieveHash_RWSetIsNil(t *testing.T) {
	coll2 := "coll2"
	ns2 := "ns2"
	tx2 := &transaction{rwset: nil}

	ret2 := tx2.retrieveHash(ns2, coll2)
	require.Nil(t, ret2)
}

func TestRetrieveHash_NsRwSetsNotEqualsToNs(t *testing.T) {
	coll3 := "coll3"
	ns3 := "ns3"
	nrs3 := &rwsetutil.NsRwSet{
		NameSpace:        "ns",
		CollHashedRwSets: []*rwsetutil.CollHashedRwSet{},
	}
	trs3 := &rwsetutil.TxRwSet{NsRwSets: []*rwsetutil.NsRwSet{nrs3}}
	tx3 := &transaction{rwset: trs3}

	ret3 := tx3.retrieveHash(ns3, coll3)
	require.Nil(t, ret3)
}

func TestApplyWriteSet(t *testing.T) {
	// Test parameters
	ns1 := "ns1"
	ver1 := &version.Height{BlockNum: 1, TxNum: 2}
	key1, value1 := "key1", []byte{11}
	key2, value2 := "key2", []byte{22}
	key3, value3 := []byte{3}, []byte{33}
	key4, value4 := []byte{4}, []byte{44}
	coll1 := "coll1"

	// Set initial state. key1/value1, key3/value3 are stored into WriteSet.
	pahu := newPubAndHashUpdates()
	pahu.publicUpdates.Put(ns1, key1, value1, ver1)
	pahu.hashUpdates.Put(ns1, coll1, key3, value3, ver1)

	// Prepare parameters
	krs1 := &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{
		{IsDelete: true, Key: key1},
		{IsDelete: false, Key: key2, Value: value2},
	}}
	hrws1 := &kvrwset.HashedRWSet{HashedWrites: []*kvrwset.KVWriteHash{
		{IsDelete: true, KeyHash: key3},
		{IsDelete: false, KeyHash: key4, ValueHash: value4},
	}}
	chrs1 := &rwsetutil.CollHashedRwSet{HashedRwSet: hrws1, CollectionName: coll1}
	nrs1 := &rwsetutil.NsRwSet{NameSpace: ns1, KvRwSet: krs1, CollHashedRwSets: []*rwsetutil.CollHashedRwSet{chrs1}}
	txRWSet1 := &rwsetutil.TxRwSet{NsRwSets: []*rwsetutil.NsRwSet{nrs1}}

	// Set expected state
	expected := newPubAndHashUpdates()
	// Cannot put nil value directly, because it causes panic.
	// Insted, put real value and delete it.
	expected.publicUpdates.Put(ns1, key1, value1, ver1)
	expected.publicUpdates.Delete(ns1, key1, ver1)
	expected.publicUpdates.Put(ns1, key2, value2, ver1)
	expected.hashUpdates.Put(ns1, coll1, key3, value3, ver1)
	expected.hashUpdates.Delete(ns1, coll1, key3, ver1)
	expected.hashUpdates.Put(ns1, coll1, key4, value4, ver1)

	testdbEnv := &privacyenabledstate.LevelDBTestEnv{}
	testdbEnv.Init(t)
	defer testdbEnv.Cleanup()
	testdb := testdbEnv.GetDBHandle("testdb")

	// Call
	require.NoError(t, pahu.applyWriteSet(txRWSet1, ver1, testdb, false))

	// Check result
	require.Equal(t, expected, pahu)
}
