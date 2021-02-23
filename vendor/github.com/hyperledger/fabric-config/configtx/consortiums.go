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

// ConsortiumsGroup encapsulates the parts of the config that control consortiums.
type ConsortiumsGroup struct {
	consortiumsGroup *cb.ConfigGroup
}

// ConsortiumGroup encapsulates the parts of the config that control
// a specific consortium. This type implements retrieval of the various
// consortium config values.
type ConsortiumGroup struct {
	consortiumGroup *cb.ConfigGroup
	name            string
}

// ConsortiumOrg encapsulates the parts of the config that control a
// consortium organization's configuration.
type ConsortiumOrg struct {
	orgGroup *cb.ConfigGroup
	name     string
}

// MSP returns an OrganizationMSP object that can be used to configure the organization's MSP.
func (c *ConsortiumOrg) MSP() *OrganizationMSP {
	return &OrganizationMSP{
		configGroup: c.orgGroup,
	}
}

// Consortiums returns the consortiums group from the updated config.
func (c *ConfigTx) Consortiums() *ConsortiumsGroup {
	consortiumsGroup := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey]
	return &ConsortiumsGroup{consortiumsGroup: consortiumsGroup}
}

// Consortium returns a consortium group from the updated config.
func (c *ConfigTx) Consortium(name string) *ConsortiumGroup {
	consortiumGroup, ok := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey].Groups[name]
	if !ok {
		return nil
	}
	return &ConsortiumGroup{name: name, consortiumGroup: consortiumGroup}
}

// SetConsortium sets the consortium in a channel configuration.
// If the consortium already exists in the current configuration, its value will be overwritten.
func (c *ConsortiumsGroup) SetConsortium(consortium Consortium) error {
	c.consortiumsGroup.Groups[consortium.Name] = newConfigGroup()

	for _, org := range consortium.Organizations {
		err := c.consortium(consortium.Name).SetOrganization(org)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *ConsortiumsGroup) consortium(name string) *ConsortiumGroup {
	consortiumGroup := c.consortiumsGroup.Groups[name]
	return &ConsortiumGroup{name: name, consortiumGroup: consortiumGroup}
}

// RemoveConsortium removes a consortium from a channel configuration.
// Removal will panic if the consortiums group does not exist.
func (c *ConsortiumsGroup) RemoveConsortium(name string) {
	delete(c.consortiumsGroup.Groups, name)
}

// Organization returns the consortium org from the original config.
func (c *ConsortiumGroup) Organization(name string) *ConsortiumOrg {
	orgGroup, ok := c.consortiumGroup.Groups[name]
	if !ok {
		return nil
	}
	return &ConsortiumOrg{name: name, orgGroup: orgGroup}
}

// SetOrganization sets the organization config group for the given org key in
// an existing Consortium configuration's Groups map.
// If the consortium org already exists in the current configuration, its
// value will be overwritten.
func (c *ConsortiumGroup) SetOrganization(org Organization) error {
	orgGroup, err := newOrgConfigGroup(org)
	if err != nil {
		return fmt.Errorf("failed to create consortium org %s: %v", org.Name, err)
	}

	c.consortiumGroup.Groups[org.Name] = orgGroup

	return nil
}

// RemoveOrganization removes an org from a consortium group.
// Removal will panic if either the consortiums group or consortium group does not exist.
func (c *ConsortiumGroup) RemoveOrganization(name string) {
	delete(c.consortiumGroup.Groups, name)
}

// Configuration returns a list of consortium configurations from the updated
// config. Consortiums are only defined for the ordering system channel.
func (c *ConsortiumsGroup) Configuration() ([]Consortium, error) {
	consortiums := []Consortium{}
	for consortiumName := range c.consortiumsGroup.Groups {
		consortium, err := c.consortium(consortiumName).Configuration()
		if err != nil {
			return nil, err
		}
		consortiums = append(consortiums, consortium)
	}

	return consortiums, nil
}

// Configuration returns the configuration for a consortium group.
func (c *ConsortiumGroup) Configuration() (Consortium, error) {
	orgs := []Organization{}
	for orgName, orgGroup := range c.consortiumGroup.Groups {
		org, err := getOrganization(orgGroup, orgName)
		if err != nil {
			return Consortium{}, fmt.Errorf("failed to retrieve organization %s from consortium %s: ", orgName, c.name)
		}
		orgs = append(orgs, org)
	}
	return Consortium{
		Name:          c.name,
		Organizations: orgs,
	}, nil
}

// Configuration retrieves an existing org's configuration from a consortium
// organization config group in the updated config.
func (c *ConsortiumOrg) Configuration() (Organization, error) {
	org, err := getOrganization(c.orgGroup, c.name)
	if err != nil {
		return Organization{}, err
	}

	// Remove AnchorPeers which are application org specific.
	org.AnchorPeers = nil

	return org, err
}

// SetMSP updates the MSP config for the specified consortium org group.
func (c *ConsortiumOrg) SetMSP(updatedMSP MSP) error {
	currentMSP, err := c.MSP().Configuration()
	if err != nil {
		return fmt.Errorf("retrieving msp: %v", err)
	}

	if currentMSP.Name != updatedMSP.Name {
		return errors.New("MSP name cannot be changed")
	}

	err = updatedMSP.validateCACerts()
	if err != nil {
		return err
	}

	err = c.setMSPConfig(updatedMSP)
	if err != nil {
		return err
	}

	return nil
}

func (c *ConsortiumOrg) setMSPConfig(updatedMSP MSP) error {
	mspConfig, err := newMSPConfig(updatedMSP)
	if err != nil {
		return fmt.Errorf("new msp config: %v", err)
	}

	err = setValue(c.orgGroup, mspValue(mspConfig), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// SetChannelCreationPolicy sets the ConsortiumChannelCreationPolicy for
// the given configuration Group.
// If the policy already exists in current configuration, its value will be overwritten.
func (c *ConsortiumGroup) SetChannelCreationPolicy(policy Policy) error {
	imp, err := implicitMetaFromString(policy.Rule)
	if err != nil {
		return fmt.Errorf("invalid implicit meta policy rule '%s': %v", policy.Rule, err)
	}

	implicitMetaPolicy, err := implicitMetaPolicy(imp.SubPolicy, imp.Rule)
	if err != nil {
		return fmt.Errorf("failed to make implicit meta policy: %v", err)
	}

	// update channel creation policy value back to consortium
	if err = setValue(c.consortiumGroup, channelCreationPolicyValue(implicitMetaPolicy), ordererAdminsPolicyName); err != nil {
		return fmt.Errorf("failed to update channel creation policy to consortium %s: %v", c.name, err)
	}

	return nil
}

// Policies returns a map of policies for a specific consortium org.
func (c *ConsortiumOrg) Policies() (map[string]Policy, error) {
	return getPolicies(c.orgGroup.Policies)
}

// SetModPolicy sets the specified modification policy for the consortium org group.
func (c *ConsortiumOrg) SetModPolicy(modPolicy string) error {
	if modPolicy == "" {
		return errors.New("non empty mod policy is required")
	}

	c.orgGroup.ModPolicy = modPolicy

	return nil
}

// SetPolicy sets the specified policy in the consortium org group's config policy map.
// If the policy already exists in current configuration, its value will be overwritten.
func (c *ConsortiumOrg) SetPolicy(name string, policy Policy) error {
	err := setPolicy(c.orgGroup, name, policy)
	if err != nil {
		return fmt.Errorf("failed to set policy '%s' to consortium org '%s': %v", name, c.name, err)
	}

	return nil
}

// SetPolicies sets the specified policies in the consortium org group's config policy map.
// If the policies already exist in current configuration, the values will be replaced with new policies.
func (c *ConsortiumOrg) SetPolicies(policies map[string]Policy) error {
	err := setPolicies(c.orgGroup, policies)
	if err != nil {
		return fmt.Errorf("failed to set policies to consortium org '%s': %v", c.name, err)
	}

	return nil
}

// RemovePolicy removes an existing policy from a consortium's organization.
// Removal will panic if either the consortiums group, consortium group, or consortium org group does not exist.
func (c *ConsortiumOrg) RemovePolicy(name string) {
	delete(c.orgGroup.Policies, name)
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
