/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"math"
	"path"
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/confighistory/confighistorytest"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/stretchr/testify/require"
)

func TestPvtdataStoreCreatedFromSnapshot(t *testing.T) {
	type snapshotData struct {
		collection         string
		keyHash, valueHash []byte
		version            *version.Height
	}

	setup := func(snapshotData []*snapshotData) *Store {
		testDir := t.TempDir()
		conf := pvtDataConf()
		conf.StorePath = testDir

		p, err := NewProvider(conf)
		require.NoError(t, err)
		t.Cleanup(func() { p.Close() })

		configHistoryMgr, err := confighistorytest.NewMgr(path.Join(testDir, "config-history"))
		require.NoError(t, err)

		require.NoError(t,
			configHistoryMgr.Setup(
				"test-ledger", "ns",
				map[uint64][]*peer.StaticCollectionConfig{
					1: {
						{
							Name:             "eligible-coll",
							MemberOrgsPolicy: iamIn.toMemberOrgPolicy(),
							BlockToLive:      40,
						},

						{
							Name:             "ineligible-coll",
							MemberOrgsPolicy: iamOut.toMemberOrgPolicy(),
							BlockToLive:      50,
						},
					},
				},
			),
		)

		snapshotDataImporter, err := p.SnapshotDataImporterFor("test-ledger", 25,
			newMockMembershipProvider("myOrg"), configHistoryMgr.GetRetriever("test-ledger"), testDir,
		)
		require.NoError(t, err)

		for _, d := range snapshotData {
			require.NoError(t,
				snapshotDataImporter.ConsumeSnapshotData("ns", d.collection, d.keyHash, d.valueHash, d.version),
			)
		}
		require.NoError(t, snapshotDataImporter.Done())

		store, err := p.OpenStore("test-ledger")
		require.NoError(t, err)
		store.Init(
			btltestutil.SampleBTLPolicy(
				map[[2]string]uint64{
					{"ns", "eligible-coll"}:   40,
					{"ns", "ineligible-coll"}: 50,
				},
			),
		)
		return store
	}

	t.Run("basic-functions-on-store", func(t *testing.T) {
		store := setup(
			[]*snapshotData{
				{
					collection: "eligible-coll",
					keyHash:    []byte("eligible-coll-key-hash"),
					valueHash:  []byte("eligible-coll-value-hash"),
					version:    version.NewHeight(20, 200),
				},
			},
		)

		require.False(t, store.isEmpty)
		require.Equal(t, &bootsnapshotInfo{createdFromSnapshot: true, lastBlockInSnapshot: 25}, store.bootsnapshotInfo)

		isEmpty, lastBlkNum, err := store.getLastCommittedBlockNum()
		require.NoError(t, err)
		require.False(t, isEmpty)
		require.Equal(t, uint64(25), lastBlkNum)

		err = store.Commit(25, nil, nil, nil)
		require.EqualError(t, err, "expected block number=26, received block number=25")
		require.NoError(t, store.Commit(26, nil, nil, nil))
	})

	t.Run("fetch-bootkv-hashes", func(t *testing.T) {
		store := setup(
			[]*snapshotData{
				{
					collection: "eligible-coll",
					keyHash:    []byte("eligible-coll-key-hash-1"),
					valueHash:  []byte("eligible-coll-value-hash-1"),
					version:    version.NewHeight(20, 200),
				},
				{
					collection: "eligible-coll",
					keyHash:    []byte("eligible-coll-key-hash-2"),
					valueHash:  []byte("eligible-coll-value-hash-2"),
					version:    version.NewHeight(20, 200),
				},
			},
		)

		m, err := store.FetchBootKVHashes(20, 200, "ns", "eligible-coll")
		require.NoError(t, err)
		require.Equal(t,
			map[string][]byte{
				"eligible-coll-key-hash-1": []byte("eligible-coll-value-hash-1"),
				"eligible-coll-key-hash-2": []byte("eligible-coll-value-hash-2"),
			},
			m,
		)

		m, err = store.FetchBootKVHashes(25, 200, "ns", "eligible-coll")
		require.NoError(t, err)
		require.Len(t, m, 0)

		_, err = store.FetchBootKVHashes(26, 200, "ns", "eligible-coll")
		require.EqualError(t, err, "unexpected call. Boot KV Hashes are persisted only for the data imported from snapshot")
	})

	t.Run("committing-old-blocks-pvtdata-deletes-bootKV-hashes", func(t *testing.T) {
		store := setup(
			[]*snapshotData{
				{
					collection: "eligible-coll",
					keyHash:    []byte("eligible-coll-key-hash"),
					valueHash:  []byte("eligible-coll-value-hash"),
					version:    version.NewHeight(20, 200),
				},
				{
					collection: "ineligible-coll",
					keyHash:    []byte("ineligible-coll-key-hash"),
					valueHash:  []byte("ineligible-coll-value-hash"),
					version:    version.NewHeight(21, 210),
				},
				{
					collection: "eligible-coll",
					keyHash:    []byte("key-hash-at-last-block-in-snapshot"),
					valueHash:  []byte("value-hash-at-last-block-in-snapshot"),
					version:    version.NewHeight(25, 250),
				},
			},
		)

		m, err := store.FetchBootKVHashes(20, 200, "ns", "eligible-coll")
		require.NoError(t, err)
		require.Equal(t,
			map[string][]byte{
				"eligible-coll-key-hash": []byte("eligible-coll-value-hash"),
			},
			m,
		)

		m, err = store.FetchBootKVHashes(25, 250, "ns", "eligible-coll")
		require.NoError(t, err)
		require.Equal(t,
			map[string][]byte{
				"key-hash-at-last-block-in-snapshot": []byte("value-hash-at-last-block-in-snapshot"),
			},
			m,
		)

		m, err = store.FetchBootKVHashes(21, 210, "ns", "ineligible-coll")
		require.NoError(t, err)
		require.Equal(t,
			map[string][]byte{
				"ineligible-coll-key-hash": []byte("ineligible-coll-value-hash"),
			},
			m,
		)

		missingDataInfo, err := store.GetMissingPvtDataInfoForMostRecentBlocks(math.MaxUint64, 4)
		require.NoError(t, err)
		require.Equal(
			t,
			ledger.MissingPvtDataInfo{
				20: ledger.MissingBlockPvtdataInfo{
					200: []*ledger.MissingCollectionPvtDataInfo{
						{
							Namespace:  "ns",
							Collection: "eligible-coll",
						},
					},
				},
				25: ledger.MissingBlockPvtdataInfo{
					250: []*ledger.MissingCollectionPvtDataInfo{
						{
							Namespace:  "ns",
							Collection: "eligible-coll",
						},
					},
				},
			},
			missingDataInfo,
		)

		err = store.CommitPvtDataOfOldBlocks(
			map[uint64][]*ledger.TxPvtData{
				20: {
					produceSamplePvtdata(t, 200, []string{"ns:eligible-coll"}),
				},
				25: {
					produceSamplePvtdata(t, 250, []string{"ns:eligible-coll"}),
				},
			},
			nil,
		)
		require.NoError(t, err)

		missingDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(math.MaxUint64, 4)
		require.NoError(t, err)
		require.Len(t, missingDataInfo, 0)

		m, err = store.FetchBootKVHashes(20, 200, "ns", "eligible-coll")
		require.NoError(t, err)
		require.Len(t, m, 0)

		m, err = store.FetchBootKVHashes(25, 250, "ns", "eligible-coll")
		require.NoError(t, err)
		require.Len(t, m, 0)

		m, err = store.FetchBootKVHashes(21, 210, "ns", "ineligible-coll")
		require.NoError(t, err)
		require.Equal(t,
			map[string][]byte{
				"ineligible-coll-key-hash": []byte("ineligible-coll-value-hash"),
			},
			m,
		)
	})

	t.Run("purger-deletes-expired-bootKV-hashes", func(t *testing.T) {
		store := setup(
			[]*snapshotData{
				{
					collection: "eligible-coll",
					keyHash:    []byte("eligible-coll-key-hash"),
					valueHash:  []byte("eligible-coll-value-hash"),
					version:    version.NewHeight(20, 200),
				},
				{
					collection: "ineligible-coll",
					keyHash:    []byte("ineligible-coll-key-hash"),
					valueHash:  []byte("ineligible-coll-value-hash"),
					version:    version.NewHeight(21, 210),
				},
			},
		)

		m, err := store.FetchBootKVHashes(20, 200, "ns", "eligible-coll")
		require.NoError(t, err)
		require.Equal(t,
			map[string][]byte{
				"eligible-coll-key-hash": []byte("eligible-coll-value-hash"),
			},
			m,
		)

		m, err = store.FetchBootKVHashes(21, 210, "ns", "ineligible-coll")
		require.NoError(t, err)
		require.Equal(t,
			map[string][]byte{
				"ineligible-coll-key-hash": []byte("ineligible-coll-value-hash"),
			},
			m,
		)

		// commit 100 blocks and the bootkvhashes should expire
		store.purgeInterval = 10
		for i := 0; i < 100; i++ {
			require.NoError(t, store.Commit(uint64(26+i), nil, nil, nil))
		}

		m, err = store.FetchBootKVHashes(20, 200, "ns", "eligible-coll")
		require.NoError(t, err)
		require.Len(t, m, 0)

		m, err = store.FetchBootKVHashes(21, 210, "ns", "ineligible-coll")
		require.NoError(t, err)
		require.Len(t, m, 0)
	})
}

func TestStoreCreationErrorPath(t *testing.T) {
	testDir := t.TempDir()
	conf := pvtDataConf()
	conf.StorePath = testDir

	p, err := NewProvider(conf)
	require.NoError(t, err)
	defer p.Close()

	configHistoryMgr, err := confighistorytest.NewMgr(path.Join(testDir, "config-history"))
	require.NoError(t, err)

	t.Run("error-while-constructing-snapshot-data-importer", func(t *testing.T) {
		_, err = p.SnapshotDataImporterFor("test-ledger", 25,
			newMockMembershipProvider("myOrg"),
			configHistoryMgr.GetRetriever("test-ledger"),
			"non-existing-dir",
		)
		require.Contains(t, err.Error(), "error while creating temp dir for sorting rows")
	})

	t.Run("error-while-writing-snapshot-info-into-pvtdata-store", func(t *testing.T) {
		p.Close()
		_, err = p.SnapshotDataImporterFor("test-ledger", 25,
			newMockMembershipProvider("myOrg"),
			configHistoryMgr.GetRetriever("test-ledger"),
			testDir,
		)
		require.Contains(t, err.Error(), "error while writing snapshot info to db")
	})
}
