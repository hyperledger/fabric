/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/configtx/internal/policydsl"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
)

// SetConsortiumChannelCreationPolicy sets the ConsortiumChannelCreationPolicy for
// the given configuration Group.
// If the policy already exist in current configuration, its value will be overwritten.
func (c *ConfigTx) SetConsortiumChannelCreationPolicy(consortiumName string, policy Policy) error {
	consortium := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey].Groups[consortiumName]

	imp, err := implicitMetaFromString(policy.Rule)
	if err != nil {
		return fmt.Errorf("invalid implicit meta policy rule '%s': %v", policy.Rule, err)
	}

	implicitMetaPolicy, err := implicitMetaPolicy(imp.SubPolicy, imp.Rule)
	if err != nil {
		return fmt.Errorf("failed to make implicit meta policy: %v", err)
	}

	// update channel creation policy value back to consortium
	if err = setValue(consortium, channelCreationPolicyValue(implicitMetaPolicy), ordererAdminsPolicyName); err != nil {
		return fmt.Errorf("failed to update channel creation policy to consortium %s: %v", consortiumName, err)
	}

	return nil
}

// ConsortiumOrgPolicies returns a map of policies for a specific consortium org.
func (c *ConfigTx) ConsortiumOrgPolicies(consortiumName, orgName string) (map[string]Policy, error) {
	consortium, ok := c.original.ChannelGroup.Groups[ConsortiumsGroupKey].Groups[consortiumName]
	if !ok {
		return nil, fmt.Errorf("consortium %s does not exist in channel config", consortiumName)
	}

	org, ok := consortium.Groups[orgName]
	if !ok {
		return nil, fmt.Errorf("consortium org %s does not exist in channel config", orgName)
	}

	return getPolicies(org.Policies)
}

// OrdererPolicies returns a map of policies for channel orderer.
func (c *ConfigTx) OrdererPolicies() (map[string]Policy, error) {
	orderer, ok := c.original.ChannelGroup.Groups[OrdererGroupKey]
	if !ok {
		return nil, errors.New("orderer missing from config")
	}

	return getPolicies(orderer.Policies)
}

// OrdererOrgPolicies returns a map of policies for a specific org.
func (c *ConfigTx) OrdererOrgPolicies(orgName string) (map[string]Policy, error) {
	org, ok := c.original.ChannelGroup.Groups[OrdererGroupKey].Groups[orgName]
	if !ok {
		return nil, fmt.Errorf("orderer org %s does not exist in channel config", orgName)
	}

	return getPolicies(org.Policies)
}

// ApplicationPolicies returns a map of policies for application config group.
// Retrieval will panic if application group does not exist.
func (c *ConfigTx) ApplicationPolicies() (map[string]Policy, error) {
	application := c.original.ChannelGroup.Groups[ApplicationGroupKey]

	return getPolicies(application.Policies)
}

// ApplicationOrgPolicies returns a map of policies for a specific application
// organization.
// Retrieval will panic if application group or application org group does not exist.
func (c *ConfigTx) ApplicationOrgPolicies(orgName string) (map[string]Policy, error) {
	orgGroup := c.original.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]

	return getPolicies(orgGroup.Policies)
}

// ChannelPolicies returns a map of policies for channel configuration.
func (c *ConfigTx) ChannelPolicies() (map[string]Policy, error) {
	return getPolicies(c.original.ChannelGroup.Policies)
}

// SetApplicationPolicy sets the specified policy in the application group's config policy map.
// If the policy already exist in current configuration, its value will be overwritten.
func (c *ConfigTx) SetApplicationPolicy(modPolicy, policyName string, policy Policy) error {
	err := setPolicy(c.updated.ChannelGroup.Groups[ApplicationGroupKey], modPolicy, policyName, policy)
	if err != nil {
		return fmt.Errorf("failed to set policy '%s': %v", policyName, err)
	}

	return nil
}

// RemoveApplicationPolicy removes an existing policy from an application's configuration.
// Removal will panic if the application group does not exist.
func (c *ConfigTx) RemoveApplicationPolicy(policyName string) error {
	policies, err := c.ApplicationPolicies()
	if err != nil {
		return err
	}

	removePolicy(c.updated.ChannelGroup.Groups[ApplicationGroupKey], policyName, policies)
	return nil
}

// SetApplicationOrgPolicy sets the specified policy in the application org group's config policy map.
// If an Organization policy already exist in current configuration, its value will be overwritten.
func (c *ConfigTx) SetApplicationOrgPolicy(orgName, modPolicy, policyName string, policy Policy) error {
	err := setPolicy(c.updated.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName], modPolicy, policyName, policy)
	if err != nil {
		return fmt.Errorf("failed to set policy '%s': %v", policyName, err)
	}

	return nil
}

// RemoveApplicationOrgPolicy removes an existing policy from an application organization.
// Removal will panic if either the application group or application org group does not exist.
func (c *ConfigTx) RemoveApplicationOrgPolicy(orgName, policyName string) error {
	policies, err := c.ApplicationOrgPolicies(orgName)
	if err != nil {
		return err
	}

	removePolicy(c.updated.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName], policyName, policies)
	return nil
}

// SetConsortiumOrgPolicy sets the specified policy in the consortium org group's config policy map.
// If the policy already exist in current configuration, its value will be overwritten.
func (c *ConfigTx) SetConsortiumOrgPolicy(consortiumName, orgName, policyName string, policy Policy) error {
	orgGroup := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey].Groups[consortiumName].Groups[orgName]

	err := setPolicy(orgGroup, AdminsPolicyKey, policyName, policy)
	if err != nil {
		return fmt.Errorf("failed to set policy '%s' to consortium org '%s': %v", policyName, orgName, err)
	}

	return nil
}

// RemoveConsortiumOrgPolicy removes an existing policy from a consortium's organization.
// Removal will panic if either the consortiums group, consortium group, or consortium org group does not exist.
func (c *ConfigTx) RemoveConsortiumOrgPolicy(consortiumName, orgName, policyName string) {
	orgGroup := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey].Groups[consortiumName].Groups[orgName]

	delete(orgGroup.Policies, policyName)

}

// SetOrdererPolicy sets the specified policy in the orderer group's config policy map.
// If the policy already exist in current configuration, its value will be overwritten.
func (c *ConfigTx) SetOrdererPolicy(modPolicy, policyName string, policy Policy) error {
	err := setPolicy(c.updated.ChannelGroup.Groups[OrdererGroupKey], modPolicy, policyName, policy)
	if err != nil {
		return fmt.Errorf("failed to set policy '%s': %v", policyName, err)
	}

	return nil
}

// RemoveOrdererPolicy removes an existing orderer policy configuration.
func (c *ConfigTx) RemoveOrdererPolicy(policyName string) error {
	if policyName == BlockValidationPolicyKey {
		return errors.New("BlockValidation policy must be defined")
	}

	policies, err := c.OrdererPolicies()
	if err != nil {
		return err
	}

	removePolicy(c.updated.ChannelGroup.Groups[OrdererGroupKey], policyName, policies)
	return nil
}

// SetOrdererOrgPolicy sets the specified policy in the orderer org group's config policy map.
// If the policy already exist in current configuration, its value will be overwritten.
func (c *ConfigTx) SetOrdererOrgPolicy(orgName, modPolicy, policyName string, policy Policy) error {
	return setPolicy(c.updated.ChannelGroup.Groups[OrdererGroupKey].Groups[orgName], modPolicy, policyName, policy)
}

// RemoveOrdererOrgPolicy removes an existing policy from an orderer organization.
func (c *ConfigTx) RemoveOrdererOrgPolicy(orgName, policyName string) error {
	policies, err := c.OrdererOrgPolicies(orgName)
	if err != nil {
		return err
	}

	removePolicy(c.updated.ChannelGroup.Groups[OrdererGroupKey].Groups[orgName], policyName, policies)
	return nil
}

// SetChannelPolicy sets the specified policy in the channel group's config policy map.
// If the policy already exist in current configuration, its value will be overwritten.
func (c *ConfigTx) SetChannelPolicy(modPolicy, policyName string, policy Policy) error {
	return setPolicy(c.updated.ChannelGroup, modPolicy, policyName, policy)
}

// RemoveChannelPolicy removes an existing channel level policy.
func (c *ConfigTx) RemoveChannelPolicy(policyName string) error {
	policies, err := c.ChannelPolicies()
	if err != nil {
		return err
	}

	removePolicy(c.updated.ChannelGroup, policyName, policies)
	return nil
}

// getPolicies returns a map of Policy from given map of ConfigPolicy in organization config group.
func getPolicies(policies map[string]*cb.ConfigPolicy) (map[string]Policy, error) {
	p := map[string]Policy{}

	for name, policy := range policies {
		switch cb.Policy_PolicyType(policy.Policy.Type) {
		case cb.Policy_IMPLICIT_META:
			imp := &cb.ImplicitMetaPolicy{}
			err := proto.Unmarshal(policy.Policy.Value, imp)
			if err != nil {
				return nil, err
			}

			rule, err := implicitMetaToString(imp)
			if err != nil {
				return nil, err
			}

			p[name] = Policy{
				Type: ImplicitMetaPolicyType,
				Rule: rule,
			}
		case cb.Policy_SIGNATURE:
			sp := &cb.SignaturePolicyEnvelope{}
			err := proto.Unmarshal(policy.Policy.Value, sp)
			if err != nil {
				return nil, err
			}

			rule, err := signatureMetaToString(sp)
			if err != nil {
				return nil, err
			}

			p[name] = Policy{
				Type: SignaturePolicyType,
				Rule: rule,
			}
		default:
			return nil, fmt.Errorf("unknown policy type: %v", policy.Policy.Type)
		}
	}

	return p, nil
}

// implicitMetaToString converts a *cb.ImplicitMetaPolicy to a string representation.
func implicitMetaToString(imp *cb.ImplicitMetaPolicy) (string, error) {
	var args string

	switch imp.Rule {
	case cb.ImplicitMetaPolicy_ANY:
		args += cb.ImplicitMetaPolicy_ANY.String()
	case cb.ImplicitMetaPolicy_ALL:
		args += cb.ImplicitMetaPolicy_ALL.String()
	case cb.ImplicitMetaPolicy_MAJORITY:
		args += cb.ImplicitMetaPolicy_MAJORITY.String()
	default:
		return "", fmt.Errorf("unknown implicit meta policy rule type %v", imp.Rule)
	}

	args = args + " " + imp.SubPolicy

	return args, nil
}

// signatureMetaToString converts a *cb.SignaturePolicyEnvelope to a string representation.
func signatureMetaToString(sig *cb.SignaturePolicyEnvelope) (string, error) {
	var roles []string

	for _, id := range sig.Identities {
		role, err := mspPrincipalToString(id)
		if err != nil {
			return "", err
		}

		roles = append(roles, role)
	}

	return signaturePolicyToString(sig.Rule, roles)
}

// mspPrincipalToString converts a *mb.MSPPrincipal to a string representation.
func mspPrincipalToString(principal *mb.MSPPrincipal) (string, error) {
	switch principal.PrincipalClassification {
	case mb.MSPPrincipal_ROLE:
		var res strings.Builder

		role := &mb.MSPRole{}

		err := proto.Unmarshal(principal.Principal, role)
		if err != nil {
			return "", err
		}

		res.WriteString("'")
		res.WriteString(role.MspIdentifier)
		res.WriteString(".")
		res.WriteString(strings.ToLower(role.Role.String()))
		res.WriteString("'")

		return res.String(), nil
		// TODO: currently fabric only support string to principle convertion for
		// type ROLE. Implement MSPPrinciple to String for types ORGANIZATION_UNIT,
		// IDENTITY, ANONYMITY, and GOMBINED once we have support from fabric.
	case mb.MSPPrincipal_ORGANIZATION_UNIT:
		return "", nil
	case mb.MSPPrincipal_IDENTITY:
		return "", nil
	case mb.MSPPrincipal_ANONYMITY:
		return "", nil
	case mb.MSPPrincipal_COMBINED:
		return "", nil
	default:
		return "", fmt.Errorf("unknown MSP principal classiciation %v", principal.PrincipalClassification)
	}
}

// signaturePolicyToString recursively converts a *cb.SignaturePolicy to a
// string representation.
func signaturePolicyToString(sig *cb.SignaturePolicy, IDs []string) (string, error) {
	switch sig.Type.(type) {
	case *cb.SignaturePolicy_NOutOf_:
		nOutOf := sig.GetNOutOf()

		var policies []string

		var res strings.Builder

		// get gate values
		gate := policydsl.GateOutOf
		if nOutOf.N == 1 {
			gate = policydsl.GateOr
		}

		if nOutOf.N == int32(len(nOutOf.Rules)) {
			gate = policydsl.GateAnd
		}

		if gate == policydsl.GateOutOf {
			policies = append(policies, strconv.Itoa(int(nOutOf.N)))
		}

		// get subpolicies recursively
		for _, rule := range nOutOf.Rules {
			subPolicy, err := signaturePolicyToString(rule, IDs)
			if err != nil {
				return "", err
			}

			policies = append(policies, subPolicy)
		}

		res.WriteString(strings.ToUpper(gate))
		res.WriteString("(")
		res.WriteString(strings.Join(policies, ", "))
		res.WriteString(")")

		return res.String(), nil
	case *cb.SignaturePolicy_SignedBy:
		return IDs[sig.GetSignedBy()], nil
	default:
		return "", fmt.Errorf("unknown signature policy type %v", sig.Type)
	}
}

// TODO: evaluate if modPolicy actually needs to be passed in if all callers pass AdminsPolicyKey.
func setPolicies(cg *cb.ConfigGroup, policyMap map[string]Policy, modPolicy string) error {
	if policyMap == nil {
		return errors.New("no policies defined")
	}

	if _, ok := policyMap[AdminsPolicyKey]; !ok {
		return errors.New("no Admins policy defined")
	}

	if _, ok := policyMap[ReadersPolicyKey]; !ok {
		return errors.New("no Readers policy defined")
	}

	if _, ok := policyMap[WritersPolicyKey]; !ok {
		return errors.New("no Writers policy defined")
	}

	for policyName, policy := range policyMap {
		err := setPolicy(cg, modPolicy, policyName, policy)
		if err != nil {
			return err
		}
	}

	return nil
}

func setPolicy(cg *cb.ConfigGroup, modPolicy, policyName string, policy Policy) error {
	if cg.Policies == nil {
		cg.Policies = make(map[string]*cb.ConfigPolicy)
	}

	switch policy.Type {
	case ImplicitMetaPolicyType:
		imp, err := implicitMetaFromString(policy.Rule)
		if err != nil {
			return fmt.Errorf("invalid implicit meta policy rule: '%s': %v", policy.Rule, err)
		}

		implicitMetaPolicy, err := proto.Marshal(imp)
		if err != nil {
			return fmt.Errorf("marshaling implicit meta policy: %v", err)
		}

		cg.Policies[policyName] = &cb.ConfigPolicy{
			ModPolicy: modPolicy,
			Policy: &cb.Policy{
				Type:  int32(cb.Policy_IMPLICIT_META),
				Value: implicitMetaPolicy,
			},
		}
	case SignaturePolicyType:
		sp, err := policydsl.FromString(policy.Rule)
		if err != nil {
			return fmt.Errorf("invalid signature policy rule: '%s': %v", policy.Rule, err)
		}

		signaturePolicy, err := proto.Marshal(sp)
		if err != nil {
			return fmt.Errorf("marshaling signature policy: %v", err)
		}

		cg.Policies[policyName] = &cb.ConfigPolicy{
			ModPolicy: modPolicy,
			Policy: &cb.Policy{
				Type:  int32(cb.Policy_SIGNATURE),
				Value: signaturePolicy,
			},
		}
	default:
		return fmt.Errorf("unknown policy type: %s", policy.Type)
	}

	return nil
}

// removePolicy removes an existing policy from an group key organization.
func removePolicy(configGroup *cb.ConfigGroup, policyName string, policies map[string]Policy) {
	delete(configGroup.Policies, policyName)
}
