/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policy

import (
	"errors"
	"fmt"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
)

var logger = flogging.MustGetLogger("policy")

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
	CheckPolicyBySignedData(channelID, policyName string, sd []*protoutil.SignedData) error

	// CheckPolicyNoChannel checks that the passed signed proposal is valid with the respect to
	// passed policy on the local MSP.
	CheckPolicyNoChannel(policyName string, signedProp *pb.SignedProposal) error

	// CheckPolicyNoChannelBySignedData checks that the passed signed data is valid with the respect to
	// passed policy on the local MSP.
	CheckPolicyNoChannelBySignedData(policyName string, signedData []*protoutil.SignedData) error
}

type policyChecker struct {
	channelPolicyManagerGetter policies.ChannelPolicyManagerGetter
	localMSP                   msp.IdentityDeserializer
	principalGetter            MSPPrincipalGetter
}

// NewPolicyChecker creates a new instance of PolicyChecker
func NewPolicyChecker(channelPolicyManagerGetter policies.ChannelPolicyManagerGetter, localMSP msp.MSP) PolicyChecker {
	return &policyChecker{
		channelPolicyManagerGetter: channelPolicyManagerGetter,
		localMSP:                   localMSP,
		principalGetter:            &localMSPPrincipalGetter{localMSP: localMSP},
	}
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
	policyManager := p.channelPolicyManagerGetter.Manager(channelID)
	if policyManager == nil {
		return fmt.Errorf("Failed to get policy manager for channel [%s]", channelID)
	}

	// Prepare SignedData
	proposal, err := protoutil.UnmarshalProposal(signedProp.ProposalBytes)
	if err != nil {
		return fmt.Errorf("Failing extracting proposal during check policy on channel [%s] with policy [%s]: [%s]", channelID, policyName, err)
	}

	header, err := protoutil.UnmarshalHeader(proposal.Header)
	if err != nil {
		return fmt.Errorf("Failing extracting header during check policy on channel [%s] with policy [%s]: [%s]", channelID, policyName, err)
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(header.SignatureHeader)
	if err != nil {
		return fmt.Errorf("Invalid Proposal's SignatureHeader during check policy on channel [%s] with policy [%s]: [%s]", channelID, policyName, err)
	}

	sd := []*protoutil.SignedData{{
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

	proposal, err := protoutil.UnmarshalProposal(signedProp.ProposalBytes)
	if err != nil {
		return fmt.Errorf("Failing extracting proposal during channelless check policy with policy [%s]: [%s]", policyName, err)
	}

	header, err := protoutil.UnmarshalHeader(proposal.Header)
	if err != nil {
		return fmt.Errorf("Failing extracting header during channelless check policy with policy [%s]: [%s]", policyName, err)
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(header.SignatureHeader)
	if err != nil {
		return fmt.Errorf("Invalid Proposal's SignatureHeader during channelless check policy with policy [%s]: [%s]", policyName, err)
	}

	// Deserialize proposal's creator with the local MSP
	id, err := p.localMSP.DeserializeIdentity(shdr.Creator)
	if err != nil {
		logger.Warnw("Failed deserializing proposal creator during channelless check policy", "error", err, "policyName", policyName, "identity", protoutil.LogMessageForSerializedIdentity(shdr.Creator))
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
		logger.Warnw("Failed verifying that proposal's creator satisfies local MSP principal during channelless check policy", "error", err, "policyName", policyName, "requiredPrincipal", principal, "signingIdentity", protoutil.LogMessageForSerializedIdentity(shdr.Creator))
		return fmt.Errorf("Failed verifying that proposal's creator satisfies local MSP principal during channelless check policy with policy [%s]: [%s]", policyName, err)
	}

	// Verify the signature
	return id.Verify(signedProp.ProposalBytes, signedProp.Signature)
}

// CheckPolicyBySignedData checks that the passed signed data is valid with the respect to
// passed policy on the passed channel.
func (p *policyChecker) CheckPolicyBySignedData(channelID, policyName string, sd []*protoutil.SignedData) error {
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
	policyManager := p.channelPolicyManagerGetter.Manager(channelID)
	if policyManager == nil {
		return fmt.Errorf("Failed to get policy manager for channel [%s]", channelID)
	}

	// Recall that get policy always returns a policy object
	policy, _ := policyManager.GetPolicy(policyName)

	// Evaluate the policy
	err := policy.EvaluateSignedData(sd)
	if err != nil {
		logger.Warnw("Failed evaluating policy on signed data", "error", err, "policyName", policyName, "identities", protoutil.LogMessageForSerializedIdentities(sd))
		return fmt.Errorf("Failed evaluating policy on signed data during check policy on channel [%s] with policy [%s]: [%s]", channelID, policyName, err)
	}

	return nil
}

// CheckPolicyNoChannelBySignedData checks that the passed signed data are valid with the respect to
// passed policy on the local MSP.
func (p *policyChecker) CheckPolicyNoChannelBySignedData(policyName string, signedData []*protoutil.SignedData) error {
	if policyName == "" {
		return errors.New("invalid policy name during channelless check policy. Name must be different from nil.")
	}

	if len(signedData) == 0 {
		return fmt.Errorf("no signed data during channelless check policy with policy [%s]", policyName)
	}

	for _, data := range signedData {
		// Deserialize identity with the local MSP
		id, err := p.localMSP.DeserializeIdentity(data.Identity)
		if err != nil {
			logger.Warnw("Failed deserializing signed data identity during channelless check policy", "error", err, "policyName", policyName, "identity", protoutil.LogMessageForSerializedIdentity(data.Identity))
			return fmt.Errorf("failed deserializing signed data identity during channelless check policy with policy [%s]: [%s]", policyName, err)
		}

		// Load MSPPrincipal for policy
		principal, err := p.principalGetter.Get(policyName)
		if err != nil {
			return fmt.Errorf("failed getting local MSP principal during channelless check policy with policy [%s]: [%s]", policyName, err)
		}

		// Verify that proposal's creator satisfies the principal
		err = id.SatisfiesPrincipal(principal)
		if err != nil {
			logger.Warnw("failed verifying that the signed data identity satisfies local MSP principal during channelless check policy", "error", err, "policyName", policyName, "requiredPrincipal", principal, "identity", protoutil.LogMessageForSerializedIdentity(data.Identity))
			return fmt.Errorf("failed verifying that the signed data identity satisfies local MSP principal during channelless check policy with policy [%s]: [%s]", policyName, err)
		}

		// Verify the signature
		if err = id.Verify(data.Data, data.Signature); err != nil {
			return err
		}
	}

	return nil
}
