/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cc

import (
	"bytes"
	"sync"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
)

// Subscription channels information flow
// about a specific channel into the Lifecycle
type Subscription struct {
	sync.Mutex
	lc             *Lifecycle
	channel        string
	queryCreator   QueryCreator
	pendingUpdates chan *cceventmgmt.ChaincodeDefinition
}

type depCCsRetriever func(Query, ChaincodePredicate, bool, ...string) (chaincode.MetadataSet, error)

// HandleChaincodeDeploy is expected to be invoked when a chaincode is deployed via a deploy transaction and the chaicndoe was already
// installed on the peer. This also gets invoked when an already deployed chaincode is installed on the peer
func (sub *Subscription) HandleChaincodeDeploy(chaincodeDefinition *cceventmgmt.ChaincodeDefinition, dbArtifactsTar []byte) error {
	Logger.Debug("Channel", sub.channel, "got a new deployment:", chaincodeDefinition)
	sub.pendingUpdates <- chaincodeDefinition
	return nil
}

func (sub *Subscription) processPendingUpdate(ccDef *cceventmgmt.ChaincodeDefinition) {
	query, err := sub.queryCreator.NewQuery()
	if err != nil {
		Logger.Errorf("Failed creating a new query for channel %s: %v", sub.channel, err)
		return
	}
	installedCC := []chaincode.InstalledChaincode{{
		Name:    ccDef.Name,
		Version: ccDef.Version,
		Id:      ccDef.Hash,
	}}
	ccs, err := queryChaincodeDefinitions(query, installedCC, DeployedChaincodes)
	if err != nil {
		Logger.Errorf("Query for channel %s for %v failed with error %v", sub.channel, ccDef, err)
		return
	}
	Logger.Debug("Updating channel", sub.channel, "with", ccs.AsChaincodes())
	sub.lc.updateState(sub.channel, ccs)
	sub.lc.fireChangeListeners(sub.channel)
}

// ChaincodeDeployDone gets invoked when the chaincode deploy transaction or chaincode install
// (the context in which the above function was invoked)
func (sub *Subscription) ChaincodeDeployDone(succeeded bool) {
	// Run a new goroutine which would dispatch a single pending update.
	// This is to prevent any ledger locks being obtained during the state query
	// to affect the locks held while invoking this method by the ledger itself.
	// We first lock and then take the pending update, to preserve order.
	sub.Lock()
	go func() {
		defer sub.Unlock()
		update := <-sub.pendingUpdates
		// If we haven't succeeded in deploying the chaincode, just skip the update
		if !succeeded {
			Logger.Error("Chaincode deploy for", update.Name, "failed")
			return
		}
		sub.processPendingUpdate(update)
	}()
}

func queryChaincodeDefinitions(query Query, ccs []chaincode.InstalledChaincode, deployedCCs depCCsRetriever) (chaincode.MetadataSet, error) {
	// map from string and version to chaincode ID
	installedCCsToIDs := make(map[nameVersion][]byte)
	// Populate the map
	for _, cc := range ccs {
		Logger.Debug("Chaincode", cc, "'s version is", cc.Version, "and Id is", cc.Id)
		installedCCsToIDs[installedCCToNameVersion(cc)] = cc.Id
	}

	filter := func(cc chaincode.Metadata) bool {
		installedID, exists := installedCCsToIDs[deployedCCToNameVersion(cc)]
		if !exists {
			Logger.Debug("Chaincode", cc, "is instantiated but a different version is installed")
			return false
		}
		if !bytes.Equal(installedID, cc.Id) {
			Logger.Debug("ID of chaincode", cc, "on filesystem doesn't match ID in ledger")
			return false
		}
		return true
	}

	return deployedCCs(query, filter, false, names(ccs)...)
}
