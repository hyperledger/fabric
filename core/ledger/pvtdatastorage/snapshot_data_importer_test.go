/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/chaincode/implicitcollection"
	"github.com/hyperledger/fabric/core/ledger/confighistory/confighistorytest"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
	"github.com/willf/bitset"
)

func TestSnapshotImporter(t *testing.T) {
	ledgerID := "test-ledger"
	myMSPID := "myOrg"

	setup := func() (*SnapshotDataImporter, *confighistorytest.Mgr, *dbEntriesVerifier) {
		testDir := testDir(t)
		t.Cleanup(func() { os.RemoveAll(testDir) })
		dbProvider, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: testDir})
		require.NoError(t, err)
		t.Cleanup(func() { dbProvider.Close() })

		configHistoryMgr, err := confighistorytest.NewMgr(path.Join(testDir, "config-history"))
		require.NoError(t, err)
		t.Cleanup(func() { configHistoryMgr.Close() })

		db := dbProvider.GetDBHandle(ledgerID)

		return newSnapshotDataImporter(
				ledgerID,
				db,
				newMockMembershipProvider(myMSPID),
				configHistoryMgr.GetRetriever(ledgerID),
			),
			configHistoryMgr,
			&dbEntriesVerifier{t, db}
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

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: "coll"},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: 20}},
			true,
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

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: "coll"},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: 20}},
			false,
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

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: "coll"},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: 20}},
			true,
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

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: "coll"},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: 20}},
			false,
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

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: myImplicitColl},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: myImplicitColl, blkNum: 20}},
			true,
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

		dbVerifier.verifyBootKVHashesEntry(
			&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: otherOrgImplicitColl},
			&BootKVHashes{
				List: []*BootKVHash{
					{KeyHash: []byte("key-hash"), ValueHash: []byte("value-hash")},
				},
			},
		)

		dbVerifier.verifyMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{ns: "ns", coll: otherOrgImplicitColl, blkNum: 20}},
			false,
			(&bitset.BitSet{}).Set(300),
		)

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

		_, ok := snapshotDataImporter.namespacesVisited["ns"]
		require.True(t, ok)

		snapshotDataImporter.eligibilityAndBTLCache.eligibilityHistory[nsColl{"ns", "coll"}] = nil
		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash-1"), []byte("value-hash-1"),
			version.NewHeight(21, 300),
		)
		require.EqualError(t, err, "unexpected error - no collection config history for <namespace=ns, collection=coll>")
	})
}

func TestSnapshotImporterErrorPropagation(t *testing.T) {
	ledgerID := "test-ledger"
	myMSPID := "myOrg"

	setup := func() (*SnapshotDataImporter, *confighistorytest.Mgr) {
		testDir := testDir(t)
		t.Cleanup(func() { os.RemoveAll(testDir) })
		dbProvider, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: testDir})
		require.NoError(t, err)
		t.Cleanup(func() { dbProvider.Close() })

		configHistoryMgr, err := confighistorytest.NewMgr(path.Join(testDir, "config-history"))
		require.NoError(t, err)
		t.Cleanup(func() { configHistoryMgr.Close() })

		db := dbProvider.GetDBHandle(ledgerID)

		return newSnapshotDataImporter(
				ledgerID,
				db,
				newMockMembershipProvider(myMSPID),
				configHistoryMgr.GetRetriever(ledgerID),
			),
			configHistoryMgr
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
		require.EqualError(t, err, "unexpected error - no collection config found below block number [10] for <namespace=ns, collection=coll>")
	})

	t.Run("error-when-membershipProvider-returns error", func(t *testing.T) {
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
		require.EqualError(t, err, "membership-error")
	})

	t.Run("error-during-decoding-existing-missing-data-entry", func(t *testing.T) {
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
		err = snapshotDataImporter.db.Put(
			encodeElgPrioMissingDataKey(
				&missingDataKey{nsCollBlk: nsCollBlk{blkNum: 20, ns: "ns", coll: "coll"}},
			),
			[]byte("garbage-value"),
			false,
		)
		require.NoError(t, err)
		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.Contains(t, err.Error(), "error while decoding missing data value")
	})

	t.Run("error-during-decoding-existing-bootKVHashes", func(t *testing.T) {
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
		err = snapshotDataImporter.db.Put(
			encodeBootKVHashesKey(
				&bootKVHashesKey{blkNum: 20, txNum: 300, ns: "ns", coll: "coll"},
			),
			[]byte("garbage-value"),
			false,
		)
		require.NoError(t, err)
		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.Contains(t, err.Error(), "error while unmarshalling bytes for BootKVHashes")
	})

	t.Run("error-during-decoding-existing-expiryEntry", func(t *testing.T) {
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
		err = snapshotDataImporter.db.Put(
			encodeExpiryKey(
				&expiryKey{committingBlk: 20, expiringBlk: 51},
			),
			[]byte("garbage-value"),
			false,
		)
		require.NoError(t, err)
		err = snapshotDataImporter.ConsumeSnapshotData("ns", "coll",
			[]byte("key-hash"), []byte("value-hash"),
			version.NewHeight(20, 300),
		)
		require.Contains(t, err.Error(), "error while decoding expiry value")
	})
}

func TestEligibilityAndBTLCacheLoadData(t *testing.T) {
	testDir := testDir(t)
	defer os.RemoveAll(testDir)

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

func TestDBUpdator(t *testing.T) {
	setup := func() (*dbUpdater, *dbEntriesVerifier, *leveldbhelper.Provider) {
		testDir := testDir(t)
		t.Cleanup(func() { os.RemoveAll(testDir) })

		p, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: testDir})
		require.NoError(t, err)
		t.Cleanup(func() { p.Close() })
		db := p.GetDBHandle("test-ledger")
		batch := db.NewUpdateBatch()
		return &dbUpdater{
				db:    db,
				batch: batch,
			},
			&dbEntriesVerifier{
				t:  t,
				db: db,
			},
			p
	}

	t.Run("upsert-missing-data-entry-eligible-data", func(t *testing.T) {
		dbUpdater, verifier, _ := setup()
		missingDataKey := &missingDataKey{
			nsCollBlk{
				ns:     "ns-1",
				coll:   "coll-1",
				blkNum: 1,
			},
		}
		require.NoError(t,
			dbUpdater.upsertMissingDataEntry(missingDataKey, 10, true),
		)
		require.NoError(t,
			dbUpdater.commitBatch(),
		)
		verifier.verifyMissingDataEntry(missingDataKey, true, (&bitset.BitSet{}).Set(10))

		require.NoError(t,
			dbUpdater.upsertMissingDataEntry(missingDataKey, 50, true),
		)
		require.NoError(t,
			dbUpdater.commitBatch(),
		)

		verifier.verifyMissingDataEntry(missingDataKey, true, (&bitset.BitSet{}).Set(10).Set(50))
	})

	t.Run("upsert-missing-data-entry-ineligible-data", func(t *testing.T) {
		dbUpdater, verifier, _ := setup()
		missingDataKey := &missingDataKey{
			nsCollBlk{
				ns:     "ns-1",
				coll:   "coll-1",
				blkNum: 1,
			},
		}

		require.NoError(t,
			dbUpdater.upsertMissingDataEntry(missingDataKey, 10, false),
		)
		require.NoError(t,
			dbUpdater.commitBatch(),
		)

		verifier.verifyMissingDataEntry(missingDataKey, false, (&bitset.BitSet{}).Set(10))

		require.NoError(t,
			dbUpdater.upsertMissingDataEntry(missingDataKey, 50, false),
		)
		require.NoError(t,
			dbUpdater.commitBatch(),
		)

		verifier.verifyMissingDataEntry(missingDataKey, false, (&bitset.BitSet{}).Set(10).Set(50))
	})

	t.Run("upsert-bootKVHashes", func(t *testing.T) {
		bootKVHashesKey := &bootKVHashesKey{
			blkNum: 1,
			txNum:  2,
			ns:     "ns-1",
			coll:   "coll-1",
		}
		dbUpdater, verifier, _ := setup()
		require.NoError(t,
			dbUpdater.upsertBootKVHashes(bootKVHashesKey, []byte("key-hash"), []byte("value-hash")),
		)
		require.NoError(t,
			dbUpdater.commitBatch(),
		)

		verifier.verifyBootKVHashesEntry(bootKVHashesKey,
			&BootKVHashes{
				List: []*BootKVHash{
					{
						KeyHash:   []byte("key-hash"),
						ValueHash: []byte("value-hash"),
					},
				},
			})

		require.NoError(t,
			dbUpdater.upsertBootKVHashes(bootKVHashesKey, []byte("another-key-hash"), []byte("another-value-hash")),
		)
		require.NoError(t,
			dbUpdater.commitBatch(),
		)

		verifier.verifyBootKVHashesEntry(bootKVHashesKey,
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
			})
	})

	t.Run("upsert-expiryEntry", func(t *testing.T) {
		expiryKey := &expiryKey{
			committingBlk: 2,
			expiringBlk:   5,
		}
		dbUpdater, verifier, _ := setup()
		require.NoError(t,
			dbUpdater.upsertExpiryEntry(expiryKey, "ns-1", "coll-1", 10),
		)
		require.NoError(t,
			dbUpdater.commitBatch(),
		)

		verifier.verifyExpiryEntry(expiryKey,
			&ExpiryData{
				Map: map[string]*NamespaceExpiryData{
					"ns-1": {
						MissingData: map[string]bool{
							"coll-1": true,
						},
						BootKVHashes: map[string]*TxNums{
							"coll-1": {List: []uint64{10}},
						},
					},
				},
			},
		)

		require.NoError(t,
			dbUpdater.upsertExpiryEntry(expiryKey, "ns-1", "coll-1", 11),
		)
		require.NoError(t,
			dbUpdater.commitBatch(),
		)

		verifier.verifyExpiryEntry(expiryKey,
			&ExpiryData{
				Map: map[string]*NamespaceExpiryData{
					"ns-1": {
						MissingData: map[string]bool{
							"coll-1": true,
						},
						BootKVHashes: map[string]*TxNums{
							"coll-1": {List: []uint64{10, 11}},
						},
					},
				},
			},
		)

		require.NoError(t,
			dbUpdater.upsertExpiryEntry(expiryKey, "ns-1", "coll-2", 12),
		)
		require.NoError(t,
			dbUpdater.commitBatch(),
		)

		verifier.verifyExpiryEntry(expiryKey,
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
		dbUpdater, _, dbProvider := setup()
		dbProvider.Close()

		err := dbUpdater.upsertBootKVHashes(
			&bootKVHashesKey{blkNum: 20, txNum: 20, ns: "nil", coll: ""},
			nil,
			nil,
		)
		require.Contains(t, err.Error(), "leveldb: closed")

		err = dbUpdater.upsertMissingDataEntry(
			&missingDataKey{nsCollBlk: nsCollBlk{blkNum: 20, ns: "", coll: ""}},
			20,
			true,
		)
		require.Contains(t, err.Error(), "leveldb: closed")

		err = dbUpdater.upsertExpiryEntry(
			&expiryKey{committingBlk: 20, expiringBlk: 10},
			"",
			"",
			20,
		)
		require.Contains(t, err.Error(), "leveldb: closed")

		dbUpdater.batch.Put([]byte("key"), []byte("value"))
		err = dbUpdater.commitBatch()
		require.Contains(t, err.Error(), "leveldb: closed")
	})
}

type dbEntriesVerifier struct {
	t  *testing.T
	db *leveldbhelper.DBHandle
}

func (v *dbEntriesVerifier) verifyMissingDataEntry(key *missingDataKey, isEligibile bool, expectedVal *bitset.BitSet) {
	var k []byte
	if isEligibile {
		k = encodeElgPrioMissingDataKey(key)
	} else {
		k = encodeInelgMissingDataKey(key)
	}

	valEnc, err := v.db.Get(k)
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

func testDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "snapshot-data-importer-")
	require.NoError(t, err)
	return dir
}
