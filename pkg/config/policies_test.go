/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"testing"

	"github.com/golang/protobuf/proto"
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	application.Organizations[0].Policies = applicationOrgStandardPolicies()
	expectedOrgConfigGroup, _ := newOrgConfigGroup(application.Organizations[0])
	expectedPolicies := expectedOrgConfigGroup.Policies

	err := c.RemoveApplicationOrgPolicy("Org1", "TestPolicy")
	gt.Expect(err).NotTo(HaveOccurred())

	actualOrg1Policies := c.updated.ChannelGroup.Groups[ApplicationGroupKey].Groups["Org1"].Policies
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	err := c.RemoveApplicationOrgPolicy("bad-org", "")
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	application.Organizations[0].Policies = applicationOrgStandardPolicies()
	expectedOrgConfigGroup, _ := newOrgConfigGroup(application.Organizations[0])
	expectedPolicies := expectedOrgConfigGroup.Policies
	expectedPolicies["TestPolicy"] = expectedPolicies[EndorsementPolicyKey]

	err := c.AddApplicationOrgPolicy("Org1", AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"})
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	err := c.AddApplicationOrgPolicy("Org1", AdminsPolicyKey, "TestPolicy", Policy{})
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	err = c.AddApplicationPolicy(AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"})
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	expectedPolicies := application.Policies
	expectedPolicies["TestPolicy"] = expectedPolicies[EndorsementPolicyKey]

	err = c.AddApplicationPolicy(AdminsPolicyKey, "TestPolicy", Policy{})
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	err = c.RemoveApplicationPolicy("TestPolicy")
	gt.Expect(err).NotTo(HaveOccurred())

	actualPolicies := c.updated.ChannelGroup.Groups[ApplicationGroupKey].Policies
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

	c := ConfigTx{
		base:    config,
		updated: config,
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

			tt.configMod(c.updated)
			err = c.RemoveApplicationPolicy("TestPolicy")
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	consortium1Org1 := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey].Groups["Consortium1"].Groups["Org1"]

	err = c.AddConsortiumOrgPolicy("Consortium1", "Org1", "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"})
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

	c := ConfigTx{
		base:    config,
		updated: config,
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
		err := c.AddConsortiumOrgPolicy(test.consortium, test.org, "TestPolicy", test.policy)
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	consortium1Org1 := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey].Groups["Consortium1"].Groups["Org1"]

	err = c.RemoveConsortiumOrgPolicy("Consortium1", "Org1", "TestPolicy")
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

	c := ConfigTx{
		base:    config,
		updated: config,
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
		err := c.RemoveConsortiumOrgPolicy(test.consortium, test.org, "TestPolicy")
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	ordererConfigGroup := c.updated.ChannelGroup.Groups[OrdererGroupKey]

	err = c.AddOrdererPolicy(AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "ANY Endorsement"})
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	err = c.AddOrdererPolicy(AdminsPolicyKey, "TestPolicy", Policy{})
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	ordererConfigGroup := c.updated.ChannelGroup.Groups[OrdererGroupKey]

	err = c.RemoveOrdererPolicy("TestPolicy")
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

	c := ConfigTx{
		base:    config,
		updated: config,
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

			err = c.RemoveOrdererPolicy(tt.policyName)
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	ordererOrgConfigGroup := c.updated.ChannelGroup.Groups[OrdererGroupKey].Groups["OrdererOrg"]

	err = c.AddOrdererOrgPolicy("OrdererOrg", AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "ANY Endorsement"})
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	err = c.AddOrdererOrgPolicy("OrdererOrg", AdminsPolicyKey, "TestPolicy", Policy{})
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	ordererOrgConfigGroup := c.updated.ChannelGroup.Groups[OrdererGroupKey].Groups["OrdererOrg"]

	err = c.RemoveOrdererOrgPolicy("OrdererOrg", "TestPolicy")
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

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	err = c.RemoveOrdererOrgPolicy("bad-org", "TestPolicy")
	gt.Expect(err).To(MatchError("orderer org bad-org does not exist in channel config"))
}

func TestUpdateConsortiumChannelCreationPolicy(t *testing.T) {
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
	c := &ConfigTx{
		base:    config,
		updated: config,
	}

	updatedPolicy := Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Admins"}

	err = c.UpdateConsortiumChannelCreationPolicy("Consortium1", updatedPolicy)
	gt.Expect(err).NotTo(HaveOccurred())

	consortium := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey].Groups["Consortium1"]
	creationPolicy := consortium.Values[ChannelCreationPolicyKey]
	policy := &cb.Policy{}
	err = proto.Unmarshal(creationPolicy.Value, policy)
	gt.Expect(err).NotTo(HaveOccurred())
	imp := &cb.ImplicitMetaPolicy{}
	err = proto.Unmarshal(policy.Value, imp)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(imp.Rule).To(Equal(cb.ImplicitMetaPolicy_MAJORITY))
	gt.Expect(imp.SubPolicy).To(Equal("Admins"))
}

func TestUpdateConsortiumChannelCreationPolicyFailures(t *testing.T) {
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
	c := &ConfigTx{
		base:    config,
		updated: config,
	}

	tests := []struct {
		name           string
		consortiumName string
		updatedpolicy  Policy
		expectedErr    string
	}{
		{
			name:           "when consortium does not exist in channel config",
			consortiumName: "badConsortium",
			updatedpolicy:  Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Admins"},
			expectedErr:    "consortium badConsortium does not exist in channel config",
		},
		{
			name:           "when policy is invalid",
			consortiumName: "Consortium1",
			updatedpolicy:  Policy{Type: ImplicitMetaPolicyType, Rule: "Bad Admins"},
			expectedErr:    "invalid implicit meta policy rule 'Bad Admins': unknown rule type 'Bad', expected ALL, ANY, or MAJORITY",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			gt := NewGomegaWithT(t)
			err := c.UpdateConsortiumChannelCreationPolicy(tt.consortiumName, tt.updatedpolicy)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestAddChannelPolicy(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channel, err := baseApplicationChannelGroup(t)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: channel,
	}
	c := New(config)

	expectedPolicy := Policy{Type: ImplicitMetaPolicyType, Rule: "ANY Readers"}

	err = c.AddChannelPolicy(AdminsPolicyKey, "TestPolicy", expectedPolicy)
	gt.Expect(err).NotTo(HaveOccurred())

	updatedChannel := c.updated.ChannelGroup
	baseChannel := c.base.ChannelGroup
	gt.Expect(updatedChannel.Policies).To(HaveLen(1))
	gt.Expect(updatedChannel.Policies["TestPolicy"]).NotTo(BeNil())
	gt.Expect(baseChannel.Policies).To(HaveLen(0))
	gt.Expect(baseChannel.Policies["TestPolicy"]).To(BeNil())
}

func TestRemoveChannelPolicy(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channel, err := baseApplicationChannelGroup(t)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: channel,
	}
	policies := standardPolicies()
	addPolicies(channel, policies, AdminsPolicyKey)
	gt.Expect(err).NotTo(HaveOccurred())
	c := New(config)

	err = c.RemoveChannelPolicy(ReadersPolicyKey)
	gt.Expect(err).NotTo(HaveOccurred())

	updatedChannel := c.updated.ChannelGroup
	baseChannel := c.base.ChannelGroup
	gt.Expect(updatedChannel.Policies).To(HaveLen(2))
	gt.Expect(updatedChannel.Policies[ReadersPolicyKey]).To(BeNil())
	gt.Expect(baseChannel.Policies).To(HaveLen(3))
	gt.Expect(baseChannel.Policies[ReadersPolicyKey]).ToNot(BeNil())
}

func TestGetPoliciesForConsortiumOrg(t *testing.T) {
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
	c := &ConfigTx{
		base:    config,
		updated: config,
	}

	expectedPolicies := map[string]Policy{
		ReadersPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "ANY Readers",
		},
		WritersPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "ANY Writers",
		},
		AdminsPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "MAJORITY Admins",
		},
		EndorsementPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "MAJORITY Endorsement",
		},
	}

	policies, err := c.GetPoliciesForConsortiumOrg("Consortium1", "Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(policies).To(Equal(expectedPolicies))
}

func TestGetPoliciesForConsortiumOrgFailures(t *testing.T) {
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
	c := &ConfigTx{
		base:    config,
		updated: config,
	}

	tests := []struct {
		testName       string
		consortiumName string
		orgName        string
		expectedErr    string
	}{
		{
			testName:       "when consortium does not exist",
			consortiumName: "BadConsortium",
			orgName:        "Org1",
			expectedErr:    "consortium BadConsortium does not exist in channel config",
		},
		{
			testName:       "when org does not exist",
			consortiumName: "Consortium1",
			orgName:        "BadOrg",
			expectedErr:    "consortium org BadOrg does not exist in channel config",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			_, err = c.GetPoliciesForConsortiumOrg(tt.consortiumName, tt.orgName)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}
