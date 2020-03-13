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
	"fmt"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric-protos-go/transientstore"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/pkg/errors"
)

// PvtRWSetAssembler assembles private read write set for distribution
// augments with additional information if needed
type PvtRWSetAssembler interface {
	// AssemblePvtRWSet prepares TxPvtReadWriteSet for distribution
	// augmenting it into TxPvtReadWriteSetWithConfigInfo adding
	// information about collections config available related
	// to private read-write set
	AssemblePvtRWSet(channelName string,
		privData *rwset.TxPvtReadWriteSet,
		txsim ledger.SimpleQueryExecutor,
		deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider) (
		*transientstore.TxPvtReadWriteSetWithConfigInfo, error,
	)
}

// CollectionConfigRetriever encapsulates sub-functionality of ledger.TxSimulator
// to abstract minimum required functions set
type CollectionConfigRetriever interface {
	// GetState gets the value for given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	GetState(namespace string, key string) ([]byte, error)
}

// AssemblePvtRWSet prepares TxPvtReadWriteSet for distribution
// augmenting it into TxPvtReadWriteSetWithConfigInfo adding
// information about collections config available related
// to private read-write set
func AssemblePvtRWSet(channelName string,
	privData *rwset.TxPvtReadWriteSet,
	txsim ledger.SimpleQueryExecutor,
	deployedCCInfoProvider ledger.DeployedChaincodeInfoProvider) (
	*transientstore.TxPvtReadWriteSetWithConfigInfo, error,
) {
	txPvtRwSetWithConfig := &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset:          privData,
		CollectionConfigs: make(map[string]*peer.CollectionConfigPackage),
	}

	for _, pvtRwset := range privData.NsPvtRwset {
		namespace := pvtRwset.Namespace
		if _, found := txPvtRwSetWithConfig.CollectionConfigs[namespace]; !found {
			colCP, err := deployedCCInfoProvider.AllCollectionsConfigPkg(channelName, namespace, txsim)
			if err != nil {
				return nil, errors.WithMessagef(err, "error while retrieving collection config for chaincode %#v", namespace)
			}
			if colCP == nil {
				return nil, errors.New(fmt.Sprintf("no collection config for chaincode %#v", namespace))
			}
			txPvtRwSetWithConfig.CollectionConfigs[namespace] = colCP
		}
	}
	trimCollectionConfigs(txPvtRwSetWithConfig)
	return txPvtRwSetWithConfig, nil
}

func trimCollectionConfigs(pvtData *transientstore.TxPvtReadWriteSetWithConfigInfo) {
	flags := make(map[string]map[string]struct{})
	for _, pvtRWset := range pvtData.PvtRwset.NsPvtRwset {
		namespace := pvtRWset.Namespace
		for _, col := range pvtRWset.CollectionPvtRwset {
			if _, found := flags[namespace]; !found {
				flags[namespace] = make(map[string]struct{})
			}
			flags[namespace][col.CollectionName] = struct{}{}
		}
	}

	filteredConfigs := make(map[string]*peer.CollectionConfigPackage)
	for namespace, configs := range pvtData.CollectionConfigs {
		filteredConfigs[namespace] = &peer.CollectionConfigPackage{}
		for _, conf := range configs.Config {
			if colConf := conf.GetStaticCollectionConfig(); colConf != nil {
				if _, found := flags[namespace][colConf.Name]; found {
					filteredConfigs[namespace].Config = append(filteredConfigs[namespace].Config, conf)
				}
			}
		}
	}
	pvtData.CollectionConfigs = filteredConfigs
}
