/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"fmt"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
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

//defaultACLProvider used if resource-based ACL Provider is not provided or
//if it does not contain a policy for the named resource
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
	d.pResourcePolicyMap[resources.LSCC_INSTALL] = ""
	d.pResourcePolicyMap[resources.LSCC_GETCHAINCODES] = ""
	d.pResourcePolicyMap[resources.LSCC_GETINSTALLEDCHAINCODES] = ""

	//c resources
	d.cResourcePolicyMap[resources.LSCC_DEPLOY] = ""  //ACL check covered by PROPOSAL
	d.cResourcePolicyMap[resources.LSCC_UPGRADE] = "" //ACL check covered by PROPOSAL
	d.cResourcePolicyMap[resources.LSCC_GETCCINFO] = CHANNELREADERS
	d.cResourcePolicyMap[resources.LSCC_GETDEPSPEC] = CHANNELREADERS
	d.cResourcePolicyMap[resources.LSCC_GETCCDATA] = CHANNELREADERS

	//-------------- QSCC --------------
	//p resources (none)

	//c resources
	d.cResourcePolicyMap[resources.QSCC_GetChainInfo] = CHANNELREADERS
	d.cResourcePolicyMap[resources.QSCC_GetBlockByNumber] = CHANNELREADERS
	d.cResourcePolicyMap[resources.QSCC_GetBlockByHash] = CHANNELREADERS
	d.cResourcePolicyMap[resources.QSCC_GetTransactionByID] = CHANNELREADERS
	d.cResourcePolicyMap[resources.QSCC_GetBlockByTxID] = CHANNELREADERS

	//--------------- CSCC resources -----------
	//p resources (implemented by the chaincode currently)
	d.pResourcePolicyMap[resources.CSCC_JoinChain] = ""
	d.pResourcePolicyMap[resources.CSCC_GetChannels] = ""

	//c resources
	d.cResourcePolicyMap[resources.CSCC_GetConfigBlock] = CHANNELREADERS
	d.cResourcePolicyMap[resources.CSCC_GetConfigTree] = CHANNELREADERS
	d.cResourcePolicyMap[resources.CSCC_SimulateConfigTreeUpdate] = CHANNELWRITERS

	//---------------- non-scc resources ------------
	//Propose
	d.cResourcePolicyMap[resources.PROPOSE] = CHANNELWRITERS

	//Chaincode-to-Chaincode
	d.cResourcePolicyMap[resources.CC2CC] = CHANNELWRITERS

	//Events (not used currently - for future)
	d.cResourcePolicyMap[resources.BLOCKEVENT] = CHANNELREADERS
	d.cResourcePolicyMap[resources.FILTEREDBLOCKEVENT] = CHANNELREADERS
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
	case *common.Envelope:
		sd, err := idinfo.(*common.Envelope).AsSignedData()
		if err != nil {
			return err
		}
		return d.policyChecker.CheckPolicyBySignedData(channelID, policy, sd)
	default:
		aclLogger.Errorf("Unmapped id on checkACL %s", resName)
		return fmt.Errorf("Unknown id on checkACL %s", resName)
	}
}
