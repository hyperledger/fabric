/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cceventmgmt

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
)

// KVLedgerLSCCStateListener listens for state changes on 'lscc' namespace
type KVLedgerLSCCStateListener struct {
}

// HandleStateUpdates iterates over key-values being written in the 'lscc' namespace (which indicates deployment of a chaincode)
// and invokes `HandleChaincodeDeploy` function on chaincode event manager (which in turn is responsible for creation of statedb
// artifacts for the chaincode statedata)
func (listener *KVLedgerLSCCStateListener) HandleStateUpdates(channelName string, stateUpdates ledger.StateUpdates) error {
	kvWrites := stateUpdates.([]*kvrwset.KVWrite)
	logger.Debugf("Channel [%s]: Handling state updates in LSCC namespace - stateUpdates=%#v", channelName, kvWrites)
	chaincodeDefs := []*ChaincodeDefinition{}
	for _, kvWrite := range kvWrites {
		// There are LSCC entries for the chaincode and for the chaincode collections.
		// We need to ignore changes to chaincode collections, and handle changes to chaincode
		// We can detect collections based on the presence of a CollectionSeparator, which never exists in chaincode names
		if privdata.IsCollectionConfigKey(kvWrite.Key) {
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
			return fmt.Errorf("Unmarshalling ChaincodeQueryResponse failed, error %s", err)
		}
		chaincodeDefs = append(chaincodeDefs, &ChaincodeDefinition{Name: chaincodeData.CCName(), Version: chaincodeData.CCVersion(), Hash: chaincodeData.Hash()})
	}
	return GetMgr().HandleChaincodeDeploy(channelName, chaincodeDefs)
}
