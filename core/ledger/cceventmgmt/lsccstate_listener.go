/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cceventmgmt

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/pkg/errors"
)

const (
	lsccNamespace = "lscc"
)

// KVLedgerLSCCStateListener listens for state changes on 'lscc' namespace
type KVLedgerLSCCStateListener struct {
}

// HandleStateUpdates iterates over key-values being written in the 'lscc' namespace (which indicates deployment of a chaincode)
// and invokes `HandleChaincodeDeploy` function on chaincode event manager (which in turn is responsible for creation of statedb
// artifacts for the chaincode statedata)
func (listener *KVLedgerLSCCStateListener) HandleStateUpdates(trigger *ledger.StateUpdateTrigger) error {
	channelName, stateUpdates := trigger.LedgerID, trigger.StateUpdates
	kvWrites := stateUpdates[lsccNamespace].([]*kvrwset.KVWrite)
	logger.Debugf("Channel [%s]: Handling state updates in LSCC namespace - stateUpdates=%#v", channelName, kvWrites)
	chaincodeDefs := []*ChaincodeDefinition{}
	chaincodesCollConfigs := make(map[string][]byte)

	for _, kvWrite := range kvWrites {
		// There are LSCC entries for the chaincode and for the chaincode collections.
		// We can detect collections based on the presence of a CollectionSeparator,
		// which never exists in chaincode names.
		if privdata.IsCollectionConfigKey(kvWrite.Key) {
			ccname := privdata.GetCCNameFromCollectionConfigKey(kvWrite.Key)
			chaincodesCollConfigs[ccname] = kvWrite.Value
			continue
		}
		// Ignore delete events
		if kvWrite.IsDelete {
			continue
		}
		// Chaincode instantiate/upgrade is not logged on committing peer anywhere else.  This is a good place to log it.
		logger.Infof("Channel [%s]: Handling LSCC state update for chaincode [%s]", channelName, kvWrite.Key)
		chaincodeData := &ccprovider.ChaincodeData{}
		if err := proto.Unmarshal(kvWrite.Value, chaincodeData); err != nil {
			return errors.Wrap(err, "error unmarshalling chaincode state data")
		}
		chaincodeDefs = append(chaincodeDefs, &ChaincodeDefinition{Name: chaincodeData.CCName(), Version: chaincodeData.CCVersion(), Hash: chaincodeData.Hash()})
	}

	for _, chaincodeDef := range chaincodeDefs {
		chaincodeCollConfigs, ok := chaincodesCollConfigs[chaincodeDef.Name]
		if ok {
			chaincodeDef.CollectionConfigs = chaincodeCollConfigs
		}
	}

	return GetMgr().HandleChaincodeDeploy(channelName, chaincodeDefs)
}

// InterestedInNamespaces implements function from interface `ledger.StateListener`
func (listener *KVLedgerLSCCStateListener) InterestedInNamespaces() []string {
	return []string{lsccNamespace}
}

// StateCommitDone implements function from interface `ledger.StateListener` as a NOOP
func (listener *KVLedgerLSCCStateListener) StateCommitDone(channelName string) {
	GetMgr().ChaincodeDeployDone(channelName)
}
