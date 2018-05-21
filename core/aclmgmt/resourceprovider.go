/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"fmt"

	"github.com/hyperledger/fabric/common/channelconfig"
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
	PolicyRefForAPI(resName string) string
	Evaluate(polName string, id []*common.SignedData) error
}

//policyEvaluatorImpl implements policyEvaluator
type policyEvaluatorImpl struct {
	bundle channelconfig.Resources
}

func (pe *policyEvaluatorImpl) PolicyRefForAPI(resName string) string {
	app, exists := pe.bundle.ApplicationConfig()
	if !exists {
		return ""
	}

	pm := app.APIPolicyMapper()
	if pm == nil {
		return ""
	}

	return pm.PolicyRefForAPI(resName)
}

func (pe *policyEvaluatorImpl) Evaluate(polName string, sd []*common.SignedData) error {
	policy, ok := pe.bundle.PolicyManager().GetPolicy(polName)
	if !ok {
		return PolicyNotFound(polName)
	}

	return policy.Evaluate(sd)
}

//------ resourcePolicyProvider ----------

//aclmgmtPolicyProvider is the interface implemented by resource based ACL.
type aclmgmtPolicyProvider interface {
	//GetPolicyName returns policy name given resource name
	GetPolicyName(resName string) string

	//CheckACL backs ACLProvider interface
	CheckACL(polName string, idinfo interface{}) error
}

//aclmgmtPolicyProviderImpl holds the bytes from state of the ledger
type aclmgmtPolicyProviderImpl struct {
	pEvaluator policyEvaluator
}

//GetPolicyName returns the policy name given the resource string
func (rp *aclmgmtPolicyProviderImpl) GetPolicyName(resName string) string {
	return rp.pEvaluator.PolicyRefForAPI(resName)
}

//CheckACL implements AClProvider's CheckACL interface so it can be registered
//as a provider with aclmgmt
func (rp *aclmgmtPolicyProviderImpl) CheckACL(polName string, idinfo interface{}) error {
	aclLogger.Debugf("acl check(%s)", polName)

	//we will implement other identifiers. In the end we just need a SignedData
	var sd []*common.SignedData
	var err error
	switch idinfo.(type) {
	case *pb.SignedProposal:
		signedProp, _ := idinfo.(*pb.SignedProposal)
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

		sd = []*common.SignedData{{
			Data:      signedProp.ProposalBytes,
			Identity:  shdr.Creator,
			Signature: signedProp.Signature,
		}}
	case *common.Envelope:
		sd, err = idinfo.(*common.Envelope).AsSignedData()
		if err != nil {
			return err
		}
	default:
		return InvalidIdInfo(polName)
	}

	err = rp.pEvaluator.Evaluate(polName, sd)
	if err != nil {
		return fmt.Errorf("failed evaluating policy on signed data during check policy [%s]: [%s]", polName, err)
	}

	return nil
}

//-------- resource provider - entry point API used by aclmgmtimpl for doing resource based ACL ----------

//resource getter gets channelconfig.Resources given channel ID
type ResourceGetter func(channelID string) channelconfig.Resources

//resource provider that uses the resource configuration information to provide ACL support
type resourceProvider struct {
	//resource getter
	resGetter ResourceGetter

	//default provider to be used for undefined resources
	defaultProvider ACLProvider
}

//create a new resourceProvider
func newResourceProvider(rg ResourceGetter, defprov ACLProvider) *resourceProvider {
	return &resourceProvider{rg, defprov}
}

//CheckACL implements the ACL
func (rp *resourceProvider) CheckACL(resName string, channelID string, idinfo interface{}) error {
	resCfg := rp.resGetter(channelID)

	if resCfg != nil {
		pp := &aclmgmtPolicyProviderImpl{&policyEvaluatorImpl{resCfg}}
		policyName := pp.GetPolicyName(resName)
		if policyName != "" {
			aclLogger.Debugf("acl policy %s found in config for resource %s", policyName, resName)
			return pp.CheckACL(policyName, idinfo)
		}
		aclLogger.Debugf("acl policy not found in config for resource %s", resName)
	}

	return rp.defaultProvider.CheckACL(resName, channelID, idinfo)
}
