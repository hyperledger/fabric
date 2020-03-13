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
	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err)
	assert.NotNil(t, pvtReadWriteSetWithConfigInfo)
	assert.NotNil(t, pvtReadWriteSetWithConfigInfo.PvtRwset)
	configPackages := pvtReadWriteSetWithConfigInfo.CollectionConfigs
	assert.NotNil(t, configPackages)
	configs, found := configPackages["myCC"]
	assert.True(t, found)
	assert.Equal(t, 1, len(configs.Config))
	assert.NotNil(t, configs.Config[0])
	assert.NotNil(t, configs.Config[0].GetStaticCollectionConfig())
	assert.Equal(t, "mycollection-1", configs.Config[0].GetStaticCollectionConfig().Name)
	assert.Equal(t, 1, len(pvtReadWriteSetWithConfigInfo.PvtRwset.NsPvtRwset))

}
