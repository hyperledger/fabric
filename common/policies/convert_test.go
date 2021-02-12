/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies_test

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestImplicitMetaPolicy_Convert(t *testing.T) {
	// Scenario: we attempt the conversion of a simple metapolicy requiring
	// ALL of 2 sub-policies, each of which are plain signedby

	pfs := &cauthdsl.EnvelopeBasedPolicyProvider{}

	p1, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    policydsl.SignedBy(0),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
		},
	})
	require.NotNil(t, p1)
	require.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    policydsl.SignedBy(0),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
		},
	})
	require.NotNil(t, p2)
	require.NoError(t, err)

	p := &policies.PolicyLogger{
		Policy: &policies.ImplicitMetaPolicy{
			Threshold:     2,
			SubPolicyName: "mypolicy",
			SubPolicies:   []policies.Policy{p1, p2},
		},
	}

	spe, err := p.Convert()
	require.NoError(t, err)
	require.NotNil(t, spe)
	require.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.And(
			policydsl.SignedBy(0),
			policydsl.SignedBy(1),
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
		},
	}, spe)
}

func TestImplicitMetaPolicy_Convert1(t *testing.T) {
	// Scenario: we attempt the conversion of a metapolicy requiring
	// ALL of 2 sub-policies, where the first one is an OR of 2 and
	// the second one is an OR of 1, with a principal that is already
	// referenced by the first

	pfs := &cauthdsl.EnvelopeBasedPolicyProvider{}

	p1, err := pfs.NewPolicy(policydsl.SignedByAnyMember([]string{"A", "B"}))
	require.NotNil(t, p1)
	require.NoError(t, err)

	p2, err := pfs.NewPolicy(policydsl.SignedByAnyMember([]string{"B"}))
	require.NotNil(t, p2)
	require.NoError(t, err)

	p := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{p1, p2},
	}

	spe, err := p.Convert()
	require.NoError(t, err)
	require.NotNil(t, spe)
	require.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.And(
			policydsl.Or(
				policydsl.SignedBy(0),
				policydsl.SignedBy(1),
			),
			policydsl.NOutOf(1,
				[]*cb.SignaturePolicy{policydsl.SignedBy(1)},
			),
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
		},
	}, spe)
}

func TestImplicitMetaPolicy_Convert2(t *testing.T) {
	// Scenario: we attempt the conversion of a metapolicy requiring
	// ALL of 2 sub-policies, where the first one is an OR of an AND
	// of 2 and an OR of 1 and the second one is an AND of 2, with
	// principal deduplication required

	pfs := &cauthdsl.EnvelopeBasedPolicyProvider{}

	p1, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.NOutOf(1,
			[]*cb.SignaturePolicy{
				policydsl.NOutOf(2,
					[]*cb.SignaturePolicy{
						policydsl.SignedBy(0),
						policydsl.SignedBy(1),
					},
				),
				policydsl.NOutOf(1,
					[]*cb.SignaturePolicy{
						policydsl.SignedBy(2),
					},
				),
			},
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "C",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "D",
					},
				),
			},
		},
	})
	require.NotNil(t, p1)
	require.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.NOutOf(2,
			[]*cb.SignaturePolicy{
				policydsl.SignedBy(0),
				policydsl.SignedBy(1),
			},
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
		},
	})
	require.NotNil(t, p2)
	require.NoError(t, err)

	p := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{p1, p2},
	}

	spe, err := p.Convert()
	require.NoError(t, err)
	require.NotNil(t, spe)
	require.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.And(
			policydsl.NOutOf(1,
				[]*cb.SignaturePolicy{
					policydsl.NOutOf(2,
						[]*cb.SignaturePolicy{
							policydsl.SignedBy(0),
							policydsl.SignedBy(1),
						},
					),
					policydsl.NOutOf(1,
						[]*cb.SignaturePolicy{
							policydsl.SignedBy(2),
						},
					),
				},
			),
			policydsl.NOutOf(2,
				[]*cb.SignaturePolicy{
					policydsl.SignedBy(3),
					policydsl.SignedBy(0),
				},
			),
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "C",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "D",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
		},
	}, spe)
}

func TestImplicitMetaPolicy_Convert3(t *testing.T) {
	// Scenario: we attempt the conversion of a metapolicy requiring
	// ALL of 2 sub-policies, where the first one is a metapolicy itself,
	// requiring ALL of 2 simple subpolicies to be satisfied and the
	// second is a simple subpolicy, with no principal deduplication

	pfs := &cauthdsl.EnvelopeBasedPolicyProvider{}

	p1, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    policydsl.SignedBy(0),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
		},
	})
	require.NotNil(t, p1)
	require.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    policydsl.SignedBy(0),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
		},
	})
	require.NotNil(t, p2)
	require.NoError(t, err)

	p3, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    policydsl.SignedBy(0),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "C",
					},
				),
			},
		},
	})
	require.NotNil(t, p3)
	require.NoError(t, err)

	mp1 := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{p1, p2},
	}

	mp2 := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{mp1, p3},
	}

	spe, err := mp2.Convert()
	require.NoError(t, err)
	require.NotNil(t, spe)
	require.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.And(
			policydsl.And(
				policydsl.SignedBy(0),
				policydsl.SignedBy(1),
			),
			policydsl.SignedBy(2),
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "C",
					},
				),
			},
		},
	}, spe)
}

func TestImplicitMetaPolicy_Convert4(t *testing.T) {
	// Scenario: we attempt the conversion of a metapolicy requiring
	// ALL of 2 sub-policies, where the first one is a metapolicy itself,
	// requiring ALL of 2 simple subpolicies to be satisfied and the
	// second is a simple subpolicy, with principal deduplication

	pfs := &cauthdsl.EnvelopeBasedPolicyProvider{}

	p1, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    policydsl.SignedBy(0),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
		},
	})
	require.NotNil(t, p1)
	require.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    policydsl.SignedBy(0),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
		},
	})
	require.NotNil(t, p2)
	require.NoError(t, err)

	p3, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    policydsl.SignedBy(0),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
		},
	})
	require.NotNil(t, p3)
	require.NoError(t, err)

	mp1 := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{p1, p2},
	}

	mp2 := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{mp1, p3},
	}

	spe, err := mp2.Convert()
	require.NoError(t, err)
	require.NotNil(t, spe)
	require.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.And(
			policydsl.And(
				policydsl.SignedBy(0),
				policydsl.SignedBy(0),
			),
			policydsl.SignedBy(0),
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
		},
	}, spe)
}

func TestImplicitMetaPolicy_Convert5(t *testing.T) {
	// Scenario: we attempt the conversion of a metapolicy requiring
	// ALL of 2 sub-policies, where the first one is an OR of an AND
	// of 2 and an OR of 1 and the second one is an AND of 2, with
	// principal deduplication required both across the two subpolicies
	// and within the first

	pfs := &cauthdsl.EnvelopeBasedPolicyProvider{}

	p1, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.NOutOf(1,
			[]*cb.SignaturePolicy{
				policydsl.NOutOf(2,
					[]*cb.SignaturePolicy{
						policydsl.SignedBy(0),
						policydsl.SignedBy(1),
					},
				),
				policydsl.NOutOf(1,
					[]*cb.SignaturePolicy{
						policydsl.SignedBy(2),
					},
				),
			},
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "C",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
		},
	})
	require.NotNil(t, p1)
	require.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.NOutOf(2,
			[]*cb.SignaturePolicy{
				policydsl.SignedBy(0),
				policydsl.SignedBy(1),
			},
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
		},
	})
	require.NotNil(t, p2)
	require.NoError(t, err)

	p := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{p1, p2},
	}

	spe, err := p.Convert()
	require.NoError(t, err)
	require.NotNil(t, spe)
	require.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.And(
			policydsl.NOutOf(1,
				[]*cb.SignaturePolicy{
					policydsl.NOutOf(2,
						[]*cb.SignaturePolicy{
							policydsl.SignedBy(0),
							policydsl.SignedBy(1),
						},
					),
					policydsl.NOutOf(1,
						[]*cb.SignaturePolicy{
							policydsl.SignedBy(0),
						},
					),
				},
			),
			policydsl.NOutOf(2,
				[]*cb.SignaturePolicy{
					policydsl.SignedBy(2),
					policydsl.SignedBy(0),
				},
			),
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "C",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "A",
					},
				),
			},
		},
	}, spe)
}

func TestImplicitMetaPolicy_Convert6(t *testing.T) {
	// Scenario: we attempt the conversion of a metapolicy requiring
	// ALL of 2 sub-policies, where the first one is an OR of an AND
	// of 2 and an OR of 1 and the second one is an AND of 2, with
	// principal deduplication required both across the two subpolicies
	// and within the first and the second

	pfs := &cauthdsl.EnvelopeBasedPolicyProvider{}

	p1, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.NOutOf(1,
			[]*cb.SignaturePolicy{
				policydsl.NOutOf(2,
					[]*cb.SignaturePolicy{
						policydsl.SignedBy(0),
						policydsl.SignedBy(1),
					},
				),
				policydsl.NOutOf(1,
					[]*cb.SignaturePolicy{
						policydsl.SignedBy(2),
					},
				),
			},
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "C",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
		},
	})
	require.NotNil(t, p1)
	require.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.NOutOf(2,
			[]*cb.SignaturePolicy{
				policydsl.SignedBy(0),
				policydsl.SignedBy(1),
			},
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
		},
	})
	require.NotNil(t, p2)
	require.NoError(t, err)

	p := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{p1, p2},
	}

	spe, err := p.Convert()
	require.NoError(t, err)
	require.NotNil(t, spe)
	require.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: policydsl.And(
			policydsl.NOutOf(1,
				[]*cb.SignaturePolicy{
					policydsl.NOutOf(2,
						[]*cb.SignaturePolicy{
							policydsl.SignedBy(0),
							policydsl.SignedBy(1),
						},
					),
					policydsl.NOutOf(1,
						[]*cb.SignaturePolicy{
							policydsl.SignedBy(0),
						},
					),
				},
			),
			policydsl.NOutOf(2,
				[]*cb.SignaturePolicy{
					policydsl.SignedBy(0),
					policydsl.SignedBy(0),
				},
			),
		),
		Identities: []*mb.MSPPrincipal{
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "B",
					},
				),
			},
			{
				PrincipalClassification: mb.MSPPrincipal_ROLE,
				Principal: protoutil.MarshalOrPanic(
					&mb.MSPRole{
						Role:          mb.MSPRole_MEMBER,
						MspIdentifier: "C",
					},
				),
			},
		},
	}, spe)
}

type inconvertiblePolicy struct{}

func (i *inconvertiblePolicy) EvaluateSignedData(signatureSet []*protoutil.SignedData) error {
	return nil
}

func (i *inconvertiblePolicy) EvaluateIdentities(signatureSet []msp.Identity) error {
	return nil
}

func TestImplicitMetaPolicy_Convert7(t *testing.T) {
	// Scenario: we attempt the conversion of a metapolicy
	// with an incovertible subpolicy

	p := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{&inconvertiblePolicy{}},
	}

	spe, err := p.Convert()
	require.EqualError(t, err, "subpolicy number 0 type *policies_test.inconvertiblePolicy of policy mypolicy is not convertible")
	require.Nil(t, spe)
}

type convertFailurePolicy struct{}

func (i *convertFailurePolicy) EvaluateSignedData(signatureSet []*protoutil.SignedData) error {
	return nil
}

func (i *convertFailurePolicy) EvaluateIdentities(identities []msp.Identity) error {
	return nil
}

func (i *convertFailurePolicy) Convert() (*cb.SignaturePolicyEnvelope, error) {
	return nil, errors.New("nope")
}

func TestImplicitMetaPolicy_Convert8(t *testing.T) {
	// Scenario: we attempt the conversion of a metapolicy
	// with a subpolicy whose conversion fails

	p := &policies.PolicyLogger{
		Policy: &policies.ImplicitMetaPolicy{
			Threshold:     2,
			SubPolicyName: "mypolicy",
			SubPolicies:   []policies.Policy{&convertFailurePolicy{}},
		},
	}

	spe, err := p.Convert()
	require.EqualError(t, err, "failed to convert subpolicy number 0 of policy mypolicy: nope")
	require.Nil(t, spe)
}
