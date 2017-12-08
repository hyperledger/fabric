/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"fmt"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

const (
	CHANNELREADERS = policies.ChannelApplicationReaders
	CHANNELWRITERS = policies.ChannelApplicationWriters
)

//defaultACLProvider if RSCC not provided use the default pre 1.0 implementation
type defaultACLProvider struct {
	policyChecker policy.PolicyChecker

	//peer wide policy (currently not used)
	pResourcePolicyMap map[string]string

	//channel specific policy
	cResourcePolicyMap map[string]string
}

func newDefaultACLProvider() ACLProvider {
	d := &defaultACLProvider{}
	d.initialize()

	return d
}

func (d *defaultACLProvider) initialize() {
	d.policyChecker = policy.NewPolicyChecker(
		peer.NewChannelPolicyManagerGetter(),
		mgmt.GetLocalMSP(),
		mgmt.NewLocalMSPPrincipalGetter(),
	)

	d.pResourcePolicyMap = make(map[string]string)
	d.cResourcePolicyMap = make(map[string]string)

	//-------------- LSCC --------------
	//p resources (implemented by the chaincode currently)
	d.pResourcePolicyMap[LSCC_INSTALL] = ""
	d.pResourcePolicyMap[LSCC_GETCHAINCODES] = ""
	d.pResourcePolicyMap[LSCC_GETINSTALLEDCHAINCODES] = ""

	//c resources
	d.cResourcePolicyMap[LSCC_DEPLOY] = ""  //ACL check covered by PROPOSAL
	d.cResourcePolicyMap[LSCC_UPGRADE] = "" //ACL check covered by PROPOSAL
	d.cResourcePolicyMap[LSCC_GETCCINFO] = CHANNELREADERS
	d.cResourcePolicyMap[LSCC_GETDEPSPEC] = CHANNELREADERS
	d.cResourcePolicyMap[LSCC_GETCCDATA] = CHANNELREADERS

	//-------------- QSCC --------------
	//p resources (none)

	//c resources
	d.cResourcePolicyMap[QSCC_GetChainInfo] = CHANNELREADERS
	d.cResourcePolicyMap[QSCC_GetBlockByNumber] = CHANNELREADERS
	d.cResourcePolicyMap[QSCC_GetBlockByHash] = CHANNELREADERS
	d.cResourcePolicyMap[QSCC_GetTransactionByID] = CHANNELREADERS
	d.cResourcePolicyMap[QSCC_GetBlockByTxID] = CHANNELREADERS

	//--------------- CSCC resources -----------
	//p resources (implemented by the chaincode currently)
	d.pResourcePolicyMap[CSCC_JoinChain] = ""
	d.pResourcePolicyMap[CSCC_GetChannels] = ""

	//c resources
	d.cResourcePolicyMap[CSCC_GetConfigBlock] = CHANNELREADERS
	d.cResourcePolicyMap[CSCC_GetConfigTree] = CHANNELREADERS
	d.cResourcePolicyMap[CSCC_SimulateConfigTreeUpdate] = CHANNELWRITERS

	//---------------- non-scc resources ------------
	//Propose
	d.cResourcePolicyMap[PROPOSE] = CHANNELWRITERS

	//Chaincode-to-Chaincode
	d.cResourcePolicyMap[CC2CC] = CHANNELWRITERS

	//Events (not used currently - for future)
	d.cResourcePolicyMap[BLOCKEVENT] = CHANNELREADERS
	d.cResourcePolicyMap[FILTEREDBLOCKEVENT] = CHANNELREADERS
}

//this should cover an exhaustive list of everything called from the peer
func (d *defaultACLProvider) defaultPolicy(resName string, cprovider bool) string {
	var pol string
	if cprovider {
		pol = d.cResourcePolicyMap[resName]
	} else {
		pol = d.pResourcePolicyMap[resName]
	}
	return pol
}

//CheckACL provides default (v 1.0) behavior by mapping resources to their ACL for a channel
func (d *defaultACLProvider) CheckACL(resName string, channelID string, idinfo interface{}) error {
	policy := d.defaultPolicy(resName, true)
	if policy == "" {
		aclLogger.Errorf("Unmapped policy for %s", resName)
		return fmt.Errorf("Unmapped policy for %s", resName)
	}

	switch idinfo.(type) {
	case *pb.SignedProposal:
		return d.policyChecker.CheckPolicy(channelID, policy, idinfo.(*pb.SignedProposal))
	default:
		aclLogger.Errorf("Unmapped id on checkACL %s", resName)
		return fmt.Errorf("Unknown id on checkACL %s", resName)
	}
}

//GenerateSimulationResults does nothing for default provider currently as it defaults to
//1.0 behavior
func (d *defaultACLProvider) GenerateSimulationResults(txEnvelop *common.Envelope, simulator ledger.TxSimulator, initializingLedger bool) error {
	return nil
}
