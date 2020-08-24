/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtstatepurgemgmt

import (
	"errors"
	"math"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/stretchr/testify/require"
)

func TestPurgeMgrBuilder(t *testing.T) {
	bookkeepingEnv := bookkeeping.NewTestEnv(t)
	defer bookkeepingEnv.Cleanup()

	ledgerID := "test-ledger"
	bookkeepingProvider := bookkeepingEnv.TestProvider
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns1", "coll1"}: 1,
			{"ns2", "coll2"}: 2,
		},
	)

	purgeMgrBuilder := NewPurgeMgrBuilder(ledgerID, btlPolicy, bookkeepingProvider)

	require.NoError(t,
		purgeMgrBuilder.ConsumeSnapshotData(
			"ns1",
			"never-expiring-collection",
			[]byte("key-hash-3"),
			[]byte("value-hash-3"),
			version.NewHeight(5, 1)),
	)
	expiry, err := purgeMgrBuilder.expKeeper.retrieveByExpiryKey(
		&expiryInfoKey{
			expiryBlk:     math.MaxUint64,
			committingBlk: 5,
		},
	)
	require.NoError(t, err)
	require.Nil(t, expiry.pvtdataKeys.Map)

	// add data that expires at block-7
	require.NoError(t,
		purgeMgrBuilder.ConsumeSnapshotData(
			"ns1",
			"coll1",
			[]byte("key-hash-1"),
			[]byte("value-hash-1"),
			version.NewHeight(5, 1),
		),
	)
	require.NoError(t,
		purgeMgrBuilder.ConsumeSnapshotData(
			"ns1", "coll1",
			[]byte("key-hash-2"),
			[]byte("value-hash-2"),
			version.NewHeight(5, 1),
		),
	)
	// add data that expires at block-8
	require.NoError(t,
		purgeMgrBuilder.ConsumeSnapshotData(
			"ns2",
			"coll2",
			[]byte("key-hash-2"),
			[]byte("value-hash-2"),
			version.NewHeight(5, 1),
		),
	)

	purgeMgr, err := InstantiatePurgeMgr(ledgerID, nil, btlPolicy, bookkeepingProvider)
	require.NoError(t, err)

	testcases := []struct {
		name           string
		expiringBlock  uint64
		expectedOutput []*expiryInfo
	}{

		{
			name:           "nothing-expires-at-block-6",
			expiringBlock:  6,
			expectedOutput: nil,
		},

		{
			name:          "data-expiring-at-block-7",
			expiringBlock: 7,
			expectedOutput: []*expiryInfo{
				{
					expiryInfoKey: &expiryInfoKey{
						committingBlk: 5,
						expiryBlk:     7,
					},
					pvtdataKeys: &PvtdataKeys{
						Map: map[string]*Collections{
							"ns1": {
								Map: map[string]*KeysAndHashes{
									"coll1": {
										List: []*KeyAndHash{
											{
												Hash: []byte("key-hash-1"),
											},
											{
												Hash: []byte("key-hash-2"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},

		{
			name:          "data-expiring-at-block-8",
			expiringBlock: 8,
			expectedOutput: []*expiryInfo{
				{
					expiryInfoKey: &expiryInfoKey{
						committingBlk: 5,
						expiryBlk:     8,
					},
					pvtdataKeys: &PvtdataKeys{
						Map: map[string]*Collections{
							"ns2": {
								Map: map[string]*KeysAndHashes{
									"coll2": {
										List: []*KeyAndHash{
											{
												Hash: []byte("key-hash-2"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},

		{
			name:           "nothing-expires-at-block-9",
			expiringBlock:  9,
			expectedOutput: nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := purgeMgr.expKeeper.retrieve(tc.expiringBlock)
			require.NoError(t, err)
			require.Equal(t, tc.expectedOutput, output)
		})
	}
}

func TestPurgeMgrBuilderErrorsPropagation(t *testing.T) {
	var btlPolicy pvtdatapolicy.BTLPolicy
	var bookkeepingProvider *bookkeeping.Provider

	init := func() {
		bookkeepingEnv := bookkeeping.NewTestEnv(t)
		bookkeepingProvider = bookkeepingEnv.TestProvider
		btlPolicy = btltestutil.SampleBTLPolicy(
			map[[2]string]uint64{
				{"ns", "coll"}: 1,
			},
		)
		t.Cleanup(bookkeepingEnv.Cleanup)
	}

	t.Run("btlpolicy-returns-error", func(t *testing.T) {
		init()
		btlPolicy = &btltestutil.ErrorCausingBTLPolicy{
			Err: errors.New("btl-error"),
		}
		purgeMgrBuilder := NewPurgeMgrBuilder("test-ledger", btlPolicy, bookkeepingProvider)
		err := purgeMgrBuilder.ConsumeSnapshotData(
			"ns",
			"coll",
			[]byte("key-hash"),
			[]byte("value-hash"),
			version.NewHeight(1, 1),
		)
		require.EqualError(t, err, "error from btlpolicy: btl-error")
	})

	t.Run("bookkeeper-returns-error", func(t *testing.T) {
		init()
		bookkeepingProvider.Close()
		purgeMgrBuilder := NewPurgeMgrBuilder("test-ledger", btlPolicy, bookkeepingProvider)
		err := purgeMgrBuilder.ConsumeSnapshotData(
			"ns",
			"coll",
			[]byte("key-hash"),
			[]byte("value-hash"),
			version.NewHeight(1, 1),
		)
		require.Contains(t, err.Error(), "error from bookkeeper")
	})
}
