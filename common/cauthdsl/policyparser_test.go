/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cauthdsl

import (
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestOutOf1(t *testing.T) {
	p1, err := FromString("OutOf(1, 'A.member', 'B.member')")
	assert.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*common.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}

	assert.Equal(t, p1, p2)
}

func TestOutOf2(t *testing.T) {
	p1, err := FromString("OutOf(2, 'A.member', 'B.member')")
	assert.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(2, []*common.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}

	assert.Equal(t, p1, p2)
}

func TestAnd(t *testing.T) {
	p1, err := FromString("AND('A.member', 'B.member')")
	assert.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       And(SignedBy(0), SignedBy(1)),
		Identities: principals,
	}

	assert.Equal(t, p1, p2)
}

func TestAndClientPeerOrderer(t *testing.T) {
	p1, err := FromString("AND('A.client', 'B.peer')")
	assert.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_CLIENT, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: "B"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       And(SignedBy(0), SignedBy(1)),
		Identities: principals,
	}

	assert.True(t, reflect.DeepEqual(p1, p2))

}

func TestOr(t *testing.T) {
	p1, err := FromString("OR('A.member', 'B.member')")
	assert.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       Or(SignedBy(0), SignedBy(1)),
		Identities: principals,
	}

	assert.Equal(t, p1, p2)
}

func TestComplex1(t *testing.T) {
	p1, err := FromString("OR('A.member', AND('B.member', 'C.member'))")
	assert.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "C"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       Or(SignedBy(2), And(SignedBy(0), SignedBy(1))),
		Identities: principals,
	}

	assert.Equal(t, p1, p2)
}

func TestComplex2(t *testing.T) {
	p1, err := FromString("OR(AND('A.member', 'B.member'), OR('C.admin', 'D.member'))")
	assert.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: "C"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "D"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       Or(And(SignedBy(0), SignedBy(1)), Or(SignedBy(2), SignedBy(3))),
		Identities: principals,
	}

	assert.Equal(t, p1, p2)
}

func TestMSPIDWIthSpecialChars(t *testing.T) {
	p1, err := FromString("OR('MSP.member', 'MSP.WITH.DOTS.member', 'MSP-WITH-DASHES.member')")
	assert.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "MSP"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "MSP.WITH.DOTS"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "MSP-WITH-DASHES"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*common.SignaturePolicy{SignedBy(0), SignedBy(1), SignedBy(2)}),
		Identities: principals,
	}

	assert.Equal(t, p1, p2)
}

func TestBadStringsNoPanic(t *testing.T) {
	_, err := FromString("OR('A.member', Bmember)") // error after 1st Evaluate()
	assert.EqualError(t, err, "unrecognized token 'Bmember' in policy string")

	_, err = FromString("OR('A.member', 'Bmember')") // error after 2nd Evalute()
	assert.EqualError(t, err, "unrecognized token 'Bmember' in policy string")

	_, err = FromString(`OR('A.member', '\'Bmember\'')`) // error after 3rd Evalute()
	assert.EqualError(t, err, "unrecognized token 'Bmember' in policy string")
}

func TestOutOfNumIsString(t *testing.T) {
	p1, err := FromString("OutOf('1', 'A.member', 'B.member')")
	assert.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "A"})})

	principals = append(principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "B"})})

	p2 := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*common.SignaturePolicy{SignedBy(0), SignedBy(1)}),
		Identities: principals,
	}

	assert.Equal(t, p1, p2)
}

func TestOutOfErrorCase(t *testing.T) {
	p1, err1 := FromString("") // 1st NewEvaluableExpressionWithFunctions() returns an error
	assert.Nil(t, p1)
	assert.EqualError(t, err1, "Unexpected end of expression")

	p2, err2 := FromString("OutOf(1)") // outof() if len(args)<2
	assert.Nil(t, p2)
	assert.EqualError(t, err2, "Expected at least two arguments to NOutOf. Given 1")

	p3, err3 := FromString("OutOf(true, 'A.member')") // outof() }else{. 1st arg is non of float, int, string
	assert.Nil(t, p3)
	assert.EqualError(t, err3, "Unexpected type bool")

	p4, err4 := FromString("OutOf(1, 2)") // oufof() switch default. 2nd arg is not string.
	assert.Nil(t, p4)
	assert.EqualError(t, err4, "Unexpected type float64")

	p5, err5 := FromString("OutOf(1, 'true')") // firstPass() switch default
	assert.Nil(t, p5)
	assert.EqualError(t, err5, "Unexpected type bool")

	p6, err6 := FromString(`OutOf('\'\\\'A\\\'\'', 'B.member')`) // secondPass() switch args[1].(type) default
	assert.Nil(t, p6)
	assert.EqualError(t, err6, "Unrecognized type, expected a number, got string")

	p7, err7 := FromString(`OutOf(1, '\'1\'')`) // secondPass() switch args[1].(type) default
	assert.Nil(t, p7)
	assert.EqualError(t, err7, "Unrecognized type, expected a principal or a policy, got float64")

	p8, err8 := FromString(`''`) // 2nd NewEvaluateExpressionWithFunction() returns an error
	assert.Nil(t, p8)
	assert.EqualError(t, err8, "Unexpected end of expression")

	p9, err9 := FromString(`'\'\''`) // 3rd NewEvaluateExpressionWithFunction() returns an error
	assert.Nil(t, p9)
	assert.EqualError(t, err9, "Unexpected end of expression")
}

func TestBadStringBeforeFAB11404_ThisCanDeleteAfterFAB11404HasMerged(t *testing.T) {
	s1 := "1" // ineger in string
	p1, err1 := FromString(s1)
	assert.Nil(t, p1)
	assert.EqualError(t, err1, `invalid policy string '1'`)

	s2 := "'1'" // quoted ineger in string
	p2, err2 := FromString(s2)
	assert.Nil(t, p2)
	assert.EqualError(t, err2, `invalid policy string ''1''`)

	s3 := `'\'1\''` // nested quoted ineger in string
	p3, err3 := FromString(s3)
	assert.Nil(t, p3)
	assert.EqualError(t, err3, `invalid policy string ''\'1\'''`)
}
