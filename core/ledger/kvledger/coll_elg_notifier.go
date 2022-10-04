/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
)

// collElgNotifier listens for the chaincode events and determines whether the peer has become eligible for one or more existing
// private data collections and notifies the registered listener
type collElgNotifier struct {
	deployedChaincodeInfoProvider ledger.DeployedChaincodeInfoProvider
	membershipInfoProvider        ledger.MembershipInfoProvider
	listeners                     map[string]collElgListener
}

// Name returns the name of the listener
func (n *collElgNotifier) Name() string {
	return "collection eligibility listener"
}

func (n *collElgNotifier) Initialize(ledgerID string, qe ledger.SimpleQueryExecutor) error {
	// Noop
	return nil
}

// InterestedInNamespaces implements function in interface ledger.StateListener
func (n *collElgNotifier) InterestedInNamespaces() []string {
	return n.deployedChaincodeInfoProvider.Namespaces()
}

// HandleStateUpdates implements function in interface ledger.StateListener
// This function gets invoked when one or more chaincodes are deployed or upgraded by a block.
// This function, for each upgraded chaincode, performs the following
//  1. Retrieves the existing collection configurations and new collection configurations
//  2. Computes the collections for which the peer is not eligible as per the existing collection configuration
//     but is eligible as per the new collection configuration
//
// Finally, it causes an invocation to function 'ProcessCollsEligibilityEnabled' on ledger store with a map {ns:colls}
// that contains the details of <ns, coll> combination for which the eligibility of the peer is switched on.
func (n *collElgNotifier) HandleStateUpdates(trigger *ledger.StateUpdateTrigger) error {
	nsCollMap := map[string][]string{}
	qe := trigger.CommittedStateQueryExecutor
	postCommitQE := trigger.PostCommitQueryExecutor

	stateUpdates := extractPublicUpdates(trigger.StateUpdates)
	ccLifecycleInfo, err := n.deployedChaincodeInfoProvider.UpdatedChaincodes(stateUpdates)
	if err != nil {
		return err
	}
	var existingCCInfo, postCommitCCInfo *ledger.DeployedChaincodeInfo
	for _, ccInfo := range ccLifecycleInfo {
		ledgerid := trigger.LedgerID
		ccName := ccInfo.Name
		if existingCCInfo, err = n.deployedChaincodeInfoProvider.ChaincodeInfo(ledgerid, ccName, qe); err != nil {
			return err
		}
		if existingCCInfo == nil { // not an upgrade transaction
			continue
		}
		if postCommitCCInfo, err = n.deployedChaincodeInfoProvider.ChaincodeInfo(ledgerid, ccName, postCommitQE); err != nil {
			return err
		}
		elgEnabledCollNames, err := n.elgEnabledCollNames(
			ledgerid,
			existingCCInfo.ExplicitCollectionConfigPkg,
			postCommitCCInfo.ExplicitCollectionConfigPkg,
		)
		if err != nil {
			return err
		}
		logger.Debugf("[%s] collections of chaincode [%s] for which peer was not eligible before and now the eligiblity is enabled - [%s]",
			ledgerid, ccName, elgEnabledCollNames,
		)
		if len(elgEnabledCollNames) > 0 {
			nsCollMap[ccName] = elgEnabledCollNames
		}
	}
	if len(nsCollMap) > 0 {
		return n.invokeLedgerSpecificNotifier(trigger.LedgerID, trigger.CommittingBlockNum, nsCollMap)
	}
	return nil
}

func (n *collElgNotifier) registerListener(ledgerID string, listener collElgListener) {
	n.listeners[ledgerID] = listener
}

func (n *collElgNotifier) invokeLedgerSpecificNotifier(ledgerID string, commtingBlk uint64, nsCollMap map[string][]string) error {
	listener := n.listeners[ledgerID]
	return listener.ProcessCollsEligibilityEnabled(commtingBlk, nsCollMap)
}

// elgEnabledCollNames returns the names of the collections for which the peer is not eligible as per 'existingPkg' and is eligible as per 'postCommitPkg'
func (n *collElgNotifier) elgEnabledCollNames(ledgerID string,
	existingPkg, postCommitPkg *peer.CollectionConfigPackage) ([]string, error) {
	collectionNames := []string{}
	exisingConfs := retrieveCollConfs(existingPkg)
	postCommitConfs := retrieveCollConfs(postCommitPkg)
	existingConfMap := map[string]*peer.StaticCollectionConfig{}
	for _, existingConf := range exisingConfs {
		existingConfMap[existingConf.Name] = existingConf
	}

	for _, postCommitConf := range postCommitConfs {
		collName := postCommitConf.Name
		existingConf, ok := existingConfMap[collName]
		if !ok { // brand new collection
			continue
		}
		membershipEnabled, err := n.elgEnabled(ledgerID, existingConf.MemberOrgsPolicy, postCommitConf.MemberOrgsPolicy)
		if err != nil {
			return nil, err
		}
		if !membershipEnabled {
			continue
		}
		// not an existing member and added now
		collectionNames = append(collectionNames, collName)
	}
	return collectionNames, nil
}

// elgEnabled returns true if the peer is not eligible for a collection as per 'existingPolicy' and is eligible as per 'postCommitPolicy'
func (n *collElgNotifier) elgEnabled(ledgerID string, existingPolicy, postCommitPolicy *peer.CollectionPolicyConfig) (bool, error) {
	existingMember, err := n.membershipInfoProvider.AmMemberOf(ledgerID, existingPolicy)
	if err != nil || existingMember {
		return false, err
	}
	return n.membershipInfoProvider.AmMemberOf(ledgerID, postCommitPolicy)
}

func extractPublicUpdates(stateUpdates ledger.StateUpdates) map[string][]*kvrwset.KVWrite {
	m := map[string][]*kvrwset.KVWrite{}
	for ns, updates := range stateUpdates {
		m[ns] = updates.PublicUpdates
	}
	return m
}

// StateCommitDone implements function in interface ledger.StateListener
func (n *collElgNotifier) StateCommitDone(ledgerID string) {
	// Noop
}

type collElgListener interface {
	ProcessCollsEligibilityEnabled(commitingBlk uint64, nsCollMap map[string][]string) error
}

func retrieveCollConfs(collConfPkg *peer.CollectionConfigPackage) []*peer.StaticCollectionConfig {
	if collConfPkg == nil {
		return nil
	}
	var staticCollConfs []*peer.StaticCollectionConfig
	protoConfArray := collConfPkg.Config
	for _, protoConf := range protoConfArray {
		staticCollConfs = append(staticCollConfs, protoConf.GetStaticCollectionConfig())
	}
	return staticCollConfs
}
