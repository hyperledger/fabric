/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	. "github.com/onsi/gomega"
)

func TestPolicies(t *testing.T) {
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
	err := setPolicies(orgGroup, expectedPolicies, AdminsPolicyKey)
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

	c := New(config)

	application.Organizations[0].Policies = applicationOrgStandardPolicies()
	expectedOrgConfigGroup, _ := newOrgConfigGroup(application.Organizations[0])
	expectedPolicies := expectedOrgConfigGroup.Policies

	err := c.RemoveApplicationOrgPolicy("Org1", "TestPolicy")
	gt.Expect(err).NotTo(HaveOccurred())

	actualOrg1Policies := c.UpdatedConfig().ChannelGroup.Groups[ApplicationGroupKey].Groups["Org1"].Policies
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

	applicationGroup.Groups["Org1"].Policies["TestPolicy"] = &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type: 15,
		},
	}
	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	c := New(config)

	err := c.RemoveApplicationOrgPolicy("Org1", "TestPolicy")
	gt.Expect(err).To(MatchError("unknown policy type: 15"))
}

func TestSetApplicationOrgPolicy(t *testing.T) {
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

	c := New(config)

	application.Organizations[0].Policies = applicationOrgStandardPolicies()
	expectedOrgConfigGroup, _ := newOrgConfigGroup(application.Organizations[0])
	expectedPolicies := expectedOrgConfigGroup.Policies
	expectedPolicies["TestPolicy"] = expectedPolicies[EndorsementPolicyKey]

	err := c.SetApplicationOrgPolicy("Org1", AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"})
	gt.Expect(err).NotTo(HaveOccurred())

	actualOrg1Policies := c.UpdatedConfig().ChannelGroup.Groups[ApplicationGroupKey].Groups["Org1"].Policies
	gt.Expect(actualOrg1Policies).To(Equal(expectedPolicies))
}

func TestSetApplicationOrgPolicyFailures(t *testing.T) {
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

	c := New(config)

	err := c.SetApplicationOrgPolicy("Org1", AdminsPolicyKey, "TestPolicy", Policy{})
	gt.Expect(err).To(MatchError("failed to set policy 'TestPolicy': unknown policy type: "))
}

func TestSetApplicationPolicy(t *testing.T) {
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

	c := New(config)

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
		"TestPolicy": {
			Type: ImplicitMetaPolicyType,
			Rule: "MAJORITY Endorsement",
		},
	}

	err = c.SetApplicationPolicy(AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"})
	gt.Expect(err).NotTo(HaveOccurred())

	updatedPolicies, err := getPolicies(c.UpdatedConfig().ChannelGroup.Groups[ApplicationGroupKey].Policies)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(updatedPolicies).To(Equal(expectedPolicies))
}

func TestSetApplicationPolicyFailures(t *testing.T) {
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

	c := New(config)

	expectedPolicies := application.Policies
	expectedPolicies["TestPolicy"] = expectedPolicies[EndorsementPolicyKey]

	err = c.SetApplicationPolicy(AdminsPolicyKey, "TestPolicy", Policy{})
	gt.Expect(err).To(MatchError("failed to set policy 'TestPolicy': unknown policy type: "))
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

	c := New(config)

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
	}

	err = c.RemoveApplicationPolicy("TestPolicy")
	gt.Expect(err).NotTo(HaveOccurred())

	updatedPolicies, err := getPolicies(c.UpdatedConfig().ChannelGroup.Groups[ApplicationGroupKey].Policies)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(updatedPolicies).To(Equal(expectedPolicies))
}

func TestRemoveApplicationPolicyFailures(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()
	application := baseApplication(t)

	applicationGroup, err := newApplicationGroup(application)
	gt.Expect(err).NotTo(HaveOccurred())

	applicationGroup.Policies[EndorsementPolicyKey] = &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type: 15,
		},
	}
	channelGroup.Groups[ApplicationGroupKey] = applicationGroup

	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	c := New(config)

	err = c.RemoveApplicationPolicy("TestPolicy")
	gt.Expect(err).To(MatchError("unknown policy type: 15"))
}

func TestSetConsortiumOrgPolicy(t *testing.T) {
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

	c := New(config)

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
		"TestPolicy": {
			Type: ImplicitMetaPolicyType,
			Rule: "MAJORITY Endorsement",
		},
	}

	err = c.SetConsortiumOrgPolicy("Consortium1", "Org1", "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"})
	gt.Expect(err).NotTo(HaveOccurred())

	consortium1Org1 := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey].Groups["Consortium1"].Groups["Org1"]
	updatedPolicies, err := getPolicies(consortium1Org1.Policies)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(updatedPolicies).To(Equal(expectedPolicies))
}

func TestSetConsortiumOrgPolicyFailures(t *testing.T) {
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

	c := New(config)

	for _, test := range []struct {
		name        string
		consortium  string
		org         string
		policy      Policy
		expectedErr string
	}{
		{
			name:        "When setting empty policy fails",
			consortium:  "Consortium1",
			org:         "Org1",
			policy:      Policy{},
			expectedErr: "failed to set policy 'TestPolicy' to consortium org 'Org1': unknown policy type: ",
		},
	} {
		err := c.SetConsortiumOrgPolicy(test.consortium, test.org, "TestPolicy", test.policy)
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

	c := New(config)

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

	c.RemoveConsortiumOrgPolicy("Consortium1", "Org1", "TestPolicy")

	consortium1Org1 := c.UpdatedConfig().ChannelGroup.Groups[ConsortiumsGroupKey].Groups["Consortium1"].Groups["Org1"]
	updatedPolicies, err := getPolicies(consortium1Org1.Policies)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(updatedPolicies).To(Equal(expectedPolicies))
}

func TestSetOrdererPolicy(t *testing.T) {
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

	c := New(config)

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
		BlockValidationPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "ANY Writers",
		},
		"TestPolicy": {
			Type: ImplicitMetaPolicyType,
			Rule: "ANY Endorsement",
		},
	}

	err = c.UpdatedConfig().Orderer().SetPolicy(AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "ANY Endorsement"})
	gt.Expect(err).NotTo(HaveOccurred())

	updatedPolicies, err := c.UpdatedConfig().Orderer().Policies()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(updatedPolicies).To(Equal(expectedPolicies))
}

func TestSetOrdererPolicyFailures(t *testing.T) {
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

	c := New(config)

	err = c.UpdatedConfig().Orderer().SetPolicy(AdminsPolicyKey, "TestPolicy", Policy{})
	gt.Expect(err).To(MatchError("failed to set policy 'TestPolicy': unknown policy type: "))
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

	c := New(config)

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
		BlockValidationPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "ANY Writers",
		},
	}

	err = c.UpdatedConfig().Orderer().RemovePolicy("TestPolicy")
	gt.Expect(err).NotTo(HaveOccurred())

	updatedPolicies, err := c.UpdatedConfig().Orderer().Policies()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(updatedPolicies).To(Equal(expectedPolicies))
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

	c := New(config)

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
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			gt := NewGomegaWithT(t)

			ordererGroup := tt.ordererGrpMod(*ordererGroup)
			if ordererGroup == nil {
				delete(config.ChannelGroup.Groups, OrdererGroupKey)
			} else {
				config.ChannelGroup.Groups[OrdererGroupKey] = ordererGroup
			}

			err = c.UpdatedConfig().Orderer().RemovePolicy(tt.policyName)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestSetOrdererOrgPolicy(t *testing.T) {
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

	c := New(config)

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
		"TestPolicy": {
			Type: ImplicitMetaPolicyType,
			Rule: "ANY Endorsement",
		},
	}

	err = c.UpdatedConfig().Orderer().Organization("OrdererOrg").SetPolicy(AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "ANY Endorsement"})
	gt.Expect(err).NotTo(HaveOccurred())

	ordererOrgConfigGroup := c.UpdatedConfig().ChannelGroup.Groups[OrdererGroupKey].Groups["OrdererOrg"]
	updatedPolicies, err := getPolicies(ordererOrgConfigGroup.Policies)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(updatedPolicies).To(Equal(expectedPolicies))
}

func TestSetOrdererOrgPolicyFailures(t *testing.T) {
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

	c := New(config)

	err = c.UpdatedConfig().Orderer().Organization("OrdererOrg").SetPolicy(AdminsPolicyKey, "TestPolicy", Policy{})
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

	c := New(config)

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

	err = c.UpdatedConfig().Orderer().Organization("OrdererOrg").RemovePolicy("TestPolicy")
	gt.Expect(err).NotTo(HaveOccurred())

	updatedPolicies, err := c.UpdatedConfig().Orderer().Organization("OrdererOrg").Policies()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(updatedPolicies).To(Equal(expectedPolicies))
}

func TestSetConsortiumChannelCreationPolicy(t *testing.T) {
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

	c := New(config)

	updatedPolicy := Policy{Type: ImplicitMetaPolicyType, Rule: "MAJORITY Admins"}

	err = c.SetConsortiumChannelCreationPolicy("Consortium1", updatedPolicy)
	gt.Expect(err).NotTo(HaveOccurred())

	consortium := c.UpdatedConfig().ChannelGroup.Groups[ConsortiumsGroupKey].Groups["Consortium1"]
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

func TestSetConsortiumChannelCreationPolicyFailures(t *testing.T) {
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

	c := New(config)

	tests := []struct {
		name           string
		consortiumName string
		updatedpolicy  Policy
		expectedErr    string
	}{
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
			err := c.SetConsortiumChannelCreationPolicy(tt.consortiumName, tt.updatedpolicy)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestSetChannelPolicy(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channel, err := baseApplicationChannelGroup(t)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: channel,
	}
	c := New(config)

	expectedPolicies := map[string]Policy{
		"TestPolicy": {Type: ImplicitMetaPolicyType, Rule: "ANY Readers"},
	}

	err = c.SetChannelPolicy(AdminsPolicyKey, "TestPolicy", Policy{Type: ImplicitMetaPolicyType, Rule: "ANY Readers"})
	gt.Expect(err).NotTo(HaveOccurred())

	updatedChannelPolicy, err := getPolicies(c.updated.ChannelGroup.Policies)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(updatedChannelPolicy).To(Equal(expectedPolicies))

	baseChannel := c.original.ChannelGroup
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
	err = setPolicies(channel, policies, AdminsPolicyKey)
	gt.Expect(err).NotTo(HaveOccurred())
	c := New(config)

	expectedPolicies := map[string]Policy{
		"Admins": {
			Type: "ImplicitMeta",
			Rule: "MAJORITY Admins",
		},
		"Writers": {
			Type: "ImplicitMeta",
			Rule: "ANY Writers",
		},
	}

	err = c.RemoveChannelPolicy(ReadersPolicyKey)
	gt.Expect(err).NotTo(HaveOccurred())

	updatedChannelPolicy, err := getPolicies(c.UpdatedConfig().ChannelGroup.Policies)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(updatedChannelPolicy).To(Equal(expectedPolicies))

	originalChannel := c.OriginalConfig().ChannelGroup
	gt.Expect(originalChannel.Policies).To(HaveLen(3))
	gt.Expect(originalChannel.Policies[ReadersPolicyKey]).ToNot(BeNil())
}

func TestRemoveChannelPolicyFailures(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channel, err := baseApplicationChannelGroup(t)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: channel,
	}
	policies := standardPolicies()
	err = setPolicies(channel, policies, AdminsPolicyKey)
	gt.Expect(err).NotTo(HaveOccurred())
	channel.Policies[ReadersPolicyKey] = &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type: 15,
		},
	}
	c := New(config)

	err = c.RemoveChannelPolicy(ReadersPolicyKey)
	gt.Expect(err).To(MatchError("unknown policy type: 15"))
}

func TestConsortiumOrgPolicies(t *testing.T) {
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

	c := New(config)

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

	policies, err := c.ConsortiumOrgPolicies("Consortium1", "Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(policies).To(Equal(expectedPolicies))
}

func TestConsortiumOrgPoliciesFailures(t *testing.T) {
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

	c := New(config)

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
			_, err = c.ConsortiumOrgPolicies(tt.consortiumName, tt.orgName)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}
