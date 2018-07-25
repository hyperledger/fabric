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

func NewDefaultACLProvider() ACLProvider {
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
	d.pResourcePolicyMap[resources.Lscc_Install] = ""
	d.pResourcePolicyMap[resources.Lscc_GetInstalledChaincodes] = ""

	//c resources
	d.cResourcePolicyMap[resources.Lscc_Deploy] = ""  //ACL check covered by PROPOSAL
	d.cResourcePolicyMap[resources.Lscc_Upgrade] = "" //ACL check covered by PROPOSAL
	d.cResourcePolicyMap[resources.Lscc_ChaincodeExists] = CHANNELREADERS
	d.cResourcePolicyMap[resources.Lscc_GetDeploymentSpec] = CHANNELREADERS
	d.cResourcePolicyMap[resources.Lscc_GetChaincodeData] = CHANNELREADERS
	d.cResourcePolicyMap[resources.Lscc_GetInstantiatedChaincodes] = CHANNELREADERS
	d.cResourcePolicyMap[resources.Lscc_GetCollectionsConfig] = CHANNELREADERS

	//-------------- QSCC --------------
	//p resources (none)

	//c resources
	d.cResourcePolicyMap[resources.Qscc_GetChainInfo] = CHANNELREADERS
	d.cResourcePolicyMap[resources.Qscc_GetBlockByNumber] = CHANNELREADERS
	d.cResourcePolicyMap[resources.Qscc_GetBlockByHash] = CHANNELREADERS
	d.cResourcePolicyMap[resources.Qscc_GetTransactionByID] = CHANNELREADERS
	d.cResourcePolicyMap[resources.Qscc_GetBlockByTxID] = CHANNELREADERS

	//--------------- CSCC resources -----------
	//p resources (implemented by the chaincode currently)
	d.pResourcePolicyMap[resources.Cscc_JoinChain] = ""
	d.pResourcePolicyMap[resources.Cscc_GetChannels] = ""

	//c resources
	d.cResourcePolicyMap[resources.Cscc_GetConfigBlock] = CHANNELREADERS
	d.cResourcePolicyMap[resources.Cscc_GetConfigTree] = CHANNELREADERS
	d.cResourcePolicyMap[resources.Cscc_SimulateConfigTreeUpdate] = CHANNELWRITERS

	//---------------- non-scc resources ------------
	//Peer resources
	d.cResourcePolicyMap[resources.Peer_Propose] = CHANNELWRITERS
	d.cResourcePolicyMap[resources.Peer_ChaincodeToChaincode] = CHANNELWRITERS

	//Event resources
	d.cResourcePolicyMap[resources.Event_Block] = CHANNELREADERS
	d.cResourcePolicyMap[resources.Event_FilteredBlock] = CHANNELREADERS
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
