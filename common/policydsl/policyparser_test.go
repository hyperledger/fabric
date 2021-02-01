/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policydsl

import (
	"reflect"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestOutOf1(t *testing.T) {
	p1, err := FromString("OutOf(1, 'A.member', 'B.member')")
	require.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"}),
	})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*common.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}

	require.Equal(t, p1, p2)
}

func TestOutOf2(t *testing.T) {
	p1, err := FromString("OutOf(2, 'A.member', 'B.member')")
	require.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"}),
	})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(2, []*common.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}

	require.Equal(t, p1, p2)
}

func TestAnd(t *testing.T) {
	p1, err := FromString("AND('A.member', 'B.member')")
	require.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"}),
	})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       And(SignedBy(0), SignedBy(1)),
		Identities: principals,
	}

	require.Equal(t, p1, p2)
}

func TestAndClientPeerOrderer(t *testing.T) {
	p1, err := FromString("AND('A.client', 'B.peer')")
	require.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_CLIENT, MspIdentifier: "A"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: "B"}),
	})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       And(SignedBy(0), SignedBy(1)),
		Identities: principals,
	}

	require.True(t, reflect.DeepEqual(p1, p2))
}

func TestOr(t *testing.T) {
	p1, err := FromString("OR('A.member', 'B.member')")
	require.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"}),
	})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       Or(SignedBy(0), SignedBy(1)),
		Identities: principals,
	}

	require.Equal(t, p1, p2)
}

func TestComplex1(t *testing.T) {
	p1, err := FromString("OR('A.member', AND('B.member', 'C.member'))")
	require.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "C"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"}),
	})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       Or(SignedBy(2), And(SignedBy(0), SignedBy(1))),
		Identities: principals,
	}

	require.Equal(t, p1, p2)
}

func TestComplex2(t *testing.T) {
	p1, err := FromString("OR(AND('A.member', 'B.member'), OR('C.admin', 'D.member'))")
	require.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: "C"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "D"}),
	})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       Or(And(SignedBy(0), SignedBy(1)), Or(SignedBy(2), SignedBy(3))),
		Identities: principals,
	}

	require.Equal(t, p1, p2)
}

func TestMSPIDWIthSpecialChars(t *testing.T) {
	p1, err := FromString("OR('MSP.member', 'MSP.WITH.DOTS.member', 'MSP-WITH-DASHES.member')")
	require.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "MSP"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "MSP.WITH.DOTS"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "MSP-WITH-DASHES"}),
	})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*common.SignaturePolicy{SignedBy(0), SignedBy(1), SignedBy(2)}),
		Identities: principals,
	}

	require.Equal(t, p1, p2)
}

func TestBadStringsNoPanic(t *testing.T) {
	_, err := FromString("OR('A.member', Bmember)") // error after 1st Evaluate()
	require.EqualError(t, err, "unrecognized token 'Bmember' in policy string")

	_, err = FromString("OR('A.member', 'Bmember')") // error after 2nd Evalute()
	require.EqualError(t, err, "unrecognized token 'Bmember' in policy string")

	_, err = FromString(`OR('A.member', '\'Bmember\'')`) // error after 3rd Evalute()
	require.EqualError(t, err, "unrecognized token 'Bmember' in policy string")
}

func TestNodeOUs(t *testing.T) {
	p1, err := FromString("OR('A.peer', 'B.admin', 'C.orderer', 'D.client')")
	require.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: "A"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: "B"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_ORDERER, MspIdentifier: "C"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_CLIENT, MspIdentifier: "D"}),
	})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*common.SignaturePolicy{SignedBy(0), SignedBy(1), SignedBy(2), SignedBy(3)}),
		Identities: principals,
	}

	require.Equal(t, p1, p2)
}

func TestOutOfNumIsString(t *testing.T) {
	p1, err := FromString("OutOf('1', 'A.member', 'B.member')")
	require.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"}),
	})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"}),
	})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*common.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}

	require.Equal(t, p1, p2)
}

func TestOutOfErrorCase(t *testing.T) {
	p1, err1 := FromString("") // 1st NewEvaluableExpressionWithFunctions() returns an error
	require.Nil(t, p1)
	require.EqualError(t, err1, "Unexpected end of expression")

	p2, err2 := FromString("OutOf(1)") // outof() if len(args)<2
	require.Nil(t, p2)
	require.EqualError(t, err2, "expected at least two arguments to NOutOf. Given 1")

	p3, err3 := FromString("OutOf(true, 'A.member')") // outof() }else{. 1st arg is non of float, int, string
	require.Nil(t, p3)
	require.EqualError(t, err3, "unexpected type bool")

	p4, err4 := FromString("OutOf(1, 2)") // oufof() switch default. 2nd arg is not string.
	require.Nil(t, p4)
	require.EqualError(t, err4, "unexpected type float64")

	p5, err5 := FromString("OutOf(1, 'true')") // firstPass() switch default
	require.Nil(t, p5)
	require.EqualError(t, err5, "unexpected type bool")

	p6, err6 := FromString(`OutOf('\'\\\'A\\\'\'', 'B.member')`) // secondPass() switch args[1].(type) default
	require.Nil(t, p6)
	require.EqualError(t, err6, "unrecognized type, expected a number, got string")

	p7, err7 := FromString(`OutOf(1, '\'1\'')`) // secondPass() switch args[1].(type) default
	require.Nil(t, p7)
	require.EqualError(t, err7, "unrecognized type, expected a principal or a policy, got float64")

	p8, err8 := FromString(`''`) // 2nd NewEvaluateExpressionWithFunction() returns an error
	require.Nil(t, p8)
	require.EqualError(t, err8, "Unexpected end of expression")

	p9, err9 := FromString(`'\'\''`) // 3rd NewEvaluateExpressionWithFunction() returns an error
	require.Nil(t, p9)
	require.EqualError(t, err9, "Unexpected end of expression")
}

func TestBadStringBeforeFAB11404_ThisCanDeleteAfterFAB11404HasMerged(t *testing.T) {
	s1 := "1" // ineger in string
	p1, err1 := FromString(s1)
	require.Nil(t, p1)
	require.EqualError(t, err1, `invalid policy string '1'`)

	s2 := "'1'" // quoted ineger in string
	p2, err2 := FromString(s2)
	require.Nil(t, p2)
	require.EqualError(t, err2, `invalid policy string ''1''`)

	s3 := `'\'1\''` // nested quoted ineger in string
	p3, err3 := FromString(s3)
	require.Nil(t, p3)
	require.EqualError(t, err3, `invalid policy string ''\'1\'''`)
}

func TestSecondPassBoundaryCheck(t *testing.T) {
	// Check lower boundary
	// Prohibit t<0
	p0, err0 := FromString("OutOf(-1, 'A.member', 'B.member')")
	require.Nil(t, p0)
	require.EqualError(t, err0, "invalid t-out-of-n predicate, t -1, n 2")

	// Permit t==0 : always satisfied policy
	// There is no clear usecase of t=0, but somebody may already use it, so we don't treat as an error.
	p1, err1 := FromString("OutOf(0, 'A.member', 'B.member')")
	require.NoError(t, err1)
	principals := make([]*msp.MSPPrincipal, 0)
	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"}),
	})
	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"}),
	})
	expected1 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(0, []*common.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}
	require.Equal(t, expected1, p1)

	// Check upper boundary
	// Permit t==n+1 : never satisfied policy
	// Usecase: To create immutable ledger key
	p2, err2 := FromString("OutOf(3, 'A.member', 'B.member')")
	require.NoError(t, err2)
	expected2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(3, []*common.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}
	require.Equal(t, expected2, p2)

	// Prohibit t>n + 1
	p3, err3 := FromString("OutOf(4, 'A.member', 'B.member')")
	require.Nil(t, p3)
	require.EqualError(t, err3, "invalid t-out-of-n predicate, t 4, n 2")
}
