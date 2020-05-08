/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
)

// Consortium is a group of non-orderer organizations used in channel transactions.
type Consortium struct {
	Name          string
	Organizations []Organization
}

// SetConsortium sets the consortium in a channel configuration.
// If the consortium already exists in the current configuration, its value will be overwritten.
func (c *ConfigTx) SetConsortium(consortium Consortium) error {
	consortiumsGroup := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey]

	consortiumsGroup.Groups[consortium.Name] = newConfigGroup()

	for _, org := range consortium.Organizations {
		err := c.SetConsortiumOrg(org, consortium.Name)
		if err != nil {
			return err
		}
	}

	return nil
}

// RemoveConsortium removes a consortium from a channel configuration.
// Removal will panic if the consortiums group does not exist.
func (c *ConfigTx) RemoveConsortium(consortiumName string) {
	consortiumGroup := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey].Groups

	delete(consortiumGroup, consortiumName)
}

// SetConsortiumOrg sets the organization config group for the given org key in a existing
// Consortium configuration's Groups map.
// If the consortium org already exists in the current configuration, its value will be overwritten.
func (c *ConfigTx) SetConsortiumOrg(org Organization, consortium string) error {
	var err error

	if consortium == "" {
		return errors.New("consortium is required")
	}

	consortiumGroup := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey].Groups[consortium]

	consortiumGroup.Groups[org.Name], err = newOrgConfigGroup(org)
	if err != nil {
		return fmt.Errorf("failed to create consortium org: %v", err)
	}

	return nil
}

// Consortiums returns a list of consortiums for the channel configuration.
// Consortiums is only defined for the ordering system channel.
func (c *ConfigTx) Consortiums() ([]Consortium, error) {
	consortiumsGroup, ok := c.original.ChannelGroup.Groups[ConsortiumsGroupKey]
	if !ok {
		return nil, errors.New("channel configuration does not have consortiums")
	}

	consortiums := []Consortium{}
	for consortiumName, consortiumGroup := range consortiumsGroup.Groups {
		consortium, err := consortium(consortiumGroup, consortiumName)
		if err != nil {
			return nil, err
		}
		consortiums = append(consortiums, consortium)
	}

	return consortiums, nil
}

// consortium returns a value of consortium from the given consortium configGroup
func consortium(consortiumGroup *cb.ConfigGroup, consortiumName string) (Consortium, error) {
	orgs := []Organization{}
	for orgName, orgGroup := range consortiumGroup.Groups {
		org, err := getOrganization(orgGroup, orgName)
		if err != nil {
			return Consortium{}, fmt.Errorf("failed to retrieve organization %s from consortium %s: ", orgName, consortiumName)
		}
		orgs = append(orgs, org)
	}
	return Consortium{
		Name:          consortiumName,
		Organizations: orgs,
	}, nil
}

// newConsortiumsGroup returns the consortiums component of the channel configuration. This element is only defined for
// the ordering system channel.
// It sets the mod_policy for all elements to "/Channel/Orderer/Admins".
func newConsortiumsGroup(consortiums []Consortium) (*cb.ConfigGroup, error) {
	var err error

	consortiumsGroup := newConfigGroup()
	consortiumsGroup.ModPolicy = ordererAdminsPolicyName

	// acceptAllPolicy always evaluates to true
	acceptAllPolicy := envelope(nOutOf(0, []*cb.SignaturePolicy{}), [][]byte{})

	// This policy is not referenced anywhere, it is only used as part of the implicit meta policy rule at the
	// channel level, so this setting effectively degrades control of the ordering system channel to the ordering admins
	signaturePolicy, err := signaturePolicy(AdminsPolicyKey, acceptAllPolicy)
	if err != nil {
		return nil, err
	}

	consortiumsGroup.Policies[signaturePolicy.key] = &cb.ConfigPolicy{
		Policy:    signaturePolicy.value,
		ModPolicy: ordererAdminsPolicyName,
	}

	for _, consortium := range consortiums {
		consortiumsGroup.Groups[consortium.Name], err = newConsortiumGroup(consortium)
		if err != nil {
			return nil, err
		}
	}

	return consortiumsGroup, nil
}

// newConsortiumGroup returns a consortiums component of the channel configuration.
func newConsortiumGroup(consortium Consortium) (*cb.ConfigGroup, error) {
	var err error

	consortiumGroup := newConfigGroup()
	consortiumGroup.ModPolicy = ordererAdminsPolicyName

	for _, org := range consortium.Organizations {
		consortiumGroup.Groups[org.Name], err = newOrgConfigGroup(org)
		if err != nil {
			return nil, fmt.Errorf("org group '%s': %v", org.Name, err)
		}
	}

	implicitMetaAnyPolicy, err := implicitMetaAnyPolicy(AdminsPolicyKey)
	if err != nil {
		return nil, err
	}

	err = setValue(consortiumGroup, channelCreationPolicyValue(implicitMetaAnyPolicy.value), ordererAdminsPolicyName)
	if err != nil {
		return nil, err
	}

	return consortiumGroup, nil
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

// signaturePolicy defines a policy with key policyName and the given signature policy.
func signaturePolicy(policyName string, sigPolicy *cb.SignaturePolicyEnvelope) (*standardConfigPolicy, error) {
	signaturePolicy, err := proto.Marshal(sigPolicy)
	if err != nil {
		return nil, fmt.Errorf("marshaling signature policy: %v", err)
	}

	return &standardConfigPolicy{
		key: policyName,
		value: &cb.Policy{
			Type:  int32(cb.Policy_SIGNATURE),
			Value: signaturePolicy,
		},
	}, nil
}

// implicitMetaPolicy creates a new *cb.Policy of cb.Policy_IMPLICIT_META type.
func implicitMetaPolicy(subPolicyName string, rule cb.ImplicitMetaPolicy_Rule) (*cb.Policy, error) {
	implicitMetaPolicy, err := proto.Marshal(&cb.ImplicitMetaPolicy{
		Rule:      rule,
		SubPolicy: subPolicyName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal implicit meta policy: %v", err)
	}

	return &cb.Policy{
		Type:  int32(cb.Policy_IMPLICIT_META),
		Value: implicitMetaPolicy,
	}, nil
}

// implicitMetaAnyPolicy defines an implicit meta policy whose sub_policy and key is policyname with rule ANY.
func implicitMetaAnyPolicy(policyName string) (*standardConfigPolicy, error) {
	implicitMetaPolicy, err := implicitMetaPolicy(policyName, cb.ImplicitMetaPolicy_ANY)
	if err != nil {
		return nil, fmt.Errorf("failed to make implicit meta ANY policy: %v", err)
	}

	return &standardConfigPolicy{
		key:   policyName,
		value: implicitMetaPolicy,
	}, nil
}

// getConsortiumOrg returns the organization config group for a consortium org in the
// provided config. It will panic if the consortium doesn't exist, and it
// will return nil if the org doesn't exist in the config.
func getConsortiumOrg(config *cb.Config, consortiumName string, orgName string) *cb.ConfigGroup {
	consortiumsGroup := config.ChannelGroup.Groups[ConsortiumsGroupKey].Groups
	consortiumGroup := consortiumsGroup[consortiumName]
	return consortiumGroup.Groups[orgName]
}
