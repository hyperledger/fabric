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

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// CollectionConfigRetriever is mock type for the CollectionConfigRetriever type
type mockCollectionConfigRetriever struct {
	mock.Mock
}

// GetState provides a mock function with given fields: namespace, key
func (_m *mockCollectionConfigRetriever) GetState(namespace string, key string) ([]byte, error) {
	result := _m.Called(namespace, key)
	return result.Get(0).([]byte), result.Error(1)
}

func TestAssemblePvtRWSet(t *testing.T) {
	collectionsConfigCC1 := &common.CollectionConfigPackage{
		Config: []*common.CollectionConfig{
			{
				Payload: &common.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &common.StaticCollectionConfig{
						Name: "mycollection-1",
					},
				},
			},
			{
				Payload: &common.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &common.StaticCollectionConfig{
						Name: "mycollection-2",
					},
				},
			},
		},
	}
	colB, err := proto.Marshal(collectionsConfigCC1)
	assert.NoError(t, err)

	configRetriever := &mockCollectionConfigRetriever{}
	configRetriever.On("GetState", "lscc", privdata.BuildCollectionKVSKey("myCC")).Return(colB, nil)

	assembler := rwSetAssembler{}

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

	pvtReadWriteSetWithConfigInfo, err := assembler.AssemblePvtRWSet(privData, configRetriever)
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
