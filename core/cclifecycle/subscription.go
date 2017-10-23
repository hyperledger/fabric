/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cc

import (
	"bytes"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/pkg/errors"
)

// Subscription channels information flow
// about a specific channel into the Lifecycle
type Subscription struct {
	lc       *Lifecycle
	channel  string
	newQuery QueryCreator
}

type depCCsRetriever func(Query, ChaincodePredicate, ...string) (chaincode.MetadataSet, error)

// HandleChaincodeDeploy is expected to be invoked when a chaincode is deployed via a deploy transaction
func (sub *Subscription) HandleChaincodeDeploy(chaincodeDefinition *cceventmgmt.ChaincodeDefinition, dbArtifactsTar []byte) error {
	logger.Debug("Channel", sub.channel, "got a new deployment:", chaincodeDefinition)
	query, err := sub.newQuery()
	if err != nil {
		return errors.WithStack(err)
	}

	installedCC := []chaincode.InstalledChaincode{{
		Name:    chaincodeDefinition.Name,
		Version: chaincodeDefinition.Version,
		Id:      chaincodeDefinition.Hash,
	}}
	ccs, err := queryChaincodeDefinitions(query, installedCC, DeployedChaincodes)
	if err != nil {
		logger.Warning("Channel", sub.channel, "returning with error for", chaincodeDefinition, ":", err)
		return errors.Wrapf(err, "failed querying chaincode definition")
	}
	logger.Debugf("Updating channel", sub.channel, "with", ccs.AsChaincodes())
	sub.lc.updateState(sub.channel, ccs)
	sub.lc.fireChangeListeners(sub.channel)
	return nil
}

func queryChaincodeDefinitions(query Query, ccs []chaincode.InstalledChaincode, deployedCCs depCCsRetriever) (chaincode.MetadataSet, error) {
	// map from string and version to chaincode ID
	installedCCsToIDs := make(map[nameVersion][]byte)
	// Populate the map
	for _, cc := range ccs {
		logger.Debugf("Chaincode", cc, "'s version is", cc.Version, "and Id is", cc.Id)
		installedCCsToIDs[installedCCToNameVersion(cc)] = cc.Id
	}

	filter := func(cc chaincode.Metadata) bool {
		installedID, exists := installedCCsToIDs[deployedCCToNameVersion(cc)]
		if !exists {
			logger.Debug("Chaincode", cc, "is instantiated but a different version is installed")
			return false
		}
		if !bytes.Equal(installedID, cc.Id) {
			logger.Debug("ID of chaincode", cc, "on filesystem doesn't match ID in ledger")
			return false
		}
		return true
	}

	return deployedCCs(query, filter, names(ccs)...)
}
