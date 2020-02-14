/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	. "github.com/onsi/gomega"
)

func TestNewConsortiumsGroup(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums()

	mspConfig := &mb.MSPConfig{}

	consortiumsGroup, err := NewConsortiumsGroup(consortiums, mspConfig)
	gt.Expect(err).NotTo(HaveOccurred())

	// ConsortiumsGroup checks
	gt.Expect(len(consortiumsGroup.Groups)).To(Equal(1))
	gt.Expect(consortiumsGroup.Groups["Consortium1"]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Values)).To(Equal(0))
	gt.Expect(len(consortiumsGroup.Policies)).To(Equal(1))
	gt.Expect(consortiumsGroup.Policies[AdminsPolicyKey]).NotTo(BeNil())

	// ConsortiumGroup checks
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups)).To(Equal(2))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Values)).To(Equal(1))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Values[ChannelCreationPolicyKey]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Policies)).To(Equal(0))

	// ConsortiumOrgGroup checks
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Groups)).To(Equal(0))
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Values)).To(Equal(1))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Values[MSPKey]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Policies)).To(Equal(4))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Policies[EndorsementPolicyKey]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Groups)).To(Equal(0))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Values[MSPKey]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Policies)).To(Equal(4))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Policies[EndorsementPolicyKey]).NotTo(BeNil())
}

func TestNewConsortiumsGroupFailure(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums()
	consortiums["Consortium1"].Organizations[0].Policies = nil

	mspConfig := &mb.MSPConfig{}

	consortiumsGroup, err := NewConsortiumsGroup(consortiums, mspConfig)
	gt.Expect(err).To(MatchError("failed to create consortium group: " +
		"failed to create consortium org group Org1: " +
		"failed to add policies: no policies defined"),
	)
	gt.Expect(consortiumsGroup).To(BeNil())
}

func TestSkipAsForeignForConsortiumOrg(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums()
	consortiums["Consortium1"].Organizations[0].SkipAsForeign = true
	consortiums["Consortium1"].Organizations[1].SkipAsForeign = true

	mspConfig := &mb.MSPConfig{}

	// returns a consortiums group with consortium groups that have empty consortium org groups with only mod policy
	consortiumsGroup, err := NewConsortiumsGroup(consortiums, mspConfig)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"]).To(Equal(&cb.ConfigGroup{
		ModPolicy: AdminsPolicyKey,
		Groups:    make(map[string]*cb.ConfigGroup),
		Values:    make(map[string]*cb.ConfigValue),
		Policies:  make(map[string]*cb.ConfigPolicy),
	}))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"]).To(Equal(&cb.ConfigGroup{
		ModPolicy: AdminsPolicyKey,
		Groups:    make(map[string]*cb.ConfigGroup),
		Values:    make(map[string]*cb.ConfigValue),
		Policies:  make(map[string]*cb.ConfigPolicy),
	}))
}

func baseConsortiums() map[string]*Consortium {
	return map[string]*Consortium{
		"Consortium1": {
			Organizations: []*Organization{
				{
					Name:     "Org1",
					ID:       "Org1MSP",
					Policies: createOrgStandardPolicies(),
				},
				{
					Name:     "Org2",
					ID:       "Org2MSP",
					Policies: createOrgStandardPolicies(),
				},
			},
		},
	}
}
