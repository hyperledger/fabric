/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lscc

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/pkg/errors"
)

const (
	lsccNamespace = "lscc"
)

// DeployedCCInfoProvider implements ineterface ledger.DeployedChaincodeInfoProvider
type DeployedCCInfoProvider struct {
}

// Namespaces implements function in interface ledger.DeployedChaincodeInfoProvider
func (p *DeployedCCInfoProvider) Namespaces() []string {
	return []string{lsccNamespace}
}

// UpdatedChaincodes implements function in interface ledger.DeployedChaincodeInfoProvider
func (p *DeployedCCInfoProvider) UpdatedChaincodes(stateUpdates map[string][]*kvrwset.KVWrite) ([]*ledger.ChaincodeLifecycleInfo, error) {
	lsccUpdates := stateUpdates[lsccNamespace]
	lifecycleInfo := []*ledger.ChaincodeLifecycleInfo{}
	updatedCCNames := map[string]bool{}

	for _, kvWrite := range lsccUpdates {
		if kvWrite.IsDelete {
			// lscc namespace is not expected to have deletes
			continue
		}
		// There are LSCC entries for the chaincode and for the chaincode collections.
		// We can detect collections based on the presence of a CollectionSeparator,
		// which never exists in chaincode names.
		if privdata.IsCollectionConfigKey(kvWrite.Key) {
			ccname := privdata.GetCCNameFromCollectionConfigKey(kvWrite.Key)
			updatedCCNames[ccname] = true
			continue
		}
		updatedCCNames[kvWrite.Key] = true
	}

	for updatedCCNames := range updatedCCNames {
		lifecycleInfo = append(lifecycleInfo, &ledger.ChaincodeLifecycleInfo{Name: updatedCCNames})
	}
	return lifecycleInfo, nil
}

// ChaincodeInfo implements function in interface ledger.DeployedChaincodeInfoProvider
func (p *DeployedCCInfoProvider) ChaincodeInfo(chaincodeName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error) {
	chaincodeDataBytes, err := qe.GetState(lsccNamespace, chaincodeName)
	if err != nil || chaincodeDataBytes == nil {
		return nil, err
	}
	chaincodeData := &ccprovider.ChaincodeData{}
	if err := proto.Unmarshal(chaincodeDataBytes, chaincodeData); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling chaincode state data")
	}
	collConfigPkg, err := fetchCollConfigPkg(chaincodeName, qe)
	if err != nil {
		return nil, err
	}
	return &ledger.DeployedChaincodeInfo{
		Name:                chaincodeName,
		Hash:                chaincodeData.Id,
		Version:             chaincodeData.Version,
		CollectionConfigPkg: collConfigPkg,
	}, nil
}

// CollectionInfo implements function in interface ledger.DeployedChaincodeInfoProvider
func (p *DeployedCCInfoProvider) CollectionInfo(chaincodeName, collectionName string, qe ledger.SimpleQueryExecutor) (*common.StaticCollectionConfig, error) {
	collConfigPkg, err := fetchCollConfigPkg(chaincodeName, qe)
	if err != nil || collConfigPkg == nil {
		return nil, err
	}
	for _, conf := range collConfigPkg.Config {
		staticCollConfig := conf.GetStaticCollectionConfig()
		if staticCollConfig != nil && staticCollConfig.Name == collectionName {
			return staticCollConfig, nil
		}
	}
	return nil, nil
}

func fetchCollConfigPkg(chaincodeName string, qe ledger.SimpleQueryExecutor) (*common.CollectionConfigPackage, error) {
	collKey := privdata.BuildCollectionKVSKey(chaincodeName)
	collectionConfigPkgBytes, err := qe.GetState(lsccNamespace, collKey)
	if err != nil || collectionConfigPkgBytes == nil {
		return nil, err
	}
	collectionConfigPkg := &common.CollectionConfigPackage{}
	if err := proto.Unmarshal(collectionConfigPkgBytes, collectionConfigPkg); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling chaincode collection config pkg")
	}
	return collectionConfigPkg, nil
}
