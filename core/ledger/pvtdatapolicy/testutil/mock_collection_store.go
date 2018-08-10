/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testutil

import (
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

type MockCollectionStore struct {
	dummyData map[[2]string]uint64
}

func NewMockCollectionStore() *MockCollectionStore {
	return &MockCollectionStore{make(map[[2]string]uint64)}
}

func (m *MockCollectionStore) RetrieveCollection(common.CollectionCriteria) (privdata.Collection, error) {
	return nil, errors.New("not implemented")
}

func (m *MockCollectionStore) RetrieveCollectionAccessPolicy(common.CollectionCriteria) (privdata.CollectionAccessPolicy, error) {
	return nil, errors.New("not implemented")
}

func (m *MockCollectionStore) RetrieveCollectionConfigPackage(common.CollectionCriteria) (*common.CollectionConfigPackage, error) {
	return nil, errors.New("not implemented")
}

func (m *MockCollectionStore) RetrieveCollectionPersistenceConfigs(cc common.CollectionCriteria) (privdata.CollectionPersistenceConfigs, error) {
	btl, ok := m.dummyData[[2]string{cc.Namespace, cc.Collection}]
	type response struct {
		privdata.CollectionPersistenceConfigs
	}
	if ok {
		return &mockResponse{btl}, nil
	}
	return nil, privdata.NoSuchCollectionError{}
}

func (m *MockCollectionStore) AccessFilter(channelName string, collectionPolicyConfig *common.CollectionPolicyConfig) (privdata.Filter, error) {
	return nil, errors.New("not implemented")
}

func (m *MockCollectionStore) SetBTL(ns, collection string, btl uint64) {
	m.dummyData[[2]string{ns, collection}] = btl
}

type mockResponse struct {
	btl uint64
}

func (m *mockResponse) BlockToLive() uint64 {
	return m.btl
}
