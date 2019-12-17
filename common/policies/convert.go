/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	"fmt"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
)

// remap explores the policy tree depth first and remaps the "signed by"
// entries according to the remapping rules; a "signed by" rule requires
// a signature from a principal given its position in the array of principals;
// the idRemap map tells us how to remap these integers given that merging two
// policies implies deduplicating their principals
func remap(sp *cb.SignaturePolicy, idRemap map[int]int) *cb.SignaturePolicy {
	switch t := sp.Type.(type) {
	case *cb.SignaturePolicy_NOutOf_:
		rules := []*cb.SignaturePolicy{}
		for _, rule := range t.NOutOf.Rules {
			// here we call remap again - we're doing a
			// depth-first traversal of this policy tree
			rules = append(rules, remap(rule, idRemap))
		}

		return &cb.SignaturePolicy{
			Type: &cb.SignaturePolicy_NOutOf_{
				NOutOf: &cb.SignaturePolicy_NOutOf{
					N:     t.NOutOf.N,
					Rules: rules,
				},
			},
		}
	case *cb.SignaturePolicy_SignedBy:
		// here we do the actual remapping because we have
		// the "signed by" rule, whose reference to the
		// principal we need to remap
		newID, in := idRemap[int(t.SignedBy)]
		if !in {
			panic("programming error")
		}

		return &cb.SignaturePolicy{
			Type: &cb.SignaturePolicy_SignedBy{
				SignedBy: int32(newID),
			},
		}
	default:
		panic(fmt.Sprintf("invalid policy type %T", t))
	}
}

// merge integrates the policy `that` into the
// policy `this`. The first argument is changed
// whereas the second isn't
func merge(this *cb.SignaturePolicyEnvelope, that *cb.SignaturePolicyEnvelope) {
	// at first we build a map of principals in `this`
	IDs := this.Identities
	idMap := map[string]int{}
	for i, id := range this.Identities {
		str := id.PrincipalClassification.String() + string(id.Principal)
		idMap[str] = i
	}

	// then we traverse each of the principals in `that`,
	// deduplicate them against the ones in `this` and
	// create remapping rules so that if `that` references
	// a duplicate policy in this, the merged policy will
	// ensure that the references in `that` point to the
	// correct principal
	idRemap := map[int]int{}
	for i, id := range that.Identities {
		str := id.PrincipalClassification.String() + string(id.Principal)
		if j, in := idMap[str]; in {
			idRemap[i] = j
		} else {
			idRemap[i] = len(IDs)
			idMap[str] = len(IDs)
			IDs = append(IDs, id)
		}
	}

	this.Identities = IDs

	newEntry := remap(that.Rule, idRemap)

	existingRules := this.Rule.Type.(*cb.SignaturePolicy_NOutOf_).NOutOf.Rules
	this.Rule.Type.(*cb.SignaturePolicy_NOutOf_).NOutOf.Rules = append(existingRules, newEntry)
}

// Convert implements the policies.Converter function to
// convert an implicit meta policy into a signature policy envelope.
func (p *ImplicitMetaPolicy) Convert() (*cb.SignaturePolicyEnvelope, error) {
	converted := &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: &cb.SignaturePolicy{
			Type: &cb.SignaturePolicy_NOutOf_{
				NOutOf: &cb.SignaturePolicy_NOutOf{
					N: int32(p.Threshold),
				},
			},
		},
	}

	// the conversion approach for an implicit meta
	// policy is to convert each of the subpolicies,
	// merge it with the previous one and return the
	// merged policy
	for i, subPolicy := range p.SubPolicies {
		convertibleSubpolicy, ok := subPolicy.(Converter)
		if !ok {
			return nil, errors.Errorf("subpolicy number %d type %T of policy %s is not convertible", i, subPolicy, p.SubPolicyName)
		}

		spe, err := convertibleSubpolicy.Convert()
		if err != nil {
			return nil, errors.WithMessagef(err, "failed to convert subpolicy number %d of policy %s", i, p.SubPolicyName)
		}

		merge(converted, spe)
	}

	return converted, nil
}
