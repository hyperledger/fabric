/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/tools/protolator"
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
	consortiums[0].Organizations[0].Policies = nil

	mspConfig := &mb.MSPConfig{}

	consortiumsGroup, err := NewConsortiumsGroup(consortiums, mspConfig)
	gt.Expect(err).To(MatchError("org group 'Org1': no policies defined"))
	gt.Expect(consortiumsGroup).To(BeNil())
}

func TestAddOrgToConsortium(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums()

	consortiumsGroup, err := NewConsortiumsGroup(consortiums, &mb.MSPConfig{})
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Consortiums": consortiumsGroup,
			},
		},
	}

	orgToAdd := &Organization{
		Name:     "Org1",
		ID:       "Org1MSP",
		Policies: applicationOrgStandardPolicies(),
	}

	expectedConfig := `
{
	"channel_group": {
		"groups": {
			"Consortiums": {
				"groups": {
					"Consortium1": {
						"groups": {
							"Org1": {
								"groups": {},
								"mod_policy": "Admins",
								"policies": {
									"Admins": {
									"mod_policy": "Admins",
									"policy": {
									"type": 3,
									"value": {
									"rule": "MAJORITY",
									"sub_policy": "Admins"
									}
									},
									"version": "0"
									},
									"Endorsement": {
									"mod_policy": "Admins",
									"policy": {
									"type": 3,
									"value": {
									"rule": "MAJORITY",
									"sub_policy": "Endorsement"
									}
									},
									"version": "0"
									},
									"LifecycleEndorsement": {
									"mod_policy": "Admins",
									"policy": {
									"type": 3,
									"value": {
									"rule": "MAJORITY",
									"sub_policy": "Endorsement"
									}
									},
									"version": "0"
									},
									"Readers": {
									"mod_policy": "Admins",
									"policy": {
									"type": 3,
									"value": {
									"rule": "ANY",
									"sub_policy": "Readers"
									}
									},
									"version": "0"
									},
									"Writers": {
									"mod_policy": "Admins",
									"policy": {
									"type": 3,
									"value": {
									"rule": "ANY",
									"sub_policy": "Writers"
									}
									},
									"version": "0"
									}
								},
								"values": {
									"MSP": {
									"mod_policy": "Admins",
									"value": {
									"config": null,
									"type": 0
									},
									"version": "0"
									}
								},
								"version": "0"
							},
							"Org2": {
								"groups": {},
								"mod_policy": "Admins",
								"policies": {
									"Admins": {
									"mod_policy": "Admins",
									"policy": {
									"type": 3,
									"value": {
									"rule": "MAJORITY",
									"sub_policy": "Admins"
									}
									},
									"version": "0"
									},
									"Endorsement": {
									"mod_policy": "Admins",
									"policy": {
									"type": 3,
									"value": {
									"rule": "MAJORITY",
									"sub_policy": "Endorsement"
									}
									},
									"version": "0"
									},
									"Readers": {
									"mod_policy": "Admins",
									"policy": {
									"type": 3,
									"value": {
									"rule": "ANY",
									"sub_policy": "Readers"
									}
									},
									"version": "0"
									},
									"Writers": {
									"mod_policy": "Admins",
									"policy": {
									"type": 3,
									"value": {
									"rule": "ANY",
									"sub_policy": "Writers"
									}
									},
									"version": "0"
									}
								},
								"values": {
									"MSP": {
									"mod_policy": "Admins",
									"value": {
									"config": null,
									"type": 0
									},
									"version": "0"
									}
								},
								"version": "0"
							}
						},
						"mod_policy": "/Channel/Orderer/Admins",
						"policies": {},
						"values": {
							"ChannelCreationPolicy": {
								"mod_policy": "/Channel/Orderer/Admins",
								"value": {
									"type": 3,
									"value": {
									"rule": "ANY",
									"sub_policy": "Admins"
									}
								},
								"version": "0"
							}
						},
						"version": "0"
					}
				},
				"mod_policy": "/Channel/Orderer/Admins",
				"policies": {
					"Admins": {
						"mod_policy": "/Channel/Orderer/Admins",
						"policy": {
							"type": 1,
							"value": {
								"identities": [],
								"rule": {
									"n_out_of": {
									"n": 0,
									"rules": []
									}
								},
								"version": 0
							}
						},
						"version": "0"
					}
				},
				"values": {},
				"version": "0"
			}
		},
		"mod_policy": "",
		"policies": {},
		"values": {},
		"version": "0"
	},
	"sequence": "0"
}
`

	expectedConfigProto := cb.Config{}
	err = protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedConfig), &expectedConfigProto)
	gt.Expect(err).NotTo(HaveOccurred())

	err = AddOrgToConsortium(config, orgToAdd, "Consortium1", &mb.MSPConfig{})
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(proto.Equal(config, &expectedConfigProto)).To(BeTrue())
}

func TestAddOrgToConsortiumFailures(t *testing.T) {
	t.Parallel()

	orgToAdd := &Organization{
		Name:     "test-org",
		ID:       "test-org-msp-id",
		Policies: applicationOrgStandardPolicies(),
	}

	for _, test := range []struct {
		name        string
		org         *Organization
		consortium  string
		config      *cb.Config
		expectedErr string
	}{
		{
			name:        "When the organization is nil",
			org:         nil,
			consortium:  "Consortium1",
			expectedErr: "organization is required",
		},
		{
			name:        "When the consortium name is not specified",
			org:         orgToAdd,
			consortium:  "",
			expectedErr: "consortium is required",
		},
		{
			name:        "When the config doesn't contain the consortium",
			org:         orgToAdd,
			consortium:  "what-the-what",
			expectedErr: "consortium 'what-the-what' does not exist",
		},
		{
			name: "When the config doesn't contain the consortium",
			org: &Organization{
				Name: "test-msp",
				ID:   "test-org-msp-id",
				Policies: map[string]*Policy{
					"Admins": nil,
				},
			},
			consortium:  "Consortium1",
			expectedErr: "failed to create consortium org: no Admins policy defined",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)

			consortiums := baseConsortiums()

			consortiumsGroup, err := NewConsortiumsGroup(consortiums, &mb.MSPConfig{})
			gt.Expect(err).NotTo(HaveOccurred())

			config := &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Groups: map[string]*cb.ConfigGroup{
						"Consortiums": consortiumsGroup,
					},
				},
			}

			err = AddOrgToConsortium(config, test.org, test.consortium, &mb.MSPConfig{})
			gt.Expect(err).To(MatchError(test.expectedErr))
		})
	}
}

func TestSkipAsForeignForConsortiumOrg(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums()
	consortiums[0].Organizations[0].SkipAsForeign = true
	consortiums[0].Organizations[1].SkipAsForeign = true

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

func baseConsortiums() []*Consortium {
	return []*Consortium{
		{
			Name: "Consortium1",
			Organizations: []*Organization{
				{
					Name:     "Org1",
					ID:       "Org1MSP",
					Policies: orgStandardPolicies(),
				},
				{
					Name:     "Org2",
					ID:       "Org2MSP",
					Policies: orgStandardPolicies(),
				},
			},
		},
	}
}
