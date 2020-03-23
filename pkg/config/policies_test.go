/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	. "github.com/onsi/gomega"
)

func TestGetPolicies(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	expectedPolicies := map[string]Policy{
		ReadersPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "ALL Member",
		},
		WritersPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "ANY Member",
		},
		AdminsPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "MAJORITY Member",
		},
		"SignaturePolicy": {
			Type: SignaturePolicyType,
			Rule: "AND('Org1.member', 'Org2.client', OR('Org3.peer', 'Org3.admin'), OUTOF(2, 'Org4.member', 'Org4.peer', 'Org4.admin'))",
		},
	}
	orgGroup := newConfigGroup()
	err := addPolicies(orgGroup, expectedPolicies, AdminsPolicyKey)
	gt.Expect(err).NotTo(HaveOccurred())

	policies, err := getPolicies(orgGroup.Policies)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(expectedPolicies).To(Equal(policies))

	policies, err = getPolicies(nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(map[string]Policy{}).To(Equal(policies))
}

func TestRemoveApplicationOrgPolicy(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()
	applicationGroup := newConfigGroup()

	application := baseApplication(t)

	for _, org := range application.Organizations {
		org.Policies = applicationOrgStandardPolicies()
		org.Policies["TestPolicy"] = Policy{
			Type: ImplicitMetaPolicyType,
			Rule: "MAJORITY Endorsement",
		}

		orgGroup, err := newOrgConfigGroup(org)
		gt.Expect(err).NotTo(HaveOccurred())

		applicationGroup.Groups[org.Name] = orgGroup
	}
	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	application.Organizations[0].Policies = applicationOrgStandardPolicies()
	expectedOrgConfigGroup, _ := newOrgConfigGroup(application.Organizations[0])
	expectedPolicies := expectedOrgConfigGroup.Policies

	err := RemoveApplicationOrgPolicy(config, "Org1", "TestPolicy")
	gt.Expect(err).NotTo(HaveOccurred())

	actualOrg1Policies := config.ChannelGroup.Groups[ApplicationGroupKey].Groups["Org1"].Policies
	gt.Expect(actualOrg1Policies).To(Equal(expectedPolicies))
}

func TestRemoveApplicationOrgPolicyFailures(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()
	applicationGroup := newConfigGroup()

	application := baseApplication(t)
	for _, org := range application.Organizations {
		org.Policies = applicationOrgStandardPolicies()
		orgGroup, err := newOrgConfigGroup(org)
		gt.Expect(err).NotTo(HaveOccurred())
		applicationGroup.Groups[org.Name] = orgGroup
	}
	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	err := RemoveApplicationOrgPolicy(config, "bad-org", "")
	gt.Expect(err).To(MatchError("application org bad-org does not exist in channel config"))
}

func TestAddApplicationOrgPolicy(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()
	applicationGroup := newConfigGroup()

	application := baseApplication(t)

	for _, org := range application.Organizations {
		org.Policies = applicationOrgStandardPolicies()

		orgGroup, err := newOrgConfigGroup(org)
		gt.Expect(err).NotTo(HaveOccurred())

		applicationGroup.Groups[org.Name] = orgGroup
	}
	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	application.Organizations[0].Policies = applicationOrgStandardPolicies()
	expectedOrgConfigGroup, _ := newOrgConfigGroup(application.Organizations[0])
	expectedPolicies := expectedOrgConfigGroup.Policies
	expectedPolicies["TestPolicy"] = expectedPolicies[EndorsementPolicyKey]

	err := AddApplicationOrgPolicy(config, "Org1", AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"})
	gt.Expect(err).NotTo(HaveOccurred())

	actualOrg1Policies := config.ChannelGroup.Groups[ApplicationGroupKey].Groups["Org1"].Policies
	gt.Expect(actualOrg1Policies).To(Equal(expectedPolicies))
}

func TestAddApplicationOrgPolicyFailures(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()
	applicationGroup := newConfigGroup()

	application := baseApplication(t)
	for _, org := range application.Organizations {
		org.Policies = applicationOrgStandardPolicies()

		orgGroup, err := newOrgConfigGroup(org)
		gt.Expect(err).NotTo(HaveOccurred())

		applicationGroup.Groups[org.Name] = orgGroup
	}
	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	err := AddApplicationOrgPolicy(config, "Org1", AdminsPolicyKey, "TestPolicy", Policy{})
	gt.Expect(err).To(MatchError("failed to add policy 'TestPolicy': unknown policy type: "))
}

func TestAddApplicationPolicy(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()
	application := baseApplication(t)

	applicationGroup, err := newApplicationGroup(application)
	gt.Expect(err).NotTo(HaveOccurred())

	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	err = AddApplicationPolicy(config, AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"})
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(applicationGroup.Policies["TestPolicy"]).NotTo(BeNil())
	gt.Expect(applicationGroup.Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Policies[WritersPolicyKey]).NotTo(BeNil())
}

func TestAddApplicationPolicyFailures(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()
	application := baseApplication(t)

	applicationGroup, err := newApplicationGroup(application)
	gt.Expect(err).NotTo(HaveOccurred())

	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	expectedPolicies := application.Policies
	expectedPolicies["TestPolicy"] = expectedPolicies[EndorsementPolicyKey]

	err = AddApplicationPolicy(config, AdminsPolicyKey, "TestPolicy", Policy{})
	gt.Expect(err).To(MatchError("failed to add policy 'TestPolicy': unknown policy type: "))
}

func TestRemoveApplicationPolicy(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()
	application := baseApplication(t)

	applicationGroup, err := newApplicationGroup(application)
	gt.Expect(err).NotTo(HaveOccurred())
	applicationGroup.Policies["TestPolicy"] = applicationGroup.Policies[AdminsPolicyKey]

	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	err = RemoveApplicationPolicy(config, "TestPolicy")
	gt.Expect(err).NotTo(HaveOccurred())

	actualPolicies := config.ChannelGroup.Groups[ApplicationGroupKey].Policies
	gt.Expect(actualPolicies).To(HaveLen(3))
	gt.Expect(applicationGroup.Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Policies[WritersPolicyKey]).NotTo(BeNil())
}

func TestRemoveApplicationPolicyFailures(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()
	application := baseApplication(t)

	applicationGroup, err := newApplicationGroup(application)
	gt.Expect(err).NotTo(HaveOccurred())

	channelGroup.Groups[ApplicationGroupKey] = applicationGroup

	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	tests := []struct {
		testName    string
		configMod   func(*cb.Config) *cb.Config
		expectedErr string
	}{
		{
			testName: "when application is missing",
			configMod: func(c *cb.Config) *cb.Config {
				delete(c.ChannelGroup.Groups, ApplicationGroupKey)
				return c
			},
			expectedErr: "application missing from config",
		},
		{
			testName: "when policy does not exist",
			configMod: func(c *cb.Config) *cb.Config {
				c.ChannelGroup.Groups[ApplicationGroupKey] = applicationGroup
				return c
			},
			expectedErr: "could not find policy 'TestPolicy'",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			gt := NewGomegaWithT(t)

			err = RemoveApplicationPolicy(tt.configMod(config), "TestPolicy")
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestAddConsortiumOrgPolicy(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums(t)

	consortiumsGroup, err := newConsortiumsGroup(consortiums)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				ConsortiumsGroupKey: consortiumsGroup,
			},
		},
	}

	consortium1Org1 := config.ChannelGroup.Groups[ConsortiumsGroupKey].Groups["Consortium1"].Groups["Org1"]

	err = AddConsortiumOrgPolicy(config, "Consortium1", "Org1", "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"})
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(consortium1Org1.Policies).To(HaveLen(5))
	gt.Expect(consortium1Org1.Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(consortium1Org1.Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortium1Org1.Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortium1Org1.Policies[EndorsementPolicyKey]).NotTo(BeNil())
	gt.Expect(consortium1Org1.Policies["TestPolicy"]).NotTo(BeNil())
}

func TestAddConsortiumOrgPolicyFailures(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums(t)

	consortiumsGroup, err := newConsortiumsGroup(consortiums)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				ConsortiumsGroupKey: consortiumsGroup,
			},
		},
	}

	for _, test := range []struct {
		name        string
		consortium  string
		org         string
		policy      Policy
		expectedErr string
	}{
		{
			name:        "When consortium does not exist in consortiums",
			consortium:  "BadConsortium",
			org:         "Org1",
			policy:      Policy{},
			expectedErr: "consortium 'BadConsortium' does not exist in channel config",
		},
		{
			name:        "When org does not exist",
			consortium:  "Consortium1",
			org:         "bad-org",
			policy:      Policy{},
			expectedErr: "consortiums org 'bad-org' does not exist in channel config",
		},
		{
			name:        "When adding empty policy fails",
			consortium:  "Consortium1",
			org:         "Org1",
			policy:      Policy{},
			expectedErr: "failed to add policy 'TestPolicy' to consortium org 'Org1': unknown policy type: ",
		},
	} {
		err := AddConsortiumOrgPolicy(config, test.consortium, test.org, "TestPolicy", test.policy)
		gt.Expect(err).To(MatchError(test.expectedErr))
	}
}

func TestRemoveConsortiumOrgPolicy(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums(t)

	consortiums[0].Organizations[0].Policies["TestPolicy"] = Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"}

	consortiumsGroup, err := newConsortiumsGroup(consortiums)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				ConsortiumsGroupKey: consortiumsGroup,
			},
		},
	}

	consortium1Org1 := config.ChannelGroup.Groups[ConsortiumsGroupKey].Groups["Consortium1"].Groups["Org1"]

	err = RemoveConsortiumOrgPolicy(config, "Consortium1", "Org1", "TestPolicy")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(consortium1Org1.Policies).To(HaveLen(4))
	gt.Expect(consortium1Org1.Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(consortium1Org1.Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortium1Org1.Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortium1Org1.Policies[EndorsementPolicyKey]).NotTo(BeNil())
}

func TestRemoveConsortiumOrgPolicyFailures(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums(t)

	consortiumsGroup, err := newConsortiumsGroup(consortiums)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				ConsortiumsGroupKey: consortiumsGroup,
			},
		},
	}

	for _, test := range []struct {
		name        string
		consortium  string
		org         string
		expectedErr string
	}{
		{
			name:        "When consortium does not exist in consortiums",
			consortium:  "BadConsortium",
			org:         "Org1",
			expectedErr: "consortium 'BadConsortium' does not exist in channel config",
		},
		{
			name:        "When org does not exist",
			consortium:  "Consortium1",
			org:         "bad-org",
			expectedErr: "consortiums org 'bad-org' does not exist in channel config",
		},
	} {
		err := RemoveConsortiumOrgPolicy(config, test.consortium, test.org, "TestPolicy")
		gt.Expect(err).To(MatchError(test.expectedErr))
	}
}

func TestAddOrdererPolicy(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)

	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Orderer": ordererGroup,
			},
		},
	}

	ordererConfigGroup := config.ChannelGroup.Groups[OrdererGroupKey]

	err = AddOrdererPolicy(config, AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "ANY Endorsement"})
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(ordererConfigGroup.Policies).To(HaveLen(5))
	gt.Expect(ordererConfigGroup.Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererConfigGroup.Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererConfigGroup.Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererConfigGroup.Policies["TestPolicy"]).NotTo(BeNil())
	gt.Expect(ordererConfigGroup.Policies[BlockValidationPolicyKey]).NotTo(BeNil())
}

func TestAddOrdererPolicyFailures(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)

	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Orderer": ordererGroup,
			},
		},
	}

	err = AddOrdererPolicy(config, AdminsPolicyKey, "TestPolicy", Policy{})
	gt.Expect(err).To(MatchError("failed to add policy 'TestPolicy': unknown policy type: "))
}

func TestRemoveOrdererPolicy(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)
	baseOrdererConf.Policies["TestPolicy"] = baseOrdererConf.Policies[AdminsPolicyKey]

	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Orderer": ordererGroup,
			},
		},
	}

	ordererConfigGroup := config.ChannelGroup.Groups[OrdererGroupKey]

	err = RemoveOrdererPolicy(config, "TestPolicy")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(ordererConfigGroup.Policies).To(HaveLen(4))
	gt.Expect(ordererConfigGroup.Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererConfigGroup.Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererConfigGroup.Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererConfigGroup.Policies[BlockValidationPolicyKey]).NotTo(BeNil())
}

func TestRemoveOrdererPolicyFailures(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)
	baseOrdererConf.Policies["TestPolicy"] = baseOrdererConf.Policies[AdminsPolicyKey]

	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: ordererGroup,
			},
		},
	}

	tests := []struct {
		testName      string
		ordererGrpMod func(cb.ConfigGroup) *cb.ConfigGroup
		policyName    string
		expectedErr   string
	}{
		{
			testName: "when removing blockvalidation policy",
			ordererGrpMod: func(og cb.ConfigGroup) *cb.ConfigGroup {
				return &og
			},
			policyName:  BlockValidationPolicyKey,
			expectedErr: "BlockValidation policy must be defined",
		},
		{
			testName: "when orderer is missing",
			ordererGrpMod: func(og cb.ConfigGroup) *cb.ConfigGroup {
				return nil
			},
			policyName:  "TestPolicy",
			expectedErr: "orderer missing from config",
		},
		{
			testName: "when policy does not exist",
			ordererGrpMod: func(og cb.ConfigGroup) *cb.ConfigGroup {
				delete(og.Policies, "TestPolicy")
				return &og
			},
			policyName:  "TestPolicy",
			expectedErr: "could not find policy 'TestPolicy'",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			gt := NewGomegaWithT(t)

			orderer := tt.ordererGrpMod(*ordererGroup)
			if orderer == nil {
				delete(config.ChannelGroup.Groups, OrdererGroupKey)
			} else {
				config.ChannelGroup.Groups[OrdererGroupKey] = orderer
			}

			err = RemoveOrdererPolicy(config, tt.policyName)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestAddOrdererOrgPolicy(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)

	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Orderer": ordererGroup,
			},
		},
	}

	ordererOrgConfigGroup := config.ChannelGroup.Groups[OrdererGroupKey].Groups["OrdererOrg"]

	err = AddOrdererOrgPolicy(config, "OrdererOrg", AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "ANY Endorsement"})
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(ordererOrgConfigGroup.Policies).To(HaveLen(5))
	gt.Expect(ordererOrgConfigGroup.Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererOrgConfigGroup.Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererOrgConfigGroup.Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererOrgConfigGroup.Policies["TestPolicy"]).NotTo(BeNil())
	gt.Expect(ordererOrgConfigGroup.Policies[EndorsementPolicyKey]).NotTo(BeNil())
}

func TestAddOrdererOrgPolicyFailures(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)

	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Orderer": ordererGroup,
			},
		},
	}

	err = AddOrdererOrgPolicy(config, "OrdererOrg", AdminsPolicyKey, "TestPolicy", Policy{})
	gt.Expect(err).To(MatchError("unknown policy type: "))
}

func TestRemoveOrdererOrgPolicy(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)
	baseOrdererConf.Organizations[0].Policies["TestPolicy"] = baseOrdererConf.Organizations[0].Policies[AdminsPolicyKey]

	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Orderer": ordererGroup,
			},
		},
	}

	ordererOrgConfigGroup := config.ChannelGroup.Groups[OrdererGroupKey].Groups["OrdererOrg"]

	err = RemoveOrdererOrgPolicy(config, "OrdererOrg", "TestPolicy")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(ordererOrgConfigGroup.Policies).To(HaveLen(4))
	gt.Expect(ordererOrgConfigGroup.Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererOrgConfigGroup.Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererOrgConfigGroup.Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(ordererOrgConfigGroup.Policies[EndorsementPolicyKey]).NotTo(BeNil())
}

func TestRemoveOrdererOrgPolicyFailures(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)

	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Orderer": ordererGroup,
			},
		},
	}

	err = RemoveOrdererOrgPolicy(config, "bad-org", "TestPolicy")
	gt.Expect(err).To(MatchError("orderer org bad-org does not exist in channel config"))
}
