/*
Copyright 2018 Hitachi, Ltd. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package internal

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/stretchr/testify/assert"
)

func TestNewPubAndHashUpdates(t *testing.T) {
	expected := &PubAndHashUpdates{
		privacyenabledstate.NewPubUpdateBatch(),
		privacyenabledstate.NewHashedUpdateBatch(),
	}

	actual := NewPubAndHashUpdates()
	assert.Equal(t, expected, actual)
}

func TestContainsPvtWrites_ReturnsTrue(t *testing.T) {
	chrs1 := &rwsetutil.CollHashedRwSet{PvtRwSetHash: []byte{0}}
	nrs1 := &rwsetutil.NsRwSet{CollHashedRwSets: []*rwsetutil.CollHashedRwSet{chrs1}}
	trs1 := &rwsetutil.TxRwSet{NsRwSets: []*rwsetutil.NsRwSet{nrs1}}
	tx1 := &Transaction{RWSet: trs1}

	ret1 := tx1.ContainsPvtWrites()
	assert.True(t, ret1)
}

func TestContainsPvtWrites_ReturnsFalse(t *testing.T) {
	nrs2 := &rwsetutil.NsRwSet{CollHashedRwSets: []*rwsetutil.CollHashedRwSet{}}
	trs2 := &rwsetutil.TxRwSet{NsRwSets: []*rwsetutil.NsRwSet{nrs2}}
	tx2 := &Transaction{RWSet: trs2}

	ret2 := tx2.ContainsPvtWrites()
	assert.False(t, ret2)
}

func TestRetrieveHash(t *testing.T) {
	expected := []byte{0xde, 0xad, 0xbe, 0xef}
	coll1 := "coll1"
	ns1 := "ns1"
	chrs1 := &rwsetutil.CollHashedRwSet{
		CollectionName: coll1,
		PvtRwSetHash:   expected}
	nrs1 := &rwsetutil.NsRwSet{
		NameSpace:        ns1,
		CollHashedRwSets: []*rwsetutil.CollHashedRwSet{chrs1}}
	trs1 := &rwsetutil.TxRwSet{NsRwSets: []*rwsetutil.NsRwSet{nrs1}}
	tx1 := &Transaction{RWSet: trs1}

	actual := tx1.RetrieveHash(ns1, coll1)
	assert.Equal(t, expected, actual)
}

func TestRetrieveHash_RWSetIsNil(t *testing.T) {
	coll2 := "coll2"
	ns2 := "ns2"
	tx2 := &Transaction{RWSet: nil}

	ret2 := tx2.RetrieveHash(ns2, coll2)
	assert.Nil(t, ret2)
}

func TestRetrieveHash_NsRwSetsNotEqualsToNs(t *testing.T) {
	coll3 := "coll3"
	ns3 := "ns3"
	nrs3 := &rwsetutil.NsRwSet{
		NameSpace:        "ns",
		CollHashedRwSets: []*rwsetutil.CollHashedRwSet{}}
	trs3 := &rwsetutil.TxRwSet{NsRwSets: []*rwsetutil.NsRwSet{nrs3}}
	tx3 := &Transaction{RWSet: trs3}

	ret3 := tx3.RetrieveHash(ns3, coll3)
	assert.Nil(t, ret3)
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
	pahu := NewPubAndHashUpdates()
	pahu.PubUpdates.Put(ns1, key1, value1, ver1)
	pahu.HashUpdates.Put(ns1, coll1, key3, value3, ver1)

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
	expected := NewPubAndHashUpdates()
	// Cannot put nil value directly, because it causes panic.
	// Insted, put real value and delete it.
	expected.PubUpdates.Put(ns1, key1, value1, ver1)
	expected.PubUpdates.Delete(ns1, key1, ver1)
	expected.PubUpdates.Put(ns1, key2, value2, ver1)
	expected.HashUpdates.Put(ns1, coll1, key3, value3, ver1)
	expected.HashUpdates.Delete(ns1, coll1, key3, ver1)
	expected.HashUpdates.Put(ns1, coll1, key4, value4, ver1)

	testdbEnv := &privacyenabledstate.LevelDBCommonStorageTestEnv{}
	testdbEnv.Init(t)
	defer testdbEnv.Cleanup()
	testdb := testdbEnv.GetDBHandle("testdb")

	// Call
	pahu.ApplyWriteSet(txRWSet1, ver1, testdb)

	// Check result
	assert.Equal(t, expected, pahu)
}
