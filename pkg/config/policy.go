/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/policydsl"
)

// GetPoliciesForConsortiums returns a map of policies for channel consortiums.
func GetPoliciesForConsortiums(config cb.Config) (map[string]*Policy, error) {
	consortiums, ok := config.ChannelGroup.Groups[ConsortiumsGroupKey]
	if !ok {
		return nil, errors.New("consortiums missing from config")
	}

	return getPolicies(consortiums.Policies)
}

// GetPoliciesForConsortium returns a map of policies for a specific consortium.
func GetPoliciesForConsortium(config cb.Config, consortiumName string) (map[string]*Policy, error) {
	consortium, ok := config.ChannelGroup.Groups[ConsortiumsGroupKey].Groups[consortiumName]
	if !ok {
		return nil, fmt.Errorf("consortium %s does not exist in channel config", consortiumName)
	}

	return getPolicies(consortium.Policies)
}

// GetPoliciesForConsortiumOrg returns a map of policies for a specific consortium org.
func GetPoliciesForConsortiumOrg(config cb.Config, consortiumName, orgName string) (map[string]*Policy, error) {
	org, ok := config.ChannelGroup.Groups[ConsortiumsGroupKey].Groups[consortiumName].Groups[orgName]
	if !ok {
		return nil, fmt.Errorf("consortium org %s does not exist in channel config", orgName)
	}

	return getPolicies(org.Policies)
}

// GetPoliciesForOrderer returns a map of policies for channel orderer.
func GetPoliciesForOrderer(config cb.Config) (map[string]*Policy, error) {
	orderer, ok := config.ChannelGroup.Groups[OrdererGroupKey]
	if !ok {
		return nil, errors.New("orderer missing from config")
	}

	return getPolicies(orderer.Policies)
}

// GetPoliciesForOrdererOrg returns a map of policies for a specific consortium org.
func GetPoliciesForOrdererOrg(config cb.Config, consortiumName, orgName string) (map[string]*Policy, error) {
	org, ok := config.ChannelGroup.Groups[OrdererGroupKey].Groups[orgName]
	if !ok {
		return nil, fmt.Errorf("orderer org %s does not exist in channel config", orgName)
	}

	return getPolicies(org.Policies)
}

// GetPoliciesForApplication returns a map of policies for application config group.
func GetPoliciesForApplication(config cb.Config) (map[string]*Policy, error) {
	application, ok := config.ChannelGroup.Groups[ApplicationGroupKey]
	if !ok {
		return nil, errors.New("application missing from config")
	}

	return getPolicies(application.Policies)
}

// GetPoliciesForApplicationOrg returns a map of policies for specific application
// organization.
func GetPoliciesForApplicationOrg(config cb.Config, orgName string) (map[string]*Policy, error) {
	orgGroup, ok := config.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]
	if !ok {
		return nil, fmt.Errorf("application org %s does not exist in channel config", orgName)
	}

	return getPolicies(orgGroup.Policies)
}

// getPolicies returns a map of Policy from given map of ConfigPolicy in organization config group.
func getPolicies(policies map[string]*cb.ConfigPolicy) (map[string]*Policy, error) {
	if policies == nil {
		return map[string]*Policy{}, nil
	}

	p := map[string]*Policy{}
	for name, policy := range policies {
		switch policy.Policy.Type {
		case int32(cb.Policy_IMPLICIT_META):
			imp := &cb.ImplicitMetaPolicy{}
			err := proto.Unmarshal(policy.Policy.Value, imp)
			if err != nil {
				return nil, err
			}
			rule, err := implicitMetaToString(imp)
			if err != nil {
				return nil, err
			}

			p[name] = &Policy{
				Type: ImplicitMetaPolicyType,
				Rule: rule,
			}

		case int32(cb.Policy_SIGNATURE):
			sp := &cb.SignaturePolicyEnvelope{}
			err := proto.Unmarshal(policy.Policy.Value, sp)
			if err != nil {
				return nil, err
			}
			rule, err := signatureMetaToString(sp)
			if err != nil {
				return nil, err
			}

			p[name] = &Policy{
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
	args := []string{}

	switch imp.Rule {
	case cb.ImplicitMetaPolicy_ANY:
		args = append(args, cb.ImplicitMetaPolicy_ANY.String())
	case cb.ImplicitMetaPolicy_ALL:
		args = append(args, cb.ImplicitMetaPolicy_ALL.String())
	case cb.ImplicitMetaPolicy_MAJORITY:
		args = append(args, cb.ImplicitMetaPolicy_MAJORITY.String())
	default:
		return "", fmt.Errorf("unknown implicit meta policy rule type %v", imp.Rule)
	}

	args = append(args, imp.SubPolicy)

	return strings.Join(args, " "), nil
}

// signatureMetaToString converts a *cb.SignaturePolicyEnvelope to a string representation.
func signatureMetaToString(sig *cb.SignaturePolicyEnvelope) (string, error) {
	roles := []string{}
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
	var res strings.Builder

	switch principal.PrincipalClassification {
	case mb.MSPPrincipal_ROLE:
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
		policies := []string{}
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
