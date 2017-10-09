/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtstatepurgemgmt

import (
	fmt "fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/stretchr/testify/assert"
)

func TestExpiryKVEncoding(t *testing.T) {
	pvtdataKeys := newPvtdataKeys()
	pvtdataKeys.add("ns1", "coll-1", "key-1", []byte("key-1-hash"))
	expiryInfo := &expiryInfo{&expiryInfoKey{expiryBlk: 10, committingBlk: 2}, pvtdataKeys}
	t.Logf("expiryInfo:%s", spew.Sdump(expiryInfo))
	k, v, err := encodeKV(expiryInfo)
	testutil.AssertNoError(t, err, "")
	expiryInfo1, err := decodeExpiryInfo(k, v)
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, expiryInfo, expiryInfo1)
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
	expiryKeeper.updateBookkeeping([]*expiryInfo{expinfo1, expinfo2}, nil)
	// Insert entries for keys at committingBlk 4 and 5
	expiryKeeper.updateBookkeeping([]*expiryInfo{expinfo3, expinfo4}, nil)

	// Retrieve entries by expiring block 13, 15, and 17
	listExpinfo1, _ := expiryKeeper.retrieve(13)
	assert.Equal(t, []*expiryInfo{expinfo1, expinfo3}, listExpinfo1)

	listExpinfo2, _ := expiryKeeper.retrieve(15)
	assert.Equal(t, []*expiryInfo{expinfo2}, listExpinfo2)

	listExpinfo3, _ := expiryKeeper.retrieve(17)
	assert.Equal(t, []*expiryInfo{expinfo4}, listExpinfo3)

	// Clear entries for keys expiring at block 13 and 15 and again retrieve by expiring block 13, 15, and 17
	expiryKeeper.updateBookkeeping(nil, []*expiryInfoKey{expinfo1.expiryInfoKey, expinfo2.expiryInfoKey, expinfo3.expiryInfoKey})
	listExpinfo4, _ := expiryKeeper.retrieve(13)
	assert.Nil(t, listExpinfo4)

	listExpinfo5, _ := expiryKeeper.retrieve(15)
	assert.Nil(t, listExpinfo5)

	listExpinfo6, _ := expiryKeeper.retrieve(17)
	assert.Equal(t, []*expiryInfo{expinfo4}, listExpinfo6)
}

func buildPvtdataKeysForTest(startingEntry int, numEntries int) *PvtdataKeys {
	pvtdataKeys := newPvtdataKeys()
	for i := startingEntry; i <= startingEntry+numEntries; i++ {
		pvtdataKeys.add(fmt.Sprintf("ns-%d", i), fmt.Sprintf("coll-%d", i), fmt.Sprintf("key-%d", i), []byte(fmt.Sprintf("key-%d-hash", i)))
	}
	return pvtdataKeys
}
