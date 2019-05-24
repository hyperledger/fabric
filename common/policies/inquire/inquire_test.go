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

type testCase struct {
	name       string
	policy     string
	expected   map[string]struct{}
	principals []*msp.MSPPrincipal
}

func createPrincipals(orgNames ...string) []*msp.MSPPrincipal {
	principals := make([]*msp.MSPPrincipal, 0)
	appendPrincipal := func(orgName string) {
		principals = append(principals, &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: orgName})})
	}
	for _, org := range orgNames {
		appendPrincipal(org)
	}
	return principals
}

var cases = []testCase{
	{
		name:   "orOfAnds",
		policy: "OR(AND('A.member', 'B.member'), 'C.member', AND('A.member', 'D.member'))",
		expected: map[string]struct{}{
			fmt.Sprintf("%v", []string{"A", "B"}): {},
			fmt.Sprintf("%v", []string{"C"}):      {},
			fmt.Sprintf("%v", []string{"A", "D"}): {},
		},
		principals: createPrincipals("A", "B", "C", "D", "A"),
	},
	{
		name:   "andOfOrs",
		policy: "AND('A.member', 'C.member', OR('B.member', 'D.member'))",
		expected: map[string]struct{}{
			fmt.Sprintf("%v", []string{"A", "C", "B"}): {},
			fmt.Sprintf("%v", []string{"A", "C", "D"}): {},
		},
		principals: createPrincipals("A", "C", "B", "D"),
	},
	{
		name:   "orOfOrs",
		policy: "OR('A.member', OR('B.member', 'C.member'))",
		expected: map[string]struct{}{
			fmt.Sprintf("%v", []string{"A"}): {},
			fmt.Sprintf("%v", []string{"B"}): {},
			fmt.Sprintf("%v", []string{"C"}): {},
		},
		principals: createPrincipals("A", "B", "C"),
	},
	{
		name:   "andOfAnds",
		policy: "AND('A.member', AND('B.member', 'C.member'), AND('D.member','A.member'))",
		expected: map[string]struct{}{
			fmt.Sprintf("%v", []string{"A", "B", "C", "D", "A"}): {},
		},
		principals: createPrincipals("A", "B", "C", "D"),
	},
}

func mspId(principal *msp.MSPPrincipal) string {
	role := &msp.MSPRole{}
	proto.Unmarshal(principal.Principal, role)
	return role.MspIdentifier
}

func TestSatisfiedBy(t *testing.T) {
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			p, err := cauthdsl.FromString(test.policy)
			assert.NoError(t, err)

			ip := NewInquireableSignaturePolicy(p)
			satisfiedBy := ip.SatisfiedBy()

			actual := make(map[string]struct{})
			for _, ps := range satisfiedBy {
				var principals []string
				for _, principal := range ps {
					principals = append(principals, mspId(principal))
				}
				actual[fmt.Sprintf("%v", principals)] = struct{}{}
			}

			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestSatisfiedByTooManyCombinations(t *testing.T) {
	// We have 26 choose 15 members which is 7,726,160
	// and we ensure we don't return so many combinations,
	// but limit it to combinationsUpperBound.

	p, err := cauthdsl.FromString("OutOf(15, 'A.member', 'B.member', 'C.member', 'D.member', 'E.member', 'F.member'," +
		" 'G.member', 'H.member', 'I.member', 'J.member', 'K.member', 'L.member', 'M.member', 'N.member', 'O.member', " +
		"'P.member', 'Q.member', 'R.member', 'S.member', 'T.member', 'U.member', 'V.member', 'W.member', 'X.member', " +
		"'Y.member', 'Z.member')")
	assert.NoError(t, err)

	ip := NewInquireableSignaturePolicy(p)
	satisfiedBy := ip.SatisfiedBy()

	actual := make(map[string]struct{})
	for _, ps := range satisfiedBy {
		// Every subset is of size 15, as needed by the endorsement policy.
		assert.Len(t, ps, 15)
		var principals []string
		for _, principal := range ps {
			principals = append(principals, mspId(principal))
		}
		actual[fmt.Sprintf("%v", principals)] = struct{}{}
	}
	// Total combinations are capped by the combinationsUpperBound.
	assert.True(t, len(actual) < combinationsUpperBound)
}
