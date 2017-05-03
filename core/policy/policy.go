/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package policy

import (
	"fmt"

	"errors"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

// PolicyChecker offers methods to check a signed proposal against a specific policy
// defined in a channel or not.
type PolicyChecker interface {
	// CheckPolicy checks that the passed signed proposal is valid with the respect to
	// passed policy on the passed channel.
	// If no channel is passed, CheckPolicyNoChannel is invoked directly.
	CheckPolicy(channelID, policyName string, signedProp *pb.SignedProposal) error

	// CheckPolicyBySignedData checks that the passed signed data is valid with the respect to
	// passed policy on the passed channel.
	// If no channel is passed, the method will fail.
	CheckPolicyBySignedData(channelID, policyName string, sd []*common.SignedData) error

	// CheckPolicyNoChannel checks that the passed signed proposal is valid with the respect to
	// passed policy on the local MSP.
	CheckPolicyNoChannel(policyName string, signedProp *pb.SignedProposal) error
}

type policyChecker struct {
	channelPolicyManagerGetter policies.ChannelPolicyManagerGetter
	localMSP                   msp.IdentityDeserializer
	principalGetter            mgmt.MSPPrincipalGetter
}

// NewPolicyChecker creates a new instance of PolicyChecker
func NewPolicyChecker(channelPolicyManagerGetter policies.ChannelPolicyManagerGetter, localMSP msp.IdentityDeserializer, principalGetter mgmt.MSPPrincipalGetter) PolicyChecker {
	return &policyChecker{channelPolicyManagerGetter, localMSP, principalGetter}
}

// CheckPolicy checks that the passed signed proposal is valid with the respect to
// passed policy on the passed channel.
func (p *policyChecker) CheckPolicy(channelID, policyName string, signedProp *pb.SignedProposal) error {
	if channelID == "" {
		return p.CheckPolicyNoChannel(policyName, signedProp)
	}

	if policyName == "" {
		return fmt.Errorf("Invalid policy name during check policy on channel [%s]. Name must be different from nil.", channelID)
	}

	if signedProp == nil {
		return fmt.Errorf("Invalid signed proposal during check policy on channel [%s] with policy [%s]", channelID, policyName)
	}

	// Get Policy
	policyManager, _ := p.channelPolicyManagerGetter.Manager(channelID)
	if policyManager == nil {
		return fmt.Errorf("Failed to get policy manager for channel [%s]", channelID)
	}

	// Prepare SignedData
	proposal, err := utils.GetProposal(signedProp.ProposalBytes)
	if err != nil {
		return fmt.Errorf("Failing extracting proposal during check policy on channel [%s] with policy [%s]: [%s]", channelID, policyName, err)
	}

	header, err := utils.GetHeader(proposal.Header)
	if err != nil {
		return fmt.Errorf("Failing extracting header during check policy on channel [%s] with policy [%s]: [%s]", channelID, policyName, err)
	}

	shdr, err := utils.GetSignatureHeader(header.SignatureHeader)
	if err != nil {
		return fmt.Errorf("Invalid Proposal's SignatureHeader during check policy on channel [%s] with policy [%s]: [%s]", channelID, policyName, err)
	}

	sd := []*common.SignedData{{
		Data:      signedProp.ProposalBytes,
		Identity:  shdr.Creator,
		Signature: signedProp.Signature,
	}}

	return p.CheckPolicyBySignedData(channelID, policyName, sd)
}

// CheckPolicyNoChannel checks that the passed signed proposal is valid with the respect to
// passed policy on the local MSP.
func (p *policyChecker) CheckPolicyNoChannel(policyName string, signedProp *pb.SignedProposal) error {
	if policyName == "" {
		return errors.New("Invalid policy name during channelless check policy. Name must be different from nil.")
	}

	if signedProp == nil {
		return fmt.Errorf("Invalid signed proposal during channelless check policy with policy [%s]", policyName)
	}

	proposal, err := utils.GetProposal(signedProp.ProposalBytes)
	if err != nil {
		return fmt.Errorf("Failing extracting proposal during channelless check policy with policy [%s]: [%s]", policyName, err)
	}

	header, err := utils.GetHeader(proposal.Header)
	if err != nil {
		return fmt.Errorf("Failing extracting header during channelless check policy with policy [%s]: [%s]", policyName, err)
	}

	shdr, err := utils.GetSignatureHeader(header.SignatureHeader)
	if err != nil {
		return fmt.Errorf("Invalid Proposal's SignatureHeader during channelless check policy with policy [%s]: [%s]", policyName, err)
	}

	// Deserialize proposal's creator with the local MSP
	id, err := p.localMSP.DeserializeIdentity(shdr.Creator)
	if err != nil {
		return fmt.Errorf("Failed deserializing proposal creator during channelless check policy with policy [%s]: [%s]", policyName, err)
	}

	// Load MSPPrincipal for policy
	principal, err := p.principalGetter.Get(policyName)
	if err != nil {
		return fmt.Errorf("Failed getting local MSP principal during channelless check policy with policy [%s]: [%s]", policyName, err)
	}

	// Verify that proposal's creator satisfies the principal
	err = id.SatisfiesPrincipal(principal)
	if err != nil {
		return fmt.Errorf("Failed verifying that proposal's creator satisfies local MSP principal during channelless check policy with policy [%s]: [%s]", policyName, err)
	}

	// Verify the signature
	return id.Verify(signedProp.ProposalBytes, signedProp.Signature)
}

// CheckPolicyBySignedData checks that the passed signed data is valid with the respect to
// passed policy on the passed channel.
func (p *policyChecker) CheckPolicyBySignedData(channelID, policyName string, sd []*common.SignedData) error {
	if channelID == "" {
		return errors.New("Invalid channel ID name during check policy on signed data. Name must be different from nil.")
	}

	if policyName == "" {
		return fmt.Errorf("Invalid policy name during check policy on signed data on channel [%s]. Name must be different from nil.", channelID)
	}

	if sd == nil {
		return fmt.Errorf("Invalid signed data during check policy on channel [%s] with policy [%s]", channelID, policyName)
	}

	// Get Policy
	policyManager, _ := p.channelPolicyManagerGetter.Manager(channelID)
	if policyManager == nil {
		return fmt.Errorf("Failed to get policy manager for channel [%s]", channelID)
	}

	// Recall that get policy always returns a policy object
	policy, _ := policyManager.GetPolicy(policyName)

	// Evaluate the policy
	err := policy.Evaluate(sd)
	if err != nil {
		return fmt.Errorf("Failed evaluating policy on signed data during check policy on channel [%s] with policy [%s]: [%s]", channelID, policyName, err)
	}

	return nil
}

var pcFactory PolicyCheckerFactory

// PolicyCheckerFactory defines a factory interface so
// that the actual implementation can be injected
type PolicyCheckerFactory interface {
	NewPolicyChecker() PolicyChecker
}

// RegisterPolicyCheckerFactory is to be called once to set
// the factory that will be used to obtain instances of PolicyChecker
func RegisterPolicyCheckerFactory(f PolicyCheckerFactory) {
	pcFactory = f
}

// GetPolicyChecker returns instances of PolicyChecker;
// the actual implementation is controlled by the factory that
// is registered via RegisterPolicyCheckerFactory
func GetPolicyChecker() PolicyChecker {
	if pcFactory == nil {
		panic("The factory must be set first via RegisterPolicyCheckerFactory")
	}
	return pcFactory.NewPolicyChecker()
}
