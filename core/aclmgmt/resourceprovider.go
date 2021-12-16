/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"fmt"

	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/protoutil"
)

//--------- errors ---------

// PolicyNotFound cache for resource
type PolicyNotFound string

func (e PolicyNotFound) Error() string {
	return fmt.Sprintf("policy %s not found", string(e))
}

// InvalidIdInfo
type InvalidIdInfo string

func (e InvalidIdInfo) Error() string {
	return fmt.Sprintf("Invalid id for policy [%s]", string(e))
}

//---------- policyEvaluator ------

// policyEvalutor interface provides the interfaces for policy evaluation
type policyEvaluator interface {
	PolicyRefForAPI(resName string) string
	Evaluate(polName string, id []*protoutil.SignedData) error
}

// policyEvaluatorImpl implements policyEvaluator
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

func (pe *policyEvaluatorImpl) Evaluate(polName string, sd []*protoutil.SignedData) error {
	policy, ok := pe.bundle.PolicyManager().GetPolicy(polName)
	if !ok {
		return PolicyNotFound(polName)
	}

	err := policy.EvaluateSignedData(sd)
	if err != nil {
		aclLogger.Warnw("EvaluateSignedData policy check failed", "error", err, "policyName", polName, policy, "policy", "signingIdentities", protoutil.LogMessageForSerializedIdentities(sd))
	}
	return err
}

//------ resourcePolicyProvider ----------

// aclmgmtPolicyProvider is the interface implemented by resource based ACL.
type aclmgmtPolicyProvider interface {
	// GetPolicyName returns policy name given resource name
	GetPolicyName(resName string) string

	// CheckACL backs ACLProvider interface
	CheckACL(polName string, idinfo interface{}) error
}

// aclmgmtPolicyProviderImpl holds the bytes from state of the ledger
type aclmgmtPolicyProviderImpl struct {
	pEvaluator policyEvaluator
}

// GetPolicyName returns the policy name given the resource string
func (rp *aclmgmtPolicyProviderImpl) GetPolicyName(resName string) string {
	return rp.pEvaluator.PolicyRefForAPI(resName)
}

// CheckACL implements AClProvider's CheckACL interface so it can be registered
// as a provider with aclmgmt
func (rp *aclmgmtPolicyProviderImpl) CheckACL(polName string, idinfo interface{}) error {
	aclLogger.Debugf("acl check(%s)", polName)

	// we will implement other identifiers. In the end we just need a SignedData
	var sd []*protoutil.SignedData
	switch idinfo := idinfo.(type) {
	case *pb.SignedProposal:
		signedProp := idinfo
		proposal, err := protoutil.UnmarshalProposal(signedProp.ProposalBytes)
		if err != nil {
			return fmt.Errorf("Failing extracting proposal during check policy with policy [%s]: [%s]", polName, err)
		}

		header, err := protoutil.UnmarshalHeader(proposal.Header)
		if err != nil {
			return fmt.Errorf("Failing extracting header during check policy [%s]: [%s]", polName, err)
		}

		shdr, err := protoutil.UnmarshalSignatureHeader(header.SignatureHeader)
		if err != nil {
			return fmt.Errorf("Invalid Proposal's SignatureHeader during check policy [%s]: [%s]", polName, err)
		}

		sd = []*protoutil.SignedData{{
			Data:      signedProp.ProposalBytes,
			Identity:  shdr.Creator,
			Signature: signedProp.Signature,
		}}

	case *common.Envelope:
		var err error
		sd, err = protoutil.EnvelopeAsSignedData(idinfo)
		if err != nil {
			return err
		}

	case *protoutil.SignedData:
		sd = []*protoutil.SignedData{idinfo}

	default:
		return InvalidIdInfo(polName)
	}

	err := rp.pEvaluator.Evaluate(polName, sd)
	if err != nil {
		return fmt.Errorf("failed evaluating policy on signed data during check policy [%s]: [%s]", polName, err)
	}

	return nil
}

//-------- resource provider - entry point API used by aclmgmtimpl for doing resource based ACL ----------

// resource getter gets channelconfig.Resources given channel ID
type ResourceGetter func(channelID string) channelconfig.Resources

// resource provider that uses the resource configuration information to provide ACL support
type resourceProvider struct {
	// resource getter
	resGetter ResourceGetter

	// default provider to be used for undefined resources
	defaultProvider defaultACLProvider
}

// create a new resourceProvider
func newResourceProvider(rg ResourceGetter, defprov defaultACLProvider) *resourceProvider {
	return &resourceProvider{rg, defprov}
}

func (rp *resourceProvider) enforceDefaultBehavior(resName string, channelID string, idinfo interface{}) bool {
	// we currently enforce using p types if defined.  In future we will allow p types
	// to be overridden through peer configuration
	return rp.defaultProvider.IsPtypePolicy(resName)
}

// CheckACL implements the ACL
func (rp *resourceProvider) CheckACL(resName string, channelID string, idinfo interface{}) error {
	if !rp.enforceDefaultBehavior(resName, channelID, idinfo) {
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
	}

	return rp.defaultProvider.CheckACL(resName, channelID, idinfo)
}

// CheckACLNoChannel implements the ACLProvider interface function
func (rp *resourceProvider) CheckACLNoChannel(resName string, idinfo interface{}) error {
	if !rp.enforceDefaultBehavior(resName, "", idinfo) {
		return fmt.Errorf("cannot override peer type policy for channeless ACL check")
	}

	return rp.defaultProvider.CheckACLNoChannel(resName, idinfo)
}
