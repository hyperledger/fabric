/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rscc

import (
	"fmt"

	"github.com/hyperledger/fabric/common/config/resources"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

//--------- errors ---------

//PolicyNotFound cache for resource
type PolicyNotFound string

func (e PolicyNotFound) Error() string {
	return fmt.Sprintf("policy %s not found", string(e))
}

//InvalidIdInfo
type InvalidIdInfo string

func (e InvalidIdInfo) Error() string {
	return fmt.Sprintf("Invalid id for policy [%s]", string(e))
}

//---------- policyEvaluator ------

//policyEvalutor interface provides the interfaces for policy evaluation
type policyEvaluator interface {
	PolicyRefForResource(resName string) string
	Evaluate(polName string, id []*common.SignedData) error
}

//policyEvaluatorImpl implements policyEvaluator
type policyEvaluatorImpl struct {
	bundle *resources.Bundle
}

func (pe *policyEvaluatorImpl) PolicyRefForResource(resName string) string {
	pm := pe.bundle.ResourcePolicyMapper()
	if pm == nil {
		return ""
	}

	return pm.PolicyRefForResource(resName)
}

func (pe *policyEvaluatorImpl) Evaluate(polName string, sd []*common.SignedData) error {
	policy, ok := pe.bundle.PolicyManager().GetPolicy(polName)
	if !ok {
		return PolicyNotFound(polName)
	}

	return policy.Evaluate(sd)
}

//------ resourcePolicyProvider ----------

//rsccPolicyProvider is the basic policy provider for RSCC. It is an ACLProvider
type rsccPolicyProvider interface {
	GetPolicyName(resName string) string
	CheckACL(resName string, idinfo interface{}) error
}

//rsccPolicyProviderImpl holds the bytes from state of the ledger
type rsccPolicyProviderImpl struct {
	//this is mainly used for logging and information
	channel string

	pEvaluator policyEvaluator
}

//GetPolicyName returns the policy name given the resource string
func (rp *rsccPolicyProviderImpl) GetPolicyName(resName string) string {
	return rp.pEvaluator.PolicyRefForResource(resName)
}

func newRsccPolicyProvider(channel string, pEvaluator policyEvaluator) rsccPolicyProvider {
	return &rsccPolicyProviderImpl{channel, pEvaluator}
}

//CheckACL rscc implements AClProvider's CheckACL interface so it can be registered
//as a provider with aclmgmt
func (rp *rsccPolicyProviderImpl) CheckACL(polName string, idinfo interface{}) error {
	rsccLogger.Debugf("rscc  acl check(%s)", polName)

	//we will implemented other identifiers. In the end we just need a SignedData`
	signedProp, _ := idinfo.(*pb.SignedProposal)
	if signedProp == nil {
		return InvalidIdInfo(polName)
	}

	// Prepare SignedData
	proposal, err := utils.GetProposal(signedProp.ProposalBytes)
	if err != nil {
		return fmt.Errorf("Failing extracting proposal during check policy with policy [%s]: [%s]", polName, err)
	}

	header, err := utils.GetHeader(proposal.Header)
	if err != nil {
		return fmt.Errorf("Failing extracting header during check policy [%s]: [%s]", polName, err)
	}

	shdr, err := utils.GetSignatureHeader(header.SignatureHeader)
	if err != nil {
		return fmt.Errorf("Invalid Proposal's SignatureHeader during check policy [%s]: [%s]", polName, err)
	}

	sd := []*common.SignedData{&common.SignedData{
		Data:      signedProp.ProposalBytes,
		Identity:  shdr.Creator,
		Signature: signedProp.Signature,
	}}

	err = rp.pEvaluator.Evaluate(polName, sd)
	if err != nil {
		return fmt.Errorf("Failed evaluating policy on signed data during check policy [%s]: [%s]", polName, err)
	}

	return nil
}
