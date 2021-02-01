/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lscc

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/pkg/errors"
)

const (
	lsccNamespace = "lscc"
)

// DeployedCCInfoProvider implements interface ledger.DeployedChaincodeInfoProvider
type DeployedCCInfoProvider struct{}

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
		if rwsetutil.IsKVWriteDelete(kvWrite) {
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

func (p *DeployedCCInfoProvider) ImplicitCollections(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) ([]*peer.StaticCollectionConfig, error) {
	return nil, nil
}

// GenerateImplicitCollectionForOrg is not implemented for legacy chaincodes
func (p *DeployedCCInfoProvider) GenerateImplicitCollectionForOrg(mspid string) *peer.StaticCollectionConfig {
	return nil
}

// ChaincodeInfo implements function in interface ledger.DeployedChaincodeInfoProvider
func (p *DeployedCCInfoProvider) ChaincodeInfo(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error) {
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
		Name:                        chaincodeName,
		Hash:                        chaincodeData.Id,
		Version:                     chaincodeData.Version,
		ExplicitCollectionConfigPkg: collConfigPkg,
		IsLegacy:                    true,
	}, nil
}

// AllChaincodesInfo returns the mapping of chaincode name to DeployedChaincodeInfo for legacy chaincodes
func (p *DeployedCCInfoProvider) AllChaincodesInfo(channelName string, qe ledger.SimpleQueryExecutor) (map[string]*ledger.DeployedChaincodeInfo, error) {
	iter, err := qe.GetStateRangeScanIterator(lsccNamespace, "", "")
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	result := make(map[string]*ledger.DeployedChaincodeInfo)
	for {
		entry, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if entry == nil {
			break
		}

		kv := entry.(*queryresult.KV)
		if !privdata.IsCollectionConfigKey(kv.Key) {
			deployedccInfo, err := p.ChaincodeInfo(channelName, kv.Key, qe)
			if err != nil {
				return nil, err
			}
			result[kv.Key] = deployedccInfo
		}
	}
	return result, nil
}

// AllCollectionsConfigPkg implements function in interface ledger.DeployedChaincodeInfoProvider
// this implementation returns just the explicit collection config package as the implicit collections
// are not used with legacy lifecycle
func (p *DeployedCCInfoProvider) AllCollectionsConfigPkg(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*peer.CollectionConfigPackage, error) {
	return fetchCollConfigPkg(chaincodeName, qe)
}

// CollectionInfo implements function in interface ledger.DeployedChaincodeInfoProvider
func (p *DeployedCCInfoProvider) CollectionInfo(channelName, chaincodeName, collectionName string, qe ledger.SimpleQueryExecutor) (*peer.StaticCollectionConfig, error) {
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

func fetchCollConfigPkg(chaincodeName string, qe ledger.SimpleQueryExecutor) (*peer.CollectionConfigPackage, error) {
	collKey := privdata.BuildCollectionKVSKey(chaincodeName)
	collectionConfigPkgBytes, err := qe.GetState(lsccNamespace, collKey)
	if err != nil || collectionConfigPkgBytes == nil {
		return nil, err
	}
	collectionConfigPkg := &peer.CollectionConfigPackage{}
	if err := proto.Unmarshal(collectionConfigPkgBytes, collectionConfigPkg); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling chaincode collection config pkg")
	}
	return collectionConfigPkg, nil
}
