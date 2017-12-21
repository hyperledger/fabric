/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package inquire

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestSatisfiedBy(t *testing.T) {
	p1, err := cauthdsl.FromString("OR(AND('A.member', 'B.member'), 'C.member', AND('A.member', 'D.member'))")
	assert.NoError(t, err)

	principals := make([]*msp.MSPPrincipal, 0)

	mspId := func(principal *msp.MSPPrincipal) string {
		role := &msp.MSPRole{}
		proto.Unmarshal(principal.Principal, role)
		return role.MspIdentifier
	}

	appendPrincipal := func(orgName string) {
		principals = append(principals, &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: orgName})})
	}

	appendPrincipal("A")
	appendPrincipal("B")
	appendPrincipal("C")
	appendPrincipal("A")
	appendPrincipal("D")

	ip := NewInquireableSignaturePolicy(p1)
	satisfiedBy := ip.SatisfiedBy()

	expected := map[string]struct{}{
		fmt.Sprintf("%v", []string{"A", "B"}): {},
		fmt.Sprintf("%v", []string{"C"}):      {},
		fmt.Sprintf("%v", []string{"A", "D"}): {},
	}

	actual := make(map[string]struct{})
	for _, ps := range satisfiedBy {
		var principals []string
		for _, principal := range ps {
			principals = append(principals, mspId(principal))
		}
		actual[fmt.Sprintf("%v", principals)] = struct{}{}
	}

	assert.Equal(t, expected, actual)

	// Bad path: Remove an identity and re-try
	p1.Identities = p1.Identities[1:]
	ip = NewInquireableSignaturePolicy(p1)
	satisfiedBy = ip.SatisfiedBy()
	assert.Nil(t, satisfiedBy)
}
