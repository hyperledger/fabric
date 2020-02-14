/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
)

// Consortium represents a group of organizations which may create channels
// with each other.
type Consortium struct {
	Organizations []*Organization
}

// NewConsortiumsGroup returns the consortiums component of the channel configuration.  This element is only defined for the ordering system channel
// It sets the mod_policy for all elements to "/Channel/Orderer/Admins".
func NewConsortiumsGroup(conf map[string]*Consortium, mspConfig *mb.MSPConfig) (*cb.ConfigGroup, error) {
	var err error

	consortiumsGroup := newConfigGroup()
	consortiumsGroup.ModPolicy = ordererAdminsPolicyName

	// acceptAllPolicy always evaluates to true
	acceptAllPolicy := envelope(nOutOf(0, []*cb.SignaturePolicy{}), [][]byte{})
	// This policy is not referenced anywhere, it is only used as part of the implicit meta policy rule at the channel level, so this setting
	// effectively degrades control of the ordering system channel to the ordering admins
	signaturePolicy, err := signaturePolicy(AdminsPolicyKey, acceptAllPolicy)
	if err != nil {
		return nil, fmt.Errorf("failed to create signature policy: %v", err)
	}
	addPolicy(consortiumsGroup, signaturePolicy, ordererAdminsPolicyName)

	for consortiumName, consortium := range conf {
		consortiumsGroup.Groups[consortiumName], err = newConsortiumGroup(consortium, mspConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create consortium group: %v", err)
		}
	}

	return consortiumsGroup, nil
}

// newConsortiumGroup returns a consortiums component of the channel configuration.
func newConsortiumGroup(conf *Consortium, mspConfig *mb.MSPConfig) (*cb.ConfigGroup, error) {
	var err error

	consortiumGroup := newConfigGroup()
	consortiumGroup.ModPolicy = ordererAdminsPolicyName

	for _, org := range conf.Organizations {
		consortiumGroup.Groups[org.Name], err = newConsortiumOrgGroup(org, mspConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create consortium org group %s: %v", org.Name, err)
		}
	}

	implicitMetaAnyPolicy, err := implicitMetaAnyPolicy(AdminsPolicyKey)
	if err != nil {
		return nil, err
	}

	err = addValue(consortiumGroup, channelCreationPolicyValue(implicitMetaAnyPolicy.value), ordererAdminsPolicyName)
	if err != nil {
		return nil, fmt.Errorf("failed to add channel creation policy value: %v", err)
	}

	return consortiumGroup, nil
}

// newConsortiumOrgGroup returns an org component of the channel configuration.
// It defines the crypto material for the organization (its MSP).
// It sets the mod_policy of all elements to "Admins".
func newConsortiumOrgGroup(conf *Organization, mspConfig *mb.MSPConfig) (*cb.ConfigGroup, error) {
	var err error

	consortiumOrgGroup := newConfigGroup()
	consortiumOrgGroup.ModPolicy = AdminsPolicyKey

	if conf.SkipAsForeign {
		return consortiumOrgGroup, nil
	}

	if err = addPolicies(consortiumOrgGroup, conf.Policies, AdminsPolicyKey); err != nil {
		return nil, fmt.Errorf("failed to add policies: %v", err)
	}

	err = addValue(consortiumOrgGroup, mspValue(mspConfig), AdminsPolicyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to add msp value: %v", err)
	}

	return consortiumOrgGroup, nil
}

// consortiumValue returns the config definition for the consortium name
// It is a value for the channel group.
func consortiumValue(name string) *standardConfigValue {
	return &standardConfigValue{
		key: ConsortiumKey,
		value: &cb.Consortium{
			Name: name,
		},
	}
}

// channelCreationPolicyValue returns the config definition for a consortium's channel creation policy
// It is a value for the /Channel/Consortiums/*/*.
func channelCreationPolicyValue(policy *cb.Policy) *standardConfigValue {
	return &standardConfigValue{
		key:   ChannelCreationPolicyKey,
		value: policy,
	}
}

// envelope builds an envelope message embedding a SignaturePolicy.
func envelope(policy *cb.SignaturePolicy, identities [][]byte) *cb.SignaturePolicyEnvelope {
	ids := make([]*mb.MSPPrincipal, len(identities))
	for i := range ids {
		ids[i] = &mb.MSPPrincipal{PrincipalClassification: mb.MSPPrincipal_IDENTITY, Principal: identities[i]}
	}

	return &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       policy,
		Identities: ids,
	}
}

// nOutOf creates a policy which requires N out of the slice of policies to evaluate to true.
func nOutOf(n int32, policies []*cb.SignaturePolicy) *cb.SignaturePolicy {
	return &cb.SignaturePolicy{
		Type: &cb.SignaturePolicy_NOutOf_{
			NOutOf: &cb.SignaturePolicy_NOutOf{
				N:     n,
				Rules: policies,
			},
		},
	}
}

// addPolicy adds a *cb.ConfigPolicy to the passed *cb.ConfigGroup's Policies map.
func addPolicy(cg *cb.ConfigGroup, policy *standardConfigPolicy, modPolicy string) {
	cg.Policies[policy.key] = &cb.ConfigPolicy{
		Policy:    policy.value,
		ModPolicy: modPolicy,
	}
}

// signaturePolicy defines a policy with key policyName and the given signature policy.
func signaturePolicy(policyName string, sigPolicy *cb.SignaturePolicyEnvelope) (*standardConfigPolicy, error) {
	signaturePolicy, err := proto.Marshal(sigPolicy)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signature policy: %v", err)
	}

	return &standardConfigPolicy{
		key: policyName,
		value: &cb.Policy{
			Type:  int32(cb.Policy_SIGNATURE),
			Value: signaturePolicy,
		},
	}, nil
}
