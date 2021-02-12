/*
 *
 * Copyright IBM Corp. All Rights Reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 * /
 *
 */

package endorser

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

func TestAssemblePvtRWSet(t *testing.T) {
	collectionsConfigCC1 := &peer.CollectionConfigPackage{
		Config: []*peer.CollectionConfig{
			{
				Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &peer.StaticCollectionConfig{
						Name: "mycollection-1",
					},
				},
			},
			{
				Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &peer.StaticCollectionConfig{
						Name: "mycollection-2",
					},
				},
			},
		},
	}

	mockDeployedCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mockDeployedCCInfoProvider.ChaincodeInfoReturns(
		&ledger.DeployedChaincodeInfo{
			ExplicitCollectionConfigPkg: collectionsConfigCC1,
			Name:                        "myCC",
		},
		nil,
	)
	mockDeployedCCInfoProvider.AllCollectionsConfigPkgReturns(collectionsConfigCC1, nil)

	privData := &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: "myCC",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "mycollection-1",
						Rwset:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
			},
		},
	}

	pvtReadWriteSetWithConfigInfo, err := AssemblePvtRWSet("", privData, nil, mockDeployedCCInfoProvider)
	require.NoError(t, err)
	require.NotNil(t, pvtReadWriteSetWithConfigInfo)
	require.NotNil(t, pvtReadWriteSetWithConfigInfo.PvtRwset)
	configPackages := pvtReadWriteSetWithConfigInfo.CollectionConfigs
	require.NotNil(t, configPackages)
	configs, found := configPackages["myCC"]
	require.True(t, found)
	require.Equal(t, 1, len(configs.Config))
	require.NotNil(t, configs.Config[0])
	require.NotNil(t, configs.Config[0].GetStaticCollectionConfig())
	require.Equal(t, "mycollection-1", configs.Config[0].GetStaticCollectionConfig().Name)
	require.Equal(t, 1, len(pvtReadWriteSetWithConfigInfo.PvtRwset.NsPvtRwset))
}
