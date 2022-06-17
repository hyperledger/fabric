/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"fmt"
	"math"
	"path"
	"testing"

	"github.com/bits-and-blooms/bitset"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/chaincode/implicitcollection"
	"github.com/hyperledger/fabric/core/ledger/confighistory/confighistorytest"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/stretchr/testify/require"
)

func TestSnapshotImporter(t *testing.T) {
	ledgerID := "test-ledger"
	myMSPID := "myOrg"

	// set smaller batch size for testing
	originalSnapshotRowSortBatchSize := maxSnapshotRowSortBatchSize
	originalBatchLenForSnapshotImport := maxBatchLenForSnapshotImport
	maxSnapshotRowSortBatchSize = 1
	maxBatchLenForSnapshotImport = 1
	defer func() {
		maxSnapshotRowSortBatchSize = originalSnapshotRowSortBatchSize
		maxBatchLenForSnapshotImport = originalBatchLenForSnapshotImport
	}()

	setup := func() (*SnapshotDataImporter, *confighistorytest.Mgr, *dbEntriesVerifier) {
		testDir := t.TempDir()
		dbProvider, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: testDir})
		require.NoError(t, err)
		t.Cleanup(func() { dbProvider.Close() })

		configHistoryMgr, err := confighistorytest.NewMgr(path.Join(testDir, "config-history"))
		require.NoError(t, err)
		t.Cleanup(func() { configHistoryMgr.Close() })

		db := dbProvider.GetDBHandle(ledgerID)

		snapshotDataImporter, err := newSnapshotDataImporter(
			ledgerID,
			db,
			newMockMembershipProvider(myMSPID),
			configHistoryMgr.GetRetriever(ledgerID),
			testDir,
		)
		require.NoError(t, err)

		return snapshotDataImporter, configHistoryMgr, &dbEntriesVerifier{t, db}
	}

	t.Run("coll-eligible-and-never-expires", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr, dbVerifier := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{
				15: {
					{
						Name:             "coll",
						MemberOrgsPolicy: iamIn.toMemberOrgPolicy(),
						BlockToLive:      0,
					},
				},
			},
		)
		require.NoError(t, err)

		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.NoError(t, err)
		require.NoError(t, snapshotDataImporter.Done())

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: "coll"},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyElgMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: 20}},
			(&bitset.BitSet{}).Set(300),
		)

		dbVerifier.verifyNoExpiryEntries()
	})

	t.Run("coll-ineligible-and-never-expires", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr, dbVerifier := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{
				15: {
					{
						Name:             "coll",
						MemberOrgsPolicy: iamOut.toMemberOrgPolicy(),
						BlockToLive:      0,
					},
				},
			},
		)
		require.NoError(t, err)

		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.NoError(t, err)
		require.NoError(t, snapshotDataImporter.Done())

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: "coll"},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyInelgMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: 20}},
			(&bitset.BitSet{}).Set(300),
		)

		dbVerifier.verifyNoExpiryEntries()
	})

	t.Run("coll-eligible-and-expires", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr, dbVerifier := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{
				15: {
					{
						Name:             "coll",
						MemberOrgsPolicy: iamIn.toMemberOrgPolicy(),
						BlockToLive:      30,
					},
				},
			},
		)
		require.NoError(t, err)

		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.NoError(t, err)
		require.NoError(t, snapshotDataImporter.Done())

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: "coll"},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyElgMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: 20}},
			(&bitset.BitSet{}).Set(300),
		)

		dbVerifier.verifyExpiryEntry(
			&expiryKey{committingBlk: 20, expiringBlk: 20 + 30 + 1},
			&ExpiryData{
				Map: map[string]*NamespaceExpiryData{
					"ns": {
						MissingData: map[string]bool{
							"coll": true,
						},
						BootKVHashes: map[string]*TxNums{
							"coll": {List: []uint64{300}},
						},
					},
				},
			},
		)
	})

	t.Run("coll-ineligible-and-expires", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr, dbVerifier := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{
				15: {
					{
						Name:             "coll",
						MemberOrgsPolicy: iamOut.toMemberOrgPolicy(),
						BlockToLive:      30,
					},
				},
			},
		)
		require.NoError(t, err)

		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.NoError(t, err)
		require.NoError(t, snapshotDataImporter.Done())

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: "coll"},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyInelgMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: 20}},
			(&bitset.BitSet{}).Set(300),
		)

		dbVerifier.verifyExpiryEntry(
			&expiryKey{committingBlk: 20, expiringBlk: 20 + 30 + 1},
			&ExpiryData{
				Map: map[string]*NamespaceExpiryData{
					"ns": {
						MissingData: map[string]bool{
							"coll": true,
						},
						BootKVHashes: map[string]*TxNums{
							"coll": {List: []uint64{300}},
						},
					},
				},
			},
		)
	})

	t.Run("implicit-coll-for-my-org", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr, dbVerifier := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{},
		)
		require.NoError(t, err)

		myImplicitColl := implicitcollection.NameForOrg(myMSPID)
		err = snapshotDataImporter.ConsumeSnapshotData("ns", myImplicitColl,
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.NoError(t, err)
		require.NoError(t, snapshotDataImporter.Done())

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: myImplicitColl},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyElgMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: myImplicitColl, blkNum: 20}},
			(&bitset.BitSet{}).Set(300),
		)

		dbVerifier.verifyNoExpiryEntries()
	})

	t.Run("implicit-coll-not-for-my-org", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr, dbVerifier := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{},
		)
		require.NoError(t, err)

		otherOrgImplicitColl := implicitcollection.NameForOrg("SomeOtherOrg")
		err = snapshotDataImporter.ConsumeSnapshotData("ns", otherOrgImplicitColl,
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.NoError(t, err)
		require.NoError(t, snapshotDataImporter.Done())

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: otherOrgImplicitColl},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyInelgMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: otherOrgImplicitColl, blkNum: 20}},
			(&bitset.BitSet{}).Set(300),
		)

		dbVerifier.verifyNoExpiryEntries()
	})

	t.Run("write-data-for-more-than-one-block-having-more-than-one-tx", func(t *testing.T) {
		originalBatchLenForSnapshotImport := maxBatchLenForSnapshotImport
		maxBatchLenForSnapshotImport = 4
		defer func() {
			maxBatchLenForSnapshotImport = originalBatchLenForSnapshotImport
		}()

		snapshotDataImporter, configHistoryMgr, dbVerifier := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{},
		)
		require.NoError(t, err)

		myImplicitColl := implicitcollection.NameForOrg(myMSPID)
		for i := 10; i < 20; i++ {
			err = snapshotDataImporter.ConsumeSnapshotData("ns", myImplicitColl,
				[]byte("key-hash"), []byte("value-hash"),
				version.NewHeight(uint64(i), 300),
			)
			require.NoError(t, err)

			err = snapshotDataImporter.ConsumeSnapshotData("ns", myImplicitColl,
				[]byte("another-key-hash"), []byte("another-value-hash"),
				version.NewHeight(uint64(i), 301),
			)
			require.NoError(t, err)

			err = snapshotDataImporter.ConsumeSnapshotData("ns", myImplicitColl,
				[]byte("another-key-hash"), []byte("another-value-hash"),
				version.NewHeight(uint64(i), 302),
			)
			require.NoError(t, err)

		}
		require.NoError(t, snapshotDataImporter.Done())

		for i := 10; i < 20; i++ {
			dbVerifier.verifyBootKVHashesEntry(
				&bootKVHashesKey{blkNum: uint64(i), txNum: 300, ns: "ns", coll: myImplicitColl},
				&BootKVHashes{
					List: []*BootKVHash{
						{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
					},
				},
			)
			dbVerifier.verifyBootKVHashesEntry(
				&bootKVHashesKey{blkNum: uint64(i), txNum: 301, ns: "ns", coll: myImplicitColl},
				&BootKVHashes{
					List: []*BootKVHash{
						{KeyHash: []byte("another-key-hash"), ValueHash: []byte("another-value-hash")},
					},
				},
			)
			dbVerifier.verifyBootKVHashesEntry(
				&bootKVHashesKey{blkNum: uint64(i), txNum: 302, ns: "ns", coll: myImplicitColl},
				&BootKVHashes{
					List: []*BootKVHash{
						{KeyHash: []byte("another-key-hash"), ValueHash: []byte("another-value-hash")},
					},
				},
			)
			dbVerifier.verifyElgMissingDataEntry(
				&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: myImplicitColl, blkNum: uint64(i)}},
				(&bitset.BitSet{}).Set(300).Set(301).Set(302),
			)
		}
		dbVerifier.verifyNoExpiryEntries()
	})

	t.Run("loads-collection-config-into-cache-only-once", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr, _ := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{
				15: {
					{
						Name:             "coll",
						MemberOrgsPolicy: iamOut.toMemberOrgPolicy(),
						BlockToLive:      30,
					},
				},
			},
		)
		require.NoError(t, err)

		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.NoError(t, err)
		snapshotDataImporter.namespacesVisited["ns"] = struct{}{}
		err = snapshotDataImporter.Done()
		require.EqualError(t, err, "unexpected error - no collection config history for <namespace=ns, collection=coll>")
	})
}

func TestSnapshotImporterErrorPropagation(t *testing.T) {
	ledgerID := "test-ledger"
	myMSPID := "myOrg"

	setup := func() (*SnapshotDataImporter, *confighistorytest.Mgr) {
		testDir := t.TempDir()
		dbProvider, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: testDir})
		require.NoError(t, err)
		t.Cleanup(func() { dbProvider.Close() })

		configHistoryMgr, err := confighistorytest.NewMgr(path.Join(testDir, "config-history"))
		require.NoError(t, err)
		t.Cleanup(func() { configHistoryMgr.Close() })

		db := dbProvider.GetDBHandle(ledgerID)

		snapshotDataImporter, err := newSnapshotDataImporter(
			ledgerID,
			db,
			newMockMembershipProvider(myMSPID),
			configHistoryMgr.GetRetriever(ledgerID),
			testDir,
		)
		require.NoError(t, err)
		return snapshotDataImporter, configHistoryMgr
	}

	t.Run("expect-error-when-no-coll-config-not-present-for-namespace", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{},
		)
		require.NoError(t, err)

		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.NoError(t, err)

		err = snapshotDataImporter.Done()
		require.EqualError(t, err, "unexpected error - no collection config history for <namespace=ns, collection=coll>")
	})

	t.Run("expect-error-when-coll-config-below-data-item-not-present", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{
				15: {
					{
						Name:             "coll",
						MemberOrgsPolicy: iamOut.toMemberOrgPolicy(),
					},
				},
			},
		)
		require.NoError(t, err)

		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(10, 300),
		)
		require.NoError(t, err)

		err = snapshotDataImporter.Done()
		require.EqualError(t, err, "unexpected error - no collection config found below block number [10] for <namespace=ns, collection=coll>")
	})

	t.Run("error-when-membershipProvider-returns-error", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{
				15: {
					{
						Name:             "coll",
						MemberOrgsPolicy: iamIn.toMemberOrgPolicy(),
						BlockToLive:      30,
					},
				},
			},
		)
		require.NoError(t, err)

		snapshotDataImporter.eligibilityAndBTLCache.membershipProvider.(*mock.MembershipInfoProvider).AmMemberOfReturns(false, fmt.Errorf("membership-error"))
		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.NoError(t, err)

		err = snapshotDataImporter.Done()
		require.EqualError(t, err, "membership-error")
	})

	t.Run("error-when-writing-pending-data-during-done", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{
				15: {
					{
						Name:             "coll",
						MemberOrgsPolicy: iamIn.toMemberOrgPolicy(),
						BlockToLive:      30,
					},
				},
			},
		)
		require.NoError(t, err)
		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.NoError(t, err)

		snapshotDataImporter.rowsSorter.dbProvider.Close()
		err = snapshotDataImporter.Done()
		require.EqualError(t, err, "error writing batch to leveldb: leveldb: closed")
	})

	t.Run("error-when-retrieving-iterator-during-done", func(t *testing.T) {
		snapshotDataImporter, configHistoryMgr := setup()
		err := configHistoryMgr.Setup(
			ledgerID, "ns",
			map[uint64][]*peer.StaticCollectionConfig{
				15: {
					{
						Name:             "coll",
						MemberOrgsPolicy: iamIn.toMemberOrgPolicy(),
						BlockToLive:      30,
					},
				},
			},
		)
		require.NoError(t, err)

		// set smaller batch size for testing
		originalSnapshotRowSortBatchSize := maxSnapshotRowSortBatchSize
		originalBatchLenForSnapshotImport := maxBatchLenForSnapshotImport
		maxSnapshotRowSortBatchSize = 1
		maxBatchLenForSnapshotImport = 1
		defer func() {
			maxSnapshotRowSortBatchSize = originalSnapshotRowSortBatchSize
			maxBatchLenForSnapshotImport = originalBatchLenForSnapshotImport
		}()

		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.NoError(t, err)

		snapshotDataImporter.rowsSorter.dbProvider.Close()
		err = snapshotDataImporter.Done()
		require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")
	})
}

func TestEligibilityAndBTLCacheLoadData(t *testing.T) {
	testDir := t.TempDir()

	configHistoryMgr, err := confighistorytest.NewMgr(testDir)
	require.NoError(t, err)
	defer configHistoryMgr.Close()

	// setup a sample config history for namespace1
	err = configHistoryMgr.Setup("test-ledger", "namespace1",
		map[uint64][]*peer.StaticCollectionConfig{
			15: {
				{
					Name:             "coll1",
					MemberOrgsPolicy: iamIn.toMemberOrgPolicy(),
					BlockToLive:      0,
				},
				{
					Name:             "coll2",
					MemberOrgsPolicy: iamOut.toMemberOrgPolicy(),
					BlockToLive:      100,
				},
				{
					Name:             "coll3",
					MemberOrgsPolicy: iamIn.toMemberOrgPolicy(),
					BlockToLive:      500,
				},
			},

			10: {
				{
					Name:             "coll1",
					MemberOrgsPolicy: iamOut.toMemberOrgPolicy(),
				},
				{
					Name:             "coll2",
					MemberOrgsPolicy: iamIn.toMemberOrgPolicy(),
				},
			},

			5: {
				{
					Name:             "coll1",
					MemberOrgsPolicy: iamIn.toMemberOrgPolicy(),
				},
			},
		},
	)
	require.NoError(t, err)

	// setup a sample config history for namespace2
	err = configHistoryMgr.Setup("test-ledger", "namespace2",
		map[uint64][]*peer.StaticCollectionConfig{
			50: {
				{
					Name:             "coll1",
					MemberOrgsPolicy: iamIn.toMemberOrgPolicy(),
					BlockToLive:      300,
				},
			},
		},
	)
	require.NoError(t, err)

	eligibilityAndBTLCache := eligibilityAndBTLCache{
		membershipProvider:     newMockMembershipProvider("myOrg"),
		configHistoryRetriever: configHistoryMgr.GetRetriever("test-ledger"),
		eligibilityHistory:     map[nsColl][]*eligibility{},
		btl:                    map[nsColl]uint64{},
	}

	err = eligibilityAndBTLCache.loadDataFor("namespace1")
	require.NoError(t, err)
	err = eligibilityAndBTLCache.loadDataFor("namespace2")
	require.NoError(t, err)

	require.Equal(
		t,
		map[nsColl]uint64{
			{ns: "namespace1", coll: "coll2"}: 100,
			{"namespace1", "coll3"}:           500,
			{"namespace2", "coll1"}:           300,
		},
		eligibilityAndBTLCache.btl,
	)

	require.Equal(
		t,
		map[nsColl][]*eligibility{
			{ns: "namespace1", coll: "coll1"}: {{configBlockNum: 15, isEligible: true}, {10, false}, {5, true}},
			{"namespace1", "coll2"}:           {{15, false}, {10, true}},
			{"namespace1", "coll3"}:           {{15, true}},
			{"namespace2", "coll1"}:           {{50, true}},
		},
		eligibilityAndBTLCache.eligibilityHistory,
	)
}

func TestEligibilityAndBTLCacheEligibleExplicitCollection(t *testing.T) {
	eligibilityAndBTLCache := eligibilityAndBTLCache{
		membershipProvider: newMockMembershipProvider("myOrg"),
	}

	t.Run("ineligible-then-eligible", func(t *testing.T) {
		eligibilityAndBTLCache.eligibilityHistory = map[nsColl][]*eligibility{
			{ns: "ns", coll: "coll"}: {
				{configBlockNum: 10, isEligible: true},
				{configBlockNum: 5, isEligible: false},
			},
		}

		for i := 6; i < 15; i++ {
			eligible, err := eligibilityAndBTLCache.isEligibile("ns", "coll", uint64(i))
			require.NoError(t, err)
			require.True(t, eligible)
		}
	})

	t.Run("ineligible-eligible-again-ineligible", func(t *testing.T) {
		eligibilityAndBTLCache.eligibilityHistory = map[nsColl][]*eligibility{
			{ns: "ns", coll: "coll"}: {
				{configBlockNum: 20, isEligible: false},
				{configBlockNum: 10, isEligible: true},
				{configBlockNum: 5, isEligible: false},
			},
		}

		for i := 6; i <= 20; i++ {
			eligible, err := eligibilityAndBTLCache.isEligibile("ns", "coll", uint64(i))
			require.NoError(t, err)
			require.True(t, eligible)
		}
		for i := 21; i < 25; i++ {
			eligible, err := eligibilityAndBTLCache.isEligibile("ns", "coll", uint64(i))
			require.NoError(t, err)
			require.False(t, eligible)
		}
	})

	t.Run("eligible-then-ineligible", func(t *testing.T) {
		eligibilityAndBTLCache.eligibilityHistory = map[nsColl][]*eligibility{
			{ns: "ns", coll: "coll"}: {
				{configBlockNum: 10, isEligible: false},
				{configBlockNum: 5, isEligible: true},
			},
		}

		for i := 6; i <= 10; i++ {
			eligible, err := eligibilityAndBTLCache.isEligibile("ns", "coll", uint64(i))
			require.NoError(t, err)
			require.True(t, eligible)
		}

		for i := 11; i < 15; i++ {
			eligible, err := eligibilityAndBTLCache.isEligibile("ns", "coll", uint64(i))
			require.NoError(t, err)
			require.False(t, eligible)
		}
	})

	t.Run("eligible-ineligible-again-eligible", func(t *testing.T) {
		eligibilityAndBTLCache.eligibilityHistory = map[nsColl][]*eligibility{
			{ns: "ns", coll: "coll"}: {
				{configBlockNum: 20, isEligible: true},
				{configBlockNum: 10, isEligible: false},
				{configBlockNum: 5, isEligible: true},
			},
		}

		for i := 6; i < 25; i++ {
			eligible, err := eligibilityAndBTLCache.isEligibile("ns", "coll", uint64(i))
			require.NoError(t, err)
			require.True(t, eligible)
		}
	})
}

func TestEligibilityAndBTLCacheDataExpiry(t *testing.T) {
	t.Run("data-expires", func(t *testing.T) {
		eligibilityAndBTLCache := eligibilityAndBTLCache{
			btl: map[nsColl]uint64{
				{ns: "ns", coll: "coll"}: 25,
			},
		}
		expires, expiringBlk := eligibilityAndBTLCache.hasExpiry("ns", "coll", 100)
		require.True(t, expires)
		require.Equal(t, uint64(100+25+1), expiringBlk)
	})

	t.Run("data-does-not-expires", func(t *testing.T) {
		eligibilityAndBTLCache := eligibilityAndBTLCache{}
		expires, expiringBlk := eligibilityAndBTLCache.hasExpiry("ns", "coll", 100)
		require.False(t, expires)
		require.Equal(t, uint64(math.MaxUint64), expiringBlk)
	})
}

func newMockMembershipProvider(myMspID string) *mock.MembershipInfoProvider {
	p := &mock.MembershipInfoProvider{}
	p.MyImplicitCollectionNameReturns(implicitcollection.NameForOrg("myOrg"))
	p.AmMemberOfStub = func(namespace string, config *peer.CollectionPolicyConfig) (bool, error) {
		return iamIn.sameAs(config), nil
	}
	return p
}

type eligibilityVal uint8

const (
	iamIn eligibilityVal = iota
	iamOut
)

func (e eligibilityVal) toMemberOrgPolicy() *peer.CollectionPolicyConfig {
	return &peer.CollectionPolicyConfig{
		Payload: &peer.CollectionPolicyConfig_SignaturePolicy{
			SignaturePolicy: &common.SignaturePolicyEnvelope{
				Identities: []*msp.MSPPrincipal{
					{
						Principal: []byte{byte(e)},
					},
				},
			},
		},
	}
}

func (e eligibilityVal) sameAs(p *peer.CollectionPolicyConfig) bool {
	return e == eligibilityVal(p.GetSignaturePolicy().Identities[0].Principal[0])
}

func TestDBUpdates(t *testing.T) {
	setup := func() *leveldbhelper.Provider {
		testDir := t.TempDir()

		p, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: testDir})
		require.NoError(t, err)
		t.Cleanup(func() { p.Close() })
		return p
	}

	t.Run("upsert-missing-data-entry-eligible-data", func(t *testing.T) {
		provider := setup()
		db := provider.GetDBHandle("test-db")
		verifier := &dbEntriesVerifier{t, db}

		dbUpdates := newDBUpdates()
		dbUpdates.upsertElgMissingDataEntry("ns-1", "coll-1", 1, 10)
		dbUpdates.upsertElgMissingDataEntry("ns-1", "coll-1", 1, 50)
		require.NoError(t, dbUpdates.commitToDB(db))
		verifier.verifyElgMissingDataEntry(
			&missingDataKey{
				nsCollBlk{"ns-1", "coll-1", 1},
			},
			(&bitset.BitSet{}).Set(10).Set(50),
		)
	})

	t.Run("upsert-missing-data-entry-ineligible-data", func(t *testing.T) {
		provider := setup()
		db := provider.GetDBHandle("test-db")
		verifier := &dbEntriesVerifier{t, db}

		dbUpdates := newDBUpdates()
		dbUpdates.upsertInelgMissingDataEntry("ns-1", "coll-1", 1, 10)
		dbUpdates.upsertInelgMissingDataEntry("ns-1", "coll-1", 1, 50)
		require.NoError(t, dbUpdates.commitToDB(db))
		verifier.verifyInelgMissingDataEntry(
			&missingDataKey{
				nsCollBlk{"ns-1", "coll-1", 1},
			},
			(&bitset.BitSet{}).Set(10).Set(50),
		)
	})

	t.Run("upsert-bootKVHashes", func(t *testing.T) {
		provider := setup()
		db := provider.GetDBHandle("test-db")
		verifier := &dbEntriesVerifier{t, db}

		dbUpdates := newDBUpdates()
		dbUpdates.upsertBootKVHashes("ns-1", "coll-1", 1, 2, []byte("key-hash"), []byte("value-hash"))
		dbUpdates.upsertBootKVHashes("ns-1", "coll-1", 1, 2, []byte("another-key-hash"), []byte("another-value-hash"))
		require.NoError(t, dbUpdates.commitToDB(db))
		verifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{
				blkNum: 1,
				txNum:  2,
				ns:     "ns-1",
				coll:   "coll-1",
			},
			&BootKVHashes{
				List: []*BootKVHash{
					{
						KeyHash:   []byte("key-hash"),
						ValueHash: []byte("value-hash"),
					},
					{
						KeyHash:   []byte("another-key-hash"),
						ValueHash: []byte("another-value-hash"),
					},
				},
			},
		)
	})

	t.Run("upsert-expiryEntry", func(t *testing.T) {
		provider := setup()
		db := provider.GetDBHandle("test-db")
		verifier := &dbEntriesVerifier{t, db}

		dbUpdates := newDBUpdates()
		dbUpdates.upsertExpiryEntry(5, 2, "ns-1", "coll-1", 10)
		dbUpdates.upsertExpiryEntry(5, 2, "ns-1", "coll-1", 11)
		dbUpdates.upsertExpiryEntry(5, 2, "ns-1", "coll-2", 12)
		require.NoError(t, dbUpdates.commitToDB(db))

		verifier.verifyExpiryEntry(
			&expiryKey{
				committingBlk: 2,
				expiringBlk:   5,
			},
			&ExpiryData{
				Map: map[string]*NamespaceExpiryData{
					"ns-1": {
						MissingData: map[string]bool{
							"coll-1": true,
							"coll-2": true,
						},
						BootKVHashes: map[string]*TxNums{
							"coll-1": {List: []uint64{10, 11}},
							"coll-2": {List: []uint64{12}},
						},
					},
				},
			},
		)
	})

	t.Run("error-propagation", func(t *testing.T) {
		dbProvider := setup()
		db := dbProvider.GetDBHandle("test-db")
		dbUpdates := newDBUpdates()
		dbUpdates.elgMissingDataEntries[missingDataKey{}] = &bitset.BitSet{}
		dbProvider.Close()
		err := dbUpdates.commitToDB(db)
		require.Contains(t, err.Error(), "leveldb: closed")
	})
}

type dbEntriesVerifier struct {
	t  *testing.T
	db *leveldbhelper.DBHandle
}

func (v *dbEntriesVerifier) verifyElgMissingDataEntry(key *missingDataKey, expectedVal *bitset.BitSet) {
	valEnc, err := v.db.Get(encodeElgPrioMissingDataKey(key))
	require.NoError(v.t, err)
	val, err := decodeMissingDataValue(valEnc)
	require.NoError(v.t, err)
	require.Equal(v.t, expectedVal, val)
}

func (v *dbEntriesVerifier) verifyInelgMissingDataEntry(key *missingDataKey, expectedVal *bitset.BitSet) {
	valEnc, err := v.db.Get(encodeInelgMissingDataKey(key))
	require.NoError(v.t, err)
	val, err := decodeMissingDataValue(valEnc)
	require.NoError(v.t, err)
	require.Equal(v.t, expectedVal, val)
}

func (v *dbEntriesVerifier) verifyBootKVHashesEntry(key *bootKVHashesKey, expectedVal *BootKVHashes) {
	encVal, err := v.db.Get(encodeBootKVHashesKey(key))
	require.NoError(v.t, err)
	val, err := decodeBootKVHashesVal(encVal)
	require.NoError(v.t, err)
	require.Equal(v.t, expectedVal, val)
}

func (v *dbEntriesVerifier) verifyExpiryEntry(key *expiryKey, expectedVal *ExpiryData) {
	encVal, err := v.db.Get(encodeExpiryKey(key))
	require.NoError(v.t, err)
	val, err := decodeExpiryValue(encVal)
	require.NoError(v.t, err)
	require.Equal(v.t, expectedVal, val)
}

func (v *dbEntriesVerifier) verifyNoExpiryEntries() {
	iter, err := v.db.GetIterator(expiryKeyPrefix, append(expiryKeyPrefix, byte(0)))
	defer iter.Release()

	require.NoError(v.t, err)
	require.False(v.t, iter.Next())
	require.NoError(v.t, iter.Error())
}

func TestSnapshotRowsSorter(t *testing.T) {
	testCases := []struct {
		inputRows         []*snapshotRow
		expectedSortOrder []int
	}{
		{
			inputRows: []*snapshotRow{
				{
					version:   version.NewHeight(100, 100),
					ns:        "ns-1",
					coll:      "coll-1",
					keyHash:   []byte("key-hash"),
					valueHash: []byte("value-hash"),
				},

				{
					version:   version.NewHeight(0, 0),
					ns:        "ns-2",
					coll:      "coll-2",
					keyHash:   []byte("key-hash"),
					valueHash: []byte("value-hash"),
				},
			},
			expectedSortOrder: []int{1, 0},
		},

		{
			inputRows: []*snapshotRow{
				{
					version:   version.NewHeight(100, 100),
					ns:        "ns-2",
					coll:      "coll-1",
					keyHash:   []byte("key-hash"),
					valueHash: []byte("value-hash"),
				},

				{
					version:   version.NewHeight(100, 100),
					ns:        "ns-1",
					coll:      "coll-1",
					keyHash:   []byte("key-hash"),
					valueHash: []byte("value-hash"),
				},
			},
			expectedSortOrder: []int{1, 0},
		},

		{
			inputRows: []*snapshotRow{
				{
					version:   version.NewHeight(100, 100),
					ns:        "ns-1",
					coll:      "coll-2",
					keyHash:   []byte("key-hash"),
					valueHash: []byte("value-hash"),
				},

				{
					version:   version.NewHeight(100, 100),
					ns:        "ns-1",
					coll:      "coll-1",
					keyHash:   []byte("key-hash"),
					valueHash: []byte("value-hash"),
				},
			},
			expectedSortOrder: []int{1, 0},
		},

		{
			inputRows: []*snapshotRow{
				{
					version:   version.NewHeight(100, 100),
					ns:        "ns-1",
					coll:      "coll-1",
					keyHash:   []byte("key-hash-2"),
					valueHash: []byte("value-hash-1"),
				},

				{
					version:   version.NewHeight(100, 100),
					ns:        "ns-1",
					coll:      "coll-1",
					keyHash:   []byte("key-hash-1"),
					valueHash: []byte("value-hash-2"),
				},
			},
			expectedSortOrder: []int{1, 0},
		},

		{
			inputRows: []*snapshotRow{
				{
					version:   version.NewHeight(100, 100),
					ns:        "ns-1",
					coll:      "coll-1",
					keyHash:   []byte("key-hash-1"),
					valueHash: []byte("value-hash-2"),
				},

				{
					version:   version.NewHeight(100, 100),
					ns:        "ns-1",
					coll:      "coll-1",
					keyHash:   []byte("key-hash-2"),
					valueHash: []byte("value-hash-1"),
				},
			},
			expectedSortOrder: []int{0, 1},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testcase-%d", i), func(t *testing.T) {
			dir := t.TempDir()

			sorter, err := newSnapshotRowsSorter(dir)
			require.NoError(t, err)

			for _, row := range testCase.inputRows {
				require.NoError(t, sorter.add(row))
			}

			require.NoError(t, sorter.addingDone())
			iter, err := sorter.iterator()
			require.NoError(t, err)
			defer iter.close()

			results := []*snapshotRow{}
			for {
				row, err := iter.next()
				require.NoError(t, err)
				if row == nil {
					break
				}
				results = append(results, row)
			}
			require.Len(t, results, len(testCase.inputRows))

			for i := 0; i < len(results); i++ {
				require.Equal(
					t,
					testCase.inputRows[testCase.expectedSortOrder[i]],
					results[i],
				)
			}
		})
	}
}

func TestSnapshotRowsSorterCleanup(t *testing.T) {
	dir := t.TempDir()

	sorter, err := newSnapshotRowsSorter(dir)
	require.NoError(t, err)
	empty, err := fileutil.DirEmpty(dir)
	require.NoError(t, err)
	require.False(t, empty)

	sorter.cleanup()
	empty, err = fileutil.DirEmpty(dir)
	require.NoError(t, err)
	require.True(t, empty)
}

func TestSnapshotRowsSortEncodingDecoding(t *testing.T) {
	rows := []*snapshotRow{
		{
			version:   version.NewHeight(100, 100),
			ns:        "ns",
			coll:      "coll",
			keyHash:   []byte("key-hash"),
			valueHash: []byte("value-hash"),
		},

		{
			version:   version.NewHeight(0, 0),
			ns:        "",
			coll:      "",
			keyHash:   []byte("key-hash"),
			valueHash: []byte("value-hash"),
		},
	}

	for _, row := range rows {
		encSortKey := encodeSnapshotRowForSorting(row)
		decSortKey, err := decodeSnapshotRowFromSortEncoding(encSortKey)
		require.NoError(t, err)
		require.Equal(t, row, decSortKey)
	}

	t.Run("error_propagation_during_decoding", func(t *testing.T) {
		_, err := decodeSnapshotRowFromSortEncoding([]byte("random-string"))
		require.EqualError(t, err, "decoded size from DecodeVarint is invalid, expected <=8, but got 114")
	})
}
