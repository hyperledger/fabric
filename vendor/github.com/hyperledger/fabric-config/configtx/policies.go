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
				Type:      ImplicitMetaPolicyType,
				Rule:      rule,
				ModPolicy: policy.GetModPolicy(),
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
				Type:      SignaturePolicyType,
				Rule:      rule,
				ModPolicy: policy.GetModPolicy(),
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

func setPolicies(cg *cb.ConfigGroup, policyMap map[string]Policy) error {
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

	cg.Policies = make(map[string]*cb.ConfigPolicy)
	for policyName, policy := range policyMap {
		err := setPolicy(cg, policyName, policy)
		if err != nil {
			return err
		}
	}

	return nil
}

func setPolicy(cg *cb.ConfigGroup, policyName string, policy Policy) error {
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

		if policy.ModPolicy == "" {
			policy.ModPolicy = AdminsPolicyKey
		}

		cg.Policies[policyName] = &cb.ConfigPolicy{
			ModPolicy: policy.ModPolicy,
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

		if policy.ModPolicy == "" {
			policy.ModPolicy = AdminsPolicyKey
		}

		cg.Policies[policyName] = &cb.ConfigPolicy{
			ModPolicy: policy.ModPolicy,
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
