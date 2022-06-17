/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/require"
)

// TestConstructHashedIndexAndUpgradeDataFmtRetroactively test that we create hashed indexes retroactively on the existing
// data and we upgrade the existing data in V11 format to V12 format.
// This test uses a pvt data store that is produced in one of the upgrade tests.
// The store contains total 15 blocks. Block number one to nine has not
// pvt data because, that time peer code was v1.0 and hence no pvt data. Block 10 contains
// a pvtdata from peer v1.1. Block 11 - 13 has not pvt data. Block 14 has pvt data from peer v1.2

func TestConstructHashedIndexAndUpgradeDataFmtRetroactively(t *testing.T) {
	testWorkingDir := t.TempDir()

	require.NoError(t, testutil.CopyDir("testdata/v11_v12/ledgersData/pvtdataStore", testWorkingDir, false))
	storePath := filepath.Join(testWorkingDir, "pvtdataStore")

	require.NoError(t, CheckAndConstructHashedIndex(storePath, []string{"ch1"}))

	pvtdataConf := pvtDataConf()
	pvtdataConf.StorePath = storePath

	storeProvider, err := NewProvider(pvtdataConf)
	require.NoError(t, err)
	defer storeProvider.Close()

	s, err := storeProvider.OpenStore("ch1")
	require.NoError(t, err)
	s.Init(btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"marbles_private", "collectionMarbles"}:              0,
			{"marbles_private", "collectionMarblePrivateDetails"}: 0,
		},
	))

	t.Run("v11-data-got-upgraded-to-v12-fmt", func(t *testing.T) {
		startKey, endKey := getDataKeysForRangeScanByBlockNum(10)
		itr, err := s.db.GetIterator(startKey, endKey)
		require.NoError(t, err)
		defer itr.Release()
		for itr.Next() {
			dataKeyBytes := itr.Key()
			v11Fmt, err := v11Format(dataKeyBytes)
			require.NoError(t, err)
			require.False(t, v11Fmt)
		}
	})

	t.Run("upgraded-v11-data-can-be-retrieved", func(t *testing.T) {
		for blk := 0; blk < 10; blk++ {
			checkDataNotExists(t, s, blk)
		}

		pvtdata, err := s.GetPvtDataByBlockNum(10, nil)
		require.NoError(t, err)
		require.Equal(t, 1, len(pvtdata))
		require.Equal(t, uint64(0), pvtdata[0].SeqInBlock)
		pvtWS, err := rwsetutil.TxPvtRwSetFromProtoMsg(pvtdata[0].WriteSet)
		require.NoError(t, err)

		require.Equal(t, &rwsetutil.TxPvtRwSet{
			NsPvtRwSet: []*rwsetutil.NsPvtRwSet{
				{
					NameSpace: "marbles_private",
					CollPvtRwSets: []*rwsetutil.CollPvtRwSet{
						{
							CollectionName: "collectionMarblePrivateDetails",
							KvRwSet: &kvrwset.KVRWSet{
								Writes: []*kvrwset.KVWrite{
									{
										Key:   "marble1",
										Value: []byte(`{"docType":"marblePrivateDetails","name":"marble1","price":150}`),
									},
								},
							},
						},

						{
							CollectionName: "collectionMarbles",
							KvRwSet: &kvrwset.KVRWSet{
								Writes: []*kvrwset.KVWrite{
									{
										Key:   "marble1",
										Value: []byte(`{"docType":"marble","name":"marble1","color":"blue","size":100,"owner":"tom"}`),
									},
								},
							},
						},
					},
				},
			},
		},
			pvtWS,
		)

		for blk := 11; blk < 14; blk++ {
			checkDataNotExists(t, s, blk)
		}

		pvtdata, err = s.GetPvtDataByBlockNum(14, nil)
		require.NoError(t, err)
		require.Equal(t, 1, len(pvtdata))
		require.Equal(t, uint64(0), pvtdata[0].SeqInBlock)
		pvtWS, err = rwsetutil.TxPvtRwSetFromProtoMsg(pvtdata[0].WriteSet)
		require.NoError(t, err)

		require.Equal(t, &rwsetutil.TxPvtRwSet{
			NsPvtRwSet: []*rwsetutil.NsPvtRwSet{
				{
					NameSpace: "marbles_private",
					CollPvtRwSets: []*rwsetutil.CollPvtRwSet{
						{
							CollectionName: "collectionMarblePrivateDetails",
							KvRwSet: &kvrwset.KVRWSet{
								Writes: []*kvrwset.KVWrite{
									{
										Key:   "marble2",
										Value: []byte(`{"docType":"marblePrivateDetails","name":"marble2","price":250}`),
									},
								},
							},
						},

						{
							CollectionName: "collectionMarbles",
							KvRwSet: &kvrwset.KVRWSet{
								Writes: []*kvrwset.KVWrite{
									{
										Key:   "marble2",
										Value: []byte(`{"docType":"marble","name":"marble2","color":"red","size":100,"owner":"tom"}`),
									},
								},
							},
						},
					},
				},
			},
		},
			pvtWS,
		)

		_, err = s.GetPvtDataByBlockNum(uint64(15), nil)
		require.EqualError(t, err, "last committed block number [14] smaller than the requested block number [15]")
	})

	t.Run("hashed-indexs-get-constructed", func(t *testing.T) {
		expectedIndexEntries := []*hashedIndexKey{
			{
				ns:         "marbles_private",
				coll:       "collectionMarblePrivateDetails",
				pvtkeyHash: util.ComputeStringHash("marble1"),
				blkNum:     10,
				txNum:      0,
			},
			{
				ns:         "marbles_private",
				coll:       "collectionMarbles",
				pvtkeyHash: util.ComputeStringHash("marble1"),
				blkNum:     10,
				txNum:      0,
			},
			{
				ns:         "marbles_private",
				coll:       "collectionMarblePrivateDetails",
				pvtkeyHash: util.ComputeStringHash("marble2"),
				blkNum:     14,
				txNum:      0,
			},
			{
				ns:         "marbles_private",
				coll:       "collectionMarbles",
				pvtkeyHash: util.ComputeStringHash("marble2"),
				blkNum:     14,
				txNum:      0,
			},
		}

		for i, e := range expectedIndexEntries {
			t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
				val, err := s.db.Get(encodeHashedIndexKey(e))
				require.NoError(t, err)
				require.Equal(t, e.pvtkeyHash, util.ComputeHash(val))
			})
		}
	})

	// keep this as last test as this closes the storeProvider
	t.Run("hashed-indexs-construction-is-done-only-once", func(t *testing.T) {
		storeProvider.Close()
		err := constructHashedIndex(storePath, []string{"ch1"})
		require.ErrorContains(t, err, "data format = [2.5], expected format = []")
	})
}

func checkDataNotExists(t *testing.T, s *Store, blkNum int) {
	data, err := s.GetPvtDataByBlockNum(uint64(blkNum), nil)
	require.NoError(t, err)
	require.Nil(t, data)
}
