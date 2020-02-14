/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"reflect"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/gomega"
)

func TestOutOf1(t *testing.T) {
	gt := NewGomegaWithT(t)
	p1, err := FromString("OutOf(1, 'A.member', 'B.member')")
	gt.Expect(err).NotTo(HaveOccurred())

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	p2 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*cb.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}

	gt.Expect(p1).To(Equal(p2))
}

func TestOutOf2(t *testing.T) {
	gt := NewGomegaWithT(t)
	p1, err := FromString("OutOf(2, 'A.member', 'B.member')")
	gt.Expect(err).NotTo(HaveOccurred())

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	p2 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(2, []*cb.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}

	gt.Expect(p1).To(Equal(p2))
}

func TestAnd(t *testing.T) {
	gt := NewGomegaWithT(t)
	p1, err := FromString("AND('A.member', 'B.member')")
	gt.Expect(err).NotTo(HaveOccurred())

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	p2 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       cbAnd(SignedBy(0), SignedBy(1)),
		Identities: principals,
	}

	gt.Expect(p1).To(Equal(p2))
}

func TestAndClientPeerOrderer(t *testing.T) {
	gt := NewGomegaWithT(t)
	p1, err := FromString("AND('A.client', 'B.peer')")
	gt.Expect(err).NotTo(HaveOccurred())

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_CLIENT, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: "B"})})

	p2 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       cbAnd(SignedBy(0), SignedBy(1)),
		Identities: principals,
	}

	gt.Expect(reflect.DeepEqual(p1, p2)).To(BeTrue())

}

func TestOr(t *testing.T) {
	gt := NewGomegaWithT(t)
	p1, err := FromString("OR('A.member', 'B.member')")
	gt.Expect(err).NotTo(HaveOccurred())

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	p2 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       cbOr(SignedBy(0), SignedBy(1)),
		Identities: principals,
	}

	gt.Expect(p1).To(Equal(p2))
}

func TestComplex1(t *testing.T) {
	gt := NewGomegaWithT(t)
	p1, err := FromString("OR('A.member', AND('B.member', 'C.member'))")
	gt.Expect(err).NotTo(HaveOccurred())

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "C"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	p2 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       cbOr(SignedBy(2), cbAnd(SignedBy(0), SignedBy(1))),
		Identities: principals,
	}

	gt.Expect(p1).To(Equal(p2))
}

func TestComplex2(t *testing.T) {
	gt := NewGomegaWithT(t)
	p1, err := FromString("OR(AND('A.member', 'B.member'), OR('C.admin', 'D.member'))")
	gt.Expect(err).NotTo(HaveOccurred())

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: "C"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "D"})})

	p2 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       cbOr(cbAnd(SignedBy(0), SignedBy(1)), cbOr(SignedBy(2), SignedBy(3))),
		Identities: principals,
	}

	gt.Expect(p1).To(Equal(p2))
}

func TestMSPIDWIthSpecialChars(t *testing.T) {
	gt := NewGomegaWithT(t)
	p1, err := FromString("OR('MSP.member', 'MSP.WITH.DOTS.member', 'MSP-WITH-DASHES.member')")
	gt.Expect(err).NotTo(HaveOccurred())

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "MSP"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "MSP.WITH.DOTS"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "MSP-WITH-DASHES"})})

	p2 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*cb.SignaturePolicy{SignedBy(0), SignedBy(1), SignedBy(2)}),
		Identities: principals,
	}

	gt.Expect(p1).To(Equal(p2))
}

func TestBadStringsNoPanic(t *testing.T) {
	gt := NewGomegaWithT(t)
	_, err := FromString("OR('A.member', Bmember)") // error after 1st Evaluate()
	gt.Expect(err).To(MatchError("unrecognized token 'Bmember' in policy string"))

	_, err = FromString("OR('A.member', 'Bmember')") // error after 2nd Evalute()
	gt.Expect(err).To(MatchError("unrecognized token 'Bmember' in policy string"))

	_, err = FromString(`OR('A.member', '\'Bmember\'')`) // error after 3rd Evalute()
	gt.Expect(err).To(MatchError("unrecognized token 'Bmember' in policy string"))
}

func TestNodeOUs(t *testing.T) {
	gt := NewGomegaWithT(t)
	p1, err := FromString("OR('A.peer', 'B.admin', 'C.orderer', 'D.client')")
	gt.Expect(err).NotTo(HaveOccurred())

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: "B"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_ORDERER, MspIdentifier: "C"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_CLIENT, MspIdentifier: "D"})})

	p2 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*cb.SignaturePolicy{SignedBy(0), SignedBy(1), SignedBy(2), SignedBy(3)}),
		Identities: principals,
	}

	gt.Expect(p1).To(Equal(p2))
}

func TestOutOfNumIsString(t *testing.T) {
	gt := NewGomegaWithT(t)
	p1, err := FromString("OutOf('1', 'A.member', 'B.member')")
	gt.Expect(err).NotTo(HaveOccurred())

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	p2 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*cb.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}

	gt.Expect(p1).To(Equal(p2))
}

func TestOutOfErrorCase(t *testing.T) {
	gt := NewGomegaWithT(t)
	p1, err1 := FromString("") // 1st NewEvaluableExpressionWithFunctions() returns an error
	gt.Expect(p1).To(BeNil())
	gt.Expect(err1).To(MatchError("Unexpected end of expression"))

	p2, err2 := FromString("OutOf(1)") // outof() if len(args)<2
	gt.Expect(p2).To(BeNil())
	gt.Expect(err2).To(MatchError("expected at least two arguments to NOutOf, but got 1"))

	p3, err3 := FromString("OutOf(true, 'A.member')") // outof() }else{. 1st arg is non of float, int, string
	gt.Expect(p3).To(BeNil())
	gt.Expect(err3).To(MatchError("unexpected type bool"))

	p4, err4 := FromString("OutOf(1, 2)") // oufof() switch default. 2nd arg is not string.
	gt.Expect(p4).To(BeNil())
	gt.Expect(err4).To(MatchError("unexpected type float64"))

	p5, err5 := FromString("OutOf(1, 'true')") // firstPass() switch default
	gt.Expect(p5).To(BeNil())
	gt.Expect(err5).To(MatchError("unexpected type bool"))

	p6, err6 := FromString(`OutOf('\'\\\'A\\\'\'', 'B.member')`) // secondPass() switch args[1].(type) default
	gt.Expect(p6).To(BeNil())
	gt.Expect(err6).To(MatchError("unrecognized type, expected a number, but got string"))

	p7, err7 := FromString(`OutOf(1, '\'1\'')`) // secondPass() switch args[1].(type) default
	gt.Expect(p7).To(BeNil())
	gt.Expect(err7).To(MatchError("unrecognized type, expected a principal or a policy, but got float64"))

	p8, err8 := FromString(`''`) // 2nd NewEvaluateExpressionWithFunction() returns an error
	gt.Expect(p8).To(BeNil())
	gt.Expect(err8).To(MatchError("Unexpected end of expression"))

	p9, err9 := FromString(`'\'\''`) // 3rd NewEvaluateExpressionWithFunction() returns an error
	gt.Expect(p9).To(BeNil())
	gt.Expect(err9).To(MatchError("Unexpected end of expression"))
}

func TestBadStringBeforeFAB11404_ThisCanDeleteAfterFAB11404HasMerged(t *testing.T) {
	gt := NewGomegaWithT(t)
	s1 := "1" // ineger in string
	p1, err1 := FromString(s1)
	gt.Expect(p1).To(BeNil())
	gt.Expect(err1).To(MatchError("invalid policy string '1'"))

	s2 := "'1'" // quoted ineger in string
	p2, err2 := FromString(s2)
	gt.Expect(p2).To(BeNil())
	gt.Expect(err2).To(MatchError("invalid policy string ''1''"))

	s3 := `'\'1\''` // nested quoted ineger in string
	p3, err3 := FromString(s3)
	gt.Expect(p3).To(BeNil())
	gt.Expect(err3).To(MatchError(`invalid policy string ''\'1\'''`))
}

func TestSecondPassBoundaryCheck(t *testing.T) {
	gt := NewGomegaWithT(t)
	// Check lower boundary
	// Prohibit t<0
	p0, err0 := FromString("OutOf(-1, 'A.member', 'B.member')")
	gt.Expect(p0).To(BeNil())
	gt.Expect(err0).To(MatchError("invalid t-out-of-n predicate, t -1, n 2"))

	// Permit t==0 : always satisfied policy
	// There is no clear usecase of t=0, but somebody may already use it, so we don't treat as an error.
	p1, err1 := FromString("OutOf(0, 'A.member', 'B.member')")
	gt.Expect(err1).NotTo(HaveOccurred())
	principals := make([]*msp.MSPPrincipal, 0)
	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})
	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})
	expected1 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(0, []*cb.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}
	gt.Expect(expected1).To(Equal(p1))

	// Check upper boundary
	// Permit t==n+1 : never satisfied policy
	// Usecase: To create immutable ledger key
	p2, err2 := FromString("OutOf(3, 'A.member', 'B.member')")
	gt.Expect(err2).NotTo(HaveOccurred())
	expected2 := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(3, []*cb.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}
	gt.Expect(expected2).To(Equal(p2))

	// Prohibit t>n + 1
	p3, err3 := FromString("OutOf(4, 'A.member', 'B.member')")
	gt.Expect(p3).To(BeNil())
	gt.Expect(err3).To(MatchError("invalid t-out-of-n predicate, t 4, n 2"))
}

// cbOr is a convenience method which utilizes NOutOf to produce Or equivalent behavior
func cbOr(lhs, rhs *cb.SignaturePolicy) *cb.SignaturePolicy {
	return NOutOf(1, []*cb.SignaturePolicy{lhs, rhs})
}

// cbAnd is a convenience method which utilizes NOutOf to produce And equivalent behavior
func cbAnd(lhs, rhs *cb.SignaturePolicy) *cb.SignaturePolicy {
	return NOutOf(2, []*cb.SignaturePolicy{lhs, rhs})
}

// NOutOf creates a policy which requires N out of the slice of policies to evaluate to true
func NOutOf(n int32, policies []*cb.SignaturePolicy) *cb.SignaturePolicy {
	return &cb.SignaturePolicy{
		Type: &cb.SignaturePolicy_NOutOf_{
			NOutOf: &cb.SignaturePolicy_NOutOf{
				N:     n,
				Rules: policies,
			},
		},
	}
}
