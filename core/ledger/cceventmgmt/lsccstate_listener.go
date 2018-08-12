/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cceventmgmt

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
)

// KVLedgerLSCCStateListener listens for state changes for chaincode lifecycle
type KVLedgerLSCCStateListener struct {
	DeployedChaincodeInfoProvider ledger.DeployedChaincodeInfoProvider
}

// HandleStateUpdates uses 'DeployedChaincodeInfoProvider' to findout deployment of a chaincode
// and invokes `HandleChaincodeDeploy` function on chaincode event manager (which in turn is responsible for creation of statedb
// artifacts for the chaincode statedata)
func (listener *KVLedgerLSCCStateListener) HandleStateUpdates(trigger *ledger.StateUpdateTrigger) error {
	channelName, kvWrites, postCommitQE, deployCCInfoProvider :=
		trigger.LedgerID, convertToKVWrites(trigger.StateUpdates), trigger.PostCommitQueryExecutor, listener.DeployedChaincodeInfoProvider

	logger.Debugf("Channel [%s]: Handling state updates in LSCC namespace - stateUpdates=%#v", channelName, kvWrites)
	updatedChaincodes, err := deployCCInfoProvider.UpdatedChaincodes(kvWrites)
	if err != nil {
		return err
	}
	chaincodeDefs := []*ChaincodeDefinition{}
	for _, updatedChaincode := range updatedChaincodes {
		logger.Infof("Channel [%s]: Handling deploy or update of chaincode [%s]", channelName, updatedChaincode.Name)
		if updatedChaincode.Deleted {
			// TODO handle delete case when delete is implemented in lifecycle
			continue
		}
		deployedCCInfo, err := deployCCInfoProvider.ChaincodeInfo(updatedChaincode.Name, postCommitQE)
		if err != nil {
			return err
		}
		chaincodeDefs = append(chaincodeDefs, &ChaincodeDefinition{
			Name:              deployedCCInfo.Name,
			Hash:              deployedCCInfo.Hash,
			Version:           deployedCCInfo.Version,
			CollectionConfigs: deployedCCInfo.CollectionConfigPkg,
		})
	}
	return GetMgr().HandleChaincodeDeploy(channelName, chaincodeDefs)
}

// InterestedInNamespaces implements function from interface `ledger.StateListener`
func (listener *KVLedgerLSCCStateListener) InterestedInNamespaces() []string {
	return listener.DeployedChaincodeInfoProvider.Namespaces()
}

// StateCommitDone implements function from interface `ledger.StateListener`
func (listener *KVLedgerLSCCStateListener) StateCommitDone(channelName string) {
	GetMgr().ChaincodeDeployDone(channelName)
}

func convertToKVWrites(stateUpdates ledger.StateUpdates) map[string][]*kvrwset.KVWrite {
	m := map[string][]*kvrwset.KVWrite{}
	for ns, updates := range stateUpdates {
		m[ns] = updates.([]*kvrwset.KVWrite)
	}
	return m
}
