/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
	mb "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestImplicitMetaPolicy_Convert(t *testing.T) {

	// Scenario: we attempt the conversion of a simple metapolicy requiring
	// ALL of 2 sub-policies, each of which are plain signedby

	pfs := &cauthdsl.EnvelopeBasedPolicyProvider{}

	p1, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    cauthdsl.SignedBy(0),
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
	assert.NotNil(t, p1)
	assert.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    cauthdsl.SignedBy(0),
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
	assert.NotNil(t, p2)
	assert.NoError(t, err)

	p := &policies.PolicyLogger{
		Policy: &policies.ImplicitMetaPolicy{
			Threshold:     2,
			SubPolicyName: "mypolicy",
			SubPolicies:   []policies.Policy{p1, p2},
		},
	}

	spe, err := p.Convert()
	assert.NoError(t, err)
	assert.NotNil(t, spe)
	assert.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: cauthdsl.And(
			cauthdsl.SignedBy(0),
			cauthdsl.SignedBy(1),
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

	p1, err := pfs.NewPolicy(cauthdsl.SignedByAnyMember([]string{"A", "B"}))
	assert.NotNil(t, p1)
	assert.NoError(t, err)

	p2, err := pfs.NewPolicy(cauthdsl.SignedByAnyMember([]string{"B"}))
	assert.NotNil(t, p2)
	assert.NoError(t, err)

	p := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{p1, p2},
	}

	spe, err := p.Convert()
	assert.NoError(t, err)
	assert.NotNil(t, spe)
	assert.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: cauthdsl.And(
			cauthdsl.Or(
				cauthdsl.SignedBy(0),
				cauthdsl.SignedBy(1),
			),
			cauthdsl.NOutOf(1,
				[]*cb.SignaturePolicy{cauthdsl.SignedBy(1)},
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
		Rule: cauthdsl.NOutOf(1,
			[]*cb.SignaturePolicy{
				cauthdsl.NOutOf(2,
					[]*cb.SignaturePolicy{
						cauthdsl.SignedBy(0),
						cauthdsl.SignedBy(1),
					},
				),
				cauthdsl.NOutOf(1,
					[]*cb.SignaturePolicy{
						cauthdsl.SignedBy(2),
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
	assert.NotNil(t, p1)
	assert.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: cauthdsl.NOutOf(2,
			[]*cb.SignaturePolicy{
				cauthdsl.SignedBy(0),
				cauthdsl.SignedBy(1),
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
	assert.NotNil(t, p2)
	assert.NoError(t, err)

	p := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{p1, p2},
	}

	spe, err := p.Convert()
	assert.NoError(t, err)
	assert.NotNil(t, spe)
	assert.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: cauthdsl.And(
			cauthdsl.NOutOf(1,
				[]*cb.SignaturePolicy{
					cauthdsl.NOutOf(2,
						[]*cb.SignaturePolicy{
							cauthdsl.SignedBy(0),
							cauthdsl.SignedBy(1),
						},
					),
					cauthdsl.NOutOf(1,
						[]*cb.SignaturePolicy{
							cauthdsl.SignedBy(2),
						},
					),
				},
			),
			cauthdsl.NOutOf(2,
				[]*cb.SignaturePolicy{
					cauthdsl.SignedBy(3),
					cauthdsl.SignedBy(0),
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
		Rule:    cauthdsl.SignedBy(0),
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
	assert.NotNil(t, p1)
	assert.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    cauthdsl.SignedBy(0),
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
	assert.NotNil(t, p2)
	assert.NoError(t, err)

	p3, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    cauthdsl.SignedBy(0),
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
	assert.NotNil(t, p3)
	assert.NoError(t, err)

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
	assert.NoError(t, err)
	assert.NotNil(t, spe)
	assert.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: cauthdsl.And(
			cauthdsl.And(
				cauthdsl.SignedBy(0),
				cauthdsl.SignedBy(1),
			),
			cauthdsl.SignedBy(2),
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
		Rule:    cauthdsl.SignedBy(0),
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
	assert.NotNil(t, p1)
	assert.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    cauthdsl.SignedBy(0),
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
	assert.NotNil(t, p2)
	assert.NoError(t, err)

	p3, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule:    cauthdsl.SignedBy(0),
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
	assert.NotNil(t, p3)
	assert.NoError(t, err)

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
	assert.NoError(t, err)
	assert.NotNil(t, spe)
	assert.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: cauthdsl.And(
			cauthdsl.And(
				cauthdsl.SignedBy(0),
				cauthdsl.SignedBy(0),
			),
			cauthdsl.SignedBy(0),
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
		Rule: cauthdsl.NOutOf(1,
			[]*cb.SignaturePolicy{
				cauthdsl.NOutOf(2,
					[]*cb.SignaturePolicy{
						cauthdsl.SignedBy(0),
						cauthdsl.SignedBy(1),
					},
				),
				cauthdsl.NOutOf(1,
					[]*cb.SignaturePolicy{
						cauthdsl.SignedBy(2),
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
	assert.NotNil(t, p1)
	assert.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: cauthdsl.NOutOf(2,
			[]*cb.SignaturePolicy{
				cauthdsl.SignedBy(0),
				cauthdsl.SignedBy(1),
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
	assert.NotNil(t, p2)
	assert.NoError(t, err)

	p := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{p1, p2},
	}

	spe, err := p.Convert()
	assert.NoError(t, err)
	assert.NotNil(t, spe)
	assert.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: cauthdsl.And(
			cauthdsl.NOutOf(1,
				[]*cb.SignaturePolicy{
					cauthdsl.NOutOf(2,
						[]*cb.SignaturePolicy{
							cauthdsl.SignedBy(0),
							cauthdsl.SignedBy(1),
						},
					),
					cauthdsl.NOutOf(1,
						[]*cb.SignaturePolicy{
							cauthdsl.SignedBy(0),
						},
					),
				},
			),
			cauthdsl.NOutOf(2,
				[]*cb.SignaturePolicy{
					cauthdsl.SignedBy(2),
					cauthdsl.SignedBy(0),
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
		Rule: cauthdsl.NOutOf(1,
			[]*cb.SignaturePolicy{
				cauthdsl.NOutOf(2,
					[]*cb.SignaturePolicy{
						cauthdsl.SignedBy(0),
						cauthdsl.SignedBy(1),
					},
				),
				cauthdsl.NOutOf(1,
					[]*cb.SignaturePolicy{
						cauthdsl.SignedBy(2),
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
	assert.NotNil(t, p1)
	assert.NoError(t, err)

	p2, err := pfs.NewPolicy(&cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: cauthdsl.NOutOf(2,
			[]*cb.SignaturePolicy{
				cauthdsl.SignedBy(0),
				cauthdsl.SignedBy(1),
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
	assert.NotNil(t, p2)
	assert.NoError(t, err)

	p := &policies.ImplicitMetaPolicy{
		Threshold:     2,
		SubPolicyName: "mypolicy",
		SubPolicies:   []policies.Policy{p1, p2},
	}

	spe, err := p.Convert()
	assert.NoError(t, err)
	assert.NotNil(t, spe)
	assert.Equal(t, &cb.SignaturePolicyEnvelope{
		Version: 0,
		Rule: cauthdsl.And(
			cauthdsl.NOutOf(1,
				[]*cb.SignaturePolicy{
					cauthdsl.NOutOf(2,
						[]*cb.SignaturePolicy{
							cauthdsl.SignedBy(0),
							cauthdsl.SignedBy(1),
						},
					),
					cauthdsl.NOutOf(1,
						[]*cb.SignaturePolicy{
							cauthdsl.SignedBy(0),
						},
					),
				},
			),
			cauthdsl.NOutOf(2,
				[]*cb.SignaturePolicy{
					cauthdsl.SignedBy(0),
					cauthdsl.SignedBy(0),
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

func (i *inconvertiblePolicy) Evaluate(signatureSet []*protoutil.SignedData) error {
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
	assert.EqualError(t, err, "subpolicy number 0 type *policies_test.inconvertiblePolicy of policy mypolicy is not convertible")
	assert.Nil(t, spe)
}

type convertFailurePolicy struct{}

func (i *convertFailurePolicy) Evaluate(signatureSet []*protoutil.SignedData) error {
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
	assert.EqualError(t, err, "failed to convert subpolicy number 0 of policy mypolicy: nope")
	assert.Nil(t, spe)
}
