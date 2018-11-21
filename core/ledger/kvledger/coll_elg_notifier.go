/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
)

// collElgNotifier listens for the chaincode events and determines whether the peer has become eligible for one or more existing
// private data collections and notifies the registered listener
type collElgNotifier struct {
	deployedChaincodeInfoProvider ledger.DeployedChaincodeInfoProvider
	membershipInfoProvider        ledger.MembershipInfoProvider
	listeners                     map[string]collElgListener
}

// InterestedInNamespaces implements function in interface ledger.StateListener
func (n *collElgNotifier) InterestedInNamespaces() []string {
	return n.deployedChaincodeInfoProvider.Namespaces()
}

// HandleStateUpdates implements function in interface ledger.StateListener
// This function gets invoked when one or more chaincodes are deployed or upgraded by a block.
// This function, for each upgraded chaincode, performs the following
// 1) Retrieves the existing collection configurations and new collection configurations
// 2) Computes the collections for which the peer is not eligible as per the existing collection configuration
//    but is eligible as per the new collection configuration
// Finally, it causes an invocation to function 'ProcessCollsEligibilityEnabled' on ledger store with a map {ns:colls}
// that contains the details of <ns, coll> combination for which the eligibility of the peer is switched on.
func (n *collElgNotifier) HandleStateUpdates(trigger *ledger.StateUpdateTrigger) error {
	nsCollMap := map[string][]string{}
	qe := trigger.CommittedStateQueryExecutor
	postCommitQE := trigger.PostCommitQueryExecutor

	stateUpdates := convertToKVWrites(trigger.StateUpdates)
	ccLifecycleInfo, err := n.deployedChaincodeInfoProvider.UpdatedChaincodes(stateUpdates)
	if err != nil {
		return err
	}
	var existingCCInfo, postCommitCCInfo *ledger.DeployedChaincodeInfo
	for _, ccInfo := range ccLifecycleInfo {
		ledgerid := trigger.LedgerID
		ccName := ccInfo.Name
		if existingCCInfo, err = n.deployedChaincodeInfoProvider.ChaincodeInfo(ccName, qe); err != nil {
			return err
		}
		if existingCCInfo == nil { // not an upgrade transaction
			continue
		}
		if postCommitCCInfo, err = n.deployedChaincodeInfoProvider.ChaincodeInfo(ccName, postCommitQE); err != nil {
			return err
		}
		elgEnabledCollNames, err := n.elgEnabledCollNames(
			ledgerid,
			existingCCInfo.CollectionConfigPkg,
			postCommitCCInfo.CollectionConfigPkg,
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
		n.invokeLedgerSpecificNotifier(trigger.LedgerID, trigger.CommittingBlockNum, nsCollMap)
	}
	return nil
}

func (n *collElgNotifier) registerListener(ledgerID string, listener collElgListener) {
	n.listeners[ledgerID] = listener
}

func (n *collElgNotifier) invokeLedgerSpecificNotifier(ledgerID string, commtingBlk uint64, nsCollMap map[string][]string) {
	listener := n.listeners[ledgerID]
	listener.ProcessCollsEligibilityEnabled(commtingBlk, nsCollMap)
}

// elgEnabledCollNames returns the names of the collections for which the peer is not eligible as per 'existingPkg' and is eligible as per 'postCommitPkg'
func (n *collElgNotifier) elgEnabledCollNames(ledgerID string,
	existingPkg, postCommitPkg *common.CollectionConfigPackage) ([]string, error) {

	collectionNames := []string{}
	exisingConfs := retrieveCollConfs(existingPkg)
	postCommitConfs := retrieveCollConfs(postCommitPkg)
	existingConfMap := map[string]*common.StaticCollectionConfig{}
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
func (n *collElgNotifier) elgEnabled(ledgerID string, existingPolicy, postCommitPolicy *common.CollectionPolicyConfig) (bool, error) {
	existingMember, err := n.membershipInfoProvider.AmMemberOf(ledgerID, existingPolicy)
	if err != nil || existingMember {
		return false, err
	}
	return n.membershipInfoProvider.AmMemberOf(ledgerID, postCommitPolicy)
}

func convertToKVWrites(stateUpdates ledger.StateUpdates) map[string][]*kvrwset.KVWrite {
	m := map[string][]*kvrwset.KVWrite{}
	for ns, updates := range stateUpdates {
		m[ns] = updates.([]*kvrwset.KVWrite)
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

func retrieveCollConfs(collConfPkg *common.CollectionConfigPackage) []*common.StaticCollectionConfig {
	if collConfPkg == nil {
		return nil
	}
	var staticCollConfs []*common.StaticCollectionConfig
	protoConfArray := collConfPkg.Config
	for _, protoConf := range protoConfArray {
		staticCollConfs = append(staticCollConfs, protoConf.GetStaticCollectionConfig())
	}
	return staticCollConfs
}
