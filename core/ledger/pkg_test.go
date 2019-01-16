/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package ledger

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/stretchr/testify/assert"
)

func TestTxPvtData(t *testing.T) {
	txPvtData := &TxPvtData{}
	assert.False(t, txPvtData.Has("ns", "coll"))

	txPvtData.WriteSet = &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: "ns",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "coll-1",
						Rwset:          []byte("RandomBytes-PvtRWSet-ns1-coll1"),
					},
					{
						CollectionName: "coll-2",
						Rwset:          []byte("RandomBytes-PvtRWSet-ns1-coll2"),
					},
				},
			},
		},
	}

	assert.True(t, txPvtData.Has("ns", "coll-1"))
	assert.True(t, txPvtData.Has("ns", "coll-2"))
	assert.False(t, txPvtData.Has("ns", "coll-3"))
	assert.False(t, txPvtData.Has("ns1", "coll-1"))
}

func TestPvtNsCollFilter(t *testing.T) {
	filter := NewPvtNsCollFilter()
	filter.Add("ns", "coll-1")
	filter.Add("ns", "coll-2")
	assert.True(t, filter.Has("ns", "coll-1"))
	assert.True(t, filter.Has("ns", "coll-2"))
	assert.False(t, filter.Has("ns", "coll-3"))
	assert.False(t, filter.Has("ns1", "coll-3"))
}

func TestAllCollectionsConfigPkg(t *testing.T) {
	explicitCollsPkg := testutilCreateCollConfigPkg(
		[]string{"ExplicitColl-1", "ExplicitColl-2"},
	)
	implicitColls := []*common.StaticCollectionConfig{
		{
			Name: "ImplicitColl-1",
		},
		{
			Name: "ImplicitColl-2",
		},
	}
	implicitCollsPkg := testutilCreateCollConfigPkg(
		[]string{"ImplicitColl-1", "ImplicitColl-2"},
	)
	combinedCollsPkg := testutilCreateCollConfigPkg(
		[]string{"ExplicitColl-1", "ExplicitColl-2", "ImplicitColl-1", "ImplicitColl-2"},
	)

	t.Run("Merge-both-explicit-and-implicit",
		func(t *testing.T) {
			ccInfo := &DeployedChaincodeInfo{
				ExplicitCollectionConfigPkg: explicitCollsPkg,
				ImplicitCollections:         implicitColls,
			}
			combinedCollsPkgRetured := ccInfo.AllCollectionsConfigPkg()
			t.Logf("combinedCollsPkgRetured = %s", spew.Sdump(combinedCollsPkgRetured))
			assert.True(t, proto.Equal(combinedCollsPkgRetured, combinedCollsPkg))
		},
	)

	t.Run("Merge-only-implicit",
		func(t *testing.T) {
			ccInfo := &DeployedChaincodeInfo{
				ExplicitCollectionConfigPkg: nil,
				ImplicitCollections:         implicitColls,
			}
			combinedCollsPkgRetured := ccInfo.AllCollectionsConfigPkg()
			t.Logf("combinedCollsPkgRetured = %s", spew.Sdump(combinedCollsPkgRetured))
			assert.True(t, proto.Equal(combinedCollsPkgRetured, implicitCollsPkg))
		},
	)

	t.Run("both-implicit-and-explict-nil",
		func(t *testing.T) {
			ccInfo := &DeployedChaincodeInfo{
				ExplicitCollectionConfigPkg: nil,
				ImplicitCollections:         nil,
			}
			combinedCollsPkgRetured := ccInfo.AllCollectionsConfigPkg()
			t.Logf("combinedCollsPkgRetured = %s", spew.Sdump(combinedCollsPkgRetured))
			assert.Nil(t, combinedCollsPkgRetured)
		},
	)
}

func testutilCreateCollConfigPkg(collNames []string) *common.CollectionConfigPackage {
	pkg := &common.CollectionConfigPackage{
		Config: []*common.CollectionConfig{},
	}
	for _, collName := range collNames {
		pkg.Config = append(pkg.Config,
			&common.CollectionConfig{
				Payload: &common.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &common.StaticCollectionConfig{
						Name: collName,
					},
				},
			},
		)
	}
	return pkg
}
