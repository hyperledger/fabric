/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"regexp"

	"github.com/hyperledger/fabric/core/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"

	"github.com/pkg/errors"
)

// Namespaces returns the list of namespaces which are relevant to chaincode lifecycle
func (l *Lifecycle) Namespaces() []string {
	return append([]string{LifecycleNamespace}, l.LegacyDeployedCCInfoProvider.Namespaces()...)
}

var SequenceMatcher = regexp.MustCompile("^" + NamespacesName + "/fields/([^/]+)/Sequence$")

// UpdatedChaincodes returns the chaincodes that are getting updated by the supplied 'stateUpdates'
func (l *Lifecycle) UpdatedChaincodes(stateUpdates map[string][]*kvrwset.KVWrite) ([]*ledger.ChaincodeLifecycleInfo, error) {
	lifecycleInfo := []*ledger.ChaincodeLifecycleInfo{}

	// If the lifecycle table was updated, report only modified chaincodes
	lifecycleUpdates := stateUpdates[LifecycleNamespace]

	for _, kvWrite := range lifecycleUpdates {
		matches := SequenceMatcher.FindStringSubmatch(kvWrite.Key)
		if len(matches) != 2 {
			continue
		}
		// XXX Note, this may not be a chaincode namespace, handle this later
		lifecycleInfo = append(lifecycleInfo, &ledger.ChaincodeLifecycleInfo{Name: matches[1]})
	}

	legacyUpdates, err := l.LegacyDeployedCCInfoProvider.UpdatedChaincodes(stateUpdates)
	if err != nil {
		return nil, errors.WithMessage(err, "error invoking legacy deployed cc info provider")
	}

	return append(lifecycleInfo, legacyUpdates...), nil
}

// ChaincodeInfo implements function in interface ledger.DeployedChaincodeInfoProvider and returns info about a deployed chaincode
func (l *Lifecycle) ChaincodeInfo(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error) {
	return l.LegacyDeployedCCInfoProvider.ChaincodeInfo(channelName, chaincodeName, qe)
}

// CollectionInfo implements function in interface ledger.DeployedChaincodeInfoProvider, it returns config for
// both static and implicit collections.
func (l *Lifecycle) CollectionInfo(channelName, chaincodeName, collectionName string, qe ledger.SimpleQueryExecutor) (*cb.StaticCollectionConfig, error) {
	return l.LegacyDeployedCCInfoProvider.CollectionInfo(channelName, chaincodeName, collectionName, qe)
}

// ImplicitCollections implements function in interface ledger.DeployedChaincodeInfoProvider.  It returns
//a slice that contains one proto msg for each of the implicit collections
func (l *Lifecycle) ImplicitCollections(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) ([]*cb.StaticCollectionConfig, error) {
	return nil, nil
}
