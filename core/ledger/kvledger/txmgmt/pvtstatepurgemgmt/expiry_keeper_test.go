/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtstatepurgemgmt

import (
	fmt "fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/stretchr/testify/require"
)

func TestExpiryKVEncoding(t *testing.T) {
	pvtdataKeys := newPvtdataKeys()
	pvtdataKeys.add("ns1", "coll-1", "key-1", []byte("key-1-hash"))
	expiryInfo := &expiryInfo{&expiryInfoKey{expiryBlk: 10, committingBlk: 2}, pvtdataKeys}
	t.Logf("expiryInfo:%s", spew.Sdump(expiryInfo))
	k, v, err := encodeKV(expiryInfo)
	require.NoError(t, err)
	expiryInfo1, err := decodeExpiryInfo(k, v)
	require.NoError(t, err)
	require.Equal(t, expiryInfo.expiryInfoKey, expiryInfo1.expiryInfoKey)
	require.True(t, proto.Equal(expiryInfo.pvtdataKeys, expiryInfo1.pvtdataKeys), "proto messages are not equal")
}

func TestExpiryKeeper(t *testing.T) {
	testenv := bookkeeping.NewTestEnv(t)
	defer testenv.Cleanup()
	expiryKeeper := newExpiryKeeper("testledger", testenv.TestProvider)

	expinfo1 := &expiryInfo{&expiryInfoKey{committingBlk: 3, expiryBlk: 13}, buildPvtdataKeysForTest(1, 1)}
	expinfo2 := &expiryInfo{&expiryInfoKey{committingBlk: 3, expiryBlk: 15}, buildPvtdataKeysForTest(2, 2)}
	expinfo3 := &expiryInfo{&expiryInfoKey{committingBlk: 4, expiryBlk: 13}, buildPvtdataKeysForTest(3, 3)}
	expinfo4 := &expiryInfo{&expiryInfoKey{committingBlk: 5, expiryBlk: 17}, buildPvtdataKeysForTest(4, 4)}

	// Insert entries for keys at committingBlk 3
	require.NoError(t, expiryKeeper.update([]*expiryInfo{expinfo1, expinfo2}, nil))
	// Insert entries for keys at committingBlk 4 and 5
	require.NoError(t, expiryKeeper.update([]*expiryInfo{expinfo3, expinfo4}, nil))

	// Retrieve entries by expiring block 13, 15, and 17
	listExpinfo1, _ := expiryKeeper.retrieve(13)
	require.Len(t, listExpinfo1, 2)
	require.Equal(t, expinfo1.expiryInfoKey, listExpinfo1[0].expiryInfoKey)
	require.True(t, proto.Equal(expinfo1.pvtdataKeys, listExpinfo1[0].pvtdataKeys))
	require.Equal(t, expinfo3.expiryInfoKey, listExpinfo1[1].expiryInfoKey)
	require.True(t, proto.Equal(expinfo3.pvtdataKeys, listExpinfo1[1].pvtdataKeys))

	listExpinfo2, _ := expiryKeeper.retrieve(15)
	require.Len(t, listExpinfo2, 1)
	require.Equal(t, expinfo2.expiryInfoKey, listExpinfo2[0].expiryInfoKey)
	require.True(t, proto.Equal(expinfo2.pvtdataKeys, listExpinfo2[0].pvtdataKeys))

	listExpinfo3, _ := expiryKeeper.retrieve(17)
	require.Len(t, listExpinfo3, 1)
	require.Equal(t, expinfo4.expiryInfoKey, listExpinfo3[0].expiryInfoKey)
	require.True(t, proto.Equal(expinfo4.pvtdataKeys, listExpinfo3[0].pvtdataKeys))

	// Clear entries for keys expiring at block 13 and 15 and again retrieve by expiring block 13, 15, and 17
	require.NoError(t,
		expiryKeeper.update(nil, []*expiryInfoKey{expinfo1.expiryInfoKey, expinfo2.expiryInfoKey, expinfo3.expiryInfoKey}),
	)
	listExpinfo4, _ := expiryKeeper.retrieve(13)
	require.Nil(t, listExpinfo4)

	listExpinfo5, _ := expiryKeeper.retrieve(15)
	require.Nil(t, listExpinfo5)

	listExpinfo6, _ := expiryKeeper.retrieve(17)
	require.Len(t, listExpinfo6, 1)
	require.Equal(t, expinfo4.expiryInfoKey, listExpinfo6[0].expiryInfoKey)
	require.True(t, proto.Equal(expinfo4.pvtdataKeys, listExpinfo6[0].pvtdataKeys))

	t.Run("test-error-path", func(t *testing.T) {
		testenv.TestProvider.Close()
		expirtyInfo, err := expiryKeeper.retrieve(15)
		require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")
		require.Nil(t, expirtyInfo)
	})
}

func buildPvtdataKeysForTest(startingEntry int, numEntries int) *PvtdataKeys {
	pvtdataKeys := newPvtdataKeys()
	for i := startingEntry; i <= startingEntry+numEntries; i++ {
		pvtdataKeys.add(fmt.Sprintf("ns-%d", i), fmt.Sprintf("coll-%d", i), fmt.Sprintf("key-%d", i), []byte(fmt.Sprintf("key-%d-hash", i)))
	}
	return pvtdataKeys
}
