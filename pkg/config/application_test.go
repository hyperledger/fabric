/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"errors"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	. "github.com/onsi/gomega"
)

func TestNewApplicationGroup(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	application := baseApplication()

	mspConfig := &mb.MSPConfig{}

	applicationGroup, err := NewApplicationGroup(application, mspConfig)
	gt.Expect(err).NotTo(HaveOccurred())

	// ApplicationGroup checks
	gt.Expect(len(applicationGroup.Groups)).To(Equal(2))
	gt.Expect(applicationGroup.Groups["Org1"]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"]).NotTo(BeNil())
	gt.Expect(len(applicationGroup.Values)).To(Equal(2))
	gt.Expect(applicationGroup.Values[ACLsKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Values[CapabilitiesKey]).NotTo(BeNil())
	gt.Expect(len(applicationGroup.Policies)).To(Equal(3))
	gt.Expect(applicationGroup.Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Policies[WritersPolicyKey]).NotTo(BeNil())

	// ApplicationOrgGroup checks
	gt.Expect(len(applicationGroup.Groups["Org1"].Groups)).To(Equal(0))
	gt.Expect(len(applicationGroup.Groups["Org1"].Values)).To(Equal(2))
	gt.Expect(applicationGroup.Groups["Org1"].Values[MSPKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org1"].Values[AnchorPeersKey]).NotTo(BeNil())
	gt.Expect(len(applicationGroup.Groups["Org1"].Policies)).To(Equal(5))
	gt.Expect(applicationGroup.Groups["Org1"].Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org1"].Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org1"].Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org1"].Policies[EndorsementPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org1"].Policies[LifecycleEndorsementPolicyKey]).NotTo(BeNil())
	gt.Expect(len(applicationGroup.Groups["Org2"].Groups)).To(Equal(0))
	gt.Expect(len(applicationGroup.Groups["Org2"].Values)).To(Equal(2))
	gt.Expect(applicationGroup.Groups["Org2"].Values[MSPKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"].Values[AnchorPeersKey]).NotTo(BeNil())
	gt.Expect(len(applicationGroup.Groups["Org2"].Policies)).To(Equal(5))
	gt.Expect(applicationGroup.Groups["Org2"].Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"].Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"].Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"].Policies[EndorsementPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"].Policies[LifecycleEndorsementPolicyKey]).NotTo(BeNil())
}

func TestNewApplicationGroupFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName       string
		applicationMod func(*Application)
		expectedErr    error
	}{
		{
			testName: "When application group policy is empty",
			applicationMod: func(a *Application) {
				a.Policies = nil
			},
			expectedErr: errors.New("failed to add policies: no policies defined"),
		},
		{
			testName: "When adding policies to application group",
			applicationMod: func(a *Application) {
				a.Organizations[0].Policies = nil
			},
			expectedErr: errors.New("failed to create application org group Org1: failed to add policies: " +
				"no policies defined"),
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			application := baseApplication()
			tt.applicationMod(application)

			mspConfig := &mb.MSPConfig{}

			configGrp, err := NewApplicationGroup(application, mspConfig)
			gt.Expect(err).To(MatchError(tt.expectedErr))
			gt.Expect(configGrp).To(BeNil())
		})
	}
}

func TestNewApplicationGroupSkipAsForeign(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	application := baseApplication()
	application.Organizations[0].SkipAsForeign = true
	application.Organizations[1].SkipAsForeign = true

	mspConfig := &mb.MSPConfig{}

	applicationGroup, err := NewApplicationGroup(application, mspConfig)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(applicationGroup.Groups["Org1"]).To(Equal(&cb.ConfigGroup{
		ModPolicy: AdminsPolicyKey,
		Groups:    make(map[string]*cb.ConfigGroup),
		Values:    make(map[string]*cb.ConfigValue),
		Policies:  make(map[string]*cb.ConfigPolicy),
	}))
	gt.Expect(applicationGroup.Groups["Org2"]).To(Equal(&cb.ConfigGroup{
		ModPolicy: AdminsPolicyKey,
		Groups:    make(map[string]*cb.ConfigGroup),
		Values:    make(map[string]*cb.ConfigValue),
		Policies:  make(map[string]*cb.ConfigPolicy),
	}))
}

func baseApplication() *Application {
	return &Application{
		Policies: createStandardPolicies(),
		Organizations: []*Organization{
			{
				Name:     "Org1",
				ID:       "Org1MSP",
				Policies: createApplicationOrgStandardPolicies(),
				AnchorPeers: []*AnchorPeer{
					{Host: "host1", Port: 123},
				},
			},
			{
				Name:     "Org2",
				ID:       "Org2MSP",
				Policies: createApplicationOrgStandardPolicies(),
				AnchorPeers: []*AnchorPeer{
					{Host: "host1", Port: 123},
				},
			},
		},
		Capabilities: map[string]bool{
			"V1_3": true,
		},
		ACLs: map[string]string{
			"acl1": "hi",
		},
	}
}
